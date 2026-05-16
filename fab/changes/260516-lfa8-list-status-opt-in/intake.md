# Intake: wt list — status opt-in, fast by default

**Change**: 260516-lfa8-list-status-opt-in
**Created**: 2026-05-16
**Status**: Draft

## Origin

Originated from a `/fab-discuss` session investigating slow `wt list` execution in a repo with 25 worktrees (`/Users/sahil/code/wvrdz/loom`). Measured wall time: **5.49s** (10s CPU). Root cause: `getEnrichedEntries` in [src/cmd/wt/list.go:266-312](src/cmd/wt/list.go#L266-L312) loops serially over every worktree, spawning up to 5 `git` subprocesses each (3 for `checkDirty`, 2 for `getUnpushedInDir`) — ~125 forks for the loom repo, at ~30–50ms each on macOS.

The user then raised the design question: *does `wt list` need to know dirty/unpushed status for each folder at all?* Discussion concluded **no** — the status enrichment is the wrong default. The user asked to draft this as a fab change.

> "Bigger qn - does wt list need to know about 'checkDirty' and 'getUnpushedInDir' status for each folder?"

Decisions reached during discussion (encoded in Assumptions below):

1. **Drop Status column from default output.** Default `wt list` shows Name, Branch, Path only.
2. **Add `--status` flag.** Opt-in to dirty/unpushed enrichment. When present, run checks in parallel and replace the 3-call dirty check with a single `git status --porcelain`.
3. **Backward-incompatible default change is acceptable.** CLI is pre-1.0; ship the new default, do not gate behind a v2 or `--legacy` flag.
4. **`--path` lookup mode is unaffected.** Already skips enrichment.

## Why

**The problem.** `wt list` is the primary discovery command for worktrees. It is invoked by:
- Humans browsing what exists (interactive)
- Operators piping output to other tools (scripts, `hop`, fab-kit)
- `wt list --path <name>` for path lookup (already cheap)
- `wt list --json` consumers (scripts that want structured data)

The current implementation conflates *discovery* with a *status dashboard*. For each worktree it runs up to 5 git subprocesses, all serial. The cost scales with worktree count: a 25-worktree repo takes 5.5s; a hypothetical 50-worktree repo would take ~11s.

**Why this default is wrong:**

1. **Scales poorly with success.** The more worktrees you create, the slower `list` gets — exactly the wrong gradient for a tool meant to encourage cheap, frequent worktree usage.
2. **Violates least-surprise for a `list` command.** Peers (`ls`, `git branch`, `git worktree list`) are all instant. Users do not expect `wt list` to fork 125 subprocesses.
3. **Redundant.** Most shell prompts already show dirty/ahead markers when the user is *in* a worktree. The status column is duplicate information for the 95% case.
4. **Stale.** By the time the table prints (5s later), half the statuses may already be wrong.
5. **Unneeded by most consumers.** `wt list --path` and most `--json` consumers (operators, scripts) want path/branch/name — not dirtiness.

**Why opt-in is the right shape:**

- Discovery becomes O(1) git invocations (just `git worktree list --porcelain`) — should be ~50ms regardless of worktree count.
- When the user actually wants the dashboard view, `wt list --status` provides it — and we have headroom to make even *that* path fast (parallelism + single `git status --porcelain`).
- Matches the spirit of `ls` vs `ls -l`: terse by default, rich on request.

**Why a flag-gated cleanup, not just "parallelize the existing behavior":**

Parallelism alone would reduce wall time (~8× with a bounded pool) but does not address the conceptual issues: scaling-with-success, staleness, redundancy, surprise. The cost of the dashboard view is the *wrong cost to pay by default*, regardless of speed.

## What Changes

### 1. Default `wt list` output drops Status column

**Before:**

```
  Name                                               Branch                                             Status  Path
* (main)                                             main                                                       loom/
  260427-eozq-runner-migration                       260427-eozq-runner-migration                               loom.worktrees/260427-eozq-runner-migration/
  calm-grove                                         260427-c2az-drop-shard-allowlist                   *       loom.worktrees/calm-grove/
  surging-shrew                                      surging-shrew                                      ↑1      loom.worktrees/surging-shrew/
```

**After (default):**

```
  Name                                               Branch                                             Path
* (main)                                             main                                                       loom/
  260427-eozq-runner-migration                       260427-eozq-runner-migration                               loom.worktrees/260427-eozq-runner-migration/
  calm-grove                                         260427-c2az-drop-shard-allowlist                           loom.worktrees/calm-grove/
  surging-shrew                                      surging-shrew                                              loom.worktrees/surging-shrew/
```

No per-worktree git invocations. `getEnrichedEntries` is reduced to: read `git worktree list --porcelain`, compute `IsMain`/`IsCurrent`/`Name`/`Path`/`Branch`. Performance target: **≤100ms wall time** for a 25-worktree repo (vs 5.5s today).

### 2. New `--status` flag

```
wt list --status
```

Re-enables dirty/unpushed enrichment. Output matches the current (pre-change) table format, including the Status column.

**Implementation notes for spec:**

- Run per-worktree enrichment in a bounded worker pool. Suggested concurrency: `min(runtime.NumCPU(), 8)`.
- Replace the 3-call `checkDirty` with a single `git status --porcelain`:
  ```go
  cmd := exec.Command("git", "status", "--porcelain")
  cmd.Dir = wtPath
  out, _ := cmd.Output()
  dirty := strings.TrimSpace(string(out)) != ""
  ```
  Captures staged, unstaged, and untracked in one shot.
- `getUnpushedInDir` can also be collapsed: `git rev-list --count @{u}..HEAD 2>/dev/null` returns 0 (or errors out) if no upstream; no separate `rev-parse --abbrev-ref` needed. Drop on error.
- Per-worktree cost when `--status` is on: 2 subprocesses instead of 5. With 8-way parallelism on 25 worktrees: ~50 forks total, ~16-deep critical path → expected wall time ~500ms-1s.

### 3. `--json` output changes

When `--status` is **not** passed, omit `dirty` and `unpushed` from each JSON object. Consumers can detect their absence and know status was not computed.

```json
// default (no --status)
[
  {"name": "main", "branch": "main", "path": "/abs/path/to/loom", "is_main": true, "is_current": true},
  {"name": "calm-grove", "branch": "260427-c2az-drop-shard-allowlist", "path": "...", "is_main": false, "is_current": false}
]

// with --status
[
  {"name": "main", "branch": "main", "path": "...", "is_main": true, "is_current": true, "dirty": false, "unpushed": 0},
  {"name": "calm-grove", "branch": "260427-c2az-drop-shard-allowlist", "path": "...", "is_main": false, "is_current": false, "dirty": true, "unpushed": 0}
]
```

Alternative (decide during spec): always include `dirty: false, unpushed: 0` as defaults when `--status` is absent. Tradeoff is between "easier to consume" (always-present fields) and "explicit about what wasn't computed" (omitted fields). **Currently Unresolved — see Assumptions.**

The `listEntry` struct in [src/cmd/wt/list.go:22-30](src/cmd/wt/list.go#L22-L30) needs `omitempty` tags on `Dirty` and `Unpushed`, or a different shape entirely (e.g., a separate `*listEntryWithStatus`).

### 4. `--path` lookup mode

Unchanged. `handlePathLookup` already uses `listWorktreeEntries` (raw, no enrichment). Verify in spec.

### 5. Help text and `--help` updates

```
wt list [flags]

Flags:
      --status         Show dirty/unpushed status for each worktree (slower)
      --path string    Output just the absolute path for a named worktree
      --json           Output worktree data as a JSON array
```

The `Long:` description should mention that `--status` enables the per-worktree git checks and is the slower mode.

### 6. Tests

[src/cmd/wt/list_test.go](src/cmd/wt/list_test.go) currently asserts on the Status column presence and on dirty/unpushed values. Tests will be split into two groups:

- **Default-mode tests**: assert Status column is absent; assert no per-worktree git invocations occur (could be verified by a slow-fail probe or by leaving a worktree's `.git` in an unreadable state and confirming no error surfaces in default mode).
- **`--status` mode tests**: assert Status column is present, dirty/unpushed values are correct, parallel execution does not corrupt output ordering.

Integration tests in [src/cmd/wt/integration_test.go](src/cmd/wt/integration_test.go) need a similar split. Existing JSON-output tests need to be updated for the new schema.

## Affected Memory

- `wt-cli/list-status-contract`: (new) Document the post-change contract: `wt list` default is enrichment-free, `--status` is opt-in, `--path` skips enrichment, `--json` omits status fields when `--status` is not set. Future changes to `wt list` should preserve this contract unless an explicit spec amendment supersedes it.

Also flags an update (not a memory file, but a spec): [docs/specs/cli-surface.md](docs/specs/cli-surface.md) lines 52-66 currently document `wt list` with the Status column as default; the spec must be updated to reflect the new flag and default output. This is a spec update, not a memory update — handled as part of the change, not in hydrate.

## Impact

**Code changes:**

- [src/cmd/wt/list.go](src/cmd/wt/list.go)
  - Add `--status` flag to `listCmd()` flag set
  - Split `getEnrichedEntries` into two paths: a cheap default (`listEntries`) and an enriched path (`getEnrichedEntries` retained for `--status`)
  - Parallelize the enriched path with a bounded worker pool (new helper, e.g., `enrichEntriesParallel`)
  - Replace `checkDirty`'s 3 git calls with a single `git status --porcelain`
  - Collapse `getUnpushedInDir` to a single `git rev-list --count @{u}..HEAD`
  - Update `listEntry` JSON tags (likely `omitempty` for `dirty`, `unpushed`) — or restructure
  - Update `handleFormattedOutput` to omit Status column when status was not computed
  - Update `handleJSONOutput` similarly (depends on Unresolved decision above)

- [src/cmd/wt/list_test.go](src/cmd/wt/list_test.go) — split test cases per output mode
- [src/cmd/wt/integration_test.go](src/cmd/wt/integration_test.go) — split per output mode; add `--status` integration coverage
- [docs/specs/cli-surface.md](docs/specs/cli-surface.md) — update `wt list` section (flags table, default output description, exit codes — unchanged but flag rows need additions)

**External callers / consumers to consider:**

- `hop` (delegates to `wt`, uses `wt open` not `wt list` per [launcher-contract.md](docs/specs/launcher-contract.md)) — unaffected
- fab-kit operators — verify if any consume `wt list --json` for `dirty`/`unpushed`. If yes, they need to add `--status` to their invocations.
- Shell aliases in `wt shell-init` output — none consume list output today
- User shell aliases — out of scope (their problem if they break)

**Backward compatibility:**

- This is a breaking change to default human output (column dropped) and JSON output (fields omitted by default).
- CLI is pre-1.0 per [build-and-release.md](docs/specs/build-and-release.md) — no constitution-mandated stability contract on flags/output.
- Mitigations: clear release notes; `--status` provides one-flag opt-in to prior behavior.

**Performance targets (for spec):**

| Mode | 25-worktree repo (current → target) |
|------|--------------------------------------|
| default | 5.5s → ≤100ms |
| `--status` | 5.5s → ≤1s (with parallelism + collapsed git calls) |
| `--path` | unchanged (already cheap) |

## Open Questions

- `--json` output: omit `dirty`/`unpushed` fields when `--status` is absent, or include them with default zero values? Tradeoffs: clarity (omit) vs. ease of consumption (always-present). User mentioned deciding during spec.
- Flag naming: `--status`, `-l`, `--long`, or `--verbose`? `--status` is the most semantically precise. `-l` mirrors `ls -l` but conflates "status" with "long format." Spec should pick one.
- Worker pool size for parallel enrichment: `min(NumCPU, 8)` is a reasonable default. Should this be configurable via flag (e.g., `--concurrency N`) or env var? Likely no — overkill for a list command.
- Should default output include a hint when worktrees are dirty? (e.g., a footer: "Run `wt list --status` for dirty/unpushed indicators.") Or just leave it silent? Spec should decide.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Drop the Status column from default `wt list` output. | Discussed and confirmed by user — the core design call of the change. | S:95 R:60 A:90 D:90 |
| 2 | Certain | Add `--status` flag to opt back into dirty/unpushed enrichment. | Discussed and confirmed — the mechanism for retaining the dashboard view on demand. | S:95 R:70 A:90 D:90 |
| 3 | Certain | Ship as a breaking change to default output without a compatibility flag or v2 gate. | Discussed — CLI is pre-1.0, user explicitly OK'd this. | S:90 R:50 A:85 D:85 |
| 4 | Confident | When `--status` is set, parallelize enrichment with a bounded worker pool (default ~8). | Standard concurrency pattern; consistent with Go idioms. Reversible — pool size is internal. | S:75 R:85 A:85 D:75 |
| 5 | Confident | Replace 3-call `checkDirty` with single `git status --porcelain` invocation. | One call captures staged/unstaged/untracked; well-known git idiom. Behaviorally equivalent. | S:85 R:90 A:90 D:90 |
| 6 | Confident | Collapse `getUnpushedInDir` to single `git rev-list --count @{u}..HEAD`; drop upstream lookup. | `@{u}` resolves upstream inline; error → 0 unpushed (no upstream configured). Cheaper and equivalent. | S:80 R:90 A:85 D:85 |
| 7 | Confident | Flag name is `--status` (not `-l`/`--long`/`--verbose`). | Most semantically precise; matches the output it gates. | S:70 R:90 A:80 D:80 |
| 8 | Confident | `--path` lookup mode remains unchanged. | Already uses raw `listWorktreeEntries`, no enrichment. Verified by code read. | S:90 R:95 A:95 D:95 |
| 9 | Tentative | `--json` output omits `dirty`/`unpushed` fields when `--status` is absent (vs. defaulting to false/0). | Both options valid; omission is more honest about what wasn't computed, but breaks downstream consumers that expect always-present fields. Lean toward omission but flag for spec discussion. | S:55 R:65 A:60 D:50 |
| 10 | Tentative | Default output includes no footer hint about `--status` availability. | Cleaner output; help text covers discoverability. Could revisit if users miss the feature. | S:50 R:90 A:70 D:60 |
| 11 | Tentative | Worker pool size is `min(runtime.NumCPU(), 8)`, not configurable. | Defaults are fine for the expected scale (≤100 worktrees). Adding a flag would be premature. | S:55 R:85 A:75 D:70 |
| 12 | Confident | Add new memory file `wt-cli/list-status-contract.md` during hydrate to document the post-change contract. | Long-term invariant worth capturing; matches the pattern of `wt-cli/init-failure-contract.md`. | S:75 R:95 A:90 D:85 |
| 13 | Confident | Performance target: default mode ≤100ms, `--status` mode ≤1s, both measured on a 25-worktree repo. | Conservative target based on current bottleneck analysis (5 git calls × 25 worktrees serial); a single `git worktree list --porcelain` plus minimal processing is well under 100ms on a warm cache. | S:75 R:80 A:80 D:75 |

13 assumptions (3 certain, 7 confident, 3 tentative, 0 unresolved).
