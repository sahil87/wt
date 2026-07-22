# Workflows & command reference

The depth behind `wt` — every command and flag, the `wt create --base`
start-point rules, the `wt open`/`wt go` launcher-and-selector matrix, and the
gotchas worth knowing. New here? Start with the [install guide](./install.md).

Run `wt <command> --help` for the same reference inline at your terminal.

## Command reference

| Command | Summary |
|---------|---------|
| `wt create [branch]` | Create a worktree as a sibling of the main repo. |
| `wt list` | List all worktrees with name, branch, and path. |
| `wt open [name\|path]` | Open a worktree (or any directory) in a detected app. No arg opens the current context. |
| `wt go [name]` | Pick a worktree (menu or by name) and `cd` there; `--open` launches it instead. |
| `wt delete [names...]` | Delete one or more worktrees with optional branch cleanup. |
| `wt init` | Run the per-worktree init script. |
| `wt shell-init <shell>` | Print the shell wrapper function (`zsh` or `bash`) for `eval`. |
| `wt update` | Self-update the binary via Homebrew. |

### `wt create [branch]`

Creates a git worktree as a sibling of the main repo
(`<repo>.worktrees/<name>/`). With no `branch` argument it makes an exploratory
worktree on a new branch named after the random worktree name; with a `branch`
argument it creates a **new** branch with that name (an existing branch name is
an error — put a worktree on an existing branch explicitly with
`--checkout <branch>`).

| Flag | Default | Description |
|------|---------|-------------|
| `--worktree-name <name>` | random adjective-noun | Set the worktree directory name; skips the name prompt. |
| `--worktree-init <true\|false>` | `true` | Run the worktree init script after creation. |
| `--worktree-open <prompt\|default\|skip\|<app>>` | `prompt` (`skip` under `--non-interactive`) | What to do after creation: show the app menu, open in the detected default, skip, or open in a named app (e.g. `code`, `cursor`). |
| `--reuse` | `false` | If a worktree with `--worktree-name` already exists, reuse it instead of erroring. Requires `--worktree-name`. |
| `--non-interactive` | `false` | No prompts; fail or use defaults rather than prompting. |
| `--base <ref>` | (none) | Git start-point (branch / tag / SHA) for new branches. See the table below. |
| `--checkout <branch>` | (none) | Check out an **existing** branch (local or remote) into the new worktree. Mutually exclusive with the positional and `--base`. |

On success the worktree path is always written as the last line of stdout.

### `wt list`

Lists every worktree for the current repository. Discovery is O(1) — no
per-worktree git invocations occur unless you ask for `--status`.

| Flag | Default | Description |
|------|---------|-------------|
| `--status` | `false` | Add a Status column: `*` for dirty, `↑N` for unpushed commits. Slower (forks git per worktree, parallelized). Mutually exclusive with `--path`. |
| `--path <name>` | (none) | Print only the absolute path for the named worktree. Mutually exclusive with `--json` and `--status`. |
| `--json` | `false` | Emit a JSON array of worktree records (`name`, `branch`, `path`, `is_main`, `is_current`; plus `dirty`/`unpushed` when `--status` is set). Mutually exclusive with `--path`. |
| `--sort <recent\|name\|branch>` | (none) | Order non-main worktrees by most-recently-modified, name, or branch. |
| `--non-interactive` | `false` | Use the stable (name) default ordering, suitable for scripts. |

### `wt open [name|path]`

