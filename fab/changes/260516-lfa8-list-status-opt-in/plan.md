# Plan: wt list — status opt-in, fast by default

**Change**: 260516-lfa8-list-status-opt-in
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Tasks

### Phase 1: Setup

- [x] T001 Update `listEntry` struct in `src/cmd/wt/list.go:22-30` — switched to `*bool`/`*int` pointer fields with `omitempty` tags. Reason: simple `omitempty` on plain `bool`/`int` would also omit keys when status WAS computed but happens to be `false`/`0`, violating the spec requirement that keys be present whenever `--status` is set. Pointers cleanly distinguish "not computed" (nil) from "computed and clean" (non-nil zero).

### Phase 2: Core Implementation

- [x] T002 Added `--status` flag, updated cobra `Long:` description.
- [x] T003 Added `--status` + `--path` mutual-exclusivity check (alongside the existing `--path`/`--json` check).
- [x] T004 Split into `listEntriesBasic` (no enrichment, zero per-worktree git calls) and `listEntriesEnriched` (parallel enrichment). Shared `buildBaseEntry` helper.
- [x] T005 Bounded worker pool via buffered semaphore + `sync.WaitGroup`. `maxListConcurrency = 8` named constant. Output ordering preserved by indexed-slice writes.
- [x] T006 Replaced `checkDirty` with single `git status --porcelain`. Non-zero exit → clean.
- [x] T007 Replaced `getUnpushedInDir` with single `git rev-list --count @{u}..HEAD`. No more `git rev-parse`. Non-zero exit → 0. Branch param dropped (no longer needed).
- [x] T008 `handleFormattedOutput` now branches on `showStatus`. Default mode renders 3 columns; `--status` mode renders 4 columns with Status. Current-marker and `(main)` rendering preserved in both.

### Phase 3: Integration & Edge Cases

- [x] T009 `RunE` dispatches to `listEntriesBasic`/`listEntriesEnriched` and passes `statusFlag` into `handleFormattedOutput`. `handleJSONOutput` unchanged — the pointer-based `omitempty` handles JSON shape automatically.
- [x] T010 Detached-HEAD guard preserved: enriched-path goroutine skips `getUnpushedInDir` when `branch == "(detached)"` and leaves `Unpushed` pointer set to `&0` (key still present with value 0 in JSON, which is correct).

### Phase 4: Tests & Documentation

- [x] T011 [P] Update `src/cmd/wt/list_test.go` — split tests into default-mode and `--status`-mode coverage per spec:
    - `TestList_Header`: remove `assertContains(t, r.Stdout, "Status")`; add `assertNotContains(t, r.Stdout, "Status")`.
    - `TestList_JSONAllFields`: remove `dirty` and `unpushed` from `requiredFields`; assert their absence in the JSON object.
    - `TestList_JSONDetectsDirty`: add `"--status"` to the `runWtSuccess` args.
    - `TestList_DirtyIndicator`: rename and split — keep one default-mode test asserting `*` is absent on the dirty row, add a `--status`-mode test asserting `*` is present.
    - Add `TestList_StatusFlagPresent`: assert `--status` produces Status header.
    - Add `TestList_StatusFlagShowsDirty`: assert dirty `*` marker appears under `--status`.
    - Add `TestList_StatusFlagShowsUnpushed`: assert `↑N` marker appears under `--status` (after creating a branch ahead of upstream).
    - Add `TestList_StatusJSONIncludesFields`: assert JSON object has `dirty` and `unpushed` keys when `--status` is set.
    - Add `TestList_StatusAndPathMutuallyExclusive`: assert `ExitInvalidArgs` and stderr message when both flags are combined.
    - Add `TestList_StatusOrderingPreserved`: assert row order matches porcelain output even with parallel enrichment.
- [x] T012 [P] Updated `docs/specs/cli-surface.md` `wt list` section — added `--status` row, rewrote default-output prose (Name/Branch/Path only), added `--status` paragraph, extended exit-code prose to cover `--path` ↔ `--status` exclusivity.
- [x] T013 `go build ./src/...` clean, `go vet ./src/...` clean, `go test ./src/cmd/wt/... -run TestList` → 24/24 pass; `go test ./src/cmd/wt/... -run TestIntegration` → all pass. Pre-existing `TestCreate_WorktreeOpenDefault` failure on this host is environment-related (no default app on GCE Linux), confirmed by re-running against the stashed pre-change tree — unrelated to this change.

## Execution Order

- T001 is independent (struct tags only).
- T002-T010 are sequential and form the core implementation chain. T004 introduces the split paths used by T005-T010.
- T011 and T012 are `[P]` — independent of each other and run after T002-T010 land. They can interleave with T013.
- T013 is the final gate.

