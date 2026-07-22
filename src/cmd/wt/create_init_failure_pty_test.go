//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The interactive open-anyway prompt on init failure is gated on stdin being a
// real TTY (term.IsTerminal) AND wt being able to read the terminal's
// foreground process group (tcgetpgrp succeeding). A plain exec with a pipe
// stdin fails the TTY check, so these tests drive wt under a real PTY via a
// session-leader launcher — the same harness shape as
// TestIntegration_ReclaimForegroundAfterInit_NotStopped, but here the launcher
// feeds a "y\n" or "n\n" answer to the open-anyway prompt and reports wt's exit
// code back through its own exit code.
//
// Host-side-effect safety (code-review.md): wt is invoked with
// --worktree-open=open_here, the cooperative app that only records the
// navigation target via wt.NavigateTo (stderr → confirmation + bare path on
// stdout; and never under WT_TEST_NO_LAUNCH short-circuits it anyway). No real
// editor/tmux/byobu window is ever created. The PTY env also clears
// TMUX/BYOBU_*/TERM_PROGRAM and sets WT_TEST_NO_LAUNCH=1.

// setupFailureExit is the launcher's "could not set up PTY / process groups"
// signal (e.g. sandboxed CI). wt itself never exits with this code, so the test
// treats it as a skip rather than a failure.
const setupFailureExit = 30

// runInteractiveInitFailure builds the launcher, wires up a failing init script,
// and runs `wt create` under a PTY with the given answer ("y" or "n") fed to the
// open-anyway prompt. It returns wt's combined PTY output and exit code, or
// skips the test when the PTY/process-group harness is unavailable.
func runInteractiveInitFailure(t *testing.T, repo, wtName, answer string) (string, int) {
	t.Helper()

	helperDir := t.TempDir()
	launcherBin := buildHelper(t, helperDir, "answer_launcher", ptyAnswerLauncherSrc)

	// Failing init script: stream a marker, exit non-zero. This drives the
	// init-failure branch without grabbing terminal foreground (the foreground
	// concern is covered by the dedicated reclaim test).
	scriptDir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll script dir: %v", err)
	}
	script := filepath.Join(scriptDir, "init-fail.sh")
	content := "#!/usr/bin/env bash\necho 'INIT_FAIL_MARKER' >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write init script: %v", err)
	}
	gitRun(t, repo, "add", "scripts/init-fail.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add failing init script")
	// Keep the repo clean so wt does not stop at the interactive dirty-state
	// prompt (which would block on PTY input before the init-failure prompt).
	gitRun(t, repo, "add", "-A")
	if status := gitRun(t, repo, "status", "--porcelain"); status != "" {
		gitRun(t, repo, "commit", "-q", "-m", "clean")
	}

	// argv: launcher <answer> <wtBinary> <repoDir> <wtName>
	launch := exec.Command(launcherBin, answer, wtBinary, repo, wtName)
	launch.Env = append(os.Environ(),
		"NO_COLOR=1",
		"WORKTREE_INIT_SCRIPT=scripts/init-fail.sh",
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
	if exitCode == setupFailureExit {
		t.Skipf("PTY/process-group harness unavailable (launcher exit %d) — skipping\n%s", exitCode, combined)
	}
	return string(combined), exitCode
}

// TestCreate_InitFailureInteractive_OpenAnyway exercises the interactive Yes
// path: the open-anyway prompt is shown, the user answers Yes, control falls
// through to the existing Open phase (open_here emits NavigateTo's stderr `→`
// confirmation and the bare path on stdout), and the process STILL exits
// ExitInitFailed (7) — a successful open must not downgrade the exit to 0. The
// worktree and branch are kept.
func TestCreate_InitFailureInteractive_OpenAnyway(t *testing.T) {
	repo := createTestRepo(t)

	out, exitCode := runInteractiveInitFailure(t, repo, "open-anyway-yes", "y")

	if exitCode != 7 {
		t.Fatalf("expected exit 7 on interactive open-anyway (Yes), got %d\noutput:\n%s", exitCode, out)
	}
	// The banner (now with the wt go hint) and the open-anyway prompt are shown.
	assertContains(t, out, "wt go 'open-anyway-yes'")
	assertContains(t, out, "Continue and open the worktree anyway?")
	// Yes fell through to the Open phase: open_here routes through NavigateTo —
	// the `→` confirmation is emitted, and with WT_CD_FILE unset (and no
	// wrapper) the shell-wrapper hint is shown. The retired `cd -- '` form must
	// never reappear (launcher-contract.md §3, v2).
	assertContains(t, out, "→ ")
	assertContains(t, out, "cd needs the shell wrapper")
	assertNotContains(t, out, "cd -- '")
	// Worktree + branch are kept.
	assertWorktreeExists(t, repo, "open-anyway-yes")
	assertBranchExists(t, repo, "open-anyway-yes")
}

