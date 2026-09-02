---
type: memory
description: "`wt open --list` / `--list --json` app-registry contract — JSON = the complete locus-aware registry (`{id, label, kind, locus}` on every record plus an omitempty `default` marker, full catalog in detection order, round-trip invariant with `-a` both directions); human table = the host-app (`locus == gui`) view with empty-case pointer copy; kind/locus enums with a fixed per-Cmd mapping; flag exclusivity, `[]`-not-`null`, no-git-required."
---
# wt-cli: Open List Contract

**Domain**: wt-cli

> Post-implementation behavior capture for the `wt open --list` query surface.
> Source change: `260722-qj66-open-list-json`.

## Overview

`wt open --list` is a pure **query** on `wt open`'s detected-app catalog: it
reports launch targets and exits without opening anything. Plain `--list`
prints a human table of the host GUI applications (editors, terminals, file
managers). `--list --json` is the **complete machine registry**: one record
per `BuildAvailableApps()` catalog entry, in detection order, carrying both
`kind` (what the target is) and `locus` (where its effect lands), with every
filtering decision left to the consumer. It exists so external consumers —
run-kit's `GET /api/open-apps` dropdown, and agents running on the host
itself inside a tmux/byobu session — can (a) discover the launch targets
valid *in this environment* and (b) validate an app id before launching via
the existing non-interactive path `wt open <path> -a <id>`, without forking
`wt`'s app-detection catalog. The `wt open` invocation surface is specified
in `docs/specs/launcher-contract.md` §2; this file records the query form's
runtime invariants.

## Requirements

### Requirement: `--list` reports detected targets without menu, launch, or git

`wt open --list` MUST print the detected-target listing and exit
`ExitSuccess` (0) — no interactive menu, no app launch, no positional target
consumed. The `--list` branch MUST run before the soft git-context detection
in `openCmd`'s `RunE`, so it works from any cwd (including a non-git
directory — external servers invoke it from arbitrary locations). Detection
MUST be the single `wt.BuildAvailableApps()` call the interactive menu and
`-a` resolution use — there is no second detection path.

#### Scenario: list from a non-git directory
- **GIVEN** a non-git working directory (e.g. a fresh temp dir)
- **WHEN** the user runs `wt open --list`
- **THEN** the command exits 0 and prints the detected-target listing, with
  no git invocation required and no app launched

### Requirement: `AppInfo.Kind` and `AppInfo.Locus` classify every catalog entry

`AppInfo` (`src/internal/worktree/apps.go`) carries `Kind string` and
`Locus string` fields, both populated at every append site in
`BuildAvailableApps()` from the named constants. `Kind` is one of
`AppKindEditor` / `AppKindTerminal` / `AppKindFileManager` /
`AppKindMultiplexer` / `AppKindShell` / `AppKindClipboard` (values `editor` /
`terminal` / `file-manager` / `multiplexer` / `shell` / `clipboard`).
`Locus` is one of `LocusGUI` (a window on the host display) / `LocusSession`
(the effect lands inside the invoking tmux/byobu server) / `LocusCaller`
(cooperates with the invoking shell via `WT_CD_FILE`/stdout) / `LocusHost`
(mutates host state observable only locally, e.g. the clipboard). The mapping
is fixed:

