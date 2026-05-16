package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns whatever
// was written. Used so PrintInitFailureBanner can be asserted without
// touching the real terminal.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	defer func() {
		os.Stderr = orig
	}()
	fn()
	w.Close()
	<-done
	wg.Wait()
	return buf.String()
}

// makeExitError runs a tiny `false`-equivalent so we get a real
// *exec.ExitError with a known status. Avoids reaching into unexported
// fields of exec.ExitError. Skips the calling test when no POSIX shell
// is on PATH (Windows, minimal containers) — the seam being tested
// (banner shape) is OS-agnostic; only the fixture needs sh.
func makeExitError(t *testing.T, status int) error {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not on PATH; cannot construct *exec.ExitError fixture: %v", err)
	}
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", status))
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected sh -c 'exit %d' to fail", status)
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
	return err
}

func TestWtError_Format(t *testing.T) {
	// Save original colors
	origRed := ColorRed
	origBold := ColorBold
	origReset := ColorReset
	defer func() {
		ColorRed = origRed
		ColorBold = origBold
		ColorReset = origReset
	}()

	// Disable colors for testing
	ColorRed = ""
	ColorBold = ""
	ColorReset = ""

	msg := WtError("Something failed", "Because of X", "Do Y instead")

	if !strings.Contains(msg, "Error: Something failed") {
		t.Errorf("error message should contain 'Error: Something failed', got: %s", msg)
	}
	if !strings.Contains(msg, "Why: Because of X") {
		t.Errorf("error message should contain 'Why: Because of X', got: %s", msg)
	}
	if !strings.Contains(msg, "Fix: Do Y instead") {
		t.Errorf("error message should contain 'Fix: Do Y instead', got: %s", msg)
	}
}

func TestWtError_WithoutFix(t *testing.T) {
	origRed := ColorRed
	origBold := ColorBold
	origReset := ColorReset
	defer func() {
		ColorRed = origRed
		ColorBold = origBold
		ColorReset = origReset
	}()

	ColorRed = ""
	ColorBold = ""
	ColorReset = ""

	msg := WtError("Something failed", "Because of X", "")

	if strings.Contains(msg, "Fix:") {
		t.Errorf("error message should not contain 'Fix:' when fix is empty, got: %s", msg)
	}
}

func TestWtError_NoColor(t *testing.T) {
	// Set NO_COLOR and manually apply
	origRed := ColorRed
	origBold := ColorBold
	origReset := ColorReset
	defer func() {
		ColorRed = origRed
		ColorBold = origBold
		ColorReset = origReset
	}()

	ColorRed = ""
	ColorBold = ""
	ColorReset = ""

	msg := WtError("Test", "Why", "Fix")

	// Should not contain ANSI codes
	if strings.Contains(msg, "\033[") {
		t.Errorf("NO_COLOR: message should not contain ANSI codes, got: %s", msg)
	}
}

func TestWtError_WithColor(t *testing.T) {
	origRed := ColorRed
	origBold := ColorBold
	origReset := ColorReset
	defer func() {
		ColorRed = origRed
		ColorBold = origBold
		ColorReset = origReset
	}()

	ColorRed = "\033[0;31m"
	ColorBold = "\033[1m"
	ColorReset = "\033[0m"

	msg := WtError("Test", "Why", "Fix")

	if !strings.Contains(msg, "\033[0;31m") {
		t.Errorf("color: message should contain ANSI red code, got: %s", msg)
	}
	if !strings.Contains(msg, "\033[1m") {
		t.Errorf("color: message should contain ANSI bold code, got: %s", msg)
	}
}

func TestPrintInitFailureBanner_ExitError(t *testing.T) {
	origRed, origBold, origReset := ColorRed, ColorBold, ColorReset
	defer func() { ColorRed, ColorBold, ColorReset = origRed, origBold, origReset }()
	ColorRed, ColorBold, ColorReset = "", "", ""

	wtPath := "/tmp/test-worktree-abc"
	name := "abc"
	err := makeExitError(t, 2)

	out := captureStderr(t, func() {
		PrintInitFailureBanner(wtPath, name, err)
	})

	if !strings.Contains(out, wtPath) {
		t.Errorf("banner missing worktree path %q:\n%s", wtPath, out)
	}
	if !strings.Contains(out, "status 2") {
		t.Errorf("banner missing 'status 2' for *exec.ExitError:\n%s", out)
	}
	if !strings.Contains(out, "wt init") {
		t.Errorf("banner missing 'wt init' retry hint:\n%s", out)
	}
	if !strings.Contains(out, "&&") {
		t.Errorf("banner missing '&&' in retry hint (must be copy-paste safe across shells):\n%s", out)
	}
	if !strings.Contains(out, "wt delete '"+name+"'") {
		t.Errorf("banner missing 'wt delete %s' remove hint (single-quoted):\n%s", name, out)
	}
}

