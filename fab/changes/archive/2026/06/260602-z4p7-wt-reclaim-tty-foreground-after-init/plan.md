# Plan: wt create — reclaim terminal foreground after init phase

**Change**: 260602-z4p7-wt-reclaim-tty-foreground-after-init
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

> Job-control correctness fix: `wt` acts as a job-control parent for the init
> child (`Setpgid: true`) while sharing its controlling terminal, but never
> reclaims terminal foreground after the child returns. If the init script (or
> any descendant) grabs the terminal foreground and exits without restoring it,
> `wt` is left in the background and the next TTY write (Open-phase menu render)
> trips SIGTTOU and suspends the process. The fix is a defensive
> capture-before / reclaim-after around the init child on every exit path,
> guarded on TTY presence, wrapped in `signal.Ignore(SIGTTOU)`, no-op on Windows.

### TTY Foreground: Reclaim after init in `wt create`

#### R1: Capture terminal foreground before the init child runs
`wt create` SHALL capture the terminal's foreground process group immediately
before reinstalling the init-phase signal handler, only when the controlling
terminal fd (`os.Stdin.Fd()`) is a real TTY. The capture SHALL be a single cheap
syscall (`tcgetpgrp` via `TIOCGPGRP`) so it does not widen the tight
signal-reinstall window.

- **GIVEN** `wt create` is running interactively with a TTY on stdin and init is enabled
- **WHEN** control reaches the init-phase signal-handler swap (`create.go:282`)
- **THEN** the current terminal foreground pgrp is captured via `terminalForeground(ttyFd)` before any init child is started
- **AND** no user prompt or I/O is inserted between `git worktree add` returning and the existing `signal.Reset`

#### R2: Reclaim terminal foreground on every init exit path in `wt create`
After the init child returns, `wt create` SHALL reclaim terminal foreground for
its own process group (`unix.Getpgrp()`) via `tcsetpgrp` (`TIOCSPGRP`) on ALL
exit paths of the init block: the success path before the Open phase, the
init-failure/banner path before `PrintInitFailureBanner` + `os.Exit(ExitInitFailed)`,
and the SIGINT-abort path. The reclaim SHALL be guarded on the same TTY check as
capture (no-op when not a TTY).

- **GIVEN** the init child has returned (success or non-zero exit) and stdin is a TTY
- **WHEN** `wt create` proceeds past the init block
- **THEN** `reclaimTerminalForeground(ttyFd, unix.Getpgrp())` runs before the Open-phase separator/menu render so the Open phase can never SIGTTOU
- **AND** on the init-failure path the reclaim runs before `PrintInitFailureBanner` so the banner write itself cannot SIGTTOU
- **AND** a deferred restore closure installed immediately after capture fires on every exit of the init block (including SIGINT-abort heading to exit 130), with the explicit pre-Open reclaim being the load-bearing one
- **AND** when stdin is not a TTY (`--non-interactive`, piped, CI), neither capture nor reclaim performs any syscall and emits no error/warning

#### R3: Reclaim does not stop on its own SIGTTOU
The reclaim SHALL wrap the `tcsetpgrp` call in `signal.Ignore(syscall.SIGTTOU)`
for the duration of the call and restore prior signal disposition afterward, so
the foreground-mutating ioctl issued from a possibly-background process is not
itself stopped by the kernel.

- **GIVEN** `wt` may be in the background (foreground was stranded by the init child)
- **WHEN** `reclaimTerminalForeground` issues `tcsetpgrp`
- **THEN** SIGTTOU is ignored around the call and prior disposition restored after, so the reclaim succeeds instead of stopping `wt`

### TTY Foreground: Unix/Windows helper split

#### R4: `terminalForeground` / `reclaimTerminalForeground` helper pair with build-tag split
A new helper pair SHALL live in `src/cmd/wt/` behind a build-tag split mirroring
`signal_unix.go` / `signal_windows.go`:
`terminalForeground(ttyFd int) (int, error)` (tcgetpgrp via `unix.IoctlGetInt(fd, unix.TIOCGPGRP)`)
and `reclaimTerminalForeground(ttyFd int, pgrp int)` (tcsetpgrp via
`unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgrp)`), in a `//go:build !windows`
file (`tty_unix.go`). A `//go:build windows` file (`tty_windows.go`) SHALL
provide no-op stubs (`reclaimTerminalForeground` does nothing;
`terminalForeground` returns `(0, nil)`). The ioctls SHALL use
`golang.org/x/sys/unix` (no new module — already in `go.mod`/`go.sum` indirect).