| Cmd key | Kind | Locus |
|---------|------|-------|
| `code`, `cursor` | `editor` | `gui` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` | `gui` |
| `finder`, `nautilus`, `dolphin` | `file-manager` | `gui` |
| `tmux_window`, `tmux_session`, `byobu_tab` | `multiplexer` | `session` |
| `open_here` | `shell` | `caller` |
| `copy_macos`, `copy_linux` | `clipboard` | `host` |

Both fields are additive metadata: the interactive menu, `-a` resolution
(`ResolveApp`), `DetectDefaultApp`, and `SaveLastApp` are untouched by their
presence. Detection is environment-gated: `tmux_window`/`tmux_session`
appear only under `IsTmuxSession()`, `byobu_tab` only under
`IsByobuSession()`, and `copy_*` only when the clipboard binary exists.

### Requirement: the human table is the host-app (`locus == gui`) view

The plain `--list` table filters to entries with `Locus == LocusGUI`, via
the `ListableApps(apps []AppInfo) []AppInfo` helper in
`src/internal/worktree/apps.go` (the filter rule lives beside the catalog
per Constitution V; `cmd/` stays orchestration-only). The non-GUI targets —
`open_here` (shell-cd), `copy_macos` / `copy_linux` (clipboard),
`byobu_tab` / `tmux_window` / `tmux_session` (multiplexer) — are absent from
the human table but remain in the interactive menu and remain valid `-a`
values. `ListableApps` preserves input order and always returns a non-nil
slice.

#### Scenario: filter a mixed catalog
- **GIVEN** a catalog slice containing both host apps and non-GUI targets
- **WHEN** `ListableApps` is applied
- **THEN** only `locus == gui` entries remain, in their original order
- **AND** the interactive menu and `-a` resolution still see the full
  unfiltered catalog

### Requirement: human table output (no `--json`)

`wt open --list` without `--json` prints a small aligned `Id` / `Label` /
`Kind` table (`Id` = `AppInfo.Cmd`, `Label` = `AppInfo.Name`, `Kind` =
`AppInfo.Kind`), with column widths computed from max cell width and
two-space padding — mirroring `wt list`'s human-default / `--json`-opt-in
split. When zero host applications are detected, human mode prints
`No host applications detected. {N} other target(s) available — see 'wt open --list --json' or the interactive menu.`
where `{N}` is the full catalog size minus the host-app rows (always ≥ 1,
since `open_here` is unconditional), and exits 0. No per-row detection cost
is added beyond what `BuildAvailableApps()` already pays.

### Requirement: `--list --json` is the complete registry with a round-trip invariant

`wt open --list --json` emits one record per `BuildAvailableApps()` catalog
entry — the unfiltered catalog, in detection order — as a JSON array of
records with keys `id`, `label`, `kind`, `locus` present on every record
(no `omitempty` on these four). The row `DetectDefaultApp()` selects — the
same row `-a default` resolves to — additionally carries `"default": true`;
every other row omits the key, and all rows omit it when `DetectDefaultApp`
returns -1. The human table does not render the marker. The machine
invariant holds in both directions: every emitted `id` resolves via
`wt open <target> -a <id>` in the current environment, and every id `-a`
resolves in the current environment is emitted.

- **`id`** = `AppInfo.Cmd` emitted verbatim (e.g. `code`, `ghostty_macos`,
  `tmux_window`). This is the exact token `wt open <path> -a <id>` accepts —
  `ResolveApp` matches `Cmd` first — so the output is a **validation source**
  for a consumer's launch path. Platform-suffixed keys (`ghostty_macos` vs
  `ghostty_linux`) are emitted as-is; the consumer round-trips ids and never
  interprets them.
- **`label`** = `AppInfo.Name` (the display name, e.g. `VSCode`,
  `Terminal.app`).
- **`kind`** = one of `editor` / `terminal` / `file-manager` / `multiplexer`
  / `shell` / `clipboard` (the closed enum).
- **`locus`** = one of `gui` / `session` / `caller` / `host` (the closed
  enum).

Zero catalog entries MUST emit `[]` — an empty JSON array, **not** `null` —
and exit 0; the record slice is initialized non-nil
(`make([]openAppRecord, 0, …)`) because a nil Go slice marshals to `null`.
Encoding mirrors `wt list --json`'s `handleJSONOutput`:
`json.MarshalIndent(…, "", "  ")` (two-space indent) printed with a trailing
newline.

#### Scenario: json record shape and id round-trip
- **GIVEN** a host with detected targets
- **WHEN** the user runs `wt open --list --json`
- **THEN** stdout parses as a JSON array where every record has the keys
  `id`, `label`, `kind`, `locus`; `kind` and `locus` are in their closed
  enums; the records match the full catalog in detection order; and each
  `id` resolves via `wt open <dir> -a <id>`

#### Scenario: default marker follows DetectDefaultApp
- **GIVEN** a plain tmux session with no `TERM_PROGRAM` editor signal and no
  last-app cache hit
- **WHEN** the user runs `wt open --list --json`
- **THEN** the `tmux_window` record carries `"default": true` and no other
  record has the key
- **AND** when `DetectDefaultApp` returns -1, no record carries the key

#### Scenario: empty catalog emits an array
- **GIVEN** zero catalog entries
- **WHEN** the JSON emitter runs on the empty slice
- **THEN** it emits `[]`, not `null`, and the command exits 0

### Requirement: deterministic ordering

The `--list` output (both modes) preserves `BuildAvailableApps()` detection
order — the JSON form emits the catalog exactly as detected; the human form
shows the same order over the `locus == gui` subset. No re-sorting is
applied. The catalog construction is deterministic per host state, so
machine consumers get stable output (Constitution VI).

### Requirement: flag exclusivity at flag-check time

`wt open` rejects the following combinations with `ExitInvalidArgs` (2) and
a what/why/fix message, following `wt list`'s
`ExitWithError(wt.ExitInvalidArgs, …)` mutex idiom. Validation happens at
flag-check time, before any detection or git work, and uses only existing
exit-code constants:

- `--list` + a positional target → `ExitInvalidArgs`
- `--list` + `--app` → `ExitInvalidArgs`
- `--list` + `--select` (or the deprecated `--go` alias — same bound
  variable) → `ExitInvalidArgs`
- `--json` without `--list` → `ExitInvalidArgs` (`wt open` has no other JSON
  surface)

#### Scenario: invalid combination is rejected before any work
- **GIVEN** any of the four invalid combinations
- **WHEN** the command runs
- **THEN** it exits 2 with a mutually-exclusive-style stderr message and
  performs no detection, launch, or git work

## Design Decisions

### `id` is the internal command key, not the display name

**Decision**: The JSON `id` is `AppInfo.Cmd` (e.g. `code`, `ghostty_macos`),
emitted verbatim including platform suffixes; `label` carries the human
display name.
**Why**: run-kit feeds `id` straight back to `wt open -a <id>`, and
`ResolveApp` matches `Cmd` keys first — only `Cmd` satisfies the
validation-source motivation. Because the listing derives from the same
`BuildAvailableApps()` catalog `-a` resolution walks, every emitted `id` is
guaranteed resolvable, so the "validation source" property falls out for
free rather than needing a separate check.
**Rejected**: emitting the display name as `id` (would not round-trip
through `-a`); normalizing platform suffixes away (the consumer round-trips
ids and must not need to interpret them).
*Introduced by*: 260722-qj66-open-list-json

### Locus as a second orthogonal axis, filtering pushed to consumers

**Decision**: Add `locus` (`gui`/`session`/`caller`/`host`) beside `kind`
and emit the full catalog on the machine surface; consumers filter.
**Why**: Launchability is consumer-relative — `tmux_window` and
`ghostty_linux` are the same kind ("a shell at this path") but land in
different places; hiding rows baked run-kit's policy into the data and
dead-ended in-session agents (backlog `[i2ap]`).
**Rejected**: a `--list --all` flag (extra surface; user chose expanded
JSON); skill-bundle-only fix (leaves the JSON registry lying); expanding the
human table too (user kept it simple — humans have the interactive menu).
*Introduced by*: 260902-ps3l-open-list-locus-registry

### `default` is omitempty; the four core keys are not

**Decision**: `id`/`label`/`kind`/`locus` always present; `default` emitted
only when true.
**Why**: The four core fields are unconditionally computed (the
no-`omitempty` rationale from 260722-qj66 still applies); a
`"default": false` key on every row is pure noise, and "marker" semantics
read naturally as presence.
**Rejected**: `default: false` on every row (noise); a separate top-level
`default_id` field (breaks the one-record-per-row shape consumers iterate).
*Introduced by*: 260902-ps3l-open-list-locus-registry

### `[]` not `null` for the empty case

**Decision**: The record slice is initialized non-nil
(`make([]openAppRecord, 0, …)`) so a zero-app host emits `[]` and exits 0.
**Why**: Machine consumers parse an array; an empty host list is a valid
answer, not an error. A nil Go slice marshals to `null`, which would force
every consumer to special-case it — the same array-semantics guarantee
`wt list --json` gives.
**Rejected**: returning `null` / a non-zero exit for zero apps (an empty
result is not a failure; `null` breaks naive array parsers).
*Introduced by*: 260722-qj66-open-list-json

### `--list` runs before git detection; no deeplink knowledge in `wt`

**Decision**: The `--list` branch (and its flag-exclusivity guards) runs at
the top of `openCmd`'s `RunE`, before the soft git-context detection. `wt`
carries no deeplink / URL-scheme knowledge — it does detection, listing, and
launch only.
**Why**: App detection is host-only and needs no repository, and run-kit's
server invokes the command from arbitrary cwds. Client-side deeplink
templates (`vscode://vscode-remote/ssh-remote+{host}{path}` etc.) depend on
*client-machine* installs that the host cannot detect; host detection would
be an inverted signal, so deeplinks live in run-kit's frontend, not `wt`.
**Rejected**: gating `--list` behind git-context detection (needless, and
breaks the external-consumer use case); teaching `wt` deeplink templates
(wrong layer — decided in the 2026-07-22 scope discussion).
*Introduced by*: 260722-qj66-open-list-json

