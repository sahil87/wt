# wt-cli: List Status Contract

> Post-implementation behavior capture for the `wt list` status-opt-in cleanup.
> Source change: `260516-lfa8-list-status-opt-in`.

This file documents the contract that `wt list` honors after the status-opt-in change. Future changes touching `cmd/wt/list.go` should preserve these invariants unless an explicit spec amendment supersedes them.

## Requirements

### Default mode is enrichment-free

- Default `wt list` (no flags, or with `--json` only) SHALL invoke exactly **one** `git` subprocess: `git worktree list --porcelain`. It MUST NOT invoke `git diff`, `git diff --cached`, `git ls-files`, `git rev-parse`, `git rev-list`, or `git status` against any worktree directory.
- The cheap code path is `listEntriesBasic(ctx)`. It builds `[]listEntry` from `listWorktreeEntries()` + name/path/branch/is_main/is_current — nothing else.
- An unreadable worktree (broken symlink, permissions error on `.git`) MUST NOT surface an error in default mode, because no per-worktree work runs.

### Default human output: Name / Branch / Path

- `handleFormattedOutput(entries, ctx, showStatus=false)` renders three columns: `Name`, `Branch`, `Path`. The `Status` header is NOT emitted.
- Current-worktree green `*` marker and bold `(main)` rendering are preserved in both modes.
- The 3-column layout has its own width-computation block, distinct from the 4-column `--status` block. They share row preparation but not header/print logic.

### Default JSON: `dirty` and `unpushed` keys are absent

- `listEntry.Dirty` is `*bool`; `listEntry.Unpushed` is `*int`. Both have `omitempty` tags.
- Default mode (no `--status`) NEVER sets these pointers — they remain nil and the keys are omitted from JSON output.
- Consumers MUST treat the absence of `dirty`/`unpushed` keys as "status was not computed", NOT as "clean / not ahead". Defaulting absent keys to false/0 in consumer code is a contract violation on the consumer side.

### `--status` flag opts back into enrichment

- `--status` triggers `listEntriesEnriched(ctx)`, which:
  1. Builds base entries via `buildBaseEntry` (same as default).
  2. Sets `Dirty = &false` and `Unpushed = &0` for EVERY entry, before any goroutine spawns. This ensures JSON output emits the keys even for vanished worktrees where `os.Stat` fails.
  3. Spawns goroutines up to `min(runtime.NumCPU(), maxListConcurrency)` workers (cap is the named constant `maxListConcurrency = 8`).
  4. Each goroutine writes through the pre-allocated pointer: `*entries[i].Dirty = checkDirty(r.path)`, and `*entries[i].Unpushed = getUnpushedInDir(r.path)` for non-detached branches.
- Under `--status`, both JSON keys SHALL be present in every object regardless of value (`dirty: false, unpushed: 0` is valid; key omission is a regression).
- `--status` + `--json` is permitted. `--status` + `--path` exits `ExitInvalidArgs` with stderr containing "mutually exclusive".

### Worker pool: semaphore channel + WaitGroup

- The pool uses a buffered channel `sem := make(chan struct{}, concurrency)` plus `sync.WaitGroup`. No third-party pool library; no lock-free constructs.
- Output ordering is preserved by indexed slice writes (`entries[i].Dirty = ...`), NOT by appending. Parallelism MUST NOT reorder rows.
- Pool size is NOT configurable in this version — no flag, no env var. `maxListConcurrency = 8` is hardcoded.

### Dirty detection: single `git status --porcelain`

- `checkDirty(wtPath)` runs ONE `git status --porcelain` invocation. Dirty iff `strings.TrimSpace(out) != ""`.
- Non-zero exit (corrupted index, unreadable repo, etc.) is treated as **clean**. Stderr is discarded — failure modes are non-actionable for a list command.

### Unpushed detection: single `git rev-list --count @{u}..HEAD`

- `getUnpushedInDir(wtPath)` runs ONE `git rev-list --count @{u}..HEAD` invocation. No separate `git rev-parse --abbrev-ref @{u}` upstream lookup — `@{u}` resolves the upstream inline.
- Non-zero exit (no upstream configured, untracked branch, detached) returns 0. Stderr discarded.
- Detached HEAD is detected via `r.branch == "(detached)"` and skips the `getUnpushedInDir` call entirely; `Unpushed` stays at `0` from the pre-allocation.

### `--path` lookup mode skips enrichment

- `handlePathLookup` uses raw `listWorktreeEntries()`. It runs BEFORE the enrichment dispatch and returns early. No `checkDirty` or `getUnpushedInDir` calls regardless of other flags.
- `--path` + `--status` is rejected at flag-validation time, not after enrichment runs.