The canonical directory launcher in the toolkit (other tools, like `hop`,
delegate to it). With no argument it opens the current context (worktree root /
repo root / cwd); a name resolves against this repo's worktrees; a path opens
literally — see the [launcher matrix](#wt-open-and-wt-go--launcher-and-selector).
Worktree *selection* is `wt go`'s job.

| Flag | Default | Description |
|------|---------|-------------|
| `--app <name\|default>`, `-a` | (none) | Open directly in the named app, skipping the app menu. `default` selects the auto-detected default. Works with every target form, including no-arg. |
| `--list [--json]` | `false` | List the detected launchable host apps and exit — no menu, no launch, no git repo required. `--json` emits `{id, label, kind}` records; every `id` is accepted by `wt open <path> -a <id>`. |

### `wt go [name]`

The worktree **selector**: pick a worktree of the current repo (menu when no
name; resolve-by-name otherwise — the name `main` resolves to the repo root)
and `cd` there via the shell wrapper. With `--open`, the selection is launched
instead of navigated to — the composition mirroring `wt create --open`.

| Flag | Default | Description |
|------|---------|-------------|
| `--open <prompt\|default\|skip\|<app>>` | (unset — navigate) | Launch the selection: `prompt` shows the app menu, `default` uses the auto-detected app, an app name launches directly, `skip` navigates. An explicit value is always required. |
| `--non-interactive` | `false` | No prompts. With no name, refuses deterministically instead of showing the menu. |

### `wt delete [worktree-names...]`

Deletes one or more worktrees with optional branch cleanup. Resolution
priority: `--delete-all` → positional names → current worktree → interactive
selection menu.

| Flag | Default | Description |
|------|---------|-------------|
| `--delete-branch <true\|false\|auto>` | `auto` | Delete the associated local branch. `auto` deletes only when the branch name matches the worktree name. |
| `--delete-remote <true\|false>` | `true` | Delete the branch on the origin remote (via `git push origin --delete`) when the local branch is deleted. |
| `--delete-all` | `false` | Delete every worktree (skips the selection logic). |
| `-s`, `--stash` | `false` | Stash uncommitted changes in the worktree before deleting. |
| `--stale[=Nd]` | `7d` when bare | Select idle worktrees (filesystem mtime older than the threshold) for deletion. Bare `--stale` uses the 7-day default; `--stale=30d` overrides. The `=` is required. |
| `--non-interactive` | `false` | No prompts; use defaults. |

### `wt init`

Runs the per-worktree init script for the current worktree. The script is
resolved from `WORKTREE_INIT_SCRIPT` (if set and non-empty), otherwise it
defaults to `fab sync`. A value containing a space is treated as a command
invocation; a value without spaces is treated as a script path relative to the
main repo root. The script runs with its working directory set to the current
worktree's top level. No flags, no positional args.

If the init command or file can't be located, `wt init` prints a guidance
warning and exits 0 (a graceful skip) — so a freshly cloned repo without an
init script just no-ops. If the script **is** found but exits non-zero, `wt`
surfaces a typed init-failure exit code so wrappers can offer a retry.

### `wt shell-init <shell>`

Prints a wrapper function for the named shell (`zsh` or `bash`) to stdout for
`eval` in your shell profile. See the
[install guide](./install.md#shell-wrapper-enables-open-here) for setup.

The shell argument is required. Everything on stdout is eval-safe shell source
and the command exits 0; a missing or unsupported shell argument is a usage
error — exit 2, usage message on stderr, nothing on stdout.

### `wt update`

Self-updates the `wt` binary via Homebrew. Runs a `brew update`, queries the tap
formula (`sahil87/tap/wt`) for the latest stable version, and runs
`brew upgrade` only when a newer version is available.

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-brew-update` | `false` | Skip the internal `brew update` tap-metadata refresh (toolkit contract flag; the version check and upgrade still run). |
| `--no-brew-update` | `false` | Alias for `--skip-brew-update` — identical behavior. |

If `wt` was installed via `just local-install` (in `~/.local/bin`) rather than
Homebrew, `wt update` reports that and tells you to reinstall with
`brew install sahil87/tap/wt` instead of attempting a self-update.

## `wt create --base` — branch start-point

`--base <ref>` controls the start-point when `wt` creates a new branch (it maps
to `git worktree add -b <branch> <path> <start-point>`). Behavior depends on
whether the branch already exists:

| Scenario | `--base` | Behavior |
|----------|----------|----------|
| New branch (doesn't exist locally or remotely) | provided | Branch created from the `--base` ref. |
| New branch | omitted | Branch created from `HEAD` (default). |
| Positional names an existing branch (local or remote) | either | Error (exit 2): the positional only creates new branches — use `--checkout <branch>`. |
| With `--checkout <branch>` | provided | Error (exit 2): `--base` sets a new branch's start-point; `--checkout` targets an existing branch. |
| Exploratory (no branch arg) | provided | Exploratory branch created from the `--base` ref. |
| Exploratory | omitted | Branch created from the current `HEAD` (default). |
| With `--reuse` (worktree exists) | provided | `--reuse` takes precedence; `--base` has no effect. |
| Invalid ref | provided | Error exit; no worktree or branch created. |

The ref is validated with `git rev-parse --verify` before worktree creation, so
an invalid ref produces a clear error rather than a partial failure.

## `wt open` and `wt go` — launcher and selector

The two commands split along two axes — **selection** (which directory) and
**action** (navigate vs. launch). Each menu lives in exactly one verb: `wt go`
owns the "which worktree?" menu, `wt open` owns the "which app?" menu. Compose
them with `wt go --open`:

| Invocation | Worktree menu? | App menu? | Result |
|---|---|---|---|
| `wt go` | yes | no | `cd` to selection |
| `wt go frosty-fox` | no | no | `cd` directly |
| `wt go --open prompt` | yes | yes | launch selection in chosen app |
| `wt go --open code` | yes | no | launch selection in VS Code |
| `wt open` | no | yes | launch *current* dir (worktree root / repo root / cwd) |
| `wt open <name\|path>` | no | yes | launch that dir |
| `wt open --app code` | no | no | launch current dir in VS Code |

Path-arg precedence: when you supply an argument to `wt open`, `wt` tries it as
a literal directory path first; only if that's not an existing directory (and
the cwd is inside a git repo) does it fall back to resolving the argument as a
worktree name.

The app menu lists the apps `wt` detected on your machine (editors, terminals,
file managers) plus an **"Open here"** option that `cd`s your current shell
into the target — that one needs the shell wrapper (see [Gotchas](#gotchas)).

Picking a worktree and launching it, with two worktrees on disk:

```text
$ wt go --open prompt
Select worktree to open:
  1) main (main)
  2) lively-otter (feature/spinner) (default)
  3) bold-fox (fix/race-condition)
  0) Cancel

