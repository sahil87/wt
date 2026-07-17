---
type: memory
description: "The `wt create` branch-selection contract — positional = new-branch-only, `--checkout` = existing-branch opt-in, `--base` = new-branch start-point, the conflict/exit-code matrix, and the internal seam (CreateNewBranchWorktree / CheckoutBranchWorktree + sentinel errors)."
---
# wt-cli: Create Branch-Selection Contract

> Post-implementation behavior capture for `wt create`'s branch-selection surface.
> Source change: `260717-2af2-explicit-base-checkout-flags`.

This file documents how `wt create` decides which branch a new worktree lands on: the
new-branch-only positional, the `--checkout` existing-branch opt-in, the `--base`
new-branch start-point, the conflict/exit-code matrix, and the `internal/worktree` seam
(`CreateNewBranchWorktree` / `CheckoutBranchWorktree` + the sentinel errors) that keeps the
existence business-rule out of `cmd/`. Future changes touching `src/cmd/wt/create.go` or the
create/checkout functions in `src/internal/worktree/crud.go` should preserve these invariants
unless an explicit spec amendment supersedes them.

**Why this contract exists.** Before this change, `wt create [branch]` silently dispatched on
branch existence: an unused name created a new branch, an *existing* local/remote name checked
that branch out directly into the new worktree — with no warning. The two outcomes differ
wildly in consequence: a user typing `wt create --worktree-name foo sockets-v2` intending a
scratch worktree landed *on* `sockets-v2` itself, one `git commit` away from polluting a shared
/ collector branch. The overload is now split into explicit modes so the dangerous direction
(check out an existing branch) is an opt-in (`--checkout`) and the safe common path
(`wt create <new-branch>`) stays short and fails closed on an existing name.

## Requirements

### The positional `[branch]` names a NEW branch only

- `wt create <branch>` treats the positional as the name of a **new** branch to create (off
  `--base`, else HEAD). Its signature is unchanged (`Use: "create [branch]"`,
  `Args: cobra.MaximumNArgs(1)`); only its *meaning* narrowed to a single mode.
- If `<branch>` already exists **locally OR remotely**, the command exits `ExitInvalidArgs` (2)
  **before any worktree mutation**, pointing at `--checkout`. Copy (via
  `ExitWithError(what, why, fix)`):
  - what: `Branch '<branch>' already exists`
  - why: `The positional argument only creates new branches`
  - fix: `To put a worktree on the existing branch: wt create --checkout <branch>`
- Remote-only is deliberately included in the existence check — a remote-only shared branch is
  the exact danger case the change targets.
- The bare-create path (no positional, no `--checkout`) is unchanged: an exploratory worktree on
  a new branch named after the (random or `--worktree-name`) worktree name, `--base` honored.

- **GIVEN** a repo with no branch `foo` (local or remote)
- **WHEN** the user runs `wt create foo`
- **THEN** a new branch `foo` is created and a worktree is placed on it.

- **GIVEN** a branch `foo` that exists locally (or remote-only)
- **WHEN** the user runs `wt create foo`
- **THEN** the command exits `ExitInvalidArgs` (2) with the `--checkout` fix hint above, and no
  worktree directory and no branch mutation is left behind.

### `--checkout <branch>` opts in to an EXISTING branch

- `wt create --checkout <branch>` places the new worktree on an **existing** branch: a local
  branch checked out as-is (`git worktree add <path> <branch>`, no `-b`); a remote-only branch
  fetched via `FetchRemoteBranch` (`git fetch origin <branch>:<branch>`) then checked out. This
  is the same existing-branch code path the old positional had — relocated, not reworked.
- If the branch exists neither locally nor remotely, the command exits `ExitInvalidArgs` (2),
  pointing at the create-new form:
  - what: `Branch '<branch>' not found`
  - why: `--checkout requires an existing local or remote branch`
  - fix: `To create a new branch: wt create <branch> [--base <ref>]`
- The suggested worktree name is `DeriveWorktreeName(<branch>)` (last path segment after `/`,
  non-`[A-Za-z0-9-_]` chars → `-`), the same derivation the old positional-existing path used.
