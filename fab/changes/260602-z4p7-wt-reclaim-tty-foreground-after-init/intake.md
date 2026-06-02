# Intake: wt create — reclaim terminal foreground after init phase

**Change**: 260602-z4p7-wt-reclaim-tty-foreground-after-init
**Created**: 2026-06-02
**Status**: Draft

## Origin

<!-- How was this change initiated? -->

Initiated from a live debugging session. The user pointed at a tmux pane (`%0`) running an
interactive `wt create` in a *different* repo (`~/code/wvrdz/loom`) and asked why the final
"Open" step was breaking. The pane showed:

```
── Init (fab sync) ─────────────────────
Running worktree init...
...
Worktree init complete.
── Open ────────────────────────────────
zsh: suspended (tty output)
```

Diagnosis (interaction mode: conversational, agent-driven from a process snapshot):

A `ps` snapshot of the pane's process tree showed the smoking gun:

```
PID    PGID    STAT   TPGID   COMMAND
71845  71845   Tl     67836   wt create     ← stopped (T), owns no terminal
72396  72396   T      67836   -zsh          ← stopped login shell, sibling of wt
67836  67836   Ss+    67836   zsh           ← pane shell = terminal foreground group
```

The terminal's foreground process group (`TPGID`) is `67836` — the **pane's interactive
shell** — not `wt`'s process group (`71845`). When `wt` reached the Open phase and `ShowMenu`
→ `term.MakeRaw(stdin)` wrote the menu to the TTY, the kernel saw a **background process
writing to its controlling terminal** and delivered **SIGTTOU**, which stopped the process.
The menu never rendered. An orphaned login `-zsh` (PID 72396, spawned ~1s after `wt` — i.e.
during the init phase, in its own process group) is the residue of whatever grabbed the
terminal foreground and exited without restoring it.

**Culprit traced (clarify session 2026-06-02).** The processes were still alive and stopped at
clarify time, so the trigger was traced end-to-end. `wt create` → `fab sync` →
`fab/sync/1-prerequisites.sh` (`exec doctor.sh --porcelain`) → **`doctor.sh:111`**:

```bash
zsh -i -c 'typeset -f _direnv_hook' &>/dev/null   # direnv-hook check
```

The `zsh -i` (interactive shell) runs with job control on a shared controlling terminal
(`/dev/pts/30`) and `tcsetpgrp`'s itself into the foreground at startup — standard interactive-
shell behavior. The `&>/dev/null` redirects its stdio but NOT its job-control side effects. On
exit it does not restore foreground to `wt`'s group (the whole init subtree is in a separate
pgrp from `wt` via `Setpgid: true`), so foreground is stranded; `wt` is left background → SIGTTOU
on the Open-phase write. (The `zsh -i` itself is also found stopped in `do_signal_stop`.)