Choice [2]: 2
Open in:
  1) Open here
  2) VSCode (default)
  3) Cursor
  4) Ghostty
  5) Terminal.app
  6) Finder
  7) Copy path
  0) Cancel

Choice [2]:
```

Pick `1` to `cd` your shell into the worktree, `2`–`6` to launch it in a
detected app, or `7` to copy the absolute path to your clipboard.

## Worktree layout

Worktrees live as **siblings** of the main repo, grouped under one per-repo
directory:

```text
<parent>/
├── <repo>/                  # main repo
└── <repo>.worktrees/        # all linked worktrees for this repo
    ├── swift-fox/
    ├── jolly-otter/
    └── crimson-heron/
```

Names are random `adjective-noun` pairs (`swift-fox`, `jolly-otter`), with the
generator retrying up to 10 times to avoid collisions; pass `--worktree-name` to
choose your own. The branch checked out in a worktree is independent of the
worktree directory name — for an exploratory worktree the two happen to match,
but for a worktree created on an existing branch they differ freely.

## Gotchas

- **`wt open` can't `cd` without the shell wrapper.** A child process can't
  change its parent shell's directory — that's a Unix constraint, not a `wt`
  bug. `eval "$(wt shell-init zsh)"` (or `bash`) installs a shell function that
  wraps the binary so the "Open here" menu option actually works. See the
  [install guide](./install.md#shell-wrapper-enables-open-here).
- **The `wt create` positional never checks out an existing branch.** Naming a
  branch that already exists (locally or on the remote) is an error (exit 2) —
  checkout of an existing branch is an explicit opt-in via `--checkout <branch>`.
  `--reuse` takes precedence over `--base`.
- **Worktrees survive `cd` into deleted directories.** If you delete a worktree
  from outside (`rm -rf`), run `git worktree prune` in the main repo to clean up
  git's bookkeeping.
- **Name resolution needs a git repo, path args don't.** `wt open <name>` walks
  the worktree list and so requires a git repo; `wt open <path>` works from any
  directory because it's a literal path.
- **`wt init` no-ops gracefully when no script is found.** A fresh clone without
  an init script (and without fab-kit installed) silently does nothing on
  `wt init` — that's intentional for the "I just want to use the worktree" path.

## See also

- [Install guide](./install.md) — Homebrew, manual install, and the shell wrapper.
- [Releases](https://github.com/sahil87/wt/releases) — download a packaged binary directly.
