---
type: memory
description: "The shared idle predicate, the `wt delete --stale` selector, and the safety invariant that idleness never gates a deletion on its own."
---
# wt-cli: Idle / Staleness Contract

> Post-implementation behavior capture for the shared idle predicate, the
> `wt delete --stale` selector, and the load-bearing safety invariant that
> idleness never gates a deletion on its own.
> Source change: `260530-5fyu-stale-worktree-hints`.

This file documents the single definition of "idle" that `wt list` and
`wt delete` honor after the stale-worktree-hints change, plus the `--stale`
flag surface and the safety invariant that backstops it. It is a distinct
cross-command contract — a predicate + a flag + a safety invariant spanning
both `wt list` (display) and `wt delete` (selection). Future changes touching
`src/internal/worktree/idle.go`, the `wt list` idle marker, or the
`wt delete --stale`/`handleDeleteMenu` paths should preserve these invariants
unless an explicit spec amendment supersedes them.

The idle predicate is built on the single `RecencyOf` recency signal — it does
NOT introduce a second staleness signal (see "Signal = dir-mtime" below and
`wt-cli/recency-ordering-contract.md`).

## Requirements

### The idle predicate: `IsIdle(recency, now, threshold)`

- The package exposes exactly one idle predicate, `wt.IsIdle(recency time.Time,
  now time.Time, threshold time.Duration) bool`, in
  `src/internal/worktree/idle.go` (Constitution V — non-trivial logic under
  `internal/worktree`, `cmd/` only consumes). It returns `now.Sub(recency) >
  threshold` (R1).
- **Value parameter, never a path.** `IsIdle` takes the recency time as a
  `time.Time` value, not a path — so callers reuse a recency value they have
  already computed (`listEntry.LastActive` on the list path; a `RecencyOf`
  result on the delete path). The predicate itself never stats (R1). This
  mirrors how `RecencyLess` is keyed on values rather than typed on a struct
  (`wt-cli/recency-ordering-contract.md`). `IsIdle(path string, ...)` was
  rejected — it would re-stat paths the caller already holds.
- **Strict boundary.** The comparison is strict `>`: a worktree whose age is
  *exactly* the threshold is NOT idle; only `age > threshold` is idle (R4).
- **Zero recency is idle.** A zero `recency` (`time.Time{}`, what `RecencyOf`
  returns for a vanished or unstattable worktree) is treated as idle against any
  positive threshold. `now.Sub(zeroTime)` is an enormous positive duration, so
  this falls out of the `>` comparison naturally — it is not special-cased, but
  it is a deliberate, tested invariant (R2). An unstattable worktree is, if
  anything, a stronger cleanup candidate, never a fresh one.

#### Scenarios captured by `idle_test.go`

```
8 days old, 7d threshold  → idle (8d > 7d)
6 days old, 7d threshold  → not idle (6d < 7d)
exactly 7d old, 7d        → not idle (strict >, not >=)
zero recency, 7d          → idle (vanished worktree is a candidate)
```

### Default threshold: a named constant, not a knob

- The default idle threshold is **7 days**, defined as the exported named
  constant `DefaultIdleThreshold = 7 * 24 * time.Hour` in
  `src/internal/worktree/idle.go` (R3). It mirrors the `maxListConcurrency`
  "named constant, not a knob" precedent (`wt-cli/list-status-contract.md`).
- It SHALL NOT be configurable via environment variable or config file
  (Constitution I — no hidden state). The ONLY per-invocation override is the
  `wt delete --stale=Nd` flag value. `wt list` has no override at all — it always
  evaluates against `DefaultIdleThreshold`.

### Threshold parsing: `ParseIdleThreshold` — `Nd`-only

