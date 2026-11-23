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
  `--checkout <branch>` puts it on an **existing** branch instead; `--pr <n>`
  puts it on pull request `n`'s head branch (via `gh`); `--base <ref>`
  sets a new branch's start-point (pinned to its resolved SHA); `--fetch` fetches
  the base's (or `--temp` ref's) remote first so the worktree starts from fresh refs;
  `--json` prints one receipt document
  `{ok,result:{path,name,branch,base_ref,base_sha,mode,init}}` on stdout instead
  of the path; `-n/--name` overrides the random name;
  `--reuse` reuses an existing named worktree; `--no-init` skips the init script.
  `--temp <ref>` makes a detached throwaway worktree (see Gotchas).
- `list` (alias `ls`) — table of worktrees (name, branch, path). `--status` adds
  dirty/unpushed indicators; `--json` emits machine records; `--path <name>`
  prints one absolute path.
- `open [name|path]` — launch a worktree (or any directory path, no git repo
  needed) in a detected target. `--list` prints
  the host-app table; `--list --json` prints the FULL registry: every emitted
  `id` is a valid `-a` value here and now (`kind` = what it is, `locus` = where
  the effect lands: gui / session / caller / host). Inside tmux/byobu the
  multiplexer targets (`tmux_window`, `tmux_session`, `byobu_tab`) are the
  usual choice on a headless host: `wt open <wt> -a tmux_window`. The row
  marked `default: true` is what `-a default` resolves to.
- `go [name]` — **select** a worktree: no arg → selection menu; a name → direct.
  By default navigates (cd) there; `--open <prompt|default|skip|app>` launches the
  selection instead (go owns the worktree menu, open owns the app menu; this flag
  composes them). Requires a git repo.
- `delete [names...]` (alias `rm`) — remove worktrees with optional branch cleanup.
  `--all/-a` removes every worktree; `-s/--stash` stashes uncommitted changes first;
  `--branch <true|false|auto>` and `--no-remote` control branch deletion.
- `init` — run the worktree init script for the current worktree/main repo.
- `shell-init <shell>` — print the shell wrapper function (`zsh` or `bash`) for
  `eval` in your profile; a missing/unsupported shell is a usage error (exit 2).
- `update` — self-update the binary via Homebrew.

## Composition patterns

- **Shell-wrapper eval flow.** A child process cannot `cd` its parent shell, so
  `wt` prints shell code that the user evals: `eval "$(wt shell-init zsh)"` (or
  `bash`) installs a function wrapping the binary. That function powers the "Open
  here" menu option in `wt open` and the navigation in `wt go`. Without it, those
  fall back to printing a path for the caller to `cd`.
- **Launcher contract (`WT_CD_FILE` / `WT_WRAPPER`).** `wt open` is the toolkit's
  canonical directory launcher — external callers (e.g. `hop`) delegate to it as a
  subprocess. "Open here" and `wt go` share one shell-cd contract: when
  `WT_CD_FILE` is set they write the resolved directory path there (mode 0600,
  truncate-on-write), and they always print the bare path as the last stdout
  line (`cd "$(command wt …)"` works without the wrapper); the caller applies
  the `cd` itself. Set `WT_WRAPPER=1` to signal you
  handle the `cd` and suppress the "wrapper not loaded" hint. A non-zero exit means
  do **not** trust `WT_CD_FILE`'s contents. See `docs/specs/launcher-contract.md`.
- **Init protocol (`WORKTREE_INIT_SCRIPT`).** Each new worktree runs an init script
  — default `fab sync`, override via `WORKTREE_INIT_SCRIPT`. A value with a space is
  a command invocation (first word looked up on PATH); a value without spaces is a
  file path resolved from the repo root and run via `bash`. It runs with the new
  worktree as its working directory; its output goes to stderr. See `docs/specs/init-protocol.md`.
- **Machine surface.** `wt list --json` is the structured composition surface (e.g.
  what `hop` reads); `wt create` prints the worktree path as its last stdout line, or —
  with `--json` — one receipt document (`{"ok":true,"result":{…}}` / `{"ok":false,"error":{code,message}}`).

## Output & exit-code contracts

- **stdout is data, stderr is human copy.** Machine results (the created worktree
  path, `list --json`, `list --path`, the `go` target path) go to stdout; all
  progress, prompts, warnings, and errors go to stderr. Init-script output is
  diagnostic and streams to stderr. So `p=$(wt create ...)` captures the path clean.
- **Errors are structured** `Error: <what>` / `Why: <why>` / `Fix: <fix>` on stderr;
  under `wt create --json` a failure document also goes to stdout (see Gotchas).
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
  `--checkout <branch>` — the explicit opt-in for existing branches.
- **`--reuse` doesn't mix with `--base` or `--fetch`.** Passing either with it is
  an error (exit 2), even when no worktree by that name exists yet — a reused
  worktree keeps its existing branch. To cut from a fresh ref, create a new
  worktree (drop `--reuse`): `wt create --name <n> --base origin/<b> --fetch`.
- **`wt create --json` is one document, nothing else, on stdout** — on success *and*
  failure. It implies `--non-interactive` and `--open skip` (an explicit non-skip
  `--open` is exit 2). `ok` is true iff the exit code is 0; failures carry
  `error.code` = the exit code. `mode` is `new|checkout|reuse|temp` (`--pr` →
  `checkout`); `base_ref`/`base_sha` are null for checkout/reuse, `branch` null for
  temp. Exit 7 (init failed, worktree kept) still carries `result` with `init:"failed"`.
- **`--fetch` is explicit and fails closed.** No fetch happens without it; a failed
  fetch exits 3 with nothing created. The new branch is pinned to the resolved
  `base_sha`, and a remote-tracking `--base` still gets its upstream (as plain git would).
- **`--pr` needs `gh` and a same-repository PR.** Without `gh` it exits `1`; a
  PR from a fork exits `2` — add the fork as a remote and use `--checkout`. It
  assumes gh's repo is `origin`; in a fork clone, use `--checkout` instead.
- **`--temp <ref>` = disposable detached worktree.** `wt create --temp <sha|branch|tag>`
  detaches at that commit (no branch, no git config), names it `tmp-<name>`, and skips
  init unless `--init`. `wt list --json` marks it `"temp": true, "branch": null`;
  `wt delete <tmp-name>` removes it with no branch handling. It is exclusive with the
  positional, `--checkout`, `--reuse`, `--base` and `--pr` (exit 2).
- **Init failure keeps the worktree.** If the init script exits non-zero, `wt`
  exits `7` (not `1`) and leaves the worktree in place with a retry hint
  (`cd <path> && wt init`) — the git operations already succeeded. It does not roll
  back. (Two non-failures still exit 0: a missing init command/file, and — for the
  default `fab sync` only — a repo that is not fab-managed.)
- **`wt open` can't `cd` without the shell wrapper.** That is a Unix constraint;
  install `eval "$(wt shell-init zsh)"` (or `bash`) to make "Open here" and
  `wt go` change your shell's directory.
- **Deleting a worktree externally leaves git bookkeeping.** If you `rm -rf` a
  worktree, run `git worktree prune` in the main repo to clean up.
