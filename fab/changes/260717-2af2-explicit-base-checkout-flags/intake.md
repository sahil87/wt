# Intake: Explicit --base/--checkout Split for wt create

**Change**: 260717-2af2-explicit-base-checkout-flags
**Created**: 2026-07-17

## Origin

Backlog item `[2af2]` (fab/backlog.md), created via `/fab-new` interactively:

> wt create's trailing positional arg silently overloads two meanings: an unused name branches new (per worktree.baseRef), an EXISTING branch name checks it out directly in-place with no warning -- dangerous for shared/collector branches (e.g. wt create --worktree-name foo sockets-v2 puts the worktree ON sockets-v2 itself, ready to commit to the shared branch). Split into explicit flags: --base <ref> (always branch new off ref) vs --checkout <branch> (explicit opt-in to shared checkout); remove the ambiguous bare-positional overload.

Two design questions were asked and answered during intake:

1. **Fate of the positional arg** — user chose **keep it, new-branch-only**: the positional survives with exactly one meaning (name for a NEW branch); naming an existing local/remote branch is a hard `ExitInvalidArgs` error pointing at `--checkout`. Full removal was rejected because worktree directory names cannot contain `/`, so removing the positional would make slashed branch names (`feature/x`, fab `branch_prefix` values like `sahil/`) impossible to create via wt.
2. **fab-kit breakage handling** — user chose **hard break now**: no deprecation window. fab-kit's `fab batch switch` (batch_switch.go:98) is migrated as a coordinated follow-up in the fab-kit repo; the typed error + fix hint is the migration signal.

Note: `--base <ref>` already exists on `wt create` (start-point for new branches, validated via `git rev-parse --verify`). The backlog's `worktree.baseRef` does not correspond to anything in the code — treated as shorthand for the `--base`/HEAD default semantics; no config key is added.

## Why

1. **The pain point**: `wt create [branch]` silently dispatches on branch existence — an unused name creates a new branch, an existing local/remote name checks that branch out directly into the new worktree. The two behaviors are wildly different in consequence, and nothing in the invocation distinguishes them. A user who types `wt create --worktree-name foo sockets-v2` intending a scratch worktree lands ON `sockets-v2` itself — one `git commit` away from polluting a shared/collector branch.
2. **If we don't fix it**: the failure mode is silent and asymmetric. Creating a new branch when you meant checkout is a recoverable annoyance; checking out a shared branch when you meant a new one risks accidental commits to shared history. The current warn-free behavior guarantees this eventually happens.
3. **Why this approach**: making checkout an explicit opt-in (`--checkout <branch>`) removes the silent overload while keeping the safe, common path (`wt create <new-branch>`) short. The dangerous direction now fails closed with a typed error (`ExitInvalidArgs`) and an exact fix hint, per the stdout/stderr + `ExitWithError(what, why, fix)` convention. Alternatives rejected: full positional removal (loses slashed branch names — see Origin); deprecation window (unneeded complexity for a single known external consumer owned by the same author).

## What Changes

### 1. Positional `[branch]` becomes new-branch-only

`wt create <branch>` retains its signature but the positional has exactly one meaning: the name of a **new** branch to create (optionally off `--base`, else HEAD).

- If `<branch>` already exists **locally or remotely** (remote-only is the exact shared-branch danger case), the command fails before any worktree mutation:

```go
wt.ExitWithError(wt.ExitInvalidArgs,
    fmt.Sprintf("Branch '%s' already exists", branchArg),
    "The positional argument only creates new branches",
    fmt.Sprintf("To put a worktree on the existing branch: wt create --checkout %s", branchArg))
```

- Existence checks reuse `BranchExistsLocally` / `BranchExistsRemotely` (internal/worktree/git.go); `ValidateBranchName` runs first, unchanged.
- The warn-and-ignore machinery for `--base` on existing branches (`baseWarnings`, `effectiveBase` in create.go:231–241) is **removed** — the positional is always a new branch, so `--base` always applies.
- The `--base` validation carve-out (create.go:103–123) simplifies: `--base` is validated whenever set and `--reuse` is not (the `existingBranch` probe is no longer needed).
- Behavior with no positional is unchanged: exploratory worktree, new branch named after the worktree name, `--base` honored (create.go:223–229).