- `wt.ParseIdleThreshold(s string) (time.Duration, error)` in
  `src/internal/worktree/idle.go` parses a day-suffixed integer threshold (R15):
  - Empty string (bare `--stale` via pflag `NoOptDefVal`) → `DefaultIdleThreshold`.
    `ParseIdleThreshold("")` resolving to the default is a robustness belt — the
    `NoOptDefVal` is actually `"7d"` (pflag forbids an empty `NoOptDefVal`), so
    in practice the bare flag arrives as `"7d"`, never `""`.
  - `Nd` form only — e.g. `7d`, `30d` — returns `days * 24 * time.Hour`.
  - A value with no `d` suffix, a non-integer day count (`banana`, `30`,
    `30h`, `2w`), or a non-positive integer (`0d`, `-5d`) is rejected with an
    error whose message names the accepted form (`Nd`, e.g. `7d` or `30d`).
- Only the `d` (day) suffix is supported — hours and weeks are deliberately out
  of scope (non-day units are a Non-Goal in the spec). The `Nd` form matches the
  `Nd ago` display buckets users already see in `wt list` (`relativeTime`).

### `wt delete --stale[=Nd]`: the non-interactive selector

- `wt delete` accepts a `--stale` flag registered as a single pflag with
  `NoOptDefVal = "7d"` (R14): bare `--stale` carries the 7d default;
  `--stale=Nd` overrides the threshold. The flag is a `StringVar` whose presence
  is detected via `cmd.Flags().Changed("stale")` — a nil/empty check on the
  value cannot distinguish "absent" from "bare" because the bare form sets the
  `NoOptDefVal`.
- **The `=` is REQUIRED for the value.** `--stale 30d` (space-separated) is NOT
  supported because `wt delete` takes positional worktree names
  (`cobra.ArbitraryArgs`), so `30d` would be parsed as a worktree name. This is
  an accepted, documented quirk of folding the value into one flag.
- `handleDeleteStale` (`src/cmd/wt/delete.go`) resolves the threshold via
  `ParseIdleThreshold`, then selects every non-main worktree that is idle per
  `IsIdle(RecencyOf(path), now, threshold)` and routes the selection through the
  existing `handleDeleteMultiple` flow (R17). An invalid threshold exits
  `ExitInvalidArgs` naming the `Nd` form (R15).
- **Empty match is exit-0, not an error.** When `--stale` matches zero
  worktrees, the command prints `No idle worktrees (threshold: Nd).` and exits
  `ExitSuccess` (R17). The `Nd` rendering comes from the local `formatThreshold`
  helper (`fmt.Sprintf("%dd", int(d.Hours())/24)`), which round-trips the
  resolved duration back to the `--stale=Nd` input form.

#### Mutex constraints

- **`--stale` ↔ positional names** → `ExitInvalidArgs` with stderr containing
  "mutually exclusive" (R16). This converts the silent `--stale 30d` parse trap
  (where `30d` becomes a positional) into a loud, recoverable error, matching the
  `--path`↔`--status` idiom in `wt list`.
- **`--stale` ↔ `--delete-all`** → `ExitInvalidArgs` with "mutually exclusive"
  (R19). `--delete-all` already targets every worktree; `--stale` is a narrowing
  selector, so combining them is contradictory.
- Both mutex checks run in `RunE` **before** any git work or menu-session setup,
  so a mis-typed invocation fails fast. They are gated on `staleRequested :=
  cmd.Flags().Changed("stale")`.

#### Composition with existing flags

- `--stale` composes with `--non-interactive`, `--delete-branch`,
  `--delete-remote`, and `--stash` exactly as positional/`--delete-all` deletion
  does today, because it reuses `handleDeleteMultiple` (R18). In
  `--non-interactive` mode the existing per-worktree safety semantics
  (stash-or-discard default, unpushed handling) are unchanged.

### `wt delete` interactive menu: stale-aware annotation + "All idle (N)"

> See `wt-cli/recency-ordering-contract.md` for the `defaultIdx` 2/3 shift and
> the `, idle` row annotation, which amend that file's `defaultIdx = 2` line.
> Summary here for the cross-command view:

