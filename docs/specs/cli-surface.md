# CLI Surface

Per-subcommand reference for the `wt` binary. Source of truth: cobra command
definitions under `src/cmd/wt/`. Exit code constants are defined in
`src/internal/worktree/errors.go`.

Run `wt <command> --help` for the full inline reference.

## Exit codes

| Constant | Value | Meaning |
|----------|-------|---------|
| `ExitSuccess` | 0 | Command completed successfully |
| `ExitGeneralError` | 1 | Non-specific failure (cannot resolve repo context, no default app, unresolved target, etc.) |
| `ExitInvalidArgs` | 2 | Caller supplied incompatible flags or invalid input (bad branch name, bad `--base` ref, mutually exclusive flags) |
| `ExitGitError` | 3 | A `git` invocation failed or the working dir is not a git repository |
| `ExitRetryExhausted` | 4 | Random-name generator could not find a non-colliding name after retries |
| `ExitByobuTabError` | 5 | Failed to open the worktree in a byobu tab |
| `ExitTmuxWindowError` | 6 | Failed to open the worktree in a tmux window |
| `ExitInitFailed` | 7 | The init script ran but exited non-zero (`wt create` keeps the worktree; `wt init` too). Distinct from `ExitGeneralError` so operators can detect "worktree exists, init didn't complete". See [`init-protocol.md`](init-protocol.md). |

Subcommands map domain failures to these codes via `wt.ExitWithError`. SIGINT
during `wt create` exits 130 after rolling back partial state (standard Unix
signal-exit convention).

## `wt create [branch]`

Aliases: `wt new`.

Create a git worktree as a sibling of the main repo (`<repo>.worktrees/<name>/`).

| Flag | Default | Description |
|------|---------|-------------|
| `--name`, `-n <name>` | random adjective-noun | Set the worktree directory name; skips the name prompt. |
| `--open`, `-o <prompt\|default\|skip\|<app>>` | `prompt` (`skip` when `--non-interactive`) | Behavior after creation: show app menu, open in detected default, skip, or open in a specific app (e.g. `code`, `cursor`). Requires an explicit value (no bare form — a bare `--open code` would be parsed as the positional `[branch]`). |
| `--no-init` | (unset — init runs) | Skip the worktree init script (init runs by default). |
| `--reuse` | `false` | If a worktree with `--name` already exists, reuse it instead of erroring. Requires `--name`. Cannot be combined with `--base`. |
| `--non-interactive` | `false` | No prompts; fail or use defaults rather than prompting. |
| `--base <ref>` | (none) | Git start-point (branch / tag / SHA) for the NEW branch. Resolved once to a full commit SHA (`git rev-parse --verify <ref>^{commit}`) and pinned — see "Base pinning" below. Cannot be combined with `--checkout` or `--reuse`. |
| `--fetch` | `false` | Run `git fetch <remote>` before the base (or the `--temp` ref) is resolved, so the worktree is cut from fresh refs. `<remote>` is the first `/`-separated segment of `--base` / `--temp` when that segment is a configured remote, else `origin` (so `--base origin/main` and bare `--fetch` both fetch `origin`; `--base upstream/main` fetches `upstream`). No-op with `--checkout` / `--pr` (the remote-only path already fetches) and when `--reuse` takes the reuse path. Never implicit: without `--fetch`, no extra `git fetch` runs. Long-only. |
| `--json` | `false` | Print one JSON receipt document on stdout instead of the path line — see "`--json` receipt" below. Implies `--non-interactive` and `--open skip`; an explicit `--open <non-skip>` (or `--worktree-open`) is a usage error. Long-only. |
| `--checkout <branch>` | (none) | Check out an EXISTING branch (local as-is, or remote-only fetched then checked out) into the worktree, instead of creating a new one. Cannot be combined with a positional branch argument or with `--base`. |
| `--pr <n>` | (none) | Create the worktree on pull request `<n>`'s head branch, resolved via `gh pr view <n> --json headRefName,isCrossRepository` (15 s timeout), then checked out exactly like `--checkout`. Same-repository PRs only. Cannot be combined with a positional branch argument, `--base`, `--checkout`, `--reuse`, or `--temp`. |
| `--temp <ref>` | (none) | Create a DETACHED throwaway worktree at `<ref>` (branch, tag, or commit SHA) — see [Temp worktrees](#temp-worktrees-wt-create---temp-ref). Cannot be combined with a positional branch argument, `--checkout`, `--reuse`, `--base`, or `--pr`. Long-only. |
| `--init` | `false` | Opt in to the init script for `--temp` (skipped by default for temp worktrees). Without `--temp` it is accepted and is a no-op (init already runs by default). Cannot be combined with `--no-init`. Long-only. |

**Deprecated aliases** (still accepted; hidden from `--help`; print a stderr deprecation warning): `--worktree-name` → `--name`, `--worktree-open` → `--open`, `--worktree-init true|false` → `--no-init` (`--worktree-init false` ≡ `--no-init`).

Positional arg `branch` (optional) — **names a NEW branch only**:

- Omitted: exploratory worktree on a new branch named after the random worktree name.
- Provided, branch does not exist: creates a new branch, optionally from `--base`.
- Provided, branch already exists (locally **or** remotely): the command fails
  with `ExitInvalidArgs` **before any worktree mutation**, pointing at
  `--checkout` (`wt create --checkout <branch>`). The positional never checks
  out an existing branch — that is `--checkout`'s job. Checking out a shared /
  collector branch is now an explicit opt-in, not a silent side effect of a
  name collision.

To put a worktree on an existing branch, use `--checkout <branch>`: a local
branch is checked out as-is, a remote-only branch is fetched then checked out,
and a branch that exists neither locally nor remotely fails with
`ExitInvalidArgs` pointing at the create-new form (`wt create <branch>
[--base <ref>]`). The worktree name is suggested from the branch name via
`DeriveWorktreeName`.

To put a worktree on a pull request's head branch, use `--pr <n>` (a positive
PR number). `wt` asks the GitHub CLI for the head (`gh pr view <n> --json
headRefName,isCrossRepository`, argv exec, 15 s timeout) before any prompt or
mutation, then runs the `--checkout <head>` path unchanged; the summary's
`From:` line reads `PR #<n> (existing branch '<head>')`. `gh` is needed only
for this flag. A PR whose head is in a fork (`isCrossRepository: true`) is
refused with `ExitInvalidArgs` — add the fork as a remote and use
`--checkout`; `wt` does not manage remotes. `gh` resolves the PR in its own
default repository (it prefers an `upstream` remote, or `gh repo set-default`)
while the checkout looks the head up locally and on `origin`, so `--pr` assumes
gh's repository *is* `origin`; in a fork clone (origin = your fork, gh default =
upstream) use `--checkout` instead.

