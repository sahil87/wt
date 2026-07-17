---
type: memory
description: "Init-failure behavior of `wt create` / `wt init` — kept-worktree contract, `ExitInitFailed` on every path, the interactive open-anyway prompt, the `wt go` banner hint, SIGINT handling, and terminal-foreground reclaim."
---
# wt-cli: Init Failure Contract

> Post-implementation behavior capture for the `wt create` init-failure DX overhaul.
> Source change: `260516-g5e7-wt-create-init-failure-dx`
> (interactive open-anyway prompt + `wt go` banner hint + unified post-init reclaim/teardown + flag-based exit added by `260626-n6ma-create-init-failure-open-anyway`).

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

### Init failure exits with `ExitInitFailed = 7` on EVERY path

- `ExitInitFailed = 7` is declared in `src/internal/worktree/errors.go`, appended after `ExitTmuxWindowError = 6`. Existing exit codes (0–6) MUST NOT be renumbered.
- **`cmd/wt/create.go` exits 7 on ALL init-failure paths** — non-interactive, interactive open-anyway *Yes* (even when the open succeeds), and interactive *No*. As of `260626-n6ma`, exit 7 is NOT issued inline at the banner. Instead an `initFailed bool` flag is set in the init-failure branch (`create.go:376`), and a single end-of-function guard `if initFailed { os.Exit(wt.ExitInitFailed) }` (`create.go:476–478`) fires after the Open phase. That guard is placed **before** `fmt.Println(wtPath)` / `return nil`, so a *successful* open-anyway open can never downgrade the exit to 0 and erase the init-failure signal. The lone inline `os.Exit(wt.ExitInitFailed)` that remains is the non-interactive branch (`create.go:400`), reached only when no prompt is offered.
- `cmd/wt/init.go` (the `wt init` subcommand) also exits via `os.Exit(wt.ExitInitFailed)` on init-script non-zero exit. Returning the error to Cobra would map to `ExitGeneralError = 1`; the explicit `os.Exit` ensures both command paths emit the typed code. (`wt init` has no open-anyway prompt — it does not call `PrintInitFailureBanner` and is unchanged by `260626-n6ma`.)
- Operators (shell wrappers, fab-kit, `hop`) can distinguish "worktree exists, init didn't complete" from any other generic failure — including when the user chose to open the kept worktree anyway.

### Default-not-applicable run-time skip bypasses the failure contract (`260705-irnt`)

> The one carve-out from "`ExitInitFailed = 7` on EVERY path" above. When the **built-in default** init script `fab sync` runs in a repo that is not fab-managed and exits fab-kit's `ExitNotManaged = 3`, the whole failure contract is bypassed: no `ExitInitFailed`, no kept-worktree banner, no open-anyway prompt, no `Worktree init complete.` line, exit 0, and `wt create`'s Open phase proceeds exactly as on init success.

