---
type: memory
description: "The `wt create` branch-selection contract — positional = new-branch-only, `--checkout` = existing-branch opt-in, `--pr <n>` = a same-repo PR's head via gh, `--base` = new-branch start-point pinned to a SHA with upstream restored (conflicts with `--checkout` and `--reuse`), `--fetch` for a fresh base, `--temp <ref>` = detached throwaway worktree, the conflict/exit-code matrix, and the internal seam (sentinel errors, LIFO rollback)."
---
# wt-cli: Create Branch-Selection Contract

> Post-implementation behavior capture for `wt create`'s branch-selection surface.
> Source changes: `260717-2af2-explicit-base-checkout-flags`, `261123-gcos-create-temp-detached-worktree`, `261123-xhgg`.

This file documents how `wt create` decides which branch a new worktree lands on: the
new-branch-only positional, the `--checkout` existing-branch opt-in, the `--pr <n>`
pull-request head resolver in front of it, the `--base` new-branch start-point, the `--temp`
detached throwaway mode, the conflict/exit-code matrix, and the `internal/worktree` seam
(`CreateNewBranchWorktree` / `CheckoutBranchWorktree` / `CreateTempWorktree` + the sentinel
errors) that keeps the existence business-rule out of `cmd/`. Future changes touching `src/cmd/wt/create.go` or the
create/checkout functions in `src/internal/worktree/crud.go` should preserve these invariants
unless an explicit spec amendment supersedes them.

**Why this contract exists.** Create-new and checkout-existing differ wildly in consequence: a
user typing `wt create --name foo sockets-v2` intending a scratch worktree would otherwise land
*on* `sockets-v2` itself, one `git commit` away from polluting a shared / collector branch. The
two operations are therefore explicit modes: the dangerous direction (check out an existing
branch) is an opt-in (`--checkout`) and the safe common path (`wt create <new-branch>`) stays
short and fails closed on an existing name (260717-2af2).

## Requirements

### The positional `[branch]` names a NEW branch only

- `wt create <branch>` treats the positional as the name of a **new** branch to create (off
  `--base`, else HEAD). Its signature is `Use: "create [branch]"`,
  `Args: cobra.MaximumNArgs(1)`; the positional has exactly this one meaning.
- If `<branch>` already exists **locally OR remotely**, the command exits `ExitInvalidArgs` (2)
  **before any worktree mutation**, pointing at `--checkout`. Copy (via
  `ExitWithError(what, why, fix)`):
  - what: `Branch '<branch>' already exists`
  - why: `The positional argument only creates new branches`
  - fix: `To put a worktree on the existing branch: wt create --checkout <branch>`
- Remote-only is deliberately included in the existence check — a remote-only shared branch is
  the exact danger case the change targets.
- The bare-create path (no positional, no `--checkout`) is unchanged: an exploratory worktree on
  a new branch named after the (random or `--name`) worktree name, `--base` honored.

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
  fetched via `FetchRemoteBranch` (`git fetch origin <branch>:<branch>`) then checked out.
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

### `--pr <n>` checks out a same-repository pull request's head branch

- `wt create --pr <n>` resolves PR `<n>`'s head with
  `gh pr view <n> --json headRefName,isCrossRepository` (argv exec, 15 s timeout, run in the
  current directory) and then runs the `--checkout <headRefName>` path **unchanged**: local
  as-is / remote-only fetch-then-checkout, suggested name `DeriveWorktreeName(<head>)`, the
  init-confirm gate, init, Open, rollback, the stdout path line. `--pr` is a resolver in front of
  `--checkout`, not a separate checkout implementation.
- `<n>` must be a positive decimal number (`^[1-9][0-9]*$`); the check keys on the flag being
  **set** (`Flags().Changed("pr")`), so an explicit empty `--pr ""` (an unset `$PR` in a script)
  is rejected rather than falling through to a bare create. Copy: `Invalid --pr value: <v>` /
  `--pr takes a pull request number` / `Example: wt create --pr 42` → `ExitInvalidArgs` (2).
- The lookup runs after `ValidateGitRepo` / `GetRepoContext` and **before** the dirty-state menu,
  the name prompt and any mutation, so every lookup failure leaves nothing behind:

  | Lookup outcome | Exit | Error copy (what) |
  |---|---|---|
  | `gh` not on PATH (`ErrGhNotFound`) | `ExitGeneralError` (1) | `GitHub CLI (gh) not found` — fix names the install URL and `--checkout` |
  | `isCrossRepository: true` (`ErrPRCrossRepository`) | `ExitInvalidArgs` (2) | `PR #<n> is from a fork (head branch '<head>')` — fix: `Add the fork as a remote and use --checkout` |
  | gh exits non-zero / times out / malformed output | `ExitGeneralError` (1) | `Could not resolve PR #<n>`; why = gh's first non-empty stderr line |
  | head branch neither local nor on origin (`ErrBranchNotFound`) | `ExitInvalidArgs` (2) | `PR #<n> head branch '<head>' not found` |

- `gh` is an **optional** runtime dependency, consulted only when `--pr` is passed; every other
  path still needs only `git`.
- **Repository assumption**: gh resolves the PR in *its* default repository (an `upstream`
  remote or `gh repo set-default` wins), while the checkout looks the head up locally and on
  `origin`. `--pr` therefore assumes gh's repository is `origin`; in a fork clone (origin = fork,
  gh default = upstream) the documented answer is `--checkout`. Detection is deliberately absent.

