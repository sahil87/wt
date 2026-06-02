# Plan: Recency-Aware Listing

**Change**: 260530-rtmf-recency-aware-listing
**Intake**: [intake.md](intake.md)
**Spec**: [spec.md](spec.md)

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- Stale-worktree detection / cleanup hints — deferred to the follow-up change `260530-5fyu-stale-worktree-hints`.
- A configurable recency signal (git commit date, reflog, HEAD mtime) — this change uses worktree-directory mtime only.
- Changing the recency signal currently used by `wt open` / `wt delete` for the *default selection* — that behavior is preserved; only menu *ordering* changes.
- A `wt prune` command or `wt delete --stale` selector.

## Recency Signal: Shared Helper

### Requirement: Single recency definition

The package `internal/worktree` SHALL expose exactly one recency function, used by every consumer (`wt list`, `wt open`, `wt delete`). The signal SHALL be the modification time (`mtime`) of the worktree's working-directory root, obtained via `os.Stat`. When the path cannot be stat'd (vanished worktree, permissions error), the function SHALL return the zero `time.Time` rather than an error.

The function signature SHALL be:

```go
// RecencyOf returns the recency signal for a worktree: the mtime of its
// working-directory root. Returns the zero time.Time if the path cannot be stat'd.
func RecencyOf(path string) time.Time
```

<!-- clarified: parameter is the directory path (string), not worktree.Info. Codebase check (worktree.go:14) confirms the three consumers do NOT hold a `worktree.Info` value at the recency call site: `cmd/wt/list.go` uses its own `listEntry`/`rawEntry` types built from the cmd-local `listWorktreeEntries()`, and `open.go`/`delete.go` build local `wtOption` structs from the same `rawEntry`. None route through `worktree.List()`/`Info`. Since `RecencyOf` only needs the path to `os.Stat`, taking a `string` lets all three callers pass `entry.Path` / `e.path` directly without an `Info` adapter, keeping the helper consumable everywhere (Constitution V) and avoiding a fabricated cross-package dependency on `Info`. -->

<!-- clarified: mtime resolution — the existing inline loops compare ModTime().Unix() (whole-second granularity, strict `>` first-wins on equal seconds). RecencyOf SHALL return the full-precision time.Time from os.Stat ModTime() (sub-second), and ordering ties are resolved by the Name tie-break below rather than by porcelain position. This is behavior-preserving for the default selection in all realistic cases (distinct mtimes) and only differs when two worktrees share an identical mtime, where the deterministic Name tie-break is an improvement over the previous non-deterministic first-wins. -->


#### Scenario: recency of an existing worktree
- **GIVEN** a worktree directory path that exists on disk
- **WHEN** `RecencyOf(path)` is called
- **THEN** it returns the directory's `os.Stat` `ModTime()`

#### Scenario: recency of a vanished worktree
- **GIVEN** a worktree directory path that no longer exists on disk
- **WHEN** `RecencyOf(path)` is called
- **THEN** it returns the zero `time.Time` and does not error or panic

### Requirement: Shared ordering comparator

`internal/worktree` SHALL expose a single recency comparator usable by all three consumers. Because the consumers hold heterogeneous types (`cmd/wt/list.go`'s `listEntry`, `open.go`/`delete.go`'s local `wtOption`), the comparator SHALL be expressed over the recency signal itself rather than over a concrete struct — i.e. a `less`/`compare` function (or `sort.Slice` helper) keyed on `(RecencyOf(path), Name)`, so each caller adapts its own slice via the key. It orders by recency most-recent first; ties (equal mtime, including two zero-time entries) SHALL be broken deterministically by worktree `Name` (ascending) so output is stable across runs. The main worktree SHALL be excluded from recency reordering by callers that pin it (see list/menu requirements) — the comparator itself operates on whatever slice it is given.
<!-- clarified: comparator keyed on (RecencyOf(path), Name) rather than typed on worktree.Info — same codebase-reality rationale as RecencyOf above. The three consumers each build their own slice from the cmd-local rawEntry; a struct-typed comparator would force an Info adapter none of them currently produce. A key-based less/compare helper (e.g. a func returning recency+name, or a sort.Slice closure factory) is consumable by all three without conversion. -->

The comparator key SHALL be derived from each entry's directory `Path` (the field every consumer struct exposes) and `Name`; no consumer is required to convert to `worktree.Info`.

