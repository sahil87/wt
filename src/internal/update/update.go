// Package update implements `wt update` — self-upgrade via Homebrew.
//
// The brew formula is referenced by its fully-qualified name (sahil87/tap/wt)
// to avoid any ambiguity with same-named entries from other taps. This package
// uses os/exec directly (no internal/proc wrapper) — consistent with the rest
// of the wt codebase, which has no centralized subprocess routing.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// brewFormula is the fully-qualified tap formula. The fully-qualified form
// disambiguates against any same-named formula or cask in Homebrew core.
const brewFormula = "sahil87/tap/wt"

// Bounds for the brew metadata calls. Per the toolkit update standard's
// brew-handling clause, any bound must be generous (sized for a network
// transfer, not a local command) and terminate gracefully — SIGTERM plus a
// grace period, never SIGKILL. `brew upgrade` is deliberately unbounded (see
// Run): brew can legitimately block for minutes, the call is interactive and
// stream-inheriting, and Ctrl-C (SIGINT, which brew traps and unwinds)
// remains the user's escape.
const (
	brewUpdateTimeout = 5 * time.Minute
	brewInfoTimeout   = 60 * time.Second
	// brewGraceDelay is how long a bounded brew subprocess gets to unwind
	// after the graceful SIGTERM before the runtime force-kills it.
	brewGraceDelay = 10 * time.Second
)

// newBoundedBrewCmd builds a context-bounded brew command that terminates
// GRACEFULLY on expiry: ctx cancellation sends SIGTERM (trappable — brew can
// finish or roll back its transaction) instead of exec.CommandContext's
// default SIGKILL, and WaitDelay grants a grace period before the runtime's
// forced kill. Every bounded brew call site MUST go through this helper so no
// code path can SIGKILL a package-manager subprocess mid-transaction (the
// toolkit update standard's brew-handling MUST).
func newBoundedBrewCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "brew", args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = brewGraceDelay
	return cmd
}

// ErrBrewNotFound is returned by Run when the `brew` executable cannot be
// located on PATH. The cobra wrapper in cmd/wt detects this sentinel and maps
// it to a typed exit so the user sees only the single hint Run prints to
// errOut (and not a duplicate cobra-formatted error line).
var ErrBrewNotFound = errors.New("brew not found on PATH")

// Run self-updates the wt binary via Homebrew.
//
// skipBrewUpdate, when true, skips ONLY the internal `brew update` tap-metadata
// refresh. Everything else is unchanged: the `brew info` version check, the
// up-to-date short-circuit, and the interactive `brew upgrade` all run exactly
// as in the default path. When false (the default, flag absent) Run behaves as
// it did before this flag existed.
//
// currentVersion is the binary's reported version (e.g. "v0.1.0"). The leading
// "v" is stripped before comparison since `brew info` reports the bare form.
//
// out and errOut receive only the WRAPPER messages this package emits ("Current
// version:", "Already up to date", error hints, etc.). Subprocess stdout/stderr
// from `brew update`, `brew info`, and `brew upgrade` is intentionally NOT
// routed through these writers — `brew update` and `brew info` capture stdout
// for parsing/discard while passing stderr through to os.Stderr; `brew upgrade`
// inherits all three streams (os.Stdin, os.Stdout, os.Stderr) so brew's
// tty-aware progress output renders inline. Callers in production should pass
// os.Stdout / os.Stderr to keep the wrapper messages consistent with the
// subprocess streams.
//
// Returns nil on success or no-op (not a brew install, already up to date).
// Returns ErrBrewNotFound when brew is missing on PATH (callers should map
// this to a typed exit so cobra does not double-print). Returns a wrapped
// error for other brew failures.
func Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer) error {
	if !isBrewInstalled() {
		fmt.Fprintf(out, "wt %s was not installed via Homebrew.\n", currentVersion)
		fmt.Fprintln(out, "Update manually, or reinstall with: brew install "+brewFormula)
		return nil
	}

	fmt.Fprintf(out, "Current version: %s\n", currentVersion)
	fmt.Fprintln(out, "Checking for updates...")

	if !skipBrewUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), brewUpdateTimeout)
		cmd := newBoundedBrewCmd(ctx, "update", "--quiet")
		cmd.Stderr = os.Stderr
		_, err := cmd.Output()
		cancel()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				fmt.Fprintln(errOut, "wt update: brew not found on PATH.")
				return ErrBrewNotFound
			}
			return fmt.Errorf("brew update failed: %w", err)
		}
	}

	latest, err := brewLatestVersion()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintln(errOut, "wt update: brew not found on PATH.")
			return ErrBrewNotFound
		}
		return fmt.Errorf("could not determine latest version: %w", err)
	}

	if normalizeVersion(latest) == normalizeVersion(currentVersion) {
		fmt.Fprintf(out, "Already up to date (%s).\n", currentVersion)
		return nil
	}

	fmt.Fprintf(out, "Updating %s → v%s...\n", currentVersion, normalizeVersion(latest))

	// No timeout on `brew upgrade` — the standard forbids a short hard bound
	// (an un-timed GitHub API call inside brew can block for minutes), and a
	// kill landing between `brew unlink` and `brew link` corrupts the keg.
	// The call inherits the user's streams; Ctrl-C (SIGINT) is the escape.
	upCmd := exec.Command("brew", "upgrade", brewFormula)
	upCmd.Stdin = os.Stdin
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintln(errOut, "wt update: brew not found on PATH.")
			return ErrBrewNotFound
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("brew upgrade exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("brew upgrade failed: %w", err)
	}

	fmt.Fprintf(out, "Updated to v%s.\n", normalizeVersion(latest))
	return nil
}

// brewLatestVersion queries Homebrew for the latest stable version of the
// tap formula. Returns the bare version string (e.g. "0.1.1") with no `v`
// prefix — that's how brew reports it in `versions.stable`.
func brewLatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), brewInfoTimeout)
	defer cancel()
	cmd := newBoundedBrewCmd(ctx, "info", "--json=v2", brewFormula)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var info struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	if len(info.Formulae) == 0 || info.Formulae[0].Versions.Stable == "" {
		return "", errors.New("no stable version found in brew info output")
	}
	return info.Formulae[0].Versions.Stable, nil
}

// isBrewInstalled checks whether the running binary lives under a Cellar
// directory, which is the canonical signature of a Homebrew install. The
// symlink at /opt/homebrew/bin/wt (or /usr/local/bin/wt on Intel) resolves
// through to .../Cellar/wt/<version>/bin/wt.
//
// Test seam: when WT_TEST_FORCE_BREW=1 is set in the environment, this returns
// true unconditionally so tests can exercise the brew code paths without a real
// Homebrew install (the `go test` binary never lives under /Cellar/). Mirrors
// the WT_TEST_NO_LAUNCH=1 seam at internal/worktree/apps.go. The env var is
// never set in production, so production behavior is unchanged.
func isBrewInstalled() bool {
	if os.Getenv("WT_TEST_FORCE_BREW") == "1" {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(self)
	if err != nil {
		return false
	}
	return strings.Contains(real, "/Cellar/")
}

// normalizeVersion strips a single leading "v" so we can compare the binary's
// `git describe`-derived version (e.g. "v0.1.0") against brew's bare report
// ("0.1.0"). It does NOT do semver parsing — string equality after normalize
// is sufficient because both sides come from the same canonical source (the
// release tag).
func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
