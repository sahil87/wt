# Intake: List Recency Column

**Change**: 260601-73cv-list-recency-column
**Created**: 2026-06-01
**Status**: Draft

## Origin

> In the recency mode, we should also show the "recency" — right now there is no
> time related column, so its tough to figure out how recent things are.

Conversational mode, preceded by a `/fab-discuss` session. The user observed that
`wt list`'s default human view is sorted newest-first (recency order) but renders
no time column, so the *ordering* is invisible — a user can't tell how recent any
worktree actually is, only its relative position. During discussion we resolved
scope, format, and contract-handling via an explicit question round:

- **Scope** → recent mode only (default human view + explicit `--sort=recent`).
- **Format** → reuse the existing `relativeTime()` coarse buckets.
- **Contract** → treat the `Name/Branch/Path` default-layout change as an
  intentional amendment to `list-status-contract.md`, documented at hydrate.

## Why

1. **Problem (pain point)**: `wt list` defaults to `recent` sort for human output
   (per `recency-ordering-contract.md` and `list-status-contract.md`), but the
   relative-time value is only rendered under `--status` (the 5-column dashboard
   view). In the *default* recency-ordered view the rows are sorted newest-first
   with **no time column**, so the sort key is invisible. Users see an order but
   cannot tell whether the top entry was touched minutes or weeks ago, nor where
   the "recent vs. stale" boundary falls.
2. **Consequence of not fixing**: The recency-aware-listing feature
   (`260530-rtmf`) delivers a *better default order* but withholds the *one piece
   of information that explains the order*. Users must opt into the slower
   `--status` path purely to see timestamps they're already being sorted by —
   defeating the ergonomic intent of recency-by-default.
3. **Why this approach**: The recency sort key is **already computed** in
   `sortEntries` for `recent` mode (`wt.RecencyOf(e.Path)` at `list.go:211`, or
   the already-set `*LastActive` under `--status`). Today that key is discarded
   after sorting. Persisting it into `entries[i].LastActive` and rendering it via
   the existing `relativeTime()` helper shows the value **at zero additional
   cost** — no new `os.Stat`, no new `git` subprocess. Alternatives rejected in
   discussion: (a) showing the column in *all* human modes (`name`/`branch` too)
   would add a per-worktree `os.Stat` to modes that currently do zero per-worktree
   work; (b) an absolute timestamp is wider and less scannable than the relative
   buckets; (c) requiring `--status` to see recency (status quo) is the exact
   friction this change removes.

## What Changes

### Render a `Last Active` column in the non-status recency view

When the resolved sort mode is `recent` AND `--status` is NOT set AND output is
human (not `--json`), `wt list` renders a **4-column** table:

```
  Name          Branch              Last Active  Path
* swift-fox     feature/login       2h ago       ../wt.worktrees/swift-fox/
  quiet-otter   fix/recency-column  3d ago       ../wt.worktrees/quiet-otter/
  (main)        main                just now     ./
```

- The `Last Active` value uses the existing `relativeTime()` buckets
  (`just now`, `Nm ago`, `Nh ago`, `Nd ago`); a zero time renders as `-`.
- The displayed value is the **same key the rows were sorted by** — no TOCTOU,
  no second stat.

### Persist the recency sort key into `LastActive`

`sortEntries` currently computes the per-entry recency key into a local `keys[]`
slice (`list.go:206-213`) and discards it after ordering. For `recent` mode it
SHALL write that key back into `entries[i].LastActive` so the rendering path can
display it. In default/basic mode this is the `wt.RecencyOf(e.Path)` value; under
`--status` `LastActive` is already non-nil and is left as-is. This reuses the
already-paid stat — no new `os.Stat`.

### Thread the sort mode into the rendering decision

`handleFormattedOutput` currently keys layout purely on `showStatus`
(`list.go:304`, 3-column `else` branch). It needs to know whether the active sort
is `recent` to choose the 4-column layout. This requires passing the resolved
`sortMode` (or a derived boolean) into `handleFormattedOutput` and adding a third
rendering branch (or refactoring the width/print blocks to share logic across the
3-, 4-, and 5-column layouts).

### Explicitly unchanged

- **JSON output**: `last_active` remains `omitempty` and is emitted only under
  `--status`. `--json` (and `--json --sort=recent`) keep stable `name` default and
  do NOT gain the field. The machine-readable contract (Constitution VI) is
  untouched.
- **`--status` view**: the existing 5-column layout (Name/Branch/Status/Last
  Active/Path) is unchanged.