## Cross-references

- Spec doc: `docs/specs/launcher-contract.md` — §1 (`wt open` owns the app
  catalog; consumers delegate rather than fork it), §2 (invocation surface —
  the `wt open --list [--json]` query form, the `{id, label, kind, locus}`
  record shape, the full-catalog semantics, and the round-trip invariant),
  §6 (adding new internal flags to `wt open` is a non-breaking evolution,
  under which `--list`/`--json` were added).
- Spec doc: `docs/specs/cli-surface.md` — `wt open` flag table (`--list` /
  `--json` rows, noting the human-table vs full-registry machine split) and
  exit-code notes.
- Source: `src/cmd/wt/open.go` — `openCmd` (flag registration + exclusivity
  guards + the pre-git-detection `--list` branch), `openAppRecord` (`locus`
  + omitempty `default`), `handleOpenList` (full catalog for JSON,
  `ListableApps` subset for the table), `printOpenListJSON`,
  `printOpenListTable`.
- Source: `src/internal/worktree/apps.go` — `AppInfo.Kind`/`AppInfo.Locus`,
  the `AppKind*` / `Locus*` constants, the fixed per-Cmd classification in
  `BuildAvailableApps()`, and the `ListableApps` filter helper (predicate
  `Locus == LocusGUI`).
