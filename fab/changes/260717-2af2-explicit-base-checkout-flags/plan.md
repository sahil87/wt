# Plan: Explicit --base/--checkout Split for wt create

**Change**: 260717-2af2-explicit-base-checkout-flags
**Intake**: `intake.md`

## Requirements

### wt create: Branch-selection contract

#### R1: Positional `[branch]` is new-branch-only
`wt create <branch>` SHALL treat the positional argument as the name of a **new** branch to create (off `--base`, else HEAD). If `<branch>` already exists locally OR remotely, the command MUST fail with `ExitInvalidArgs` **before any worktree mutation**, pointing the user at `--checkout`.

- **GIVEN** a repo with no branch `foo` (local or remote)
- **WHEN** the user runs `wt create foo`
- **THEN** a new branch `foo` is created and a worktree is placed on it
- **AND** the exploratory bare-create path (no positional) is unchanged: a new branch named after the worktree name, `--base` honored.

- **GIVEN** a branch `foo` that exists locally (or remote-only)
- **WHEN** the user runs `wt create foo`
- **THEN** the command exits `ExitInvalidArgs` (2) with `Branch 'foo' already exists` / `The positional argument only creates new branches` / `To put a worktree on the existing branch: wt create --checkout foo`
- **AND** no worktree directory and no branch mutation is left behind.

#### R2: New `--checkout <branch>` flag checks out an existing branch
`wt create --checkout <branch>` SHALL place the new worktree on an **existing** branch: a local branch as-is, a remote-only branch fetched-then-checked-out. If the branch exists neither locally nor remotely, the command MUST fail with `ExitInvalidArgs`, pointing the user at the create-new form. The worktree name suggestion SHALL be `DeriveWorktreeName(branch)`.

- **GIVEN** a branch `feature/auth` that exists locally
- **WHEN** the user runs `wt create --checkout feature/auth`
- **THEN** the worktree is created on `feature/auth` (no `-b`), suggested name derived from the branch, with all surrounding phases (dirty-state menu, name prompt, init incl. the confirm, open menu, rollback, SIGINT, phase separators) identical to today's positional-existing path.

