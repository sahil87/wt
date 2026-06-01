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
