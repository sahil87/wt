# wt skill bundle

`wt` wraps `git worktree` with opinionated defaults: worktrees are created as
siblings of the main repo (`<repo>.worktrees/<name>/`), branches get memorable
random adjective-noun names, and a shell wrapper makes `cd`-into-worktree from a
menu actually work. Built for parallel work where each branch — or each agent
session — gets its own checkout.

## When to use

- Reach for `wt` when you need **multiple branches checked out at once** — parallel
  edits, reviews, or agent sessions — without stash/switch churn on a single
  working tree.
- **Not** for plain branch switching in one working tree: if you don't need a
  second checkout on disk, `git switch` is simpler. `wt` adds a directory per
  branch; that is the whole point, and the cost.

## Capabilities map

One line per subcommand (run `wt <cmd> --help` for flags):

- `create [branch]` (alias `new`) — new-branch worktree with a random name.
  `--checkout <branch>` puts it on an **existing** branch instead; `--base <ref>`
  sets a new branch's start-point; `-n/--name` overrides the random name;
  `--reuse` reuses an existing named worktree; `--no-init` skips the init script.
- `list` (alias `ls`) — table of worktrees (name, branch, path). `--status` adds
  dirty/unpushed indicators; `--json` emits machine records; `--path <name>`
  prints one absolute path.
- `open [name|path]` — launch a worktree/dir in a detected app (editor, terminal,
  file manager) via a menu. `-a/--app <name>` skips the menu; `default` picks the
  auto-detected app. `--select` picks a worktree first, then launches.
- `go [name]` — **select** a worktree and navigate (cd) there; launches nothing.
  No arg → selection menu; a name → cd directly. Requires a git repo.
- `delete [names...]` (alias `rm`) — remove worktrees with optional branch cleanup.
  `--all/-a` removes every worktree; `-s/--stash` stashes uncommitted changes first;
  `--branch <true|false|auto>` and `--no-remote` control branch deletion.
- `init` — run the worktree init script for the current worktree/main repo.
- `shell-init` — print the shell wrapper function for `eval` in your profile.
- `update` — self-update the binary via Homebrew.

## Composition patterns

- **Shell-wrapper eval flow.** A child process cannot `cd` its parent shell, so
  `wt` prints shell code that the user evals: `eval "$(wt shell-init)"` installs a
  function wrapping the binary. That function powers the "Open here" menu option in
  `wt open` and the navigation in `wt go`. Without it, those fall back to printing a
  path for the caller to `cd`.
- **Launcher contract (`WT_CD_FILE` / `WT_WRAPPER`).** `wt open` is the toolkit's
  canonical directory launcher — external callers (e.g. `hop`) delegate to it as a
  subprocess. When `WT_CD_FILE` is set, "Open here" and `wt go` write the resolved
  directory path there (mode 0600, truncate-on-write) instead of printing a
  `cd` line; the caller applies the `cd` itself. Set `WT_WRAPPER=1` to signal you
  handle the `cd` and suppress the "wrapper not loaded" hint. A non-zero exit means
  do **not** trust `WT_CD_FILE`'s contents. See `docs/site` / `launcher-contract`.
- **Init protocol (`WORKTREE_INIT_SCRIPT`).** Each new worktree runs an init script
  — default `fab sync`, override via `WORKTREE_INIT_SCRIPT`. A value with a space is
  a command invocation (first word looked up on PATH); a value without spaces is a
  file path resolved from the repo root and run via `bash`. It runs with the new
  worktree as its working directory; its output goes to stderr. See `init-protocol`.
- **Machine surface.** `wt list --json` is the structured composition surface (e.g.
  what `hop` reads); `wt create` prints the worktree path as its last stdout line.

## Output & exit-code contracts

- **stdout is data, stderr is human copy.** Machine results (the created worktree
  path, `list --json`, `list --path`, the `go` target path) go to stdout; all
  progress, prompts, warnings, and errors go to stderr. Init-script output is
  diagnostic and streams to stderr. So `p=$(wt create ...)` captures the path clean.
- **Errors are structured** `Error: <what>` / `Why: <why>` / `Fix: <fix>` on stderr,
  emitted via `ExitWithError`.
- **Typed exit codes** (from `internal/worktree/errors.go`) let scripts branch:
  - `0` success · `1` general error · `2` invalid args / incompatible flags
  - `3` git error (or not a git repo) · `4` name-generation retries exhausted
  - `5` byobu-tab error · `6` tmux-window error
  - `7` init script ran but exited non-zero (**worktree is kept**, not rolled back)
  - `130` SIGINT during `wt create` (partial state rolled back)
- **Scriptable on demand.** Interactive commands take `--non-interactive` for
  deterministic, prompt-free behavior; output degrades gracefully when stdout is
  not a TTY. `NO_COLOR` disables color.

## Gotchas

- **Worktrees live beside the repo, not inside it** — at `<repo>.worktrees/<name>/`,
  a sibling directory. They are not created under the main working tree.
- **Random names by default.** New worktrees get an adjective-noun name (e.g.
  `lively-otter`) unless you pass `-n/--name`.
- **The `create` positional is new-branch-only.** Naming a branch that already
  exists (locally or on the remote) is an error (exit 2) pointing you at
  `--checkout <branch>` — the explicit opt-in for existing branches. `--reuse`
  takes precedence over `--base`.
- **Init failure keeps the worktree.** If the init script exits non-zero, `wt`
  exits `7` (not `1`) and leaves the worktree in place with a retry hint
  (`cd <path> && wt init`) — the git operations already succeeded. It does not roll
  back. (Two non-failures still exit 0: a missing init command/file, and — for the
  default `fab sync` only — a repo that is not fab-managed.)
- **`wt open` can't `cd` without the shell wrapper.** That is a Unix constraint;
  install `eval "$(wt shell-init)"` to make "Open here" and `wt go` change your
  shell's directory.
- **Deleting a worktree externally leaves git bookkeeping.** If you `rm -rf` a
  worktree, run `git worktree prune` in the main repo to clean up.
