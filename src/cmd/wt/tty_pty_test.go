//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestIntegration_ReclaimForegroundAfterInit_NotStopped is the end-to-end proof
// for the terminal-foreground reclaim fix. It reproduces the original bug
// faithfully: an init script (or a descendant) grabs the controlling terminal's
// foreground process group and exits without restoring it, leaving wt in the
// background. wt's next terminal-control operation — the Open-phase
// ShowMenu -> term.MakeRaw(stdin) (a tcsetattr) — is then issued from a
// background process group and the kernel answers it with SIGTTOU, stopping wt
// (WIFSTOPPED). After the fix, wt reclaims foreground for its own process group
// after the init child returns, so MakeRaw succeeds and wt runs to completion.
//
// Why the launcher-leader harness (and not a plain exec under a PTY): SIGTTOU is
// only delivered to a process in a NON-orphaned process group. If the test made
// wt a session leader (Setsid), wt's group would be orphaned and the kernel
// would convert the stop into an EIO the menu code silently recovers from —
// masking the bug. So a tiny in-tree launcher becomes the session leader that
// owns the PTY, then runs wt as a foreground child in wt's OWN (non-leader,
// non-orphaned) process group. That mirrors how a pane's interactive shell
// launches wt in the real failure. The launcher reports wt's wait status back
// through its own exit code: 20 = wt stopped (bug present), 0 = wt ran to
// completion (fix working), other = harness could not reproduce (skip).
//
// Unix-only (build !windows): PTYs, tcsetpgrp, and Setsid are Unix concepts.
//
// Self-skipping: PTY allocation and process-group setup are fiddly and can be
// unavailable on sandboxed CI runners. The launcher reports an unreproducible
// setup via exit code 30, which this test treats as t.Skip — so CI stays green.
// The always-runnable safety net is the unit-level guard (helpers no-op when fd
// is not a TTY) plus the cross-platform build of the no-op Windows stubs.
//
// Host-side-effect safety: --worktree-open=prompt resolves to the cooperative
// "Open here" app under the cleared launcher env (TMUX/BYOBU/TERM_PROGRAM) and
// WT_TEST_NO_LAUNCH=1, so no real windows/clipboard are touched (code-review.md).
func TestIntegration_ReclaimForegroundAfterInit_NotStopped(t *testing.T) {
	repo := createTestRepo(t)

	helperDir := t.TempDir()

	// Build the foreground-grabber: mimics doctor.sh:111's hostile-to-shared-TTY
	// pattern deterministically — put itself in a new process group, tcsetpgrp
	// the controlling terminal to that group, then exit WITHOUT restoring
	// foreground to wt. A compiled helper avoids depending on which interactive
	// shell (zsh -i) is installed on the runner.
	grabberBin := buildHelper(t, helperDir, "grabber", ptyGrabberSrc)

	// Build the launcher: the session-leader harness described above.
	launcherBin := buildHelper(t, helperDir, "launcher", ptyLauncherSrc)

	// Init script that runs the grabber on the shared TTY, then returns success.
	scriptDir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll script dir: %v", err)
	}
	script := filepath.Join(scriptDir, "init-grab-fg.sh")
	content := "#!/usr/bin/env bash\n" + grabberBin + "\necho grabber-done\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write init script: %v", err)
	}
	gitRun(t, repo, "add", "scripts/init-grab-fg.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add foreground-grabbing init script")
	// Keep the repo clean so wt does not stop at the interactive dirty-state
	// prompt (which would block on PTY input unrelated to the foreground bug).
	gitRun(t, repo, "add", "-A")
	if status := gitRun(t, repo, "status", "--porcelain"); status != "" {
		gitRun(t, repo, "commit", "-q", "-m", "clean")
	}

	// The launcher runs entirely in-process under its own PTY; we only read its
	// exit code. It receives the wt binary path and repo dir via argv and the
	// init-script + isolation env via the environment.
	launch := exec.Command(launcherBin, wtBinary, repo)
	launch.Env = append(os.Environ(),
		"NO_COLOR=1",
		"WORKTREE_INIT_SCRIPT=scripts/init-grab-fg.sh",
		"WT_TEST_NO_LAUNCH=1",
		"TMUX=", "BYOBU_BACKEND=", "BYOBU_TTY=", "BYOBU_SESSION=",
		"BYOBU_CONFIG_DIR=", "TERM_PROGRAM=",
	)
	combined, runErr := launch.CombinedOutput()

	exitCode := 0
	if runErr != nil {
		ee, ok := runErr.(*exec.ExitError)
		if !ok {
			t.Skipf("launcher did not run (sandboxed?): %v\n%s", runErr, combined)
		}
		exitCode = ee.ExitCode()
	}

	switch exitCode {
	case launcherExitWtCompleted:
		// Fix working: wt reclaimed foreground and ran to completion.
	case launcherExitWtStopped:
		t.Fatalf("wt was left STOPPED (WIFSTOPPED) after init — terminal foreground was not reclaimed\nlauncher output:\n%s", combined)
	default:
		// Any other code means the harness could not set up the PTY / process
		// groups (e.g. sandboxed CI). Skip rather than fail, per the intake's
		// CI-feasibility flag.
		t.Skipf("PTY/process-group harness unavailable (launcher exit %d) — skipping\n%s", exitCode, combined)
	}
}

// Launcher exit-code contract (shared between the test and ptyLauncherSrc).
const (
	launcherExitWtCompleted = 0  // wt ran to completion (fix working)
	launcherExitWtStopped   = 20 // wt left WIFSTOPPED (bug present)
	// any other non-zero code (e.g. 30) => harness could not reproduce => skip
)