- **GIVEN** a branch that exists only on `origin`
- **WHEN** the user runs `wt create --checkout <branch>`
- **THEN** the branch is fetched via `FetchRemoteBranch` then checked out (today's behavior, unchanged).

- **GIVEN** a name that exists neither locally nor remotely
- **WHEN** the user runs `wt create --checkout missing`
- **THEN** the command exits `ExitInvalidArgs` (2) with `Branch 'missing' not found` / `--checkout requires an existing local or remote branch` / `To create a new branch: wt create missing [--base <ref>]`
- **AND** no worktree directory is left behind.

#### R3: Conflicting selectors are hard errors
`--checkout` combined with a positional branch argument, and `--checkout` combined with `--base`, SHALL each exit `ExitInvalidArgs`.

- **GIVEN** `--checkout` and a positional arg
- **WHEN** the user runs `wt create --checkout foo bar`
- **THEN** exit `ExitInvalidArgs` (2) with `--checkout cannot be combined with a positional branch argument` / `The positional creates a new branch; --checkout checks out an existing one` / `Use one of: wt create <new-branch> | wt create --checkout <existing-branch>`.

- **GIVEN** `--checkout` and `--base`
- **WHEN** the user runs `wt create --checkout foo --base main`
- **THEN** exit `ExitInvalidArgs` (2) with `--base cannot be combined with --checkout` / `--base is the start-point for a NEW branch; --checkout targets an existing branch` / `Drop --base, or create a new branch: wt create <name> --base <ref>`.

#### R4: `--base` validation simplifies; warn-and-ignore removed
Because the positional is always a new branch, `--base` SHALL always apply to it. The `baseWarnings`/`effectiveBase` warn-and-ignore machinery SHALL be removed, and the `--base` validation carve-out SHALL simplify to: validate `--base` (via `git rev-parse --verify`) whenever it is set and `--reuse` is not.

- **GIVEN** `--base <bad-ref>` with a new positional branch (and no `--reuse`)
- **WHEN** the user runs `wt create newbranch --base bad-ref`
- **THEN** exit `ExitInvalidArgs` with `Invalid --base ref: bad-ref`, no worktree or branch created.

- **GIVEN** `--reuse --worktree-name X --base <bad-ref>` with an existing worktree `X`
- **WHEN** the user runs it
- **THEN** `--reuse` short-circuits and `--base` is not validated (existing worktree reused).

#### R5: `--reuse` semantics unchanged
`--reuse` SHALL continue to require `--worktree-name`, and on name collision SHALL reuse the existing worktree without consulting branch selectors.

- **GIVEN** `--reuse` without `--worktree-name`
- **WHEN** the user runs it
- **THEN** exit `ExitInvalidArgs` with `--reuse requires --worktree-name` (unchanged).

- **GIVEN** `--reuse --worktree-name X` where worktree `X` already exists
- **WHEN** the user runs it
- **THEN** the existing worktree is reused (branch selectors not consulted).

### internal/worktree: seam split (constitution V)

#### R6: `CreateBranchWorktree` splits into two mode-explicit functions with typed sentinel errors
The existence-dispatch hidden inside `CreateBranchWorktree` SHALL be replaced by two internal functions so the business rule lives in `internal/` and `cmd/` only routes flags:
- `CreateNewBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint string) (string, error)` — returns a typed sentinel error (`ErrBranchExists`) if the branch already exists locally or remotely; otherwise today's new-branch path (`git worktree add -b`, rollback registers worktree removal + branch deletion).
- `CheckoutBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback) (string, error)` — returns a typed sentinel error (`ErrBranchNotFound`) if the branch is missing both locally and remotely; otherwise today's local/remote checkout paths.
`cmd/create.go` SHALL map these sentinel errors to the `ExitInvalidArgs` copy in R1/R2 (via `errors.Is`).

- **GIVEN** `CreateNewBranchWorktree` called with an already-existing branch
- **WHEN** invoked
- **THEN** it returns `ErrBranchExists` and creates no worktree.

- **GIVEN** `CheckoutBranchWorktree` called with a missing branch
- **WHEN** invoked
- **THEN** it returns `ErrBranchNotFound` and creates no worktree.

#### R7: Init confirm prompt gate generalizes to "branch explicitly named"
The "Initialize worktree?" confirm SHALL fire whenever a branch was explicitly named — positional (new-branch) OR `--checkout` — and SHALL be skipped for the bare exploratory create, preserving today's user-visible behavior.

- **GIVEN** an interactive `wt create --checkout foo` (foo exists)
- **WHEN** init would run
- **THEN** the "Initialize worktree?" confirm is shown (as it was for a positional existing branch today).

- **GIVEN** an interactive bare `wt create` (no positional, no `--checkout`)
- **WHEN** init would run
- **THEN** the confirm is skipped (unchanged).

### Help text & docs

#### R8: Help text reflects the new surface
`create.go`'s `Long` description SHALL state the new-branch-only positional plus `--checkout`, and a `--checkout` flag help string SHALL be registered; `--base` help is unchanged in meaning. `wt help-dump` output follows automatically.

- **GIVEN** `wt create --help`
- **WHEN** rendered
- **THEN** `Long` describes positional = new branch only and `--checkout` = existing branch, and `--checkout` appears in the flag list.

#### R9: Specs updated
`docs/specs/cli-surface.md` (§ `wt create`) and `docs/specs/worktree-layout.md` (§ Branch ↔ worktree relationship) SHALL be updated to describe the new-branch-only positional, `--checkout`, and the conflict/exit-code matrix.

- **GIVEN** the specs after this change
- **WHEN** read
- **THEN** cli-surface documents `--checkout`, new-branch-only positional (exit-2 on existing), and the `--base`/`--checkout` conflict; worktree-layout case 2 is keyed to `--checkout`.

### Non-Goals

- fab-kit's `fab batch switch` migration — a coordinated follow-up in the fab-kit repo (hard break accepted). Out of scope here.
- Any change to `--checkout`'s underlying checkout behavior beyond relocating today's existing-branch code path.
- A `docs/memory/wt-cli/create-branch-semantics.md` file — created at hydrate, not apply.

### Design Decisions

1. **Typed sentinel errors over inline existence probes in `cmd/`**: `internal/` owns the existence rules (`ErrBranchExists`/`ErrBranchNotFound`), `cmd/` maps them to exit codes via `errors.Is`. — *Why*: Constitution V (business rules in `internal/`, `cmd/` thin); mirrors the existing `InitNotFound`/`DefaultNotApplicable` typed-classification idiom in the same package. — *Rejected*: probing `BranchExistsLocally`/`BranchExistsRemotely` directly in `cmd/create.go` (leaks the rule into the thin layer, duplicates the check the internal function must do anyway).
2. **Pre-mutation guard placement**: the positional-existing and `--checkout`-missing errors surface from the internal functions (sentinel), but `cmd/` maps them before the summary/init phases — the internal functions create no worktree on the error path, so the "before any worktree mutation" guarantee holds without a separate pre-check. — *Why*: single source of truth for the existence rule; no double git query. — *Rejected*: a duplicate existence pre-check in `cmd/` purely to fail early (redundant git calls, drift risk).
3. **Flag/mutex validation lives in `cmd/`, existence rules in `internal/`**: `--checkout`+positional and `--checkout`+`--base` are pure flag-combination errors with no git state, so they are validated in `cmd/create.go` alongside the existing `--reuse` mutex check. — *Why*: these are argument-parsing concerns, not worktree business rules. — *Rejected*: pushing mutex checks into `internal/` (they have no git dependency).

## Tasks

### Phase 1: Internal seam (constitution V)

- [x] T001 Add typed sentinel errors `ErrBranchExists` and `ErrBranchNotFound` to `src/internal/worktree/crud.go` (package-level `var`s via `errors.New`), and split `CreateBranchWorktree` into `CreateNewBranchWorktree(branch, name, ctx, rb, startPoint)` (returns `ErrBranchExists` when `BranchExistsLocally` OR `BranchExistsRemotely`; else today's new-branch path) and `CheckoutBranchWorktree(branch, name, ctx, rb)` (local checkout as-is; remote-only → `FetchRemoteBranch` then checkout; else returns `ErrBranchNotFound`). Remove the old `CreateBranchWorktree`. <!-- R6 -->

### Phase 2: cmd/create.go routing

- [x] T002 In `src/cmd/wt/create.go`, add the `--checkout` flag (`cmd.Flags().StringVar(&checkout, "checkout", "", ...)`) and the `checkout` var. Register a help string describing checkout of an existing branch. <!-- R2 R8 -->
- [x] T003 In `src/cmd/wt/create.go`, add the two flag-mutex validations (before git work, near the `--reuse` check): `--checkout`+positional → `ExitInvalidArgs` (R3 copy) and `--checkout`+`--base` → `ExitInvalidArgs` (R3 copy). <!-- R3 -->
- [x] T004 In `src/cmd/wt/create.go`, simplify the `--base` validation carve-out (create.go:103-123): validate `--base` via `git rev-parse --verify` whenever `base != "" && !reuse` (drop the `existingBranch` probe). <!-- R4 -->
- [x] T005 In `src/cmd/wt/create.go`, run `ValidateBranchName(checkout)` when `checkout != ""` (mirror the positional validation), and set `suggestedName = DeriveWorktreeName(checkout)` for the checkout mode; keep `suggestedName = DeriveWorktreeName(branchArg)` for the positional (now new-branch) mode and `GenerateUniqueName` for bare create. <!-- R2 -->
- [x] T006 In `src/cmd/wt/create.go`, rewrite the create dispatch (create.go:220-248): remove `baseWarnings`/`effectiveBase`; route bare→`CreateExploratoryWorktree`, positional→`CreateNewBranchWorktree` (mapping `ErrBranchExists` to the R1 `ExitInvalidArgs` copy, other errors to `ExitGitError`), `--checkout`→`CheckoutBranchWorktree` (mapping `ErrBranchNotFound` to the R2 `ExitInvalidArgs` copy, other errors to `ExitGitError`). Set `createdSummaryBranch` accordingly. <!-- R1 R2 R6 -->
- [x] T007 In `src/cmd/wt/create.go`, remove the `baseWarnings` loop in the deferred-summary block (create.go:274-276) and generalize the init-confirm gate to fire when a branch was explicitly named — positional or `--checkout` (`!(nonInteractive || (branchArg == "" && checkout == ""))`). <!-- R4 R7 -->
- [x] T008 In `src/cmd/wt/create.go`, rewrite the `Long` help text to describe: bare create = exploratory new branch; positional = NEW branch only (existing → error, use `--checkout`); `--checkout` = existing branch. <!-- R8 -->

### Phase 3: Tests (constitution IV)

- [x] T009 In `src/cmd/wt/create_test.go`, update call sites that relied on positional-checks-out-existing to use `--checkout`: `TestCreate_ExistingLocalBranch`, `TestCreate_RemoteBranch`, `TestCreate_BranchNameDerivation`, `TestCreate_ExistingBranchUnaffectedByCurrentBranch`, `TestCreate_InitFailureKeepsWorktree_ExistingBranch`. Remove/replace the now-invalid `--base`-with-existing-branch warn tests (`TestCreate_BaseWithExistingLocalBranch`, `TestCreate_BaseWithExistingRemoteBranch`, `TestCreate_BaseInvalidRefExistingBranch`) — these tested warn-and-ignore that R4 removes; replace with `--checkout`+`--base` conflict coverage where they map. <!-- R1 R2 R4 -->
- [x] T010 In `src/cmd/wt/create_test.go`, add coverage: positional-existing local → exit 2 (points at `--checkout`, no worktree left); positional-existing remote-only → exit 2; `--checkout` local branch success; `--checkout` remote-only success; `--checkout` missing → exit 2 (points at create-new, no worktree left); `--checkout`+`--base` → exit 2; `--checkout`+positional → exit 2. <!-- R1 R2 R3 -->
- [x] T011 In `src/cmd/wt/integration_test.go`, update `TestIntegration_BranchDeletePreservesOthers` (create on existing `feature/delete-me`) to use `--checkout`; audit the other ~4 create call sites and update any relying on positional-existing. <!-- R1 R2 -->
- [x] T012 In `src/cmd/wt/edge_test.go`, update `TestEdge_BranchWithSpecialChars` (creates existing `feature/my_special-branch` then positional-checks-out) to use `--checkout`; `TestEdge_BranchWithSlashes` (new branch via positional) stays valid — verify it still creates a new branch. <!-- R1 R2 -->

### Phase 4: Docs

- [x] T013 [P] Update `docs/specs/cli-surface.md` § `wt create`: add `--checkout` to the flags table; rewrite the positional-arg semantics (new-branch-only, exit-2 on existing pointing at `--checkout`); update `--base` row (no longer "ignored for existing branches"); update exit-code notes (`--base`/`--checkout` conflict, existing-branch positional, `--checkout` missing). <!-- R9 -->
- [x] T014 [P] Update `docs/specs/worktree-layout.md` § Branch ↔ worktree relationship: rewrite case 2 around `--checkout` (existing-branch checkout), key the `DeriveWorktreeName` reference to `--checkout`. <!-- R9 -->

## Execution Order

- T001 (internal seam) blocks T006 (cmd routing depends on the new functions/sentinels).
- T002 blocks T003, T005, T006, T007 (they reference the `checkout` var).
- T009–T012 depend on Phase 1–2 being complete (they run against the new binary behavior).
- T013, T014 are independent docs edits ([P]).

## Acceptance

### Functional Completeness

- [x] A-001 R1: Bare `wt create` and `wt create <new-branch>` both create a new branch + worktree; a positional naming an existing branch (local or remote) exits 2 with the `--checkout` hint and leaves no state.
- [x] A-002 R2: `wt create --checkout <local>` and `--checkout <remote-only>` both put the worktree on the existing branch; `--checkout <missing>` exits 2 with the create-new hint and leaves no state; name suggestion is `DeriveWorktreeName`.
- [x] A-003 R3: `--checkout`+positional and `--checkout`+`--base` each exit 2 with the documented copy.
- [x] A-004 R6: `CreateNewBranchWorktree` returns `ErrBranchExists` on an existing branch; `CheckoutBranchWorktree` returns `ErrBranchNotFound` on a missing branch; `cmd/` maps both via `errors.Is` to exit 2.
- [x] A-005 R8: `wt create --help` (and `wt help-dump`) show the new-branch-only positional + `--checkout` description and the `--checkout` flag.
- [x] A-006 R9: `docs/specs/cli-surface.md` and `docs/specs/worktree-layout.md` describe the new surface (`--checkout`, new-branch-only positional, conflict/exit matrix).

### Behavioral Correctness

- [x] A-007 R4: The `baseWarnings`/`effectiveBase` warn-and-ignore machinery is gone; `--base` is validated whenever set and `--reuse` is not, and always applies to the (new) positional branch.
- [x] A-008 R5: `--reuse` still requires `--worktree-name` and reuses on collision without consulting branch selectors (`--base`/invalid-`--base` still short-circuit under `--reuse`).
- [x] A-009 R7: The "Initialize worktree?" confirm fires for a `--checkout` (or positional) create and is skipped for a bare exploratory create.

### Scenario Coverage

- [x] A-010 R1: Test coverage exists for positional-existing local → exit 2 and positional-existing remote-only → exit 2.
- [x] A-011 R2: Test coverage exists for `--checkout` local success, `--checkout` remote-only success, and `--checkout` missing → exit 2.
- [x] A-012 R3: Test coverage exists for `--checkout`+`--base` → exit 2 and `--checkout`+positional → exit 2.

### Edge Cases & Error Handling

- [x] A-013 R1: No partial worktree directory or branch is left behind on the positional-existing and `--checkout`-missing error paths (rollback / pre-mutation guarantee).
- [x] A-014 R6: SIGINT / rollback / phase-separator / init-confirm behavior on the `--checkout` success path is identical to the former positional-existing path.

### Code Quality

- [x] A-015 Pattern consistency: New code follows the surrounding create.go / crud.go patterns (error copy via `ExitWithError(what, why, fix)`, sentinels via `errors.New`/`errors.Is`, stdout=machine / stderr=human).
- [x] A-016 No unnecessary duplication: existence rules live once in `internal/worktree` (reusing `BranchExistsLocally`/`BranchExistsRemotely`/`FetchRemoteBranch`); `cmd/` does not re-probe branch existence.
- [x] A-017 No god functions: the split keeps `CreateNewBranchWorktree`/`CheckoutBranchWorktree` focused; the create.go `RunE` does not grow a new deeply-nested block beyond the routing dispatch.
- [x] A-018 No magic strings: exit codes use the `internal/worktree/errors.go` constants; error copy is inline literals consistent with the intake's exact wording.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- Module root is `src/`; run gofmt/vet/tests from there. gofmt is CI-enforced before vet/test.

## Deletion Candidates

- `README.md:95-96` (plus the summary row at `README.md:78` and the gotcha bullet at `README.md:152`) — the `wt create --base` scenario-table rows "Existing local branch" / "Existing remote branch" document the removed warn-and-ignore behavior (`--base ignored: ...`, "wt checks out the existing branch instead"); those invocations now exit 2, so the rows are dead copy to delete/rewrite around `--checkout`.
- `docs/site/workflows.md:123-124` (plus `docs/site/workflows.md:26` and `docs/site/workflows.md:210`) — same removed behavior documented in the site guide: the two warn-and-ignore table rows, the "checks out that branch (existing) or creates it (new)" positional description, and the existing-branch gotcha bullet.
- Source: none — the change itself already removed everything it made redundant (`CreateBranchWorktree`, the `baseWarnings`/`effectiveBase` machinery, and `cmd/create.go`'s `git rev-parse --verify <branch>` existence probe); `BranchExistsLocally`/`BranchExistsRemotely`/`FetchRemoteBranch` remain live via the new internal functions.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Sentinel errors named `ErrBranchExists` / `ErrBranchNotFound`, defined in `crud.go`, mapped in `cmd/` via `errors.Is` | Intake §4 specifies "typed sentinel error"; Go idiom is `errors.New` sentinels + `errors.Is`; mirrors the package's existing typed-classification helpers | S:70 R:85 A:90 D:85 |
| 2 | Certain | Flag-mutex checks (`--checkout`+positional, `--checkout`+`--base`) live in `cmd/create.go`, existence rules in `internal/` | Mutex checks have no git state (pure arg parsing); existence checks are worktree business rules per Constitution V; matches the existing `--reuse` mutex placement | S:75 R:85 A:92 D:88 |
| 3 | Confident | The removed `--base`-with-existing-branch warn tests are replaced/dropped rather than kept | R4 removes the warn-and-ignore behavior those tests assert; keeping them would contradict the spec (Constitution: tests conform to spec) | S:65 R:80 A:88 D:82 |
| 4 | Confident | `--checkout` runs `ValidateBranchName` first (mirroring the positional) | The positional path validates the name before existence checks; `--checkout` is the analogous existing-branch entry and should reject malformed refs the same way | S:60 R:80 A:82 D:78 |
| 5 | Confident | `CheckoutBranchWorktree` takes no `startPoint` param (checkout never branches new) | Intake §4 gives its signature without a start-point; a checkout of an existing branch has no start-point concept | S:70 R:82 A:88 D:85 |
| 6 | Confident | Init-confirm gate generalizes to `!(nonInteractive || (branchArg == "" && checkout == ""))` | Intake §4 states "prompt when a branch was explicitly named (positional or `--checkout`)"; direct generalization of today's `branchArg == ""` gate | S:65 R:85 A:85 D:80 |

6 assumptions (2 certain, 4 confident, 0 tentative).
