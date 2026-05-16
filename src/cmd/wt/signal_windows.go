//go:build windows

package main

import "os/exec"

// signalInitProcessGroup is a no-op on Windows. Setpgid is unavailable
// there, and the SIGINT-during-init test is skipped on Windows per spec.
func signalInitProcessGroup(cmd *exec.Cmd) {}