### Temp worktrees: `wt create --temp <ref>`

`--temp <ref>` creates a **detached** worktree at the commit `<ref>` resolves to
— the surface for PR surgery (resolve a conflict, restack, inspect a commit)
that must not pollute any branch.

- `<ref>` is resolved once to a commit SHA (`git rev-parse --verify --quiet
  <ref>^{commit}`) and the worktree is made with `git worktree add --detach
  <path> <sha>` under `<repo>.worktrees/`. A branch name resolves to its tip and
  the branch is **not** checked out. No branch is created and no git config is
  written; the rollback registers only the worktree removal.
- The name is always `tmp-`-prefixed: `tmp-<adjective>-<noun>` when `--name` is
  absent; `--name foo` → `tmp-foo`; `--name tmp-foo` → `tmp-foo` (no double
  prefix). The name prompt is skipped. An existing directory at the final name
  is the usual `Worktree '<name>' already exists` error (`ExitGeneralError`).
- The Git-phase summary (stderr) reads `Branch: (detached)` and
  `From: <ref as typed> @ <7-char sha>`.
- The init script is **skipped by default** (no Init phase output); `--init`
  runs it through the normal runner, with no `Initialize worktree?` confirm. See
  [`init-protocol.md`](init-protocol.md).
- Conflicts exit `ExitInvalidArgs` before any mutation: `--temp` with a
  positional, `--checkout`, `--reuse`, `--base`, or `--pr`; and `--init` with
  `--no-init` (whether or not `--temp` is set). An unresolvable `<ref>` also
  exits `ExitInvalidArgs` (`Invalid --temp ref: <ref>`).
- Temp worktrees show in `wt list` (`temp: true`, see below) and are removed
  with `wt delete <name>`, which does no branch handling for them.

