# wt-cli: Init Failure Contract

> Post-implementation behavior capture for the `wt create` init-failure DX overhaul.
> Source change: `260516-g5e7-wt-create-init-failure-dx`.

This file documents the contract that `wt create` and `wt init` honor when the init script is missing or fails. Future changes touching `cmd/wt/create.go`, `cmd/wt/init.go`, or `src/internal/worktree/{init.go,errors.go,crud.go,apps.go}` should preserve these invariants unless an explicit spec amendment supersedes them.

## Requirements

### Init invocation resolution is unified

- `src/internal/worktree/init.go` exposes a single resolver:
  ```go
  func ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error)
  ```
- Both `cmd/wt/init.go` (the `wt init` subcommand) and `src/internal/worktree/crud.go` (the `wt create` init step, via `RunWorktreeSetup`) consume this resolver.
- No other code may re-implement the command-vs-path detection, `exec.LookPath` check, or file-existence check. Duplicating this logic is an explicit anti-pattern.
- The resolver leaves `Dir`, `Stdout`, `Stderr` unset on the returned `*exec.Cmd` — callers wire those themselves because `wt init` and `wt create` use different working directories; as of `260531-pnmi` both now stream init diagnostics uniformly to stderr (`cmd.Stdout = cmd.Stderr = os.Stderr`), so only the working directory still differs between them (see `create-output-phases.md`).

### Not-found is non-fatal and uniformly warned

- `InitNotFound` is a **typed struct** (not a sentinel error). It carries a named `Kind` (`CommandNotOnPath` | `FileNotFound`) plus `Name` / `Path` / `RelPath` fields.
- `(InitNotFound).RenderWarning() string` is the **single rendering helper** for both call sites. Warning text MUST be byte-identical between `wt init` and `wt create`.
- Not-found is non-fatal: both call sites print the warning and continue. `wt create` keeps the worktree and exits 0. `ExitInitFailed` is NOT triggered by not-found.

### `wt create` keeps the worktree on init-script non-zero exit

- When the init script's process exits non-zero, the worktree directory, the branch, and any fetched refs are **kept**. The rollback is disarmed via `rb.Disarm()` (not `rb.Execute()`).
- This applies to both invocation paths in `cmd/wt/create.go` (auto-init and prompted-init).
- The `defer rb.Execute()` still fires on **any other** failure between worktree creation and the init step — only init-script non-zero exit is exempt from rollback.
- Not-found (handled above) is distinct from non-zero exit. Only "resolver succeeded, process executed, exited non-zero" triggers this requirement.

### Init failure exits with `ExitInitFailed = 7`

- `ExitInitFailed = 7` is declared in `src/internal/worktree/errors.go`, appended after `ExitTmuxWindowError = 6`. Existing exit codes (0–6) MUST NOT be renumbered.
- `cmd/wt/create.go` exits via `os.Exit(wt.ExitInitFailed)` on init-script non-zero exit — NOT via `wt.ExitWithError(...)` with a generic code.
- `cmd/wt/init.go` (the `wt init` subcommand) also exits via `os.Exit(wt.ExitInitFailed)` on init-script non-zero exit. Returning the error to Cobra would map to `ExitGeneralError = 1`; the explicit `os.Exit` ensures both command paths emit the typed code.
- Operators (shell wrappers, fab-kit, `hop`) can distinguish "worktree exists, init didn't complete" from any other generic failure.

### `--reuse` is exempt from the new failure contract

- The `--reuse` code path in `cmd/wt/create.go` keeps its prior "warn-but-continue on init failure" semantics.
- The reuse path does NOT adopt `ExitInitFailed`. A reused worktree is presumed functional pre-existing; init there is a refresh, not a gate.

### Init-failure banner

