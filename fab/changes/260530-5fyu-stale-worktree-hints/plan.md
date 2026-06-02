# Plan: Stale Worktree Hints

**Change**: 260530-5fyu-stale-worktree-hints
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Tasks

### Phase 1: Idle Predicate (internal/worktree)

- [x] T001 Add `IsIdle(recency, now time.Time, threshold time.Duration) bool` to a new `src/internal/worktree/idle.go` — strict `now.Sub(recency) > threshold` (R1, R4). Zero recency is idle by construction (R2). <!-- A-001 A-004 A-005 -->
- [x] T002 Add exported constant `DefaultIdleThreshold = 7 * 24 * time.Hour` in `src/internal/worktree/idle.go` (R3). <!-- A-002 -->
- [x] T003 Add `ParseIdleThreshold(s string) (time.Duration, error)` in `src/internal/worktree/idle.go`: empty → `DefaultIdleThreshold`; `Nd` form only; reject non-`d` suffix / non-integer / non-positive with an error naming the accepted form (R15). <!-- A-003 -->
- [x] T004 [P] Tests in `src/internal/worktree/idle_test.go`: idle boundary (just-under, just-over, exactly-at), zero-recency idle, ParseIdleThreshold valid `Nd` forms + rejects (banana, no-suffix, wrong unit, 0d, negative, empty number). <!-- A-006 A-013 -->

### Phase 2: wt list Idle Marker (cmd/wt/list.go)

- [x] T005 Add `Idle *bool` field with `json:"idle,omitempty"` to `listEntry` (R9). <!-- A-007 -->
- [x] T006 Add `populateIdle(entries, now)` called after `sortEntries` in `listCmd`: set `Idle` non-nil exactly when `LastActive` is non-nil; main worktree → `false`; reuse persisted `LastActive` (no new os.Stat, no git subprocess) (R6, R7, R9). <!-- A-007 A-008 A-009 -->
- [x] T007 In `handleFormattedOutput`, append ` ⚠ idle` to the Last Active cell in the 4-column recent human layout AND the 5-column `--status` layout when `e.Idle` is true; use "idle" wording (R5, R8). The 3-column name/branch modes show no marker and add no stat (R6). <!-- A-008 A-010 A-011 -->

- [x] T008 [P] Tests in `src/cmd/wt/list_test.go` using `os.Chtimes`: 4-col human idle marker (40d worktree marked, fresh not, main not), 5-col `--status` marker, name/branch modes no marker, JSON `idle` absent in default/`--json`/`--json --sort=recent`, JSON `idle` present+boolean under `--status` (true for old, false for main). <!-- A-013 -->

### Phase 3: wt delete Stale-Aware Menu + --stale Selector (cmd/wt/delete.go)

- [x] T009 In `handleDeleteMenu`: compute idleness per non-main option via `wt.IsIdle(wt.RecencyOf(path), now, DefaultIdleThreshold)`; annotate idle labels with trailing `, idle` (R10). <!-- A-014 -->
- [x] T010 Add an "All idle (N)" entry right after "All (N worktrees)" ONLY when N>=1 (R11, R12); route its selection through `handleDeleteMultiple` (R11). <!-- A-015 A-016 -->
- [x] T011 Shift `defaultIdx` 2→3 only when "All idle" is present; else stays 2; newest non-main worktree remains the pre-selected default (R13). <!-- A-017 -->
- [x] T012 Add the `--stale` flag with pflag `NoOptDefVal` (`7d`): bare `--stale`=default, `--stale=Nd` override; `=` required (R14). <!-- A-018 -->
- [x] T013 Add `handleDeleteStale`: resolve threshold via `ParseIdleThreshold`, select all non-main idle worktrees, route through `handleDeleteMultiple`; zero matches → print "No idle worktrees (threshold: Nd)." and exit `ExitSuccess` (R17). Main excluded (R21). <!-- A-019 A-022 A-023 -->
- [x] T014 Mutex checks in `RunE`: `--stale` + positional names → `ExitInvalidArgs` "mutually exclusive" (R16); `--stale` + `--delete-all` → `ExitInvalidArgs` "mutually exclusive" (R19). Invalid threshold → `ExitInvalidArgs` naming `Nd` form (R15). <!-- A-020 A-021 A-024 -->

- [x] T015 [P] Tests in `src/cmd/wt/delete_test.go` (controlled mtimes via `os.Chtimes`, non-interactive / EOF-cancel paths only — no real interactive menus/side effects): menu idle annotation + "All idle (N)" + defaultIdx shift; no "All idle" when none idle; `--stale --non-interactive` deletes idle subset; `--stale=Nd` override; zero-match message + exit 0; positional mutex; `--delete-all` mutex; invalid threshold; main never targeted. Adjust the pre-existing `TestDelete_MenuOrdersNewestFirst` fixtures to recent (non-idle) mtimes so it stays focused on ordering. <!-- A-013 A-025 -->

### Phase 4: Verification

- [x] T016 `gofmt -w` every touched .go file; verify `gofmt -l src/` empty, `go build ./...`, `go vet ./...` clean. <!-- A-012 -->

## Execution Order

- Phase 1 (T001–T003) blocks Phase 2 (T006) and Phase 3 (T009, T013, T014) — they consume `IsIdle`/`DefaultIdleThreshold`/`ParseIdleThreshold`.
- T005 blocks T006 (field must exist); T006 blocks T007 (renderer reads `Idle`).
- T009/T010 block T011 (defaultIdx depends on "All idle" presence).
- T012 blocks T013/T014 (flag must be registered).
- `[P]` test tasks (T004, T008, T015) run alongside their phase's implementation per test-alongside strategy.

