# wt

> Part of [@sahil87's open source toolkit](https://ai.shll.in) — see all projects there.

[![Latest release](https://img.shields.io/github/v/release/sahil87/wt)](https://github.com/sahil87/wt/releases) [![Downloads](https://img.shields.io/github/downloads/sahil87/wt/total)](https://github.com/sahil87/wt/releases) [![Stars](https://img.shields.io/github/stars/sahil87/wt?style=social)](https://github.com/sahil87/wt/stargazers)

A small CLI that wraps `git worktree` with opinionated defaults: worktrees
are created as siblings of the main repo (`<repo>.worktrees/<name>/`), names
are memorable random adjective-noun pairs, and a shell wrapper makes
`cd`-into-worktree from a menu actually work. Designed for the parallel-edit
workflow where each branch (or each AI session) gets its own checkout.

## Install

Homebrew (preferred):

```bash
brew install sahil87/tap/wt
```

Manual (requires Go and `just`):

```bash
git clone https://github.com/sahil87/wt
cd wt
just local-install   # builds bin/wt and copies to ~/.local/bin/wt
```

For the "Open here" menu option to actually `cd` your current shell, add the
wrapper to your shell profile:

```bash
eval "$(wt shell-setup)"
```

## Usage

| Command | Summary |
|---------|---------|
| `wt create [branch]` | Create a worktree (random name + new branch, or named branch). |
| `wt list` | List all worktrees with branch, dirty/unpushed status, and path. |
| `wt open [name\|path]` | Open a worktree in a detected app (editor, terminal, file manager). |
| `wt delete [names...]` | Delete one or more worktrees with optional branch cleanup. |
| `wt init` | Run the worktree init script (default `fab sync`, override via `WORKTREE_INIT_SCRIPT`). |
| `wt shell-setup` | Print a shell wrapper function for `eval` in your shell profile. |

Run `wt <command> --help` for the full reference.

## Specs

Pre-implementation specs live under [`docs/specs/`](docs/specs/index.md):
CLI surface, worktree layout, init protocol, and build/release flow.

---

> Part of [@sahil87's open source toolkit](https://ai.shll.in) — see all projects there. Originally extracted from [fab-kit](https://github.com/sahil87/fab-kit); the fab-kit repo continues to bundle a copy during the transition.