- **GIVEN** the codebase already splits Unix/Windows for signal + init group helpers
- **WHEN** the tty helpers are added
- **THEN** they follow the same pattern, compile on both `!windows` and `windows`, and add no runtime dependency beyond promoting the already-present `x/sys` to a direct require

### TTY Foreground: Apply the same reclaim to `wt init`

#### R5: Standalone `wt init` captures and reclaims terminal foreground
`wt init` (`runInitScript` in `src/cmd/wt/init.go`) SHALL apply the same
capture-before / reclaim-after guard around its init child (`cmd.Run()`), so a
standalone interactive `wt init` is not left suspended after the child strands
foreground. Guarded on TTY presence; no-op when not a TTY.

- **GIVEN** `wt init` runs interactively with a TTY and the init child grabs foreground and exits without restoring it
- **WHEN** the init child returns (success or non-zero exit)
- **THEN** `wt init` reclaims terminal foreground for its own pgrp before returning / before the failure exit, so the user's shell is not stranded at a suspended prompt

### TTY Foreground: Preserve the SIGINT Option B contract

#### R6: SIGINT-during-init Option B invariants are preserved
The new bookkeeping SHALL NOT alter the init child's process group or the SIGINT
handler swap. `SysProcAttr{Setpgid: true}` on the init child, the tight
reinstall window, and `defer rb.Execute()` as the panic fallback SHALL all be
preserved. The reclaim only touches the TERMINAL's foreground, never the child's
pgrp; the foreground-restore defer is independent and best-effort (its failure
never blocks rollback).

- **GIVEN** the SIGINT Option B contract pinned in `init-failure-contract.md`
- **WHEN** the reclaim bookkeeping is added inside the same region as the handler swap
- **THEN** `Setpgid: true`, the tight reinstall window, and `defer rb.Execute()` are unchanged, and the existing SIGINT integration test still passes

### TTY Foreground: No CLI surface change

#### R7: No change to flags, exit codes, or stdout discipline
The change SHALL NOT alter the public CLI surface: same flags, same exit codes,
and stdout SHALL remain solely the worktree-path line. The
`create-output-phases.md` invariants (phase separators stderr-only) SHALL be
preserved.

- **GIVEN** the fix is a pure job-control correctness change
- **WHEN** `wt create` / `wt init` run in any mode
- **THEN** stdout still emits only the worktree-path line (create) / nothing machine-readable (init), exit codes are unchanged, and separators stay on stderr

### TTY Foreground: PTY integration test

#### R8: Unix-only self-skipping PTY test proves no suspension
A Unix-only (`//go:build !windows`) PTY integration test SHALL allocate a real
PTY via an in-tree `openpty` helper built on `x/sys/unix`
(`/dev/ptmx` + `TIOCSPTLCK`/`TIOCGPTN` + `/dev/pts/N`), run `wt create` under it
with an init script that grabs terminal foreground and exits without restoring,
and assert the `wt` process is NOT left stopped (exit status not `WIFSTOPPED`;
runs to completion). The test SHALL self-skip (`t.Skip`) when it cannot allocate
a PTY or set up process groups so CI stays green, and SHALL NOT leak host side
effects (uses `--worktree-open=skip` / `WT_TEST_NO_LAUNCH` env isolation).

- **GIVEN** a runner that can allocate a PTY and set process groups
- **WHEN** `wt create` runs under the PTY with a foreground-stranding init script
- **THEN** `wt` runs to completion (exit status not `WIFSTOPPED`) rather than being left stopped
- **AND** **GIVEN** a runner that cannot allocate a PTY / set process groups, **WHEN** the test runs, **THEN** it self-skips cleanly without failing

### Design Decisions

