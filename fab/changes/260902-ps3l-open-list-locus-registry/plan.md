# Plan: Open-List Locus Registry

**Change**: 260902-ps3l-open-list-locus-registry
**Intake**: `intake.md`

## Requirements

### wt-cli: Complete machine registry

#### R1: `wt open --list --json` emits the full catalog with a round-trip invariant
`wt open --list --json` SHALL emit one record per `BuildAvailableApps()` catalog entry — the
**unfiltered** catalog, in detection order — as a JSON array of records with keys `id`, `label`,
`kind`, `locus` present on every record (no `omitempty` on these four). The machine invariant
SHALL hold: every emitted `id` resolves via `wt open <target> -a <id>` in the current
environment, and every id `-a` resolves is emitted. Encoding stays `json.MarshalIndent(…, "", "  ")`
with a trailing newline; the record slice stays initialized non-nil (`[]`-not-`null` guarantee).
Flag-exclusivity rules are unchanged.

- **GIVEN** a headless Linux host inside a tmux session with no GUI apps installed
- **WHEN** `wt open --list --json` runs
- **THEN** the array is non-empty and contains `open_here`, `tmux_window`, and `tmux_session`
  records (plus `byobu_tab` under byobu, `copy_linux` when xclip exists)
- **AND** each emitted `id` resolves through `-a <id>` (verifiable under `WT_TEST_NO_LAUNCH=1`)

#### R2: Every catalog row carries `Kind` and the new `Locus` classification
`AppInfo` SHALL gain a `Locus string` field. Named constants SHALL be added: `LocusGUI = "gui"`,
`LocusSession = "session"`, `LocusCaller = "caller"`, `LocusHost = "host"`, plus
`AppKindMultiplexer = "multiplexer"`, `AppKindShell = "shell"`, `AppKindClipboard = "clipboard"`.
Every `BuildAvailableApps()` append site SHALL populate both fields per this fixed mapping:

