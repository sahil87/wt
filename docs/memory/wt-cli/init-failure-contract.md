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

### Terminal foreground is reclaimed after the init child (job-control bookkeeping)

> Lives **next to** the SIGINT Option B contract above and preserves all of its invariants. This is purely additive job-control bookkeeping around the same init region — it touches only the **terminal's** foreground process group, never the init child's process group.

- `wt create` AND `wt init` capture the controlling terminal's foreground process group (`tcgetpgrp` on `os.Stdin.Fd()`) **immediately before** running the init child, and reclaim it (`tcsetpgrp` back to that captured pgrp — `wt`'s own group, since `wt` is the foreground at capture time) **after** the child returns, on ALL exit paths:
  - **`wt create`** (`cmd/wt/create.go`):
    - Capture is adjacent to the SIGINT-Option-B `signal.Reset` (`create.go:292–304`), gated on `term.IsTerminal(os.Stdin.Fd())`. If `tcgetpgrp` itself errors on a TTY, the bookkeeping is disabled (no reclaim to a bogus pgrp) rather than guessed.
    - **Success path**: explicit reclaim after `signal.Stop`/`close`, **before** the Open phase separator + menu render (`create.go:361–363`) — the load-bearing reclaim that prevents the Open-phase TTY write from SIGTTOU-suspending `wt`.
    - **Init-failure path**: explicit reclaim **before** `PrintInitFailureBanner` + `os.Exit(ExitInitFailed)` (`create.go:349–351`) — the banner is itself a TTY write and would SIGTTOU if foreground were lost.
    - **Panic / non-`os.Exit` early-return**: a best-effort `defer reclaimTerminalForeground(...)` installed immediately after capture (`create.go:314–316`). Because both load-bearing paths exit via `os.Exit` (which skips deferred funcs) or fall through to Open, this defer only actually fires on a panic or a non-`os.Exit` return. (The pre-init SIGINT exit-130 path runs before this defer is installed and before any foreground was captured, so it has nothing to reclaim.)
  - **`wt init`** (`cmd/wt/init.go::runInitScript`): same capture before `cmd.Run()` (`init.go:96–113`), explicit reclaim before the failure trailer + `os.Exit(ExitInitFailed)` (`init.go:121–124`) and before the `Worktree init complete.` trailer on success (`init.go:129–132`), plus the same best-effort `defer` safety net. A standalone interactive `wt init` (no Open phase) would otherwise strand the user's shell at a suspended prompt after returning.