## Acceptance

### Functional Completeness

- [x] A-001 Idle predicate (R1): `IsIdle(recency, now, threshold)` in `internal/worktree` returns true iff `now.Sub(recency) > threshold`; takes a `time.Time` value, performs no stat.
- [x] A-002 Default threshold (R3): exported `DefaultIdleThreshold = 7 * 24 * time.Hour` constant; not env/config configurable.
- [x] A-003 Threshold parser (R15): `ParseIdleThreshold` resolves empty → default, accepts `Nd`, rejects non-`d`/non-integer/non-positive with an error naming the `Nd` form.
- [x] A-004 Strict boundary (R4): age exactly equal to threshold is NOT idle (strict `>`).
- [x] A-005 Vanished worktree (R2): zero recency is idle against any positive threshold.
- [x] A-006 Boundary cases tested (R4): just-under, just-over, exactly-at, and zero-recency are covered by tests.
- [x] A-007 JSON `idle` field (R9): `listEntry.Idle *bool` with `json:"idle,omitempty"`, non-nil exactly when `LastActive` is non-nil, main → false.
- [x] A-008 No new stat/subprocess (R6): list idle determination reuses persisted `LastActive`; no new `os.Stat`, no git subprocess; single-subprocess default-mode contract preserved.
- [x] A-009 Main never idle (R7): main worktree's idle is always false/absent across layouts.

### Behavioral Correctness

- [x] A-010 Recent human marker (R5): 4-column recency view appends ` ⚠ idle` (wording "idle") to the Last Active cell of an idle non-main worktree; fresh worktrees show no marker.
- [x] A-011 Status human marker (R8): 5-column `--status` view shows the same marker on the Last Active cell using the enrichment `LastActive`.
- [x] A-014 Menu idle annotation (R10): each idle non-main option label gains a trailing `, idle`; menu ordering stays newest-first via `SortByRecency`.
- [x] A-015 All idle entry present (R11): when ≥1 worktree is idle, an "All idle (N)" entry sits immediately after "All (N worktrees)"; selecting it routes the idle subset through `handleDeleteMultiple`.
- [x] A-016 All idle entry hidden (R12): when no worktree is idle, no "All idle" entry is rendered (no "All idle (0)").
- [x] A-017 Default index shift (R13): `defaultIdx` is 3 only when "All idle" is present, else 2; the newest worktree is the pre-selected default.
- [x] A-018 `--stale` flag shape (R14): bare `--stale` uses 7d default via `NoOptDefVal`; `--stale=Nd` overrides; `=` required.
- [x] A-019 `--stale` selection (R17): selects all non-main idle worktrees against the resolved threshold and routes them through `handleDeleteMultiple`.

### Scenario Coverage

- [x] A-022 Threshold override (R14): `--stale=30d` uses a 30-day threshold for that invocation.
- [x] A-023 Empty state (R17): `--stale` with zero idle matches prints "No idle worktrees (threshold: Nd)." and exits `ExitSuccess`.
- [x] A-024 Mutex errors (R16, R19): `--stale <name>` and `--stale --delete-all` both exit `ExitInvalidArgs` with stderr containing "mutually exclusive".
- [x] A-025 Compose with existing flags (R18): `--stale` composes with `--non-interactive`/`--delete-branch`/`--delete-remote`/`--stash` via `handleDeleteMultiple` reuse.

### Edge Cases & Error Handling

- [x] A-012 Build/vet/gofmt clean: `go build ./...`, `go vet ./...`, and `gofmt -l src/` all clean.
- [x] A-013 Tests use controlled mtimes (`os.Chtimes`) and non-side-effecting paths (no real tmux windows/sessions/interactive menus); idle-boundary and menu/selector cases covered.
- [x] A-020 Safety invariant (R20): idleness is never the sole delete gate — every `--stale`/"All idle" removal still passes through `handleDeleteMultiple`'s per-worktree safety flow.
- [x] A-021 Main exclusion (R21): the `--stale` selector and `handleDeleteMenu` never select the main worktree even when its dir mtime is past the threshold.

### Code Quality

- [x] A-007Q Pattern consistency: new code follows surrounding naming/structure (pointer-field JSON shape, `ExitWithError` mutex idiom, named-constant precedent, `internal/worktree` boundary per Constitution V).
- [x] A-008Q No unnecessary duplication: reuses `RecencyOf`/`IsIdle`/`SortByRecency`/`handleDeleteMultiple`; no second recency or staleness signal introduced.
- [x] A-009Q Readability over cleverness: predicate and parser are small, focused functions; no god functions; no magic numbers (threshold is the named `DefaultIdleThreshold`).

## Notes

- Idle predicate, threshold constant, and parser all live in `src/internal/worktree/idle.go` per Constitution V (cmd/ only consumes).
- The signal is dir-mtime via the existing `RecencyOf`; framed as "idle/untouched on disk", not a verdict that work is dead (Design Decision 1, 4).
- `--stale` uses `NoOptDefVal = "7d"` (pflag forbids an empty `NoOptDefVal`); presence is detected via `cmd.Flags().Changed("stale")`. `ParseIdleThreshold("")` still resolves to default for robustness.

## Deletion Candidates

- None — this change adds new functionality (idle predicate, `--stale` selector, idle marker) without making existing code redundant. The flat `defaultIdx = 2` literal in `handleDeleteMenu` was replaced by the `firstWorktreeIdx` 2/3 computation rather than left dead, and no prior function, branch, or symbol became unused. (Note: `docs/memory/wt-cli/recency-ordering-contract.md:86` still documents the old flat `defaultIdx = 2`; that is a memory-drift item for the hydrate stage, not a code deletion.)
