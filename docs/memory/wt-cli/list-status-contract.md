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
- `--path` ↔ `--sort` — added by `260530-rtmf`. `--path` is a single-worktree lookup for which ordering is meaningless; `wt list --path <name> --sort=<any>` exits `wt.ExitInvalidArgs` with stderr containing "--path and --sort are mutually exclusive". It MUST NOT silently ignore `--sort`. Same `ExitWithError` idiom as the two checks above.

### `--sort` flag and accepted values

- `wt list` accepts `--sort <recent|name|branch>` (a `StringVar`, default empty).
  `recent` orders non-main worktrees newest-first via the shared recency
  comparator; `name` orders by `Name` ascending; `branch` orders by `Branch`
  ascending.
- An unrecognized value exits `wt.ExitInvalidArgs` with a message naming the
  accepted values (`recent`, `name`, `branch`). Validation is `isValidSort(s)`,
  checked at flag-validation time before any git work.

### Audience-split default ordering (flag-based, no isatty)

- When `--sort` is NOT supplied, the default order is decided **purely by flags** —
  there is no runtime `isatty`/`term.IsTerminal` probe anywhere in `src/` and no
  terminal-detection dependency in `go.mod`. "Human output" means *neither*
  `--json` *nor* `--non-interactive` was supplied.
- Default = `recent` for human output; default = `name` (stable) whenever `--json`
  OR `--non-interactive` is set. An explicit `--sort` overrides the default in any
  mode (including `--json` and `--non-interactive`). Resolution lives in
  `resolveSort(sortFlag, jsonOut, nonInteractive) sortMode`.
- This preserves deterministic machine-readable output (Constitution VI) — fab-kit
  operators parsing `wt list`/`--json` get stable name order — while giving humans
  recency by default. It mirrors the opt-in, JSON-aware design of `--status`.
- `wt list` gained a `--non-interactive` `BoolVar` (previously absent on `list`;
  added following the `create.go`/`delete.go` flag pattern) whose *only* effect is
  selecting the stable default order. A piped `wt list | cat` without
  `--non-interactive` still gets recency — the accepted tradeoff of the
  no-new-dependency constraint.

### Main worktree pinned first under all sort modes

- Regardless of `--sort` value or default mode, the main worktree (`IsMain`)
  occupies the first output row. Only non-main worktrees are reordered. This
  matches the `git worktree list --porcelain` convention.
- `sortEntries(entries, mode)` partitions the porcelain-first entry out when
  `entries[0].IsMain`, then reorders only `entries[1:]`.

### Sorting is a deterministic post-enrichment step

- Ordering is applied by `sortEntries` to the FINAL slice **after** enrichment
  writes complete (or to the basic slice in default mode). It is independent of
  the worker pool: the pool's indexed-write ordering invariant (parallelism must
  not reorder rows relative to the input slice) is about preserving porcelain
  order during enrichment; `sortEntries` is a separate, deterministic post-step.
  No conflict — the two passes compose.
- For `recent` mode, `sortEntries` uses a recency-key closure that prefers the
  already-computed `*LastActive` (set under `--status`) over a fresh `os.Stat`:
  this keeps the sort key consistent with the displayed value (no TOCTOU) and
  avoids O(N log N) redundant stats on the `--status` path. When `LastActive` is
  nil (default/basic mode), it falls back to `wt.RecencyOf(e.Path)`. Both keys feed
  the same `wt.RecencyLess` comparator. See `wt-cli/recency-ordering-contract.md`
  for the shared comparator definition.

### `last_active` opt-in pointer field

- `listEntry` has `LastActive *time.Time` with the `json:"last_active,omitempty"`
  tag — the same opt-in pointer shape as `Dirty *bool` / `Unpushed *int`.
- Default mode (no `--status`) NEVER sets the pointer — it stays nil and the JSON
  key is omitted. Consumers MUST treat an absent `last_active` as "recency was not
  computed", not as a real timestamp.
- Under `--status`, `listEntriesEnriched` pre-allocates `LastActive` to a non-nil
  pointer (zero `time.Time`) BEFORE the `os.Stat` stat-gate `continue`, mirroring
  `Dirty`/`Unpushed`. So every entry emits the key even for a vanished worktree,
  whose `LastActive` is the zero `time.Time`, serialized as
  `"0001-01-01T00:00:00Z"` (the established present-but-uncomputed signal,
  analogous to `dirty:false`/`unpushed:0`).
- `last_active` is computed by `os.Stat` only — NEVER a `git` call — and only under
  `--status`. It reuses the enrichment stat-gate's own `os.Stat` result
  (`*entries[i].LastActive = info.ModTime()`), so it adds **no** new `os.Stat` and
  **no** new `git` subprocess to either mode. The single-subprocess default-mode
  contract (one `git worktree list --porcelain`) is preserved.
- Human output under `--status` renders a relative value via
  `relativeTime(t time.Time)` with coarse buckets (`just now`, `Nm ago`, `Nh ago`,
  `Nd ago`); a zero time renders as `-`. JSON emits the raw RFC3339 timestamp via
  the `*time.Time` field, never the relative string. The `--status` table gains a
  `Last Active` column (the human table is now 5 columns: Name / Branch / Status /
  Last Active / Path).

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

### Audience-split default ordering, flag-based not isatty (`260530-rtmf`)

