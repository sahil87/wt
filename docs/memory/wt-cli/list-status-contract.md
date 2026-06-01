# wt-cli: List Status Contract

> Post-implementation behavior capture for the `wt list` status-opt-in cleanup.
> Source change: `260516-lfa8-list-status-opt-in`.

This file documents the contract that `wt list` honors after the status-opt-in change. Future changes touching `cmd/wt/list.go` should preserve these invariants unless an explicit spec amendment supersedes them.

## Requirements

### Default mode is enrichment-free

- Default `wt list` (no flags, or with `--json` only) SHALL invoke exactly **one** `git` subprocess: `git worktree list --porcelain`. It MUST NOT invoke `git diff`, `git diff --cached`, `git ls-files`, `git rev-parse`, `git rev-list`, or `git status` against any worktree directory.
- The cheap code path is `listEntriesBasic(ctx)`. It builds `[]listEntry` from `listWorktreeEntries()` + name/path/branch/is_main/is_current — nothing else.
- An unreadable worktree (broken symlink, permissions error on `.git`) MUST NOT surface an error in default mode, because no per-worktree work runs.

### Human output layout is keyed on the resolved sort mode

> **Amended by `260601-73cv-list-recency-column`** — the prior invariant "default human output is always Name / Branch / Path" is superseded for the recency-ordered view. The default human view (and explicit `--sort=recent`) now renders a 4th `Last Active` column. `name`/`branch` human modes stay 3-column; `--status` stays 5-column.

- `handleFormattedOutput(entries, ctx, showStatus, mode sortMode)` keys its layout on the resolved sort `mode`, **not on `showStatus` alone**. It derives `recentLayout := mode == sortRecent && !showStatus` and selects:
  - `--status` (`showStatus == true`, any mode) → **5-column** `Name / Branch / Status / Last Active / Path` (unchanged).
  - else recent human mode (`recentLayout == true`) → **4-column** `Name / Branch / Last Active / Path`. Because `recent` is the human default (per "Audience-split default ordering" below), bare `wt list` and `wt list --sort=recent` both land here.
  - else (`name`/`branch` human modes) → **3-column** `Name / Branch / Path` (unchanged). The `Status` and `Last Active` headers are NOT emitted.
- The `Last Active` cell in the 4-column branch is rendered via the existing `relativeTime(t time.Time)` helper (coarse buckets `just now`, `Nm ago`, `Nh ago`, `Nd ago`; a zero `time.Time` renders as `-`). The value is the persisted recency sort key (see "`last_active` opt-in pointer field" below), so it equals the key the row was sorted by — no second `os.Stat`, no `git` subprocess, no TOCTOU.
- Current-worktree green `*` marker and bold `(main)` rendering are preserved in all three layouts. The main worktree stays pinned to the first data row under recent mode and displays its own `Last Active`.
- Each of the three layouts has its own header array, `colWidths` computation, header `Printf`, and row `Printf`. They share the per-entry `displayRow` preparation loop (which populates `lastActive` from `relativeTime` when `showStatus || recentLayout`) but not the header/print logic. The 4-column branch sits between the 5-column `--status` branch and the 3-column `else` branch.

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
- `sortEntries(entries, mode, persistKey)` partitions the porcelain-first entry out
  when `entries[0].IsMain`, then reorders only `entries[1:]`. (The `persistKey` param
  was added by `260601-73cv`; it gates the recent-mode `LastActive` write-back and
  does not affect ordering or main pinning.)

### Sorting is a deterministic post-enrichment step

- Ordering is applied by `sortEntries` to the FINAL slice **after** enrichment
  writes complete (or to the basic slice in default mode). It is independent of
  the worker pool: the pool's indexed-write ordering invariant (parallelism must
  not reorder rows relative to the input slice) is about preserving porcelain
  order during enrichment; `sortEntries` is a separate, deterministic post-step.
  No conflict — the two passes compose.
- For `recent` mode, `sortEntries` computes a per-entry recency key into a local
  `keys[]` slice and uses it to order via `wt.RecencyLess`. The key prefers the
  already-computed `*LastActive` (set under `--status`) over a fresh `os.Stat`:
  this keeps the sort key consistent with the displayed value (no TOCTOU) and
  avoids O(N log N) redundant stats on the `--status` path. When `LastActive` is
  nil (default/basic mode), it falls back to `wt.RecencyOf(e.Path)`. Both keys feed
  the same `wt.RecencyLess` comparator. See `wt-cli/recency-ordering-contract.md`
  for the shared comparator definition.

### Recent mode persists the sort key into `LastActive` on the human path (`260601-73cv`)

- `sortEntries(entries, mode, persistKey bool)` takes a third `persistKey bool`
  parameter. The `listCmd` caller passes `!jsonOut` — `persistKey` is true on the
  human-output path and false on the `--json` path. The resolved sort mode is
  captured once into a local (`mode := resolveSort(...)`) and reused for both
  `sortEntries(entries, mode, !jsonOut)` and `handleFormattedOutput(..., mode)`.