| Cmd key | Kind | Locus |
|---------|------|-------|
| `code`, `cursor` | `editor` | `gui` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` | `gui` |
| `finder`, `nautilus`, `dolphin` | `file-manager` | `gui` |
| `tmux_window`, `tmux_session`, `byobu_tab` | `multiplexer` | `session` |
| `open_here` | `shell` | `caller` |
| `copy_macos`, `copy_linux` | `clipboard` | `host` |

`ListableApps` SHALL change its predicate from "non-empty `Kind`" to `Locus == LocusGUI`,
preserving its name, detection-order preservation, and non-nil-return contract — the selected
row set is behavior-identical (host applications only). Detection gating (tmux/byobu/clipboard
presence), the interactive menu, `ResolveApp`, `DetectDefaultApp`, `SaveLastApp`, and
`OpenInApp` MUST NOT change behavior.

- **GIVEN** the full mapping table above
- **WHEN** `BuildAvailableApps()` constructs the catalog in any environment
- **THEN** every returned `AppInfo` has non-empty `Kind` and `Locus` matching the table
- **AND** `ListableApps` returns exactly the `locus == "gui"` rows in detection order

#### R3: `default: true` marker on the detected default row (JSON only)
The JSON record SHALL carry `"default": true` (Go: `Default bool` with `json:"default,omitempty"`)
on exactly the row `DetectDefaultApp()` selects — the same row `-a default` resolves to. All other
rows omit the key. When `DetectDefaultApp` returns -1, no record carries the key. The human table
does not render it.

- **GIVEN** a plain tmux session (no `TERM_PROGRAM` editor, no last-app cache hit)
- **WHEN** `wt open --list --json` runs
- **THEN** the `tmux_window` record carries `"default": true` and no other record has the key

#### R4: Plain `--list` (human) unchanged except the empty-case copy
The human table SHALL keep today's exact shape — `Id` / `Label` / `Kind` columns over the
`ListableApps` (host-app) set, no `Locus` column, no new rows. The zero-host-apps message SHALL
change from `No launchable applications detected.` to:
`No host applications detected. {N} other target(s) available — see 'wt open --list --json' or the interactive menu.`
where `{N}` = full catalog size minus host-app rows. Exit code 0 unchanged. (`open_here` is
unconditional, so `{N} >= 1` always.)

- **GIVEN** a host with zero detected GUI applications
- **WHEN** `wt open --list` runs (no `--json`)
- **THEN** the new message prints with the correct `{N}` and the command exits 0
- **AND** on a host with GUI apps the table output is byte-identical to the pre-change output

### wt-cli: Documentation surfaces

#### R5: Skill bundle teaches the id vocabulary
`docs/site/skill.md`'s `open` capability line SHALL be replaced with the intake §4 text (naming
`--list` vs `--list --json`, the kind/locus semantics, the multiplexer ids `tmux_window` /
`tmux_session` / `byobu_tab` with the `-a tmux_window` example, and the `default: true` marker).
The committed embed copy `src/cmd/wt/skill.md` SHALL be refreshed via the sync mechanism so
`TestSkill_EmbedMatchesCanonical` passes, and the bundle SHALL stay within the 150-line budget
(`TestSkill_LineBudget`).

- **GIVEN** the updated canonical bundle
- **WHEN** `go test ./...` runs
- **THEN** the drift-guard and line-budget tests pass and `wt skill` output names the
  multiplexer ids

#### R6: Specs updated for the human/machine split
`docs/specs/launcher-contract.md` §2 SHALL document the new `--list --json` record shape (four
always-present keys + optional `default`), full-catalog semantics, and the round-trip invariant.
`docs/specs/cli-surface.md`'s `wt open` `--list`/`--json` notes SHALL reflect the human/machine
split.

- **GIVEN** the two spec files
- **WHEN** a reader checks the `--list --json` contract
- **THEN** the documented record shape and semantics match the implementation

### Non-Goals

- run-kit's `locus == "gui"` filter — separate shll-repo change; deploys before this ships
  (coordination recorded in the PR body).
- Changes to `-a` resolution or its error messages, the interactive menu, `DetectDefaultApp`
  logic, or `OpenInApp` launch behavior.
- New CLI filter flags — consumers filter the JSON.

### Design Decisions

#### Locus as a second orthogonal axis, filtering pushed to consumers
**Decision**: Add `locus` (`gui`/`session`/`caller`/`host`) beside `kind` and emit the full
catalog on the machine surface; consumers filter.
**Why**: Launchability is consumer-relative — `tmux_window` and `ghostty_linux` are the same
kind ("a shell at this path") but land in different places; hiding rows baked run-kit's policy
into the data and dead-ended in-session agents (backlog `[i2ap]`).
**Rejected**: a `--list --all` flag (extra surface; user chose expanded JSON); skill-bundle-only
fix (leaves the JSON registry lying); expanding the human table too (user kept it simple —
humans have the interactive menu).
*Introduced by*: 260902-ps3l-open-list-locus-registry

#### `default` is omitempty; the four core keys are not
**Decision**: `id`/`label`/`kind`/`locus` always present; `default` emitted only when true.
**Why**: The four core fields are unconditionally computed (the no-`omitempty` rationale from
260722-qj66 still applies); a `"default": false` key on every row is pure noise, and "marker"
semantics read naturally as presence.
**Rejected**: `default: false` on every row (noise); a separate top-level `default_id` field
(breaks the one-record-per-row shape consumers iterate).
*Introduced by*: 260902-ps3l-open-list-locus-registry

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `Locus` field + `Locus*`/new `AppKind*` constants to `src/internal/worktree/apps.go`; populate `Kind`+`Locus` at every `BuildAvailableApps()` append site per the R2 mapping; switch `ListableApps` predicate to `Locus == LocusGUI` and update its doc comment (and the `AppKind*` block comment) <!-- R2 -->
- [x] T002 Update `src/internal/worktree/apps_test.go`: extend the kind-classification test to assert `Kind` AND `Locus` for every Cmd key; rework `TestListableApps_*` for the new predicate (action rows now carry non-empty `Kind` but non-`gui` `Locus`; fixtures must set `Locus`) <!-- R2 -->
- [x] T003 In `src/cmd/wt/open.go`: add `Locus string \`json:"locus"\`` and `Default bool \`json:"default,omitempty"\`` to `openAppRecord`; make `handleOpenList` pass the full `BuildAvailableApps()` catalog to `printOpenListJSON` (table branch keeps `ListableApps`); set `Default` on the `DetectDefaultApp` row; replace the empty-table message with the R4 copy computing `{N}` <!-- R1 R3 R4 -->
- [x] T004 Update `src/cmd/wt/open_test.go`: full-catalog JSON expectations (4 keys on every record, action rows present, detection order, `id` → `-a` round-trip under `WT_TEST_NO_LAUNCH=1`), a `default`-marker test (exactly one/zero rows carry it), the new empty-case copy assertion (currently `open_test.go:396`), keep the `[]`-not-`null` test green <!-- R1 R3 R4 -->

