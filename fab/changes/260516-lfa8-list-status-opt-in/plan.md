# Plan: wt list — status opt-in, fast by default

**Change**: 260516-lfa8-list-status-opt-in
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- **Caching of `git worktree list` output** — out of scope; the porcelain call is already fast (<50ms on warm cache). Add later if measurements show a need.
- **Per-worktree progress indicators during `--status` enrichment** — out of scope; the target of ≤1s for 25 worktrees does not warrant a spinner.
- **Configurable concurrency** — explicitly rejected (Assumption #11). Worker pool size is hardcoded; no flag, no env var.
- **Footer hint** advertising `--status` from default output — explicitly rejected (Assumption #10).
- **`--reuse`-style legacy flag** for restoring pre-change default — explicitly rejected (Assumption #3). CLI is pre-1.0; ship the breaking default change cleanly.
- **Cross-worktree summary metrics** in `--status` mode (e.g., "3 dirty, 1 unpushed") — out of scope.

## wt list: Default output is enrichment-free

### Requirement: Default `wt list` SHALL NOT spawn per-worktree git subprocesses

The default `wt list` invocation (no `--status` flag, no `--path` flag) SHALL produce output using only the single `git worktree list --porcelain` invocation already performed by `listWorktreeEntries()`. It MUST NOT invoke `git diff`, `git diff --cached`, `git ls-files --others`, `git rev-parse`, `git rev-list`, or `git status` against any worktree directory. The number of `git` subprocesses MUST be exactly 1 regardless of the number of worktrees in the repository.

#### Scenario: 25-worktree repo, default invocation
- **GIVEN** a repository with 25 worktrees, each potentially dirty and/or ahead of upstream
- **WHEN** the user runs `wt list`
- **THEN** exactly 1 `git` subprocess (`git worktree list --porcelain`) SHALL be spawned
- **AND** wall-clock time SHALL be ≤100ms on a warm filesystem cache
- **AND** the output table SHALL display Name, Branch, and Path columns only

#### Scenario: Worktree with unreadable `.git` directory
- **GIVEN** a worktree whose `.git` symlink target is unreadable (permissions, broken link)
- **WHEN** the user runs `wt list` (default mode)
- **THEN** the worktree SHALL still appear in the output
- **AND** no error SHALL be surfaced to the user
- **AND** no per-worktree git invocation SHALL occur

### Requirement: Default human output SHALL omit the Status column

The default formatted output produced by `handleFormattedOutput` SHALL render three columns: `Name`, `Branch`, `Path`. The `Status` column header and per-row status cells SHALL NOT be emitted. The current-worktree marker (green `*` prefix on the marker column) and the main-worktree highlight (`(main)` rendered bold) SHALL be preserved.

#### Scenario: Default output formatting
- **GIVEN** 3 worktrees including main, one dirty, one with unpushed commits
- **WHEN** the user runs `wt list`
- **THEN** stdout SHALL include header tokens `Name`, `Branch`, `Path`
- **AND** stdout SHALL NOT include the header token `Status`
- **AND** the dirty marker `*` (yellow) and unpushed marker `↑N` SHALL NOT appear in any data row
- **AND** the current-worktree green `*` prefix SHALL still mark the correct row
- **AND** the bold `(main)` rendering SHALL still appear on the main row

### Requirement: Default JSON output SHALL omit `dirty` and `unpushed` fields

When `wt list --json` is invoked without `--status`, each emitted JSON object SHALL contain the keys `name`, `branch`, `path`, `is_main`, `is_current` and SHALL NOT contain the keys `dirty` or `unpushed`. Omission is implemented via `omitempty` JSON tags on `Dirty` and `Unpushed` combined with not populating those fields in default mode. Defaulting these fields to `false`/`0` SHALL be considered incorrect — a consumer reading `dirty: false` MUST be able to trust that the worktree was actually checked, not that the check was skipped.

#### Scenario: Default JSON shape
- **GIVEN** a worktree that would otherwise be dirty and 2 commits ahead
- **WHEN** the user runs `wt list --json` (no `--status`)
- **THEN** the JSON object for that worktree SHALL contain `name`, `branch`, `path`, `is_main`, `is_current` keys
- **AND** the JSON object SHALL NOT contain the `dirty` key
- **AND** the JSON object SHALL NOT contain the `unpushed` key

## wt list: `--status` flag opts back into enrichment

### Requirement: `wt list --status` SHALL produce enriched output with dirty/unpushed information

When the `--status` flag is present, `wt list` SHALL compute per-worktree `Dirty` and `Unpushed` values and SHALL include them in both formatted and JSON output. The Status column SHALL be restored to the formatted table; the `dirty` and `unpushed` keys SHALL be present in every JSON object regardless of value. The flag SHALL be mutually compatible with `--json` but mutually exclusive with `--path` (the existing `--path` exclusivity rules continue to apply unchanged; `--status` combined with `--path` SHALL exit with `ExitInvalidArgs` and a clear error message).

#### Scenario: `--status` with formatted output
- **GIVEN** 2 worktrees: one dirty, one with 1 unpushed commit
- **WHEN** the user runs `wt list --status`
- **THEN** the output SHALL contain a `Status` header
- **AND** the dirty worktree row SHALL contain the yellow `*` marker
- **AND** the unpushed worktree row SHALL contain the yellow `↑1` marker
- **AND** wall-clock time SHALL be ≤1s on a 25-worktree repo with a warm filesystem cache

#### Scenario: `--status --json` combined
- **GIVEN** the conditions above
- **WHEN** the user runs `wt list --status --json`
- **THEN** each JSON object SHALL contain `dirty` (boolean) and `unpushed` (number) fields
- **AND** absent or zero values SHALL still emit the keys (`dirty: false`, `unpushed: 0`)

#### Scenario: `--status --path` rejected
- **GIVEN** the user attempts to combine `--status` with `--path foo`
- **WHEN** the command runs
- **THEN** the command SHALL exit with `ExitInvalidArgs`
- **AND** stderr SHALL contain a message indicating `--path` is incompatible with the other flag

### Requirement: `--status` enrichment SHALL run in parallel with a bounded worker pool

Per-worktree enrichment in `--status` mode SHALL execute via a bounded worker pool. The pool size SHALL be `min(runtime.NumCPU(), 8)`. The pool size SHALL NOT be configurable via flag or environment variable in this change. Output ordering SHALL match the order returned by `git worktree list --porcelain` — parallelism MUST NOT reorder rows.

#### Scenario: Output ordering preserved under parallelism
- **GIVEN** 10 worktrees where enrichment of worktree #5 takes longer than the others (e.g., a deliberately slow `git status`)
- **WHEN** the user runs `wt list --status`
- **THEN** worktree #5 SHALL appear in position 5 in the output table
- **AND** all other worktrees SHALL appear in their porcelain-determined positions

#### Scenario: Pool size derivation
- **GIVEN** a host with `runtime.NumCPU()` reporting 16
- **WHEN** `--status` enrichment runs
- **THEN** the worker pool SHALL be capped at 8 concurrent workers

#### Scenario: Pool size on low-core host
- **GIVEN** a host with `runtime.NumCPU()` reporting 2
- **WHEN** `--status` enrichment runs
- **THEN** the worker pool SHALL use 2 concurrent workers

### Requirement: Dirty detection SHALL use a single `git status --porcelain` invocation

The `checkDirty` helper SHALL be replaced with a single `git status --porcelain` invocation per worktree. A worktree is considered dirty if and only if the trimmed stdout of `git status --porcelain` is non-empty. The previous 3-call sequence (`git diff --quiet`, `git diff --cached --quiet`, `git ls-files --others --exclude-standard`) SHALL be removed.

#### Scenario: Untracked file detected
- **GIVEN** a worktree containing only an untracked file (no staged or unstaged changes)
- **WHEN** `wt list --status` runs
- **THEN** the worktree SHALL be reported as dirty

#### Scenario: Clean worktree
- **GIVEN** a worktree with no staged, unstaged, or untracked changes
- **WHEN** `wt list --status` runs
- **THEN** the worktree SHALL be reported as clean (no `*` marker; `dirty: false` in JSON)

#### Scenario: `git status` invocation failure
- **GIVEN** a worktree where `git status --porcelain` exits non-zero (e.g., corrupted index)
- **WHEN** `wt list --status` runs
- **THEN** the worktree SHALL be reported as clean (graceful degradation matches current `checkDirty` behavior)
- **AND** no error SHALL be surfaced to stderr from the enrichment step

### Requirement: Unpushed detection SHALL use a single `git rev-list --count @{u}..HEAD`

The `getUnpushedInDir` helper SHALL be replaced with a single `git rev-list --count @{u}..HEAD` invocation per worktree. The separate `git rev-parse --abbrev-ref` upstream lookup SHALL be removed; the `@{u}` shorthand resolves the upstream inline. If `@{u}` is unset (no upstream configured) the command exits non-zero; this SHALL be treated as zero unpushed commits.

#### Scenario: Branch with no upstream
- **GIVEN** a worktree on a branch with no configured upstream
- **WHEN** `wt list --status` runs
- **THEN** the unpushed count SHALL be reported as 0
- **AND** no error SHALL surface to stderr from the enrichment step

#### Scenario: Branch 3 commits ahead of upstream
- **GIVEN** a worktree on a branch with an upstream, with 3 local commits not yet pushed
- **WHEN** `wt list --status` runs
- **THEN** the unpushed count SHALL be reported as 3
- **AND** the formatted output SHALL render `↑3`

#### Scenario: Detached HEAD
- **GIVEN** a worktree in detached-HEAD state
- **WHEN** `wt list --status` runs
- **THEN** the unpushed count SHALL be reported as 0
- **AND** no per-detached-worktree `git rev-list` invocation SHALL occur

## wt list: `--path` lookup mode is unchanged

### Requirement: `--path` MUST continue to skip enrichment

The `--path <name>` lookup mode SHALL continue to use the raw `listWorktreeEntries` path with no enrichment. It SHALL NOT invoke `checkDirty`, the new `git status --porcelain` helper, or the unpushed-count helper, regardless of whether `--status` is also set. (Although `--path` combined with `--status` SHALL be rejected per the `--status` requirement, this requirement guards against future regressions where enrichment might be invoked before flag validation.)

#### Scenario: `--path` is fast
- **GIVEN** a 25-worktree repo
- **WHEN** the user runs `wt list --path some-name`
- **THEN** exactly 1 `git` subprocess (`git worktree list --porcelain`) SHALL be spawned
- **AND** wall-clock time SHALL match pre-change performance (already under 100ms)

## wt list: Help text reflects new flag

### Requirement: `--help` SHALL document the new `--status` flag

The `--help` output for `wt list` SHALL include `--status` as a documented flag with description "Show dirty/unpushed status for each worktree (slower)". The cobra `Long:` description SHALL mention that `--status` enables the per-worktree git checks and is the slower mode.

#### Scenario: `--help` shows `--status`
- **GIVEN** any environment
- **WHEN** the user runs `wt list --help`
- **THEN** stdout SHALL contain the substring `--status`
- **AND** stdout SHALL contain a description identifying `--status` as the slower mode

## wt list: Test coverage split per output mode

### Requirement: Tests SHALL be split into default-mode and `--status`-mode coverage

The unit tests in `src/cmd/wt/list_test.go` SHALL be reorganized so that:

- Default-mode tests assert (a) the `Status` column header is absent, (b) no per-worktree `git` subprocesses are spawned, (c) JSON output omits `dirty` and `unpushed` keys.
- `--status`-mode tests assert (a) the `Status` column is present, (b) dirty / unpushed values are correct, (c) parallel execution preserves row ordering.

Existing tests `TestList_JSONAllFields`, `TestList_JSONDetectsDirty`, and `TestList_DirtyIndicator` SHALL be either updated to pass `--status` or split into a default-mode counterpart that asserts the new omission semantics. Existing scaffold tests (`TestList_ShowsMainRepo`, `TestList_PathReturnsAbsolutePath`, `TestList_PathAndJSONMutuallyExclusive`, `TestList_NoColorSupport`) SHALL continue to pass without modification.

Integration tests in `src/cmd/wt/integration_test.go` that invoke `wt list` SHALL continue to pass without modification — they assert worktree name appears in stdout, which the new default output still satisfies.

#### Scenario: Default-mode test asserts no Status header
- **GIVEN** a test repo with one dirty worktree
- **WHEN** the test runs `wt list` (no `--status`)
- **THEN** the test SHALL assert `Status` header token is absent
- **AND** the test SHALL assert no `*` dirty marker appears in any data row

#### Scenario: `--status`-mode test asserts full enrichment
- **GIVEN** a test repo with one dirty worktree and one with 1 unpushed commit
- **WHEN** the test runs `wt list --status`
- **THEN** the test SHALL assert `Status` header appears
- **AND** the test SHALL assert the dirty worktree row contains `*`
- **AND** the test SHALL assert the unpushed worktree row contains `↑1`

## wt list: Spec doc reflects new contract

### Requirement: `docs/specs/cli-surface.md` SHALL document `--status` and the new default

The `wt list` section in `docs/specs/cli-surface.md` (lines 52-66) SHALL be updated to:

- Add a `--status` row to the flag table with default `false` and description matching the help text.
- Update the prose description of default human output to remove "dirty/unpushed status" from the default-column list.
- Update the prose description of `--json` to note that `dirty` and `unpushed` are present only when `--status` is set.

The exit-code list and the `--path`/`--json` mutual-exclusivity rule SHALL remain unchanged in prose but extended to cover `--path` ↔ `--status` exclusivity.

#### Scenario: Spec doc updated
- **GIVEN** the current `cli-surface.md`
- **WHEN** the spec update is applied
- **THEN** the `wt list` flag table SHALL list 3 rows: `--path`, `--json`, `--status`
- **AND** the default-output prose SHALL describe Name, Branch, Path (not Status)
- **AND** the `--status` row SHALL note it triggers per-worktree git checks

## Deprecated Requirements

### Default `wt list` output includes a Status column

**Reason**: `wt list` is invoked at discovery cadence and should be O(1) git invocations rather than O(N) with up to 5 forks per worktree. The status enrichment is the wrong default — it scales poorly with success, violates least-surprise for a `list` command, duplicates information shell prompts already display, and is stale by the time it prints. Removed per intake-stage decision and the project's pre-1.0 stability posture.

**Migration**: Users who want the prior dashboard view SHALL pass `--status`. JSON consumers that previously consumed `dirty`/`unpushed` keys MUST either add `--status` to their invocation or treat the absence of those keys as "status not computed". The CLI is pre-1.0 (per `build-and-release.md`), so no compatibility flag is provided.

### `checkDirty` 3-call sequence

**Reason**: The sequence `git diff --quiet` + `git diff --cached --quiet` + `git ls-files --others --exclude-standard` is fully replaced by a single `git status --porcelain` invocation. The replacement is behaviorally equivalent (captures staged, unstaged, and untracked changes in one shot) and ~3× cheaper per worktree.

**Migration**: N/A — internal helper, no external surface.

### `getUnpushedInDir` two-call sequence

**Reason**: The sequence `git rev-parse --abbrev-ref <branch>@{upstream}` followed by `git rev-list --count <upstream>..<branch>` is fully replaced by a single `git rev-list --count @{u}..HEAD` invocation. The `@{u}` shorthand resolves the upstream inline; absence of upstream causes the command to exit non-zero, which is treated as zero unpushed commits (matching current behavior).

**Migration**: N/A — internal helper, no external surface.

## Design Decisions

1. **Flag name is `--status`, not `-l`/`--long`/`--verbose`**
   - *Why*: Most semantically precise; matches the output it gates. `--status` reads naturally in scripts and shell aliases.
   - *Rejected*: `-l` would mirror `ls -l` but conflates "status" with "long format"; users reading `wt list -l` would not know whether it triggers git work. `--verbose` is too generic.

2. **JSON shape: omit `dirty`/`unpushed` keys (vs. default-zero or split struct)**
   - *Why*: Explicit about what wasn't computed. A consumer reading `dirty: false` MUST be able to trust that the check actually ran. Default-zero values would be misleading: a clean worktree and an unchecked worktree would be indistinguishable.
   - *Rejected*: (a) Always-zero defaults — invites silent bugs in downstream consumers that assume false-y means clean. (b) Split struct (`listEntry` vs `listEntryWithStatus`) — overkill for two optional fields; `omitempty` tags are simpler and idiomatic.

3. **No footer hint about `--status` in default output**
   - *Why*: Cleaner output; matches the `ls` convention (no hint about `-l`). Discoverability is covered by `--help` and the cobra `Long:` description. Footer noise on every invocation is worse than missed discoverability for a flag a user will encounter as soon as they need it.
   - *Rejected*: Always-on footer (noisy on every run); TTY-only footer (introduces a third output mode and another decision point).

4. **Pool size is hardcoded `min(NumCPU, 8)`, not configurable**
   - *Why*: Default fits the expected scale (≤100 worktrees). No real workload data justifies tuning. Adding a flag or env var now creates a surface that's hard to retract.
   - *Rejected*: `--concurrency N` flag (overkill); `WT_LIST_CONCURRENCY` env var (escape hatch without justification).

5. **Breaking change to default output, no compatibility flag**
   - *Why*: CLI is pre-1.0 per `build-and-release.md`. The constitution does not mandate output stability. A `--legacy` or `--v1` flag would be dead weight: users who want the old view get `--status`, which is also faster than today's default.
   - *Rejected*: `--legacy` opt-out, gated v2 behind a major version bump.

6. **Single worker-pool implementation, no fallback to serial**
   - *Why*: The worker pool with size 1 is functionally equivalent to serial execution. A dedicated serial path would only add a branch.
   - *Rejected*: Special-case serial path when `NumCPU() == 1`.


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