**Base pinning.** For a new branch (positional or bare), the start-point is
resolved once — `<--base>^{commit}`, else `HEAD^{commit}` — before the worktree
is created, and the full 40-hex SHA is what `git worktree add -b` receives. The
branch is therefore cut from exactly the commit reported as `base_sha`, even if
the ref moves mid-command. Passing a SHA would lose the upstream git auto-sets for
a ref name, so `wt` restores it after the add per `branch.autoSetupMerge`: a
remote-tracking `--base` (e.g. `origin/main`, covered by a configured remote's
fetch refspec — a stray `refs/remotes/*` ref no remote maps is left untracked, as
in git) becomes the new branch's upstream (`always` also tracks a local branch;
`simple` tracks a remote-tracking base only when its branch name equals the new
branch's; `inherit` copies a local base branch's own upstream when it has one;
`false` never; a SHA, tag or `HEAD` never) —
this keeps `wt delete`'s unpushed-commits guard and `wt list --status` ↑N working.
A set-upstream failure rolls the create back (exit 3). The Git-phase summary's `From:` line is
`From: <ref-as-typed> @ <short-sha>` (7 characters) when `--base` was given; the
no-`--base` HEAD label and the `--checkout` copy are unchanged.

**`--json` receipt.** Under `--json`, stdout carries exactly one JSON document
(compact, one line, terminated by a newline) on every exit path `RunE`
controls, and nothing else; phase separators, the summary, warnings, init output
and the structured `Error/Why/Fix` stay on stderr.

```
{"ok":true,"result":{"path","name","branch","base_ref","base_sha","mode","init"}}
{"ok":false,"error":{"code":<exit code>,"message":"<the Error: line>"}[,"result":{…}]}
```

- `path` — absolute worktree path; `name` — worktree directory name; `branch` —
  the new branch / `--checkout` or `--pr` head / (reuse) the existing worktree's
  current branch (`null` when unreadable or detached, and always `null` for `temp`).
- `base_ref` — `--base` verbatim, or the `HEAD` label when no `--base`, or the
  `--temp` ref as typed; `base_sha` — the full 40-hex start-point (for `temp`, the
  detached commit). Both are `null` for `checkout` (incl. `--pr`) and `reuse`.
- `mode` — `new` | `checkout` (also `--pr`) | `reuse` | `temp`.
- `init` — `ok` (script ran, exit 0) | `failed` | `skipped` (`--no-init`, or the
  built-in default's not-applicable skip) | `not-found` (the init command/file is
  missing).
- `error.code` is the integer process exit code and `ok` is true iff the exit
  code is 0. An init failure (exit 7, worktree kept) is `ok:false` and also
  carries `result` (with `init:"failed"`) so the kept worktree is discoverable; a
  `--reuse` hit whose init fails stays non-fatal (`ok:true`, `init:"failed"`,
  exit 0). SIGINT/SIGTERM rollback emits `{"ok":false,"error":{"code":130,"message":"interrupted"}}`.
  Cobra-level parse errors raised before `RunE` (unknown flag, more than one
  positional) are out of scope and keep their usual stderr-only behavior.
- `wt list --json` keeps its bare array — only the new `wt create --json` surface
  uses this envelope.

Plan: `fab/plans/sahil/26-11-23-worktree-base-receipts.md` (W1).

Without `--json`, the worktree path is always written as the last line of stdout.
(When the chosen app was `open_here`, the unified shell-cd contract's own
path emission precedes it — the contract is "the path is the last line", not
"exactly one line".)