- The reclaim wraps the `tcsetpgrp` ioctl in `signal.Ignore(syscall.SIGTTOU)` with `defer signal.Reset(syscall.SIGTTOU)` (`tty_unix.go`). Without this, a `tcsetpgrp` issued from a possibly-background process is itself a foreground-mutating operation the kernel answers with SIGTTOU — the reclaim that is supposed to un-stick `wt` would stick it instead. `signal.Reset` is the correct restore because `wt` installs no other SIGTTOU handler.
- All bookkeeping is **gated on `term.IsTerminal(os.Stdin.Fd())`** — when stdin is not a TTY (`--non-interactive`, piped, CI), capture and reclaim are complete no-ops: no syscall, no error, no warning. This mirrors the existing `isInteractiveTTY()` gate in `menu.go` and Constitution VI.
- Reclaim errors are intentionally ignored (`_ = unix.IoctlSetPointerInt(...)`): the bookkeeping is best-effort cleanup that MUST never block `rb.Execute()` rollback or alter exit codes.
- **Preserved SIGINT Option B invariants**: `SysProcAttr{Setpgid: true}` on the init child is unchanged (the reclaim never touches the child's pgrp); the tight reinstall window is NOT widened (`tcgetpgrp` is a single cheap syscall with no I/O or prompt, placed adjacent to `signal.Reset`); `defer rb.Execute()` remains the panic fallback and the foreground-restore defer is independent of it.
- Helpers `terminalForeground(ttyFd int) (int, error)` (tcgetpgrp via `unix.IoctlGetInt(fd, unix.TIOCGPGRP)`) and `reclaimTerminalForeground(ttyFd int, pgrp int)` (tcsetpgrp via `unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgrp)`) live in `src/cmd/wt/tty_unix.go` (`//go:build !windows`) with no-op stubs in `src/cmd/wt/tty_windows.go` (`//go:build windows`) — mirroring the `init_unix.go`/`init_windows.go` and `signal_unix.go`/`signal_windows.go` build-tag split. Windows has no `tcsetpgrp` / process-group terminal model, so the stubs do nothing (`terminalForeground` returns `(0, nil)`). The ioctls use `golang.org/x/sys/unix`, promoted from indirect to direct in `src/go.mod` with no new module (already present transitively via `golang.org/x/term`, used by `ShowMenu`).

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

### Defensive reclaim-after over proactive foreground handoff

`wt` does NOT proactively hand the terminal to the init child before running it (`tcsetpgrp` child→foreground, then reclaim). Instead it captures its own foreground pgrp, runs the child sharing the TTY exactly as before, and reclaims after. The real-world trigger validated this choice: an interactive child in an init script (`zsh -i` in a direnv-hook check at `doctor.sh:111`, reached via `fab sync` → `1-prerequisites.sh`) `tcsetpgrp`s itself to the foreground at startup and exits without restoring it. Because the whole init subtree is in a separate pgrp (`Setpgid: true`), foreground is stranded and `wt` is orphaned to the background, so its next TTY write (the Open-phase menu, or a `wt init` trailer) SIGTTOU-suspends it (`zsh: suspended (tty output)`). The init child ran *correctly* sharing the TTY — only the *cleanup* was missing. Reclaim-after restores ownership regardless of which descendant grabbed it, without `wt` needing to know who did. Proactive handoff was rejected as added failure surface for no benefit. The grabber lives in another repo (`loom`) that `wt` cannot police, so the fix has to live in `wt`. (Source: change `260602-z4p7`, intake assumptions #2/#3/#4.)

### Banner text is not byte-pinned

Tests assert presence of the worktree path, the `wt init` retry hint, and the `wt delete <name>` remove hint — NOT byte-equality of the banner template. Wording can evolve without test churn; the contract is the information surface, not the prose.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260516-g5e7-wt-create-init-failure-dx` | 2026-05-16 | Established kept-worktree contract, `ExitInitFailed = 7`, unified `ResolveInitInvocation`, `PrintInitFailureBanner`, SIGINT Option B, and the `WT_TEST_NO_LAUNCH=1` test seam. |
| `260531-pnmi-add-phase-separators` | 2026-05-31 | Corrected the resolver note: callers' streaming setups are no longer different — both `wt init` and `wt create` now wire init `cmd.Stdout`/`cmd.Stderr` to `os.Stderr`; only the working directory still differs. See `create-output-phases.md`. |
| `260602-z4p7-wt-reclaim-tty-foreground-after-init` | 2026-06-02 | Added the terminal-foreground reclaim contract: `wt create` and `wt init` capture the controlling terminal's foreground pgrp (`tcgetpgrp` on `os.Stdin.Fd()`) before the init child and reclaim it (`tcsetpgrp` to `wt`'s own group) after, on all exit paths (success before Open; init-failure before `PrintInitFailureBanner`; best-effort `defer` for panic/early-return). Reclaim wraps `tcsetpgrp` in `signal.Ignore(syscall.SIGTTOU)`; all bookkeeping gated on `term.IsTerminal` (no-op when piped / `--non-interactive` / CI). New `terminalForeground`/`reclaimTerminalForeground` helpers in `tty_unix.go` (no-op `tty_windows.go` stubs), `x/sys` promoted indirect→direct (no new module). Lives next to and preserves the SIGINT Option B contract (`Setpgid: true`, tight reinstall window, `defer rb.Execute()` all unchanged). |

## Cross-references

- Spec doc: `docs/specs/init-protocol.md` — wire contract + "Script failure semantics" + "SIGINT during init" sections (rewritten by this change).
- Source: `src/internal/worktree/init.go` (resolver, `InitNotFound`, `RenderWarning`), `src/internal/worktree/errors.go` (`ExitInitFailed`, `PrintInitFailureBanner`), `src/internal/worktree/crud.go` (`RunWorktreeSetup`, `RunWorktreeSetupWithObserver`), `cmd/wt/create.go` (rollback disarm + handler swap + foreground capture/reclaim around the init block), `cmd/wt/init.go` (resolver consumer + foreground capture/reclaim around `runInitScript`'s `cmd.Run()`), `src/internal/worktree/apps.go` (`OpenInApp` test seam), `src/cmd/wt/tty_unix.go` / `src/cmd/wt/tty_windows.go` (`terminalForeground` / `reclaimTerminalForeground` build-tag split).
- Tests: `src/cmd/wt/tty_pty_test.go` — `TestIntegration_ReclaimForegroundAfterInit_NotStopped` (Unix-only, `//go:build !windows`): allocates a real PTY via an in-tree `openpty` helper on `x/sys/unix` (`/dev/ptmx` + `TIOCSPTLCK`/`TIOCGPTN` + `/dev/pts/N`, no `creack/pty`), runs `wt create` under it via a session-leader launcher with a foreground-stranding init script, and asserts the `wt` process is not left `WIFSTOPPED`; self-skips when a PTY / process groups can't be set up. Verified non-vacuous (fails `WIFSTOPPED` without the reclaim).
- Constitution: Principle III (Typed Exit Codes) — `ExitInitFailed` is the canonical example for "worktree exists, init didn't complete". Principle I (Single-Binary CLI, slim deps) — drove the `x/sys/unix` ioctls (no new module) and the in-tree `openpty` helper (no `creack/pty`). Principle VI (Interactive by Default, Scriptable on Demand) — the reclaim restores the broken interactive Open path; the `term.IsTerminal` gate keeps the scriptable / non-interactive path a no-op.
- Sibling memory: `wt-cli/create-output-phases.md` — the Open-phase separator + menu render now sits behind the unconditional pre-Open foreground reclaim.
