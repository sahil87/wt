//go:build !windows

package worktree

import (
	"os/exec"
	"syscall"
	"testing"
)

// assertInitProcessGroupSet verifies the resolver set Setpgid=true on the
// returned cmd so wt create's SIGINT-during-init handler can target the
// init child's process group via syscall.Kill(-pid, ...). The assertion
// lives in a Unix-only test file because syscall.SysProcAttr does not
// expose Setpgid on Windows — the cross-platform init_test.go cannot
// reference the field directly without breaking the Windows build.
func assertInitProcessGroupSet(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.SysProcAttr == nil {
		t.Error("expected SysProcAttr to be set on Unix")
		return
	}
	sysAttr, ok := any(cmd.SysProcAttr).(*syscall.SysProcAttr)
	if !ok || !sysAttr.Setpgid {
		t.Error("expected SysProcAttr.Setpgid = true on Unix")
	}
}
