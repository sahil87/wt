# Plan: List Recency Column

**Change**: 260601-73cv-list-recency-column
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

<!--
  AUTO-GENERATED at the apply stage entry.
  Apply parses `## Tasks` only; Review parses `## Acceptance` only.
  Section headings (`## Tasks`, `## Acceptance`) are the stable parser contract.
-->

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- JSON / `--non-interactive` output changes — the machine-readable contract (Constitution VI) is untouched; `last_active` stays `omitempty` and `--status`-only.
- The `--status` 5-column dashboard view — its layout (Name / Branch / Status / Last Active / Path) is unchanged.
- `name` / `branch` human sort modes — they stay 3-column (Name / Branch / Path) and acquire no per-worktree `os.Stat`.
- A new recency signal, finer-grained or absolute timestamps, or any new flag — none are introduced; the change reuses `RecencyOf`, `relativeTime`, and the existing flag surface.
- Changes to the shared comparator/ordering in `src/internal/worktree/recency.go` — the ordering definition is unchanged.

## wt-cli/list-status-contract: Recency Human View

### Requirement: Last Active column in the recency-ordered human view
When the resolved sort mode is `recent` AND `--status` is NOT set AND output is human (neither `--json` nor `--non-interactive`), `wt list` SHALL render a **4-column** table with columns `Name`, `Branch`, `Last Active`, `Path` in that order. The `Last Active` cell SHALL be rendered via the existing `relativeTime(t time.Time)` helper (coarse buckets `just now`, `Nm ago`, `Nh ago`, `Nd ago`; a zero `time.Time` renders as `-`). The displayed value MUST be the same recency key the rows were sorted by — no second `os.Stat`, no `git` subprocess, no TOCTOU between sort key and displayed value.

#### Scenario: Default human view shows the relative time column
- **GIVEN** a repository with a main worktree and one or more non-main worktrees with distinct recent mtimes
- **WHEN** the user runs `wt list` with no flags (default human output → `recent`)
- **THEN** the table header SHALL read `Name`, `Branch`, `Last Active`, `Path` in that order
- **AND** each non-main row SHALL show a relative-time value in the `Last Active` column matching the row's worktree-directory mtime bucket

#### Scenario: Explicit `--sort=recent` shows the column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=recent` (human output)
- **THEN** the 4-column `Name / Branch / Last Active / Path` layout SHALL be rendered, identical to the default human view

#### Scenario: Recent and stale worktrees render distinct buckets
- **GIVEN** one worktree touched seconds ago and another whose mtime is several days old
- **WHEN** the user runs `wt list` (human, recent default)
- **THEN** the recently-touched worktree's `Last Active` SHALL render `just now`
- **AND** the older worktree's `Last Active` SHALL render `Nd ago`
- **AND** the recently-touched worktree SHALL appear above the older one (newest-first ordering preserved)

#### Scenario: Displayed value equals the sort key
- **WHEN** the recent-mode human view renders a row's `Last Active`
- **THEN** the value SHALL be derived from the same recency key used to order that row (the persisted `LastActive`), not from a fresh `os.Stat` performed in the render path

### Requirement: Persist the recency sort key into LastActive
For `recent` mode, `sortEntries` SHALL write the per-entry recency key it computes back into `entries[i].LastActive` instead of discarding it after ordering. In default/basic mode this key is the `wt.RecencyOf(e.Path)` value; under `--status` `LastActive` is already non-nil and SHALL be left as-is. This write SHALL reuse the stat already paid for the sort key and MUST NOT introduce a *second* `os.Stat` for the same entry or any `git` subprocess.

The main-worktree case requires explicit handling. Today `sortEntries` partitions the main worktree out (`start = 1` when `entries[0].IsMain`; only the non-main `rest` slice has keys computed) and does NOT stat the main entry in basic mode. Because the recent-mode human view SHALL display the main worktree's own `Last Active` (see "Current-worktree and main-worktree rendering preserved"), the implementation SHALL, when `persistKey` is true and `LastActive` is nil, populate the main entry's `LastActive` via a single `wt.RecencyOf(entries[0].Path)` — exactly one stat for the main entry, paralleling how `listEntriesEnriched` already stats every entry including main. Under `--status`, the main entry's `LastActive` is already non-nil and SHALL be left as-is. The main worktree's row position is unchanged (pinned first); only its `LastActive` is populated for display.
<!-- clarified: main-worktree LastActive — sortEntries partitions main out of the key-computation `rest` slice (list.go:186-190), so basic recent mode never stats main and its Last Active would wrongly render `-`. Resolved: when persistKey is true, populate the pinned main entry's nil LastActive via a single RecencyOf(Path). Grounded in list.go and the existing per-entry stat pattern in listEntriesEnriched. -->