- `handleDeleteMenu` annotates each idle non-main option label with a trailing
  `, idle` (e.g. `feature-x (feat-x), idle`) and, when at least one worktree is
  idle, inserts an **"All idle (N)"** entry immediately after the existing
  "All (N worktrees)" entry (R10, R11). When no worktree is idle, the entry is
  omitted entirely — no "All idle (0)" row (R12). Selecting "All idle" routes the
  idle subset through `handleDeleteMultiple` (R11) — no new deletion code path.
- **Extra stat on the menu path.** Unlike `wt list` (which reuses the persisted
  `LastActive` — see below), `handleDeleteMenu` computes idleness with one extra
  `os.Stat` per option (`wt.IsIdle(wt.RecencyOf(o.path), now,
  DefaultIdleThreshold)`), because `SortByRecency` does not expose its internal
  recency keys. This is acceptable at the ≤100-worktree interactive scale and
  keeps the menu annotation consistent with `--stale`. R6's no-extra-stat
  guarantee governs only the `wt list` path, NOT this menu.

### `wt list` idle marker: reuses `LastActive`, adds no stat

> See `wt-cli/list-status-contract.md` for the `Idle *bool` JSON field, the
> `⚠ idle` rendering in the two `Last Active`-bearing layouts, and the
> displayWidth padding. Summary here for the cross-command view:

- `wt list` derives idleness from the recency value already persisted into
  `listEntry.LastActive` (by `--status` enrichment or recent-mode `sortEntries`)
  via `populateIdle(entries, now)` — **no new `os.Stat`, no `git` subprocess**
  (R6). The single-subprocess default-mode contract (one `git worktree list
  --porcelain`) is preserved; `name`/`branch` 3-column human modes perform zero
  per-worktree stat and show no marker.
- The marker (`⚠ idle`, on the `Last Active` cell) and the JSON `idle` field are
  shown only where `LastActive` is non-nil — i.e. under `--status`, and in the
  recent-mode human view (4-column). The main worktree is never marked idle (R7).

### SAFETY INVARIANT — idleness is never the sole delete gate (R20)

This is the load-bearing invariant of the whole change. **Idleness only ever
*selects* candidates; it never authorizes a removal on its own.**

- Every worktree selected by `--stale` or "All idle" passes through
  `handleDeleteMultiple` (and per-name removals through `handleDeleteByName`),
  whose existing per-worktree handling of uncommitted changes (stash/discard) and
  unpushed commits is unchanged. There is NO new deletion code path — both
  surfaces converge on the same rollback-safe flow used by positional and
  `--delete-all` deletion today.
- **Safe-by-direction.** The mtime signal *under-reports* staleness: a `fab
  sync`/build that touches an idle worktree makes it look fresh, so it escapes
  detection. This direction is safe — it hides genuinely-idle worktrees rather
  than ever exposing an unsafe worktree as deletable without the safety flow. (A
  signal that *over-reported* staleness would be the dangerous direction; this
  one does not.)
- **Main worktree is never an idle candidate (R21).** `handleDeleteStale` and
  `handleDeleteMenu` both `continue` past `e.path == ctx.RepoRoot`, and
  `populateIdle` forces the main worktree's `Idle` to `false`. No special-casing
  of the threshold is required — the structural `RepoRoot` exclusion already
  guarantees it — but it is a tested invariant in all three surfaces.

## Design Decisions

### Signal = dir-mtime (`RecencyOf`), framed as "idle / untouched on disk"