- All surrounding phases are identical to today's former positional-existing path: dirty-state
  menu, name prompt/collision, init (including the "Initialize worktree?" confirm — see below),
  Open menu, rollback registration, the SIGINT / phase-separator contracts. `--checkout` changed
  *which flag* selects checkout mode, not *how* checkout behaves.

- **GIVEN** a branch `feature/auth` that exists locally
- **WHEN** the user runs `wt create --checkout feature/auth`
- **THEN** the worktree is created on `feature/auth` (no `-b`), suggested name `auth`.

- **GIVEN** a branch that exists only on `origin`
- **WHEN** the user runs `wt create --checkout <branch>`
- **THEN** the branch is fetched then checked out.

- **GIVEN** a name that exists neither locally nor remotely
- **WHEN** the user runs `wt create --checkout missing`
- **THEN** the command exits `ExitInvalidArgs` (2) with the create-new fix hint, and no worktree
  directory is left behind.

### `--base <ref>` is the NEW-branch start-point; it always applies to the positional

- `--base` is the git start-point (branch / tag / SHA) for the **new** branch, validated via
  `git rev-parse --verify` whenever it is set and `--reuse` is not. Because the positional is now
  *always* a new branch, `--base` always applies to it — the former warn-and-ignore behavior
  (`--base` silently dropped when the positional named an existing branch) is **gone**.
- The old `baseWarnings` / `effectiveBase` machinery and the `existingBranch`-probe carve-out in
  the `--base` validation are removed. Validation simplified to a single guard: `if base != ""
  && !reuse { git rev-parse --verify <base> }`. `--reuse` still short-circuits before `--base` is
  validated, so `wt create --reuse --worktree-name X --base <bad-ref>` on an existing worktree
  reuses without failing on the ref.

- **GIVEN** `--base <bad-ref>` with a new positional branch and no `--reuse`
- **WHEN** the user runs `wt create newbranch --base bad-ref`
- **THEN** the command exits `ExitInvalidArgs` with `Invalid --base ref: bad-ref`, creating no
  worktree or branch.

### Conflicting selectors are hard `ExitInvalidArgs` errors

Two flag combinations select contradictory modes and each exits `ExitInvalidArgs` (2) — matching
the documented exit-2 class "mutually exclusive flags". Both are pure argument-parsing checks
(no git state), validated in `cmd/create.go` **before** any git work, alongside the existing
`--reuse` mutex check:

- `--checkout` + a positional arg:
  - what: `--checkout cannot be combined with a positional branch argument`
  - why: `The positional creates a new branch; --checkout checks out an existing one`
  - fix: `Use one of: wt create <new-branch> | wt create --checkout <existing-branch>`
- `--checkout` + `--base` (replaces the former warn-and-ignore of `--base` on an existing branch):
  - what: `--base cannot be combined with --checkout`
  - why: `--base is the start-point for a NEW branch; --checkout targets an existing branch`
  - fix: `Drop --base, or create a new branch: wt create <name> --base <ref>`

- **GIVEN** `--checkout` and a positional arg
- **WHEN** the user runs `wt create --checkout foo bar`
- **THEN** the command exits `ExitInvalidArgs` (2) with the combined-with-positional copy.

- **GIVEN** `--checkout` and `--base`
- **WHEN** the user runs `wt create --checkout foo --base main`
- **THEN** the command exits `ExitInvalidArgs` (2) with the combined-with-`--base` copy.

### Exit-code / mode matrix

| Invocation | Mode | Outcome |
|------------|------|---------|
| `wt create` | bare exploratory | new branch named after the worktree name; `--base` honored |
| `wt create <new>` | positional, new | new branch `<new>` (off `--base`, else HEAD) |
| `wt create <existing>` | positional, existing | `ExitInvalidArgs` (2) → `--checkout` |
| `wt create --checkout <existing>` | checkout | worktree on `<existing>` (local as-is / remote-only fetch-then-checkout) |
| `wt create --checkout <missing>` | checkout | `ExitInvalidArgs` (2) → create-new |
| `wt create --checkout X <pos>` | conflict | `ExitInvalidArgs` (2) |
| `wt create --checkout X --base Y` | conflict | `ExitInvalidArgs` (2) |
| `wt create <new> --base <bad>` | positional, new | `ExitInvalidArgs` (2), invalid `--base` |
| `wt create --reuse --worktree-name X …` | reuse | reuse on collision; branch selectors NOT consulted |

