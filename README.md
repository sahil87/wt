# wt

> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.

[![Latest release](https://img.shields.io/github/v/release/sahil87/wt)](https://github.com/sahil87/wt/releases) [![Downloads](https://img.shields.io/github/downloads/sahil87/wt/total)](https://github.com/sahil87/wt/releases) [![Stars](https://img.shields.io/github/stars/sahil87/wt?style=social)](https://github.com/sahil87/wt/stargazers)

A small CLI that wraps `git worktree` with opinionated defaults: worktrees
are created as siblings of the main repo (`<repo>.worktrees/<name>/`), names
are memorable random adjective-noun pairs, and a shell wrapper makes
`cd`-into-worktree from a menu actually work. Designed for the parallel-edit
workflow where each branch (or each AI session) gets its own checkout.

## Install

```sh
curl -fsSL https://shll.ai/install | sh -s -- wt
```

Installs wt (plus the shll meta-CLI) via Homebrew, handling tap trust automatically. To install the entire sahil87 toolkit instead:

```sh
curl -fsSL https://shll.ai/install | sh
```

## Why wt?

- **Sibling layout, not clutter** — worktrees go in `<repo>.worktrees/<name>/`, never inside the main repo.
- **Memorable names** — random adjective-noun pairs (`lively-otter`, `bold-fox`) instead of `feature-1`, `feature-2`.
- **Real `cd` from a menu** — the shell wrapper lets `wt open` actually change your shell's directory (something a plain binary can't do).
- **Per-worktree init** — each new worktree runs an init script (default `fab sync`, override via `WORKTREE_INIT_SCRIPT`) so it's ready to use immediately.

## Other ways to install

Manual (requires Go and `just`):

```bash
git clone https://github.com/sahil87/wt
cd wt
just local-install   # builds bin/wt and copies to ~/.local/bin/wt
```

For the "Open here" menu option to actually `cd` your current shell, add the
wrapper to your shell profile:

```bash
eval "$(wt shell-init)"
```

> 💡 Have other sahil87 tools? [`shll shell-install`](https://github.com/sahil87/shll#shll-shell-install--wire-the-rc-file-recommended) handles all of their shell integrations and autocompletions at once.

For the full install guide — every path, the shell wrapper, and cross-tool setup — see the [install guide](docs/site/install.md).

## Usage

A typical first session:

```text
$ wt create
Created: ../wt.worktrees/lively-otter
  Branch: lively-otter (from main)

$ wt list
Worktrees for: wt
Location: /Users/you/code/wt.worktrees

  Name          Branch         Path
* (main)        main           wt/
  lively-otter  lively-otter   wt.worktrees/lively-otter/

$ wt open lively-otter        # menu → "Open here" cd's your shell
$ wt delete lively-otter      # removes worktree (and optionally the branch)
```

### Command reference

| Command | Summary |
|---------|---------|
| `wt create [branch]` (alias `wt new`) | Create a worktree on a **new** branch (random name, or the named positional). Key flags: `--checkout <branch>` (existing branch), `--base <ref>`, `--reuse`, `--name`/`-n`, `--non-interactive`. |
| `wt list` (alias `wt ls`) | List all worktrees with name, branch, and path. Add `--status` for dirty/unpushed indicators; `--path` and `--json` for scripting. |
| `wt open [name\|path]` | Open a worktree in a detected app (editor, terminal, file manager). `--app`/`-a` to skip the menu. |
| `wt delete [names...]` (alias `wt rm`) | Delete one or more worktrees with optional branch cleanup. |
| `wt init` | Run the worktree init script (default `fab sync`, override via `WORKTREE_INIT_SCRIPT`). |
| `wt shell-init` | Print a shell wrapper function for `eval` in your shell profile. |

Run `wt <command> --help` for inline flag details. For every flag and exit code, see the auto-generated [command reference](https://shll.ai/tools/wt/commands/); for the `--base` start-point rules and the `wt open` launcher matrix, see the [workflows guide](docs/site/workflows.md).

### `wt create --base` — branch start-point

`--base <ref>` controls the start-point when wt creates a new branch (maps to `git worktree add -b <branch> <path> <start-point>`). Behavior depends on whether the branch already exists:

| Scenario | `--base` | Behavior |
|----------|----------|----------|
| New branch (doesn't exist locally or remotely) | provided | Branch created from `--base` ref |
| New branch | omitted | Branch created from `HEAD` (default) |
| Positional names an existing branch (local or remote) | either | Error (exit 2): the positional only creates new branches — use `--checkout <branch>` |
| With `--checkout <branch>` | provided | Error (exit 2): `--base` sets a new branch's start-point; `--checkout` targets an existing branch |
| Exploratory (no branch arg) | provided | Exploratory branch created from `--base` ref |
| Exploratory | omitted | Branch created from current `HEAD` (default) |
| With `--reuse` (worktree exists) | provided | `--reuse` takes precedence; `--base` has no effect |
| Invalid ref | provided | Error exit; no worktree or branch created |

To put a worktree on an **existing** branch (local or remote), opt in explicitly
with `--checkout <branch>` — e.g. `wt create --checkout sockets-v2`. A remote-only
branch is fetched first; a branch that exists nowhere is an error pointing you
back at the positional/`--base` form.

The ref is validated via `git rev-parse --verify` before worktree creation, so invalid refs produce a clear error rather than a partial failure.

### `wt open` — context-aware launcher

`wt open` is the one command worth knowing in detail. It's the canonical
directory launcher in the toolkit (`hop` delegates to it too), and what it
does depends on where you run it from and what you pass it:

| Where you are | What you type | What happens |
|---------------|---------------|--------------|
| Inside a worktree | `wt open` | Opens the **current** worktree in your editor / terminal / file manager. |
| In the main repo | `wt open` | Shows a **worktree-selection menu** (most recently modified is highlighted). |
| In a non-git directory | `wt open` | Opens the **current directory** (equivalent to `wt open .`). |
| Anywhere | `wt open lively-otter` | Resolves the name against this repo's worktrees and opens it. (Requires a git repo.) |
| Anywhere | `wt open /tmp/notes` | Opens that directory literally — git context doesn't matter. |
| Anywhere | `wt open --app cursor` | Skips the menu and opens in the named app. |

The menu lists the apps wt detected on your machine (editors, terminals, file
managers) plus an **"Open here"** option that `cd`s your current shell into
the target — that one needs the shell wrapper (see [Gotchas](#gotchas)).

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

## Gotchas

- **`wt open` can't `cd` without the shell wrapper.** A child process can't change its parent shell's directory — that's a Unix constraint, not a wt bug. `eval "$(wt shell-init)"` installs a shell function that wraps the binary so the "Open here" menu option actually works.
- **The `wt create` positional never checks out an existing branch.** Naming a branch that already exists (locally or on the remote) is an error (exit 2) — checkout of an existing branch is an explicit opt-in via `--checkout <branch>`. `--reuse` takes precedence over `--base`.
- **Worktrees survive `cd` into deleted directories.** If you delete a worktree from outside (`rm -rf`), run `git worktree prune` in the main repo to clean up git's bookkeeping.
