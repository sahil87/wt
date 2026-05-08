# CLI Surface

Per-subcommand reference for the `wt` binary. Source of truth: cobra command
definitions under `src/cmd/wt/`. Exit code constants are defined in
`src/internal/worktree/errors.go`.

Run `wt <command> --help` for the full inline reference.

## Exit codes

| Constant | Value | Meaning |
|----------|-------|---------|
| `ExitSuccess` | 0 | Command completed successfully |
| `ExitGeneralError` | 1 | Non-specific failure (cannot resolve repo context, init failed, no default app, etc.) |
| `ExitInvalidArgs` | 2 | Caller supplied incompatible flags or invalid input (bad branch name, bad `--base` ref, mutually exclusive flags) |
| `ExitGitError` | 3 | A `git` invocation failed or the working dir is not a git repository |
| `ExitRetryExhausted` | 4 | Random-name generator could not find a non-colliding name after retries |
| `ExitByobuTabError` | 5 | Failed to open the worktree in a byobu tab |
| `ExitTmuxWindowError` | 6 | Failed to open the worktree in a tmux window |

Subcommands map domain failures to these codes via `wt.ExitWithError`. SIGINT
during `wt create` exits 130 after rolling back partial state (standard Unix
signal-exit convention).

## `wt create [branch]`

Create a git worktree as a sibling of the main repo (`<repo>.worktrees/<name>/`).

| Flag | Default | Description |
|------|---------|-------------|
| `--worktree-name <name>` | random adjective-noun | Set the worktree directory name; skips the name prompt. |
| `--worktree-init <true\|false>` | `true` | Run the worktree init script after creation. |
| `--worktree-open <prompt\|default\|skip\|<app>>` | `prompt` (`skip` when `--non-interactive`) | Behavior after creation: show app menu, open in detected default, skip, or open in a specific app (e.g. `code`, `cursor`). |
| `--reuse` | `false` | If a worktree with `--worktree-name` already exists, reuse it instead of erroring. Requires `--worktree-name`. |
| `--non-interactive` | `false` | No prompts; fail or use defaults rather than prompting. |
| `--base <ref>` | (none) | Git start-point (branch / tag / SHA) for new branches. Validated via `git rev-parse --verify`. Ignored for existing branches and when `--reuse` is set. |

Positional arg `branch` (optional):

- Omitted: exploratory worktree on a new branch named after the random worktree name.
- Provided, branch exists locally or remotely: checks out that branch into the new worktree.
- Provided, branch does not exist: creates a new branch, optionally from `--base`.

On success, the worktree path is written as the last line of stdout (suppressed
when the chosen app was `open_here` because the wrapper consumed it via
`WT_CD_FILE`).

Exit codes: `ExitInvalidArgs` for flag misuse or invalid `--base`/branch name;
`ExitGitError` for `git worktree add` failures; `ExitRetryExhausted` for name
generation; `ExitGeneralError` for init script failure.

## `wt list`

List all worktrees for the current repository.

| Flag | Default | Description |
|------|---------|-------------|
| `--path <name>` | (none) | Print only the absolute path for the named worktree. Mutually exclusive with `--json`. |
| `--json` | `false` | Emit a JSON array of worktree records (`name`, `branch`, `path`, `is_main`, `is_current`, `dirty`, `unpushed`). Mutually exclusive with `--path`. |

Default human output: a table with name, branch, dirty/unpushed status, and a
short relative path. The current worktree is marked with a green asterisk.

Exit codes: `ExitInvalidArgs` if `--path` and `--json` are combined;
`ExitGitError` if `git worktree list --porcelain` fails; `ExitGeneralError` if
`--path` cannot resolve the name.

## `wt open [name|path]`

Open a directory in a detected application (editor, terminal, file manager).
`wt open` is the canonical directory launcher — external callers (notably
`hop`) MAY delegate to it via subprocess invocation. The full env-var contract
is documented in [`launcher-contract.md`](launcher-contract.md).

| Flag | Default | Description |
|------|---------|-------------|
| `--app <name\|default>` | (none) | Open directly in the named app, skipping the menu. `default` selects the auto-detected default. |

Positional arg `[name|path]`:

- Omitted, called from inside a worktree: opens the current worktree.
- Omitted, called from the main repo: shows a worktree-selection menu (default
  highlight: most recently modified worktree).
- Omitted, called from a non-git directory: opens the current working
  directory (equivalent to `wt open .`).
- Existing directory path: treated as a literal path. Works regardless of git
  context — `wt open /tmp/notes` succeeds from any cwd as long as the path is
  a real directory.
- Otherwise: resolved as a worktree name (case-insensitive). **Requires a git
  repository** — name resolution walks the worktree list, which is only
  reachable when the cwd is inside a git repo.

Exit codes: `ExitInvalidArgs` when `--app` is used with the main-repo selection
menu; `ExitGitError` only when a git operation fails during name resolution
(not for path-only or no-args invocations from outside a repo);
`ExitByobuTabError` / `ExitTmuxWindowError` for terminal-app failures;
`ExitGeneralError` for unknown apps, unresolved targets, or name args supplied
from a non-git cwd.

## `wt delete [worktree-names...]`

Delete one or more worktrees with optional branch cleanup.

| Flag | Default | Description |
|------|---------|-------------|
| `--worktree-name <name>` | (none) | **Deprecated**: use positional arguments instead. |
| `--delete-branch <true\|false\|auto>` | `auto` | Delete the associated local branch. `auto` deletes only when the branch name matches the worktree name. |
| `--delete-remote <true\|false>` | `true` | Delete the remote-tracking branch when the local branch is deleted. |
| `--delete-all` | `false` | Delete every worktree (skips the current selection logic). |
| `-s`, `--stash` | `false` | Stash uncommitted changes in the worktree before deleting. |
| `--non-interactive` | `false` | No prompts; use defaults. |

Positional args (zero or more): worktree names to delete. Resolution priority:
`--delete-all` → positional names → `--worktree-name` (deprecated) → current
worktree → interactive selection menu.

Mixing positional args with `--worktree-name` exits with `ExitInvalidArgs`.

Exit codes: `ExitInvalidArgs` for flag conflicts; `ExitGitError` for git
failures; `ExitGeneralError` for non-git failures during deletion.

## `wt init`

Run the worktree init script for the current worktree (or main repo). The
lookup contract is documented in [`init-protocol.md`](init-protocol.md).

No flags. No positional args.

Exit codes: `ExitGitError` when not in a repo; `ExitGeneralError` (1) when the
init script runs but exits non-zero (the script's exit code is **not**
preserved — `RunE` returns an error, which `main.go` maps to
`ExitGeneralError`). Missing init command/file results in a graceful skip with
guidance — exit 0. See [`init-protocol.md`](init-protocol.md) for full
semantics.

## `wt update`

Self-update the `wt` binary via Homebrew. Runs `brew update`, queries the tap
formula (`sahil87/tap/wt`) for its latest stable version, and invokes
`brew upgrade` when a newer version is available. Implementation lives under
`src/internal/update/`.

No flags. No positional args.

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

## `wt shell-setup`

Print a shell wrapper function (bash/zsh) to stdout. The function reads
`WT_CD_FILE` after each `wt` invocation and runs `cd` in the parent shell when
the file is non-empty — this is what powers the "Open here" menu option.

Usage: add `eval "$(wt shell-setup)"` to `~/.bashrc` or `~/.zshrc`.

No flags. No positional args. Always exit 0. A warning is printed to stderr if
`$SHELL` is set to something other than bash/zsh, but the wrapper is still
emitted (it is bash-compatible).