### Phase 3: Documentation propagation

- [x] T005 Rewrite the `open` capability line in `docs/site/skill.md` per intake §4; refresh the embed copy via `just sync-skill` (or `bash scripts/sync-skill.sh`); confirm ≤150 lines <!-- R5 -->
- [x] T006 [P] Update `docs/specs/launcher-contract.md` §2 (record shape, full-catalog semantics, round-trip invariant) and `docs/specs/cli-surface.md` (`wt open` `--list`/`--json` rows) <!-- R6 -->

### Phase 4: Polish

- [x] T007 Full verification from `src/`: `gofmt -l .` (must print nothing — CI fails fast on it), `go vet ./...`, `go test ./...` all green <!-- R1 -->

## Execution Order

- T001 blocks T002, T003; T003 blocks T004
- T005, T006 independent of each other after T001 lands (T005's drift-guard runs in T007)
- T007 last

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt open --list --json` emits one record per unfiltered catalog entry, detection order, with `id`/`label`/`kind`/`locus` on every record
- [x] A-002 R2: every `BuildAvailableApps()` row's `Kind`/`Locus` matches the R2 mapping table exactly; `ListableApps` selects the identical host-app set as before
- [x] A-003 R3: the `DetectDefaultApp` row (and only it) carries `"default": true` in JSON
- [x] A-004 R4: human `--list` table shape and row set unchanged; new empty-case copy with correct `{N}`
- [x] A-005 R5: skill bundle names the multiplexer ids; drift-guard and line-budget tests pass
- [x] A-006 R6: both spec files describe the new record shape and human/machine split

### Behavioral Correctness

- [x] A-007 R1: inside a tmux session with no GUI apps, `--list --json` is non-empty and includes `tmux_window`/`tmux_session`; outside tmux those rows are absent
- [x] A-008 R4: on a GUI host, human-table output is byte-identical to pre-change output

### Scenario Coverage

- [x] A-009 R1: an automated test round-trips every emitted `id` through `wt open <dir> -a <id>` under `WT_TEST_NO_LAUNCH=1`
- [x] A-010 R3: a test covers the no-default case (no record carries the key when `DetectDefaultApp` returns -1)

### Edge Cases & Error Handling

- [x] A-011 R1: `printOpenListJSON` of an empty slice still emits `[]`, not `null`; flag-exclusivity rejections unchanged (`ExitInvalidArgs`)
- [x] A-012 R4: `{N}` grammar handles 1 vs many ("target" / "targets") or uses the invariant "target(s)" form consistently

### Code Quality

- [x] A-013 Pattern consistency: new code follows naming and structural patterns of surrounding code (constants block style, table-driven tests)
- [x] A-014 No unnecessary duplication: existing utilities reused (single `BuildAvailableApps` detection path preserved — no second catalog walk)
- [x] A-015 No magic strings: every kind and locus value referenced via the named constants
- [x] A-016 No god functions: `handleOpenList` and helpers stay small and single-purpose

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change reclassifies existing catalog rows and swaps one filter predicate and one message string; it adds new behavior without leaving any symbol, branch, or block unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | `{N}` in the empty-case copy = `len(BuildAvailableApps()) - len(ListableApps(...))`, rendered with the literal "target(s)" form | Simplest correct derivation; wording trivially adjustable (intake assumption 6) | S:60 R:90 A:85 D:75 |
| 2 | Confident | `Default` computed by calling `DetectDefaultApp` on the full catalog inside the JSON path only; index -1 ⇒ no marker | Mirrors `-a default` resolution exactly; zero cost added to the human path | S:70 R:80 A:85 D:75 |
| 3 | Certain | Go commands run from module root `src/`; `gofmt -l` must be clean before ship | CI memory records the gofmt fail-fast gate; config lists src paths | S:90 R:95 A:95 D:95 |

3 assumptions (1 certain, 2 confident, 0 tentative).
