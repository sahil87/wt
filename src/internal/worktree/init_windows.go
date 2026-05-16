//go:build windows

package worktree

import "os/exec"

// setInitProcessGroup is a no-op on Windows. Setpgid is unavailable; the
// SIGINT-during-init handler in wt create skips process-group signaling on
// Windows accordingly. See docs/specs/init-protocol.md.
func setInitProcessGroup(cmd *exec.Cmd) {}