#### Scenario: newest-first ordering
- **GIVEN** worktrees with mtimes t1 < t2 < t3
- **WHEN** the recency comparator sorts them
- **THEN** the order is [t3, t2, t1]

#### Scenario: deterministic tie-break
- **GIVEN** two worktrees `bravo` and `alpha` with identical mtime
- **WHEN** the recency comparator sorts them
- **THEN** `alpha` precedes `bravo` (Name ascending tie-break)

### Requirement: Open and delete menus consume the shared helper

`wt open` (`src/cmd/wt/open.go`) and `wt delete` (`src/cmd/wt/delete.go`) SHALL replace their existing inline `os.Stat`/`ModTime` loops with calls to the shared `RecencyOf` / comparator. The refactor SHALL be behavior-preserving for the *default selection*: the worktree pre-selected as the menu default MUST remain the most-recent one, identical to today. The menus' item *ordering* changes per the next requirement.

#### Scenario: default selection unchanged after refactor
- **GIVEN** a set of worktrees where worktree X has the newest mtime
- **WHEN** the `open` (or `delete`) menu is built using the shared helper
- **THEN** worktree X is still the pre-selected default

## Interactive Menus: Recency Ordering

### Requirement: Menus list non-main worktrees newest-first

The `wt open` and `wt delete` interactive selection menus SHALL list non-main worktrees in recency order (most-recent first) via the shared comparator, replacing the previous behavior where items appeared in porcelain order and only the newest was highlighted. The newest worktree SHALL therefore appear at the top of the menu and remain the default selection.

#### Scenario: newest at top of open menu
- **GIVEN** worktrees `old` (mtime t1) and `new` (mtime t2), t1 < t2
- **WHEN** `wt open` shows the selection menu
- **THEN** `new` is listed first and is the default selection
- **AND** `old` is listed after it

#### Scenario: delete menu mirrors open ordering
- **GIVEN** the same worktrees
- **WHEN** `wt delete` shows the selection menu
- **THEN** the ordering is identical to the `wt open` menu (newest-first)

## `wt list`: Sorting

### Requirement: `--sort` flag

`wt list` SHALL accept a `--sort` flag with the values `recent`, `name`, and `branch`. `recent` orders non-main worktrees newest-first via the shared comparator; `name` orders by worktree `Name` ascending; `branch` orders by `Branch` ref ascending. An unrecognized value SHALL exit `wt.ExitInvalidArgs` with a message naming the accepted values.

#### Scenario: explicit recency sort
- **GIVEN** worktrees with distinct mtimes
- **WHEN** `wt list --sort=recent` runs
- **THEN** non-main worktrees are printed newest-first

#### Scenario: explicit name sort
- **GIVEN** worktrees `charlie`, `alpha`, `bravo`
- **WHEN** `wt list --sort=name` runs
- **THEN** non-main worktrees are printed `alpha`, `bravo`, `charlie`

#### Scenario: invalid sort value
- **GIVEN** any repository
- **WHEN** `wt list --sort=bogus` runs
- **THEN** the command exits `ExitInvalidArgs` with a message listing `recent`, `name`, `branch`

### Requirement: Main worktree pinned first under all sort modes

Regardless of `--sort` value or default mode, the main worktree SHALL occupy the first output row. Only non-main worktrees are reordered by the chosen sort. This matches the `git worktree list --porcelain` convention and the existing `IsMain` semantics.

#### Scenario: main stays first under recency sort
- **GIVEN** a main worktree whose mtime is older than several non-main worktrees
- **WHEN** `wt list --sort=recent` runs
- **THEN** the main worktree is still the first row
- **AND** non-main worktrees follow it newest-first

### Requirement: Audience-split default ordering

When `--sort` is NOT supplied, the default order depends on output mode, decided **purely by flags** (no runtime `isatty`/terminal detection):

- Human output — `wt list` with no `--json` and no `--non-interactive` — SHALL default to `recent` ordering.
- `wt list --json` SHALL default to stable `name` ordering.
- `wt list --non-interactive` SHALL default to stable `name` ordering.

An explicit `--sort` value SHALL override the default in any mode (including `--json` and `--non-interactive`). This preserves deterministic machine-readable output (Constitution VI) while giving humans recency by default, mirroring the opt-in, JSON-aware design of `--status`.

