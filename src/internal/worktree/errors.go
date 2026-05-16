package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Exit codes matching the bash wt scripts.
const (
	ExitSuccess          = 0
	ExitGeneralError     = 1
	ExitInvalidArgs      = 2
	ExitGitError         = 3
	ExitRetryExhausted   = 4
	ExitByobuTabError    = 5
	ExitTmuxWindowError  = 6
	ExitInitFailed       = 7
)

// ANSI color codes, disabled when NO_COLOR is set.
var (
	ColorRed   = "\033[0;31m"
	ColorYellow = "\033[0;33m"
	ColorGreen = "\033[0;32m"
	ColorBold  = "\033[1m"
	ColorReset = "\033[0m"
)

func init() {
	if os.Getenv("NO_COLOR") != "" {
		ColorRed = ""
		ColorYellow = ""
		ColorGreen = ""
		ColorBold = ""
		ColorReset = ""
	}
}

// WtError formats a structured error message and writes it to stderr.
// Format: "Error: {what}\n  Why: {why}\n  Fix: {fix}"
func WtError(what, why, fix string) string {
	msg := fmt.Sprintf("%sError:%s %s\n  %sWhy:%s %s",
		ColorRed, ColorReset, what,
		ColorBold, ColorReset, why)
	if fix != "" {
		msg += fmt.Sprintf("\n  %sFix:%s %s", ColorBold, ColorReset, fix)
	}
	return msg
}

// PrintError writes a structured error to stderr.
func PrintError(what, why, fix string) {
	fmt.Fprintln(os.Stderr, WtError(what, why, fix))
}

// ExitWithError prints a structured error and exits with the given code.
func ExitWithError(code int, what, why, fix string) {
	PrintError(what, why, fix)
	os.Exit(code)
}

// Banner labels for PrintInitFailureBanner. Named so the canonical wording
// lives in one place and tests can assert against shape, not byte equality.
const (
	bannerLabelWorktree = "Worktree:"
	bannerLabelRetry    = "Retry:   "
	bannerLabelRemove   = "Remove:  "
	bannerKeptNote      = "(kept — git operations succeeded)"
	bannerOutputNote    = "(init output is above)"
)

// PrintInitFailureBanner writes a structured init-failure banner to stderr.
// The banner reports the failure shape (status code when err unwraps to
// *exec.ExitError, otherwise a generic phrasing), the absolute worktree path
// that was kept, a copy-paste-ready retry command, and a remove hint.
//
// Banner order:
//  1. Status line ("init script exited with status N" / "did not complete")
//     + reminder that the init output streamed above.
//  2. Worktree: <absolute path>  (kept — git operations succeeded)
//  3. Retry:    cd <wtPath> && wt init
//  4. Remove:   wt delete <name>
//
// `&&` (never `;`) is used in the retry hint so it parses identically in
// bash, zsh, and fish.
func PrintInitFailureBanner(wtPath, name string, err error) {
	var statusLine string
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		statusLine = fmt.Sprintf("%sError:%s init script exited with status %d %s",
			ColorRed, ColorReset, exitErr.ExitCode(), bannerOutputNote)
	} else {
		statusLine = fmt.Sprintf("%sError:%s init script did not complete %s",
			ColorRed, ColorReset, bannerOutputNote)
	}

	fmt.Fprintln(os.Stderr, statusLine)
	fmt.Fprintf(os.Stderr, "  %s%s%s %s  %s\n",
		ColorBold, bannerLabelWorktree, ColorReset, wtPath, bannerKeptNote)
	fmt.Fprintf(os.Stderr, "  %s%s%s cd %s && wt init\n",
		ColorBold, bannerLabelRetry, ColorReset, wtPath)
	fmt.Fprintf(os.Stderr, "  %s%s%s wt delete %s\n",
		ColorBold, bannerLabelRemove, ColorReset, name)
}
