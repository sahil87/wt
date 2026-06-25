# Workflows & command reference

The depth behind `wt` — every command and flag, the `wt create --base`
start-point rules, the `wt open` launcher matrix, and the gotchas worth knowing.
New here? Start with the [install guide](./install.md).

Run `wt <command> --help` for the same reference inline at your terminal.

## Command reference

| Command | Summary |
|---------|---------|
| `wt create [branch]` | Create a worktree as a sibling of the main repo. |
| `wt list` | List all worktrees with name, branch, and path. |
| `wt open [name\|path]` | Open a worktree (or any directory) in a detected app. |
| `wt delete [names...]` | Delete one or more worktrees with optional branch cleanup. |
| `wt init` | Run the per-worktree init script. |
| `wt shell-init` | Print the shell wrapper function for `eval`. |
| `wt update` | Self-update the binary via Homebrew. |

### `wt create [branch]`

Creates a git worktree as a sibling of the main repo
(`<repo>.worktrees/<name>/`). With no `branch` argument it makes an exploratory
worktree on a new branch named after the random worktree name; with a `branch`
argument it checks out that branch (existing) or creates it (new).

| Flag | Default | Description |
|------|---------|-------------|
| `--worktree-name <name>` | random adjective-noun | Set the worktree directory name; skips the name prompt. |
| `--worktree-init <true\|false>` | `true` | Run the worktree init script after creation. |
| `--worktree-open <prompt\|default\|skip\|<app>>` | `prompt` (`skip` under `--non-interactive`) | What to do after creation: show the app menu, open in the detected default, skip, or open in a named app (e.g. `code`, `cursor`). |
| `--reuse` | `false` | If a worktree with `--worktree-name` already exists, reuse it instead of erroring. Requires `--worktree-name`. |
| `--non-interactive` | `false` | No prompts; fail or use defaults rather than prompting. |
| `--base <ref>` | (none) | Git start-point (branch / tag / SHA) for new branches. See the table below. |

On success the worktree path is written as the last line of stdout (suppressed
when the chosen app was "Open here", because the shell wrapper consumed it via
`WT_CD_FILE`).

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
delegate to it). What it does depends on where you run it and what you pass —
see the [launcher matrix](#wt-open--context-aware-launcher).

| Flag | Default | Description |
|------|---------|-------------|
| `--app <name\|default>` | (none) | Open directly in the named app, skipping the menu. `default` selects the auto-detected default. Incompatible with the main-repo selection menu. |

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

### `wt shell-init`

Prints a bash/zsh wrapper function to stdout for `eval` in your shell profile.
See the [install guide](./install.md#shell-wrapper-enables-open-here) for setup.
No flags, no positional args; always exits 0.

### `wt update`

Self-updates the `wt` binary via Homebrew. Runs a `brew update`, queries the tap
formula (`sahil87/tap/wt`) for the latest stable version, and runs
`brew upgrade` only when a newer version is available.

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-brew-update` | `false` | Skip the internal `brew update` tap-metadata refresh (the version check and upgrade still run). |

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
| Existing local branch | provided | Warning: `--base ignored: branch already exists locally`. |
| Existing remote branch | provided | Warning: `--base ignored: fetching existing remote branch`. |
| Exploratory (no branch arg) | provided | Exploratory branch created from the `--base` ref. |
| Exploratory | omitted | Branch created from the current `HEAD` (default). |
| With `--reuse` (worktree exists) | provided | `--reuse` takes precedence; `--base` has no effect. |
| Invalid ref | provided | Error exit; no worktree or branch created. |

The ref is validated with `git rev-parse --verify` before worktree creation, so
an invalid ref produces a clear error rather than a partial failure.

## `wt open` — context-aware launcher

`wt open` is the one command worth knowing in detail. What it does depends on
where you run it from and what you pass it:

| Where you are | What you type | What happens |
|---------------|---------------|--------------|
| Inside a worktree | `wt open` | Opens the **current** worktree in your editor / terminal / file manager. |
| In the main repo | `wt open` | Shows a **worktree-selection menu** (most recently modified is highlighted). |
| In a non-git directory | `wt open` | Opens the **current directory** (equivalent to `wt open .`). |
| Anywhere | `wt open lively-otter` | Resolves the name against this repo's worktrees and opens it. (Requires a git repo.) |
| Anywhere | `wt open /tmp/notes` | Opens that directory literally — git context doesn't matter. |
| Anywhere | `wt open --app cursor` | Skips the menu and opens in the named app. |

Path-arg precedence: when you supply an argument, `wt` tries it as a literal
directory path first; only if that's not an existing directory (and the cwd is
inside a git repo) does it fall back to resolving the argument as a worktree
name.

The menu lists the apps `wt` detected on your machine (editors, terminals, file
managers) plus an **"Open here"** option that `cd`s your current shell into the
target — that one needs the shell wrapper (see [Gotchas](#gotchas)).

Running `wt open` from the main repo, with two worktrees on disk:

```text
$ wt open
Select worktree to open:
  1) lively-otter (feature/spinner) (default)
  2) bold-fox    (fix/race-condition)
  0) Cancel

Choice [1]: 1
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
  bug. `eval "$(wt shell-init)"` installs a shell function that wraps the binary
  so the "Open here" menu option actually works. See the
  [install guide](./install.md#shell-wrapper-enables-open-here).
- **`--base` is ignored when the branch already exists** (locally or on the
  remote) — `wt` checks out the existing branch instead and prints a warning.
  `--reuse` also takes precedence over `--base`.
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