### Mutually exclusive flags

- `--path` ↔ `--json` — original constraint, unchanged.
- `--path` ↔ `--status` — new constraint. Both exits use `wt.ExitWithError(wt.ExitInvalidArgs, ...)` with stderr containing "mutually exclusive".

## Design Decisions

### Pointer fields over plain `bool`/`int` for JSON shape

`listEntry.Dirty` and `Unpushed` are `*bool` and `*int`, not plain values, specifically to distinguish "not computed" (nil → key omitted via `omitempty`) from "computed and clean / 0 unpushed" (non-nil zero → key present with value). A plain `bool` with `omitempty` would omit the key whenever the value is false, conflating clean and uncomputed.

A custom `MarshalJSON` was considered but rejected: pointer fields are idiomatic Go for "optional" semantics, and `encoding/json` handles them natively. A split struct (`listEntry` vs `listEntryWithStatus`) was also rejected — overkill for two optional fields, and downstream consumers would have to handle two shapes.

### Pre-allocate pointers BEFORE the stat check

In `listEntriesEnriched`, `Dirty` and `Unpushed` pointers are allocated to `&false`/`&0` *before* the `os.Stat(r.path)` check. If we allocated only inside the goroutine, a vanished worktree (stat fails → `continue`) would leave the pointers nil and JSON would drop the keys — violating the `--status` contract that says keys are present regardless of value.

The pre-allocation ensures a stable post-condition: under `--status`, every entry in the output has non-nil `Dirty` and `Unpushed`. Goroutines that DO run overwrite the pre-allocated zeros via `*entries[i].Dirty = ...`.

### Worker pool size: hardcoded, not configurable

`maxListConcurrency = 8` is a named constant — not a flag, not an env var. The expected scale is ≤100 worktrees per repo; CPU saturation isn't a real concern, and a "concurrency" knob invites premature configuration. Future changes can add a flag if measurements demand it; for now the surface is intentionally narrow.

### Breaking default, no compatibility flag

CLI is pre-1.0 (per `build-and-release.md`). The constitution does not require output stability. A `--legacy` or `--v1` flag would be dead weight — users who want the old view get `--status`, which is also faster than today's default thanks to the parallel collapsed-git-calls implementation.

### No footer hint about `--status`

Default output has NO footer like "Run `wt list --status` for dirty/unpushed". Discoverability is via `--help` and the cobra `Long:` description. Matches the `ls` convention (no hint about `-l`).

## Cross-references

- Spec doc: `docs/specs/cli-surface.md` — `wt list` section (lines ~52-73, flag table + prose).
- Source: `src/cmd/wt/list.go` — `listEntry`, `listCmd`, `listEntriesBasic`, `listEntriesEnriched`, `buildBaseEntry`, `checkDirty`, `getUnpushedInDir`, `handleFormattedOutput`, `maxListConcurrency`.
- Tests: `src/cmd/wt/list_test.go` — default-mode coverage (`TestList_DefaultHeader`, `TestList_DefaultModeNoDirtyIndicator`, `TestList_JSONDefaultFields`) and `--status`-mode coverage (`TestList_StatusHeader`, `TestList_StatusModeShowsDirty`, `TestList_StatusFlagShowsUnpushed`, `TestList_JSONStatusFields`, `TestList_StatusAndPathMutuallyExclusive`, `TestList_StatusOrderingPreserved`).
- Constitution: Principle II (Cobra command surface), III (Typed exit codes — `ExitInvalidArgs` for the new mutex check), IV (test coverage split per mode).
- Sibling memory: `wt-cli/init-failure-contract.md` — same pattern of post-change invariant capture for a different `wt` subcommand.

## Open follow-ups (not in scope for this change)

- `src/internal/worktree/git.go` still hosts the OLD slow patterns (`HasUncommittedChanges` + `HasUntrackedFiles`, and `HasUnpushedCommits` + `GetUnpushedCount` with a separate upstream lookup). These are consumed by `wt create` / `wt delete`, not `wt list`, so they were intentionally left untouched. A future change SHOULD unify them with the faster patterns from this change.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260516-lfa8-list-status-opt-in` | 2026-05-16 | Established default `wt list` as enrichment-free (Name/Branch/Path), introduced `--status` opt-in flag for the dashboard view, replaced 3-call `checkDirty` with single `git status --porcelain`, replaced 2-call `getUnpushedInDir` with single `git rev-list --count @{u}..HEAD`, parallelized `--status` enrichment with bounded worker pool, pointer-field JSON shape for present-vs-absent semantics. |