## Acceptance

### Functional Completeness

- [x] A-001 Default-mode enrichment-free: `wt list` spawns exactly 1 `git` subprocess regardless of worktree count.
- [x] A-002 Default human output: Name/Branch/Path columns only; no `Status` header.
- [x] A-003 Default JSON output: `dirty` and `unpushed` keys absent from every object.
- [x] A-004 `--status` flag exists and is documented in `wt list --help`.
- [x] A-005 `--status` formatted output: Status column present; `*` and `↑N` markers render correctly.
- [x] A-006 `--status` JSON output: `dirty` and `unpushed` keys present in every object regardless of value.
- [x] A-007 Parallel enrichment: bounded worker pool with `min(runtime.NumCPU(), 8)` workers, no third-party pool library.
- [x] A-008 Dirty detection: single `git status --porcelain` per worktree (no `git diff` / `git ls-files`).
- [x] A-009 Unpushed detection: single `git rev-list --count @{u}..HEAD` per worktree (no separate `git rev-parse`).
- [x] A-010 `--path` mode unchanged: skips enrichment entirely.
- [x] A-011 `cli-surface.md` updated: flag table includes `--status`, prose reflects new default behavior.

### Behavioral Correctness

- [x] A-012 Output ordering under parallelism matches porcelain order (verified by `TestList_StatusOrderingPreserved`).
- [x] A-013 Current-worktree marker (green `*`) still appears on the correct row in both modes.
- [x] A-014 Bold `(main)` rendering still appears in both modes.
- [x] A-015 Detached HEAD skips `git rev-list --count @{u}..HEAD` and reports 0 unpushed.

### Removal Verification

- [x] A-016 No code path invokes `git diff`, `git diff --cached`, or `git ls-files --others --exclude-standard` from `wt list`.
- [x] A-017 No code path invokes `git rev-parse --abbrev-ref` from `wt list`.
- [x] A-018 The old 3-call `checkDirty` body is deleted (not just dead-coded behind a flag).
- [x] A-019 The old `getUnpushedInDir` two-call body is deleted.

### Scenario Coverage

- [x] A-020 25-worktree-repo default invocation: covered by code inspection (single porcelain call) — no automated perf test in this change.
- [x] A-021 Branch-with-upstream and branch-without-upstream both handled. `TestList_StatusFlagShowsUnpushed` verifies the ahead-of-upstream case (`↑2` appears in output). The no-upstream case is covered by code inspection at `src/cmd/wt/list.go:438-440` (non-zero exit from `git rev-list --count @{u}..HEAD` returns 0, matching pre-change semantics).
- [x] A-022 Unreadable `.git` worktree: default mode skips enrichment so no error surfaces — covered by A-001 (zero per-worktree calls).
- [x] A-023 `--status` + `--path` rejected: verified by `TestList_StatusAndPathMutuallyExclusive`.
- [x] A-024 `--status` + `--json` permitted: verified by `TestList_StatusJSONIncludesFields`.

### Edge Cases & Error Handling

- [x] A-025 `git status` non-zero exit: worktree reported as clean; no stderr leak.
- [x] A-026 `git rev-list` non-zero exit: unpushed reported as 0; no stderr leak.
- [x] A-027 Mutual-exclusivity violation: exit `ExitInvalidArgs` with clear stderr message.

### Code Quality

- [x] A-028 Pattern consistency: new code follows the naming and structural patterns of surrounding `cmd/wt` package code (lowercase package-local helpers, `*RepoContext` plumbing, cobra flag binding pattern).
- [x] A-029 No unnecessary duplication: existing `listWorktreeEntries` reused, `ColorYellow`/`ColorGreen`/etc. constants reused.
- [x] A-030 Readability over cleverness: worker pool uses idiomatic Go channel + WaitGroup; no clever lock-free constructs.
- [x] A-031 Anti-pattern: no god functions — `getEnrichedEntries` is split, not extended. New helpers stay <50 lines.
- [x] A-032 Anti-pattern: no magic numbers — pool-size cap of 8 is a named constant (e.g., `maxListConcurrency`) or inline-commented.

### Test Coverage

- [x] A-033 All existing `list_test.go` tests pass (after the split/update).
- [x] A-034 New `--status`-mode tests cover: flag presence, Status header, dirty marker, unpushed marker, JSON shape, ordering, mutual exclusivity.
- [x] A-035 Integration tests (`integration_test.go`) continue to pass without modification (per spec Assumption #17).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
