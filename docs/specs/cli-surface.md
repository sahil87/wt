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
`wt open --go` composes the two (select, then launch).

| Flag | Default | Description |
|------|---------|-------------|
| `--app <name\|default>` | (none) | Open directly in the named app, skipping the menu. `default` selects the auto-detected default. |
| `--go` | `false` | Select a worktree first (via `wt go`'s menu when no name is given, or resolve-by-name when one is), then launch it. Requires a git repository; composes with `--app`. From a non-git cwd, exits `ExitGitError`. |

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

- Omitted: shows a worktree-selection menu for the current repo (newest-first,
  branch shown per entry, newest pre-selected as default). Reachable from
  anywhere in the repository — the main repo **or** inside another worktree.
  On selection, navigates to the chosen worktree.
- Provided: resolved as a worktree name (case-insensitive); navigates there
  directly with no menu.

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

## `wt shell-init`

Print a shell wrapper function (bash/zsh) to stdout. The function reads
`WT_CD_FILE` after each `wt` invocation and runs `cd` in the parent shell when
the file is non-empty — this is what powers the "Open here" menu option.

Usage: add `eval "$(wt shell-init)"` to `~/.bashrc` or `~/.zshrc`.

No flags. No positional args. Always exit 0. A warning is printed to stderr if
`$SHELL` is set to something other than bash/zsh, but the wrapper is still
emitted (it is bash-compatible).
