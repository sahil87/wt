---
type: memory
description: "`wt open --list` / `--list --json` app-registry contract — `{id, label, kind}` records, id-equals-`-a`-token guarantee, kind enum, action-row filtering, detection-order preservation, flag exclusivity, `[]`-not-`null`, exit codes, no-git-required."
---
# wt-cli: Open List Contract

**Domain**: wt-cli

> Post-implementation behavior capture for the `wt open --list` query surface.
> Source change: `260722-qj66-open-list-json`.

## Overview

`wt open --list` is a pure **query** on `wt open`'s detected-app catalog: it lists the
launchable host applications (editors, terminals, file managers) and exits without
opening anything. `--list --json` emits the same list as a machine-readable JSON array
of `{id, label, kind}` records. It exists so an external consumer (concretely run-kit's
`GET /api/open-apps`) can (a) populate an "open on host" dropdown and (b) validate an
app id before launching via the existing non-interactive path `wt open <path> -a <id>`,
without forking `wt`'s app-detection catalog. The `wt open` invocation surface is
specified in `docs/specs/launcher-contract.md` §2; this file records the query form's
runtime invariants.

## Requirements

### Requirement: `--list` lists detected apps without menu, launch, or git

`wt open --list` MUST print the detected launchable host applications and exit
`ExitSuccess` (0) — no interactive menu, no app launch, no positional target consumed.
The `--list` branch MUST run before the soft git-context detection in `openCmd`'s
`RunE`, so it works from any cwd (including a non-git directory — external servers
invoke it from arbitrary locations). Detection MUST be the single `wt.BuildAvailableApps()`
call the interactive menu and `-a` resolution use — there is no second detection path.

#### Scenario: list from a non-git directory
- **GIVEN** a non-git working directory (e.g. a fresh temp dir)
- **WHEN** the user runs `wt open --list`
- **THEN** the command exits 0 and prints the detected app listing, with no git
  invocation required and no app launched

### Requirement: `AppInfo.Kind` classifies each catalog entry

`AppInfo` (`src/internal/worktree/apps.go`) carries a `Kind string` field, populated at
each append site in `BuildAvailableApps()` from the named constants `AppKindEditor` /
`AppKindTerminal` / `AppKindFileManager` (values `editor` / `terminal` / `file-manager`).
The mapping is fixed:

