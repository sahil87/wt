# wt-cli: Recency Ordering Contract

> Post-implementation behavior capture for the shared recency signal and
> newest-first ordering across `wt list`, `wt open`, and `wt delete`.
> Source change: `260530-rtmf-recency-aware-listing`.

This file documents the single definition of "recent" that `wt list`, `wt open`,
and `wt delete` honor after the recency-aware-listing change. Future changes
touching `src/internal/worktree/recency.go`, or the menu/list ordering in
`cmd/wt/{list,open,delete}.go`, should preserve these invariants unless an
explicit spec amendment supersedes them.

## Requirements

### One recency signal: worktree-directory mtime

- The recency signal is the modification time (`mtime`) of a worktree's
  working-directory root, obtained via `os.Stat`. There is exactly one function
  that defines it — `wt.RecencyOf(path string) time.Time` in
  `src/internal/worktree/recency.go` — consumed by every caller (`wt list`,
  `wt open`, `wt delete`). No caller may define a second notion of recency.
- The signal is the **full-precision** `os.Stat` `ModTime()` (sub-second), NOT
  the whole-second `ModTime().Unix()` granularity used by the old inline loops.
  Ordering ties are resolved by the Name tie-break (below), not by porcelain
  position.
- `RecencyOf` takes a directory **path** (`string`), never a `worktree.Info`.
  The three consumers each build their own cmd-local struct (list's `listEntry`,
  open/delete's local `wtOption`) from `listWorktreeEntries()`'s `rawEntry`; none
  routes through `worktree.List()`/`Info`. A `string` parameter lets every caller
  pass its own `.path`/`.Path` field directly without an `Info` adapter
  (Constitution V — shared logic in `internal/worktree`, consumable everywhere).
- When the path cannot be stat'd (vanished worktree, permissions error),
  `RecencyOf` SHALL return the zero `time.Time` — never an error, never a panic.
  Recency is an ordering hint, not an operation that should fail a command.

### Comparator: newest-first with deterministic Name tie-break

- `wt.RecencyLess(aRecency time.Time, aName string, bRecency time.Time, bName string) bool`
  is the single comparator. It is keyed on `(recency, Name)` rather than typed
  on a concrete struct, so the heterogeneous consumers reuse it without
  conversion.
- Ordering is **most-recent first**: `RecencyLess` returns `aRecency.After(bRecency)`
  when recencies differ.
- Ties (equal mtime, **including two zero-time entries**) are broken
  deterministically by worktree `Name` **ascending** (`aName < bName`), so output
  is stable across runs. This is a strict improvement over the old loops'
  non-deterministic first-wins behavior on equal seconds.

### Shared sort adapter: `SortByRecency`

- `wt.SortByRecency[T any](items []T, pathOf func(T) string, nameOf func(T) string)`
  orders a caller's slice newest-first in place. The `pathOf`/`nameOf` accessors
  adapt each consumer's own struct to the `(RecencyOf(path), Name)` key.
- It is implemented with `sort.SliceStable`, so equal-key items keep input order
  before the Name tie-break decides — deterministic and re-run-stable.
- `wt open` and `wt delete` consume `SortByRecency` directly over their local
  `wtOption` slices. `wt list` does NOT call `SortByRecency` for its recent mode:
  it inlines `RecencyLess` over a per-entry `keys[]` slice that prefers the
  already-computed `*LastActive` over a fresh stat (see
  `wt-cli/list-status-contract.md`). Both paths use the same `RecencyLess`
  ordering definition, so they never drift. (`260601-73cv` did not change this:
  `wt list` still inlines `RecencyLess` and does not call `SortByRecency`.)
- **`260601-73cv` (sort-key reuse for display)**: `wt list` recent mode now
  PERSISTS the recency key it computes — previously discarded after sorting —
  into `entries[i].LastActive`, gated on the human-output path (`persistKey ==
  !jsonOut`), so the new `Last Active` column can display it without a second
  `os.Stat`. This is a pure side-effect on the existing key computation: the
  comparator/ordering definition (`RecencyOf`/`RecencyLess`/`SortByRecency`,
  newest-first with Name-ascending tie-break) is UNCHANGED. The resulting order is
  byte-for-byte identical to before; only the discard was removed. See
  `wt-cli/list-status-contract.md` ("Recent mode persists the sort key into
  `LastActive`") for the `persistKey` seam and the main-worktree populate.

### Open / delete menus list non-main worktrees newest-first

- `selectAndOpen` (`src/cmd/wt/open.go`) and `handleDeleteMenu`
  (`src/cmd/wt/delete.go`) build a `wtOption` slice of non-main worktrees
  (the main worktree / `ctx.RepoRoot` is skipped) and sort it with
  `wt.SortByRecency` so the **newest worktree appears at the top** of the menu.
  This replaces the previous behavior where items appeared in porcelain order and
  only the newest was highlighted.
- The pre-selected menu default remains the **most-recent** worktree — this is
  behavior-preserving for the default selection. Only the item *ordering* changed.
- `wt open`: `defaultIdx = 1` (newest is the first menu item; index 0 is the
  cancel/menu-zero slot in `ShowMenu`).
- `wt delete`: `defaultIdx` is **2 by default**, shifting to **3 ONLY when the
  "All idle (N)" entry is present** (amended by `260530-5fyu` — see below). The
  newest worktree is always the first *worktree* row and stays the pre-selected
  default; the index just shifts by the number of prepended summary entries
  ("All (N worktrees)" always, plus "All idle (N)" when ≥1 worktree is idle).
- The two menus produce identical non-main ordering (both driven by the same
  `SortByRecency` call), so `wt open` and `wt delete` never disagree on order.

### `wt delete` menu: stale-aware annotation + "All idle" (`260530-5fyu`)

- `handleDeleteMenu` (`src/cmd/wt/delete.go`) now annotates each idle non-main
  option label with a trailing `, idle` (e.g. `feature-x (feat-x), idle`), and
  inserts an **"All idle (N)"** entry immediately after the existing "All (N
  worktrees)" entry **when at least one non-main worktree is idle**. When none is
  idle the entry is omitted (no "All idle (0)" row). Menu ordering is unchanged —
  non-main worktrees still list newest-first via `wt.SortByRecency`.
- The pre-selected default index is computed via a local `firstWorktreeIdx`
  (`2`, or `3` when "All idle" is present); `defaultIdx = firstWorktreeIdx`. This
  is what shifts the documented `defaultIdx = 2` to `3` only in the "All idle"
  case. Selecting "All idle" routes the idle subset through `handleDeleteMultiple`
  — no new deletion code path.
- **Idleness is derived from `RecencyOf` via the new `IsIdle` predicate**:
  `wt.IsIdle(wt.RecencyOf(o.path), now, wt.DefaultIdleThreshold)`, computed per
  option. This costs **one extra `os.Stat` per option** on the interactive menu
  path, because `SortByRecency` does not expose its internal recency keys for
  reuse. That is acceptable at the ≤100-worktree interactive scale and keeps the
  annotation consistent with `wt delete --stale`. **The `wt list` path adds no
  such stat** — it reuses the persisted `LastActive` (see
  `wt-cli/list-status-contract.md`). The shared `IsIdle` predicate,
  `DefaultIdleThreshold`, and the `--stale` selector are documented in
  `wt-cli/idle-staleness-contract.md`.

### Refactor consolidated the duplicated inline loops

- The old `os.Stat`/`ModTime` newest-tracking loops that lived inline in
  `open.go` (`selectAndOpen`) and `delete.go` (`handleDeleteMenu`) are removed.
  All recency logic now lives only in `src/internal/worktree/recency.go`
  (Constitution V; no duplicated-utility anti-pattern). A future third consumer
  must reuse `RecencyOf`/`RecencyLess`/`SortByRecency`, not re-derive mtime
  comparison.

## Design Decisions

### One `RecencyOf` + comparator in `internal/worktree`

Consolidates the duplicated inline mtime loops in `open.go` and `delete.go` into
a single definition consumed everywhere, and gives `wt list` sorting the same
notion of "recent". Honors Constitution V (non-trivial logic lives under
`internal/worktree`). Leaving the loops inline and adding a third copy in
`list.go` was rejected — it would triple the duplication and invite three
divergent definitions of recency.

### Worktree-directory mtime as the signal

A free `os.Stat` that is already performed on the menu/enrichment paths, so it
adds zero `git` subprocesses and preserves the existing menu default-selection
behavior exactly. git commit date / reflog / HEAD-file mtime were rejected — each
costs a subprocess or extra complexity.

### Path-keyed comparator over a struct-typed one

`RecencyOf`/`RecencyLess`/`SortByRecency` are keyed on `(path, Name)` rather than
on `worktree.Info` because the three consumers each build their own cmd-local
slice from `rawEntry` and none holds an `Info` at the call site. A struct-typed
comparator would force every caller to produce an `Info` adapter that does not
otherwise exist — a fabricated cross-package dependency. The generic
`SortByRecency[T]` with `pathOf`/`nameOf` accessors keeps the helper consumable
by all three heterogeneous types without conversion.

## Signal-quality caveat (resolved by `260530-5fyu`)

Worktree-directory `mtime` is a **noisy** recency signal: the directory's mtime
moves on *any* file write inside the worktree root (editor save, build artifact,
`git` operation that touches a top-level file), not only on meaningful
"activity". It is sufficient for *ordering* (the most-recently-touched worktree
is a reasonable default), but it is not a reliable staleness measure.

The follow-up change `260530-5fyu-stale-worktree-hints` revisited this and
**resolved the signal-quality question**: the decision was to **keep dir-mtime
with honest "idle / untouched on disk" framing** (option b), NOT to add a
cleaner per-staleness signal (git commit date, reflog, HEAD mtime — each costs a
`git` subprocess per worktree and a second, divergent definition). The
under-reporting tendency of mtime is safe-by-direction for the cleanup use case
because the `wt delete` safety flow backstops every removal — idleness only
*selects* candidates, never gates them. A configurable recency signal remains a
future option if under-reporting proves painful. See
`wt-cli/idle-staleness-contract.md` for the idle predicate, the 7-day
`DefaultIdleThreshold`, and the `--stale` selector that this resolution
produced.

## Cross-references

- Sibling memory: `wt-cli/list-status-contract.md` — the `wt list` ordering and
  `last_active` field that consume this contract; documents how `sortEntries`
  reuses the already-computed `*LastActive` as the recent-sort key and that
  ordering is a post-enrichment step that does not disturb the worker-pool
  indexed-write ordering.
- Sibling memory: `wt-cli/idle-staleness-contract.md` — the `IsIdle` predicate
  (built on the `RecencyOf` signal documented here), `DefaultIdleThreshold`, and
  the `wt delete --stale` selector; the authoritative cross-command idle contract
  behind the `defaultIdx` 2/3 shift and `, idle` annotation noted above.
- Spec doc: `docs/specs/cli-surface.md` — `wt list` (`--sort` flag), `wt open`
  (selection menu, "most recently modified worktree" default), `wt delete`
  (selection menu).
- Spec doc: `docs/specs/worktree-layout.md` — worktree filesystem layout and
  naming; the directory whose mtime is the recency signal is the
  `<repo>.worktrees/<name>/` root.
- Source: `src/internal/worktree/recency.go` — `RecencyOf`, `RecencyLess`,
  `SortByRecency`.
- Source: `src/cmd/wt/open.go` (`selectAndOpen`), `src/cmd/wt/delete.go`
  (`handleDeleteMenu` — now stale-aware: `firstWorktreeIdx`/`defaultIdx` 2/3,
  `, idle` annotation, "All idle (N)"; plus `handleDeleteStale`), `src/cmd/wt/list.go`
  (`sortEntries`, `resolveSort`).
- Source: `src/internal/worktree/idle.go` — `IsIdle`, `DefaultIdleThreshold`
  (the idle predicate consuming `RecencyOf`; see `wt-cli/idle-staleness-contract.md`).
- Tests: `src/internal/worktree/recency_test.go` (helper + comparator + tie-break,
  controlled mtimes via `os.Chtimes`), `src/cmd/wt/open_test.go`,
  `src/cmd/wt/delete_test.go` (menu newest-first ordering).
- Constitution: Principle V (non-trivial logic under `internal/worktree`),
  VI (interactive-by-default menus; deterministic ordering).

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260530-rtmf-recency-aware-listing` | 2026-05-31 | Introduced the single recency definition: `RecencyOf(path)` (worktree-dir `os.Stat` mtime, zero time on failure), `RecencyLess` (newest-first, deterministic Name-ascending tie-break), and the generic `SortByRecency[T]` adapter in `internal/worktree`. Consolidated the duplicated inline mtime loops in `open.go`/`delete.go` into this shared helper; `wt open`/`wt delete` menus now list non-main worktrees newest-first with the newest pre-selected as the default (behavior-preserving default selection). |
| `260601-73cv-list-recency-column` | 2026-06-01 | `wt list` recent mode now PERSISTS the computed recency key into `entries[i].LastActive` for display (previously discarded after sorting), gated on the human-output path via `sortEntries`'s new `persistKey` param. The comparator/ordering definition (`RecencyOf`/`RecencyLess`/`SortByRecency`, newest-first with Name-ascending tie-break) is UNCHANGED, and `wt list` still inlines `RecencyLess` (does not call `SortByRecency`). Ordering output is identical to before; only the key-discard was removed. See `wt-cli/list-status-contract.md` for the rendering/`persistKey` details. |
| `260530-5fyu-stale-worktree-hints` | 2026-06-01 | Amended the `wt delete` menu `defaultIdx`: now **2 by default, 3 only when the "All idle (N)" entry is present** (computed via a local `firstWorktreeIdx`), keeping the newest worktree as the pre-selected default in both cases. `handleDeleteMenu` annotates idle non-main option labels with a trailing `, idle` and inserts the "All idle (N)" summary entry (after "All (N worktrees)") when ≥1 worktree is idle; menu ordering still newest-first via `SortByRecency`. Idleness is derived from the `RecencyOf` signal via the new `IsIdle` predicate — one extra `os.Stat` per option on the interactive menu path (acceptable at ≤100-worktree scale; the `wt list` path adds no stat). **Resolved this file's signal-quality caveat**: kept dir-mtime with honest "idle / untouched on disk" framing (no second/cleaner staleness signal). The predicate, `DefaultIdleThreshold`, and `--stale` selector live in `wt-cli/idle-staleness-contract.md`. |
