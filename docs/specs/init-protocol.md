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

`stdin`, `stdout`, and `stderr` are inherited from `wt init`'s parent process,
so interactive scripts (prompts, password reads) work normally.

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

## Script failure semantics

When the init script **is** found and executed but exits non-zero, `wt init`
returns the error from `RunE`, which the root cobra handler maps to
`ExitGeneralError` (1) via `os.Exit(wt.ExitGeneralError)`.

For `wt create`, an init failure additionally triggers a rollback of the
just-created worktree before exiting — see `internal/worktree/rollback.go`.
