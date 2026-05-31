# Plan: Recency-Aware Listing

**Change**: 260530-rtmf-recency-aware-listing
**Intake**: [intake.md](intake.md)
**Spec**: [spec.md](spec.md)

## Tasks

### Phase 1: Shared recency helper (internal/worktree)

- [x] T001 Create `src/internal/worktree/recency.go` with `RecencyOf(path string) time.Time` (os.Stat ModTime, zero time on stat failure) and a recency ordering helper keyed on `(RecencyOf(path), Name)`: `RecencyLess(aRecency time.Time, aName string, bRecency time.Time, bName string) bool` (most-recent first, Name-ascending tie-break) plus a generic `SortByRecency[T any](items []T, pathOf func(T) string, nameOf func(T) string)` adapter consumable by the heterogeneous caller structs. <!-- A-001 A-002 -->
- [x] T002 Create `src/internal/worktree/recency_test.go`: `RecencyOf` returns ModTime for an existing dir and zero time for a vanished path; `RecencyLess`/`SortByRecency` order newest-first; deterministic Name-ascending tie-break for equal mtimes (incl. two zero-time entries). Use `os.Chtimes` for controlled mtimes. <!-- A-009 A-010 -->

### Phase 2: Consume shared helper in open/delete menus

- [x] T003 Refactor `selectAndOpen` in `src/cmd/wt/open.go`: replace the inline `os.Stat`/`ModTime` newest-tracking loop (~256-287) with `wt.SortByRecency` over the `options` slice (newest-first), then set `defaultIdx = 1` (newest now at top). Behavior-preserving default selection (newest still pre-selected); menu items now newest-first. <!-- A-013 A-014 -->
- [x] T004 Refactor `handleDeleteMenu` in `src/cmd/wt/delete.go`: replace the inline mtime loop (~457-489) with `wt.SortByRecency` over `options` (newest-first), then `defaultIdx = 2` (offset by 1 for the prepended "All" entry; newest now first among worktrees). <!-- A-013 A-014 -->
- [x] T005 Add menu-ordering tests: `src/cmd/wt/open_test.go` and `src/cmd/wt/delete_test.go` assert non-main worktrees print newest-first in the menu (newest at top). Drive via `wt open`/`wt delete` from main repo with controlled mtimes (`os.Chtimes`) and empty stdin (ShowMenu prints the menu before reading); no side-effecting app/target is invoked. <!-- A-013 A-014 -->

### Phase 3: `wt list` sorting + audience-split default

- [x] T006 In `src/cmd/wt/list.go` `listCmd`: add `--sort` StringVar and `--non-interactive` BoolVar. Add flag validation following the existing mutex pattern — `--path` + `--sort` → `wt.ExitInvalidArgs` with "--path and --sort are mutually exclusive"; invalid `--sort` value → `wt.ExitInvalidArgs` naming `recent`, `name`, `branch`. <!-- A-003 A-006 A-007 -->
- [x] T007 In `src/cmd/wt/list.go`: implement the ordering step applied to the final `entries` slice AFTER enrichment (and to the basic slice in default mode). Resolve effective sort: explicit `--sort` wins; else default = `recent` unless `--json` or `--non-interactive` is set, in which case default = `name`. Pin the main worktree (`IsMain`) to the first row; reorder only non-main entries. `recent` uses `wt.SortByRecency`; `name` sorts by `Name` ascending; `branch` sorts by `Branch` ascending. <!-- A-004 A-005 -->

### Phase 4: `--status` last-active column

