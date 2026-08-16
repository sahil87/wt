# Plan: wt open --list --json — machine-readable registry of detected apps

**Change**: 260722-qj66-open-list-json
**Intake**: `intake.md`

## Requirements

### wt-cli: `wt open --list` query surface

#### R1: `--list` lists detected apps without menu, launch, or git
`wt open` SHALL accept a new bool flag `--list`. When set, the command MUST print the detected launchable host applications and exit `ExitSuccess` (0) — no interactive menu, no app launch, no positional target consumed. The `--list` branch MUST run before the soft git-context detection in `openCmd`'s `RunE`, so `wt open --list` works from any cwd, including a non-git directory. Detection MUST be the same single `wt.BuildAvailableApps()` call the interactive menu and `-a` resolution use — no second detection path.

- **GIVEN** a non-git working directory (e.g. a fresh temp dir)
- **WHEN** the user runs `wt open --list`
- **THEN** the command exits 0 and prints the detected app listing, with no git invocation required and no app launched

#### R2: `AppInfo.Kind` classification at the catalog
`AppInfo` (`src/internal/worktree/apps.go`) SHALL gain a `Kind string` field, populated where each app is appended in `BuildAvailableApps()`, using named constants for the closed enum `editor | terminal | file-manager`. The mapping MUST be:

