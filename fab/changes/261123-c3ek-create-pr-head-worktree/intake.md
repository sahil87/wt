# Intake: `wt create --pr <n>` — a worktree on a PR's head branch

**Change**: 261123-c3ek-create-pr-head-worktree
**Created**: 2026-11-23

## Origin

> Execute step W4 (--pr <n>: a worktree on a PR's head branch) of the plan at
> /home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md. Read the whole
> plan first (Decisions and Rules bind you), then implement only the W4 Brief.

One-shot `/fab-new`, dispatched by a hexokit operator executing the plan's step queue
(`merge-auto`, no user gates). The plan (a copy also lives in-repo at
`fab/plans/sahil/26-11-23-worktree-base-receipts.md`) is the design source; its **Decisions** table
and **Rules for the operator** bind this change and are not reopened here. Only the **W4 Brief** is
in scope — W1 (`--fetch`/`--json`), W2 (recorded base), W3 (`--reuse`+`--base` conflict) and W5
(`--temp`) are sibling steps running in parallel worktrees (`wtr-w1`, `wtr-w3`, `wtr-w5`) and MUST
NOT be implemented here.

The W4 Brief, verbatim:

> - `wt create --pr <n>` resolves the PR's head with
>   `gh pr view <n> --json headRefName,isCrossRepository` (argv exec, 15 s timeout) and runs the
>   `--checkout <headRefName>` path (local as-is, remote-only fetched then checked out). Exclusive
>   with the positional, `--base`, `--checkout`, `--reuse`, `--temp`. `isCrossRepository: true` is
>   exit 2 (`Fix: add the fork as a remote and use --checkout`); `gh` missing or failing is exit 1
>   with the `gh` error's first line. The `From:` line reads `PR #<n> (existing branch '<head>')`.
> - Tests: scripted `gh` fake on PATH (the repo's integration tests build the binary; a shell stub
>   for `gh` in a temp PATH dir); fork case; missing gh.
> - Docs: `cli-surface.md`, `create-branch-semantics.md` (a fourth mode row), skill bundle,
>   help-dump.

Binding plan decisions relevant to W4:

- **`--pr <n>` decision row**: resolve the head branch via `gh pr view`, then run the
  `--checkout <head>` path; a cross-repository (fork) head is exit 2 in v1; `gh` missing or
  unauthenticated is exit 1 with the hint. *Why*: reuses the existing remote-only checkout path;
  forks need a remote `wt` does not manage.
- **Exit codes**: no new codes — `--pr` lookup failures map to 1 (gh) / 2 (fork, args).
  Constitution III.
- **Back-compat**: additive only; no existing flag changes meaning.
- **Scope**: `wt` is mechanism — no PR creation/merging, no fork-remote management.
- **Rules 2–5**: flag parsing/output in `src/cmd/wt/create.go`, every external-process call in
  `src/internal/worktree/`; unit tests per path + an integration case in
  `src/cmd/wt/integration_test.go` against a real temp repo (bare origin + clone); CLI ⇒ docs in
  the same change, each memory edit with a Design Decision citing the plan; long flag name, no
  short flag; review must-fix bar = worktree/branch left behind on failure, wrong base, changed
  exit code.

## Why

1. **Problem.** PR-surgery and review worktrees ("check out PR 5686 so I can resolve its
   conflict / run its tests") are hand-rolled today: a caller (human, agent, or hexokit's operator
   Merging rules) runs `gh pr view <n> --json headRefName`, parses the JSON, then calls
   `wt create --checkout <head>` — or worse, raw `git worktree add`. That is the plan's gap #5,
   duplicated across two codebases.
2. **Consequence if not fixed.** Every caller re-implements the lookup slightly differently; the
   fork case (head lives in another repository, so `origin/<head>` is the *wrong* branch or does
   not exist) is silently mishandled — a same-named branch on `origin` could be checked out instead
   of the PR's actual head.
3. **Why this approach.** `wt create --checkout` already handles local-as-is and remote-only
   fetch-then-checkout, rollback, naming, init and the Open phase. `--pr` is therefore a thin
   *resolver* in front of that path: one `gh` call turns `<n>` into a branch name, then the
   existing, tested checkout mode runs unchanged. Forks are refused (exit 2) rather than guessed,
   because handling them needs a remote `wt` does not manage. Rejected: talking to the GitHub API
   directly (adds auth handling and a dependency; `gh` is already the toolkit's GitHub seam);
   fetching `refs/pull/<n>/head` (gives a detached commit, not the PR's branch — that is what W5's
   `--temp` is for).

## What Changes

### 1. New flag `--pr <n>` on `wt create` (`src/cmd/wt/create.go`)

- Registered as `cmd.Flags().StringVar(&pr, "pr", "", "Create the worktree on pull request <n>'s head branch (resolved via gh; same-repository PRs only)")`.
  Long name only — no short flag (`flag-naming-conventions.md`). A **string** flag validated in
  `cmd/` as a positive decimal integer (`^[1-9][0-9]*$`), so a bad value gets the standard
  `Error/Why/Fix` copy and exit 2 instead of cobra's flag-parse error:
  - what: `Invalid --pr value: <v>`
  - why: `--pr takes a pull request number`
  - fix: `Example: wt create --pr 42`
  <!-- assumed: string flag + digits-only validation (no "#42", no PR URL) — keeps `<n>` exactly what the Brief names and gives typed exit-2 copy -->
- `Use` stays `create [branch]`; the `Long` help text gains a paragraph:

  ```
  --pr <n> creates the worktree on pull request <n>'s head branch, resolved
  with the GitHub CLI (gh pr view), then checked out exactly like --checkout.
  Only same-repository PRs are supported; a PR from a fork exits 2. --pr cannot
  be combined with a positional branch argument, --base, --checkout, or --reuse.
  ```

### 2. Mutual exclusion (pure argument checks in `cmd/`, before any git work)

Placed next to the existing `--checkout` mutex checks, before `ValidateGitRepo`. Each exits
`ExitInvalidArgs` (2) via `wt.ExitWithError(what, why, fix)`. One check per conflicting flag, in
this order: positional, `--base`, `--checkout`, `--reuse`:

| Combination | what | why | fix |
|---|---|---|---|
| `--pr` + positional | `--pr cannot be combined with a positional branch argument` | `--pr checks out the PR's existing head branch; the positional creates a new branch` | `Use one of: wt create <new-branch> \| wt create --pr <n>` |
| `--pr` + `--base` | `--base cannot be combined with --pr` | `--base is the start-point for a NEW branch; --pr targets the PR's existing head branch` | `Drop --base, or create a new branch: wt create <name> --base <ref>` |
| `--pr` + `--checkout` | `--pr cannot be combined with --checkout` | `Both select an existing branch to check out` | `Use one of: wt create --pr <n> \| wt create --checkout <branch>` |
| `--pr` + `--reuse` | `--pr cannot be combined with --reuse` | `--reuse keeps an existing worktree's branch, so there is no PR head to check out` | `Drop --reuse, or reuse by name: wt create --reuse --name <name>` |

Place the `--pr` checks after the existing "`--reuse` requires `--name`" check, so `--reuse`
without `--pr` behaves byte-identically to today (both orders exit 2 with nothing created).

**`--temp`** does not exist on `main` yet (W5, parallel). W5's Brief owns the `--temp`+`--pr`
exclusion ("Exclusive with the positional, `--checkout`, `--reuse`, `--pr`"); this change adds no
`--temp` code. **`--fetch`** (W1, parallel) likewise does not exist yet; its interaction with
`--pr` (the checkout path already fetches a remote-only branch) is for whichever step lands second.

### 3. PR-head resolution in `src/internal/worktree/` (new file `pr.go`)

Constitution V: the `gh` exec lives in `internal/`, `cmd/` only maps errors.

```go
// PRHead is what `gh pr view` reports about a pull request's head.
type PRHead struct {
    Branch         string // headRefName
    CrossRepository bool  // isCrossRepository
}

var (
    ErrGhNotFound      = errors.New("gh not found")          // exec.LookPath("gh") failed
    ErrPRCrossRepository = errors.New("pull request head is in another repository")
)

const prLookupTimeout = 15 * time.Second

// ResolvePRHead runs `gh pr view <n> --json headRefName,isCrossRepository`
// (argv exec, never a shell) with a 15 s timeout and returns the head branch.
func ResolvePRHead(number string) (PRHead, error)
```

- Exec: `exec.CommandContext(ctx, "gh", "pr", "view", number, "--json", "headRefName,isCrossRepository")`,
  `ctx` from `context.WithTimeout(context.Background(), prLookupTimeout)`; mirror
  `internal/update/update.go`'s bounded-exec idiom (`cmd.Cancel` sending SIGTERM + `cmd.WaitDelay`
  grace) — or a simpler kill on timeout is acceptable for a read-only lookup; the apply agent picks
  whichever reads cleanest, but the 15 s bound is a requirement. Run in the current working
  directory (gh resolves the repository from the git remotes there).
- `exec.LookPath("gh")` failure → `ErrGhNotFound`.
- Non-zero exit → an error whose message is the **first non-empty line of gh's stderr**
  (e.g. `no pull requests found for branch "99"` / `To get started with GitHub CLI, please run:  gh auth login`);
  fall back to the exec error text when stderr is empty.
- Timeout → an error `gh pr view timed out after 15s`.
- Parse stdout as JSON `{"headRefName": string, "isCrossRepository": bool}`; malformed JSON or an
  empty `headRefName` → an error (`unexpected gh output: …`).
- `isCrossRepository: true` → return the parsed `PRHead` **and** `ErrPRCrossRepository` (so `cmd/`
  can name the head in its copy).
- The `"gh"` binary name is a package-level `var` only if a unit test needs a seam; prefer testing
  through a stub `gh` placed first on `PATH` (`t.Setenv("PATH", …)`) so the real exec path is
  exercised.

### 4. Wiring in `create.go`

After `ValidateGitRepo` / `GetRepoContext` and **before** the rollback/signal setup's first
mutation and before the dirty-state menu (so every lookup failure leaves nothing behind):

```go
if pr != "" {
    head, err := wt.ResolvePRHead(pr)
    switch {
    case errors.Is(err, wt.ErrGhNotFound):
        wt.ExitWithError(wt.ExitGeneralError,
            "GitHub CLI (gh) not found",
            "--pr resolves the pull request's head branch with gh",
            "Install gh (https://cli.github.com) and run gh auth login, or use: wt create --checkout <branch>")
    case errors.Is(err, wt.ErrPRCrossRepository):
        wt.ExitWithError(wt.ExitInvalidArgs,
            fmt.Sprintf("PR #%s is from a fork (head branch '%s')", pr, head.Branch),
            "--pr only supports pull requests whose head branch is in this repository",
            "Add the fork as a remote and use --checkout")   // Brief's Fix copy; ExitWithError adds the "Fix:" label
    case err != nil:
        wt.ExitWithError(wt.ExitGeneralError,
            fmt.Sprintf("Could not resolve PR #%s", pr),
            err.Error(),                       // gh's first stderr line
            "Check the PR number and that gh is authenticated (gh auth status)")
    }
    checkout = head.Branch
}
```

Setting `checkout = head.Branch` makes everything downstream identical to `--checkout`:
`ValidateBranchName`, the suggested worktree name `DeriveWorktreeName(head)`, the name
prompt/collision, `CheckoutBranchWorktree` (local as-is / remote-only `FetchRemoteBranch` then
checkout / rollback registers only worktree removal), the init-confirm gate (fires — a branch was
named), init, Open. Two `--pr`-specific deltas:

- **`From:` summary line**: `PR #<n> (existing branch '<head>')` instead of
  `existing branch '<head>' (checked out directly)`. Resolved pre-dispatch like the other `From:`
  values (no git query inside the reinstall window).
- **`ErrBranchNotFound` under `--pr`** (same-repo PR whose head exists neither locally nor on
  `origin` — e.g. the head branch was deleted after merge, or gh resolved against a different
  remote than `origin`): exit `ExitInvalidArgs` (2) with
  - what: `PR #<n> head branch '<head>' not found`
  - why: `The branch exists neither locally nor on origin`
  - fix: `Fetch it yourself, or check the PR: gh pr view <n>`
  <!-- assumed: PR-specific copy for ErrBranchNotFound, keeping --checkout's exit 2 rather than a new mapping -->

Exit-code summary for `--pr` (no new codes):

| Outcome | Code |
|---|---|
| success | 0 |
| bad `<n>`, conflicting flag, fork PR, head branch not found | `ExitInvalidArgs` (2) |
| `gh` missing, `gh` non-zero (unauthenticated, PR not found, network), timeout, bad JSON | `ExitGeneralError` (1) |
| `git worktree add` failure | `ExitGitError` (3) — unchanged checkout-path mapping |
| init failure | `ExitInitFailed` — unchanged |

stdout contract unchanged: the worktree path as the last stdout line; all other output on stderr.

### 5. Tests (Constitution IV, test-alongside)

- **Unit** — `src/internal/worktree/pr_test.go` with a stub `gh` shell script written to a
  `t.TempDir()` placed first on `PATH`:
  - same-repo PR → `PRHead{Branch: "feature/x", CrossRepository: false}`, nil; the stub asserts
    its argv is exactly `pr view 42 --json headRefName,isCrossRepository` (e.g. echo args to a
    file the test reads, or fail the stub on mismatch).
  - cross-repo → `ErrPRCrossRepository` with the branch populated.
  - stub exits 1 with a multi-line stderr → error text equals the first line.
  - malformed JSON / empty `headRefName` → error.
  - no `gh` on PATH → `ErrGhNotFound`.
  - (timeout path: optional — a stub that sleeps would cost 15 s; skip or make the timeout a
    package var only if it is cheap and readable.)
- **cmd-level** — `src/cmd/wt/create_test.go` (binary-driven, like the existing `--checkout`
  tests): each of the four flag conflicts exits 2 with nothing created; `--pr abc` exits 2;
  fork PR exits 2 and no worktree dir/branch is left; missing gh exits 1 with the install hint
  (PATH set to a temp dir containing only a `git` symlink — `exec.LookPath("git")` resolves the
  real git for the symlink target); gh failure exits 1 with gh's first stderr line in the output.
- **Integration** — `src/cmd/wt/integration_test.go`: real temp repo with bare origin
  (`createTestRepo` + push), a branch pushed to origin then deleted locally (remote-only), stub
  `gh` on PATH reporting that branch → `wt create --pr 7 --non-interactive` creates the worktree
  on the branch, the `From:` line reads `PR #7 (existing branch '<head>')` on stderr, stdout's
  last line is the path; plus the local-branch variant (checked out as-is).

### 6. Docs (same change, Rule 3)

- `docs/specs/cli-surface.md` § `wt create [branch]`: flags-table row for `--pr <n>`; prose
  paragraph (resolution via `gh pr view`, same-repo only, the conflicts); exit-code note
  (2 = conflicts/fork/bad number/head not found, 1 = gh missing/failing).
- `docs/site/skill.md` (canonical) then `just sync-skill` to refresh `src/cmd/wt/skill.md`; keep
  within the 150-line budget enforced by `skill_test.go`. One line under the create flags, one
  gotcha (forks unsupported; needs `gh`).
- `wt help-dump`: no committed fixtures exist (the JSON is derived from the cobra tree at runtime
  — `help_dump_test.go` asserts shape, not content); the new flag and `Long` text flow through
  automatically. Verify `wt help-dump` output includes `--pr` and that its tests still pass.
- Memory (at hydrate): `create-branch-semantics.md` gains the `--pr` mode (a fourth mode row in
  the exit-code/mode matrix and the `From:` table, the conflict rows, the `ResolvePRHead` seam +
  sentinels) with a Design Decision citing the plan; `create-output-phases.md` `From:` table gains
  the `--pr` value; `flag-naming-conventions.md` only if it enumerates create's flags.

## Affected Memory

- `wt-cli/create-branch-semantics`: (modify) `--pr <n>` as a fourth create mode — gh resolution,
  same-repo-only (fork → exit 2), the four flag conflicts, exit-code matrix rows, `From:` value,
  the `ResolvePRHead` / `ErrGhNotFound` / `ErrPRCrossRepository` seam; Design Decision citing plan
  `26-11-23-worktree-base-receipts.md` W4
- `wt-cli/create-output-phases`: (modify) the `From:` line's `--pr` value `PR #<n> (existing branch '<head>')`
- `wt-cli/skill-command-contract`: (modify) only if the bundle's line count / content receipt
  is recorded there

## Impact

- **Code**: `src/cmd/wt/create.go` (flag, validation, conflicts, error mapping, `From:` label,
  `Long` help); new `src/internal/worktree/pr.go` (+ `pr_test.go`); `src/cmd/wt/create_test.go`,
  `src/cmd/wt/integration_test.go` (possibly a `writeGhStub` helper in `testutil_test.go`).
- **Docs**: `docs/specs/cli-surface.md`, `docs/site/skill.md` → `src/cmd/wt/skill.md`.
- **Runtime dependency**: `gh` becomes an *optional* runtime dependency, used only when `--pr` is
  passed (Constitution I's "no runtime dependencies beyond git" holds for every other path; `--pr`
  degrades to a typed exit 1 with an install hint when `gh` is absent).
- **Downstream**: additive. hexokit's operator Merging rules can use `wt create --pr <n>` (plan's
  Downstream section, not a step here). No existing flag changes meaning; no exit codes added.