- [x] T008 In `src/cmd/wt/list.go`: add `LastActive *time.Time `+"`json:\"last_active,omitempty\"`"+` to `listEntry`. In `listEntriesEnriched`, pre-allocate `LastActive` to a non-nil pointer (zero `time.Time`) BEFORE the stat-gate `continue` (mirroring Dirty/Unpushed), and set it from `wt.RecencyOf(r.path)` so vanished worktrees keep a zero-time key. No new git subprocess. Default mode keeps the pointer nil (key omitted). <!-- A-008 A-011 -->
- [x] T009 In `src/cmd/wt/list.go` `handleFormattedOutput`: under `--status`, render a relative "last active" value (e.g. `2h ago`, `3d ago`, `just now`) for each row's human output. Add a `relativeTime(t time.Time) string` helper with coarse buckets. JSON path is unchanged (emits raw RFC3339 via the `*time.Time` field). <!-- A-012 -->

### Phase 5: Tests for list sorting + last-active

- [x] T010 Extend `TestList_StatusOrderingPreserved` in `src/cmd/wt/list_test.go` so the stable-order invariant is asserted in stable mode (`--status` defaults to name order now under non-interactive? no — `--status` human output is recency by default). Adjust the test to assert against the EFFECTIVE order: pass `--sort=name` (or `--non-interactive`/`--json`) so the parallel-enrichment "no reorder relative to chosen order" invariant is checked deterministically. Add recency-sort ordering tests (`--sort=recent` newest-first with controlled mtimes), name-sort test, invalid-sort-value test, `--path`+`--sort` mutex test, main-pinned-first-under-recency test. <!-- A-003 A-004 A-005 A-006 -->
- [x] T011 Add JSON-default-stable integration test in `src/cmd/wt/list_test.go`: `wt list --json` (no `--sort`) prints non-main worktrees in name-ascending order regardless of mtime; `wt list --json --sort=recent` prints newest-first; `last_active` key absent without `--status` and present (RFC3339) with `--status --json`. <!-- A-008 A-016 -->

## Execution Order

Phase 1 (T001) is a prerequisite for Phases 2-4 (the shared helper is consumed everywhere). T002 can run alongside once T001 exists. Phases 2, 3, 4 are otherwise independent of each other and depend only on Phase 1. Phase 5 tests depend on their respective implementation tasks (T010/T011 on T006-T009).

## Acceptance

### Functional Completeness

- [x] A-001 `internal/worktree` exposes exactly one `RecencyOf(path string) time.Time` returning the directory's `os.Stat` ModTime, or zero time on stat failure.
- [x] A-002 `internal/worktree` exposes one recency ordering helper keyed on `(RecencyOf(path), Name)`, most-recent-first with deterministic Name-ascending tie-break, consumable by list/open/delete without an `Info` adapter.
- [x] A-003 `wt list --sort` accepts `recent`, `name`, `branch`; an unrecognized value exits `ExitInvalidArgs` naming the three accepted values.
- [x] A-004 The main worktree occupies the first output row under every sort mode and the default; only non-main entries are reordered.
- [x] A-005 Default ordering is `recent` unless `--json` or `--non-interactive` is set (then `name`); an explicit `--sort` overrides the default in any mode.
- [x] A-006 `wt list --path <name> --sort=<any>` exits `ExitInvalidArgs` with stderr containing "--path and --sort are mutually exclusive".
- [x] A-007 `wt list` defines a `--non-interactive` BoolVar whose only effect is selecting the stable default order.
- [x] A-008 `listEntry` has `LastActive *time.Time` with `omitempty`; nil (key omitted) in default mode, non-nil under `--status` for every entry (zero time for vanished worktrees).
- [x] A-012 Under `--status`, human output renders a relative last-active value per worktree; JSON emits the raw RFC3339 timestamp.
- [x] A-013 `wt open` and `wt delete` menus list non-main worktrees newest-first via the shared comparator, newest at top.
- [x] A-014 The `open`/`delete` default menu selection remains the most-recent worktree after the refactor (behavior-preserving default).

### Behavioral Correctness

- [x] A-016 `wt list --json` default order is stable name order (not recency); `wt list --json --sort=recent` is newest-first.

### Scenario Coverage

- [x] A-009 `RecencyOf` of an existing worktree returns its ModTime; of a vanished worktree returns zero time without error/panic.
- [x] A-010 The comparator yields `[t3,t2,t1]` for mtimes `t1<t2<t3`, and breaks `alpha` before `bravo` on equal mtime.

### Edge Cases & Error Handling

- [x] A-017 A vanished worktree under `--status` emits `last_active: "0001-01-01T00:00:00Z"` (zero time) — key present, consistent with `dirty:false`/`unpushed:0`.

### Code Quality

- [x] A-018 The duplicated inline mtime loops in `open.go` and `delete.go` are removed; recency logic lives only in `internal/worktree` (Constitution V; no duplicated-utility anti-pattern).
- [x] A-019 New code follows existing patterns (pointer-field JSON shape, `ExitWithError`/`ExitInvalidArgs` mutex idiom, flag registration style, function size) and adds no new dependency.
- [x] A-020 The `list-status-contract.md` invariants hold: default mode stays one git subprocess (no `last_active`/`dirty`/`unpushed` enrichment), pointer pre-allocation before the stat gate, worker-pool indexed-write ordering preserved (sorting is a separate post-enrichment pass), existing `--path`/`--json` and `--path`/`--status` mutex checks unchanged.

### Testing

- [x] A-021 Recency helper, comparator, list sorting (recent/name/invalid/main-pinned/path-mutex), menu ordering, JSON-default-stable, and last-active key presence/absence are covered by tests; tests use controlled mtimes (`os.Chtimes`) and do not leak host side-effects.