`ExitInvalidArgs` = 2, `ExitGitError` = 3 (`git worktree add` failure), both from
`src/internal/worktree/errors.go`.

### The creation mode is visible at creation time in the summary output (`260717-aeka`)

As of `260717-aeka-create-summary-base-ref`, the three creation modes are **visually distinguished
in the Git-phase summary output** by a fourth `From:` line — so a user can tell from the output
which of this file's modes ran and what ref the branch was based on (the exact create-vs-checkout
confusion this contract's flag split targets, now surfaced at the output layer too). This is a
*display* addition only — **no branch-selection semantics change**; the `From:` value is derived
from the same `checkout` / `base` inputs and the mode dispatch stays exactly as defined above:

| This file's mode | `From:` summary value |
|------------------|-----------------------|
| positional new / bare, `--base` given | the `--base` ref verbatim as typed |
| positional new / bare, no `--base` | the resolved HEAD label (`DescribeHead()` — current branch, or short SHA when detached) |
| `--checkout <branch>` (local or remote-only) | `existing branch '<branch>' (checked out directly)` |
| `--reuse` | *(no summary block — no `From:` line)* |

The line's full contract (stderr-only, pre-dispatch resolution outside the reinstall window, the
best-effort `DescribeHead()` helper) lives in [`create-output-phases`](/wt-cli/create-output-phases.md)
"The Git-phase summary block carries a fourth `From:` line".

### `--reuse` is unchanged

`--reuse` still requires `--worktree-name`, and on name collision it reuses the existing worktree
**without consulting any branch selector** (positional / `--checkout` / `--base`). The reuse
short-circuit sits *before* the branch-dispatch switch, so a reused worktree's branch is never
re-derived. (Init-on-reuse remains warn-but-continue — see `/wt-cli/init-failure-contract.md`.)

- **GIVEN** `--reuse` without `--worktree-name`
- **WHEN** the user runs it
- **THEN** the command exits `ExitInvalidArgs` with `--reuse requires --worktree-name`.

- **GIVEN** `--reuse --worktree-name X` where worktree `X` already exists
- **WHEN** the user runs it
- **THEN** the existing worktree is reused; no branch selector is read.

### The init-confirm gate fires whenever a branch was explicitly named

The interactive "Initialize worktree?" confirm fires when a branch was explicitly named —
positional (new branch) **or** `--checkout` (existing branch) — and is skipped for the bare
exploratory create, preserving the pre-change user-visible behavior. The gate generalized from
`!(nonInteractive || branchArg == "")` to `!(nonInteractive || (branchArg == "" && checkout ==
""))`.

- **GIVEN** an interactive `wt create --checkout foo` (foo exists)
- **WHEN** init would run
- **THEN** the "Initialize worktree?" confirm is shown.

- **GIVEN** an interactive bare `wt create` (no positional, no `--checkout`)
- **WHEN** init would run
- **THEN** the confirm is skipped.

### The `internal/worktree` seam: two mode-explicit functions + sentinel errors

The existence-dispatch formerly hidden inside `CreateBranchWorktree` is now split into two
mode-explicit functions in `src/internal/worktree/crud.go`, so the branch-existence business
rule lives in `internal/` and `cmd/` only routes flags (Constitution V). `CreateBranchWorktree`
is removed.

- `CreateNewBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint string) (string, error)`
  — returns the sentinel `ErrBranchExists` and creates nothing when `BranchExistsLocally(branch)
  || BranchExistsRemotely(branch)`; otherwise runs the new-branch path (`git worktree add -b`
  from `startPoint`, else HEAD) and registers **both** rollbacks: worktree removal AND
  `git branch -D <branch>` (the branch is newly created, so it must be torn down on rollback).