#### Scenario: Main worktree recency key is persisted in basic recent mode
- **GIVEN** default/basic recent mode (no `--status`), where `entries[0].IsMain` and its `LastActive` is nil on entry
- **WHEN** `sortEntries` runs with `persistKey == true`
- **THEN** the main entry's `LastActive` SHALL be set to `wt.RecencyOf(entries[0].Path)`
- **AND** exactly one `os.Stat` SHALL be performed for the main entry (no second stat, no `git` subprocess)

The write-back MUST be gated so it is NOT performed on the JSON path. In `list.go`, `sortEntries` runs (line 123) **before** the `jsonOut` branch (lines 125-127), so an unconditional write-back in `sortEntries` would cause `--json --sort=recent` to leak `last_active` (a non-nil `*time.Time` is serialized despite `omitempty`). The implementation SHALL therefore gate the persistence on the human-output path: `sortEntries` SHALL accept an explicit `persistKey bool` parameter (set `!jsonOut` by the caller, i.e. true only when output is human), and SHALL write the recency key into `entries[i].LastActive` only when `persistKey` is true. Under `--status`, `LastActive` is already non-nil from `listEntriesEnriched`, so `--status --json` continues to emit `last_active` regardless of `persistKey`. This keeps the displayed value equal to the sort key on the human path while guaranteeing the JSON path observes the same nil `LastActive` it does today.
<!-- clarified: write-back seam pinned to a `persistKey bool` parameter on `sortEntries` (caller passes `!jsonOut`). Grounded in list.go:123-129 where sortEntries precedes the jsonOut branch — an unconditional write would leak last_active into `--json --sort=recent`. Resolves the open implementation choice flagged in Assumption #8 and the serialization note. -->

#### Scenario: JSON path does not persist the recency key
- **GIVEN** `--json --sort=recent` (human output is false → `persistKey == false`)
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** it SHALL order the non-main entries newest-first using the computed keys
- **AND** it SHALL NOT write any value into `entries[i].LastActive` (the nil pointer is preserved, so `handleJSONOutput` omits `last_active` via `omitempty`)

#### Scenario: Sort key is persisted in default mode
- **GIVEN** default/basic mode (no `--status`), where `entries[i].LastActive` is nil on entry
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** each reordered non-main entry's `LastActive` SHALL be set to the `wt.RecencyOf(Path)` value used as its sort key
- **AND** no additional `os.Stat` beyond the one already computed for the sort key SHALL be performed

#### Scenario: Existing --status LastActive is not recomputed
- **GIVEN** `--status` mode, where `listEntriesEnriched` has already set `LastActive` to a non-nil pointer for every entry
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** the existing non-nil `LastActive` SHALL be used as the sort key and left unchanged (no overwrite, no re-stat)

#### Scenario: Vanished worktree yields zero time
- **GIVEN** a worktree whose directory cannot be stat'd (vanished / permission error)
- **WHEN** `sortEntries` computes its recency key via `wt.RecencyOf(Path)`
- **THEN** the key SHALL be the zero `time.Time`
- **AND** the persisted `LastActive` SHALL be that zero time, which `relativeTime` renders as `-` in the human view

### Requirement: Thread the resolved sort mode into the rendering decision
`handleFormattedOutput` SHALL receive the resolved `sortMode` (or a boolean derived from it) so it can select the 4-column recent-mode layout. It MUST NOT key the layout solely on `showStatus`. The layout selection SHALL be: `--status` → 5-column (Name / Branch / Status / Last Active / Path); else recent human mode → 4-column (Name / Branch / Last Active / Path); else (`name`/`branch` modes) → 3-column (Name / Branch / Path).