- **GIVEN** a same-repo PR 7 whose head `feature/x` exists only on origin
- **WHEN** the user runs `wt create --pr 7 --non-interactive`
- **THEN** `feature/x` is fetched and checked out, the summary reads
  `From: PR #7 (existing branch 'feature/x')`, and stdout's last line is the worktree path.

- **GIVEN** a PR whose head is in a fork
- **WHEN** the user runs `wt create --pr 7`
- **THEN** the command exits `ExitInvalidArgs` (2) with the fork copy and creates nothing, even
  when a same-named branch exists on origin.

### `--base <ref>` is the NEW-branch start-point; it always applies to the positional

- `--base` is the git start-point (branch / tag / SHA) for the **new** branch. Because the positional is *always* a new branch, `--base` always applies to it — there is no warn-and-ignore path.
- `--base` is validated by resolving it (see the pinning section below). A `--base` that does not resolve to a commit is `ExitInvalidArgs` (2) with `Invalid --base ref: <ref>`, before any mutation. Both combinations that would leave `--base` with no new branch to apply to (`--checkout` + `--base`, `--reuse` + `--base`) are rejected earlier as argument conflicts, so any `--base` reaching resolution applies to a new branch (positional or bare).

- **GIVEN** `--base <bad-ref>` with a new positional branch and no `--reuse` hit
- **WHEN** the user runs `wt create newbranch --base bad-ref`
- **THEN** the command exits `ExitInvalidArgs` with `Invalid --base ref: bad-ref`, creating no
  worktree or branch.

### `--temp <ref>` creates a DETACHED throwaway worktree

- `wt create --temp <ref>` resolves `<ref>` (branch, tag, or commit SHA) once to a full commit SHA
  via `ResolveCommit` (`git rev-parse --verify --quiet <ref>^{commit}`) and runs
  `git worktree add --detach <path> <sha>` under `<repo>.worktrees/`. It creates **no branch**,
  writes **no git config**, and leaves the ref's own branch un-checked-out (`--temp main` detaches
  at main's tip; it never occupies `main`). Stdout's last line is the worktree path, as for every
  mode.
- Resolution runs in `cmd/create.go` up front, before any prompt or mutation. An unresolvable ref
  exits `ExitInvalidArgs` (2): what `Invalid --temp ref: <ref>`, why `'<ref>' does not resolve to a
  commit`, fix `Provide a valid branch name, tag, or commit SHA`. A `git worktree add` failure
  exits `ExitGitError` (3) with `Failed to create worktree`, and the rollback leaves no directory.
- The worktree name is always `tmp-`-prefixed: no `--name` → `tmp-<adjective>-<noun>` from the
  existing generator (`GenerateUniqueNameWithPrefix`, collision-checked on the prefixed name);
  `--name foo` → `tmp-foo`; `--name tmp-foo` → `tmp-foo` (`TempName` never doubles the prefix).
  The interactive name prompt is skipped. An existing directory at the final name hits the
  ordinary `Worktree '<name>' already exists` error (exit 1); random-name exhaustion is
  `ExitRetryExhausted` (4).
- Init is skipped by default under `--temp`; `--init` opts in (no `Initialize worktree?` confirm —
  the flag is the consent). `--init` without `--temp` is a silent no-op. See
  [`create-output-phases`](/wt-cli/create-output-phases.md) for the phase output and
  [`init-failure-contract`](/wt-cli/init-failure-contract.md) for the failure behavior when
  `--init` runs.
- The dirty-state prompt, Open phase, and rollback/signal machinery behave as for every other
  mode. The rollback registers only `git worktree remove --force <path>` (no `branch -D`; there is
  no branch).

- **GIVEN** a repo with a commit `C` that is not the current HEAD
- **WHEN** the user runs `wt create --temp <C-sha> --non-interactive`
- **THEN** `<repo>.worktrees/tmp-<adj>-<noun>` exists with HEAD at `C`, detached, `git branch
  --list` is unchanged, and stdout's last line is the path.

- **GIVEN** `--temp no-such-ref`
- **WHEN** the user runs it
- **THEN** the command exits `ExitInvalidArgs` (2) with the invalid-ref copy and creates nothing.

### The new-branch start-point is resolved once and pinned to a SHA

