# Plan: Stale Worktree Hints

**Change**: 260530-5fyu-stale-worktree-hints
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- **A dedicated `wt prune` command** — the `--stale` selector on the existing `wt delete` covers batch cleanup without a new command surface or a duplicated safety path.
- **A second/cleaner staleness signal** (last-commit-date, reflog, `HEAD` mtime) — this change reuses the single `RecencyOf` (dir-mtime) signal. A cleaner per-staleness signal is explicitly rejected for this change (see Design Decisions).
- **Auto-deletion** — idleness only ever *selects* candidates; every removal still runs through the existing per-worktree safety flow (uncommitted/unpushed prompts, rollback). No worktree is ever deleted solely because it is idle.
- **A configurable threshold via env var or config file** — the threshold is a built-in 7-day default, overridable only by the per-invocation `--stale=Nd` flag.
- **A non-day threshold unit** (hours, weeks) — `--stale` accepts only a day-suffixed integer (`Nd`).
- **Marking the main worktree idle** — the main worktree is never an idle candidate in any surface.

## worktree: Idle Predicate

The idle predicate is the single definition of "idle" shared by `wt list` and `wt delete`, built on the existing `RecencyOf` recency signal (`src/internal/worktree/recency.go`).

### Requirements

- **R1.** The package SHALL expose a single idle predicate in `src/internal/worktree/` (Constitution V) of the form `IsIdle(recency time.Time, now time.Time, threshold time.Duration) bool`, returning `true` when `now.Sub(recency) > threshold`. It SHALL take the recency time as a parameter (not a path) so callers reuse a recency value they have already computed — no new `os.Stat` is introduced by the predicate itself.
- **R2.** A zero `recency` (`time.Time{}`, the value `RecencyOf` returns for a vanished/unstattable worktree) SHALL be treated as **idle** when compared against any positive threshold — a worktree whose directory cannot be stat'd is, if anything, a stronger cleanup candidate, never a fresh one. (`now.Sub(zeroTime)` is an enormous positive duration, so this falls out of R1 naturally; it is called out as a deliberate, tested invariant.)
- **R3.** The default idle threshold SHALL be **7 days**, defined as an exported named constant `DefaultIdleThreshold = 7 * 24 * time.Hour` in `src/internal/worktree/` (mirroring the `maxListConcurrency` "named constant, not a knob" precedent). It SHALL NOT be configurable via environment variable or config file.
- **R4.** The boundary SHALL be strict: a worktree whose age is *exactly* the threshold is **not** idle; only `age > threshold` is idle.

#### Scenarios

```
GIVEN a worktree last touched 8 days ago and a 7-day threshold
WHEN IsIdle(recency, now, threshold) is evaluated
THEN it returns true (8d > 7d)

GIVEN a worktree last touched 6 days ago and a 7-day threshold
WHEN IsIdle is evaluated
THEN it returns false (6d < 7d)

GIVEN a worktree whose age equals exactly the threshold
WHEN IsIdle is evaluated
THEN it returns false (strict >, not >=)

GIVEN a vanished worktree (RecencyOf returned the zero time)
WHEN IsIdle(zeroTime, now, 7d) is evaluated
THEN it returns true (an unstattable worktree is an idle candidate)
```

## wt list: Idle Marker

`wt list` surfaces idleness as a non-destructive visual marker in its recency-ordered human view, reusing the `Last Active` column machinery added by `260601-73cv`.

### Requirements

- **R5.** In the recency-ordered human layout (the 4-column `Name / Branch / Last Active / Path` view rendered when `recentLayout == true`), a non-main worktree that is idle per R1 SHALL be annotated with a trailing ` ⚠ idle` marker on its `Last Active` cell. The marker text uses the word **"idle"**, not "stale" — the user-facing framing is "untouched on disk," not a verdict that the work is dead (see Design Decisions).
- **R6.** The idle determination for display SHALL reuse the recency key already persisted into `entries[i].LastActive` by `sortEntries` on the human path — it SHALL NOT introduce any new `os.Stat` or any `git` subprocess. The single-subprocess default-mode contract (one `git worktree list --porcelain`) is preserved; `name`/`branch` human modes (3-column) still perform zero per-worktree `os.Stat` and SHALL NOT show the marker.
- **R7.** The main worktree SHALL NEVER be annotated idle, in any layout.
- **R8.** In `--status` mode (5-column), the idle marker MAY also be shown on the `Last Active` cell using the same predicate and the enrichment-computed `LastActive`. This is consistent because `--status` already populates `LastActive`; it adds no new stat. (Both human layouts that display `Last Active` show the marker; the 3-column `name`/`branch` modes do not.)
- **R9.** JSON output SHALL gain an **additive** opt-in field `idle *bool` with `json:"idle,omitempty"`, following the established pointer-field pattern (`Dirty *bool`, `Unpushed *int`, `LastActive *time.Time`). The field SHALL be non-nil (key present) exactly when `LastActive` is non-nil — i.e. under `--status` (and never on the plain `--json` / `--json --sort=recent` path, where `LastActive` stays nil and `omitempty` omits the key). Default JSON output remains byte-for-byte stable per Change A's contract (Constitution VI). The main worktree's `idle` SHALL be `false` whenever the field is present.