- `CheckoutBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback) (string, error)`
  — takes **no** `startPoint` (a checkout of an existing branch has no start-point). Local branch
  → checkout as-is; remote-only → `FetchRemoteBranch` then checkout; neither → returns the
  sentinel `ErrBranchNotFound` and creates nothing. Registers **only** worktree removal on
  rollback — the branch pre-existed and must NOT be deleted.
- `ErrBranchExists` and `ErrBranchNotFound` are package-level `errors.New` sentinels declared in
  `crud.go`. `cmd/create.go` maps them via `errors.Is`: `ErrBranchExists` → the R1 positional
  `ExitInvalidArgs` copy; `ErrBranchNotFound` → the R2 `--checkout` `ExitInvalidArgs` copy. Any
  **other** error from either function maps to `ExitGitError` ("Failed to create worktree").
- `CreateExploratoryWorktree` (the bare path) is untouched. The reused `git.go` helpers
  (`BranchExistsLocally` / `BranchExistsRemotely` / `FetchRemoteBranch`) are unchanged.

Because the existence check lives inside the internal function and the function creates no
worktree on the error path, the "before any worktree mutation" guarantee holds without any
duplicate pre-check in `cmd/` — a single source of truth, no double git query.

- **GIVEN** `CreateNewBranchWorktree` called with an already-existing branch
- **WHEN** invoked
- **THEN** it returns `ErrBranchExists` and creates no worktree.

- **GIVEN** `CheckoutBranchWorktree` called with a missing branch
- **WHEN** invoked
- **THEN** it returns `ErrBranchNotFound` and creates no worktree.

### fab-kit `batch switch` migration note (hard break — no deprecation window)

fab-kit's `fab batch switch` (`src/go/fab/cmd/fab/batch_switch.go:98`) invokes
`wt create --non-interactive --reuse --worktree-name <name> <branch>`, relying on the old
create-or-checkout dual semantics. After this change that call exits `ExitInvalidArgs` (2)
whenever the branch already exists and the worktree does not (e.g. worktree deleted, branch
kept) — because the positional now rejects an existing branch. This is a **deliberate hard
break**, no deprecation window: the fab-kit migration (an existence probe + routing to
`--checkout` vs. the positional) is a coordinated follow-up **in the fab-kit repo**, out of
scope for this change. The typed error + fix hint is the migration signal. `fab batch new`
(no positional) is unaffected. This was accepted at intake because the single known external
consumer is owned by the same author.

## Design Decisions

### Typed sentinel errors in `internal/`, mapped to exit codes in `cmd/`
**Decision**: `internal/worktree` owns the existence rules and exposes `ErrBranchExists` /
`ErrBranchNotFound` sentinels; `cmd/create.go` maps them to `ExitInvalidArgs` via `errors.Is`.
**Why**: Constitution V (business rules in `internal/`, `cmd/` thin), and it mirrors the
package's existing typed-classification idiom (e.g. `InitNotFound` / `DefaultNotApplicable` in
`init.go`). The internal function does the existence check it needs anyway, so mapping its
sentinel is free — no second git query, no rule leaking into the thin layer.
**Rejected**: probing `BranchExistsLocally` / `BranchExistsRemotely` directly in `cmd/create.go`
(leaks the rule into `cmd/` and duplicates the check the internal function must do anyway).
*Introduced by*: `260717-2af2-explicit-base-checkout-flags`.

### Flag-mutex validation in `cmd/`, existence rules in `internal/`
**Decision**: the `--checkout`+positional and `--checkout`+`--base` conflicts are validated in
`cmd/create.go` (next to the existing `--reuse` mutex check), while branch-existence checks live
in `internal/`.
**Why**: the mutex conflicts are pure argument-parsing concerns with no git state; the existence
checks are worktree business rules with git dependencies. Splitting them along that line keeps
each concern where it belongs.
**Rejected**: pushing the mutex checks into `internal/` (they have no git dependency, so they do
not belong with the worktree logic).
*Introduced by*: `260717-2af2-explicit-base-checkout-flags`.