Human output (neither `--json` nor `--non-interactive`) defaults to `recent`;
`--json`/`--non-interactive` default to stable `name`. The human-vs-machine
signal is the *absence of both flags*, NOT a real TTY probe — there is no
`isatty`/`term.IsTerminal` code or terminal-detection dependency in the repo, and
the change forbade adding one. A recency-shuffling default in machine modes would
make fab-kit operators' parsed output non-deterministic, so JSON/non-interactive
stay stable (Constitution VI). A recency default in *all* modes (breaks machine
parsers) and an opt-in-only default with no human recency (loses the ergonomic
win) were both rejected. `wt list` gained a `--non-interactive` BoolVar solely to
drive this — it has no other effect on `list`.

### `LastActive *time.Time` opt-in pointer (`260530-rtmf`)

Same opt-in pointer shape as `Dirty *bool` / `Unpushed *int`: nil → key omitted
("not computed"); non-nil → key present ("computed", including the zero time for a
vanished worktree). A plain `time.Time` with `omitempty` was rejected — the zero
time would be indistinguishable from uncomputed, the exact ambiguity the pointer
pattern exists to avoid. The field is computed inside the existing
`listEntriesEnriched` pass by reusing the stat-gate's `os.Stat` result, so it adds
no new `os.Stat` and no new `git` subprocess.

## Cross-references

- Spec doc: `docs/specs/cli-surface.md` — `wt list` section (lines ~52-73, flag table + prose).
- Source: `src/cmd/wt/list.go` — `listEntry` (now with `LastActive *time.Time`), `listCmd`, `listEntriesBasic`, `listEntriesEnriched`, `buildBaseEntry`, `checkDirty`, `getUnpushedInDir`, `handleFormattedOutput`, `maxListConcurrency`, plus the `260530-rtmf` additions: `sortMode`, `isValidSort`, `resolveSort`, `sortEntries`, `relativeTime`.
- Source: `src/internal/worktree/recency.go` — `RecencyOf`, `RecencyLess`, `SortByRecency` (the shared comparator consumed by `sortEntries`).
- Tests: `src/cmd/wt/list_test.go` — default-mode coverage (`TestList_DefaultHeader`, `TestList_DefaultModeNoDirtyIndicator`, `TestList_JSONDefaultFields`), `--status`-mode coverage (`TestList_StatusHeader`, `TestList_StatusModeShowsDirty`, `TestList_StatusFlagShowsUnpushed`, `TestList_JSONStatusFields`, `TestList_StatusAndPathMutuallyExclusive`, `TestList_StatusOrderingPreserved`), and `260530-rtmf` ordering/last-active coverage (recency/name/invalid-sort/main-pinned/`--path`+`--sort` mutex, JSON-default-stable, `last_active` key presence/absence).
- Constitution: Principle II (Cobra command surface), III (Typed exit codes — `ExitInvalidArgs` for the new mutex check), IV (test coverage split per mode).
- Sibling memory: `wt-cli/init-failure-contract.md` — same pattern of post-change invariant capture for a different `wt` subcommand.
- Sibling memory: `wt-cli/recency-ordering-contract.md` — the shared `RecencyOf`/`RecencyLess`/`SortByRecency` definition that this file's `--sort`/`recent` ordering and `last_active` field consume; also covers the `wt open`/`wt delete` menu ordering.

## Open follow-ups (not in scope for this change)

- `src/internal/worktree/git.go` still hosts the OLD slow patterns (`HasUncommittedChanges` + `HasUntrackedFiles`, and `HasUnpushedCommits` + `GetUnpushedCount` with a separate upstream lookup). These are consumed by `wt create` / `wt delete`, not `wt list`, so they were intentionally left untouched. A future change SHOULD unify them with the faster patterns from this change. (Note: `260530-rtmf` consolidated only the *inline mtime/recency loops* in `open.go`/`delete.go` into `wt.SortByRecency` — these slow dirty/unpushed `git.go` patterns are a separate concern and remain unchanged.)
- The worktree-directory `mtime` recency signal is noisy (moves on any file write). The follow-up `260530-5fyu-stale-worktree-hints` will revisit signal quality for staleness detection; see `wt-cli/recency-ordering-contract.md`.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260516-lfa8-list-status-opt-in` | 2026-05-16 | Established default `wt list` as enrichment-free (Name/Branch/Path), introduced `--status` opt-in flag for the dashboard view, replaced 3-call `checkDirty` with single `git status --porcelain`, replaced 2-call `getUnpushedInDir` with single `git rev-list --count @{u}..HEAD`, parallelized `--status` enrichment with bounded worker pool, pointer-field JSON shape for present-vs-absent semantics. |
| `260530-rtmf-recency-aware-listing` | 2026-05-31 | Added `--sort <recent\|name\|branch>` and a `--non-interactive` BoolVar to `wt list`; audience-split default ordering (human→recent, `--json`/`--non-interactive`→stable name) decided purely by flags (no isatty); `--path`↔`--sort` mutex (`ExitInvalidArgs`, "mutually exclusive"); main worktree pinned first under all sort modes; sorting is a deterministic post-enrichment step that does not disturb the worker-pool indexed-write ordering. Added the `LastActive *time.Time` opt-in pointer (`last_active,omitempty`): nil/key-omitted in default mode, non-nil under `--status` (zero time for vanished worktrees → `"0001-01-01T00:00:00Z"`), computed from the enrichment stat-gate's own `os.Stat` (no new git subprocess, no new stat); human `--status` output gains a relative `Last Active` column via `relativeTime`. |
