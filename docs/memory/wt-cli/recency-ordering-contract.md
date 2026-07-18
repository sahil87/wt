---
type: memory
description: "The single recency definition (`RecencyOf`/`RecencyLess`/`SortByRecency`) and newest-first ordering across `wt list`, `wt open`, and `wt delete`."
---
# wt-cli: Recency Ordering Contract

> Post-implementation behavior capture for the shared recency signal and
> newest-first ordering across `wt list`, `wt open`, and `wt delete`.
> Source change: `260530-rtmf-recency-aware-listing`.
> Amended by `260718-daqj-go-include-main-worktree` (the open/go selection menus
> now pin the main worktree to row 1 *outside* the recency ordering — mirroring
> `wt list`'s pin-first convention — shifting the pre-selected default to the
> newest non-main worktree at `defaultIdx = 2` (or `1` when main is the only
> row); non-main newest-first ordering and the single `SortByRecency` call site
> unchanged).

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

- `selectAndOpen` (`src/cmd/wt/open.go`, via the shared `selectWorktree` helper as
  of `260620-3pp5` — see below) and `handleDeleteMenu` (`src/cmd/wt/delete.go`)
  sort the **non-main** worktrees with `wt.SortByRecency` so the **newest
  non-main worktree appears at the top** of that group. This replaces the
  previous behavior where items appeared in porcelain order and only the newest
  was highlighted.
- The pre-selected menu default remains the **most-recent non-main** worktree —
  this is behavior-preserving for the default selection. Only the item
  *ordering* changed (and, for open/go, the addition of the pinned main row —
  see below).
- **`wt open` / `wt go` menus pin main to row 1, OUTSIDE the recency ordering**
  (`260718-daqj`, via the shared `selectWorktree` helper): the porcelain-first
  entry (`entries[0]`, always the main worktree) is partitioned out and
  **prepended** as a pinned `main (<branch>)` row after the non-main slice
  (`entries[1:]`) is sorted newest-first — the same pattern `wt list`'s
  `sortEntries` uses (partition out row 0, reorder the rest; see
  `wt-cli/list-status-contract.md`). Main is pinned, never sorted into the
  recency order. The pre-selected default stays the newest *non-main* worktree:
  `defaultIdx = 2` when ≥1 non-main worktree exists (main row 1, newest worktree
  row 2), `defaultIdx = 1` when main is the only row. (Before `260718-daqj` the
  main worktree was *skipped* in these menus and `defaultIdx = 1`; `wt delete`'s
  main exclusion at all its list sites is a separate contract and is **not**
  changed by this — see below.)
- **`wt delete`** still builds a `wtOption` slice of **non-main** worktrees only
  (the main worktree / `ctx.RepoRoot` is skipped — deleting main must stay
  impossible, an explicit Non-Goal of `260718-daqj`).
- `wt delete`: `defaultIdx` is **2 by default**, shifting to **3 ONLY when the
  "All idle (N)" entry is present** (amended by `260530-5fyu` — see below). The
  newest worktree is always the first *worktree* row and stays the pre-selected
  default; the index just shifts by the number of prepended summary entries
  ("All (N worktrees)" always, plus "All idle (N)" when ≥1 worktree is idle).
- All these menus produce identical **non-main** ordering (all driven by the
  shared `SortByRecency` definition), so they never disagree on order. They
  differ only in the fixed rows each prepends: open/go pin the `main` row
  (`260718-daqj`); delete prepends its "All (N worktrees)" / "All idle (N)"
  summary rows and omits main entirely.

### `wt go`'s no-arg menu is a new `SortByRecency` consumer (`260620-3pp5`)

- The `260620-3pp5-open-worktree-from-worktree` change extracted the
  `selectAndOpen` menu logic into the shared `selectWorktree` helper
  (`src/cmd/wt/open.go`), which calls `wt.SortByRecency` over its local non-main
  `wtOption` slice. That single helper now backs **three** menu callers —
  `wt open` (main-repo no-arg, prompt "Select worktree to open:"), `wt go`
  (no-arg, prompt "Select worktree to go to:"), and `wt open --select` (no-arg).
  So `wt go`'s selection menu is a consumer of the same newest-first ordering,
  joining `wt list` / `wt open` / `wt delete` — there is still exactly one
  `SortByRecency` call site for the open/go selection menu, not a per-verb copy.
  *(The helper's shape has since changed — `260718-daqj` dropped the `ctx`
  parameter and the main-repo filter, pinning main as row 1 instead, and shifted
  the default to `defaultIdx = 2/1`; its signature is now `(session, prompt)`.
  The single-call-site invariant this section documents is unchanged.)*
- The non-main ordering, branch display, and newest-non-main default are
  byte-identical across all three callers because they share the one helper. See
  `/wt-cli/go-command-contract.md` for the `wt go` / `wt open --select` behavior
  contract, the pinned main row, and the `selectWorktree` extraction details.

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
- Sibling memory: `wt-cli/go-command-contract.md` — the `wt go` selector and
  `wt open --select` composition (flag formerly `--go`, now a deprecated alias);
  the `selectWorktree` menu consumers of the `SortByRecency` ordering documented
  here, and the pinned main row (`260718-daqj`) that sits above that ordering.
- Spec doc: `docs/specs/cli-surface.md` — `wt list` (`--sort` flag), `wt open`
  (selection menu, "most recently modified worktree" default), `wt delete`
  (selection menu).
- Spec doc: `docs/specs/worktree-layout.md` — worktree filesystem layout and
  naming; the directory whose mtime is the recency signal is the
  `<repo>.worktrees/<name>/` root.
- Source: `src/internal/worktree/recency.go` — `RecencyOf`, `RecencyLess`,
  `SortByRecency`.
- Source: `src/cmd/wt/open.go` (`selectWorktree` shared helper + `selectAndOpen`;
  `selectWorktree` is also called by `wt go` and `wt open --select`; `260718-daqj`:
  it now sorts only the non-main `entries[1:]` slice and prepends the pinned
  `main` row, `defaultIdx = 2/1`), `src/cmd/wt/delete.go`
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