- **Parallel steps**: touches the same `create.go` region (flag block, mutex checks, `From:`
  switch) as W1/W3/W5 — expect a textual merge conflict for whichever lands later; keep the
  additions compact and self-contained to ease the rebase.

## Open Questions

- None blocking. The plan's Decisions table fixes the behaviour; remaining choices are copy and
  validation details recorded as assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--pr <n>` resolves via `gh pr view <n> --json headRefName,isCrossRepository` (argv exec, 15 s timeout) then runs the existing `--checkout <head>` path unchanged | Plan Decisions table + W4 Brief state it verbatim | S:95 R:80 A:95 D:95 |
| 2 | Certain | Exit codes: fork → 2; gh missing/failing → 1 with gh's first stderr line; no new codes | Plan Decisions (Exit codes row) + Brief; Constitution III | S:95 R:70 A:95 D:95 |
| 3 | Certain | `--pr` exclusive with positional, `--base`, `--checkout`, `--reuse` → exit 2 before any git work; `--temp` exclusion left to W5 (flag not on main) | Brief lists the set; Rule "implement only W4"; W5 Brief owns the `--pr` exclusion on its side | S:90 R:85 A:90 D:85 |
| 4 | Certain | `From:` summary line reads `PR #<n> (existing branch '<head>')` | Brief states it verbatim | S:95 R:90 A:95 D:95 |
| 5 | Certain | gh exec + JSON parsing + sentinels live in new `src/internal/worktree/pr.go`; `cmd/create.go` only maps errors | Constitution V + plan Rule 2; mirrors `ErrBranchExists`/`ErrBranchNotFound` sentinel idiom | S:85 R:85 A:95 D:85 |
| 6 | Confident | `--pr` is a string flag validated as a positive decimal integer in `cmd/` (exit 2 with Error/Why/Fix); `#42` and PR URLs are not accepted | Brief names `<n>`; typed copy beats cobra's int-parse error; easy to widen later | S:60 R:85 A:70 D:65 |
| 7 | Certain | PR lookup runs after `ValidateGitRepo` and before the dirty-state menu / any mutation, so lookup failures leave nothing behind | Review must-fix bar: nothing left behind on failure; gh needs the repo cwd | S:70 R:85 A:85 D:80 |
| 8 | Certain | Suggested worktree name for `--pr` is `DeriveWorktreeName(head)` — identical to `--checkout` | "runs the `--checkout <headRefName>` path" — reuse everything downstream | S:75 R:90 A:80 D:80 |
| 9 | Confident | Same-repo PR whose head is not found locally or on origin → exit 2 with PR-specific copy (`PR #<n> head branch '<head>' not found`) | Keeps `--checkout`'s ErrBranchNotFound exit 2; copy wording is a guess | S:45 R:85 A:60 D:55 |
| 10 | Confident | Error copy for gh missing / gh failing / fork / conflicts as drafted in What Changes §2 and §4 | Follows the file's existing Error/Why/Fix patterns; fork Fix text from the Brief | S:65 R:90 A:75 D:65 |
| 11 | Certain | Tests use a stub `gh` shell script first on PATH (unit + binary-driven); missing-gh case uses a PATH holding only a `git` symlink | Brief prescribes the PATH stub; Constitution IV wants the real exec path exercised | S:80 R:90 A:80 D:75 |
| 12 | Certain | `wt help-dump` needs no fixture update (no golden files exist; output derives from the cobra tree) — only verify `--pr` appears | Checked: no testdata/golden for help-dump in the repo | S:70 R:90 A:85 D:85 |
| 13 | Certain | 15 s bound implemented with `exec.CommandContext` + `context.WithTimeout`, following `internal/update`'s bounded-exec idiom where it reads cleanly | Brief fixes 15 s; existing idiom in-repo | S:70 R:90 A:80 D:75 |

13 assumptions (10 certain, 3 confident, 0 tentative, 0 unresolved).