Exit codes: `ExitInvalidArgs` for flag misuse (including `--checkout` combined
with a positional argument or with `--base`), invalid `--base`/branch name, a
positional naming an already-existing branch, `--checkout` on a branch that
does not exist, a non-numeric `--pr`, `--pr` combined with a positional /
`--base` / `--checkout` / `--reuse`, a fork PR, a PR head branch that exists
neither locally nor on origin, the `--temp` conflicts above, an invalid
`--temp` ref, and `--json` combined with a non-`skip` `--open`;
`ExitGeneralError` when `gh` is missing or
`gh pr view` fails (unauthenticated, PR not found, timeout — the message carries
gh's first error line); `ExitGitError` for `git worktree add` failures
(including `--temp`) and a failed `--fetch` (nothing is created);
`ExitRetryExhausted` for name generation; `ExitInitFailed` (7) when the init
script runs but exits non-zero
(the worktree is kept; the code holds on every init-failure path, including a
successful interactive open-anyway open). Two init outcomes are **not** failures
and exit 0: a graceful skip when the init command/file is missing, and — for the
built-in default `fab sync` only — the default-not-applicable skip when the repo
is not fab-managed. See [`init-protocol.md`](init-protocol.md).

## `wt list`

Aliases: `wt ls`.

List all worktrees for the current repository.

| Flag | Default | Description |
|------|---------|-------------|
| `--path <name>` | (none) | Print only the absolute path for the named worktree. Mutually exclusive with `--json` and `--status`. |
| `--json` | `false` | Emit a JSON array of worktree records. Always emits `name`, `branch`, `path`, `is_main`, `is_current`, `temp`. The `dirty` and `unpushed` keys are present only when `--status` is set. `temp` is `true` for `wt create --temp` worktrees, whose `branch` is `null`. Mutually exclusive with `--path`. |
| `--status` | `false` | Compute and display dirty/unpushed status per worktree. Slower (forks 2 git subprocesses per worktree; parallelized). Mutually exclusive with `--path`. |

Default human output: a table with Name, Branch, and Path columns. The current
worktree is marked with a green asterisk. No per-worktree git invocations
occur — discovery is O(1) regardless of worktree count.

A worktree is **temp** when it is detached, not the main worktree, and its
directory name starts with `tmp-` — computed from the single porcelain listing,
with no extra git invocation. Temp records emit `"branch": null` in `--json`;
every other record (including a hand-made detached worktree, which keeps the
`"(detached)"` string) keeps its branch string. Human output shows `(detached)`
in the Branch column for temp worktrees in every layout.

With `--status`: the table gains a Status column. Dirty worktrees show `*`,
unpushed commits show `↑N`. Enrichment uses a bounded worker pool of
`min(runtime.NumCPU(), 8)` workers; output ordering matches the porcelain
ordering regardless of parallelism.

Exit codes: `ExitInvalidArgs` if `--path` is combined with `--json` or
`--status`; `ExitGitError` if `git worktree list --porcelain` fails;
`ExitGeneralError` if `--path` cannot resolve the name.

## Selection × action model: `wt go` vs `wt open`

The two verbs split along two axes — **selection** (which directory) and
**action** (navigate vs. launch). Each menu lives in exactly one verb: `go`
owns the "which worktree?" menu, `open` owns the "which app?" menu. Composition
is via `wt go --open` (same value grammar as `wt create --open`).

| Invocation | Worktree menu? | App menu? | Result |
|---|---|---|---|
| `wt go` | yes | no | cd to selection |
| `wt go frosty-fox` | no | no | cd directly |
| `wt go --open prompt` | yes | yes | launch selection in chosen app |
| `wt go --open code` | yes | no | launch selection in VS Code |
| `wt open` | no | yes | launch *current* dir (worktree root / repo root / cwd) |
| `wt open <name\|path>` | no | yes | launch that dir |
| `wt open --app code` | no | no | launch current dir in VS Code |

## `wt open [name|path]`

Open a directory in a detected application (editor, terminal, file manager).
`wt open` is the canonical directory **launcher** — external callers (notably
`hop`) MAY delegate to it via subprocess invocation. The full env-var contract
is documented in [`launcher-contract.md`](launcher-contract.md). Worktree
**selection** (picking which worktree) is the job of [`wt go`](#wt-go-name);
`wt go --open` composes the two (select, then launch). `wt open` runs no
worktree-selection menu on any path.

| Flag | Default | Description |
|------|---------|-------------|
| `--app`, `-a <name\|default>` | (none) | Open directly in the named app, skipping the app menu. `default` selects the auto-detected default. Composes with every target form, including the no-arg current-context forms. |
| `--list` | `false` | List the detected host GUI applications and exit — no menu, no launch, **no git repository required** (the branch runs before git-context detection). Human output remains the aligned Id / Label / Kind table over `locus == "gui"` rows in `BuildAvailableApps()` detection order; session/caller/host targets remain in the interactive menu and remain valid `--app` values. When no host apps exist, reports how many other targets are available and points to `--list --json`. Mutually exclusive with a positional target, `--app`, and `--select` (or `--go`). |
| `--json` | `false` | With `--list`, emit the **full unfiltered** app registry as a JSON array in detection order. Every record has `id`, `label`, `kind`, and `locus`; the `DetectDefaultApp()` row alone additionally has `"default": true`. `id` is the exact concrete token `wt open <path> -a <id>` accepts, and the registry is complete in both directions with `--app` resolution for the current environment. An empty input emits `[]` (never `null`) and exits 0. Without `--list`, exits `ExitInvalidArgs`. |

**Deprecated aliases** (still accepted; hidden from `--help`; print a stderr
deprecation warning `use "wt go --open" instead`): `--select` and `--go` — the
former select-then-launch composition flags. Both still perform the full
select-then-launch flow (menu when no name, resolve-by-name when one is given;
require a git repository, else `ExitGitError`; compose with `--app`) until a
later removal change.

Positional arg `[name|path]` — omitted, the **current context** is opened:

- Omitted, called from inside a worktree: opens the current worktree root.
- Omitted, called from the main repo (a non-worktree git cwd): opens the
  **repo root** (tab-named `main`). A one-line transitional stderr tip points
  worktree-picking at the selection verb:
  `tip: to pick a worktree, use wt go (or wt go --open)`.
- Omitted, called from a non-git directory: opens the current working
  directory (equivalent to `wt open .`).
- Existing directory path: treated as a literal path. Works regardless of git
  context — `wt open /tmp/notes` succeeds from any cwd as long as the path is
  a real directory.
- Otherwise: resolved as a worktree name (case-insensitive). **Requires a git
  repository** — name resolution walks the worktree list, which is only
  reachable when the cwd is inside a git repo. The name `main` resolves to the
  main worktree (the repo root); an exact-basename match takes precedence, so a
  worktree directory literally named `main` still resolves to that worktree.

Choosing "Open here" in the app menu (or `--app open_here`) records the target
via the unified shell-cd contract shared with `wt go` — `WT_CD_FILE` write when
set, bare resolved path as the last stdout line, stderr confirmation — see
[`launcher-contract.md`](launcher-contract.md) §3.

Exit codes: `ExitInvalidArgs` when `--list` is combined with a positional
target / `--app` / `--select` (or `--go`), or when `--json` is passed without
`--list` (all `--list`/`--json` validation happens at flag-check time, before
any detection or git work); `ExitGitError` when a git operation fails during
name resolution, or when `--select`/`--go` is invoked from a non-git cwd (the
selection git-repo precondition) — but not for path-only or no-args invocations
from outside a repo; `ExitByobuTabError` / `ExitTmuxWindowError` for
terminal-app failures; `ExitGeneralError` for unknown apps, unresolved targets,
or name args supplied from a non-git cwd. (The former `ExitInvalidArgs`
"`--app` with the main-repo selection menu" case is retired — there is no menu
on that path; `--app` from the main repo opens the repo root.)

## `wt go [name]`

Select a worktree of the current repository and act on it. `wt go` is the
worktree **selector** (the counterpart to `wt open`, the launcher): it owns the
"which worktree?" menu. By default the selection is **navigated** to (the
shell's working directory changes; nothing is launched); with `--open` the
selection is **launched** instead — the selector composing with the launcher,
mirroring `wt create --open`. Navigation uses the same unified `WT_CD_FILE`
shell-cd contract as the launcher's "Open here" option — see
[`launcher-contract.md`](launcher-contract.md) §3.

| Flag | Default | Description |
|------|---------|-------------|
| `--open <prompt\|default\|skip\|<app>>` | (unset — navigate) | What to do with the selection: `prompt` shows the "Open in:" app menu, `default` launches the auto-detected default app, an app name (e.g. `code`, `cursor`, `tmux_window`) launches it directly, `skip` navigates (equivalent to omitting the flag; kept for grammar parity with `wt create --open`). Requires an explicit value (no bare form — a bare `--open code` would be parsed as the positional `[name]`). No short flag. A non-`skip` value replaces navigation with launch — it does not also cd the parent shell (`--open open_here` yields navigation via the unified shell-cd contract). |
| `--non-interactive` | `false` | No prompts. With no name, refuses deterministically (a no-arg selection menu has no non-interactive default) instead of prompting — regardless of `--open`. With a name, an explicit `--open prompt` is honored as-is (mirroring `wt create --open`, where the flag only affects `--open`'s default). |

Positional arg `[name]`:

- Omitted: shows the worktree-selection menu for the current repo. The **main
  worktree is pinned to row 1** (rendered `main (<branch>)`); non-main worktrees
  follow newest-first below it, branch shown per entry. The pre-selected default
  is the newest *non-main* worktree (or the main row when it is the only entry).
  Reachable from anywhere in the repository — the main repo **or** inside another
  worktree. On selection, navigates to (or, with `--open`, launches) the chosen
  worktree. With `--open prompt`, the selection menu and the "Open in:" menu run
  on one shared menu session (single stdin reader).
- Provided: resolved as a worktree name (case-insensitive); acted on directly
  with no worktree menu. The name `main` resolves to the main worktree (the repo
  root); an exact-basename match takes precedence, so a worktree directory
  literally named `main` still resolves to that worktree.

`wt go` always **requires a git repository** — worktree resolution walks the
repo's worktree list. It is scoped to the current repo's worktrees only;
cross-repo navigation is `hop`'s job.

Navigation mechanism (the unified shell-cd contract, shared with "Open here"):
the resolved absolute path is written to `WT_CD_FILE` (when set; mode `0600`,
truncate-on-write) so the `wt shell-init` wrapper cd's the parent shell there,
**and** is printed to stdout as the last line so the no-wrapper scripting form
works: `cd "$(command wt go some-name)"`. When `WT_CD_FILE` is unset and
`WT_WRAPPER` is not `1`, the "shell wrapper not loaded" hint applies. `wt go`
never cd's the parent shell directly.

Exit codes: `ExitGitError` (3) when the cwd is not in a git repository or
`git worktree list` fails; `ExitGeneralError` (1) for an unknown worktree name,
for a no-arg invocation under `--non-interactive`, or — via `--open` — for an
unknown app / no detected default app; `ExitByobuTabError` (5) /
`ExitTmuxWindowError` (6) when a `--open` launch into a byobu tab / tmux
window-or-session fails (the launcher exit codes, gained through the `--open`
composition).

## `wt delete [worktree-names...]`

Aliases: `wt rm`.

Delete one or more worktrees with optional branch cleanup.

| Flag | Default | Description |
|------|---------|-------------|
| `--branch <true\|false\|auto>` | `auto` | Delete the associated local branch. `auto` deletes only when the branch name matches the worktree name. Stays a string tri-state (`auto` is a genuine third value). |
| `--no-remote` | (unset — remote deleted) | Do NOT delete the branch on the origin remote when the local branch is deleted (the remote branch is deleted by default). |
| `--all`, `-a` | `false` | Delete every worktree (skips the current selection logic). |
| `-s`, `--stash` | `false` | Stash uncommitted changes in the worktree before deleting. |
| `--dry-run` | `false` | Preview what would be deleted without making any change and without confirmation prompts. All decision logic (target resolution, `--branch` auto rule, remote-existence check, hazard detection) runs live; only the mutations are suppressed and replaced by `Would …` lines on stdout under a `Dry run — no changes will be made.` header. Long-only (no short flag). Applies to every target-resolution path; the selection menu still shows (selection is not consent), and the non-interactive no-target refusal is unchanged. Exit codes are identical to the live run. |
| `--non-interactive` | `false` | No prompts; use defaults. |

**Deprecated aliases** (still accepted; hidden from `--help`; print a stderr deprecation warning): `--delete-branch` → `--branch`, `--delete-remote true|false` → `--no-remote` (`--delete-remote false` ≡ `--no-remote`), `--delete-all` → `--all`, and the pre-existing `--worktree-name` → use positional arguments instead.

Positional args (zero or more): worktree names to delete. Resolution priority:
`--all` → positional names → `--worktree-name` (deprecated) → current
worktree → interactive selection menu.

Mixing positional args with `--worktree-name` exits with `ExitInvalidArgs`.

**Detached worktrees have no branch.** For a detached worktree (a `wt create
--temp` one, or any other) on every target-resolution path, `wt delete`
removes the worktree and does no branch handling: no branch cleanup (so
`--branch true` is a no-op), no `wt/<name>` orphan cleanup, no unpushed-commits
check, and under `--dry-run` no `Would delete branch` lines. The header's
`Branch:` line shows `(detached)`. Everything else (stash, dirty-discard
hazard, confirmations, exit codes) is unchanged.

Exit codes: `ExitInvalidArgs` for flag conflicts; `ExitGitError` for git
failures; `ExitGeneralError` for non-git failures during deletion.

## `wt init`

Run the worktree init script for the current worktree (or main repo). The
lookup contract is documented in [`init-protocol.md`](init-protocol.md).

No flags. No positional args.

Exit codes: `ExitGitError` when not in a repo; `ExitInitFailed` (7) when the
init script runs but exits non-zero (the script's own exit code is **not**
preserved — `runInitScript` maps every hard init failure to the typed
`ExitInitFailed` via an explicit `os.Exit(wt.ExitInitFailed)`, matching
`wt create`, rather than returning the error to `RunE` — which would map to
`ExitGeneralError`). Three init outcomes are non-failures and exit 0 with
guidance: (1) the init command is not on PATH, (2) the init file path does not
exist, and (3) — for the built-in default `fab sync` only — the repo is not
fab-managed and `fab sync` exits `ExitNotManaged = 3` (run-time skip;
provenance-gated, so an explicit `WORKTREE_INIT_SCRIPT` still exits 7). See
[`init-protocol.md`](init-protocol.md) for full semantics.

## `wt update`

Self-update the `wt` binary via Homebrew. Runs `brew update`, queries the tap
formula (`sahil87/tap/wt`) for its latest stable version, and invokes
`brew upgrade` when a newer version is available. Implementation lives under
`src/internal/update/`.

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-brew-update` | `false` | Skip the internal `brew update` tap-metadata refresh. The version check (`brew info`) and the `brew upgrade` still run. Toolkit contract flag: the update standard freezes the literal substring `--skip-brew-update` in `wt update --help` (shll probes it via a substring check), so this flag is visible and never deprecated. |
| `--no-brew-update` | `false` | Alias for `--skip-brew-update` — same bool, identical behavior, no warning. |

No positional args. Neither flag prints a deprecation warning.

Brew-handling safety (per the toolkit update standard): `brew upgrade` runs
with **no timeout** (interactive, stream-inheriting; Ctrl-C is the escape).
The bounded metadata calls are generous and terminate gracefully — `brew
update` 5 minutes, `brew info` 60 seconds, both canceling via SIGTERM with a
10-second grace (`cmd.Cancel`/`cmd.WaitDelay`); no code path sends SIGKILL to
brew.

User-facing outcomes:

- **Homebrew upgrade succeeds**: prints `Current version: <v>` → `Checking for
  updates...` → `Updating <current> → v<latest>...` → brew's tty-aware
  progress (inherits `os.Stdin`/`os.Stdout`/`os.Stderr`) → `Updated to
  v<latest>.`
- **Already up to date**: prints `Already up to date (<currentVersion>).` and
  exits without invoking `brew upgrade`.
- **Not installed via Homebrew** (e.g., `just local-install` builds in
  `~/.local/bin`): prints `wt <version> was not installed via Homebrew.` and
  `Update manually, or reinstall with: brew install sahil87/tap/wt`. No
  `brew` subprocess is invoked.
- **brew not on PATH**: prints `wt update: brew not found on PATH.` to stderr
  and exits — the cobra wrapper bypasses the default error formatter so the
  user sees exactly one line.

Exit codes: `ExitSuccess` (0) on successful upgrade, no-op when already up to
date, and the not-installed-via-Homebrew fast path; `ExitGeneralError` (1)
when `brew` is missing on PATH, `brew update` / `brew info` / `brew upgrade`
returns a non-zero status, the `brew info` JSON cannot be parsed, or no
stable version is reported by the tap formula.

## `wt shell-init <shell>`

Print a shell wrapper function for the named shell to stdout. The function
reads `WT_CD_FILE` after each `wt` invocation and runs `cd` in the parent
shell when the file is non-empty — this is what powers the "Open here" menu
option.

Usage: add `eval "$(wt shell-init zsh)"` to `~/.zshrc`, or
`eval "$(wt shell-init bash)"` to `~/.bashrc`.

One required positional arg: the target shell, `zsh` or `bash` (the emitted
wrapper is valid source for both). No flags.

Contract (toolkit shell-init standard): with a supported shell argument,
stdout carries **only** eval-safe shell source and the command exits 0;
diagnostics, if any, go to stderr. A **missing or unsupported** shell argument
is a usage error: usage message on stderr, **empty stdout**, exit
`ExitInvalidArgs` (2) — emitted via the direct-exit pattern since `main.go`
maps `RunE` errors to exit 1. `$SHELL` is never consulted (the former
inference path is removed).
