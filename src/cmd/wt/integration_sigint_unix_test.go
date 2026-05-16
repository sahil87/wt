//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestIntegration_SIGINTDuringInit_KeepsWorktreeAndExits7 verifies the
// SIGINT Option B contract: while a long-running init script is in flight,
// SIGINT against the wt process should terminate the init child, keep the
// worktree, and exit with ExitInitFailed (7) — NOT roll back the worktree
// or exit 130. Required by spec.md "Requirement: Automated SIGINT
// integration test".
//
// Lives in a Unix-only file (build !windows) because syscall.SysProcAttr
// does not expose Setpgid on Windows and syscall.Kill is Unix-only. The
// SIGINT-during-init contract itself is Unix-only.
func TestIntegration_SIGINTDuringInit_KeepsWorktreeAndExits7(t *testing.T) {
	repo := createTestRepo(t)

	// Slow init script: sleep keeps the init phase in-flight long enough
	// for the test to deliver SIGINT deterministically.
	scriptDir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	script := filepath.Join(scriptDir, "init-slow.sh")
	content := "#!/usr/bin/env bash\nsleep 30\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile slow init: %v", err)
	}
	gitRun(t, repo, "add", "scripts/init-slow.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add slow init script")

	wtPath := worktreePath(repo, "sigint-test")

	// Build the command but Start (not Run) so we can signal it mid-flight.
	cmd := exec.Command(wtBinary, "create", "--non-interactive",
		"--worktree-name", "sigint-test",
		"--worktree-open", "skip")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"WORKTREE_INIT_SCRIPT=scripts/init-slow.sh",
		"TMUX=", "BYOBU_BACKEND=", "BYOBU_TTY=", "BYOBU_SESSION=",
		"BYOBU_CONFIG_DIR=", "TERM_PROGRAM=",
	)
	// Put the wt process in its own group so killing the group reaps any
	// orphans if the test fails partway.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		// Best-effort: kill the process group if anything leaked.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})

	// Poll for the worktree directory's existence: this proves git ops
	// finished and we're in the init phase (where SIGINT must NOT trigger
	// rollback).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(wtPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(wtPath); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("worktree never created (init didn't start): %v\nstderr: %s", err, stderr.String())
	}
	// Extra cushion to ensure the signal-handler swap completed.
	time.Sleep(100 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal(SIGINT): %v", err)
	}

	// Wait with a generous timeout.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("wt did not exit within 15s after SIGINT\nstderr: %s", stderr.String())
	}

	// Exit code 7 (ExitInitFailed) — SIGINT routed through the init child
	// into the init-failure path, NOT through rollback (which would exit 130).
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected wait error: %v", waitErr)
		}
	}
	if exitCode != 7 {
		t.Errorf("expected exit code 7, got %d\nstderr: %s", exitCode, stderr.String())
	}

	// Worktree + branch survive.
	assertDirExists(t, wtPath)
	assertBranchExists(t, repo, "sigint-test")
}
