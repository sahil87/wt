# Init Protocol

How `wt init` (and the implicit init step in `wt create`) discovers, resolves,
and runs the per-worktree initialization script.

## Lookup contract

The init script value is resolved by `worktree.InitScriptPath()`:

1. If the environment variable `WORKTREE_INIT_SCRIPT` is set and non-empty,
   its value is used verbatim.
2. Otherwise, the default value `"fab sync"` is used.

Alongside the value, `InitScriptPath` reports its **provenance** — whether the
value is the built-in default or an explicit override:

```go
func InitScriptPath() (script string, isDefault bool) {
    if v := os.Getenv("WORKTREE_INIT_SCRIPT"); v != "" {
        return v, false
    }
    return "fab sync", true
}
```

`isDefault` is true **only** when `WORKTREE_INIT_SCRIPT` is unset/empty. It is
**provenance, not string equality**: an explicit `WORKTREE_INIT_SCRIPT="fab sync"`
returns `("fab sync", false)` even though the string matches the default. The
run-time graceful-skip classification (see **Graceful skip behavior** case 3)
keys on this flag, so an explicitly configured script always fails hard while the
built-in default may skip gracefully in a non-fab-managed repo.

The default exists so that users with `fab-kit` installed get a working init
flow out of the box. Users who do not run fab-kit override the env var
(typically in `.envrc`, `~/.zshrc`, or a project-local rc file) to point at
their own script.

## Command-vs-path detection

`wt init` decides at runtime whether the resolved value is a **command
invocation** or a **filesystem path** by checking for whitespace:

| Resolved value | Detection rule | Treatment |
|----------------|---------------|-----------|
| Contains a space (e.g. `fab sync`, `make init args`) | Command invocation | The first word is looked up via `exec.LookPath`. The remaining words are passed as arguments. |
| No spaces (e.g. `scripts/init.sh`, `init.sh`) | File path | Resolved relative to the main repo root. The script is invoked via `bash <path>`. |

The main repo root is computed from `git rev-parse --git-common-dir` (same
resolution used by `RepoContext`), so a relative path resolves identically
whether `wt init` was invoked from the main repo or from a linked worktree.

## Working directory

The init script (command or file) runs with its working directory set to the
**current worktree's** top level — `git rev-parse --show-toplevel`. This lets
init scripts mutate the active worktree (install dependencies, sync
templates, set up symlinks) without needing to know which worktree they are
in.

The init script's `stdin` is inherited from the parent process, so interactive
scripts (prompts, password reads) work normally. Its `stdout` and `stderr` are
both wired to the parent's **stderr** — init output is diagnostic, and the
parent's stdout is reserved for machine-readable results (e.g. `wt create`'s
final worktree-path line). This holds for both `wt create`'s init step and
standalone `wt init`; the two were aligned so init diagnostics always go to
stderr regardless of which command runs the script.

## Phase separators

Both `wt create` and `wt init` frame the init step with a phase separator
written to **stderr** — a labeled rule of the form `── Init (<cmd>) ──…`
(unicode) or `-- Init (<cmd>) --…` (plain, when `NO_COLOR` is set), where
`<cmd>` is the resolved init command (e.g. `fab sync`, or a script path). The
separator is emitted by the init runner only when a command is actually
resolved and run; on the not-found path (`*InitNotFound`) the canonical
`RenderWarning()` is printed and **no** separator is emitted. `wt create`
additionally frames its git and open phases with `── Git ──…` and `── Open ──…`
separators on stderr; the stdout final-path line is never touched. The
separator helper (`PhaseSeparator`) lives in `internal/worktree/errors.go`.

## Graceful skip behavior

The init step is non-blocking when the script cannot be located, or when the
built-in default does not apply. Cases 1–2 are **resolve-time** (the script
never runs); case 3 is **run-time** (the default script runs and reports it
does not apply).

1. **Command not on PATH** (e.g., `fab` not installed):
   ```
   Warning: "fab" not found on PATH, skipping init
   Install fab-kit or set WORKTREE_INIT_SCRIPT to a custom script.
   ```
   `wt init` exits 0 — the warning is informational, not an error.

2. **File path not found**:
   ```
   No init script found at: <repo-root>/<resolved-path>

   To add an init script:
     mkdir -p <dir>
     touch <resolved-path>
   ```
   Again, exit 0.