// TestCreate_InitFailureInteractive_DeclineOpen exercises the interactive No
// path: the prompt is shown, the user answers No, the Open phase is skipped (no
// cd line, no app menu), and the process exits ExitInitFailed (7). The worktree
// and branch are kept.
func TestCreate_InitFailureInteractive_DeclineOpen(t *testing.T) {
	repo := createTestRepo(t)

	out, exitCode := runInteractiveInitFailure(t, repo, "open-anyway-no", "n")

	if exitCode != 7 {
		t.Fatalf("expected exit 7 on interactive open-anyway (No), got %d\noutput:\n%s", exitCode, out)
	}
	assertContains(t, out, "wt go 'open-anyway-no'")
	assertContains(t, out, "Continue and open the worktree anyway?")
	// No: the Open phase did not run — no NavigateTo confirmation and no
	// "Open in:" menu.
	assertNotContains(t, out, "→ ")
	assertNotContains(t, out, "Open in:")
	// Worktree + branch are still kept.
	assertWorktreeExists(t, repo, "open-anyway-no")
	assertBranchExists(t, repo, "open-anyway-no")
}

// ptyAnswerLauncherSrc is the session-leader harness for the interactive
// open-anyway prompt. It allocates a PTY, becomes a session leader owning the
// slave as its controlling terminal, runs wt as a foreground child in wt's own
// (non-orphaned) process group with a failing init script wired in, feeds the
// requested answer to the open-anyway prompt, copies wt's PTY output to its own
// stdout (for the test to assert on), and exits with wt's exit code (or 30 on
// setup failure).
//
// argv: launcher <answer> <wtBinary> <repoDir> <wtName>
// env:  WORKTREE_INIT_SCRIPT, WT_TEST_NO_LAUNCH, NO_COLOR, TMUX=, BYOBU_*, etc.
const ptyAnswerLauncherSrc = `package main

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"sync"
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
	if len(os.Args) < 5 {
		fail()
	}
	answer, wtBin, repo, wtName := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

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

	master := os.NewFile(uintptr(mfd), "ptmx")

	// Drain the master into a buffer and mirror it to our stdout so the test can
	// assert on wt's banner / prompt / cd-line output.
	var mu sync.Mutex
	var buf bytes.Buffer
	go func() {
		b := make([]byte, 4096)
		for {
			m, e := master.Read(b)
			if m > 0 {
				mu.Lock()
				buf.Write(b[:m])
				mu.Unlock()
				_, _ = os.Stdout.Write(b[:m])
			}
			if e != nil {
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
	// its live parent in the same session). NOT --non-interactive, so the
	// open-anyway prompt is reachable; --worktree-name skips the name prompt;
	// exploratory (no branch arg) skips the "Initialize worktree?" prompt;
	// --worktree-open=open_here is the side-effect-free Open target.
	cmd := exec.Command(wtBin, "create", "--worktree-name", wtName, "--worktree-open", "open_here")
	cmd.Dir = repo
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fail()
	}
	wpgid, err := unix.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		fail()
	}
	// Put wt's process group in the terminal foreground so wt's tcgetpgrp
	// (terminalForeground) succeeds and reclaimTTY stays true — that is what
	// makes the interactive open-anyway prompt reachable. The launcher's own
	// group is the current foreground (it just did setsid + TIOCSCTTY), so this
	// handoff does not SIGTTOU; ignore it defensively anyway.
	signal.Ignore(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(sfd, unix.TIOCSPGRP, wpgid)

	// Feed the answer to the open-anyway prompt. The prompt uses a line-buffered
	// ReadString('\n') in cooked mode, so the bytes can be written ahead of the
	// prompt being rendered — they wait in the PTY line buffer until wt reads
	// them. Give wt a brief head start so the answer lands after the banner.
	go func() {
		time.Sleep(500 * time.Millisecond)
		_, _ = master.Write([]byte(answer + "\n"))
	}()

	// Wait for wt to exit, bounded by a deadline so a hang cannot wedge the test.
	type waitResult struct {
		ws  unix.WaitStatus
		err error
	}
	done := make(chan waitResult, 1)
	go func() {
		var ws unix.WaitStatus
		_, e := unix.Wait4(cmd.Process.Pid, &ws, 0, nil)
		done <- waitResult{ws: ws, err: e}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			_ = syscall.Kill(-wpgid, syscall.SIGKILL)
			fail()
		}
		// Let the drain goroutine flush any tail output.
		time.Sleep(100 * time.Millisecond)
		_ = master.Close()
		if r.ws.Exited() {
			os.Exit(r.ws.ExitStatus())
		}
		// Signaled or stopped — not an expected wt outcome here.
		os.Exit(30)
	case <-time.After(20 * time.Second):
		_ = syscall.Kill(-wpgid, syscall.SIGKILL)
		fail()
	}
}
`
