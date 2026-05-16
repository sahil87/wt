//go:build windows

package worktree

import (
	"os/exec"
	"testing"
)

// assertInitProcessGroupSet is a no-op on Windows: syscall.SysProcAttr does
// not expose Setpgid, and the SIGINT-during-init contract is Unix-only
// (the integration test that exercises it skips on GOOS=windows).
func assertInitProcessGroupSet(_ *testing.T, _ *exec.Cmd) {}
