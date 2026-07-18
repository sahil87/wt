# Plan: Include Main Worktree in `wt go` Menu and Name Resolution

**Change**: 260718-daqj-go-include-main-worktree
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. The change lives at two shared seams in
     src/cmd/wt/open.go (selectWorktree, resolveWorktreeByName) plus caller
     simplifications, with spec + memory amendments. -->

### wt-cli: Menu includes the main worktree pinned to row 1

#### R1: `selectWorktree` includes main, pinned first, rendered `main (<branch>)`

The shared `selectWorktree` helper (`src/cmd/wt/open.go`) SHALL include the main
worktree as a selectable menu row, pinned to row 1, rendered `main (<branch>)`
via the existing `getBranchForPath`. The `ctx.RepoRoot` skip filter SHALL be
dropped. Non-main worktrees SHALL keep their newest-first `wt.SortByRecency`
ordering **below** the pinned main row (main is pinned outside the recency
ordering, mirroring `wt list`'s `sortEntries` pin-first convention). The main
row's returned `name` SHALL be the stable key `"main"`.

- **GIVEN** a repo with a main worktree and one or more non-main worktrees
- **WHEN** the no-arg `wt go` menu (or `wt open` no-arg main-repo menu, or `wt open --select`) renders
- **THEN** row 1 is `main (<branch>)` and the non-main worktrees follow newest-first below it
- **AND** selecting row 1 returns `(path=<mainPath>, name="main")`

#### R2: Pre-selected default stays the newest non-main worktree

The pre-selected menu default SHALL remain the newest *worktree* (not main),
preserving enter-key muscle memory. With main pinned as row 1: `defaultIdx = 2`
when at least one non-main worktree exists (main row 1, newest worktree row 2);
`defaultIdx = 1` when main is the only row.

- **GIVEN** a repo with main plus ≥1 non-main worktree
- **WHEN** the menu renders
- **THEN** the newest non-main worktree (row 2) carries the `(default)` marker, not main (row 1)
- **GIVEN** a repo with only the main worktree
- **WHEN** the menu renders
- **THEN** the single `main` row (row 1) is the default

#### R3: The "No worktrees found." path becomes the one-row menu

With main always present in-repo, the previous `No worktrees found.` /
`noWorktrees=true` branch SHALL become unreachable and SHALL be removed. When a
repo has no non-main worktrees, `selectWorktree` SHALL show the one-row menu
(just `main (<branch>)`) instead. The `noWorktrees` return value and the
`No worktrees found.` message SHALL be retired, and the three callers'
`if !noWorktrees` guards SHALL be simplified accordingly.

- **GIVEN** a repo with only the main worktree (no non-main worktrees)
- **WHEN** no-arg `wt go` runs
- **THEN** a one-row menu showing `main (<branch>)` is presented (no `No worktrees found.` line)
- **AND** Cancel still prints `Cancelled.` and exits 0; the navigation confirmation is still absent until a selection is made

### wt-cli: Name resolution learns a stable `main` key

#### R4: `resolveWorktreeByName` resolves `main` with exact-basename precedence

`resolveWorktreeByName` (`src/cmd/wt/open.go`) SHALL, after the existing
exact-basename loop finds no match, resolve `main` (case-insensitive) to the
porcelain-first entry's path (`entries[0].path`, always the main worktree). This
fixes `wt go main`, `wt open main`, and `wt open --select main` in one place.
Exact-basename match SHALL take precedence (a worktree directory literally named
`main` keeps today's behavior). The existing accidental repo-dir-basename
resolution SHALL be left unchanged (the `main` key is purely additive). Error
mapping is untouched: `errWorktreeNotFound` → `ExitGeneralError` (1); git-list
failure → `ExitGitError` (3).

- **GIVEN** a repo with a main worktree
- **WHEN** `wt go main` runs (no worktree dir literally named `main`)
- **THEN** it resolves to the main worktree path and navigates there (exit 0)
- **GIVEN** a repo containing a worktree directory literally named `main`
- **WHEN** `wt go main` runs
- **THEN** it resolves to that worktree (exact-basename precedence), not the repo root
- **GIVEN** a repo with a main worktree
- **WHEN** `wt go no-such-name` runs
- **THEN** it exits `ExitGeneralError` (1) with the "not found" message

### wt-cli: Non-negotiable constraints preserved

#### R5: `wt go` stdout machine contract and `selectWorktree` single-source-of-truth are preserved

`wt go`'s stdout machine contract SHALL be unchanged: bare resolved path as the
last stdout line; `WT_CD_FILE` mode `0600`/truncate-on-write; navigation
confirmation on stderr; `navigateTo` (`src/cmd/wt/go.go`) not modified. The
shared `selectWorktree` helper SHALL NOT fork per-caller behavior — all three
callers get the identical row set; only the `prompt` string differs. No new
`internal/` business rule (Constitution V): the change composes existing exported
helpers. Exit codes, `--non-interactive` refusal, non-TTY fallback, and the
`MenuSession` single-reader contract are untouched.

- **GIVEN** `wt go main` (or any resolved selection)
- **WHEN** it navigates
- **THEN** stdout's last line is exactly the bare resolved path; the `→ repo / worktree (branch)` confirmation is on stderr only
- **AND** `wt delete`'s main exclusion at all its list sites is untouched

### Non-Goals

- **`wt delete`'s main exclusion is untouched** at all its list sites (selection map, `--all`, `--stale`). Out of scope (Assumption 8).
- **No wiring-in of `internal/worktree/worktree.go`'s `List`/`FindByName`** (the parallel stable-key API with zero `cmd/` callers). The `cmd/`-local resolver/helper is patched per the existing pattern; the unused internal API is a future reconciliation candidate (Assumption 7).
- No menu-default-to-main (rejected), no per-caller filtering fork (rejected), no new flags, no new env vars.

### Design Decisions

1. **Pin main via a partition, not a sort key**: build the main `wtOption` from the porcelain-first entry (`entries[0]`), prepend it after sorting the non-main slice — mirroring `list.go`'s `sortEntries` (partition out row 0, reorder the rest). *Why*: keeps main pinned outside the recency ordering exactly as `wt list` does; `SortByRecency` still has one call site over the non-main slice only. *Rejected*: giving main an artificial max-recency so the sort floats it first (couples the pin to the comparator, fragile).
2. **`main` key resolves to `entries[0].path`**: the porcelain-first entry is always the main worktree (confirmed: `list.go` sets `mainPath = raw[0].path`). *Why*: no extra git call, matches `buildBaseEntry`'s existing convention. *Rejected*: a dedicated "find main" git call (redundant — porcelain order already encodes it).
3. **Retire `noWorktrees` entirely** rather than keep a defensive dead path (Assumption 5). *Why*: in a validated git repo porcelain always yields ≥1 entry, so the branch is unreachable; code-quality bars dead code; trivially reversible.

## Tasks

### Phase 2: Core Implementation

- [x] T001 In `src/cmd/wt/open.go` `selectWorktree`: drop the `e.path == ctx.RepoRoot` skip; build the non-main `wtOption` slice (all entries except the porcelain-first/main), sort it newest-first via `wt.SortByRecency`, then prepend a pinned `wtOption{path: entries[0].path, name: "main"}` (rendered `main (<branch>)`). Set `defaultIdx = 2` when ≥1 non-main worktree exists, else `1`. Remove the `noWorktrees` return value and the `No worktrees found.` early-return (retire the empty-options branch). Update the doc comment. <!-- R1 R2 R3 R5 -->
- [x] T002 In `src/cmd/wt/open.go` `resolveWorktreeByName`: after the exact-basename loop, add `if strings.EqualFold(name, "main") && len(entries) > 0 { return entries[0].path, nil }` before the `errWorktreeNotFound` return. <!-- R4 -->

### Phase 3: Integration & Edge Cases (caller simplification)

- [x] T003 In `src/cmd/wt/open.go`: update the three `selectWorktree` call sites (`openGo`, `selectAndOpen`) to the new signature `(path, name, cancelled, err)` — drop `noWorktrees` and the `if !noWorktrees` guards so Cancel always prints `Cancelled.`; keep the `Cancelled.` line and exit 0 on cancel. <!-- R3 R5 -->
- [x] T004 In `src/cmd/wt/go.go`: update the `selectWorktree` call site to the new 4-value signature — drop `noWorktrees` and the `if !noWorktrees` guard around the `Cancelled.` line. `navigateTo` is NOT modified. <!-- R3 R5 -->

### Phase 3b: Tests

- [x] T005 In `src/cmd/wt/go_test.go`: rewrite `TestGo_NoWorktrees_NoConfirmation` to assert the one-row menu behavior (menu shows `main (`, no `No worktrees found.`, confirmation arrow absent until selection). Add `wt go main` resolution coverage (a `TestGo_MainKey_*` test writing `WT_CD_FILE`+stdout to the repo root, and case-insensitive `MAIN`). <!-- R3 R4 -->
- [x] T006 In `src/cmd/wt/open_test.go`: update `TestOpen_MenuOrdersNewestFirst` for the pinned main row (main is row 1, newest non-main default marker unchanged on the newest worktree). Add `wt open main` name-resolution coverage if not covered by go_test. <!-- R1 R2 R4 -->
- [x] T007 In `src/cmd/wt/integration_test.go`: update `TestIntegration_Go_MenuOrdersNewestFirst` for the pinned main row; add an end-to-end `wt go main` from a sibling worktree resolving to the repo root. <!-- R1 R2 R4 -->

### Phase 4: Docs

- [x] T008 In `docs/specs/cli-surface.md`: amend the `## wt go [name]` no-arg bullet (menu contents/ordering/default — main pinned row 1, newest-first below, newest pre-selected) and name resolution (`main` resolves to the main worktree); amend the `## wt open [name|path]` "Omitted, called from the main repo" bullet and name-resolution note with the same main-row / `main`-key semantics. Also add the stable `main` key to `docs/specs/launcher-contract.md`'s `<name>` resolution row. <!-- R1 R2 R4 -->

## Execution Order

- T001 and T002 are independent (same file, different functions) but T003/T004 depend on T001's signature change.
- T005-T007 (tests) depend on T001-T004. T008 (docs) is independent of code.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `selectWorktree` includes the main worktree as row 1 rendered `main (<branch>)`; the `ctx.RepoRoot` filter is gone; non-main worktrees follow newest-first; the main row returns `name == "main"`.
- [x] A-002 R4: `resolveWorktreeByName` resolves `main` (case-insensitive) to `entries[0].path` after the exact-basename loop; `wt go main` / `wt open main` / `wt open --select main` all navigate to the main worktree.

### Behavioral Correctness

- [x] A-003 R2: With ≥1 non-main worktree, `defaultIdx = 2` (newest non-main worktree carries `(default)`, not main); with only main present, `defaultIdx = 1`.
- [x] A-004 R4: Exact-basename precedence holds — a worktree directory literally named `main` still resolves to that worktree, not the repo root; the accidental repo-dir-basename resolution is unchanged.
- [x] A-005 R3: The `noWorktrees` return value and `No worktrees found.` message are removed; the empty-repo case now shows the one-row `main` menu; the three callers' `if !noWorktrees` guards are simplified so Cancel always prints `Cancelled.` and exits 0.

### Scenario Coverage

- [x] A-006 R1: `TestOpen_MenuOrdersNewestFirst` and `TestIntegration_Go_MenuOrdersNewestFirst` assert the pinned main row plus unchanged non-main newest-first ordering.
- [x] A-007 R4: A `wt go main` resolution test (unit + end-to-end from a sibling worktree) verifies navigation to the repo root, including case-insensitivity.
- [x] A-008 R3: `TestGo_NoWorktrees_NoConfirmation` (rewritten) verifies the one-row menu and the absence of the confirmation arrow until selection.

### Edge Cases & Error Handling

- [x] A-009 R4: `wt go <unknown>` still exits `ExitGeneralError` (1) with the "not found" message; a git-list failure still exits `ExitGitError` (3).
- [x] A-010 R5: `wt go`'s stdout stays exactly the bare resolved path (confirmation on stderr); `WT_CD_FILE` mode `0600`/truncate semantics unchanged; `navigateTo` untouched.

### Code Quality

- [x] A-011 Pattern consistency: The main-pin follows `wt list`'s `sortEntries` partition-first convention; the `main`-key resolution follows `buildBaseEntry`'s `entries[0]`-is-main convention; no new `internal/` business rule (Constitution V).
- [x] A-012 No unnecessary duplication: `getBranchForPath` and `SortByRecency` are reused (no new recency or branch logic); the shared `selectWorktree` helper is not forked per-caller.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

- `src/cmd/wt/open.go:279` — `resolveWorktreeByName`'s `ctx *wt.RepoContext` parameter: unused in the function body (it was already unused pre-change; the rework's removal of `selectWorktree`'s `ctx` makes it the selection/resolution seam's last vestigial `ctx`). Deletable along with the argument at its 3 call sites (`go.go:61`, `open.go:91`, `open.go:190`). *(Prior cycle's `selectWorktree` `ctx`-param candidate was resolved by the rework — the parameter is removed.)*
- `src/internal/worktree/worktree.go:23,69` — `List`/`FindByName`: the parallel stable-`main`-key API with zero `cmd/` callers; this change reimplemented its `main`-key semantics in `cmd/`'s `resolveWorktreeByName`, making the reuse-or-delete reconciliation (intake Assumption 7, deliberately deferred) more pressing.

## Assumptions

<!-- Carried from intake.md's SRAD assumptions (all Certain/Confident; 0 Unresolved).
     These are the decisions the plan implements. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Drop the `ctx.RepoRoot` filter in shared `selectWorktree`; pin main as row 1 rendered `main (<branch>)`, across all three menu callers | Explicit discussion decision; single-source-of-truth invariant makes the `wt open` menus gaining the row intentional; copies `wt list`'s pin-first convention | S:90 R:75 A:90 D:90 |
| 2 | Confident | Pre-selected default stays the newest worktree (`defaultIdx = 2` with main + ≥1 non-main; `1` when main only), NOT main | Preserves enter-key muscle memory (create → go → newest); genuinely arguable, main-as-default was the rejected alternative | S:80 R:85 A:70 D:55 |
| 3 | Certain | `resolveWorktreeByName` gains a stable `main` key resolving to the porcelain-first entry, exact-basename precedence; fixes `wt go/open/open --select main` in one place | Explicit discussion decision; matches `wt list`'s displayed name and the internal stable-key convention | S:90 R:80 A:90 D:85 |
| 4 | Certain | The in-repo "No worktrees found." outcome is replaced by the one-row menu (just main) | Explicit discussion decision; navigating to main is still meaningful | S:85 R:80 A:85 D:80 |
| 5 | Confident | Retire the `noWorktrees` return flag and `No worktrees found.` message entirely (simplify the three callers) rather than keep a defensive dead path | In a validated git repo porcelain always yields ≥1 entry so the branch is unreachable; code-quality bars dead code; trivially reversible | S:60 R:85 A:75 D:65 |
| 6 | Confident | The accidental repo-dir-basename resolution stays unchanged; the `main` key is purely additive | Implied by the exact-basename-precedence decision; removing it would be an unrelated breaking change | S:70 R:85 A:80 D:70 |
| 7 | Confident | Fix in `cmd/`'s existing resolver/helper; do NOT wire in `internal/worktree`'s parallel `List`/`FindByName` API — note it as a future reconciliation candidate | Constitution V keeps selection orchestration in `cmd/`; the internal API lacks the `errWorktreeNotFound` sentinel the exit-code mapping needs; minimal-diff | S:60 R:80 A:70 D:50 |
| 8 | Certain | `wt delete`'s main exclusion at all its list sites is untouched | Explicit discussion decision (correct as-is; deleting main must stay impossible) | S:95 R:90 A:95 D:95 |
| 9 | Certain | `wt go`'s stdout machine contract is unchanged; `navigateTo` untouched | Pinned non-negotiable constraint (launcher-contract §3, stdout/stderr discipline) | S:95 R:85 A:95 D:95 |
| 10 | Confident | `selectWorktree`'s `name` return for the main row is the stable key `"main"` | Consistent with `wt list` display and the internal stable-key convention; menu row text and returned name should agree | S:65 R:85 A:80 D:70 |

10 assumptions (5 certain, 5 confident, 0 tentative).