- **`name` / `branch` sort modes**: stay 3-column (Name/Branch/Path), no time
  column, no new `os.Stat`.

## Affected Memory

- `wt-cli/list-status-contract.md`: (modify) The "Default human output: Name /
  Branch / Path" invariant changes for recency-ordered output — the default human
  view now renders a 4th `Last Active` column when sorting by recency. The
  `relativeTime` rendering and `LastActive` population are no longer exclusive to
  `--status`. Document the sort-key persistence and the sort-mode threading into
  `handleFormattedOutput`.
- `wt-cli/recency-ordering-contract.md`: (modify, minor) Note that `wt list`
  recent mode now persists the computed recency key into `LastActive` for display
  (previously discarded after sorting). The comparator/ordering definition itself
  is unchanged.

## Impact

- **Code**: `src/cmd/wt/list.go` — `handleFormattedOutput` (new signature /
  branch), `sortEntries` (write back `LastActive` in `recent` mode). No changes to
  `src/internal/worktree/recency.go`.
- **Tests**: `src/cmd/wt/list_test.go` — new coverage for the 4-column recent-mode
  header and a relative-time value in default human output; assert `name`/`branch`
  modes stay 3-column; assert `--json` still omits `last_active` without
  `--status`.
- **Dependencies**: none. No new `os.Stat`, no new `git` subprocess, no new module
  dependency.
- **Constitution**: II (Cobra surface — no flag change), IV (test what the user
  sees — new render assertions), VI (interactive-by-default; machine output stays
  stable). No constitution amendment required.

## Open Questions

- ~~Column order: `Name / Branch / Last Active / Path` vs. trailing after `Path`.~~
  **Resolved** (clarify 2026-06-01): `Name / Branch / Last Active / Path` —
  mirrors the `--status` layout's relative placement and keeps the time column
  left-aligned ahead of the wide variable-width `Path`.
  <!-- clarified: column order = Name/Branch/Last Active/Path — user confirmed recommendation -->

## Clarifications

### Session 2026-06-01 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 4 | Confirmed | — |
| 5 | Confirmed | — |
| 6 | Confirmed | — |

### Session 2026-06-01 (tentative)

| # | Q | A |
|---|---|---|
| 7 | Column order for the new view | Confirmed `Name / Branch / Last Active / Path` (recommendation) |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = recent mode only (default human view + `--sort=recent`); `name`/`branch` stay 3-column | Discussed — user chose "Only in recent mode" over "All human modes"; avoids adding `os.Stat` to modes that do zero per-worktree work | S:95 R:80 A:90 D:90 |
| 2 | Certain | Reuse existing `relativeTime()` buckets for the column value | Discussed — user chose "Reuse relativeTime()" over finer-grained/absolute; zero new formatting logic, consistent with `--status` column | S:95 R:85 A:95 D:90 |
| 3 | Certain | Treat the `Name/Branch/Path` default-layout change as an intentional amendment to `list-status-contract.md`, documented at hydrate | Discussed — user chose "Treat as intentional amendment" over "argue it's not a violation" | S:90 R:70 A:85 D:85 |
| 4 | Certain | Persist the discarded `sortEntries` recency key into `entries[i].LastActive` rather than re-statting in the render path | Clarified — user confirmed. Reuses the already-paid `os.Stat`; avoids TOCTOU between sort key and displayed value; the alternative (re-stat in render) re-introduces a second stat and a drift window | S:95 R:75 A:85 D:80 |
| 5 | Certain | JSON output unchanged — `last_active` stays `omitempty`/`--status`-only; `--json`/`--non-interactive` keep stable `name` default | Clarified — user confirmed. Preserves Constitution VI machine-stability and the existing JSON contract; the request is explicitly about the *human* recency view | S:95 R:70 A:90 D:85 |
| 6 | Certain | Pass resolved `sortMode` (or derived bool) into `handleFormattedOutput` to select the 4-column layout | Clarified — user confirmed. `handleFormattedOutput` keys only on `showStatus` today; recency rendering needs the sort mode. Threading the already-resolved mode is the minimal seam | S:95 R:75 A:80 D:75 |
| 7 | Certain | Column order = `Name / Branch / Last Active / Path` | Clarified — user confirmed recommendation. Mirrors `--status` relative placement (Last Active immediately before Path) | S:95 R:85 A:60 D:55 |

7 assumptions (7 certain, 0 confident, 0 tentative, 0 unresolved).