#### Scenarios

```
GIVEN three non-main worktrees, one untouched for 40 days, under bare `wt list`
WHEN the recency-ordered human table renders
THEN the 40-day worktree's Last Active cell reads "40d ago ⚠ idle"
AND the other two (recent) show no marker
AND the main worktree (pinned first) shows no marker

GIVEN `wt list --sort=name` (3-column human mode)
WHEN the table renders
THEN no idle marker is shown for any row (no per-worktree stat in this mode)

GIVEN `wt list --json` (default machine mode)
WHEN output is produced
THEN no "idle" key appears in any object (LastActive nil → omitempty)

GIVEN `wt list --status --json`
WHEN output is produced
THEN every object has an "idle" boolean key
AND a worktree older than the threshold has "idle": true
AND the main worktree has "idle": false
```

## wt delete: Stale-Aware Menu and `--stale` Selector

`wt delete` gains an idle-aware interactive menu and a `--stale` non-interactive selector. Both route every actual removal through the existing safety flow — idleness only pre-selects candidates.

### Requirements

#### Interactive menu

- **R10.** In `handleDeleteMenu`, each non-main worktree option that is idle per R1 SHALL be annotated in its menu label with a trailing `, idle` (e.g. `feature-x (feat-x) — 41d, idle`) so the user can see which candidates are idle before selecting. The menu continues to list non-main worktrees newest-first via `wt.SortByRecency` (unchanged ordering).
- **R11.** When at least one non-main worktree is idle, the menu SHALL include an **"All idle (N)"** entry, where N is the count of idle worktrees, positioned immediately after the existing "All (N worktrees)" entry. Selecting it SHALL route the idle subset through the existing `handleDeleteMultiple` flow (per-worktree stash/unpushed safety prompts, confirm, rollback, branch cleanup) — no new deletion code path.
- **R12.** When **no** non-main worktree is idle, the "All idle" entry SHALL be omitted entirely (no "All idle (0)" row).
- **R13.** The pre-selected default SHALL remain the newest non-main worktree. With the "All" entry at index 1 and (when present) "All idle" at index 2, the first worktree row — and thus `defaultIdx` — SHALL shift from 2 to 3 **only when the "All idle" entry is present**; when it is absent, `defaultIdx` remains 2. (This amends the recency-ordering contract's documented `defaultIdx = 2`.)

#### `--stale` flag

- **R14.** `wt delete` SHALL accept a `--stale` flag implemented as a single flag with pflag `NoOptDefVal`: bare `--stale` uses the 7-day default (R3); `--stale=Nd` overrides the threshold with a day-suffixed integer. The `=` form is REQUIRED for the value; `--stale 30d` (space-separated) is NOT supported because `wt delete` takes positional worktree names (`cobra.ArbitraryArgs`) and `30d` would be parsed as a worktree name.
- **R15.** The `--stale` value SHALL be parsed by a helper in `src/internal/worktree/` (Constitution V) that accepts a `Nd` string (e.g. `7d`, `30d`) and returns a `time.Duration`. An empty string (bare `--stale` via `NoOptDefVal`) SHALL resolve to `DefaultIdleThreshold`. A value with no `d` suffix, a non-integer, or a non-positive integer SHALL be rejected with `ExitInvalidArgs` and a message naming the accepted form (`Nd`, e.g. `30d`).
- **R16.** `--stale` SHALL be mutually exclusive with positional worktree-name arguments. `wt delete --stale <names...>` SHALL exit `ExitInvalidArgs` with stderr containing "mutually exclusive", matching the existing `--path`↔`--status` idiom in `wt list`. This converts the silent `--stale 30d` parse trap into a loud, recoverable error.
- **R17.** `--stale` SHALL select all non-main worktrees that are idle per R1 against the resolved threshold, then route them through the existing `handleDeleteMultiple` flow. When `--stale` matches zero worktrees, the command SHALL print an informational message (e.g. "No idle worktrees (threshold: Nd).") and exit `ExitSuccess` — it is not an error.
- **R18.** `--stale` SHALL compose with `--non-interactive`, `--delete-branch`, `--delete-remote`, and `--stash` exactly as positional/`--delete-all` deletion does today, since it reuses `handleDeleteMultiple`. In `--non-interactive` mode the existing per-worktree safety semantics (stash-or-discard default, unpushed handling) are unchanged.
- **R19.** `--stale` SHALL be mutually exclusive with `--delete-all` (`--delete-all` already targets every worktree; combining the two is contradictory). Exit `ExitInvalidArgs` with "mutually exclusive".

#### Safety invariant

- **R20.** Idleness SHALL NEVER be the sole gate for removal. Every worktree selected by `--stale` or "All idle" SHALL still pass through `handleDeleteMultiple`/`handleDeleteByName`, whose existing per-worktree handling of uncommitted changes (stash/discard) and unpushed commits is unchanged. mtime under-reporting is safe-by-direction: it hides genuinely-idle worktrees (they look fresh) rather than ever exposing an unsafe worktree as deletable without the safety flow.
- **R21.** The main worktree SHALL NEVER be an idle candidate — `handleDeleteMenu` and the `--stale` selector already exclude `ctx.RepoRoot`, so no special-casing is required, but the exclusion is a tested invariant.

#### Scenarios

```
GIVEN 5 non-main worktrees, 2 of them untouched > 7 days
WHEN `wt delete` opens the interactive menu
THEN the menu lists: "All (5 worktrees)", "All idle (2)", then the 5 worktrees newest-first
AND the 2 idle worktrees' labels end with ", idle"
AND the pre-selected default is the newest worktree row (defaultIdx = 3)

GIVEN no non-main worktree is idle
WHEN `wt delete` opens the menu
THEN there is no "All idle" entry
AND defaultIdx is 2 (unchanged from today)

GIVEN `wt delete --stale` with 2 worktrees older than 7 days
WHEN the command runs
THEN exactly those 2 worktrees are routed through the existing multi-delete flow
AND each still triggers its uncommitted/unpushed safety prompts before removal

GIVEN `wt delete --stale=30d`
WHEN the command runs
THEN the idle threshold is 30 days for this invocation

GIVEN `wt delete --stale feature-x`
WHEN the command runs
THEN it exits ExitInvalidArgs with "mutually exclusive" (no worktree named feature-x is deleted)

GIVEN `wt delete --stale=banana`
WHEN the command runs
THEN it exits ExitInvalidArgs naming the accepted Nd form

GIVEN `wt delete --stale` and no worktree is idle
WHEN the command runs
THEN it prints "No idle worktrees (threshold: 7d)." and exits 0
```

## Design Decisions

1. **Signal = dir-mtime (`RecencyOf`), framed as "idle/untouched on disk"**
   - *Why*: One signal definition across `list`/`open`/`delete`; zero new `git` subprocesses; reuses the recency key `sortEntries`/enrichment already computes. The delete menu only *offers* deletion — the existing per-worktree safety flow gates the actual removal — so mtime's tendency to *under-report* staleness (a `fab sync`/build touch makes an old worktree look fresh) is safe-by-direction.
   - *Rejected*: A cleaner per-staleness signal (last-commit-date / reflog / `HEAD` mtime) — costs a `git` subprocess per worktree and introduces a second, divergent signal definition, for accuracy the safety flow already backstops. Remains a future option if under-reporting proves painful.

2. **Predicate takes a recency `time.Time`, not a path**
   - *Why*: Every caller already holds a recency value (`LastActive` on the list path, a `RecencyOf` result on the delete path). A `time.Time` parameter lets them reuse it with no extra `os.Stat`, mirroring how `RecencyLess` is keyed on values rather than typed on a struct.
   - *Rejected*: `IsIdle(path string, ...)` — would re-stat paths the caller already stat'd, violating the cheap-path contract.

3. **`--stale` as a single `NoOptDefVal` flag with `=`-required value**
   - *Why*: Matches the user's `--stale` / `--stale=30d` shorthand in one flag. The `=`-required quirk is an accepted, documented tradeoff.
   - *Rejected*: Two flags (`--stale` + `--stale-after`) — cleaner parse but more surface than the user wanted. A bare-int value — less self-documenting than `Nd`, which matches the `Nd ago` display buckets users already see.
   - *Mitigation*: Because the space-separated form collides with positional args, R16 makes `--stale` + positionals a hard `ExitInvalidArgs` error rather than a silent mis-target.

4. **"idle" in user-facing copy, threshold as a named constant**
   - *Why*: "idle/untouched" states the fact (mtime age) without the unprovable verdict that "stale" implies; the `--stale` *flag name* is fine because a user typing it already intends cleanup. The 7-day threshold is a named constant, not an env/config knob — same "no premature configuration" stance as `maxListConcurrency`.
   - *Rejected*: "stale" as the display marker (overclaims); a configurable env/config threshold (premature surface).

## Clarifications

### Session 2026-06-01 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 4 | Confirmed | Settled in fab-discuss |
| 5 | Confirmed | Settled in fab-discuss |
| 6 | Confirmed | Settled in fab-discuss |
| 8 | Confirmed | Settled in fab-discuss |
| 9 | Confirmed | Settled in fab-discuss |
| 10 | Confirmed | Settled in fab-discuss |
| 11 | Confirmed | Settled in fab-discuss |


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