This makes the bug **two-sided**: `doctor.sh`'s `zsh -i` is hostile to a shared TTY (it lives in
a *different* repo — `loom` — that `wt` cannot police), AND `wt` does no foreground cleanup after
the init child. The trace *validates* the chosen fix: the `zsh -i` ran correctly; only the
foreground *cleanup* is missing — exactly what reclaim-after restores (see assumption #4).

> **Raw user input / decision:** "Check the pane %0 in tmux... Can you find out why the last
> step of 'wt open' is breaking." → root-caused to SIGTTOU during the init phase, then the
> user chose "Open a fix: change" over "Confirm with fg first" / "Just discuss design".

## Why

<!-- Motivation: problem, consequence, reasoning. -->

**Problem.** `wt create` is acting as a job-control parent — it launches the init child
(`fab sync`) in its **own process group** via `SysProcAttr{Setpgid: true}`
(`src/internal/worktree/init_unix.go:15`) while sharing `wt`'s controlling terminal
(`cmd.Stdin = os.Stdin`, `crud.go:162`) — but it never does the corresponding terminal
**foreground-ownership bookkeeping**. There is no `tcsetpgrp` anywhere in the source tree
(confirmed by grep: the only process-group manipulation is the lone `Setpgid: true`). So if
the init script, or any descendant it spawns, grabs the terminal's foreground group and exits
without restoring it, `wt` is silently orphaned into the background. The very next TTY write —
the Open-phase menu render at `create.go:319+` — trips SIGTTOU and suspends the process.

**Consequence if unfixed.** `wt create` becomes unreliable for *any* init script that touches
terminal job control (spawns an interactive shell, runs a TUI, calls `tcsetpgrp`, etc.). The
user is dropped at a `zsh: suspended` prompt mid-create, must know to type `fg`, and the
worktree is left in a half-finished state (created + initialized, but never opened). This is a
silent, environment-dependent footgun that contradicts Constitution VI (Interactive by Default)
— the interactive Open phase is exactly where it breaks.

**Why this approach (defensive reclaim) over alternatives.** The correct, robust fix is to make
`wt` reclaim terminal foreground for its own process group **after** the init child returns, on
every exit path, wrapping the reclaim in `signal.Ignore(SIGTTOU)` so the reclaim call itself is
not stopped. This is correct **regardless of which descendant grabbed foreground** — `wt`
doesn't need to know who did it, only that it owns the terminal again before its next TTY write.
The heavier "full job-control parent" approach (proactively `tcsetpgrp` the child into the
foreground before running it, then reclaim) is rejected as unnecessary risk: the init child
already runs correctly sharing the TTY today; only the *cleanup* is missing.

## What Changes

<!-- Specific behavior. -->

### Change area 1: Save/restore terminal foreground around the init phase (`wt create`)

In `src/cmd/wt/create.go`, wrap the init block (the `if runInit { ... }` region,
`create.go:271–309`) with terminal-foreground bookkeeping:

1. **Before** running the init child (just before / alongside the existing `signal.Reset` +
   init-handler install at `create.go:282`), **capture** the current terminal foreground process
   group: `fg := tcgetpgrp(ttyFd)`. This is `wt`'s own pgrp at that point. Only do this when the
   controlling terminal fd is a real TTY (see Change area 3).
2. **After** the init child returns — on **all three** exit paths — restore foreground to `wt`'s
   own process group via `tcsetpgrp(ttyFd, wtPgid)`:
   - **Success path**: after `signal.Stop(initSigCh); close(initSigCh)` (`create.go:307–308`),
     before the Open phase.
   - **Non-zero-exit / banner path** (`create.go:301–306`): before `PrintInitFailureBanner` /
     `os.Exit(ExitInitFailed)` — so a failed init also leaves the terminal sane (the banner is a
     TTY write too, and would itself SIGTTOU if foreground was lost).
   - **SIGINT-abort path**: if the init handler fires and the process is heading to exit 130, the
     terminal should be restored too (best-effort; exiting anyway).
   The cleanest implementation is a single deferred restore closure installed right after capture,
   so it fires on every exit of the init block (`defer restoreForeground()`), with the explicit
   pre-Open restore being the load-bearing one.
3. **Wrap the restore `tcsetpgrp` in `signal.Ignore(syscall.SIGTTOU)`** (and SIGTTIN) for the
   duration of the call, then restore prior disposition. Rationale: `tcsetpgrp` from a background
   process is itself a foreground-mutating operation that the kernel answers with SIGTTOU — if we
   don't ignore it, the reclaim that is supposed to un-stick us would stick us instead.

Reclaim target = `wt`'s own process group = `syscall.Getpgrp()` (equivalently the captured `fg`
value from step 1, since `wt` was foreground before init). Use `Getpgrp()` as the source of
truth for "give the terminal back to me".

### Change area 2: Unix/Windows split for the tcsetpgrp helpers

Mirror the existing `init_unix.go` / `init_windows.go` and `signal_unix.go` / `signal_windows.go`
build-tag split. Add a small helper pair, e.g. in `src/cmd/wt/` (or `internal/worktree/`,
matching where the tty fd is owned):