#### Scenario: Recent mode selects the 4-column layout
- **GIVEN** `showStatus == false` and resolved sort mode `recent`
- **WHEN** `handleFormattedOutput` renders
- **THEN** it SHALL emit the 4-column `Name / Branch / Last Active / Path` table

#### Scenario: Status mode still selects the 5-column layout
- **GIVEN** `showStatus == true` (any sort mode)
- **WHEN** `handleFormattedOutput` renders
- **THEN** it SHALL emit the unchanged 5-column `Name / Branch / Status / Last Active / Path` table

### Requirement: Name and branch human modes stay three columns
When the resolved sort mode is `name` or `branch` AND `--status` is NOT set, `wt list` SHALL render the unchanged 3-column `Name / Branch / Path` table with no `Last Active` column. These modes MUST NOT perform any per-worktree `os.Stat` for the purpose of rendering recency.

#### Scenario: `--sort=name` human view has no time column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=name` (human output)
- **THEN** the header SHALL read `Name`, `Branch`, `Path` with no `Last Active` column
- **AND** the rows SHALL be ordered by `Name` ascending (main pinned first)

#### Scenario: `--sort=branch` human view has no time column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=branch` (human output)
- **THEN** the header SHALL read `Name`, `Branch`, `Path` with no `Last Active` column

#### Scenario: Name/branch modes add no recency stat
- **GIVEN** `name` or `branch` mode without `--status`
- **WHEN** the list is rendered
- **THEN** no `os.Stat` SHALL be invoked to populate a recency value for display (the single-`git`-subprocess basic-mode contract is preserved)

### Requirement: Current-worktree and main-worktree rendering preserved
The current-worktree green `*` marker and the bold `(main)` name rendering SHALL be preserved in the new 4-column recent-mode layout, identical to their behavior in the existing 3- and 5-column layouts. The main worktree SHALL remain pinned to the first output row under recent mode and SHALL display its own `Last Active` value.

#### Scenario: Main worktree row in recent mode
- **GIVEN** a repository whose main worktree participates in the recent-mode human view
- **WHEN** `wt list` renders the 4-column recent view
- **THEN** the main worktree SHALL be the first data row, rendered bold as `(main)`
- **AND** its `Last Active` cell SHALL show the relative time of its own directory mtime (or `-` if zero)

#### Scenario: Current worktree marker in recent mode
- **GIVEN** the user's CWD is inside one of the listed worktrees
- **WHEN** the recent-mode 4-column view renders
- **THEN** that worktree's row SHALL be prefixed with the green `*` marker

### Requirement: Empty and single-worktree lists render the new header without rows
The recent-mode 4-column header SHALL render correctly when there are zero non-main worktrees (header plus the main row, or header plus the total line), without panic or misalignment.

#### Scenario: Repository with only the main worktree
- **GIVEN** a repository with no non-main worktrees
- **WHEN** the user runs `wt list` (human, recent default)
- **THEN** the 4-column header SHALL be emitted
- **AND** the main worktree row (with its `Last Active`) and the `Total: 1 worktree(s)` line SHALL follow without error

## wt-cli/list-status-contract: Machine Output Unchanged

### Requirement: JSON output is unchanged by this change
`--json` output SHALL NOT gain a `last_active` value as a result of this change. `last_active` SHALL remain `omitempty` on `listEntry` and SHALL be emitted only under `--status` (where it is already populated by `listEntriesEnriched`). `--json` and `--json --sort=recent` SHALL NOT emit `last_active` unless `--status` is also set. `--json` without `--sort` SHALL keep its stable `name` default order; `--json --sort=recent` SHALL order recent but still omit `last_active`. `--non-interactive` SHALL likewise keep its stable `name` default and gain no field.

#### Scenario: `--json` without `--status` omits last_active
- **GIVEN** any repository state
- **WHEN** the user runs `wt list --json` (no `--status`)
- **THEN** no object in the JSON array SHALL contain a `last_active` key
- **AND** the order SHALL be stable `name` ascending (main first)

