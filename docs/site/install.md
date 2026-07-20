# Installing wt

`wt` ships as a single self-contained Go binary — the only runtime dependency
is `git`. Pick the install path that fits your setup, then wire up the shell
wrapper so the "Open here" menu option can actually `cd` your shell.

## Via shll.ai (preferred)

```bash
curl -fsSL https://shll.ai/install | sh -s -- wt
```

Installs wt (plus the shll meta-CLI) via Homebrew, handling tap trust
automatically. To install the entire [shll toolkit](https://shll.ai) instead,
drop the `-s -- wt` suffix; if you already have the `shll` meta-CLI,
`shll install wt` does the same thing. For the full install story, see
[https://shll.ai](https://shll.ai). To upgrade later, `wt update` self-updates
via Homebrew (see the [workflows reference](./workflows.md#wt-update)).

## Manual (requires Go and `just`)

```bash
git clone https://github.com/sahil87/wt
cd wt
just local-install   # builds bin/wt and copies it to ~/.local/bin/wt
```

`just local-install` compiles `bin/wt` (stamping the version from
`git describe --tags --always`) and copies it to `~/.local/bin/wt`. For this to
put `wt` on your `PATH`, `~/.local/bin` must already be in `PATH`. Binaries
installed this way are **not** managed by Homebrew, so `wt update` will tell you
to reinstall via `brew` rather than attempting a self-update.

## Shell wrapper (enables "Open here")

A child process can't change its parent shell's directory — that's a Unix
constraint, not a `wt` limitation. To make `wt open`'s "Open here" menu option
`cd` your current shell, add the wrapper to your shell profile:

```bash
eval "$(wt shell-init zsh)"     # in ~/.zshrc
eval "$(wt shell-init bash)"    # in ~/.bashrc
```

`wt shell-init <shell>` prints a wrapper function for the named shell (`zsh` or
`bash`) that reads `WT_CD_FILE` after each `wt` invocation and runs `cd` in the
parent shell when that file is non-empty. Without it, "Open here" falls back to
printing a `cd -- '<path>'` line you can copy. See the
[gotchas](./workflows.md#gotchas) for the full story.

## Already use other shll tools?

If you have other tools from the toolkit installed,
[`shll shell-install`](https://github.com/sahil87/shll#shll-shell-install--wire-the-rc-file-recommended)
wires up all of their shell integrations and autocompletions at once — including
the `wt` wrapper above — so you don't have to add each `eval` line by hand.

## Where wt came from

`wt` was originally extracted from
[fab-kit](https://github.com/sahil87/fab-kit), which bundled the worktree helper
alongside its other binaries. It now lives in its own repo
([`sahil87/wt`](https://github.com/sahil87/wt)) and is released independently;
the fab-kit repo continues to bundle a copy during the transition.

## Next steps

- [Workflows & command reference](./workflows.md) — every flag, the `--base`
  start-point rules, the `wt open` launcher matrix, and in-depth gotchas.
