//go:build windows

package main

// Windows has no tcsetpgrp / process-group terminal model, so terminal
// foreground bookkeeping is a no-op there — mirroring signal_windows.go and
// internal/worktree/init_windows.go. The init/signal job-control code is
// already Unix-only by the same build-tag pattern.

// terminalForeground is a no-op on Windows; there is no foreground process
// group to capture.
func terminalForeground(ttyFd int) (int, error) { return 0, nil }

// reclaimTerminalForeground is a no-op on Windows.
func reclaimTerminalForeground(ttyFd int, pgrp int) {}