- **Run-time classification, not resolve-time.** This is a distinct axis from the not-found skip above: resolution *succeeds* and the script genuinely *runs*. Only after `cmd.Run()` returns non-zero is the outcome classified. So it composes with, and is mutually exclusive from, the resolve-time `CommandNotOnPath` / `FileNotFound` skips by construction (post-run vs. pre-run — a script that never ran cannot have exited 3; a script that exited 3 was resolved and ran). `ResolveInitInvocation` and `InitNotFound` are untouched.
- **Single shared helper, keyed on the documented exit code.** `src/internal/worktree/init.go` exposes `func DefaultNotApplicable(err error, isDefault bool) bool` — it returns true only when `isDefault` is true AND `err` unwraps (via `errors.As`) to an `*exec.ExitError` whose code is exactly the documented `exitNotManaged = 3` const. `func RenderDefaultSkipWarning() string` is the single renderer for the two-line skip copy. Both mirror the `RenderWarning()` anti-drift discipline so the two run sites cannot diverge. `wt` embeds the numeric `3` (documented as fab-kit's `ExitNotManaged`) rather than importing from fab-kit — separate repos, no import (Constitution I).
- **Provenance-gated (built-in default only).** The gate is `isDefault`, which `InitScriptPath()` reports as provenance — true only when `WORKTREE_INIT_SCRIPT` is unset/empty, never by string equality. An explicit `WORKTREE_INIT_SCRIPT="fab sync"` (`isDefault=false`) still hard-fails on exit 3 exactly like any other configured script — the user opted into it, so the failure is theirs to see. `DefaultNotApplicable` returns false whenever `isDefault` is false, regardless of exit code.
- **Both run sites route through the helper.** `RunWorktreeSetupWithObserver` (`crud.go:178–181`) and `wt init`'s own `cmd.Run()` failure path (`cmd/wt/init.go:128–131`) apply `DefaultNotApplicable` before the hard-fail path: on the skip they write `RenderDefaultSkipWarning()` to stderr and return `nil` (no `Worktree init complete.`). `wt init` reclaims the terminal foreground **first** (reclaim-before-write ordering preserved, `init.go:119–121`) so the skip-warning write cannot SIGTTOU. `wt create`'s two call sites (normal via `RunWorktreeSetupWithObserver`, `--reuse` via `RunWorktreeSetup`) pass `isDefault` from `InitScriptPath()` and need no other logic — the skip surfaces as a `nil` return, so `initFailed` is never set (no banner, no prompt, exit 0, stdout path line unaffected).
- **Old fab-kit degrades to today's hard-fail.** An installed fab-kit predating PR #471 exits 1 in a non-fab repo, which is not 3, so `DefaultNotApplicable` returns false and behavior is exactly as today (hard fail, exit 7, banner). No version detection, no fallback probe — the skip lights up only when fab-kit updates. Every non-zero exit other than 3 (any provenance) keeps `ExitInitFailed = 7` on every path.

### Interactive open-anyway prompt (`260626-n6ma`)

- On init-script non-zero exit, `wt create` offers to open the kept worktree anyway — but **only when interactive**, gated by `!nonInteractive && reclaimTTY` (`reclaimTTY` is `term.IsTerminal(os.Stdin.Fd())`, already computed earlier in the init block; `create.go:388`). This is the same TTY / `--non-interactive` discipline the rest of `create` uses (Constitution VI).
- **Interactive**: after the banner, prompt `wt.ConfirmYesNo("Continue and open the worktree anyway?")` (`ConfirmYesNo` in `src/internal/worktree/menu.go`).
  - **Yes** → fall through into the EXISTING Open phase (no new open codepath) so the user can open the kept worktree. The function still exits 7 at the end via the `initFailed` guard.
  - **No** → set `worktreeOpen = "skip"` so the Open separator + app menu are suppressed; the banner's `Go:` line already shows how to reach the kept worktree. Exit 7 at the end.
- **Non-interactive / piped / CI**: the prior behavior is preserved exactly — banner (now including the `Go:` line) + inline `os.Exit(wt.ExitInitFailed)`, **NO prompt** (`create.go:398–401`).

### `--reuse` is exempt from the new failure contract

- The `--reuse` code path in `cmd/wt/create.go` keeps its prior "warn-but-continue on init failure" semantics.
- The reuse path does NOT adopt `ExitInitFailed`. A reused worktree is presumed functional pre-existing; init there is a refresh, not a gate.

### Init-failure banner

- `internal/worktree/errors.go` exposes `PrintInitFailureBanner(wtPath, name string, err error)`.
- Banner contents, in order: status line (with `*exec.ExitError` numeric code when available, generic phrasing otherwise) + reminder that init output streamed above; kept-worktree marker with absolute `wtPath`; **go hint `wt go '<name>'`** (`260626-n6ma`); retry hint `cd '<wtPath>' && wt init` (using `&&` so it parses in bash/zsh/fish); remove hint `wt delete '<name>'` (using the worktree name, not the absolute path, so it composes with `wt delete`'s name resolution). Path and name are all single-quoted via `shellQuoteSingle` so paths with spaces / shell metacharacters stay copy-paste-safe.
- The **`Go:` hint** (`260626-n6ma`) points the user at the selection-free navigation command (`wt go`) for the kept worktree. It is grouped with the other action hints, placed **after** the `Worktree:` line and **before** `Retry:` (banner order: status; `Worktree:`; `Go:`; `Retry:`; `Remove:`). It lives in the shared banner helper, so EVERY caller path gains the same discoverability — interactive Yes, interactive No, AND non-interactive/CI.
- Labels are named constants in one `const (...)` block — `bannerLabelWorktree` / `bannerLabelGo` (`"Go:      "`) / `bannerLabelRetry` / `bannerLabelRemove`, column-aligned — so the canonical wording lives in one place (no duplication; `bannerLabelGo` was added alongside its siblings by `260626-n6ma`).
- Uses existing `ColorRed` / `ColorBold` / `ColorReset` helpers. Labels are confined to this one helper — no duplication elsewhere.

### SIGINT during init: Option B (handler swap + Setpgid)

- After `git worktree add` succeeds and **immediately before** the init step starts, `cmd/wt/create.go`:
  1. Calls `signal.Reset(syscall.SIGINT, syscall.SIGTERM)` to drop the rollback-based handler.
  2. Installs a new handler that signals the init `*exec.Cmd`'s process group.
- The init `*exec.Cmd` is constructed with `SysProcAttr.Setpgid = true` so the script's own children also receive the signal.
- After the child exits, control falls through to the unified post-init teardown/reclaim block (below), then the init-failure path: banner + (interactive) open-anyway prompt or (non-interactive) `os.Exit(ExitInitFailed)`. Either way the worktree is kept and the process exits 7 (the end-of-function `initFailed` guard handles the interactive paths). The init-child signal handler is torn down (`signal.Stop`/`close(initSigCh)`) before any of this, on every init outcome (`260626-n6ma`).
- Before worktree creation succeeds, the original SIGINT semantics apply: rollback executes and the process exits 130.
- The reinstallation window (between `git worktree add` returning and the new handler being installed) MUST stay tight — no I/O, prompts, or non-trivial work. If a panic occurs here, the original `defer rb.Execute()` still fires (losing the worktree is the correct fallback when the handler is in an inconsistent state).
- `RunWorktreeSetupWithObserver` is the variant exposed for SIGINT integration; `RunWorktreeSetup` is the thin wrapper used elsewhere.

### Terminal foreground is reclaimed after the init child (job-control bookkeeping)

> Lives **next to** the SIGINT Option B contract above and preserves all of its invariants. This is purely additive job-control bookkeeping around the same init region — it touches only the **terminal's** foreground process group, never the init child's process group.

- `wt create` AND `wt init` capture the controlling terminal's foreground process group (`tcgetpgrp` on `os.Stdin.Fd()`) **immediately before** running the init child, and reclaim it (`tcsetpgrp` back to that captured pgrp — `wt`'s own group, since `wt` is the foreground at capture time) **after** the child returns, on ALL exit paths:
  - **`wt create`** (`cmd/wt/create.go`):
    - Capture is adjacent to the SIGINT-Option-B `signal.Reset` (`create.go:298–310`), gated on `term.IsTerminal(os.Stdin.Fd())`. If `tcgetpgrp` itself errors on a TTY, the bookkeeping is disabled (no reclaim to a bogus pgrp) rather than guessed.
    - **Unified post-init reclaim/teardown** (`260626-n6ma`): as of the open-anyway change, the previously-duplicated teardown + reclaim now run **ONCE, unconditionally**, in a single post-init block immediately after `RunWorktreeSetupWithObserver` returns — `signal.Stop(initSigCh)` + `close(initSigCh)` (`create.go:355–356`) then `reclaimTerminalForeground(ttyFd, wtPgid)` (gated on `reclaimTTY`, `create.go:363–365`) — **before** the init-failure banner, the open-anyway prompt, AND the Open phase separator + menu render. This single reclaim is the load-bearing one for every downstream TTY write: it covers the success-path Open menu, the init-failure banner (itself a TTY write), the open-anyway prompt, and the Open menu reached via the open-anyway *Yes* fall-through. It replaced the prior two separate reclaim sites (a success-path reclaim before Open and an init-failure-path reclaim before the banner), removing a double `close(initSigCh)` and a duplicate reclaim.
    - **Panic / non-`os.Exit` early-return**: a best-effort `defer reclaimTerminalForeground(...)` installed immediately after capture (`create.go:322–324`). The unconditional post-init reclaim above is the load-bearing one; because every init-failure path either `os.Exit`s (non-interactive) or falls through to Open, this defer only actually fires on a panic or a non-`os.Exit` return. (The pre-init SIGINT exit-130 path runs before this defer is installed and before any foreground was captured, so it has nothing to reclaim.)
  - **`wt init`** (`cmd/wt/init.go::runInitScript`): same capture before `cmd.Run()` (`init.go:96–113`), explicit reclaim before the failure trailer + `os.Exit(ExitInitFailed)` (`init.go:121–124`) and before the `Worktree init complete.` trailer on success (`init.go:129–132`), plus the same best-effort `defer` safety net. A standalone interactive `wt init` (no Open phase, no open-anyway prompt) would otherwise strand the user's shell at a suspended prompt after returning. (`wt init` was unchanged by `260626-n6ma`.)
- The reclaim wraps the `tcsetpgrp` ioctl in `signal.Ignore(syscall.SIGTTOU)` with `defer signal.Reset(syscall.SIGTTOU)` (`tty_unix.go`). Without this, a `tcsetpgrp` issued from a possibly-background process is itself a foreground-mutating operation the kernel answers with SIGTTOU — the reclaim that is supposed to un-stick `wt` would stick it instead. `signal.Reset` is the correct restore because `wt` installs no other SIGTTOU handler.
- All bookkeeping is **gated on `term.IsTerminal(os.Stdin.Fd())`** — when stdin is not a TTY (`--non-interactive`, piped, CI), capture and reclaim are complete no-ops: no syscall, no error, no warning. This mirrors the existing `isInteractiveTTY()` gate in `menu.go` and Constitution VI.
- Reclaim errors are intentionally ignored (`_ = unix.IoctlSetPointerInt(...)`): the bookkeeping is best-effort cleanup that MUST never block `rb.Execute()` rollback or alter exit codes.
- **Preserved SIGINT Option B invariants**: `SysProcAttr{Setpgid: true}` on the init child is unchanged (the reclaim never touches the child's pgrp); the tight reinstall window (worktree-add → `signal.Reset`) is NOT widened (`tcgetpgrp` is a single cheap syscall with no I/O or prompt, placed adjacent to `signal.Reset`); `defer rb.Execute()` remains the panic fallback and the foreground-restore defer is independent of it. **These invariants hold on the open-anyway fall-through path too** (`260626-n6ma`): the SIGINT teardown (`signal.Stop`/`close(initSigCh)`) and the foreground reclaim both run in the unified post-init block — once, before the banner, the open-anyway prompt, and the Open menu — so the *Yes* fall-through reaches the Open menu with the init handler torn down and foreground already reclaimed (no SIGTTOU), and `rb.Disarm()` (not `rb.Execute()`) still keeps the worktree on the new path. The end-of-function `if initFailed { os.Exit(ExitInitFailed) }` is the single exit point for the open-anyway path.
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

Tests assert presence of the worktree path, the `wt go '<name>'` go hint (`260626-n6ma`, including the single-quote-escaping case for a name with an embedded quote), the `wt init` retry hint, and the `wt delete <name>` remove hint — NOT byte-equality of the banner template. Wording can evolve without test churn; the contract is the information surface, not the prose.

### Open-anyway via flag + Open-phase fall-through, not a new open codepath (`260626-n6ma`)

The interactive *Yes* path does NOT build a second open codepath inside the init-failure branch — it sets `worktreeOpen` appropriately and falls through to the EXISTING Open phase. *Why*: the load-bearing pre-Open terminal-foreground reclaim already guards the Open menu against SIGTTOU, so reusing the Open phase inherits that guarantee for free and avoids duplicating the open logic. The exit code is driven by an `initFailed` bool checked once at the function's end (before the stdout path line), so a successful open never downgrades to 0. *Rejected*: a separate inline open call in the failure block (would duplicate the Open phase and bypass its reclaim); a new exit code 8 or exit 0 on successful open (would erase the operator-depended-on `ExitInitFailed = 7` signal — Constitution III). (Source: change `260626-n6ma`, plan Design Decisions 1/3.)

## Cross-references

- Spec doc: `docs/specs/init-protocol.md` — wire contract + "Script failure semantics" + "SIGINT during init" sections (rewritten by this change).
- Source: `src/internal/worktree/init.go` (resolver, `InitNotFound`, `RenderWarning`; the `260705-irnt` run-time skip helpers `DefaultNotApplicable` / `RenderDefaultSkipWarning` + the documented `exitNotManaged = 3` const), `src/internal/worktree/context.go` (`InitScriptPath() (script string, isDefault bool)` — provenance report, `260705-irnt`), `src/internal/worktree/errors.go` (`ExitInitFailed`, `PrintInitFailureBanner` + the `bannerLabelGo` `Go:` hint, `260626-n6ma`), `src/internal/worktree/crud.go` (`RunWorktreeSetup`, `RunWorktreeSetupWithObserver` — both gained the `isDefault` param + post-run `DefaultNotApplicable` classification, `260705-irnt`), `cmd/wt/create.go` (rollback disarm + handler swap + unified post-init foreground reclaim/teardown + `initFailed`-flag-driven open-anyway fall-through and end-of-function exit, `260626-n6ma`; both init call sites pass `isDefault`, `260705-irnt`), `cmd/wt/init.go` (resolver consumer + foreground capture/reclaim around `runInitScript`'s `cmd.Run()`; `DefaultNotApplicable` skip before `os.Exit(ExitInitFailed)`, reclaim-before-write preserved, `260705-irnt`), `src/internal/worktree/menu.go` (`ConfirmYesNo`, the open-anyway prompt), `src/internal/worktree/apps.go` (`shellQuoteSingle`, `OpenInApp` test seam), `src/cmd/wt/tty_unix.go` / `src/cmd/wt/tty_windows.go` (`terminalForeground` / `reclaimTerminalForeground` build-tag split).
- Tests: `src/cmd/wt/tty_pty_test.go` — `TestIntegration_ReclaimForegroundAfterInit_NotStopped` (Unix-only, `//go:build !windows`): allocates a real PTY via an in-tree `openpty` helper on `x/sys/unix` (`/dev/ptmx` + `TIOCSPTLCK`/`TIOCGPTN` + `/dev/pts/N`, no `creack/pty`), runs `wt create` under it via a session-leader launcher with a foreground-stranding init script, and asserts the `wt` process is not left `WIFSTOPPED`; self-skips when a PTY / process groups can't be set up. Verified non-vacuous (fails `WIFSTOPPED` without the reclaim).
- Constitution: Principle III (Typed Exit Codes) — `ExitInitFailed` is the canonical example for "worktree exists, init didn't complete". Principle I (Single-Binary CLI, slim deps) — drove the `x/sys/unix` ioctls (no new module) and the in-tree `openpty` helper (no `creack/pty`). Principle VI (Interactive by Default, Scriptable on Demand) — the reclaim restores the broken interactive Open path; the `term.IsTerminal` gate keeps the scriptable / non-interactive path a no-op.
- Sibling memory: `wt-cli/create-output-phases.md` — the Open-phase separator + menu render now sits behind the unconditional pre-Open foreground reclaim. `wt-cli/create-branch-semantics.md` — the `wt create` branch-selection contract (new-branch positional / `--checkout` / `--base`); the init step, kept-worktree banner, open-anyway prompt, SIGINT-Option-B, and foreground-reclaim invariants documented here run identically on the `--checkout` and positional-new paths, and it is the source of the `--reuse` init-failure exemption cited here.