### 2. New `--checkout <branch>` flag

Explicit opt-in to put the new worktree on an **existing** branch. Reuses today's existing-branch paths in `CreateBranchWorktree`:

- Local branch → `git worktree add <path> <branch>` (no `-b`).
- Remote-only branch → `FetchRemoteBranch` then checkout (today's behavior, unchanged).
- Branch missing both locally and remotely:

```go
wt.ExitWithError(wt.ExitInvalidArgs,
    fmt.Sprintf("Branch '%s' not found", checkout),
    "--checkout requires an existing local or remote branch",
    fmt.Sprintf("To create a new branch: wt create %s [--base <ref>]", checkout))
```

- Worktree name suggestion: `DeriveWorktreeName(branch)`, same as today's positional-existing path.
- All surrounding phases identical to today: dirty-state menu, name prompt/collision, init (including the "Initialize worktree?" confirm — see §4), open menu, rollback registration, SIGINT contract, phase separators.

### 3. Conflicting selectors are hard errors

Both were previously silent or impossible; both now exit `ExitInvalidArgs` (matches the documented exit-2 class "incompatible flags"):

- `--checkout` + positional arg:

```go
wt.ExitWithError(wt.ExitInvalidArgs,
    "--checkout cannot be combined with a positional branch argument",
    "The positional creates a new branch; --checkout checks out an existing one",
    "Use one of: wt create <new-branch> | wt create --checkout <existing-branch>")
```

- `--checkout` + `--base` (replaces today's warn-and-ignore for existing branches):

```go
wt.ExitWithError(wt.ExitInvalidArgs,
    "--base cannot be combined with --checkout",
    "--base is the start-point for a NEW branch; --checkout targets an existing branch",
    "Drop --base, or create a new branch: wt create <name> --base <ref>")
```

- `--reuse` interaction is unchanged: still requires `--worktree-name`; on name collision the existing worktree is reused and branch selectors are not consulted (today's short-circuit at create.go:189–204).

### 4. cmd/internal seam (constitution V)

`CreateBranchWorktree` (internal/worktree/crud.go:71) currently hides the existence dispatch. Split it into two explicit internal functions so the business rule lives in `internal/` and `cmd/` only routes flags:

- `CreateNewBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint string)` — fails with a typed sentinel error if the branch exists locally/remotely; otherwise today's new-branch path (`git worktree add -b`, rollback registers branch deletion).
- `CheckoutBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback)` — fails with a typed sentinel error if the branch is missing; otherwise today's local/remote checkout paths.
- `cmd/create.go` maps the sentinel errors to the `ExitInvalidArgs` copy above (exact function shape left to apply).

The init confirm prompt gate keeps its current shape generalized: prompt "Initialize worktree?" whenever a branch was explicitly named (positional **or** `--checkout`), skip it for the bare exploratory create (today: `!(nonInteractive || branchArg == "")`).

Help text updates: `Use: "create [branch]"` stays; `Long` rewritten to state new-branch-only positional + `--checkout`; flag help strings for `--base` (unchanged meaning) and new `--checkout`. `wt help-dump` output follows automatically.

### 5. Docs

- `docs/specs/cli-surface.md` § `wt create`: flags table gains `--checkout`; positional semantics rewritten (new-branch-only, exit-2 on existing); exit-code notes updated (`--base`/`--checkout` conflict, existing-branch positional).
- `docs/specs/worktree-layout.md` § Branch ↔ worktree relationship: case 2 rewritten around `--checkout`; `DeriveWorktreeName` reference now keyed to `--checkout`.

### 6. Breaking change — external consumer

fab-kit's `fab batch switch` (src/go/fab/cmd/fab/batch_switch.go:98) invokes `wt create --non-interactive --reuse --worktree-name <name> <branch>` relying on the create-or-checkout dual semantics. After this change the call fails with exit 2 whenever the branch already exists and the worktree does not (e.g. worktree deleted, branch kept). **Decided: hard break** — fab-kit migration (existence probe + `--checkout`/positional routing) is a coordinated follow-up in the fab-kit repo, out of scope here. `fab batch new` (no positional) is unaffected.

## Affected Memory

- `wt-cli/create-branch-semantics`: (new) The `wt create` branch-selection contract — positional = new-branch-only, `--checkout` = existing-branch opt-in, `--base` = new-branch start-point, conflict/exit-code matrix, and the fab-kit `batch switch` migration note.

## Impact

- **Source**: `src/cmd/wt/create.go` (flag defs, arg/mutex validation, error copy, baseWarnings removal, init-prompt gate); `src/internal/worktree/crud.go` (`CreateBranchWorktree` split into new-branch/checkout functions; `CreateExploratoryWorktree` untouched). `git.go` helpers (`BranchExistsLocally/Remotely`, `FetchRemoteBranch`) reused as-is.
- **Tests** (constitution IV): `src/cmd/wt/create_test.go` (~11 create invocations), `integration_test.go` (~4), `edge_test.go` (~2) — update any case relying on positional-checks-out-existing; add: positional-existing → exit 2 (local and remote-only), `--checkout` local, `--checkout` remote-only, `--checkout` missing → exit 2, `--checkout`+`--base` → exit 2, `--checkout`+positional → exit 2.
- **Docs**: `docs/specs/cli-surface.md`, `docs/specs/worktree-layout.md`.
- **External**: fab-kit `batch switch` breaks until its follow-up migration (accepted); `wt help-dump` JSON envelope (consumed by shll.ai's puller) reflects the new help text — no schema change.
- **CI**: gofmt enforced before vet/test (module root `src/`).

## Open Questions

- None — both structural questions (positional fate, fab-kit transition) were asked and resolved at intake.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Positional survives with single new-branch-only meaning; existing local/remote branch → `ExitInvalidArgs` pointing at `--checkout` | Asked — user selected "Keep it, new-branch-only" (preserves slashed branch names) | S:85 R:90 A:95 D:90 |
| 2 | Certain | Hard break, no deprecation window; fab-kit `batch switch` migrates in a coordinated follow-up | Asked — user selected "Hard break now" | S:80 R:80 A:90 D:90 |
| 3 | Certain | `--checkout`+`--base` and `--checkout`+positional are `ExitInvalidArgs` conflicts | Spec's exit-2 class explicitly covers mutually exclusive flags; the flags select contradictory modes | S:70 R:85 A:90 D:85 |
| 4 | Confident | `--checkout` keeps today's existing-branch behavior: local as-is, remote-only fetch-then-checkout, name suggested via `DeriveWorktreeName` | Straight relocation of the existing code path; no behavior change requested for checkout itself | S:65 R:80 A:88 D:82 |
| 5 | Confident | Positional existence check covers local AND remote branches | Remote-only shared branch is the exact danger case cited in the backlog | S:60 R:75 A:85 D:78 |
| 6 | Confident | `--reuse` semantics unchanged (requires `--worktree-name`; collision short-circuit ignores branch selectors) | Out of the stated scope; current behavior documented in spec | S:55 R:82 A:85 D:80 |
| 7 | Confident | Internal split: `CreateBranchWorktree` → `CreateNewBranchWorktree` + `CheckoutBranchWorktree` with typed sentinel errors; cmd maps to exit codes | Constitution V places existence rules in internal/; exact shape reversible at apply | S:45 R:70 A:75 D:60 |
| 8 | Confident | Init confirm prompt fires when a branch was explicitly named (positional or `--checkout`), skipped for bare exploratory create | Generalizes the existing `branchArg == ""` gate without changing user-visible behavior | S:50 R:85 A:80 D:70 |
| 9 | Confident | `worktree.baseRef` from the backlog maps to existing `--base`/HEAD semantics; no config key added | No such identifier exists in the codebase; `--base` already implements the described behavior | S:40 R:80 A:70 D:65 |

9 assumptions (3 certain, 6 confident, 0 tentative, 0 unresolved).