```go
// tty_unix.go  (//go:build !windows)
func reclaimTerminalForeground(ttyFd int, pgrp int) {
    // Ignore SIGTTOU so this tcsetpgrp from a possibly-background process
    // is not itself stopped, then restore prior disposition.
    signal.Ignore(syscall.SIGTTOU)
    defer signal.Reset(syscall.SIGTTOU) // or restore prior handler set
    _ = unix.IoctlSetPointerInt(ttyFd, unix.TIOCSPGRP, pgrp) // tcsetpgrp
}
func terminalForeground(ttyFd int) (int, error) { /* tcgetpgrp via TIOCGPGRP */ }
```

```go
// tty_windows.go  (//go:build windows)
func reclaimTerminalForeground(ttyFd int, pgrp int) {}          // no-op
func terminalForeground(ttyFd int) (int, error) { return 0, nil } // no-op
```

The Windows stubs are no-ops — Windows has no `tcsetpgrp` / process-group terminal model, and
the existing init/signal code is already Unix-only by the same pattern. Issue the ioctls via
`golang.org/x/sys/unix` — `unix.IoctlGetInt(fd, unix.TIOCGPGRP)` for `tcgetpgrp` and
`unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgrp)` for `tcsetpgrp`. This adds **no new runtime
dependency**: `x/sys/unix` is already in the module graph transitively via `golang.org/x/term`
(used by `ShowMenu`), so the single-binary / minimal-dep posture (Constitution I) is preserved.
(Decided in clarify 2026-06-02 — confirm `x/sys/unix` is in `go.sum` at plan time.)

### Change area 3: Guard on TTY presence; degrade gracefully when piped / non-interactive

The foreground bookkeeping MUST be a no-op when there is no controlling terminal:

- Guard the capture+restore on `term.IsTerminal(ttyFd)` being true. When stdin/stdout is a pipe
  (`--non-interactive`, redirected, CI), `tcgetpgrp`/`tcsetpgrp` are meaningless and MUST be
  skipped — no error, no warning. This matches the existing `isInteractiveTTY()` gate in
  `menu.go:629` (which already requires both stdin and stdout to be terminals).
- Which fd is "the controlling terminal"? `wt` wires `cmd.Stdin = os.Stdin` for the init child,
  so the foreground contention is over **stdin's** terminal. Use `os.Stdin.Fd()` as `ttyFd`,
  consistent with `ShowMenu`'s `term.MakeRaw(int(os.Stdin.Fd()))`. (Whether to instead open
  `/dev/tty` directly is a Tentative decision below; stdin is the default to match the menu.)

### Change area 4: Preserve the SIGINT-during-init Option B contract

The new bookkeeping sits **inside** the same region as the SIGINT-during-init handler swap
(`init-failure-contract.md` § "SIGINT during init"). It MUST NOT violate those invariants:

- Keep `SysProcAttr{Setpgid: true}` on the init child — the group-signal handler
  (`signalInitProcessGroup`, `signal_unix.go`) still targets `-cmd.Process.Pid`. This change does
  not touch the child's process group; it only restores the **terminal's** foreground afterward.
- The "tight reinstall window" rule (no I/O, prompts, or non-trivial work between
  `git worktree add` returning and `signal.Reset`) is preserved: `tcgetpgrp` is a single cheap
  syscall with no I/O and no prompt, and it is placed adjacent to the existing `signal.Reset`
  without inserting user interaction.
- `defer rb.Execute()` remains the panic fallback; the foreground-restore defer is independent
  and best-effort (its failure never blocks rollback).

### Change area 5 (in scope, smaller): apply the same reclaim to `wt init`

`wt init` (`src/cmd/wt/init.go`, `runInitScript`) runs the *same* init child via the same runner
and is susceptible to the identical foreground loss when run interactively. Apply the same
capture/restore guard there so a standalone `wt init` is not left suspended either. (`wt init`
has no Open phase, but losing foreground would still strand the user's shell at a suspended
prompt after `wt init` returns.)

### Test: PTY integration test asserting no suspension

Add an integration test (Unix-only, `//go:build !windows`) that:

1. Allocates a real PTY using an **in-tree `openpty` helper built on `golang.org/x/sys/unix`**
   (`unix.Openpt` / `unix.Grantpt` / `unix.Unlockpt` / `unix.PtsName`) — **no
   `github.com/creack/pty` dependency** (decided in clarify 2026-06-02).
2. Runs `wt create` under that PTY with `WORKTREE_INIT_SCRIPT` pointing at a script that
   **grabs the terminal foreground and exits without restoring it** (e.g. spawns a child in its
   own pgrp, `tcsetpgrp`s the tty to it, lets it exit), reproducing the bug. The minimal
   reproducer is exactly `doctor.sh:111`'s pattern: an init script that runs
   `zsh -i -c true &>/dev/null` (or any interactive shell) on the shared TTY.
3. Asserts the `wt` process is **not left stopped** — i.e. it runs to completion / its exit
   status is not `WIFSTOPPED`, and the menu/open phase output appears.

**CI feasibility flag:** PTY + job-control tests are fiddly and can be flaky on CI runners
(no controlling terminal, restricted `setpgid`/`tcsetpgrp`, sandboxing). The test MUST self-skip
(`t.Skip`) when it cannot allocate a PTY or set up process groups, rather than fail. The unit-
level guard (helper no-ops when fd is not a TTY) is the always-runnable safety net; the PTY test
is the end-to-end proof that runs locally / where supported. This mirrors `code-review.md`'s rule
that window/tty-touching tests must not leak host side effects.

## Affected Memory

<!-- Memory files created/modified/removed. -->

- `wt-cli/init-failure-contract.md`: (modify) Add a requirement documenting that `wt create`
  (and `wt init`) save/restore terminal foreground around the init child — capture `tcgetpgrp`
  before, `tcsetpgrp` back to `wt`'s pgrp after on all exit paths, reclaim wrapped in
  `signal.Ignore(SIGTTOU)`, guarded on TTY presence, no-op on Windows. Note that this lives next
  to the SIGINT Option B contract and preserves `Setpgid: true`.
- `wt-cli/create-output-phases.md`: (modify) Note that the Open-phase separator + menu render is
  preceded by an unconditional foreground reclaim, so the Open phase can never SIGTTOU on a
  shared-TTY init child. (Light touch — the separator output contract itself is unchanged.)

## Impact

<!-- Affected code areas, APIs, dependencies. -->

- **Code**:
  - `src/cmd/wt/create.go` — capture/restore foreground around the `runInit` block; restore
    before Open phase, before init-failure banner, and on SIGINT-abort.
  - `src/cmd/wt/init.go` — same capture/restore around `runInitScript` for standalone `wt init`.
  - New `src/cmd/wt/tty_unix.go` + `src/cmd/wt/tty_windows.go` (or under `internal/worktree/`) —
    `terminalForeground` / `reclaimTerminalForeground` build-tag split.
  - New `src/cmd/wt/*_pty_test.go` (Unix-only) — PTY integration test (self-skipping).
- **APIs**: no public CLI surface change — same flags, same exit codes, same stdout discipline
  (stdout stays the worktree-path line only). Purely a job-control correctness fix.
- **Dependencies**: **no new dependency.** Both the ioctls and the test's PTY allocation use
  `golang.org/x/sys/unix`, already present transitively via `golang.org/x/term`. No
  `github.com/creack/pty` (in-tree `openpty` helper instead). Decided in clarify 2026-06-02;
  confirm `x/sys/unix` resolves from `go.sum` at plan time.
- **Constitution**: I (single-binary, minimal deps — drives the raw-syscall-vs-x/sys decision),
  VI (interactive by default — this fix restores the broken interactive Open path), V (internal
  package boundary — keep the syscall helper in a thin, testable seam).
- **Cross-repo**: no impact on `hop` / launcher-contract — stdout discipline and exit codes are
  unchanged.

## Open Questions

<!-- Clarifying questions. -->

All intake-level questions were resolved in the clarify session 2026-06-02:

- ~~ioctl mechanism~~ → **`golang.org/x/sys/unix`** (`unix.IoctlGetInt`/`IoctlSetPointerInt` with
  `TIOCGPGRP`/`TIOCSPGRP`). Already in the module graph transitively via `golang.org/x/term`
  (which `ShowMenu` uses), so **no new runtime dependency** — single static binary preserved
  (Constitution I). Verify it is in `go.sum` at plan time. <!-- clarified: x/sys/unix, already-present dep -->
- ~~foreground fd~~ → **`os.Stdin.Fd()`** — the same terminal `ShowMenu`'s
  `term.MakeRaw(int(os.Stdin.Fd()))` and the init child (`cmd.Stdin = os.Stdin`) key off, so
  reclaim and raw-mode are guaranteed to target the same TTY. No-ops via the `term.IsTerminal`
  guard when stdin is redirected. `/dev/tty` (robust to stdin redirection) noted as future
  hardening, not in scope. <!-- clarified: os.Stdin.Fd(), /dev/tty deferred -->
- ~~proactive handoff vs reclaim-after~~ → **reclaim-after only** — confirmed by the culprit
  trace: the init child's `zsh -i` ran correctly sharing the TTY; only cleanup was missing.
- ~~PTY test dependency~~ → **in-tree `openpty` helper via `x/sys/unix`** (`unix.Openpt` /
  `Grantpt` / `Unlockpt`); **no `github.com/creack/pty`**. Test is Unix-only and self-skips when a
  PTY / process groups can't be set up.

## Clarifications

### Session 2026-06-02 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 1 | Confirmed | — |
| 3 | Confirmed | — |
| 4 | Confirmed | — |
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 9 | Confirmed | — |
| 11 | Confirmed | — |

### Session 2026-06-02 (tentative resolution)

| # | Action | Detail |
|---|--------|--------|
| 2 | Traced | User asked to trace; culprit proven = `doctor.sh:111` `zsh -i` direnv-hook check (reached via `fab sync` → `1-prerequisites.sh`). Both `wt` and the `zsh -i` found still stopped on `/dev/pts/30`. |
| 7 | Decided (delegated) | `golang.org/x/sys/unix` ioctls — already a transitive dep via `x/term`, so no new dependency. |
| 8 | Confirmed | `os.Stdin.Fd()` — matches the menu's `term.MakeRaw` fd; `/dev/tty` deferred as future hardening. |
| 10 | Decided (delegated) | Keep self-skipping PTY test; PTY via in-tree `openpty` on `x/sys/unix`, no `creack/pty`. |

## Assumptions