<!-- clarified: the human-vs-machine signal is flag-based (absence of both --json and --non-interactive), NOT real TTY detection. Codebase check confirms there is no isatty/term.IsTerminal mechanism anywhere in src/ and no terminal-detection dependency in go.mod; intake "Dependencies: none new" forbids adding one. So "Human/TTY output" means "neither --json nor --non-interactive supplied" — this change SHALL NOT add stdout-fd terminal detection. Scenarios below mention "TTY stdout" only as the human invocation context; they do not require an isatty probe. (Constitution VI's "when stdout is not a TTY, degrade gracefully" remains a SHOULD and is out of scope for this ordering change — a piped `wt list | cat` without --non-interactive still gets recency, which is the accepted tradeoff of the no-new-dependency constraint.) -->

> Note on `--non-interactive`: `wt list` does not currently define a `--non-interactive` flag (confirmed at `src/cmd/wt/list.go` — only `--path`, `--json`, `--status` exist; `create.go` and `delete.go` do define `--non-interactive`). This change SHALL add a `--non-interactive` BoolVar to `wt list`, following the existing `cmd.Flags().BoolVar(&nonInteractive, "non-interactive", ...)` pattern in those siblings, solely to control default ordering; it has no other effect on `list`. [NEEDS CLARIFICATION resolved: add the flag to list for ordering-determinism parity.]

#### Scenario: human default is recency
- **GIVEN** worktrees with distinct mtimes and a TTY stdout
- **WHEN** `wt list` runs with no sort flag
- **THEN** non-main worktrees are printed newest-first

#### Scenario: JSON default is stable name order
- **GIVEN** the same worktrees
- **WHEN** `wt list --json` runs with no sort flag
- **THEN** non-main worktrees appear in `name`-ascending order, independent of mtime

#### Scenario: explicit sort overrides JSON default
- **GIVEN** the same worktrees
- **WHEN** `wt list --json --sort=recent` runs
- **THEN** non-main worktrees appear newest-first in the JSON array

### Requirement: `--sort` is mutually exclusive with `--path`

`--path` is a single-worktree lookup mode for which ordering is meaningless. `wt list --path <name> --sort=<any>` SHALL exit `wt.ExitInvalidArgs` with stderr containing "mutually exclusive", following the identical pattern as the existing `--path`/`--json` and `--path`/`--status` checks in `listCmd` (`src/cmd/wt/list.go`). It MUST NOT silently ignore `--sort`.

#### Scenario: sort with path is rejected
- **GIVEN** any repository
- **WHEN** `wt list --path foo --sort=recent` runs
- **THEN** the command exits `ExitInvalidArgs`
- **AND** stderr contains "--path and --sort are mutually exclusive"

## `wt list --status`: Last-Active Column

### Requirement: `last_active` field on list entries

`listEntry` (`src/cmd/wt/list.go`) SHALL gain an opt-in recency field following the existing `*bool`/`*int` pointer pattern used by `Dirty` and `Unpushed`:

```go
LastActive *time.Time `json:"last_active,omitempty"`
```

In default mode (no `--status`) the pointer SHALL remain nil and the JSON key SHALL be omitted. Under `--status`, every entry SHALL have a non-nil `LastActive` (set during enrichment, pre-allocated like `Dirty`/`Unpushed` so vanished worktrees still emit the key). A vanished worktree's `LastActive` SHALL be the zero `time.Time`.

#### Scenario: last_active omitted in default mode
- **GIVEN** any worktrees
- **WHEN** `wt list --json` runs (no `--status`)
- **THEN** no object contains a `last_active` key

#### Scenario: last_active present under --status
- **GIVEN** any worktrees
- **WHEN** `wt list --status --json` runs
- **THEN** every object contains a `last_active` key (RFC3339 timestamp)

### Requirement: Human-readable relative time column

Under `--status`, human/TTY output SHALL render a relative "last active" value per worktree (e.g. `2h ago`, `3d ago`, `just now`). The relative formatting applies to human output only; JSON SHALL emit the raw timestamp.

#### Scenario: relative time in status output
- **GIVEN** a worktree whose directory mtime is ~2 hours ago and a TTY stdout
- **WHEN** `wt list --status` runs
- **THEN** that worktree's row shows a relative time such as `2h ago`

## Memory & Spec Cross-References

### Requirement: Preserve existing list-status contract invariants

This change SHALL preserve all invariants in `docs/memory/wt-cli/list-status-contract.md`: default mode stays enrichment-free (one `git` subprocess), `Dirty`/`Unpushed` pointer semantics unchanged, worker-pool ordering preserved by indexed writes, the `--path`/`--status` and `--path`/`--json` mutex checks unchanged. The new `last_active` field SHALL be computed within the existing `listEntriesEnriched` enrichment pass and MUST NOT add a per-worktree `git` subprocess to either mode.
<!-- clarified: "via os.Stat, which the enrichment path may already perform" → confirmed. listEntriesEnriched (src/cmd/wt/list.go:403) already calls os.Stat(r.path) per worktree to gate goroutine spawn for vanished worktrees. last_active SHALL be sourced from RecencyOf(r.path) (its own os.Stat) or from reusing that existing stat result; either way no additional git subprocess is introduced. The pre-allocation must mirror Dirty/Unpushed: set LastActive to a non-nil pointer (zero time.Time) BEFORE the stat-gate `continue`, so vanished worktrees still emit the key (consistent with list-status-contract.md "Pre-allocate pointers BEFORE the stat check"). -->
<!-- clarified: LastActive of a vanished worktree = zero time.Time, serialized in JSON as "0001-01-01T00:00:00Z" (Go's time.Time zero value under RFC3339). This is the established present-but-uncomputed signal, analogous to dirty:false/unpushed:0 for entries whose goroutine never ran. -->
<!-- clarified: ordering vs enrichment are independent passes. The chosen --sort/default order applies to the final entries slice AFTER enrichment writes complete (or to the basic slice in default mode). The worker pool's indexed-write ordering invariant is about not letting parallelism reorder rows relative to the input slice; sorting is a separate, deterministic post-step. No conflict. -->

The new `last_active` field SHALL be computed via `os.Stat` only — never a `git` call — and only under `--status`.

#### Scenario: default mode remains single-subprocess
- **GIVEN** a repository with several worktrees
- **WHEN** `wt list` (default, no `--status`) runs
- **THEN** exactly one `git` subprocess (`git worktree list --porcelain`) is invoked
- **AND** no `last_active`, `dirty`, or `unpushed` enrichment occurs

#### Scenario: status ordering still preserved by indexed writes
- **GIVEN** 5 worktrees enriched in parallel under `--status`
- **WHEN** the output rows are produced
- **THEN** parallelism does not reorder rows relative to the chosen sort order

## Design Decisions

1. **One `RecencyOf` + comparator in `internal/worktree`**: Consolidates the duplicated inline mtime loops in `open.go` and `delete.go` into a single definition consumed everywhere.
   - *Why*: Eliminates drift between the two menus and gives `list` sorting the same definition of "recent". Honors Constitution V (non-trivial logic lives under `internal/worktree`).
   - *Rejected*: Leaving the loops inline and adding a third copy in `list.go` — triples the duplication and invites three divergent definitions of recency.

2. **Worktree-directory mtime as the signal**: Free `os.Stat`, already implemented.
   - *Why*: Zero new git subprocesses; preserves the existing menu default-selection behavior exactly.
   - *Rejected*: git commit date / reflog / HEAD-file mtime — each costs a subprocess or extra complexity; the noisiness of dir-mtime is acceptable for ordering and is explicitly deferred as a concern for the stale-hints follow-up.

3. **Audience-split default (TTY recency / JSON+non-interactive stable)**: Matches the `--status` opt-in, JSON-aware precedent.
   - *Why*: Constitution VI — deterministic scriptable output. fab-kit operators parse `wt list`/`--json`; a recency-shuffling default would make their output non-deterministic.
   - *Rejected*: Recency default in all modes (breaks machine parsers); opt-in-only with no human default (loses the ergonomic win the change is for).

4. **`--sort` ⇄ `--path` mutually exclusive via `ExitInvalidArgs`**: Reuses the exact established mutex pattern (`--path`/`--json`, `--path`/`--status`).
   - *Why*: Consistency with the existing two checks in `listCmd`; Constitution III typed exit codes; no silent no-op.
   - *Rejected*: Ignoring `--sort` silently (footgun); a bespoke error code (inconsistent with siblings).

5. **`LastActive *time.Time` opt-in pointer**: Same shape as `Dirty *bool` / `Unpushed *int`.
   - *Why*: Distinguishes "not computed" (nil → key omitted) from "computed" (non-nil) — the established contract in `list-status-contract.md`.
   - *Rejected*: Plain `time.Time` with `omitempty` (zero time would be indistinguishable from uncomputed); a separate struct (rejected already for `Dirty`/`Unpushed`).


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
