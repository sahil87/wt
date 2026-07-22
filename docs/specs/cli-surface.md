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
| `--reuse` | `false` | If a worktree with `--name` already exists, reuse it instead of erroring. Requires `--name`. |
| `--non-interactive` | `false` | No prompts; fail or use defaults rather than prompting. |
| `--base <ref>` | (none) | Git start-point (branch / tag / SHA) for the NEW branch. Validated via `git rev-parse --verify` whenever set and `--reuse` is not. Cannot be combined with `--checkout`. |
| `--checkout <branch>` | (none) | Check out an EXISTING branch (local as-is, or remote-only fetched then checked out) into the worktree, instead of creating a new one. Cannot be combined with a positional branch argument or with `--base`. |

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

On success, the worktree path is written as the last line of stdout (suppressed
when the chosen app was `open_here` because the wrapper consumed it via
`WT_CD_FILE`).

Exit codes: `ExitInvalidArgs` for flag misuse (including `--checkout` combined
with a positional argument or with `--base`), invalid `--base`/branch name, a
positional naming an already-existing branch, or `--checkout` on a branch that
does not exist; `ExitGitError` for `git worktree add` failures;
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
| `--json` | `false` | Emit a JSON array of worktree records. Always emits `name`, `branch`, `path`, `is_main`, `is_current`. The `dirty` and `unpushed` keys are present only when `--status` is set. Mutually exclusive with `--path`. |
| `--status` | `false` | Compute and display dirty/unpushed status per worktree. Slower (forks 2 git subprocesses per worktree; parallelized). Mutually exclusive with `--path`. |

Default human output: a table with Name, Branch, and Path columns. The current
worktree is marked with a green asterisk. No per-worktree git invocations
occur — discovery is O(1) regardless of worktree count.

With `--status`: the table gains a Status column. Dirty worktrees show `*`,
unpushed commits show `↑N`. Enrichment uses a bounded worker pool of
`min(runtime.NumCPU(), 8)` workers; output ordering matches the porcelain
ordering regardless of parallelism.

Exit codes: `ExitInvalidArgs` if `--path` is combined with `--json` or
`--status`; `ExitGitError` if `git worktree list --porcelain` fails;
`ExitGeneralError` if `--path` cannot resolve the name.

## `wt open [name|path]`

Open a directory in a detected application (editor, terminal, file manager).
`wt open` is the canonical directory **launcher** — external callers (notably
`hop`) MAY delegate to it via subprocess invocation. The full env-var contract
is documented in [`launcher-contract.md`](launcher-contract.md). Worktree
**selection** (picking which worktree) is the job of [`wt go`](#wt-go-name);
`wt open --select` composes the two (select, then launch).

| Flag | Default | Description |
|------|---------|-------------|
| `--app`, `-a <name\|default>` | (none) | Open directly in the named app, skipping the menu. `default` selects the auto-detected default. |
| `--select` | `false` | Select a worktree first (via `wt go`'s menu when no name is given, or resolve-by-name when one is), then launch it. Requires a git repository; composes with `--app`. From a non-git cwd, exits `ExitGitError`. |
| `--list` | `false` | List the detected launchable host apps (kinds `editor` / `terminal` / `file-manager`) and exit — no menu, no launch, **no git repository required** (the branch runs before git-context detection). Action rows (`open_here`, `copy_*`, `byobu_tab`, `tmux_window`, `tmux_session`) are excluded from the listing but remain in the interactive menu and remain valid `--app` values. Human output is an aligned Id / Label / Kind table in `BuildAvailableApps()` detection order. Mutually exclusive with a positional target, `--app`, and `--select`. |
| `--json` | `false` | With `--list`, emit the app registry as a JSON array of `{id, label, kind}` records — `id` is the internal command key (`AppInfo.Cmd`, the exact token `wt open <path> -a <id>` accepts), `label` the display name, `kind` one of `editor` / `terminal` / `file-manager`. Zero detected apps emit `[]` (never `null`) and exit 0. Without `--list`, exits `ExitInvalidArgs`. |

**Deprecated alias** (still accepted; hidden from `--help`; prints a stderr deprecation warning): `--go` → `--select`.

Positional arg `[name|path]`:

- Omitted, called from inside a worktree: opens the current worktree.
- Omitted, called from the main repo: shows a worktree-selection menu. The
  **main worktree is pinned to row 1** (rendered `main (<branch>)`); non-main
  worktrees follow newest-first below it. The default highlight is the most
  recently modified *non-main* worktree (or the main row when it is the only
  entry).
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

Exit codes: `ExitInvalidArgs` when `--app` is used with the main-repo selection
menu, when `--list` is combined with a positional target / `--app` / `--select`
(or `--go`), or when `--json` is passed without `--list` (all `--list`/`--json`
validation happens at flag-check time, before any detection or git work);
`ExitGitError` when a git operation fails during name resolution, or when
`--select` is invoked from a non-git cwd (the `--select` git-repo precondition) — but
not for path-only or no-args invocations from outside a repo;
`ExitByobuTabError` / `ExitTmuxWindowError` for terminal-app failures;
`ExitGeneralError` for unknown apps, unresolved targets, or name args supplied
from a non-git cwd.

## `wt go [name]`

Select a worktree of the current repository and **navigate** there. `wt go` is
the worktree **selector** (the counterpart to `wt open`, the launcher): it
changes the shell's working directory to the chosen worktree and launches
nothing. Navigation reuses the same `WT_CD_FILE` shell-cd plumbing as the
launcher's "Open here" option — see [`launcher-contract.md`](launcher-contract.md) §3.

| Flag | Default | Description |
|------|---------|-------------|
| `--non-interactive` | `false` | No prompts. With no name, refuses deterministically (a no-arg selection menu has no non-interactive default) instead of prompting. |

Positional arg `[name]`:

- Omitted: shows a worktree-selection menu for the current repo. The **main
  worktree is pinned to row 1** (rendered `main (<branch>)`); non-main worktrees
  follow newest-first below it, branch shown per entry. The pre-selected default
  is the newest *non-main* worktree (or the main row when it is the only entry).
  Reachable from anywhere in the repository — the main repo **or** inside another
  worktree. On selection, navigates to the chosen worktree.
- Provided: resolved as a worktree name (case-insensitive); navigates there
  directly with no menu. The name `main` resolves to the main worktree (the repo
  root); an exact-basename match takes precedence, so a worktree directory
  literally named `main` still resolves to that worktree.

`wt go` always **requires a git repository** — worktree resolution walks the
repo's worktree list. It is scoped to the current repo's worktrees only;
cross-repo navigation is `hop`'s job.

Navigation mechanism: the resolved absolute path is written to `WT_CD_FILE`
(when set; mode `0600`, truncate-on-write) so the `wt shell-init` wrapper cd's
the parent shell there, **and** is printed to stdout as the last line so the
no-wrapper scripting form works: `cd "$(command wt go some-name)"`. When
`WT_CD_FILE` is unset and `WT_WRAPPER` is not `1`, the same "shell wrapper not
loaded" hint the launcher prints applies. `wt go` never cd's the parent shell
directly.

Exit codes: `ExitGitError` (3) when the cwd is not in a git repository or
`git worktree list` fails; `ExitGeneralError` (1) for an unknown worktree name,
or for a no-arg invocation under `--non-interactive`.

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