func TestPrintInitFailureBanner_NonExitError(t *testing.T) {
	origRed, origBold, origReset := ColorRed, ColorBold, ColorReset
	defer func() { ColorRed, ColorBold, ColorReset = origRed, origBold, origReset }()
	ColorRed, ColorBold, ColorReset = "", "", ""

	wtPath := "/tmp/test-worktree-xyz"
	name := "xyz"
	// Plain error that does NOT unwrap to *exec.ExitError.
	err := errors.New("init process killed before exit")

	out := captureStderr(t, func() {
		PrintInitFailureBanner(wtPath, name, err)
	})

	if strings.Contains(out, "status ") {
		t.Errorf("non-ExitError banner should NOT contain 'status N':\n%s", out)
	}
	if !strings.Contains(out, wtPath) {
		t.Errorf("banner missing worktree path %q:\n%s", wtPath, out)
	}
	if !strings.Contains(out, "wt init") {
		t.Errorf("banner missing 'wt init' retry hint:\n%s", out)
	}
	if !strings.Contains(out, "&&") {
		t.Errorf("banner missing '&&' in retry hint:\n%s", out)
	}
	if !strings.Contains(out, "wt delete '"+name+"'") {
		t.Errorf("banner missing 'wt delete %s' remove hint (single-quoted):\n%s", name, out)
	}
}

// TestPrintInitFailureBanner_PathWithSpaces verifies the retry/remove hints
// stay copy-paste-safe when the worktree path or name contains shell-special
// characters (spaces, single quotes). Regression: pre-fix the hint
// `cd /tmp/my repo/wt && wt init` would shell-split into `cd /tmp/my`.
func TestPrintInitFailureBanner_PathWithSpaces(t *testing.T) {
	origRed, origBold, origReset := ColorRed, ColorBold, ColorReset
	defer func() { ColorRed, ColorBold, ColorReset = origRed, origBold, origReset }()
	ColorRed, ColorBold, ColorReset = "", "", ""

	wtPath := "/tmp/my repo/wt with spaces"
	name := "my'name"
	err := errors.New("init failed")

	out := captureStderr(t, func() {
		PrintInitFailureBanner(wtPath, name, err)
	})

	// The path is wrapped in single quotes so the shell treats it as one token.
	if !strings.Contains(out, "cd '/tmp/my repo/wt with spaces' && wt init") {
		t.Errorf("retry hint must single-quote a spaces-containing path:\n%s", out)
	}
	// The name contains a single quote; shellQuoteSingle escapes it as '\''.
	if !strings.Contains(out, `wt delete 'my'\''name'`) {
		t.Errorf("remove hint must escape embedded single quotes in name:\n%s", out)
	}
}

func TestExitCodes(t *testing.T) {
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitGeneralError != 1 {
		t.Errorf("ExitGeneralError = %d, want 1", ExitGeneralError)
	}
	if ExitInvalidArgs != 2 {
		t.Errorf("ExitInvalidArgs = %d, want 2", ExitInvalidArgs)
	}
	if ExitGitError != 3 {
		t.Errorf("ExitGitError = %d, want 3", ExitGitError)
	}
	if ExitRetryExhausted != 4 {
		t.Errorf("ExitRetryExhausted = %d, want 4", ExitRetryExhausted)
	}
	if ExitByobuTabError != 5 {
		t.Errorf("ExitByobuTabError = %d, want 5", ExitByobuTabError)
	}
	if ExitTmuxWindowError != 6 {
		t.Errorf("ExitTmuxWindowError = %d, want 6", ExitTmuxWindowError)
	}
	if ExitInitFailed != 7 {
		t.Errorf("ExitInitFailed = %d, want 7", ExitInitFailed)
	}
}