- For the positional and bare exploratory modes, `wt create` resolves the start-point **once** — `git rev-parse --verify <base>^{commit}` (`wt.ResolveCommit`) when `--base` is given, else the same for `HEAD` — **before** the create dispatch (outside the reinstall window), and passes that full 40-hex SHA as the `git worktree add -b` start-point. The branch therefore cannot move with the ref mid-command, and the `--json` receipt's `base_sha` is exactly what the branch was cut from.
- Unresolvable `HEAD` (no commits) without `--base` is `ExitGitError` (3) `Failed to resolve HEAD` / `Fix: Make an initial commit first, or pass --base <ref>`, before any mutation.
- The Git-phase summary `From:` line reports the pinned commit: `From: <ref-as-typed> @ <7-char sha>` (see [`create-output-phases`](/wt-cli/create-output-phases.md)).
- **Upstream is restored explicitly.** Passing a SHA to `git worktree add -b` makes git skip its `branch.autoSetupMerge` tracking, which would silently disable `wt delete`'s unpushed-commits guard and `wt list --status` ↑N for a branch cut from a remote-tracking base. `wt.UpstreamForBase(base, branch)` (a pure lookup, resolved pre-dispatch) decides the upstream git would have set, and `wt.SetUpstream` (`git branch --set-upstream-to=<base> <branch>`) applies it after the add:
  - **Remote-tracking base only if refspec-mapped**: a base resolving under `refs/remotes/` counts only when some configured remote's `remote.<r>.fetch` refspec destination (glob `*` on the dst side; `+` prefix and multiple values handled) maps onto the full ref — git neither auto-tracks nor accepts `--set-upstream-to` for a stray `refs/remotes/ghost/main` with no covering refspec, so such a base gets no upstream and the create still exits 0.
  - **`branch.autoSetupMerge` arms**: unset / `true` → a refspec-mapped remote-tracking base is tracked; `always` → a local branch (`refs/heads/…`) base is tracked too; `simple` → a remote-tracking base only when its branch name equals the new branch's; `inherit` → a local base's own upstream is copied when it has one, else none; `false` (also `no`/`off`/`0`) → never.
  - A SHA, tag, `HEAD`, or empty `--base` never tracks.
  - **Rollback**: a set-upstream failure maps to `ExitGitError` (3) — `Failed to set upstream '<upstream>' for branch '<branch>'` / `Why:` git's first output line / `Fix: Check that .git/config is writable (no stale config.lock), then retry` — and rolls the new worktree and branch back (`trackUpstream` executes and disarms the rollback), leaving nothing behind.
- **Known gap**: a create with no `--base` (or `HEAD`) does not replicate git's `always` / `inherit` upstream for a branch-valued HEAD — `UpstreamForBase` returns none for `HEAD`/empty. Default config (`autoSetupMerge` unset) is unaffected, since git never tracks from `HEAD` either.

- **GIVEN** `wt create --non-interactive --name w nb --base origin/main` in a clone
- **WHEN** it succeeds
- **THEN** `git rev-parse --abbrev-ref nb@{upstream}` prints `origin/main` and the branch tip is the pinned SHA; with `--base main` (local) and default config `nb` has no upstream.

### `--fetch` refreshes the base's remote before the base is resolved

- `--fetch` is a long-only bool flag. On a new-branch create (positional or bare exploratory) or a `--temp` create, `wt create` runs `git fetch <remote>` after flag-mutex validation, the git-repo check and the `--pr` lookup, and **before** the `--base` (or `--temp`) ref is resolved, so the pinned SHA comes from fresh refs. The fetch is argv-exec'd (no shell) and its output never reaches stdout.
- **Remote selection** (`wt.RemoteForRef`): the first `/`-separated segment of `--base` (or the `--temp` ref) iff it is a configured remote (`git remote` output), else `origin`. So `--base upstream/main` with an `upstream` remote fetches `upstream`; `--base feature/x` (no `feature` remote), a bare SHA, and no `--base` fetch `origin`. **Known gap**: a fully-qualified `--base refs/remotes/<r>/<branch>` takes `refs` as the first segment, which is not a remote, so it fetches `origin` rather than `<r>`.
- **Failure**: a non-zero fetch is `ExitGitError` (3): `Error: Failed to fetch remote '<remote>'` / `Why:` the trimmed git output / `Fix:` naming `git remote -v` and dropping `--fetch`. Nothing is created — no worktree directory, no branch.
- **No-ops**: `--fetch` with `--checkout` or `--pr` (which runs the checkout path) is accepted and runs no separate fetch (a remote-only checkout already fetches via `FetchRemoteBranch`; a local branch is checked out as-is). `--fetch` with `--reuse` is a usage conflict (see "Conflicting selectors"), so it never reaches the reuse path. Without `--fetch`, `wt create` runs no `git fetch` of its own.
- With no `--base`, `--fetch` fetches `origin` and the base stays `HEAD`.

- **GIVEN** a clone whose `origin/main` is one commit behind the bare origin
- **WHEN** `wt create --base origin/main --fetch` runs
- **THEN** `git fetch origin` runs first and the new branch's tip is the origin's newest commit; without `--fetch` it is the stale commit.

### `--json` forces non-interactive create and open skip

`--json` (the stdout receipt document — see [`create-output-phases`](/wt-cli/create-output-phases.md)) implies `--non-interactive` and defaults the open phase to `skip`. An explicit `--open` / `--worktree-open` other than `skip` together with `--json` is `ExitInvalidArgs` (2) before any mutation. Under `--json` the `--reuse` hit path reports `mode:"reuse"` with `base_ref`/`base_sha` null and the existing worktree's current branch.

### Conflicting selectors are hard `ExitInvalidArgs` errors

Flag combinations that select contradictory modes each exit `ExitInvalidArgs` (2) — matching
the documented exit-2 class "mutually exclusive flags". All are pure argument-parsing checks
(no git state), validated in `cmd/create.go` **before** any git work (before
`ValidateGitRepo`, rollback setup, branch-name and `--base` ref validation). The `--temp` and
`--init` conflicts are held in `checkModeConflicts`, which runs first; the `--checkout` and
`--reuse` + `--base` / `--reuse` + `--fetch` checks run after the `--reuse requires --name` check, and the `--pr` checks after those:

