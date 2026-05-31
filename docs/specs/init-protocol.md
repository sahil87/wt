# Init Protocol

How `wt init` (and the implicit init step in `wt create`) discovers, resolves,
and runs the per-worktree initialization script.

## Lookup contract

The init script value is resolved by `worktree.InitScriptPath()`:

1. If the environment variable `WORKTREE_INIT_SCRIPT` is set and non-empty,
   its value is used verbatim.
2. Otherwise, the default value `"fab sync"` is used.

```go
func InitScriptPath() string {
    if v := os.Getenv("WORKTREE_INIT_SCRIPT"); v != "" {
        return v
    }
    return "fab sync"
}
```

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

The init step is non-blocking when the script cannot be located. Two cases:

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

This means a freshly-cloned repo without an init script (and without
fab-kit installed) silently no-ops on `wt init`, which is the desired
behavior for the "I just want to use the worktree" path.

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

For `wt create`, init failure does **not** roll back the just-created
worktree. The git operations all succeeded; only the user-supplied script
failed. The worktree directory, the branch, and any fetched refs are all
kept. The user sees a structured banner containing:

- A status line with the extracted exit code (or a generic phrase if the
  underlying error does not unwrap to `*exec.ExitError`).
- The absolute worktree path that was kept.
- A copy-paste-ready retry hint: `cd <wtPath> && wt init` (uses `&&`, never
  `;`, so it parses identically in bash/zsh/fish).
- A remove hint: `wt delete <name>`.

The `wt create --reuse` path is exempt from `ExitInitFailed`: a reused
worktree is presumed functional pre-existing, so its init step is a refresh
(warn-but-continue) rather than a gate. `wt init` also exits with
`ExitInitFailed` when the init script it located exits non-zero.

### SIGINT during init

When the user sends SIGINT (or SIGTERM) after `wt create` has finished its
git operations and while the init script is running, the signal is delivered
to the init process group only (the script and any descendants it spawned).
The worktree is kept and `wt` exits with `ExitInitFailed`, just like a
natural init-script failure. SIGINT during the git-operations phase preserves
the existing semantics (rollback + exit 130).