The idle predicate reuses the single `RecencyOf` (worktree-directory mtime)
signal — one signal definition across `wt list`/`wt open`/`wt delete`, zero new
`git` subprocesses, and it consumes the recency key `sortEntries`/enrichment
already computes. This **resolved** the open signal-quality question that was
deferred from `260530-rtmf` (the recency-ordering contract's "Signal-quality
caveat"): the decision is **option (b) — keep dir-mtime, with honest "idle /
untouched on disk for Nd" framing**, NOT a cleaner per-staleness signal.

A cleaner per-staleness signal (last-commit-date / reflog / `HEAD` mtime) was
*rejected* for this change: it costs a `git` subprocess per worktree and
introduces a second, divergent signal definition, for accuracy the safety flow
already backstops. It remains a future option if under-reporting proves painful.

### "idle" in user-facing copy; "stale" only in the flag name

The display marker and the JSON field use the word **"idle"**, not "stale".
"idle / untouched" states the verifiable fact (filesystem mtime age) without the
unprovable verdict that "stale" implies (that the work is dead). The `--stale`
*flag name* is acceptable because a user typing it already intends cleanup.
`--help` and the spec state plainly that the signal is filesystem mtime, not
commit activity. "stale" as the display marker was rejected (overclaims).

### Predicate takes a recency value, not a path

Every caller already holds a recency value (`LastActive` on the list path, a
`RecencyOf` result on the delete path). A `time.Time` parameter lets them reuse
it with no extra `os.Stat` on the cheap list path, mirroring how `RecencyLess`
is keyed on values. The interactive delete menu still pays one stat per option
(it has no persisted key to reuse) — an accepted cost at interactive scale.

### `--stale` as a single `NoOptDefVal` flag with `=`-required value

One flag covers both `--stale` (7d default) and `--stale=30d` (override),
matching the user's shorthand. The `=`-required quirk is an accepted tradeoff;
because the space-separated form collides with positional args, R16 makes
`--stale` + positionals a hard `ExitInvalidArgs` error rather than a silent
mis-target. Two flags (`--stale` + `--stale-after`) were rejected as more surface
than wanted; a bare-int value was rejected as less self-documenting than `Nd`.

## Cross-references

- Sibling memory: `wt-cli/recency-ordering-contract.md` — the shared `RecencyOf`
  signal this predicate consumes; the `wt delete` menu `defaultIdx` 2/3 shift and
  `, idle` row annotation; the resolved signal-quality caveat.
- Sibling memory: `wt-cli/list-status-contract.md` — the `Idle *bool`
  (`idle,omitempty`) JSON field, the `⚠ idle` marker on the `Last Active` cell in
  the 4-column recent and 5-column `--status` layouts, and the displayWidth-based
  padding of that cell.
- Spec doc: `docs/specs/cli-surface.md` — `wt delete` (`--stale` flag, selection
  menu), `wt list` (idle marker / `idle` field).
- Source: `src/internal/worktree/idle.go` — `IsIdle`, `DefaultIdleThreshold`,
  `ParseIdleThreshold`.
- Source: `src/cmd/wt/delete.go` — `--stale` flag registration (`NoOptDefVal =
  "7d"`), `handleDeleteStale`, `formatThreshold`, stale-aware `handleDeleteMenu`
  (`, idle` annotation, "All idle (N)", `firstWorktreeIdx`/`defaultIdx` 2/3), the
  `RunE` mutex checks.
- Source: `src/cmd/wt/list.go` — `listEntry.Idle *bool`, `populateIdle`, the
  `⚠ idle` marker rendering in the 5-column and 4-column branches of
  `handleFormattedOutput`.
- Source: `src/internal/worktree/recency.go` — `RecencyOf` (the consumed signal).
- Tests: `src/internal/worktree/idle_test.go` (predicate boundary cases,
  zero-recency, `ParseIdleThreshold` valid/reject), `src/cmd/wt/delete_test.go`
  (menu annotation, "All idle (N)", `defaultIdx` shift, `--stale` selection +
  override, zero-match exit-0, positional/`--delete-all` mutexes, main-never-
  targeted), `src/cmd/wt/list_test.go` (idle marker in 4-col/5-col, no marker in
  name/branch modes, JSON `idle` absent/present).
- Constitution: Principle I (no hidden state — threshold is a named constant, not
  an env/config knob), III (typed exit codes — `ExitInvalidArgs` for the mutexes
  and invalid threshold), V (predicate/constant/parser live in
  `internal/worktree`), VI (machine-output stability — `idle` is additive and
  absent on the default `--json` path).