| Cmd key | Kind |
|---------|------|
| `code`, `cursor` | `editor` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` |
| `finder`, `nautilus`, `dolphin` | `file-manager` |
| `open_here`, `copy_macos`, `copy_linux`, `byobu_tab`, `tmux_window`, `tmux_session` | (empty — action rows) |

`Kind` is additive metadata: menu behavior, `-a` resolution (`ResolveApp`), `DetectDefaultApp`, and `SaveLastApp` MUST be untouched.

- **GIVEN** the catalog built by `BuildAvailableApps()`
- **WHEN** each entry's `Cmd` is checked against the table above
- **THEN** its `Kind` matches the mapped value exactly, and action rows carry an empty `Kind`

#### R3: Action-row filtering lives beside the catalog
The `--list` output SHALL filter to launchable host applications — entries with non-empty `Kind`. The filter rule MUST live in `src/internal/worktree/` (a `ListableApps(apps []AppInfo) []AppInfo` helper beside the catalog, per Constitution V — `cmd/` stays orchestration-only). Excluded action rows (`open_here`, `copy_macos`, `copy_linux`, `byobu_tab`, `tmux_window`, `tmux_session`) MUST remain in the interactive menu and remain valid `-a` values, unchanged.

- **GIVEN** a catalog slice containing both host apps and action rows
- **WHEN** `ListableApps` is applied
- **THEN** only non-empty-`Kind` entries remain, in their original order
- **AND** the interactive menu and `-a` resolution still see the full unfiltered catalog

#### R4: Human table output (no `--json`)
`wt open --list` without `--json` SHALL print a small aligned table with `Id` / `Label` / `Kind` columns (Id = `AppInfo.Cmd`, Label = `AppInfo.Name`), mirroring `wt list`'s human-default/`--json`-opt-in split. When zero launchable apps are detected, the human mode SHALL print a short "no launchable applications detected" message and exit 0. No per-row detection cost beyond what `BuildAvailableApps()` already does.

- **GIVEN** a host with at least one detected launchable app
- **WHEN** the user runs `wt open --list`
- **THEN** stdout carries an aligned table with `Id`, `Label`, and `Kind` headers and one row per listable app, and no action row appears

#### R5: `--list --json` machine output
`wt open` SHALL accept a new bool flag `--json`, valid only together with `--list`. `wt open --list --json` SHALL emit a JSON array of records `{"id": ..., "label": ..., "kind": ...}` — `id` = `AppInfo.Cmd` verbatim (the exact token `wt open <path> -a <id>` accepts, platform-suffixed keys as-is), `label` = `AppInfo.Name`, `kind` one of `editor | terminal | file-manager`. All three keys MUST be present on every record. Zero listed apps MUST emit `[]` (an empty JSON array, **not** `null` — the output slice is initialized non-nil) and exit 0. JSON encoding mirrors `wt list --json` (`json.MarshalIndent` two-space, printed with trailing newline).

- **GIVEN** a host with detected apps
- **WHEN** the user runs `wt open --list --json`
- **THEN** stdout parses as a JSON array where every record has exactly the keys `id`, `label`, `kind`, `kind` is in the closed enum, and each `id` resolves via `wt open <dir> -a <id>`
- **GIVEN** zero listable apps
- **WHEN** the JSON emitter runs on the empty filtered slice
- **THEN** it emits `[]`, not `null`, and the command exits 0

#### R6: Flag exclusivity at flag-check time
Following `wt list`'s `ExitWithError(wt.ExitInvalidArgs, …)` mutual-exclusion idiom, `wt open` MUST reject with `ExitInvalidArgs` (2) and a what/why/fix message: (a) `--list` combined with a positional arg; (b) `--list` with `--app`; (c) `--list` with `--select` (or the deprecated `--go` alias — same bound variable); (d) `--json` without `--list`. Validation MUST happen at flag-check time, before any detection or git work. Only existing exit-code constants are used.

- **GIVEN** any of the four invalid combinations
- **WHEN** the command runs
- **THEN** it exits 2 with a mutually-exclusive-style stderr message and performs no detection, launch, or git work

#### R7: Deterministic ordering
The `--list` output (both modes) SHALL preserve `BuildAvailableApps()` detection order — the same order the menu shows, minus filtered rows. No re-sorting is applied.

- **GIVEN** the catalog order returned by `BuildAvailableApps()`
- **WHEN** `wt open --list --json` is emitted
- **THEN** record order equals catalog order with action rows removed

#### R8: Spec docs and toolkit-standards conformance
`docs/specs/cli-surface.md` (`wt open` flag table + exit-code notes) and `docs/specs/launcher-contract.md` §2 (invocation surface) SHALL gain the `--list` / `--json` query form (non-breaking per launcher-contract §6: "Adding new internal flags to `wt open`"). The CLI-surface/help-output change MUST be checked against the published shll toolkit standards (`shll standards`), and `docs/site/skill.md` verified for whether it enumerates `wt open` flags (sync only if it does).

- **GIVEN** the implemented flags
- **WHEN** the spec docs are read
- **THEN** both files describe the query form, and the standards check is recorded with no new violations

### Non-Goals

- No deeplink/URL-scheme knowledge in `wt` — client-side deeplink templates live in run-kit's frontend (host detection would be an inverted signal).
- No changes to the launch path (`-a` resolution, `OpenInApp`), the menu, default-app detection, or the `WT_CD_FILE`/`WT_WRAPPER` contract.
- No `default`-app marker in the JSON record — the shape stays the minimal `{id, label, kind}`.
- No new exit-code constants.

## Tasks

### Phase 1: Core Implementation (internal package)

- [x] T001 Add `Kind string` field to `AppInfo` with named constants `AppKindEditor` / `AppKindTerminal` / `AppKindFileManager`, and populate `Kind` at each append site in `BuildAvailableApps()` per the R2 mapping (action rows keep empty Kind) in `src/internal/worktree/apps.go`; update any positional `AppInfo` composite literals in `src/internal/worktree/apps_test.go` to keyed form so they compile <!-- R2 -->
- [x] T002 Add `ListableApps(apps []AppInfo) []AppInfo` filter helper (non-empty `Kind`, order-preserving, returns non-nil slice) in `src/internal/worktree/apps.go` <!-- R3 -->
- [x] T003 Tests in `src/internal/worktree/apps_test.go`: exhaustive Kind-classification test over `BuildAvailableApps()` output (every `Cmd` maps per the R2 table, unknown keys fail), and `ListableApps` filter/order-preservation test on a synthetic mixed slice <!-- R2 --> <!-- R3 --> <!-- R7 -->

### Phase 2: Command surface (cmd/wt)

- [x] T004 In `src/cmd/wt/open.go`: register `--list` and `--json` bool flags (long-form only, no shorts); add flag-check-time exclusivity guards (list+positional, list+app, list+select/--go, json-without-list → `wt.ExitWithError(wt.ExitInvalidArgs, …)`); add the `--list` branch before the soft git detection calling a thin `handleOpenList(jsonOut bool)` that filters via `wt.ListableApps(wt.BuildAvailableApps())` and renders either the aligned Id/Label/Kind table (with zero-apps message) or the JSON array (non-nil record slice, `json.MarshalIndent` mirroring `handleJSONOutput` in list.go); update the command's `Long` help text <!-- R1 --> <!-- R4 --> <!-- R5 --> <!-- R6 -->
- [x] T005 Happy-path tests in `src/cmd/wt/open_test.go`: `--list` human table (exit 0, headers, no action rows), `--list --json` shape (parse array; exactly keys id/label/kind; kind in enum; no action-row ids; order matches catalog order), id round-trip (each emitted id launches via `wt open <dir> -a <id>` under the default `WT_TEST_NO_LAUNCH=1` seam), and a direct unit test of the JSON emitter on an empty slice asserting `[]`-not-`null` <!-- R1 --> <!-- R4 --> <!-- R5 --> <!-- R7 -->
- [x] T006 Failure-path tests in `src/cmd/wt/open_test.go`: each exclusivity pair (`--list <arg>`, `--list --app code`, `--list --select`, `--list --go`, `--json` alone) exits 2 with the mutex message, plus `wt open --list` from a non-git temp dir exits 0 <!-- R6 --> <!-- R1 -->

### Phase 3: Docs & standards

- [x] T007 Update `docs/specs/cli-surface.md` (`wt open` flag table gains `--list`/`--json` rows + exit-code note) and `docs/specs/launcher-contract.md` §2 (invocation surface gains the query form, citing §6 non-breaking evolution) <!-- R8 -->
- [x] T008 Check the CLI-surface/help-output change against `shll standards` (skip gracefully with a note if `shll` is unavailable) and verify `docs/site/skill.md` does not enumerate `wt open` flags (no sync needed if so) <!-- R8 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt open --list` prints the detected-app listing and exits 0 with no menu, no launch, and no git requirement; the branch runs before git-context detection
- [x] A-002 R2: `AppInfo` carries a `Kind` field populated per the closed mapping table via named constants; menu, `-a` resolution, `DetectDefaultApp`, and `SaveLastApp` are untouched
- [x] A-003 R3: `ListableApps` lives in `src/internal/worktree/apps.go`, filters to non-empty `Kind`, and action rows remain in the menu and as valid `-a` values
- [x] A-004 R4: Human mode prints an aligned Id/Label/Kind table (zero-apps message when empty)
- [x] A-005 R5: `--list --json` emits an array of `{id, label, kind}` records with all keys always present, ids verbatim `AppInfo.Cmd`
- [x] A-006 R6: All four invalid flag combinations exit `ExitInvalidArgs` (2) at flag-check time
- [x] A-007 R7: Output order preserves `BuildAvailableApps()` detection order minus filtered rows
- [x] A-008 R8: `docs/specs/cli-surface.md` and `docs/specs/launcher-contract.md` describe the query form; standards check performed