- Previously the `keys[]` recency slice was computed and **discarded** after
  ordering. Now, in `recent` mode when `persistKey` is true, `sortEntries` writes
  each non-main entry's key back into `entries[i].LastActive` — but **only when the
  pointer is nil**. A non-nil `LastActive` (the `--status` enrichment value set by
  `listEntriesEnriched`) is the source of truth and is NEVER clobbered. The write
  reuses the stat already paid for the sort key: no second `os.Stat`, no `git`
  subprocess. The `keys[]` slice is indexed by pre-sort position, so the write-back
  reads through the sort permutation (`keys[idx]`).
- **Main-worktree single populate**: `sortEntries` partitions the main worktree out
  of the key-computation `rest` slice (`start = 1` when `entries[0].IsMain`), so
  basic recent mode never stats main and its `LastActive` would render `-`. When
  `persistKey` is true, `start == 1`, and `entries[0].LastActive == nil`, the main
  entry's `LastActive` is populated via a single `wt.RecencyOf(entries[0].Path)` —
  exactly one stat for main, no `git` subprocess, paralleling how
  `listEntriesEnriched` stats every entry. A non-nil main `LastActive` (the
  `--status` path) is left as-is. Main's pinned-first row position is unchanged;
  only its `LastActive` is populated for display.
- **Why gate on `persistKey`**: `sortEntries` runs (list.go:130) **before** the
  `jsonOut` branch (list.go:132-134), so an unconditional write-back would serialize
  the transiently-set `LastActive` into `--json --sort=recent` despite `omitempty`.
  Gating on `!jsonOut` keeps the JSON path observing the same nil `LastActive` it
  always did. See `wt-cli/recency-ordering-contract.md` for the note that the
  comparator/ordering definition itself is unchanged — only the discard was removed.

### `last_active` opt-in pointer field

- `listEntry` has `LastActive *time.Time` with the `json:"last_active,omitempty"`
  tag — the same opt-in pointer shape as `Dirty *bool` / `Unpushed *int`.
- **JSON output**: the pointer is set on the JSON path ONLY under `--status` (where
  `listEntriesEnriched` populates it). On the `--json` path, `sortEntries` is called
  with `persistKey == false` (caller passes `!jsonOut`), so even `--json
  --sort=recent` leaves the pointer nil and `omitempty` omits the key. Consumers
  MUST treat an absent `last_active` as "recency was not computed", not as a real
  timestamp. JSON output is **unchanged** by `260601-73cv` (see the dedicated
  amendment note below).
- **Human output**: the pointer is now ALSO populated transiently on the recent-mode
  human path (no `--status`) by `sortEntries` when `persistKey == true` — see "Recent
  mode persists the sort key into `LastActive`" above. This is the value the
  4-column `Last Active` cell displays. It never reaches JSON because the JSON path
  passes `persistKey == false`.
- Under `--status`, `listEntriesEnriched` pre-allocates `LastActive` to a non-nil
  pointer (zero `time.Time`) BEFORE the `os.Stat` stat-gate `continue`, mirroring
  `Dirty`/`Unpushed`. So every entry emits the key even for a vanished worktree,
  whose `LastActive` is the zero `time.Time`, serialized as
  `"0001-01-01T00:00:00Z"` (the established present-but-uncomputed signal,
  analogous to `dirty:false`/`unpushed:0`).
- `last_active` is computed by `os.Stat` only — NEVER a `git` call. Under `--status`
  it reuses the enrichment stat-gate's own `os.Stat` result
  (`*entries[i].LastActive = info.ModTime()`). On the basic recent-mode human path it
  reuses the `wt.RecencyOf(e.Path)` stat already paid for the sort key (plus a single
  `RecencyOf` stat for the pinned main entry). Either way it adds **no** new
  `os.Stat` and **no** new `git` subprocess to either mode. The single-subprocess
  default-mode contract (one `git worktree list --porcelain`) is preserved —
  `name`/`branch` human modes still perform zero per-worktree `os.Stat`.
- Human output renders a relative value via `relativeTime(t time.Time)` with coarse
  buckets (`just now`, `Nm ago`, `Nh ago`, `Nd ago`); a zero time renders as `-`.
  This rendering is **no longer exclusive to `--status`** (amended by `260601-73cv`):
  the per-entry `displayRow` loop populates `lastActive` whenever `showStatus ||
  recentLayout`. JSON emits the raw RFC3339 timestamp via the `*time.Time` field,
  never the relative string. The `--status` table renders 5 columns (Name / Branch /
  Status / Last Active / Path); the recent-mode human table renders 4 columns (Name /
  Branch / Last Active / Path).

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

### Persist the discarded recency key vs. re-stat in the render path (`260601-73cv`)