- Tests: `src/cmd/wt/open_test.go` — `TestOpen_List_HumanTable`,
  `TestOpen_List_NoGitRequired`, `TestOpen_ListJSON_ShapeAndOrder` (four
  core keys on every record, closed kind/locus enums, full-catalog detection
  order), `TestOpen_ListJSON_OutsideTmuxOmitsSessionTargets`,
  `TestOpen_ListJSON_IDsRoundTrip` (the id-equals-`-a`-token guarantee under
  the `WT_TEST_NO_LAUNCH=1` seam), `TestOpen_ListJSON_DefaultMarker` and
  `TestPrintOpenListJSON_NoDefaultMarker` (exactly-one / zero `default`
  markers), `TestPrintOpenListJSON_EmptyEmitsArray` (`[]`-not-`null`),
  `TestPrintOpenListTable_EmptyMessage` (the zero-host-apps copy),
  `TestOpen_List_FlagExclusivity` (all four pairs),
  `TestOpen_HelpShowsListAndJSON`.
- Tests: `src/internal/worktree/apps_test.go` —
  `TestBuildAvailableApps_KindClassification` (every `Cmd` maps to its
  `Kind`/`Locus` pair per the table, via `appClassificationByCmd`),
  `TestListableApps_FiltersActionRowsPreservingOrder` (non-GUI rows dropped,
  order kept), `TestListableApps_EmptyInputReturnsNonNil`.
- Constitution: Principle II (Cobra command surface), III (Typed exit codes
  — `ExitInvalidArgs` for the four mutex checks, no new code), V (the
  `ListableApps` filter rule lives in `internal/worktree`, cmd/ stays
  orchestration-only), VI (deterministic machine output — detection-order
  preservation, `--list` works with no git).
- Sibling memory: [list-status-contract](/wt-cli/list-status-contract.md) —
  the `wt list` human-default / `--json`-opt-in split and `handleJSONOutput`
  encoding pattern this contract mirrors (only the omitempty `default`
  marker is conditionally emitted here; the four core keys are
  unconditionally computed).
- Sibling memory: [skill-command-contract](/wt-cli/skill-command-contract.md)
  — the `wt skill` agent bundle whose `open` capability line names the
  `--list` vs `--list --json` split, the kind/locus semantics, and the
  multiplexer ids (`tmux_window`, `tmux_session`, `byobu_tab`).
- Sibling memory: [go-command-contract](/wt-cli/go-command-contract.md) —
  the `wt open` launcher surface and `--select` (deprecated `--go`) flag
  that `--list` is mutually exclusive with.
- Sibling memory: [recency-ordering-contract](/wt-cli/recency-ordering-contract.md)
  — the `wt open` menu's newest-first worktree ordering; unrelated to
  `--list`, whose ordering is `BuildAvailableApps()` catalog order, not
  recency.
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md)
  — the CLI-surface re-audit trigger; the `--list`/`--json` addition was
  checked against the shll standards with no verdict change.
- External consumer: run-kit's `GET /api/open-apps` (the dropdown consumer
  of the JSON form). run-kit MUST filter to `locus == "gui"` rows before
  serving its dropdown — that its-side change is tracked in run-kit backlog
  item `[k2pm]` and deploys before the `wt` release carrying the expanded
  registry ships (the locus filter is compatible with both old and new
  output). In-session agents consume the registry unfiltered.