### Keep the positional (new-branch-only) rather than remove it
**Decision**: the positional survives with exactly one meaning (name a NEW branch); an existing
local/remote name is a hard error pointing at `--checkout`.
**Why**: worktree directory names cannot contain `/`, so removing the positional would make
slashed branch names (`feature/x`, fab `branch_prefix` values like `sahil/`) impossible to
create via `wt` — the positional is the only way to name a new branch with a slash.
**Rejected**: full positional removal (loses slashed new-branch names); a deprecation window for
the checkout overload (unneeded complexity for a single known external consumer owned by the same
author — hence the hard break above).
*Introduced by*: `260717-2af2-explicit-base-checkout-flags`.

## Cross-references

- Spec doc: [`docs/specs/cli-surface.md`](../../specs/cli-surface.md) § `wt create` (flags table
  incl. `--checkout`, new-branch-only positional, exit-code notes) and
  [`docs/specs/worktree-layout.md`](../../specs/worktree-layout.md) § Branch ↔ worktree
  relationship (case 1 new-branch, case 2 keyed to `--checkout`).
- Source: `src/cmd/wt/create.go` (`--checkout` flag, the two flag-mutex checks, the simplified
  `--base` validation, `ValidateBranchName` on positional-or-`--checkout`, `DeriveWorktreeName`
  suggestion, the create-dispatch switch mapping the sentinels, the generalized init-confirm
  gate, the `Long` help text); `src/internal/worktree/crud.go`
  (`CreateNewBranchWorktree` / `CheckoutBranchWorktree`, `ErrBranchExists` / `ErrBranchNotFound`,
  `CreateExploratoryWorktree` untouched); `src/internal/worktree/git.go`
  (`BranchExistsLocally` / `BranchExistsRemotely` / `FetchRemoteBranch`, reused);
  `src/internal/worktree/context.go` (`ValidateBranchName`, `DeriveWorktreeName`);
  `src/internal/worktree/errors.go` (`ExitInvalidArgs`, `ExitGitError`, `ExitWithError`).
- Tests: `src/cmd/wt/create_test.go` (positional-existing local/remote → exit 2, `--checkout`
  local/remote-only success, `--checkout` missing → exit 2, `--checkout`+`--base` → exit 2,
  `--checkout`+positional → exit 2; the removed `--base`-with-existing-branch warn tests);
  `src/cmd/wt/integration_test.go` (`TestIntegration_BranchDeletePreservesOthers` on an existing
  branch via `--checkout`); `src/cmd/wt/edge_test.go` (`TestEdge_BranchWithSpecialChars` via
  `--checkout`; `TestEdge_BranchWithSlashes` stays a new-branch positional).
- Constitution: Principle III (Typed Exit Codes — the conflict/existence errors map to
  `ExitInvalidArgs`, never ad-hoc integers), Principle V (Internal Package Boundary — existence
  rules + sentinels in `internal/`, `cmd/` only routes flags and maps codes), Principle VI
  (Interactive by Default, Scriptable on Demand — `--non-interactive` gates the init-confirm and
  every prompt).
- External: fab-kit `fab batch switch` (`batch_switch.go:98`) breaks until its coordinated
  follow-up migration (hard break, accepted); `wt help-dump`'s JSON envelope reflects the new
  `Long` help text with no schema change (see `/wt-cli/help-dump-contract.md`).
- Sibling memory: [`create-output-phases`](/wt-cli/create-output-phases.md) — the Git / Init /
  Open phase-separator + stdout-path-line contract shared by every create mode (new-branch,
  `--checkout`, and bare); as of `260717-aeka` it also owns the fourth `From:` summary line that
  *names* which of this file's modes ran (base ref verbatim / resolved HEAD / existing-branch copy). [`init-failure-contract`](/wt-cli/init-failure-contract.md) — the
  kept-worktree / `ExitInitFailed` / open-anyway / SIGINT-Option-B / foreground-reclaim behavior
  that runs identically on the `--checkout` path and the positional-new path; also the source of
  the `--reuse` init warn-but-continue exemption. [`help-dump-contract`](/wt-cli/help-dump-contract.md)
  — the JSON envelope that carries the rewritten `create` help text to shll.ai.