| Cmd key | Kind |
|---------|------|
| `code`, `cursor` | `editor` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` |
| `finder`, `nautilus`, `dolphin` | `file-manager` |
| `open_here`, `copy_macos`, `copy_linux`, `byobu_tab`, `tmux_window`, `tmux_session` | (empty — action rows) |

`Kind` is additive metadata: the interactive menu, `-a` resolution (`ResolveApp`),
`DetectDefaultApp`, and `SaveLastApp` are untouched by its presence.

### Requirement: action rows are filtered from the listing but stay in the menu

The `--list` output filters to entries with a **non-empty `Kind`**, via the
`ListableApps(apps []AppInfo) []AppInfo` helper in `src/internal/worktree/apps.go`
(the filter rule lives beside the catalog per Constitution V; `cmd/` stays
orchestration-only). Excluded action rows — `open_here` (shell-cd), `copy_macos` /
`copy_linux` (clipboard), `byobu_tab` / `tmux_window` / `tmux_session` (multiplexer) —
depend on `wt`'s own process environment and are wrong signals for an external
launcher. They remain in the interactive menu and remain valid `-a` values, unchanged.
`ListableApps` preserves input order and always returns a non-nil slice.

#### Scenario: filter a mixed catalog
- **GIVEN** a catalog slice containing both host apps and action rows
- **WHEN** `ListableApps` is applied
- **THEN** only non-empty-`Kind` entries remain, in their original order
- **AND** the interactive menu and `-a` resolution still see the full unfiltered catalog

### Requirement: human table output (no `--json`)

`wt open --list` without `--json` prints a small aligned `Id` / `Label` / `Kind`
table (`Id` = `AppInfo.Cmd`, `Label` = `AppInfo.Name`, `Kind` = `AppInfo.Kind`), with
column widths computed from max cell width and two-space padding — mirroring `wt list`'s
human-default / `--json`-opt-in split. When zero launchable apps are detected, human
mode prints `No launchable applications detected.` and exits 0. No per-row detection
cost is added beyond what `BuildAvailableApps()` already pays.

### Requirement: `--list --json` machine output

`wt open --list --json` emits a JSON array of records `{"id": …, "label": …, "kind": …}`,
one per listable app, where:

- **`id`** = `AppInfo.Cmd` emitted verbatim (e.g. `code`, `ghostty_macos`, `terminal_app`).
  This is the exact token `wt open <path> -a <id>` accepts — `ResolveApp` matches `Cmd`
  first — so the output is a **validation source** for a consumer's launch path.
  Platform-suffixed keys (`ghostty_macos` vs `ghostty_linux`) are emitted as-is; the
  consumer round-trips ids and never interprets them.
- **`label`** = `AppInfo.Name` (the display name, e.g. `VSCode`, `Terminal.app`).
- **`kind`** = one of `editor` / `terminal` / `file-manager` (the closed enum).

All three keys are present on every record (no `omitempty` — no field is conditionally
computed, unlike `wt list --status`'s opt-in pointer fields). Zero listable apps MUST
emit `[]` — an empty JSON array, **not** `null` — and exit 0; the record slice is
initialized non-nil (`make([]openAppRecord, 0, …)`) because a nil Go slice marshals to
`null`. Encoding mirrors `wt list --json`'s `handleJSONOutput`: `json.MarshalIndent(…, "", "  ")`
(two-space indent) printed with a trailing newline.

#### Scenario: json record shape and id round-trip
- **GIVEN** a host with detected apps
- **WHEN** the user runs `wt open --list --json`
- **THEN** stdout parses as a JSON array where every record has exactly the keys `id`,
  `label`, `kind`; `kind` is in the closed enum; no action-row id appears; and each `id`
  resolves via `wt open <dir> -a <id>`

#### Scenario: empty catalog emits an array
- **GIVEN** zero listable apps
- **WHEN** the JSON emitter runs on the empty filtered slice
- **THEN** it emits `[]`, not `null`, and the command exits 0

### Requirement: deterministic ordering

The `--list` output (both modes) preserves `BuildAvailableApps()` detection order — the
same order the interactive menu shows, minus filtered action rows. No re-sorting is
applied. The catalog construction is deterministic per host state, so machine consumers
get stable output (Constitution VI).

### Requirement: flag exclusivity at flag-check time

`wt open` rejects the following combinations with `ExitInvalidArgs` (2) and a
what/why/fix message, following `wt list`'s `ExitWithError(wt.ExitInvalidArgs, …)`
mutex idiom. Validation happens at flag-check time, before any detection or git work,
and uses only existing exit-code constants:

- `--list` + a positional target → `ExitInvalidArgs`
- `--list` + `--app` → `ExitInvalidArgs`
- `--list` + `--select` (or the deprecated `--go` alias — same bound variable) → `ExitInvalidArgs`
- `--json` without `--list` → `ExitInvalidArgs` (`wt open` has no other JSON surface)

#### Scenario: invalid combination is rejected before any work
- **GIVEN** any of the four invalid combinations
- **WHEN** the command runs
- **THEN** it exits 2 with a mutually-exclusive-style stderr message and performs no
  detection, launch, or git work

## Design Decisions

### `id` is the internal command key, not the display name

**Decision**: The JSON `id` is `AppInfo.Cmd` (e.g. `code`, `ghostty_macos`), emitted
verbatim including platform suffixes; `label` carries the human display name.
**Why**: run-kit feeds `id` straight back to `wt open -a <id>`, and `ResolveApp` matches
`Cmd` keys first — only `Cmd` satisfies the validation-source motivation. Because the
listing derives from the same `BuildAvailableApps()` catalog `-a` resolution walks, every
emitted `id` is guaranteed resolvable, so the "validation source" property falls out for
free rather than needing a separate check.
**Rejected**: emitting the display name as `id` (would not round-trip through `-a`);
normalizing platform suffixes away (the consumer round-trips ids and must not need to
interpret them).
*Introduced by*: 260722-qj66-open-list-json

### Filter action rows via non-empty `Kind`, rule beside the catalog

**Decision**: The `--list` filter is "keep entries with non-empty `Kind`", implemented as
`ListableApps` in `internal/worktree` beside `BuildAvailableApps()`. Action rows carry an
empty `Kind` and are dropped from the listing only.
**Why**: The three-kind enum is host applications by definition; the excluded rows
(`open_here`, `copy_*`, `byobu_tab`, `tmux_window`, `tmux_session`) are detected from
`wt`'s *own* process environment (`IsTmuxSession()` / `IsByobuSession()`, shell wrapper,
clipboard) — a consumer server's session, which is a wrong/inverted signal for a remote
browser user (the same inversion argument as the deeplink scope boundary). Keeping the
filter in `internal/worktree` honors Constitution V (cmd/ stays orchestration-only) and
keeps the rule co-located with the catalog that defines `Kind`.
**Rejected**: an explicit exclusion list in `cmd/` (business rule leaks into the command
layer, and drifts from the catalog); excluding action rows from the menu / `-a` too
(would be a breaking behavior change — they are still meaningful for an interactive user).
*Introduced by*: 260722-qj66-open-list-json

### Minimal `{id, label, kind}` record; no `default` marker, no pointer fields

**Decision**: The record is exactly `{id, label, kind}`, all keys always present, plain
strings — no `omitempty`, no pointer fields, no `default`-app marker.
**Why**: The backlog specifies this minimal shape. No field is conditionally computed, so
the pointer/`omitempty` machinery `wt list --status` needs (to distinguish "not computed"
from "computed zero") is unnecessary ceremony here. Preselection is not requested; a
`default` marker would be a purely additive follow-up.
**Rejected**: mirroring `wt list --status`'s pointer-field shape (no optional field
exists to justify it); adding a `default: true` marker now (unrequested, additive later
if run-kit needs preselection).
*Introduced by*: 260722-qj66-open-list-json

### `[]` not `null` for the empty case

**Decision**: The record slice is initialized non-nil (`make([]openAppRecord, 0, …)`) so a
zero-app host emits `[]` and exits 0.
**Why**: Machine consumers parse an array; an empty host list is a valid answer, not an
error. A nil Go slice marshals to `null`, which would force every consumer to special-case
it — the same array-semantics guarantee `wt list --json` gives.
**Rejected**: returning `null` / a non-zero exit for zero apps (an empty result is not a
failure; `null` breaks naive array parsers).
*Introduced by*: 260722-qj66-open-list-json

### `--list` runs before git detection; no deeplink knowledge in `wt`

**Decision**: The `--list` branch (and its flag-exclusivity guards) runs at the top of
`openCmd`'s `RunE`, before the soft git-context detection. `wt` carries no deeplink /
URL-scheme knowledge — it does detection, listing, and launch only.
**Why**: App detection is host-only and needs no repository, and run-kit's server invokes
the command from arbitrary cwds. Client-side deeplink templates
(`vscode://vscode-remote/ssh-remote+{host}{path}` etc.) depend on *client-machine* installs
that the host cannot detect; host detection would be an inverted signal, so deeplinks live
in run-kit's frontend, not `wt`.
**Rejected**: gating `--list` behind git-context detection (needless, and breaks the
external-consumer use case); teaching `wt` deeplink templates (wrong layer — decided in the
2026-07-22 scope discussion).
*Introduced by*: 260722-qj66-open-list-json

