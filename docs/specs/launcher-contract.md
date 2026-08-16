# Launcher Contract (v2)

> Specifies the contract between `wt open` and external callers that delegate
> to it via subprocess invocation. This is the formal version of a
> previously-implicit contract that consumers (notably
> [`hop`](https://github.com/sahil87/hop)) already rely on.
>
> **v2** (change `260722-0is3-go-open-orthogonality`): the shell-cd mechanism
> is unified — see §3 and the §6 changelog note.

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
wt open --list [--json]
```

| Argument | Behavior |
|----------|----------|
| (none) | Opens the **current context**: the current worktree root (when in a worktree), the repo root (when in a non-worktree git cwd — a one-line transitional stderr tip points worktree-picking at `wt go`), or the current working directory (when not in a git repo). `wt open` runs no worktree-selection menu — worktree selection is `wt go`'s job (`wt go --open` composes select-then-launch). |
| `<path>` | Treated as a literal path when `os.Stat(<path>)` succeeds and the entry is a directory. Works regardless of git context — the path may be unrelated to any repo. |
| `<name>` | Resolved as a worktree name (case-insensitive). The name `main` resolves to the main worktree (the repo root); an exact-basename match takes precedence, so a worktree directory literally named `main` still resolves to that worktree. **Requires** a git repository in the current working directory. From a non-git cwd, exits `ExitGeneralError` with a "name resolution requires a git repository" message; the message suggests passing a path and does NOT suggest cd'ing into a repo. |
| `--app <app>` | Opens directly in the named app, bypassing the app menu. Orthogonal to every target form above, including all three no-arg current-context forms. The literal name `default` resolves to the auto-detected default app. *(v2: the former `ExitInvalidArgs` incompatibility with the main-repo no-arg form is retired — there is no selection menu on that path.)* |
| `--list [--json]` | **Query form** — lists the detected launchable host apps and exits without launching anything. Requires no git repository and no target. `--json` emits a JSON array of `{id, label, kind}` records (`kind` ∈ `editor` / `terminal` / `file-manager`); every emitted `id` is guaranteed accepted by `wt open <path> -a <id>` (the listing derives from the same `BuildAvailableApps()` catalog `-a` resolution uses), making the output a validation source for consumer launch paths. Action rows (`open_here`, `copy_*`, `byobu_tab`, `tmux_window`, `tmux_session`) are excluded from the listing (they signal `wt`'s own process environment, not the consumer's) but remain valid `-a` values. Zero detected apps emit `[]`, not `null`, and exit 0. Ordering is `BuildAvailableApps()` detection order. Mutually exclusive with a positional arg, `--app`, and the deprecated `--select`/`--go` (`ExitInvalidArgs`); `--json` without `--list` also exits `ExitInvalidArgs`. Added under §6's "adding new internal flags to `wt open`" non-breaking evolution clause. |

The deprecated `--select` / `--go` flags (hidden from `--help`, stderr warning
`use "wt go --open" instead`) still perform the former select-then-launch
composition and remain functional until a later removal change. New consumers
compose selection and launch via `wt go --open` instead.

Path-arg precedence: when an arg is supplied, `os.Stat` + `IsDir()` is
attempted *first*. Name resolution is only attempted when the arg is not an
existing directory and the cwd is in a git repo.

## 3. The unified shell-cd contract (`WT_CD_FILE` + stdout)

**One mechanism, two verbs.** As of v2, `wt go` navigation and the launcher's
"Open here" action (`--app open_here`, the menu row) share a **single**
shell-cd implementation (`internal/worktree`'s `NavigateTo`). Every shell-cd
emission does all of the following, in order:

1. **`WT_CD_FILE` write (when set).** `WT_CD_FILE` is a path to a writable
   file. The resolved directory path is written to it with truncate-on-write
   semantics. `wt` opens the file with mode `0600`, but Go's `os.WriteFile`
   only applies the mode when *creating* the file — it does not `chmod` an
   existing file. Consumers are therefore responsible for ensuring the path is
   either fresh (e.g., produced by `mktemp` per invocation) or already mode
   `0600`. Mode `0600` is intentional — the file may contain a path the user
   considers private; consumers MUST NOT relax this. A write failure aborts
   before any success output.
2. **Wrapper hint (stderr, gated).** When `WT_CD_FILE` is unset and
   `WT_WRAPPER != "1"`, a two-line "shell wrapper not loaded" hint prints to
   stderr (see §4).
3. **Confirmation (stderr).** A two-line human confirmation:
   `→ {repo} / {dir}  ({branch})` plus the two-space-indented absolute path.
   Outside a git context the first line degrades to `→ {dir}`. Diagnostic copy
   only — consumers MUST NOT parse it.
4. **Bare path on stdout (always).** The resolved absolute path is printed to
   stdout as the **last line**, so the no-wrapper scripting form works
   uniformly: `cd "$(command wt go <name>)"` /
   `cd "$(command wt open <path> -a open_here)"`.

Consumers read `WT_CD_FILE` after a zero-exit invocation and apply the `cd` to
the parent shell themselves (via shell wrapper, eval, or whatever mechanism the
consumer uses). The `wt shell-init` wrapper reads `WT_CD_FILE` only — never
stdout — so it is unaffected by step 4.

**Retired: the `cd -- '<path>'` stdout fallback.** In v1, "Open here" with
`WT_CD_FILE` unset printed an eval-able `cd -- '<path>'` line to stdout
(mutually exclusive with the file write). v2 retires that form: consumers that
eval'd stdout switch to the `cd "$( … )"` command-substitution form, which the
always-bare-path stdout serves directly.

## 4. `WT_WRAPPER`

`WT_WRAPPER=1` signals that the caller is handling the `cd` itself (via
`WT_CD_FILE`, an outer shell wrapper, or equivalent). When set:

- Suppresses the "shell wrapper not loaded" hint that would otherwise print to
  stderr when `WT_CD_FILE` is unset and a shell-cd emission runs ("Open here"
  or `wt go` navigation). The hint is informational ("eval `wt shell-init` to
  make this work") — confusing when displayed to a user whose consumer already
  handles `cd`.
- Has no other observable effect; in particular, it does NOT change exit-code
  semantics or alter the `WT_CD_FILE` write behavior.

Consumers that handle their own `cd` SHOULD set `WT_WRAPPER=1`. Consumers that
do not handle `cd` (e.g., scripts that don't care about the parent shell)
SHOULD leave it unset and ignore the stderr hint.

## 5. Exit-Code Contract

| Constant | Value | Meaning in launcher context |
|----------|-------|-----------------------------|
| `ExitSuccess` | 0 | App launch succeeded (or a shell-cd emission completed, incl. the `WT_CD_FILE` write). The consumer MAY trust the contents of `WT_CD_FILE`. |
| `ExitGeneralError` | 1 | Arg-resolution failure: unknown app (`--app foo`), unknown worktree name, name arg supplied from a non-git cwd, or no default app detected. |
| `ExitInvalidArgs` | 2 | Flag misuse — the `--list`/`--json` exclusivity cases (`--list` with a positional target / `--app` / `--select`/`--go`; `--json` without `--list`). *(v2: the former "`--app` with the main-repo selection menu" case is retired — that path now opens the repo root and exits per its outcome.)* |
| `ExitGitError` | 3 | A git operation actually failed during name resolution (e.g., `git worktree list` errored). Does NOT apply to path-only or no-args-from-non-git invocations. |
| `ExitByobuTabError` | 5 | `byobu new-window` failed when the chosen app was `byobu_tab`. |
| `ExitTmuxWindowError` | 6 | `tmux new-window` or `tmux new-session` failed when the chosen app was `tmux_window` / `tmux_session`. |

**Critical rule for consumers**: a non-zero exit means the consumer MUST NOT
trust the contents of `WT_CD_FILE`. The file may be empty, stale, or contain
a partially-written path. Always check the exit code before reading.

## 6. Stability Guarantees

> **Changelog — v2 amendment (2026-07-22).** This revision was an **authorized
> amendment** under the rule below: the user explicitly waived the v1 freeze
> ("we can update hop also", 2026-07-22 discussion; change
> `260722-0is3-go-open-orthogonality`) to unify the shell-cd mechanism. What
> changed: §2's no-arg behavior (current context; the main-repo selection menu
> moved to `wt go`), §3's unified mechanism (bare path always on stdout; the
> `cd -- '<path>'` fallback retired), and §5's `ExitInvalidArgs` row (the
> `--app`+menu case retired). The env-var names and `WT_CD_FILE`/`WT_WRAPPER`
> semantics are unchanged; hop was verified against v2 at design time (it sets
> `WT_CD_FILE` via its shell shim, inherits stdio without parsing stdout, and
> checks exit codes only — no hop change required).

The following are **stable** parts of the contract *as revised by v2* —
further changes require an explicit authorized amendment (per the precedent
set by Module Path Stability in `fab/project/constitution.md`):

- The set of environment-variable names: `WT_CD_FILE`, `WT_WRAPPER`.
- The semantics of `WT_CD_FILE` (file path, mode `0600`, truncate-on-write,
  contents = resolved directory path).
- The semantics of `WT_WRAPPER=1` (suppresses the hint; no other effect).
- The unified shell-cd emission shape of §3 (file write when set; bare
  resolved path always the last stdout line).
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