1. **Reclaim-after only (reject proactive handoff)**: capture foreground before init, reclaim after. — *Why*: the init child runs correctly sharing the TTY today; only the cleanup is missing (proven by the culprit trace in intake #2). — *Rejected*: proactively `tcsetpgrp`-ing the child into the foreground then reclaiming — adds a failure surface for no benefit.
2. **`os.Stdin.Fd()` as ttyFd**: matches `ShowMenu`'s `term.MakeRaw(int(os.Stdin.Fd()))` and the init child's `cmd.Stdin = os.Stdin`. — *Why*: reclaim and raw-mode target the same TTY. — *Rejected*: opening `/dev/tty` directly — deferred as future hardening (robust to stdin redirection, out of scope).
3. **`golang.org/x/sys/unix` ioctls**: `IoctlGetInt`+`TIOCGPGRP` / `IoctlSetPointerInt`+`TIOCSPGRP`. — *Why*: already in the module graph (indirect via `x/term`), so no new runtime dependency (Constitution I). — *Rejected*: raw `syscall.Syscall(SYS_IOCTL, ...)` — more error-prone, no benefit.
4. **Deferred restore closure + explicit pre-Open reclaim**: install a `defer` right after capture (best-effort, fires on all exits) AND an explicit reclaim before the Open phase. — *Why*: the explicit one is load-bearing/ordered before the menu write; the defer covers the failure/abort paths uniformly.
5. **`signal.Reset(syscall.SIGTTOU)` to restore disposition after `signal.Ignore`**: acceptable because `wt` installs no other SIGTTOU handler. — *Why*: simplest correct restore; SIGTTOU default is the kernel's stop-on-bg-tty-write, which is fine once reclaim succeeded.

### Non-Goals

- Opening `/dev/tty` directly (robust to stdin redirection) — future hardening.
- Proactively handing the terminal to the init child before it runs.
- Any change to the SIGINT Option B contract or the init child's process group.
- Policing what init scripts in other repos do with job control.

## Tasks

### Phase 1: Setup

- [x] T001 Add `src/cmd/wt/tty_unix.go` (`//go:build !windows`) with `terminalForeground(ttyFd int) (int, error)` (tcgetpgrp via `unix.IoctlGetInt(fd, unix.TIOCGPGRP)`) and `reclaimTerminalForeground(ttyFd int, pgrp int)` (tcsetpgrp via `unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgrp)` wrapped in `signal.Ignore(syscall.SIGTTOU)` + `signal.Reset(syscall.SIGTTOU)`) <!-- R3 R4 -->
- [x] T002 [P] Add `src/cmd/wt/tty_windows.go` (`//go:build windows`) with no-op `reclaimTerminalForeground` and `terminalForeground` returning `(0, nil)` <!-- R4 -->
- [x] T003 Run `cd src && go mod tidy` to promote `golang.org/x/sys` from indirect to direct in `src/go.mod` <!-- R4 -->

### Phase 2: Core Implementation

- [x] T004 In `src/cmd/wt/create.go`, inside the `if runInit { ... }` block: compute `ttyFd := int(os.Stdin.Fd())`, capture `terminalForeground(ttyFd)` adjacent to the existing `signal.Reset` (`create.go:282`) only when `term.IsTerminal(ttyFd)`, install a `defer` best-effort reclaim closure right after capture, and add explicit reclaims before the init-failure banner path (before `PrintInitFailureBanner`/`os.Exit`) and on the success path (after `signal.Stop`/`close`, before the Open phase) <!-- R1 R2 R6 -->
- [x] T005 In `src/cmd/wt/init.go` (`runInitScript`), apply the same capture-before / reclaim-after guard around `cmd.Run()` (capture before run when `term.IsTerminal`, reclaim on both the success return and the `ExitInitFailed` failure path) <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T006 Add `src/cmd/wt/tty_pty_test.go` (`//go:build !windows`): in-tree PTY allocation on `x/sys/unix` (`/dev/ptmx` + `TIOCSPTLCK`/`TIOCGPTN` + open `/dev/pts/N`) via a session-leader launcher harness, run `wt create --worktree-open=prompt` under the PTY with `WORKTREE_INIT_SCRIPT` pointing at a foreground-stranding script, assert `wt` is not left `WIFSTOPPED`; `t.Skip` when PTY/process-group setup is unavailable. NOTE: implemented with a launcher-leader harness (not a plain exec) because a plain Setsid'd exec orphans wt's group and converts the SIGTTOU stop into a silently-recovered EIO, masking the bug; verified to fail (exit 20 / WIFSTOPPED) without the fix and pass (exit 0) with it. <!-- R8 -->

### Phase 4: Polish

- [x] T007 Verify no CLI surface change: ran full `cmd/wt` suite (create/init/integration/sigint + new PTY test), confirmed stdout/exit-code/separator invariants hold; ran `gofmt -l .` (clean), `go vet ./...` (clean), `go build ./...` + windows/darwin cross-builds (all OK) from `src/` <!-- R7 R6 -->

## Execution Order

- T001, T002 before T003 (tidy needs the `unix` import to exist) before T004/T005 (which call the helpers).
- T004, T005 before T006 (the PTY test exercises the implemented reclaim).
- T007 last (whole-suite + tooling gate).

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `wt create` captures terminal foreground (`tcgetpgrp`) before the init child, only when `os.Stdin` is a TTY, adjacent to the existing `signal.Reset` with no inserted I/O.
- [ ] A-002 R2: `wt create` reclaims terminal foreground (`tcsetpgrp` to `unix.Getpgrp()`) on the success path before Open, on the init-failure path before the banner, and via a best-effort defer on the SIGINT-abort path.
- [ ] A-003 R3: The reclaim wraps `tcsetpgrp` in `signal.Ignore(syscall.SIGTTOU)` and restores disposition afterward.
- [ ] A-004 R4: `terminalForeground` / `reclaimTerminalForeground` exist with a `!windows` Unix impl using `x/sys/unix` ioctls and a `windows` no-op stub; both build tags compile.
- [ ] A-005 R5: `wt init` (`runInitScript`) applies the same capture/reclaim guard around its init child.

### Behavioral Correctness

- [ ] A-006 R2: When stdin is not a TTY (`--non-interactive`, piped), capture/reclaim are no-ops — no syscall, no error/warning, existing non-interactive tests unaffected.
- [ ] A-007 R8: Under a real PTY with a foreground-stranding init script, `wt create` runs to completion and is NOT left stopped (`WIFSTOPPED` false); the test self-skips where PTY/process groups are unavailable.

### Scenario Coverage

- [ ] A-008 R8: The PTY integration test exists in `src/cmd/wt/tty_pty_test.go`, is `//go:build !windows`, allocates the PTY via in-tree `openpty` on `x/sys/unix` (no `creack/pty`), and uses `--worktree-open=skip` / env isolation (no host side effects).

### Edge Cases & Error Handling

- [ ] A-009 R6: `SysProcAttr{Setpgid: true}`, the tight reinstall window, and `defer rb.Execute()` are all preserved; the existing SIGINT integration test (`integration_sigint_unix_test.go`) still passes.
- [ ] A-010 R2: The foreground-restore defer is best-effort — its failure never blocks `rb.Execute()` rollback or alters exit codes.

### Code Quality

- [ ] A-011 R7: No public CLI surface change — same flags, exit codes; stdout stays the worktree-path line only (create) and separators stay stderr-only (`create-output-phases.md`).
- [ ] A-012 Pattern consistency: New code follows the `signal_unix.go`/`signal_windows.go` build-tag pattern and surrounding naming/error-handling style.
- [ ] A-013 No unnecessary duplication: The ioctl helpers are the single seam for tcgetpgrp/tcsetpgrp; no duplicate process-group/terminal manipulation elsewhere.
- [ ] A-014 No magic numbers: ioctl request numbers come from named `unix.TIOC*` constants, not literals.
- [ ] A-015 No god functions: helper functions stay small and focused; the create.go additions do not bloat the RunE body beyond the codebase norm.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reclaim target pgrp = `unix.Getpgrp()` (wt's own group), used as the source of truth for "give the terminal back to me", equivalent to the captured pre-init `fg`. | Intake assumption #3/#8 and "What Changes" §1 state Getpgrp() explicitly; wt was foreground before init so the captured fg == Getpgrp(). | S:95 R:80 A:90 D:90 |
| 2 | Certain | Restore SIGTTOU disposition via `signal.Reset(syscall.SIGTTOU)` after `signal.Ignore`, since wt installs no other SIGTTOU handler. | Intake "What Changes" §2 and key decision both call this acceptable. | S:95 R:80 A:90 D:85 |
| 3 | Certain | PTY allocated via `/dev/ptmx` + `unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)` (unlockpt) + `unix.IoctlGetInt(fd, unix.TIOCGPTN)` (ptsname number) + open `/dev/pts/N`, because x/sys/unix v0.44.0 does NOT expose the high-level `Openpt`/`Grantpt`/`Unlockpt`/`PtsName` helpers. | Verified via `go doc` at plan time: those symbols are absent; the listed Linux ioctl constants (`TIOCSPTLCK`, `TIOCGPTN`) are present. Intake permitted "whatever x/sys/unix exposes on linux". | S:80 R:70 A:80 D:75 |
| 4 | Certain | The helper pair lives in `src/cmd/wt/` (not `internal/worktree/`) because the tty fd is owned at the cmd layer (`os.Stdin`) alongside the signal handling already in create.go. | Intake key decision explicitly chose cmd/wt/, mirroring signal_unix.go's location. | S:95 R:80 A:90 D:90 |

4 assumptions (4 certain, 0 confident, 0 tentative).
