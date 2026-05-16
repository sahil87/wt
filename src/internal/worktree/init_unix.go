//go:build !windows

package worktree

import (
	"os/exec"
	"syscall"
)

// setInitProcessGroup configures cmd so the child runs in its own process
// group. This lets a signal handler deliver SIGINT to the whole group
// (script + descendants) by signaling -cmd.Process.Pid. Unix-only — Windows
// uses a different process-grouping mechanism that we do not target.
func setInitProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
