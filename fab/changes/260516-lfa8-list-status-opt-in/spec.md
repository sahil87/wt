# Spec: wt list — status opt-in, fast by default

**Change**: 260516-lfa8-list-status-opt-in
**Created**: 2026-05-16
**Affected memory**: `docs/memory/wt-cli/list-status-contract.md`

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Drop the Status column from default `wt list` output. | Confirmed from intake #1 — core design decision, ratified during clarification. | S:95 R:60 A:90 D:90 |
| 2 | Certain | Add `--status` flag to opt back into dirty/unpushed enrichment. | Confirmed from intake #2 — mechanism for retaining the dashboard view. | S:95 R:70 A:90 D:90 |
| 3 | Certain | Ship as a breaking change to default output without a compatibility flag. | Confirmed from intake #3 — pre-1.0 CLI; constitution does not require output stability. | S:95 R:50 A:90 D:90 |
| 4 | Certain | When `--status` is set, parallelize enrichment via bounded worker pool sized `min(NumCPU, 8)`. | Confirmed from intake #4 + clarification — standard Go idiom; pool size internal and reversible. | S:95 R:85 A:90 D:80 |
| 5 | Certain | Replace 3-call `checkDirty` with a single `git status --porcelain` invocation. | Confirmed from intake #5 + clarification — behaviorally equivalent, ~3× cheaper per worktree. | S:95 R:90 A:95 D:95 |
| 6 | Certain | Collapse `getUnpushedInDir` to a single `git rev-list --count @{u}..HEAD`. | Confirmed from intake #6 + clarification — `@{u}` resolves upstream inline; error → 0 (matches current). | S:95 R:90 A:90 D:90 |
| 7 | Certain | Flag name is `--status`. | Confirmed from intake #7 + clarification — most semantically precise. | S:95 R:90 A:90 D:90 |
| 8 | Certain | `--path` lookup mode remains unchanged. | Confirmed from intake #8 + clarification — already uses raw `listWorktreeEntries`. | S:95 R:95 A:95 D:95 |
| 9 | Certain | `--json` omits `dirty`/`unpushed` keys when `--status` absent (via `omitempty`). | Upgraded from intake Tentative — user explicitly confirmed during clarification. | S:95 R:65 A:80 D:85 |
| 10 | Certain | Default output includes no footer hint about `--status`. | Upgraded from intake Tentative — user confirmed silent default. | S:95 R:90 A:80 D:85 |
| 11 | Certain | Worker pool size is hardcoded `min(NumCPU, 8)`; no flag, no env var. | Upgraded from intake Tentative — user confirmed hardcoded. | S:95 R:85 A:85 D:80 |
| 12 | Certain | Add `wt-cli/list-status-contract.md` memory file during hydrate. | Confirmed from intake #12 — long-term invariant matching `init-failure-contract.md` pattern. | S:95 R:95 A:90 D:90 |
| 13 | Certain | Performance targets: default ≤100ms, `--status` ≤1s, on a 25-worktree repo (warm cache). | Confirmed from intake #13 — single porcelain call plus parallel enrichment with collapsed git calls. | S:95 R:80 A:85 D:80 |
| 14 | Certain | `--path` combined with `--status` exits `ExitInvalidArgs`. | New: spec-level decision. Both flags trigger mutually incompatible code paths; current `--path`/`--json` exclusivity sets the precedent for clear up-front rejection. | S:90 R:85 A:90 D:90 |
| 15 | Certain | `--status` combined with `--json` is permitted; JSON keys present regardless of value. | New: spec-level decision. `--status` selects the data model; `--json` selects the encoding — orthogonal. JSON consumers benefit from always-present keys when status WAS computed. | S:90 R:90 A:90 D:95 |
| 16 | Certain | On `git status` / `git rev-list` non-zero exit, treat the worktree as clean / 0-unpushed and suppress stderr. | New: spec-level decision. Matches existing graceful-degradation semantics of `checkDirty` / `getUnpushedInDir`; failure modes are non-actionable for a list command. | S:85 R:90 A:90 D:90 |
| 17 | Certain | Existing integration tests asserting `assertContains(t, r.Stdout, "<name>")` continue to pass under the new default output (name column unchanged). | New: spec-level decision. Verified by code read — `Name` column remains the first data column. | S:95 R:95 A:95 D:95 |
| 18 | Confident | The worker-pool implementation uses a buffered semaphore channel plus `sync.WaitGroup` (standard Go pattern), not a third-party pool library. | Plan-level call but worth noting: dependency-free, ≤30 lines, idiomatic. | S:80 R:90 A:90 D:85 |

18 assumptions (17 certain, 1 confident, 0 tentative, 0 unresolved).