- `internal/worktree/errors.go` exposes `PrintInitFailureBanner(wtPath, name string, err error)`.
- Banner contents, in order: status line (with `*exec.ExitError` numeric code when available, generic phrasing otherwise) + reminder that init output streamed above; kept-worktree marker with absolute `wtPath`; retry hint `cd '<wtPath>' && wt init` (using `&&` so it parses in bash/zsh/fish); remove hint `wt delete '<name>'` (using the worktree name, not the absolute path, so it composes with `wt delete`'s name resolution). Path and name are both single-quoted via `shellQuoteSingle` so paths with spaces / shell metacharacters stay copy-paste-safe.
- Uses existing `ColorRed` / `ColorBold` / `ColorReset` helpers. Labels are confined to this one helper — no duplication elsewhere.

### SIGINT during init: Option B (handler swap + Setpgid)

- After `git worktree add` succeeds and **immediately before** the init step starts, `cmd/wt/create.go`:
  1. Calls `signal.Reset(syscall.SIGINT, syscall.SIGTERM)` to drop the rollback-based handler.
  2. Installs a new handler that signals the init `*exec.Cmd`'s process group.
- The init `*exec.Cmd` is constructed with `SysProcAttr.Setpgid = true` so the script's own children also receive the signal.
- After the child exits, control falls through to the natural init-failure path: banner + `ExitInitFailed`. The worktree is kept.
- Before worktree creation succeeds, the original SIGINT semantics apply: rollback executes and the process exits 130.
- The reinstallation window (between `git worktree add` returning and the new handler being installed) MUST stay tight — no I/O, prompts, or non-trivial work. If a panic occurs here, the original `defer rb.Execute()` still fires (losing the worktree is the correct fallback when the handler is in an inconsistent state).
- `RunWorktreeSetupWithObserver` is the variant exposed for SIGINT integration; `RunWorktreeSetup` is the thin wrapper used elsewhere.

### `RunWorktreeSetup` is thin (<50 lines)

- `RunWorktreeSetup(wtPath, initScript, repoRoot)` does only: resolve → warn-and-return if not-found → wire `Dir`/`Stdout`/`Stderr`/`Stdin` → `cmd.Run()` and return error verbatim.
- The confirmation prompt was originally inside the runner (gated by a `mode` parameter) but was hoisted up to `cmd/wt/create.go` after the Copilot PR review on PR #7. Reason: the init-phase SIGINT handler must be installed AFTER the prompt completes — otherwise Ctrl-C during the prompt is consumed by the init handler with no init child to target (deadlock). Callers that want a prompt MUST call `ConfirmYesNo("Initialize worktree?")` themselves before invoking the runner.
- All resolution/detection logic lives in `ResolveInitInvocation`.

### `WT_TEST_NO_LAUNCH=1` test seam in `OpenInApp`

- `src/internal/worktree/apps.go::OpenInApp` honors `WT_TEST_NO_LAUNCH=1`: every `appCmd` **except `open_here`** short-circuits, prints a `[wt-test-no-launch] ...` marker line to stderr, and returns nil instead of exec'ing the GUI/terminal/clipboard binary.
- The `open_here` case is exempt because it is cooperative (writes to `WT_CD_FILE` or stdout) and has no host side effect.
- The seam is defaulted **ON** in `cmd/wt`'s `runWt` test helper so `go test ./...` cannot leak real VSCode / Cursor / iTerm / etc. windows onto the developer's host.

## Design Decisions

### Typed struct over sentinel error for not-found

`InitNotFound` is a struct, not a sentinel `error`. Each call site renders the warning with a single `switch notFound.Kind` — sentinel errors would force `errors.As` plus a type assertion per case with no readability gain. `Kind` is a named type so the compiler can flag unhandled cases when new kinds are added. (Source: spec g5e7 "Init Invocation Resolution".)

### Option B (handler reinstall) over Option A (phase flag) for SIGINT

Option A — a phase flag plus a shared `currentInitCmd` reference guarded by a mutex — was explicitly rejected. Option B keeps the two phases' signal handling fully separated: phase A's handler is dropped via `signal.Reset` before phase B's handler is installed, so there is no shared mutable state and no mutex. The narrow reinstallation window is acceptable because `defer rb.Execute()` is still in scope as a panic fallback. (Source: spec g5e7 "Signal Handling During Init" + intake assumption set.)

### `&&` chaining in the retry hint

`cd <wtPath> && wt init` is the canonical retry copy-paste. `&&` was chosen over `;` because it parses identically in bash, zsh, and fish — `;` semantics differ subtly in fish, and a copied retry that silently runs `wt init` from the wrong directory would be worse than a syntax error.

### `--reuse` not migrated to `ExitInitFailed`

The reuse path's init step is a refresh, not a creation gate. Operators do not need to distinguish "reuse refresh failed" from "reuse refresh succeeded" — the worktree was already there. Keeping the existing warn-but-continue semantics avoids a behavior-change surface that would be invisible to most users and confusing to operators who scripted around the prior behavior.

### Banner text is not byte-pinned

Tests assert presence of the worktree path, the `wt init` retry hint, and the `wt delete <name>` remove hint — NOT byte-equality of the banner template. Wording can evolve without test churn; the contract is the information surface, not the prose.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260516-g5e7-wt-create-init-failure-dx` | 2026-05-16 | Established kept-worktree contract, `ExitInitFailed = 7`, unified `ResolveInitInvocation`, `PrintInitFailureBanner`, SIGINT Option B, and the `WT_TEST_NO_LAUNCH=1` test seam. |
| `260531-pnmi-add-phase-separators` | 2026-05-31 | Corrected the resolver note: callers' streaming setups are no longer different — both `wt init` and `wt create` now wire init `cmd.Stdout`/`cmd.Stderr` to `os.Stderr`; only the working directory still differs. See `create-output-phases.md`. |

## Cross-references

- Spec doc: `docs/specs/init-protocol.md` — wire contract + "Script failure semantics" + "SIGINT during init" sections (rewritten by this change).
- Source: `src/internal/worktree/init.go` (resolver, `InitNotFound`, `RenderWarning`), `src/internal/worktree/errors.go` (`ExitInitFailed`, `PrintInitFailureBanner`), `src/internal/worktree/crud.go` (`RunWorktreeSetup`, `RunWorktreeSetupWithObserver`), `cmd/wt/create.go` (rollback disarm + handler swap), `cmd/wt/init.go` (resolver consumer), `src/internal/worktree/apps.go` (`OpenInApp` test seam).
- Constitution: Principle III (Typed Exit Codes) — `ExitInitFailed` is the canonical example for "worktree exists, init didn't complete".
