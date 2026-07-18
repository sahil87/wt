# Launcher Contract

> Specifies the contract between `wt open` and external callers that delegate
> to it via subprocess invocation. This is the formal version of a
> previously-implicit contract that consumers (notably
> [`hop`](https://github.com/sahil87/hop)) already rely on.

## 1. Purpose

`wt open` is the canonical directory launcher in this toolchain. It owns the
detected-app catalog (editors, terminals, file managers, tmux/byobu tabs and
sessions), the menu UX, the `last-app` cache, the `TERM_PROGRAM`-based default
detection, and the "Open here" shell-wrapper integration.

External tools that need to launch a directory in the user's chosen app SHOULD
delegate to `wt open` as a subprocess rather than reimplementing this logic.
Subprocess delegation lets the caller inherit `wt`'s view of the world
(cached defaults, session detection) for free, and avoids forking the app
catalog.

The alternative — extracting the apps subsystem into a public Go package and
having both `wt` and consumers import it — is explicitly deferred. See
`fab/changes/260508-evbf-wt-open-any-directory/spec.md` Design Decision 2.

## 2. Invocation Surface

```
wt open [<path>|<name>] [--app <app>]
```

| Argument | Behavior |
|----------|----------|
| (none) | Opens current worktree (when in a worktree), the main-repo selection menu (when in a non-worktree git repo with no `--app`), or the current working directory (when not in a git repo). |
| `<path>` | Treated as a literal path when `os.Stat(<path>)` succeeds and the entry is a directory. Works regardless of git context — the path may be unrelated to any repo. |
| `<name>` | Resolved as a worktree name (case-insensitive). The name `main` resolves to the main worktree (the repo root); an exact-basename match takes precedence, so a worktree directory literally named `main` still resolves to that worktree. **Requires** a git repository in the current working directory. From a non-git cwd, exits `ExitGeneralError` with a "name resolution requires a git repository" message; the message suggests passing a path and does NOT suggest cd'ing into a repo. |
| `--app <app>` | Opens directly in the named app, bypassing the menu. Works in all of the above contexts. The literal name `default` resolves to the auto-detected default app. Incompatible with the main-repo selection menu (`<no args>` from a non-worktree git repo) — combining the two exits `ExitInvalidArgs`. |

Path-arg precedence: when an arg is supplied, `os.Stat` + `IsDir()` is
attempted *first*. Name resolution is only attempted when the arg is not an
existing directory and the cwd is in a git repo.

## 3. `WT_CD_FILE`

`WT_CD_FILE` is a path to a writable file. When set:

- The "Open here" menu option (and `--app open_here`) writes the resolved
  directory path to that file instead of printing a `cd --` line to stdout.
- Consumers read the file after a zero-exit `wt open` invocation and apply the
  `cd` to the parent shell themselves (via shell wrapper, eval, or whatever
  mechanism the consumer uses).
- The file is overwritten on each invocation; truncate-on-open semantics.
- `wt` opens the file with mode `0600`, but Go's `os.WriteFile` only applies
  the mode when *creating* the file — it does not `chmod` an existing file.
  Consumers are therefore responsible for ensuring the path is either fresh
  (e.g., produced by `mktemp` per invocation) or already mode `0600`. Mode
  `0600` is intentional — the file may contain a path the user considers
  private; consumers MUST NOT relax this.

When `WT_CD_FILE` is unset, "Open here" falls back to printing `cd -- '<path>'`
to stdout (single-quoted with `'\''` escaping for shell safety).

**Reused by `wt go`.** The `wt go` selector (see
[`cli-surface.md`](cli-surface.md#wt-go-name)) navigates to a worktree using
this **same** `WT_CD_FILE` mechanism — no new environment variable is
introduced. It writes the resolved absolute path to `WT_CD_FILE` with the
identical semantics above (mode `0600`, truncate-on-write, contents = resolved
directory path), and additionally always prints the path to stdout as the last
line (so `cd "$(command wt go <name>)"` works without the wrapper). Because
`wt go` adds no new env-var name and does not alter any semantics in this
section or §5, the stability guarantees in §6 are unchanged — no constitution
amendment is required.

## 4. `WT_WRAPPER`

`WT_WRAPPER=1` signals that the caller is handling the `cd` itself (via
`WT_CD_FILE`, an outer shell wrapper, or equivalent). When set:

- Suppresses the "shell wrapper not loaded" hint that would otherwise print to
  stderr when `WT_CD_FILE` is unset and the user selects "Open here". The hint
  is informational ("eval `wt shell-init` to make this work") — confusing
  when displayed to a user whose consumer already handles `cd`.
- Has no other observable effect; in particular, it does NOT change exit-code
  semantics or alter the `WT_CD_FILE` write behavior.

Consumers that handle their own `cd` SHOULD set `WT_WRAPPER=1`. Consumers that
do not handle `cd` (e.g., scripts that don't care about the parent shell)
SHOULD leave it unset and ignore the stderr hint.

## 5. Exit-Code Contract

| Constant | Value | Meaning in launcher context |
|----------|-------|-----------------------------|
| `ExitSuccess` | 0 | App launch succeeded (or "Open here" wrote the cd file successfully). The consumer MAY trust the contents of `WT_CD_FILE`. |
| `ExitGeneralError` | 1 | Arg-resolution failure: unknown app (`--app foo`), unknown worktree name, name arg supplied from a non-git cwd, or no default app detected. |
| `ExitInvalidArgs` | 2 | Flag misuse — currently only `--app` combined with the main-repo selection menu. |
| `ExitGitError` | 3 | A git operation actually failed during name resolution (e.g., `git worktree list` errored). Does NOT apply to path-only or no-args-from-non-git invocations. |
| `ExitByobuTabError` | 5 | `byobu new-window` failed when the chosen app was `byobu_tab`. |
| `ExitTmuxWindowError` | 6 | `tmux new-window` or `tmux new-session` failed when the chosen app was `tmux_window` / `tmux_session`. |

**Critical rule for consumers**: a non-zero exit means the consumer MUST NOT
trust the contents of `WT_CD_FILE`. The file may be empty, stale, or contain
a partially-written path. Always check the exit code before reading.

## 6. Stability Guarantees

The following are **stable** parts of the contract — changes require a
constitution amendment (per the precedent set by Module Path Stability in
`fab/project/constitution.md`):

- The set of environment-variable names: `WT_CD_FILE`, `WT_WRAPPER`.
- The semantics of `WT_CD_FILE` (file path, mode `0600`, truncate-on-write,
  contents = resolved directory path).
- The semantics of `WT_WRAPPER=1` (suppresses the hint; no other effect).
- The exit-code values and their meanings as listed in §5.
- Path-arg precedence over name resolution.
- The behavior that path args work without a git repo and name args require
  one.

The following are **not** breaking changes — `wt` MAY evolve them freely:

- Adding new app types to the catalog.
- Adding new menu items.
- Reordering existing menu items.
- Adding new internal flags to `wt open`.
- Refining default-app detection heuristics.
- Cosmetic changes to error message wording (so long as the structure
  what/why/fix is preserved and exit codes are unchanged).

## 7. Non-Goals

- **Not a general-purpose `xdg-open` replacement.** `wt open` accepts
  directories only. URLs, individual files (`.md`, `.png`, etc.), and
  protocol handlers are out of scope. Consumers needing those should call
  `xdg-open` / `open(1)` / equivalent directly.
- **No support for non-existent paths.** A path arg that does not exist (or
  is not a directory) falls through to name resolution; if name resolution
  also fails, the command exits with an arg-resolution error. `wt open` will
  not create the path on the user's behalf.
- **No remote URL support.** `wt open` does not fetch, clone, or otherwise
  resolve remote URLs. `git@host:org/repo.git`, `https://...`, and similar
  inputs are treated as name args (which will fail).
- **No bidirectional state.** `wt` does not signal the consumer beyond the
  exit code and `WT_CD_FILE` contents. There is no return-channel for
  selected-app metadata, menu cancellation reasons, etc. Consumers that need
  richer signaling should fork the design or wait for the deferred
  shared-library extraction.