## Cross-references

- Spec doc: `docs/specs/launcher-contract.md` — §1 (`wt open` owns the app catalog;
  consumers delegate rather than fork it), §2 (invocation surface — the `wt open --list
  [--json]` query form), §6 (adding new internal flags to `wt open` is a non-breaking
  evolution, under which `--list`/`--json` were added).
- Spec doc: `docs/specs/cli-surface.md` — `wt open` flag table (`--list` / `--json` rows)
  and exit-code notes.
- Source: `src/cmd/wt/open.go` — `openCmd` (flag registration + exclusivity guards + the
  pre-git-detection `--list` branch), `openAppRecord`, `handleOpenList`,
  `printOpenListJSON`, `printOpenListTable`.
- Source: `src/internal/worktree/apps.go` — `AppInfo.Kind`, the `AppKindEditor` /
  `AppKindTerminal` / `AppKindFileManager` constants, `Kind` population in
  `BuildAvailableApps()`, and the `ListableApps` filter helper.
- Tests: `src/cmd/wt/open_test.go` — `TestOpen_List_HumanTable`,
  `TestOpen_List_NoGitRequired`, `TestOpen_ListJSON_ShapeAndOrder`,
  `TestOpen_ListJSON_IDsRoundTrip` (the id-equals-`-a`-token guarantee under the
  `WT_TEST_NO_LAUNCH=1` seam), `TestPrintOpenListJSON_EmptyEmitsArray` (`[]`-not-`null`),
  `TestOpen_List_FlagExclusivity` (all four pairs), `TestOpen_HelpShowsListAndJSON`.
- Tests: `src/internal/worktree/apps_test.go` — `TestBuildAvailableApps_KindClassification`
  (every `Cmd` maps per the table), `TestListableApps_FiltersActionRowsPreservingOrder`,
  `TestListableApps_EmptyInputReturnsNonNil`.
- Constitution: Principle II (Cobra command surface), III (Typed exit codes —
  `ExitInvalidArgs` for the four mutex checks, no new code), V (the `ListableApps` filter
  rule lives in `internal/worktree`, cmd/ stays orchestration-only), VI (deterministic
  machine output — detection-order preservation, `--list` works with no git).
- Sibling memory: [list-status-contract](/wt-cli/list-status-contract.md) — the
  `wt list` human-default / `--json`-opt-in split and `handleJSONOutput` encoding pattern
  this contract mirrors (though `wt open --list` needs no pointer/`omitempty` machinery,
  as no field is conditionally computed here).
- Sibling memory: [go-command-contract](/wt-cli/go-command-contract.md) — the `wt open`
  launcher surface and `--select` (deprecated `--go`) flag that `--list` is mutually
  exclusive with.
- Sibling memory: [recency-ordering-contract](/wt-cli/recency-ordering-contract.md) — the
  `wt open` menu's newest-first worktree ordering; unrelated to `--list`, whose ordering is
  `BuildAvailableApps()` catalog order, not recency.
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md)
  — the CLI-surface re-audit trigger; the `--list`/`--json` addition was checked against
  the shll standards with no verdict change.
- External consumer: run-kit's `GET /api/open-apps` (the intended consumer of the JSON
  form and its validation-source guarantee).