### Scenario Coverage

- [x] A-009 R1: A test invokes `wt open --list` from a non-git directory and asserts exit 0
- [x] A-010 R5: A test asserts the empty-catalog JSON output is `[]`, not `null`
- [x] A-011 R6: Each exclusivity pair (including the deprecated `--go` alias) has a failing-path test asserting exit 2
- [x] A-012 R5: A test round-trips every emitted `id` through `wt open <dir> -a <id>` successfully (validation-source guarantee)

### Edge Cases & Error Handling

- [x] A-013 R4: Zero listable apps in human mode prints the no-apps message and exits 0 <!-- review: code path present & correct (printOpenListTable len==0 → "No launchable applications detected.", returns nil→exit 0); no direct e2e test since a real host always has ≥1 launchable app, but the JSON analogue is unit-tested (TestPrintOpenListJSON_EmptyEmitsArray) and the human branch is trivial -->

- [x] A-014 R6: `--json` without `--list` is rejected before any detection or git work

### Code Quality

- [x] A-015 Pattern consistency: New code follows the `wt list` mutex/JSON idioms and existing cobra/flag conventions (long-form, no shorts for script-facing flags)
- [x] A-016 No unnecessary duplication: single `BuildAvailableApps()` detection path; JSON emit mirrors the existing `handleJSONOutput` pattern rather than inventing a new one
- [x] A-017 No magic strings: kind enum values are named constants in `internal/worktree`

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change is purely additive: `AppInfo.Kind` is new metadata, `ListableApps`, the `--list`/`--json` flags, and the `handleOpenList`/`printOpenList*` helpers are all new. No existing file, function, branch, or config is made redundant; the menu, `-a` resolution, `DetectDefaultApp`, and `SaveLastApp` paths are untouched.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | `Kind` is a plain `string` field with three named string constants (`AppKindEditor` etc.), not a dedicated type | Matches the codebase's simple-struct style (`AppInfo` is two plain strings today); named constants satisfy the no-magic-strings anti-pattern; a dedicated type adds ceremony with no consumer | S:60 R:90 A:85 D:70 |
| 2 | Confident | Filter helper is `ListableApps(apps []AppInfo) []AppInfo` in `apps.go`, taking the slice rather than re-detecting | Intake names "a small `ListableApps()`/filter helper"; parameterized form keeps it unit-testable on synthetic slices | S:75 R:90 A:90 D:80 |
| 3 | Confident | Human zero-apps output is the message `No launchable applications detected.` with exit 0 | Mirrors `handleAppMenu`'s existing "No supported applications detected." message; intake specifies only the JSON `[]` case, human analogue follows the same non-error semantics | S:50 R:90 A:80 D:70 |
| 4 | Certain | JSON emitted via `json.MarshalIndent(…, "", "  ")` + `fmt.Println`, byte-style-identical to `wt list --json`'s `handleJSONOutput` | Intake says "mirroring the wt list --json machine-surface pattern" verbatim | S:85 R:90 A:95 D:90 |
| 5 | Certain | `--list` and `--json` get no short flags | Flag-naming memory: shorts only for common interactive use; these are script-facing query flags (same rationale as `--select`, `--dry-run`) | S:80 R:95 A:95 D:90 |
| 6 | Confident | Apply updates the two spec files (`cli-surface.md`, `launcher-contract.md`) directly in this change | Intake's Impact enumerates the exact edits and says "flag with the PR" — shipping the edits in the same branch is the only reading that keeps the PR self-consistent; specs remain human-reviewable at PR time | S:55 R:85 A:75 D:60 |
| 7 | Confident | Human table columns are two-space-padded `Id` / `Label` / `Kind` headers computed from max cell width, mirroring `wt list`'s table style | Intake asks for "a small aligned table (Id / Label / Kind columns)"; list.go's renderer is the in-repo precedent | S:70 R:95 A:90 D:80 |
| 8 | Confident | The id-equals-`-a`-token guarantee is tested by round-tripping each emitted id through `wt open <dir> -a <id>` under the default `WT_TEST_NO_LAUNCH=1` seam | The guarantee is the change's core motivation; the no-launch seam makes the round-trip side-effect-free per the project's review rules | S:65 R:90 A:85 D:80 |
| 9 | Confident | `docs/site/skill.md` needs no sync — its `wt open` line names no flags | Verified: line 29 describes `open [name|path]` without enumerating flags; the drift guard is byte-identity against the committed copy, which is unchanged | S:70 R:95 A:90 D:85 |

9 assumptions (2 certain, 7 confident, 0 tentative).