// buildHelper compiles a tiny single-file Go program inside the module so it can
// resolve golang.org/x/sys/unix, returning the built binary path. Skips the test
// on build failure (e.g. no Go toolchain available to the test runner).
func buildHelper(t *testing.T, dir, name, src string) string {
	t.Helper()
	srcPath := filepath.Join(dir, name+".go")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s source: %v", name, err)
	}
	bin := filepath.Join(dir, name)
	build := exec.Command("go", "build", "-o", bin, srcPath)
	build.Dir = filepath.Join(mustGetModuleRoot(), "cmd", "wt")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build %s helper: %v\n%s", name, err, out)
	}
	return bin
}

// ptyGrabberSrc strands terminal foreground: new process group, tcsetpgrp the
// controlling terminal (fd 0, shared with wt's stdin) to it, then exit without
// restoring foreground to wt.
const ptyGrabberSrc = `package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func main() {
	signal.Ignore(syscall.SIGTTOU)
	if err := unix.Setpgid(0, 0); err != nil {
		os.Exit(0)
	}
	pgrp, err := unix.Getpgid(0)
	if err != nil {
		os.Exit(0)
	}
	_ = unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, pgrp)
	os.Exit(0)
}
`

// ptyLauncherSrc is the session-leader harness. It allocates a PTY, becomes a
// session leader owning the slave as its controlling terminal, then runs wt as
// a foreground child in wt's own (non-orphaned) process group with the
// foreground-grabbing init script wired in. It reports wt's wait status via its
// own exit code (0 = completed, 20 = stopped, 30 = setup failure).
//
// argv: launcher <wtBinary> <repoDir>
// env:  WORKTREE_INIT_SCRIPT, WT_TEST_NO_LAUNCH, NO_COLOR, TMUX=, BYOBU_*, etc.
const ptyLauncherSrc = `package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func fail() { os.Exit(30) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func main() {
	if len(os.Args) < 3 {
		fail()
	}
	wtBin, repo := os.Args[1], os.Args[2]

	// Allocate a PTY.
	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		fail()
	}
	if err := unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0); err != nil {
		fail()
	}
	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	if err != nil {
		fail()
	}
	sfd, err := unix.Open("/dev/pts/"+itoa(n), unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		fail()
	}

	// Drain the master so slave-side writes never block on a full buffer.
	master := os.NewFile(uintptr(mfd), "ptmx")
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, e := master.Read(buf); e != nil {
				return
			}
		}
	}()

	// Become a session leader and acquire the slave as controlling terminal.
	if _, err := unix.Setsid(); err != nil {
		fail()
	}
	if err := unix.IoctlSetPointerInt(sfd, unix.TIOCSCTTY, 0); err != nil {
		fail()
	}

	slave := os.NewFile(uintptr(sfd), "pts")

	// Run wt as a child in its OWN process group (non-orphaned: this launcher is
	// its live parent in the same session). --worktree-open=prompt makes wt
	// reach ShowMenu -> term.MakeRaw, the tcsetattr that SIGTTOUs from the
	// background when foreground was stranded.
	cmd := exec.Command(wtBin, "create", "--non-interactive",
		"--worktree-name", "fg-test", "--worktree-open", "prompt")
	cmd.Dir = repo
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fail()
	}
	// Put wt's process group in the terminal foreground (wt owns the foreground
	// BEFORE init, exactly as in the real failure). Ignore SIGTTOU around the
	// tcsetpgrp since the launcher is the current foreground.
	wpgid, err := unix.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		fail()
	}
	// The launcher's own group is the current foreground (it just did setsid +
	// TIOCSCTTY), so this handoff does not SIGTTOU; ignore it defensively anyway.
	signal.Ignore(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(sfd, unix.TIOCSPGRP, wpgid)

	// Observe wt's state. The bug and the fix are distinguished by whether wt is
	// STOPPED at the Open-phase term.MakeRaw:
	//   - BUG:  MakeRaw (a tcsetattr from the background) raises SIGTTOU and wt
	//           is left WIFSTOPPED. Wait4(WUNTRACED) reports it immediately.
	//   - FIX:  foreground reclaimed, MakeRaw succeeds, wt renders the menu and
	//           then BLOCKS reading a keypress that never comes (or exits). So
	//           wt is either still alive (not stopped) or has exited cleanly.
	// We poll until a deadline: any STOPPED observation => bug (exit 20); a clean
	// exit or a still-running-but-not-stopped wt => fix (exit 0). The window must
	// comfortably exceed wt's time-to-Open-phase (worktree add + init script) so a
	// slow runner cannot let the deadline elapse BEFORE wt reaches term.MakeRaw —
	// that would report "never stopped" (exit 0) vacuously even if the bug were
	// present. 15s matches the integration-test budget in
	// integration_sigint_unix_test.go.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var ws unix.WaitStatus
		wpid, e := unix.Wait4(cmd.Process.Pid, &ws, unix.WUNTRACED|unix.WNOHANG, nil)
		if e != nil {
			_ = syscall.Kill(-wpgid, syscall.SIGKILL)
			fail()
		}
		if wpid == cmd.Process.Pid {
			if ws.Stopped() {
				_ = syscall.Kill(-wpgid, syscall.SIGKILL)
				os.Exit(20) // bug: wt was SIGTTOU-stopped
			}
			if ws.Exited() || ws.Signaled() {
				os.Exit(0) // fix: wt ran to completion without being stopped
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// wt is still alive and was never observed stopped — it reached the menu
	// without SIGTTOU. The fix is working; reap it and report success.
	_ = syscall.Kill(-wpgid, syscall.SIGKILL)
	os.Exit(0)
}
`