3. **Default init not applicable** (repo is not fab-managed):
   ```
   Warning: not a fab-managed repo — skipping init (default "fab sync" does not apply)
   Set WORKTREE_INIT_SCRIPT to a custom script, or run 'fab init' to make this repo fab-managed.
   ```
   When the **built-in default** `fab sync` runs and exits `ExitNotManaged = 3`
   (fab-kit ≥ PR #471 checks a config walk-up before any git resolution), `wt`
   treats it as "the default does not apply" rather than an init failure. This is
   a **run-time classification** — resolution succeeded and the script genuinely
   ran; only after `cmd.Run()` returns is the `(isDefault, exit code 3)` pair
   inspected (via `errors.As` → `*exec.ExitError`) by the shared helper
   `DefaultNotApplicable`. On the skip: the warning above is printed to stderr,
   `wt init` exits 0, and `wt create` proceeds to its Open phase exactly as on
   init success (no kept-worktree banner, no open-anyway prompt, no
   `Worktree init complete.` line, exit 0).

   Two constraints scope this narrowly:
   - **Provenance-gated**: only the built-in default skips. An explicit
     `WORKTREE_INIT_SCRIPT="fab sync"` (`isDefault=false`) still hard-fails on
     exit 3 — the user opted into that script, so the failure is theirs to see
     (see **Script failure semantics**).
   - **Exit-3-only, no version detection**: only exit code 3 skips. An older
     fab-kit predating PR #471 exits 1 in a non-fab repo, which is not 3, so it
     degrades to today's hard-fail (`ExitInitFailed = 7` + banner). There is no
     fallback probe — the feature lights up when fab-kit updates.

   Because detection is post-run, the Init phase separator,
   `Running worktree init...`, and fab's own `ERROR: not in a fab-managed repo…`
   stderr line all print before the skip warning; the skip warning is the last
   word and the exit code is 0.

Cases 1–2 mean a freshly-cloned repo without an init script (and without
fab-kit installed) silently no-ops on `wt init`; case 3 extends that "I just
want to use the worktree" ergonomics to a repo where fab-kit **is** installed
but the repo itself is not fab-managed.

## Resolution contract

Both `wt init` and `wt create`'s init step route through a single resolver
in `internal/worktree/`:

```go
func ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error)
```

- On success: returns a runnable `*exec.Cmd` (with `Setpgid: true` on Unix
  so SIGINT can target the process group). Callers wire `cmd.Dir`/`Stdout`/
  `Stderr`/`Stdin` themselves.
- On structured not-found: returns `(nil, *InitNotFound, nil)`. The
  `*InitNotFound`'s `RenderWarning()` method produces the canonical verbose
  warning that both call sites print — they cannot drift.
- On unexpected error: returns `(nil, nil, error)` for failures the caller
  cannot recover from (e.g., an empty init-script string).

No other code in the repository performs `exec.LookPath` or `os.Stat`
parsing of the init-script string. `ResolveInitInvocation` is the canonical
contract.

## Script failure semantics

When the init script **is** found and executed but exits non-zero, `wt`
exits with `ExitInitFailed = 7` — a typed exit code distinct from
`ExitGeneralError` (1) so operators (shell wrappers, fab-kit, `hop`) can
programmatically detect "worktree exists, init didn't complete" and offer
a retry-init affordance.

**Exit-3 carve-out for the built-in default.** The one exception is the
default-not-applicable skip (see **Graceful skip behavior** case 3): when the
**built-in default** `fab sync` (`isDefault=true`) exits `ExitNotManaged = 3`,
`wt` does **not** exit 7 — it prints the skip warning and exits 0, treating the
init step as a no-op. This carve-out is narrow: it applies **only** to the
built-in default and **only** to exit code 3. An explicit `WORKTREE_INIT_SCRIPT`
value (including the literal `"fab sync"`) keeps `ExitInitFailed = 7` on **every**
non-zero exit, and the default itself keeps `ExitInitFailed = 7` on every non-zero
exit **other than 3**. All the failure behavior described below (kept worktree,
banner, open-anyway prompt) applies to those hard-failure paths, unchanged.

For `wt create`, init failure does **not** roll back the just-created
worktree. The git operations all succeeded; only the user-supplied script
failed. The worktree directory, the branch, and any fetched refs are all
kept. The user sees a structured banner containing:

- A status line with the extracted exit code (or a generic phrase if the
  underlying error does not unwrap to `*exec.ExitError`).
- The absolute worktree path that was kept.
- A navigation hint: `wt go '<name>'` — the selection-free command to reach
  the kept worktree. This hint lives in the banner itself, so it appears on
  every path (interactive and non-interactive).
- A copy-paste-ready retry hint: `cd <wtPath> && wt init` (uses `&&`, never
  `;`, so it parses identically in bash/zsh/fish).
- A remove hint: `wt delete <name>`.

**Interactive open-anyway prompt.** When `wt create` is interactive (stdin is
a TTY and `--non-interactive` was not passed), after printing the banner it
prompts `Continue and open the worktree anyway?`. Answering **yes** opens the
kept worktree via the normal Open phase (app menu); answering **no** skips the
Open phase. **In both cases `wt create` still exits `ExitInitFailed = 7`** — a
successful open never downgrades the exit code, so operators always see the
init-failure signal. When `wt create` is **non-interactive** (piped, CI, or
`--non-interactive`), no prompt is shown: it prints the banner and exits 7
immediately, exactly as before.

`ExitInitFailed = 7` thus holds on **every** init-failure path: non-interactive,
interactive open-anyway yes (even after a successful open), and interactive no.

The `wt create --reuse` path is exempt from `ExitInitFailed`: a reused
worktree is presumed functional pre-existing, so its init step is a refresh
(warn-but-continue) rather than a gate. `wt init` also exits with
`ExitInitFailed` when the init script it located exits non-zero (it has no
open-anyway prompt — it is not a worktree-creating command).

### SIGINT during init

When the user sends SIGINT (or SIGTERM) after `wt create` has finished its
git operations and while the init script is running, the signal is delivered
to the init process group only (the script and any descendants it spawned).
The worktree is kept and `wt` exits with `ExitInitFailed`, just like a
natural init-script failure. SIGINT during the git-operations phase preserves
the existing semantics (rollback + exit 130).
