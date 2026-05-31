package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// Exit codes matching the bash wt scripts.
const (
	ExitSuccess         = 0
	ExitGeneralError    = 1
	ExitInvalidArgs     = 2
	ExitGitError        = 3
	ExitRetryExhausted  = 4
	ExitByobuTabError   = 5
	ExitTmuxWindowError = 6
	ExitInitFailed      = 7
)

// ANSI color codes, disabled when NO_COLOR is set.
var (
	ColorRed    = "\033[0;31m"
	ColorYellow = "\033[0;33m"
	ColorGreen  = "\033[0;32m"
	ColorBold   = "\033[1m"
	ColorReset  = "\033[0m"
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

// phaseSeparatorWidth is the fixed total visible width (label + glyphs +
// spaces) of a phase separator line. The separator does NOT query the
// terminal size — a fixed width keeps output deterministic for tests and is
// consistent with the single-binary / no-hidden-state posture.
const phaseSeparatorWidth = 40

// PhaseSeparator returns a labeled rule line for stderr (no trailing newline —
// callers add it via the Fprintln they already use). It frames a bold label
// with a fixed-width rule so phase boundaries (Git, Init, Open) are
// attributable in captured logs.
//
// Layout: two leading glyphs, a space, the label, a space, then enough
// trailing glyphs to fill phaseSeparatorWidth VISIBLE columns (ANSI escapes
// around the label are not counted). When color is enabled the rule uses the
// unicode box-drawing glyph and the label is wrapped in ColorBold:
//
//	── Git ──────────────────────────────
//
// When NO_COLOR is set, the package init() blanks ColorBold/ColorReset; the
// separator detects that (via the blanked vars, not a fresh os.Getenv) and
// emits a plain-ASCII rule with no ANSI escapes:
//
//	-- Git ------------------------------
func PhaseSeparator(label string) string {
	colorEnabled := ColorReset != ""

	glyph := "-"
	if colorEnabled {
		glyph = "─" // ─ U+2500 BOX DRAWINGS LIGHT HORIZONTAL
	}

	const leading = 2
	// Visible width = leading glyphs + space + label + space + trailing glyphs.
	// Count the label in runes (visible columns), not bytes, so a label with
	// multi-byte runes (e.g. a non-ASCII WORKTREE_INIT_SCRIPT path) still
	// renders at the fixed visible width.
	trailing := phaseSeparatorWidth - leading - 1 - utf8.RuneCountInString(label) - 1
	if trailing < 0 {
		trailing = 0
	}

	renderedLabel := label
	if colorEnabled {
		renderedLabel = ColorBold + label + ColorReset
	}

	return fmt.Sprintf("%s %s %s",
		strings.Repeat(glyph, leading),
		renderedLabel,
		strings.Repeat(glyph, trailing))
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
	// Single-quote both path and name so the hints stay correct when they
	// contain spaces or shell metacharacters (e.g. macOS dirs with spaces).
	fmt.Fprintf(os.Stderr, "  %s%s%s cd '%s' && wt init\n",
		ColorBold, bannerLabelRetry, ColorReset, shellQuoteSingle(wtPath))
	fmt.Fprintf(os.Stderr, "  %s%s%s wt delete '%s'\n",
		ColorBold, bannerLabelRemove, ColorReset, shellQuoteSingle(name))
}