- `--checkout` + a positional arg:
  - what: `--checkout cannot be combined with a positional branch argument`
  - why: `The positional creates a new branch; --checkout checks out an existing one`
  - fix: `Use one of: wt create <new-branch> | wt create --checkout <existing-branch>`
- `--checkout` + `--base`:
  - what: `--base cannot be combined with --checkout`
  - why: `--base is the start-point for a NEW branch; --checkout targets an existing branch`
  - fix: `Drop --base, or create a new branch: wt create <name> --base <ref>`
- `--reuse` + `--base`:
  - what: `--reuse and --base cannot be combined`
  - why: `--reuse keeps the existing worktree's branch, so a start-point has nothing to apply to`
  - fix: `Drop --base, or drop --reuse to create a new worktree`

  The `--reuse` + `--base` conflict is **unconditional on name collision**: it fires whether or
  not a worktree named `--name` exists, so the outcome never depends on filesystem state the
  caller cannot see in its own argv. A non-empty `--base` is the trigger (an explicit
  `--base ""` reads as unset, like the `--checkout` + `--base` check).
- `--reuse` + `--fetch` (checked right after `--reuse` + `--base`, so passing all three reports
  the `--base` conflict):
  - what: `--reuse and --fetch cannot be combined`
  - why: `--reuse keeps the existing worktree's branch, and a new worktree from --reuse starts at HEAD, so there is no base for --fetch to refresh`
  - fix: `Drop --fetch, or drop --reuse to create a new worktree (wt create --name <name> --base origin/<branch> --fetch)`

  Also **unconditional on name collision**, and it fires before any network call: on a reuse
  hit the fetch could never apply, and on a reuse miss the new branch starts at `HEAD` (`--base`
  being banned with `--reuse`), which a fetch never moves — so the flag could only be silently
  ignored or a wasted network call.
