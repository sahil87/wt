//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// signalInitProcessGroup delivers SIGINT to the init child's process group
// so any descendants the script spawned also receive the signal. This pairs
// with the Setpgid: true SysProcAttr set on the cmd by ResolveInitInvocation
// (see internal/worktree/init_unix.go).
//
// We pass -pid to syscall.Kill — the negative PID is the documented way to
// target a process group on Unix.
func signalInitProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Best-effort: errors here mean the process already exited, which is
	// fine — cmd.Run() will return naturally.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}