<!-- STATE TRANSFER table. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Root cause is SIGTTOU from a background `wt` writing to its controlling TTY in the Open phase, after the init phase lost terminal foreground ownership. | Clarified — user confirmed. Process snapshot is unambiguous (`wt` `Tl`, `TPGID` = pane shell, orphaned `-zsh`), and the culprit was traced end-to-end (see #2): kernel SIGTTOU-on-background-TTY-write explains `suspended (tty output)`. | S:95 R:60 A:80 D:78 |
| 2 | Certain | The foreground-grabber is **`doctor.sh:111`** (`zsh -i -c '...' &>/dev/null`, the direnv-hook check) reached via `fab sync` → `1-prerequisites.sh` → `doctor.sh`. The interactive `zsh` `tcsetpgrp`s itself to foreground and exits without restoring it. | Clarified — user asked to trace; traced from still-stopped processes at clarify time (both `wt` and the `zsh -i` found in `do_signal_stop` on `/dev/pts/30`). Proven, not inferred. Validates #4 (the `zsh -i` ran fine; only cleanup was missing). `doctor.sh` lives in another repo (`loom`) that `wt` cannot police → reclaim-in-wt is the right fix. | S:95 R:75 A:90 D:85 |
| 3 | Certain | Fix = defensive reclaim: after the init child returns (all exit paths), `tcsetpgrp(tty, wtPgid)` wrapped in `signal.Ignore(SIGTTOU)`, guarded on TTY presence, no-op on Windows. | Clarified — user confirmed. Standard job-control parent bookkeeping; `signal.Ignore(SIGTTOU)` around the reclaim is required so the reclaim itself isn't stopped. Correct independent of the (now-proven) culprit. | S:95 R:65 A:82 D:75 |
| 4 | Certain | Reject the "proactive handoff" variant (tcsetpgrp child→foreground before run, then reclaim). Reclaim-after is sufficient. | Clarified — user confirmed; reinforced by the #2 trace: the init child's `zsh -i` ran correctly sharing the TTY, so only cleanup is missing. Proactive handoff adds a failure surface for no benefit. | S:95 R:70 A:80 D:72 |
| 5 | Certain | Preserve the SIGINT-during-init Option B contract: keep `Setpgid: true`, keep the tight reinstall window, keep `defer rb.Execute()` fallback. The new bookkeeping is additive and best-effort. | Clarified — user confirmed. `init-failure-contract.md` pins these invariants; `tcgetpgrp` is a single cheap syscall (no I/O, no prompt) so it does not widen the reinstall window. Reclaim only touches the *terminal's* foreground, never the child's pgrp. | S:95 R:55 A:80 D:72 |
| 6 | Certain | Apply the same capture/restore to standalone `wt init`, not just `wt create`. | Clarified — user confirmed. `wt init` runs the identical init child via the same runner and is susceptible to the identical suspension when run interactively. | S:95 R:70 A:78 D:72 |
| 7 | Certain | Use `golang.org/x/sys/unix` ioctls (`IoctlGetInt`+`TIOCGPGRP` / `IoctlSetPointerInt`+`TIOCSPGRP`). NOT a raw `syscall` wrapper. | Clarified — user delegated ("your call"); chose `x/sys/unix` because it is already in the module graph transitively via `golang.org/x/term` (used by `ShowMenu`), so **no new runtime dependency** — Constitution I single-binary posture preserved. Confirm in `go.sum` at plan time. | S:95 R:78 A:75 D:70 |
| 8 | Certain | Foreground fd = `os.Stdin.Fd()` (matches the menu's `term.MakeRaw`), NOT an explicitly-opened `/dev/tty`. | Clarified — user confirmed (recommended option). Stdin is the terminal the menu and init child share, so reclaim and raw-mode target the same TTY. `/dev/tty` (robust to stdin redirection) noted as future hardening, out of scope. The `term.IsTerminal` guard skips the redirected case. | S:95 R:75 A:65 D:75 |
| 9 | Certain | Guard all foreground bookkeeping on `term.IsTerminal(fd)`; no-op (no error/warn) when piped / `--non-interactive` / CI. | Clarified — user confirmed. `tcgetpgrp`/`tcsetpgrp` are meaningless without a controlling terminal. Mirrors the existing `isInteractiveTTY()` gate (`menu.go:629`) and Constitution VI (scriptable on demand). | S:95 R:75 A:82 D:80 |
| 10 | Certain | Add a Unix-only PTY integration test (self-skipping) reproducing foreground loss via an interactive-shell init script and asserting `wt` is not left stopped. PTY via in-tree `openpty` helper on `x/sys/unix` — NO `creack/pty`. | Clarified — user delegated ("your call"); keep the test (Constitution IV "test what the user sees" — bug is invisible to non-PTY tests), self-skip when PTY/process-groups unavailable to keep CI green, and avoid a test-only dep by using `x/sys/unix` (already present). | S:95 R:70 A:60 D:65 |
| 11 | Certain | No public CLI surface change: same flags, exit codes, and stdout discipline (stdout stays the worktree-path line only). Pure job-control correctness fix. | Clarified — user confirmed. The change touches only terminal-foreground bookkeeping around init; it does not alter output streams, exit codes, or flags. Preserves the launcher-contract / `create-output-phases.md` stdout=machine-result invariant. | S:95 R:65 A:85 D:80 |

11 assumptions (11 certain, 0 confident, 0 tentative, 0 unresolved).
