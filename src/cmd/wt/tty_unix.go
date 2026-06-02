//go:build !windows

package main

import (
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// terminalForeground returns the process group currently in the foreground of
// the terminal referenced by ttyFd (tcgetpgrp). Callers MUST guard on
// term.IsTerminal(ttyFd) first — issuing this ioctl against a non-terminal fd
// is meaningless and returns an error.
func terminalForeground(ttyFd int) (int, error) {
	return unix.IoctlGetInt(ttyFd, unix.TIOCGPGRP)
}

// reclaimTerminalForeground gives the terminal referenced by ttyFd back to the
// process group pgrp (tcsetpgrp). It is the corrective bookkeeping wt performs
// after the init child returns: if the init script (or a descendant) grabbed
// the terminal foreground and exited without restoring it, wt would be left in
// the background and its next TTY write (the Open-phase menu render) would trip
// SIGTTOU and suspend the process.
//
// The tcsetpgrp itself is a foreground-mutating operation, so a background
// process issuing it is answered by the kernel with SIGTTOU. We therefore
// ignore SIGTTOU for the duration of the call and restore the default
// disposition afterward — without this, the reclaim that is supposed to
// un-stick us would stick us instead. signal.Reset is the correct restore
// because wt installs no other SIGTTOU handler.
//
// Callers MUST guard on term.IsTerminal(ttyFd) first; this is a no-op-by-error
// against a non-terminal fd. Errors are intentionally ignored: this is
// best-effort cleanup that must never block rollback or change exit codes.
func reclaimTerminalForeground(ttyFd int, pgrp int) {
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(ttyFd, unix.TIOCSPGRP, pgrp)
}