The recent-mode `Last Active` column displays the recency key `sortEntries` already
computes, persisted into `entries[i].LastActive` rather than re-stat'd in the render
path. Writing it back shows the value at zero additional cost and guarantees the
displayed value equals the sort key (no TOCTOU drift window). Gated on a `persistKey
bool` (caller passes `!jsonOut`): the human path persists and renders; the JSON path
passes `false`, leaving the pointer nil so `omitempty` keeps `last_active` out of
`--json --sort=recent`. Rejected: re-statting in `handleFormattedOutput` (reintroduces
a per-worktree `os.Stat`, creates a TOCTOU drift window, duplicates recency logic the
render layer should not own); an unconditional write-back in `sortEntries` (would
serialize the transiently-set `LastActive` into `--json --sort=recent`, breaking the
present-vs-uncomputed JSON contract).

### Thread the resolved sort mode into `handleFormattedOutput` (`260601-73cv`)

`handleFormattedOutput` previously keyed layout only on `showStatus`. To select the
4-column recent layout it now receives the resolved `sortMode` and derives
`recentLayout := mode == sortRecent && !showStatus`. The already-resolved mode (from
`resolveSort`, captured once in `listCmd`) is the minimal seam — no new state, no
recomputation. Rejected: recomputing `resolveSort` inside `handleFormattedOutput`
(duplicates resolution logic and its flag inputs); inferring recency from a non-nil
`LastActive` (ambiguous — `--status` also sets it, and that path must stay 5-column).

### Scope the `Last Active` column to recent mode only (`260601-73cv`)

`name`/`branch` human modes do zero per-worktree work today; adding a `Last Active`
column there would force a per-worktree `os.Stat` purely for display, violating the
cheap-default-path spirit of this contract. The user explicitly chose to treat the
default-layout change (Name/Branch/Path → Name/Branch/Last Active/Path under recent)
as a deliberate amendment to this contract, documented at hydrate, rather than
arguing it is not a violation. Rejected: showing the column in all human modes (adds
stats to cheap modes); an absolute timestamp column (wider, less scannable than the
relative buckets).

## Cross-references

- Spec doc: `docs/specs/cli-surface.md` — `wt list` section (lines ~52-73, flag table + prose).
- Source: `src/cmd/wt/list.go` — `listEntry` (now with `LastActive *time.Time`), `listCmd`, `listEntriesBasic`, `listEntriesEnriched`, `buildBaseEntry`, `checkDirty`, `getUnpushedInDir`, `maxListConcurrency`, plus the `260530-rtmf` additions: `sortMode`, `isValidSort`, `resolveSort`, `sortEntries`, `relativeTime`. `260601-73cv` changed `sortEntries(entries, mode, persistKey bool)` and `handleFormattedOutput(entries, ctx, showStatus, mode sortMode)`, and added the 4-column recent-layout rendering branch in `handleFormattedOutput`.
- Source: `src/internal/worktree/recency.go` — `RecencyOf`, `RecencyLess`, `SortByRecency` (the shared comparator consumed by `sortEntries`).
- Tests: `src/cmd/wt/list_test.go` — default-mode coverage (`TestList_DefaultHeader`, `TestList_DefaultModeNoDirtyIndicator`, `TestList_JSONDefaultFields`), `--status`-mode coverage (`TestList_StatusHeader`, `TestList_StatusModeShowsDirty`, `TestList_StatusFlagShowsUnpushed`, `TestList_JSONStatusFields`, `TestList_StatusAndPathMutuallyExclusive`, `TestList_StatusOrderingPreserved`), `260530-rtmf` ordering/last-active coverage (recency/name/invalid-sort/main-pinned/`--path`+`--sort` mutex, JSON-default-stable, `last_active` key presence/absence), and `260601-73cv` recent-column coverage (updated `TestList_HumanDefaultIsRecency` and `TestList_LastActiveRelativeTimeInHumanStatus`; new 4-column `Last Active` header + relative-time value in the default human view, `name`/`branch` modes stay 3-column, `--json`/`--json --sort=recent` omit `last_active`, main-worktree shows its own relative time, vanished worktree renders `-`, and a main-only repo renders the 4-column header without misalignment).
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
| `260601-73cv-list-recency-column` | 2026-06-01 | **Intentional amendment** to the default-human-output invariant: the recency-ordered human view (default human mode + explicit `--sort=recent`) now renders a 4-column `Name / Branch / Last Active / Path` table; `name`/`branch` human modes stay 3-column, `--status` stays 5-column. `relativeTime` rendering and `LastActive` population are no longer exclusive to `--status`. `sortEntries(entries, mode, persistKey bool)` gained a `persistKey` param (caller passes `!jsonOut`); recent mode now persists the previously-discarded recency key into each non-nil-only `entries[i].LastActive` (reusing the already-paid stat — no second `os.Stat`, no git subprocess) and populates the pinned main entry's nil `LastActive` via a single `wt.RecencyOf`. `handleFormattedOutput(entries, ctx, showStatus, mode sortMode)` gained the `mode` param and keys layout on `recentLayout := mode == sortRecent && !showStatus` (no longer on `showStatus` alone), with a new 4-column rendering branch. **JSON unchanged**: `persistKey == false` on the `--json` path leaves `LastActive` nil so `omitempty` keeps `last_active` out of `--json`/`--json --sort=recent`; machine modes keep their stable `name` default. |