#### Scenario: `--json --sort=recent` sorts recent but still omits last_active
- **GIVEN** a repository with multiple worktrees of distinct mtimes
- **WHEN** the user runs `wt list --json --sort=recent` (no `--status`)
- **THEN** the array SHALL be ordered newest-first (non-main entries)
- **AND** no object SHALL contain a `last_active` key (because `sortEntries` is called with `persistKey == false` on the JSON path, the nil `LastActive` pointer is preserved and `omitempty` omits the key — see Design Decisions #1 and the serialization note)

#### Scenario: `--status --json` still emits last_active
- **GIVEN** a repository with worktrees
- **WHEN** the user runs `wt list --status --json`
- **THEN** every object SHALL contain a `last_active` key (unchanged `--status` behavior), a vanished worktree serialized as `"0001-01-01T00:00:00Z"`

## wt-cli/recency-ordering-contract: Sort-Key Reuse

### Requirement: Recent-mode list persists the computed recency key for display
The `recent`-mode list path SHALL persist the recency key it computes (previously discarded after sorting) into `LastActive` for the rendering layer to display. The comparator and ordering definition (`RecencyOf` / `RecencyLess`, newest-first with Name-ascending tie-break) SHALL be unchanged — this change only stops discarding the already-computed key.

#### Scenario: Ordering definition is unchanged
- **GIVEN** the same set of worktree mtimes
- **WHEN** `wt list --sort=recent` orders them
- **THEN** the resulting order SHALL be identical to the order produced before this change (newest-first, Name-ascending tie-break)
- **AND** only the side-effect of persisting the key into `LastActive` SHALL differ

## Testability (Constitution IV / VI)

### Requirement: Behavior is covered by unit and integration tests
Every behavior above SHALL be covered by tests that assert what the user sees: the 4-column recent-mode header and a relative-time value in default human output; that `name`/`branch` human modes remain 3-column; that `--json` (with and without `--sort=recent`) omits `last_active` without `--status`; and that `--status` is unchanged. Integration tests SHALL exercise the binary end-to-end against a real git repo via `t.TempDir()`. Tests MUST NOT leak side-effects to the host (no real worktree-open/app shell-outs).

#### Scenario: Recent human header asserted by test
- **GIVEN** a temp-repo fixture with worktrees of controlled mtimes (e.g., via `os.Chtimes`)
- **WHEN** the test runs `wt list` (human default)
- **THEN** the captured stdout SHALL contain the `Last Active` header and a relative-time string for a non-main row

#### Scenario: Machine-output stability asserted by test
- **GIVEN** a temp-repo fixture
- **WHEN** the test runs `wt list --json` and `wt list --json --sort=recent`
- **THEN** in both cases no object SHALL contain a `last_active` key
- **AND** plain `--json` SHALL be `name`-ordered while `--json --sort=recent` SHALL be recency-ordered

## Design Decisions

1. **Persist the discarded sort key rather than re-stat in the render path**
   - *Why*: `sortEntries` already computes the recency key (`wt.RecencyOf(e.Path)` in basic mode, or reuses the non-nil `*LastActive` under `--status`) and discards it. Writing it back into `entries[i].LastActive` shows the value at zero additional cost and guarantees the displayed value equals the sort key.
   - *Gated by `persistKey`*: because `sortEntries` runs before the `jsonOut` branch (list.go:123-129), the write-back is gated on a `persistKey bool` parameter (caller passes `!jsonOut`). Human output persists the key (and renders it); the JSON path passes `persistKey == false`, leaving `LastActive` nil so `omitempty` keeps `last_active` out of `--json --sort=recent`.
   - *Rejected*: re-statting in `handleFormattedOutput` — reintroduces a per-worktree `os.Stat`, creates a TOCTOU drift window between the sort key and the displayed value, and duplicates recency logic the render layer should not own.
   - *Rejected*: unconditional write-back in `sortEntries` — would serialize the transiently-set `LastActive` into `--json --sort=recent`, breaking the JSON present-vs-uncomputed contract.

2. **Scope the column to recent mode only**
   - *Why*: `name`/`branch` human modes do zero per-worktree work today; adding a `Last Active` column there would force a per-worktree `os.Stat` purely for display, violating the cheap-default-path spirit of the list contract.
   - *Rejected*: showing the column in all human modes (adds stats to cheap modes); an absolute timestamp column (wider, less scannable than the relative buckets).

3. **Thread the resolved sortMode into handleFormattedOutput**
   - *Why*: the function keys layout only on `showStatus` today; recency rendering needs to know the sort mode. Passing the already-resolved `sortMode` (or a derived bool) is the minimal seam — no new state, no recomputation.
   - *Rejected*: recomputing `resolveSort` inside `handleFormattedOutput` (duplicates resolution logic and its flag inputs); inferring recency from non-nil `LastActive` (ambiguous — `--status` also sets it, and that path must stay 5-column).

4. **Leave JSON / `--non-interactive` untouched**
   - *Why*: the request is explicitly about the human recency view. Constitution VI requires deterministic machine output; fab-kit operators parse stable `name`-ordered `--json`. `last_active` stays `omitempty`/`--status`-only.
   - *Rejected*: emitting `last_active` whenever it is set (would leak the transiently-persisted recent-mode key into JSON and break the present-vs-uncomputed contract) — see the JSON-unchanged requirement and the note below.

> **Serialization note (resolved, for plan/apply)**: because `recent`-mode `sortEntries` writes back into `entries[i].LastActive`, the implementation MUST ensure `--json` does not begin emitting `last_active` as a side effect. Correction to an earlier draft: `sortEntries` runs at list.go:123 **before** the `jsonOut` branch at list.go:125-127, so the write-back IS observable on the JSON path — the order does NOT protect JSON on its own. The resolved seam: `sortEntries` takes a `persistKey bool` (caller passes `!jsonOut`); the key is written into `LastActive` only on the human path. For `--json --sort=recent`, `persistKey == false`, the nil pointer is preserved, and `omitempty` omits `last_active`. Bare `--json` additionally stays `name`-ordered via `resolveSort`. This fully resolves the implementation choice flagged in Assumptions #8 — no open question remains.


## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 1: Core Implementation — sortEntries write-back

<!-- Persist the already-computed recency key into LastActive on the human path only. Order matters: signature change first, then the gated write-backs that depend on it. -->

- [x] T001 In `src/cmd/wt/list.go`, change the `sortEntries` signature to `sortEntries(entries []listEntry, mode sortMode, persistKey bool)`. Update the call site (~line 123) in `listCmd`'s `RunE` to pass `!jsonOut` as the third argument (true only on the human output path, false for `--json`). <!-- A-002, A-004 -->
- [x] T002 In `src/cmd/wt/list.go` `sortEntries`, within the `sortRecent` branch (after the `keys[]` slice is computed, ~lines 206-226), when `persistKey` is true write each non-main entry's recency key back into `rest[i].LastActive` only when it is nil (i.e. `&keys[i]`), reusing the already-paid stat. Do NOT overwrite a non-nil `LastActive` (the `--status` path). Do NOT write anything when `persistKey` is false. <!-- A-001, A-003, A-004, A-007 -->
- [x] T003 In `src/cmd/wt/list.go` `sortEntries`, handle the pinned main worktree: when `persistKey` is true, `mode == sortRecent`, `len(entries) > 0 && entries[0].IsMain`, and `entries[0].LastActive == nil`, populate it via a single `wt.RecencyOf(entries[0].Path)` (exactly one stat for main, no git subprocess). Leave a non-nil main `LastActive` (the `--status` path) untouched; do not change main's pinned-first position. <!-- A-005, A-008 -->

### Phase 2: Core Implementation — recent-mode rendering

<!-- Thread the resolved sort mode into the renderer and add the 4-column branch. Depends on Phase 1 having populated LastActive. -->

- [x] T004 In `src/cmd/wt/list.go`, change `handleFormattedOutput` to receive the resolved sort mode (add a `sortMode sortMode` parameter, or a derived `recentLayout bool`). Update the call site (~line 129) in `listCmd` to pass the resolved mode (capture the result of `resolveSort(...)` into a local at ~line 123 and reuse it for both `sortEntries` and `handleFormattedOutput`). Do NOT key the layout on `showStatus` alone. <!-- A-006 -->
- [x] T005 In `src/cmd/wt/list.go` `handleFormattedOutput`, in the per-entry row build loop populate `lastActive` from `relativeTime(*e.LastActive)` for the recent-mode layout too (currently only set when `showStatus`); guard the deref on `e.LastActive != nil`, falling back to `relativeTime` of the zero time (renders `-`) so a nil/vanished entry stays aligned. Reuse the existing `relativeTime` helper — no second `os.Stat`, no new formatting. <!-- A-001, A-009 -->
- [x] T006 In `src/cmd/wt/list.go` `handleFormattedOutput`, add the 4-column recent-mode rendering branch between the `--status` (5-column) branch and the `name`/`branch` (3-column) `else` branch. Selection: `showStatus` → 5-column (Name/Branch/Status/Last Active/Path, unchanged); else recent layout → 4-column (Name/Branch/Last Active/Path); else → 3-column (Name/Branch/Path, unchanged). Give the 4-column branch its own header array, `colWidths` computation (Name/Branch/LastActive/Path), header `Printf`, and row `Printf` that preserves the green `*` current-marker and bold `(main)` rendering. <!-- A-001, A-006, A-010, A-011 -->

### Phase 3: Tests — update existing, add new

<!-- test-alongside per code-quality.md. Update the two tests that encode the old (status-only) Last Active contract, then add coverage for the new behaviors. Standard runWt isolation applies (no worktree-open/app shell-outs). -->

- [x] T007 In `src/cmd/wt/list_test.go`, UPDATE `TestList_HumanDefaultIsRecency` (~line 614): keep the recency-order assertion and additionally assert the human default output now contains the `Last Active` header and a relative-time value (e.g. via `relativeTime`-style bucket strings) for a non-main row, reflecting the 4-column amendment. <!-- A-012 -->
- [x] T008 In `src/cmd/wt/list_test.go`, UPDATE `TestList_LastActiveRelativeTimeInHumanStatus` (~line 670) and/or its comment so it no longer encodes "Last Active appears ONLY under --status" as an invariant. Keep its `--status` "2h ago" assertion intact (the 5-column view is unchanged); adjust naming/comment to reflect that the column is now also shown in recent human mode. <!-- A-011, A-013 -->
- [x] T009 [P] In `src/cmd/wt/list_test.go`, ADD a test asserting the default human view (`wt list`) emits the 4-column `Last Active` header AND a relative-time string on a non-main row whose mtime is set via `chtimesWt` (e.g. `just now` and `Nd ago` for distinct buckets), with newest-first order preserved. <!-- A-001, A-012 -->
- [x] T010 [P] In `src/cmd/wt/list_test.go`, ADD a test asserting `wt list --sort=name` and `wt list --sort=branch` human output do NOT contain a `Last Active` header (3-column layout preserved). <!-- A-010 -->
- [x] T011 [P] In `src/cmd/wt/list_test.go`, ADD a test asserting `wt list --json` and `wt list --json --sort=recent` produce no `last_active` key in any array object (extend/parallel `TestList_LastActiveOmittedInDefaultMode`), and that `--json --sort=recent` is recency-ordered while bare `--json` stays name-ordered. <!-- A-003, A-014 -->
- [x] T012 [P] In `src/cmd/wt/list_test.go`, ADD a test asserting the main worktree row in recent human mode shows its own relative-time `Last Active` (not `-`) — set the main repo dir mtime via `os.Chtimes`/`chtimesWt`, run `wt list`, and assert the `(main)` row carries a relative-time bucket. <!-- A-005, A-011 -->
- [x] T013 [P] In `src/cmd/wt/list_test.go`, ADD a test asserting a vanished/unstat-able worktree renders `-` in the recent-mode `Last Active` column (zero `time.Time` → `-` via `relativeTime`). <!-- A-009 -->
- [x] T014 [P] In `src/cmd/wt/list_test.go`, ADD a test for a repository with only the main worktree (no non-main worktrees): run `wt list` (human, recent default) and assert the 4-column `Last Active` header is emitted, the `(main)` row follows, and the `Total: 1 worktree(s)` line renders without panic or column misalignment. The existing `TestList_SucceedsWithNoWorktrees` only asserts the total line, not the new header. <!-- A-018 -->
<!-- clarified: added T014 to cover the spec's "Empty and single-worktree lists render the new header without rows" requirement (spec lines 135-143, "Repository with only the main worktree" scenario), which had no dedicated task — the recent 4-column header/colWidths path must render correctly when `rest` is empty and only the pinned main row exists. Resolvable: low-blast-radius test add at the same runWtSuccess binary level as the other tasks; T003 already populates main's LastActive on this path. -->

### Phase 4: Verification

- [x] T015 Run `go test ./src/cmd/wt/...` (then `go test ./...` if the scoped run is green) and confirm all list unit/integration tests pass, including the updated and newly added cases. Fix any failures at the source. <!-- A-015 -->

## Execution Order

- T001 → T002, T003 (write-backs depend on the new `persistKey` parameter).
- T004 → T005 → T006 (renderer needs the threaded mode before the 4-column branch is meaningful; row population precedes the print branch).
- Phase 2 depends on Phase 1 (LastActive must be populated before the renderer reads it).
- Phase 3 tests depend on Phases 1-2; T009-T014 are `[P]` (distinct test functions, no shared state). T007/T008 edit existing functions — run before/independently of the new adds.
- T015 last.

## Acceptance

<!-- Declarative outcomes verified by the review stage. -->

### Functional Completeness

- [ ] A-001 Last Active column in recency human view: with resolved mode `recent`, no `--status`, and human output, `wt list` renders a 4-column `Name / Branch / Last Active / Path` table; each non-main `Last Active` cell is the `relativeTime` rendering of the row's recency sort key.
- [ ] A-002 sortEntries signature: `sortEntries` accepts `persistKey bool`; the `listCmd` call site passes `!jsonOut`.
- [ ] A-003 JSON unchanged: `--json` and `--json --sort=recent` (without `--status`) emit no `last_active` key (`persistKey == false` leaves the pointer nil; `omitempty` omits it); `--status --json` still emits `last_active`.
- [ ] A-004 Persist sort key on human path: in `recent` mode with `persistKey == true`, each reordered non-main entry's nil `LastActive` is set to its `wt.RecencyOf(Path)` sort key, with no `os.Stat` beyond the one already computed for the key; a non-nil `LastActive` (`--status`) is left unchanged.
- [ ] A-005 Main-worktree Last Active: when `persistKey == true`, `recent` mode, and the pinned main entry's `LastActive` is nil, it is populated via a single `wt.RecencyOf(entries[0].Path)`; the main row shows its own relative time (not `-`) and stays pinned first.
- [ ] A-006 Sort mode threaded into renderer: `handleFormattedOutput` selects layout from the resolved sort mode (not `showStatus` alone): `--status` → 5-column, recent human → 4-column, `name`/`branch` → 3-column.

### Behavioral Correctness

- [ ] A-007 No second stat / no git subprocess: the write-back reuses the key already computed in `sortEntries`; no additional `os.Stat` per non-main entry and no `git` subprocess is introduced for display.
- [ ] A-008 Single main stat: at most one `os.Stat` is performed for the main entry, only when `persistKey` is true and `LastActive` is nil.
- [ ] A-009 Vanished worktree: an unstat-able worktree's recency key is the zero `time.Time`, persisted as such, and rendered `-` by `relativeTime` in the recent human view.
- [ ] A-010 Name/branch modes unchanged: `--sort=name` and `--sort=branch` human output remain 3-column `Name / Branch / Path` with no `Last Active` column and no per-worktree recency `os.Stat`.
- [ ] A-011 Current/main markers preserved: the green `*` current-worktree marker and bold `(main)` name render correctly in the 4-column layout; `--status` 5-column layout is byte-for-byte unchanged.

### Scenario Coverage

- [ ] A-012 Recent human header asserted: a test confirms default `wt list` output contains the `Last Active` header and a relative-time value for a non-main row.
- [ ] A-013 Status view unchanged asserted: a test confirms `--status` still renders `Last Active` with the expected relative time (5-column view intact).
- [ ] A-014 Machine-output stability asserted: a test confirms `--json` and `--json --sort=recent` omit `last_active`, with `--json` name-ordered and `--json --sort=recent` recency-ordered.
- [ ] A-015 Suite green: `go test ./src/cmd/wt/...` (and `go test ./...`) passes, including updated `TestList_HumanDefaultIsRecency` / `TestList_LastActiveRelativeTimeInHumanStatus` and the new cases.
- [ ] A-018 Main-only repo asserted: a test confirms a repository with only the main worktree renders the recent-mode 4-column `Last Active` header, the `(main)` row, and the `Total: 1 worktree(s)` line without panic or misalignment (zero non-main entries). <!-- clarified: covers spec "Empty and single-worktree lists render the new header without rows" requirement, previously untested by the A-NNN set. -->
<!-- clarified: A-018 added to give the spec's empty/single-worktree header requirement (spec lines 135-143) an acceptance criterion; verified by new task T014. -->

### Code Quality

- [ ] A-016 Pattern consistency: new rendering branch and write-back follow the existing `handleFormattedOutput` / `sortEntries` structure, width-computation idiom, and color/marker helpers.
- [ ] A-017 No unnecessary duplication: the change reuses `relativeTime`, `RecencyOf`, `displayWidth`, and `relativePath` rather than reimplementing recency or formatting logic; the comparator/ordering in `src/internal/worktree/recency.go` is untouched.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
- Constitution IV: these list tests do not open worktrees or shell out to apps, so standard `runWt` env isolation applies — no `t.Cleanup` window-reaping is required. The existing `cmd/integration_test.go` already exercises `wt list` end-to-end against a real `t.TempDir()` repo; the new assertions are added at the same end-to-end (binary) level via `runWtSuccess`, so a separate integration fixture is not warranted for this display-only change.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = recent mode only (default human view + `--sort=recent`); `name`/`branch` human modes stay 3-column with no recency stat | Carried from spec #1. Preserves the cheap-default-path contract; adds no `os.Stat` to modes that do zero per-worktree work | S:95 R:80 A:90 D:90 |
| 2 | Certain | Reuse `relativeTime()` coarse buckets (`just now`/`Nm`/`Nh`/`Nd`; zero → `-`) for the column value | Carried from spec #2. Zero new formatting logic; consistent with the `--status` Last Active column | S:95 R:85 A:95 D:90 |
| 3 | Certain | The `Name/Branch/Path` default-layout change is an intentional amendment to `list-status-contract.md`, documented at hydrate | Carried from spec #3. The default-human-output invariant is superseded for recency-ordered output by explicit amendment | S:90 R:70 A:85 D:85 |
| 4 | Certain | Persist the discarded `sortEntries` recency key into `entries[i].LastActive` rather than re-statting in the render path | Carried from spec #4. Reuses the already-paid stat; avoids TOCTOU between sort key and displayed value | S:95 R:75 A:85 D:80 |
| 5 | Certain | JSON / `--non-interactive` output unchanged — `last_active` stays `omitempty`/`--status`-only; machine modes keep stable `name` default | Carried from spec #5. Preserves Constitution VI machine-stability and the JSON present-vs-uncomputed contract | S:95 R:70 A:90 D:85 |
| 6 | Certain | Thread the resolved `sortMode` (or a derived bool) into `handleFormattedOutput` to select the 4-column layout | Carried from spec #6. The function keys only on `showStatus` today; threading the already-resolved mode is the minimal seam | S:95 R:75 A:80 D:75 |
| 7 | Certain | Column order = `Name / Branch / Last Active / Path` (4 columns) | Carried from spec #7. Mirrors the `--status` relative placement (Last Active immediately before the wide Path) | S:95 R:85 A:60 D:55 |
| 8 | Certain | Gate the `LastActive` write-back on a `persistKey bool` parameter to `sortEntries` (caller passes `!jsonOut`); JSON path passes `false`, leaving the pointer nil so `omitempty` omits the key | Carried from spec #8. `sortEntries` (line 123) runs before the `jsonOut` branch (lines 125-127); the gated `persistKey` seam is the single correct mechanism, grounded in the real call order | S:95 R:85 A:90 D:90 |
| 9 | Certain | Populate the pinned main worktree's nil `LastActive` via a single `wt.RecencyOf(Path)` when `persistKey` is true — `sortEntries` partitions main out of the key-computation `rest` slice today | Carried from spec #9. Without this the main row's Last Active would render `-`; one extra stat for the single main entry parallels `listEntriesEnriched` | S:90 R:85 A:85 D:85 |
| 10 | Certain | Update (not just add) `TestList_HumanDefaultIsRecency` and `TestList_LastActiveRelativeTimeInHumanStatus` because they encode the old status-only Last Active contract | Derived from the source: both tests pass `humanNonMainOrder`/`--status` assertions that survive mechanically, but their intent encodes the superseded contract; test-alongside (code-quality.md) requires them to reflect the amended behavior | S:90 R:90 A:90 D:85 |

10 assumptions (10 certain, 0 confident, 0 tentative, 0 unresolved).