- `--pr` excludes every other branch selector — each check exits 2 before `gh` runs, after the
  `--reuse requires --name` check:
  - `--pr` + positional: `--pr cannot be combined with a positional branch argument`
  - `--pr` + `--base`: `--base cannot be combined with --pr`
  - `--pr` + `--checkout`: `--pr cannot be combined with --checkout`
  - `--pr` + `--reuse`: `--pr cannot be combined with --reuse` (`--reuse` keeps an existing
    worktree's branch, so there is no PR head to check out)

- `--temp` + a positional arg:
  - what: `--temp cannot be combined with a positional branch argument`
  - why: `--temp creates a detached worktree with no branch; the positional names a new branch`
  - fix: `Use one of: wt create <new-branch> | wt create --temp <ref>`
- `--temp` + `--checkout`:
  - what: `--temp cannot be combined with --checkout`
  - why: `--temp creates a detached worktree; --checkout puts a worktree on an existing branch`
  - fix: `Use one of: wt create --checkout <branch> | wt create --temp <ref>`
- `--temp` + `--reuse`:
  - what: `--temp cannot be combined with --reuse`
  - why: `A temp worktree is always freshly created; --reuse returns an existing one`
  - fix: `Drop --reuse, or reuse by name without --temp: wt create --reuse --name <name>`
- `--temp` + `--base`:
  - what: `--temp cannot be combined with --base`
  - why: `--temp's <ref> is already the commit to detach at; --base is the start-point for a NEW branch`
  - fix: `Pass the commit to --temp directly: wt create --temp <ref>`
- `--temp` + `--pr` (rejected before `--pr`'s own validation, so `gh` never runs):
  - what: `--temp cannot be combined with --pr`
  - why: `--pr checks out the PR's head branch; --temp detaches at a commit`
  - fix: `For a detached worktree at a PR head, pass its SHA: wt create --temp <sha>`
- `--init` + `--no-init`:
  - what: `--init cannot be combined with --no-init`
  - why: `--init opts in to the init script for --temp; --no-init skips it`
  - fix: `Pass one of --init or --no-init`

The check order is `--init`×`--no-init` first, then (when `--temp` is set) positional,
`--checkout`, `--reuse`, `--base`; the first match exits.

- **GIVEN** `--checkout` and a positional arg
- **WHEN** the user runs `wt create --checkout foo bar`
- **THEN** the command exits `ExitInvalidArgs` (2) with the combined-with-positional copy.

- **GIVEN** `--checkout` and `--base`
- **WHEN** the user runs `wt create --checkout foo --base main`
- **THEN** the command exits `ExitInvalidArgs` (2) with the combined-with-`--base` copy.

- **GIVEN** `--reuse --name X --base <ref>`, whether or not worktree `X` exists
- **WHEN** the user runs it
- **THEN** the command exits `ExitInvalidArgs` (2) with the `--reuse and --base` copy, nothing on
  stdout, no worktree or branch created, and an existing `X` untouched (no reuse, no init).

- **GIVEN** `--reuse --name X --base <bad-ref>`
- **WHEN** the user runs it
- **THEN** it reports the conflict, not `Invalid --base ref` (the conflict check runs first).

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
| `wt create --pr <n>` (same-repo, head exists) | pr | worktree on the PR's head branch (the `--checkout` path) |
| `wt create --pr <n>` (fork / bad `<n>` / head missing) | pr | `ExitInvalidArgs` (2) |
| `wt create --pr <n>` (gh missing / failing) | pr | `ExitGeneralError` (1) |
| `wt create --pr <n>` + positional / `--base` / `--checkout` / `--reuse` | conflict | `ExitInvalidArgs` (2) |
| `wt create --temp <ref>` | temp | detached worktree `tmp-*` at `<ref>`'s commit; no branch/config; init skipped unless `--init` |
| `wt create --temp <bad-ref>` | temp | `ExitInvalidArgs` (2), invalid `--temp` ref |
| `wt create --temp X <pos>` / `--checkout` / `--reuse` / `--base` / `--pr` | conflict | `ExitInvalidArgs` (2) |
| `wt create --init --no-init` | conflict | `ExitInvalidArgs` (2) |
| `wt create <new> --base <bad>` | positional, new | `ExitInvalidArgs` (2), invalid `--base` |
| `wt create --reuse --name X --base Y` | conflict | `ExitInvalidArgs` (2), with or without an existing `X` |
| `wt create --reuse --name X --fetch` | conflict | `ExitInvalidArgs` (2), with or without an existing `X`; no fetch runs |
| `wt create --reuse --base Y` (no `--name`) | conflict | `ExitInvalidArgs` (2), `--reuse requires --name` (checked first) |
| `wt create --reuse --name X …` (no `--base` / `--fetch`) | reuse | reuse on collision; branch selectors NOT consulted |
| `wt create --fetch …` (fetch fails) | positional / bare | `ExitGitError` (3), nothing created |
| `wt create --json --open <non-skip>` | conflict | `ExitInvalidArgs` (2) |

`ExitInvalidArgs` = 2, `ExitGitError` = 3 (`git worktree add` failure, including `--detach`), both from
`src/internal/worktree/errors.go`.

### The creation mode is visible at creation time in the summary output

The creation modes are **visually distinguished in the Git-phase summary output** by a
fourth `From:` line — so a user can tell from the output which of this file's modes ran and what
ref the branch was based on (the exact create-vs-checkout confusion this contract's flag split
targets, surfaced at the output layer too) (260717-aeka-create-summary-base-ref). This is
*display* only — the `From:` value is derived from the same `checkout` / `base` inputs and the
mode dispatch is exactly as defined above:

| This file's mode | `From:` summary value |
|------------------|-----------------------|
| positional new / bare, `--base` given | `<ref as typed> @ <7-char sha>` (the pinned commit) |
| positional new / bare, no `--base` | the resolved HEAD label (`DescribeHead()` — current branch, or short SHA when detached) |
| `--checkout <branch>` (local or remote-only) | `existing branch '<branch>' (checked out directly)` |
| `--pr <n>` | `PR #<n> (existing branch '<head>')` |
| `--temp <ref>` | `<ref as typed> @ <7-char short sha>` (the resolved commit; `Branch:` reads `(detached)`) |
| `--reuse` | *(no summary block — no `From:` line)* |

The line's full contract (stderr-only, pre-dispatch resolution outside the reinstall window, the
best-effort `DescribeHead()` helper) lives in [`create-output-phases`](/wt-cli/create-output-phases.md)
"The Git-phase summary block carries a fourth `From:` line".

### `--reuse` keeps the existing worktree as-is

`--reuse` requires `--name` (the worktree-name flag; its deprecated alias `--worktree-name`
also satisfies the requirement — see
[flag-naming-conventions](/wt-cli/flag-naming-conventions.md)), and on
name collision it reuses the existing worktree **without consulting any branch selector**
(positional / `--checkout`). The reuse hit is computed up front (the final name is known because `--reuse` requires `--name`) and sits *before* the base resolution and the branch-dispatch
switch, so a reused worktree's branch is never re-derived. `--base` and `--fetch` are not flags `--reuse` can ignore —
each combination is a hard conflict (see "Conflicting selectors" above). (Init-on-reuse remains
warn-but-continue — see `/wt-cli/init-failure-contract.md`.)

- **GIVEN** `--reuse` without `--name` (or the deprecated `--worktree-name`)
- **WHEN** the user runs it
- **THEN** the command exits `ExitInvalidArgs` with `--reuse requires --name`.

- **GIVEN** `--reuse --name X` where worktree `X` already exists
- **WHEN** the user runs it
- **THEN** the existing worktree is reused; no branch selector is read.

### The init-confirm gate fires whenever a branch was explicitly named

The interactive "Initialize worktree?" confirm fires when a branch was explicitly named —
positional (new branch) **or** `--checkout` / `--pr` (existing branch) — and is skipped for the bare
exploratory create. The gate is `!(nonInteractive || (branchArg == "" && checkout == ""))`. Under `--temp` init is
instead gated by `--init` alone and the confirm never fires.

- **GIVEN** an interactive `wt create --checkout foo` (foo exists)
- **WHEN** init would run
- **THEN** the "Initialize worktree?" confirm is shown.

- **GIVEN** an interactive bare `wt create` (no positional, no `--checkout`)
- **WHEN** init would run
- **THEN** the confirm is skipped.

### The `internal/worktree` seam: two mode-explicit functions + sentinel errors

Mode-explicit functions in `src/internal/worktree/crud.go` own the existence dispatch, so
the branch-existence business rule lives in `internal/` and `cmd/` only routes flags
(Constitution V). There is deliberately no combined create-or-checkout entry point.

- `CreateNewBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint, upstream string) (string, error)`
  — returns the sentinel `ErrBranchExists` and creates nothing when `BranchExistsLocally(branch)
  || BranchExistsRemotely(branch)`; otherwise runs the new-branch path (`git worktree add -b`
  from `startPoint` — a pinned SHA), applies `upstream` (when non-empty) via `trackUpstream`, and registers **both** rollbacks: worktree removal AND
  `git branch -D <branch>` (the branch is newly created, so it must be torn down on rollback). Rollback runs **LIFO**, so `registerNewBranchRollback` registers `git branch -D` **before** `git worktree remove --force`: the worktree is removed first, then the branch (git refuses to delete a branch still checked out in a worktree). `CreateExploratoryWorktree` registers in the same order.
- `CheckoutBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback) (string, error)`
  — takes **no** `startPoint` (a checkout of an existing branch has no start-point). Local branch
  → checkout as-is; remote-only → `FetchRemoteBranch` then checkout; neither → returns the
  sentinel `ErrBranchNotFound` and creates nothing. Registers **only** worktree removal on
  rollback — the branch pre-existed and must NOT be deleted.
- `ErrBranchExists` and `ErrBranchNotFound` are package-level `errors.New` sentinels declared in
  `crud.go`. `cmd/create.go` maps them via `errors.Is`: `ErrBranchExists` → the R1 positional
  `ExitInvalidArgs` copy; `ErrBranchNotFound` → the R2 `--checkout` `ExitInvalidArgs` copy. Any
  **other** error from either function maps to `ExitGitError` ("Failed to create worktree").
- `CreateTempWorktree(sha, name string, ctx *RepoContext, rb *Rollback) (string, error)` — takes the
  **already-resolved** commit SHA (callers resolve with `ResolveCommit` in `git.go`, which also
  powers `ShortSHA` for the summary label), runs `EnsureWorktreesDir` then
  `git worktree add --detach <path> <sha>`, and registers **only** worktree removal on rollback.
  It writes no git config. Name helpers (`TempNamePrefix = "tmp-"`, `TempName`,
  `GenerateUniqueNameWithPrefix`, `IsTempWorktree`) live in `names.go`.
- `CreateExploratoryWorktree(name, ctx, rb, startPoint, upstream)` (the bare path) takes the same pinned `startPoint` and `upstream`. The `git.go` helpers `RemoteForRef`, `FetchRemote`, `ResolveCommit`, `ShortSHA`, `UpstreamForBase`, and `SetUpstream` (with its `*SetUpstreamError`) serve the base pipeline; `BranchExistsLocally` / `BranchExistsRemotely` / `FetchRemoteBranch` are reused unchanged. All of these git operations live in `internal/worktree` (Constitution V); `cmd/create.go` only orchestrates and maps errors.

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
`wt create --non-interactive --reuse --worktree-name <name> <branch>`, written against the
pre-split create-or-checkout dual semantics. (`--worktree-name` is the permanent deprecated
alias of `--name`; the call parses and behaves identically but prints a one-line deprecation
warning to stderr, harmless to the machine-parsed stdout.)
That call exits `ExitInvalidArgs` (2)
whenever the branch already exists and the worktree does not (e.g. worktree deleted, branch
kept) — the positional rejects an existing branch. This is a **deliberate hard
break** (260717-2af2), no deprecation window: the fab-kit migration (an existence probe + routing to
`--checkout` vs. the positional) is a coordinated follow-up **in the fab-kit repo**, out of
scope for this change. The typed error + fix hint is the migration signal. `fab batch new`
(no positional) is unaffected. This was accepted at intake because the single known external
consumer is owned by the same author.

## Design Decisions

### `--pr` is a resolver in front of `--checkout`
**Decision**: `--pr <n>` only turns `<n>` into a branch name via `gh pr view` (in
`internal/worktree/pr.go`: `ResolvePRHead`, `ErrGhNotFound`, `ErrPRCrossRepository`) and then
sets the `--checkout` target; every later phase is the existing checkout path.
**Why**: plan `fab/plans/sahil/26-11-23-worktree-base-receipts.md` (step W4) — reuse the tested
remote-only checkout path instead of the hand-rolled `gh` + `git worktree add` callers wrote;
Constitution V keeps the `gh` exec in `internal/`.
**Rejected**: fetching `refs/pull/<n>/head` (a detached commit, not the PR's branch); calling the
GitHub API directly (auth handling and a dependency — `gh` is the toolkit's GitHub seam).
*Introduced by*: 261123-c3ek-create-pr-head-worktree

### Fork PRs are refused, and the gh-repo = origin assumption is documented, not detected
**Decision**: `isCrossRepository: true` exits 2 pointing at "add the fork as a remote and use
--checkout"; the case where gh's default repository is not `origin` (a fork clone) is a
documented limitation, and the gh argv stays `--json headRefName,isCrossRepository`.
**Why**: a fork's head is not on `origin`, so checking out a same-named `origin` branch would be
the wrong code; `wt` does not manage remotes (plan scope, Constitution I). Detecting the
repo mismatch would need origin-URL parsing against gh's repository for a rare setup.
**Rejected**: auto-adding the fork as a remote (hidden state); requesting
`headRepositoryOwner` and comparing it to a parsed origin URL (complexity beyond the W4 Brief).
*Introduced by*: 261123-c3ek-create-pr-head-worktree

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
**Decision**: the `--checkout`+positional, `--checkout`+`--base` and `--reuse`+`--base`
conflicts are validated in `cmd/create.go` (next to the `--reuse requires --name` check), while
branch-existence checks live in `internal/`.
**Why**: the mutex conflicts are pure argument-parsing concerns with no git state; the existence
checks are worktree business rules with git dependencies. Splitting them along that line keeps
each concern where it belongs.
**Rejected**: pushing the mutex checks into `internal/` (they have no git dependency, so they do
not belong with the worktree logic).
*Introduced by*: `260717-2af2-explicit-base-checkout-flags`.

### `--reuse` + `--base` is a hard conflict, not silent precedence
**Decision**: `wt create --reuse --base <ref>` exits `ExitInvalidArgs` (2) before any git work,
unconditionally (not only on a name collision), mirroring the `--checkout` + `--base` conflict.
**Why**: silent precedence let a caller believe it got a worktree cut from `<ref>` when it got an
existing worktree on whatever base it had — and the `--json` receipt / recorded base planned
alongside report no base for a reuse, so the mismatch would only surface downstream (a PR diff
full of unrelated commits). `--base` now only ever pairs with creating a new branch. Plan
`fab/plans/sahil/26-11-23-worktree-base-receipts.md`, step W3.
**Rejected**: keep `--reuse` precedence (the documented gotcha); warn-and-ignore (scripts proceed
on a wrong assumption); reject only on collision (the outcome would depend on filesystem state
the caller cannot see from its own argv).
*Introduced by*: `261123-udcv-reuse-base-usage-error`.

### Pin the new-branch start-point to a resolved SHA, restoring upstream explicitly

**Decision**: the start-point is always `rev-parse --verify <base|HEAD>^{commit}`, resolved once pre-dispatch and passed as a SHA to `git worktree add -b`; the upstream git would have auto-set from a ref name is computed pre-dispatch (`UpstreamForBase`) and written after the add (`SetUpstream`), inside the rollback scope.
**Why**: the branch cannot move with the ref mid-command, and the receipt's `base_sha` is exactly the commit the branch was cut from. A SHA start-point loses git's `branch.autoSetupMerge` tracking, which silently disabled `wt delete`'s unpushed-commits guard and `wt list --status` ↑N, so the tracking is restored per the same config arms and refspec-mapping rule git applies — a data-safety requirement, not a nicety.
**Rejected**: passing the ref name (racy; `base_sha` could disagree with the branch); dropping the upstream (a data-safety regression); setting upstream for any `refs/remotes/` ref (git rejects a ref no remote's refspec maps — would turn a git exit 0 into exit 3); resolving the upstream decision after the add (a lookup has no reason to sit inside the post-add window).
*Introduced by*: `261123-xhgg-create-fetch-json-receipt` (plan `fab/plans/sahil/26-11-23-worktree-base-receipts.md`, step W1).

### `--fetch` runs before base resolution and fails closed

**Decision**: `--fetch` runs `git fetch <RemoteForRef(base)>` before the base is resolved, as a flag rather than default behavior; a failed fetch is `ExitGitError` with nothing created; it is a no-op for `--checkout` / `--pr`, and `--reuse` + `--fetch` is a usage conflict (W3's brief; it could only be silently ignored on a reuse hit or a wasted network call on a reuse miss starting at `HEAD`).
**Why**: a branch cut from a stale `origin/main` is the failure the flag exists to prevent, so the fetch must precede the pin; failing closed keeps a user who asked for fresh refs from silently getting stale ones. Making it opt-in keeps `wt create` offline-capable and byte-compatible for existing callers (a default fetch would add a network dependency to every create).
**Rejected**: fetching by default (network dependency on every create); falling back to the local ref when the fetch fails (silently stale); fetching after resolution (pins the stale SHA).
*Introduced by*: `261123-xhgg-create-fetch-json-receipt` (plan `fab/plans/sahil/26-11-23-worktree-base-receipts.md`, step W1).

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

### `--temp` is its own mode: detached, `tmp-`-named, init off by default
**Decision**: `--temp <ref>` creates a detached worktree (`git worktree add --detach <path> <sha>`)
with a `tmp-`-prefixed name, no branch, and no git config, and skips init unless `--init` is
passed. Plan `26-11-23-worktree-base-receipts.md` W5 binds the mode; the init default resolves
that plan's open question.
**Why**: surgery worktrees (conflict resolution, restacking, inspecting a commit) must not be able
to pollute a branch and must be visibly disposable; the prefix makes them so in every listing and
lets `wt list` mark them without extra git calls. Init is skipped because `fab sync` would write
files into the very tree being operated on, and init is the slowest create phase.
**Rejected**: running init as usual (every throwaway pays for and is dirtied by init, forcing
`--no-init` on every call); recording the base in git config (the plan's recorded-base step
excludes `--temp`).
*Introduced by*: `261123-gcos-create-temp-detached-worktree`.

### `--temp` + `--base` is a usage error, not silent precedence
**Decision**: the combination exits `ExitInvalidArgs` (2) even though the plan's W5 exclusive
list names only the positional, `--checkout`, `--reuse`, and `--pr`.
**Why**: `--temp`'s value already is the commit to detach at; `--base` is the start-point for a
*new* branch, and `--temp` creates none. Letting one silently win is the gotcha the plan's
`--reuse`+`--base` rule removes elsewhere.
**Rejected**: ignoring `--base`, or letting it override `<ref>`.
*Introduced by*: `261123-gcos-create-temp-detached-worktree`.

## Cross-references

- Spec doc: [`docs/specs/cli-surface.md`](../../specs/cli-surface.md) § `wt create` (flags table
  incl. `--checkout`, new-branch-only positional, exit-code notes) and
  [`docs/specs/worktree-layout.md`](../../specs/worktree-layout.md) § Branch ↔ worktree
  relationship (case 1 new-branch, case 2 keyed to `--checkout`).
- Source: `src/cmd/wt/create.go` (`--checkout` / `--fetch` / `--json` flags, the three flag-mutex checks, the up-front reuse-hit computation, the `--fetch` step, the pre-dispatch base resolution to a pinned SHA + upstream lookup, `ValidateBranchName` on positional-or-`--checkout`, `DeriveWorktreeName`
  suggestion, the create-dispatch switch mapping the sentinels, the generalized init-confirm
  gate, the `Long` help text); `src/internal/worktree/crud.go`
  (`CreateNewBranchWorktree` / `CheckoutBranchWorktree` / `CreateTempWorktree`,
  `ErrBranchExists` / `ErrBranchNotFound`, `CreateExploratoryWorktree`, `registerNewBranchRollback`,
  `trackUpstream`); `src/internal/worktree/names.go` (`TempNamePrefix`, `TempName`,
  `GenerateUniqueNameWithPrefix`, `IsTempWorktree`); `src/internal/worktree/git.go`
  (`BranchExistsLocally` / `BranchExistsRemotely` / `FetchRemoteBranch` reused; `ResolveCommit` /
  `ShortSHA` shared by `--temp` and the base pin; `RemoteForRef`, `FetchRemote`, `UpstreamForBase`,
  `SetUpstream` — `261123-xhgg`);
  `src/internal/worktree/context.go` (`ValidateBranchName`, `DeriveWorktreeName`);
  `src/internal/worktree/errors.go` (`ExitInvalidArgs`, `ExitGitError`, `ExitWithError`).
- `--pr` source and tests: `src/internal/worktree/pr.go` (`ResolvePRHead`, `PRHead`,
  `ErrGhNotFound`, `ErrPRCrossRepository`, the 15 s `prLookupTimeout`) with
  `src/internal/worktree/pr_test.go` (stub `gh` on PATH); `src/cmd/wt/create_test.go`
  (`TestCreate_PR*` — local head + gh argv, invalid/empty value, the four conflicts, fork, gh
  missing, gh failure, head not found) and `src/cmd/wt/integration_test.go`
  (`TestIntegration_CreatePR_RemoteOnlyHead`), via the `writeGhStub` helper in
  `src/cmd/wt/testutil_test.go`.
- Tests: `src/cmd/wt/integration_test.go` (`261123-xhgg`: bare-origin + clone fixture proving `--fetch` picks up a commit pushed after the clone's last fetch, the stale commit without it, fetch failure exit 3 with nothing created, `--fetch --checkout` no-op); `src/cmd/wt/create_test.go` (`261123-xhgg`: base pin, upstream restoration, `ghost/main` stray remote-tracking ref, set-upstream failure rollback, the `--reuse` + `--base` conflict as a `--json` failure document; positional-existing local/remote → exit 2, `--checkout`
  local/remote-only success, `--checkout` missing → exit 2, `--checkout`+`--base` → exit 2,
  `--checkout`+positional → exit 2, `--reuse`+`--fetch` → exit 2 with/without collision, no fetch attempted (`TestCreate_ReuseWithFetchConflict`, incl. the `--json` document); `--reuse`+`--base` → exit 2 with/without collision and with
  an invalid ref, `--reuse`+`--base` without `--name`; the removed `--base`-with-existing-branch
  warn tests; `--temp` happy path / naming / conflicts / bad ref / init gate);
  `src/cmd/wt/integration_test.go` (`TestIntegration_BranchDeletePreservesOthers` on an existing
  branch via `--checkout`; `TestIntegration_ReuseWithBaseConflict` end-to-end); `src/cmd/wt/edge_test.go` (`TestEdge_BranchWithSpecialChars` via
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
  `--checkout`, and bare); it also owns the fourth `From:` summary line that
  *names* which of this file's modes ran (base ref verbatim / resolved HEAD / existing-branch copy) (260717-aeka). [`init-failure-contract`](/wt-cli/init-failure-contract.md) — the
  kept-worktree / `ExitInitFailed` / open-anyway / SIGINT-Option-B / foreground-reclaim behavior
  that runs identically on the `--checkout` path and the positional-new path; also the source of
  the `--reuse` init warn-but-continue exemption. [`help-dump-contract`](/wt-cli/help-dump-contract.md)
  — the JSON envelope that carries the `create` help text to shll.ai.
  [`flag-naming-conventions`](/wt-cli/flag-naming-conventions.md) — the flag-surface
  conventions behind the `--name`/`-n` (deprecated `--worktree-name`), `--open`/`-o` (deprecated
  `--worktree-open`), and `--no-init` (deprecated `--worktree-init`) surface and the `wt new`
  command alias on this command (260717-59u8).
