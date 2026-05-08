// Package update implements `wt update` — self-upgrade via Homebrew.
//
// The brew formula is referenced by its fully-qualified name (sahil87/tap/wt)
// to avoid any ambiguity with same-named entries from other taps. This package
// uses os/exec directly per wt convention (no internal/proc wrapper); the
// constitution does not mandate centralized subprocess routing.
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
	"time"
)

// brewFormula is the fully-qualified tap formula. The fully-qualified form
// disambiguates against any same-named formula or cask in Homebrew core.
const brewFormula = "sahil87/tap/wt"

const (
	brewUpdateTimeout  = 30 * time.Second
	brewInfoTimeout    = 30 * time.Second
	brewUpgradeTimeout = 120 * time.Second
)

// ErrBrewNotFound is returned by Run when the `brew` executable cannot be
// located on PATH. The cobra wrapper in cmd/wt detects this sentinel and maps
// it to a typed exit so the user sees only the single hint Run prints to
// errOut (and not a duplicate cobra-formatted error line).
var ErrBrewNotFound = errors.New("brew not found on PATH")

// Run self-updates the wt binary via Homebrew.
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
func Run(currentVersion string, out, errOut io.Writer) error {
	if !isBrewInstalled() {
		fmt.Fprintf(out, "wt %s was not installed via Homebrew.\n", currentVersion)
		fmt.Fprintln(out, "Update manually, or reinstall with: brew install "+brewFormula)
		return nil
	}

	fmt.Fprintf(out, "Current version: %s\n", currentVersion)
	fmt.Fprintln(out, "Checking for updates...")

	ctx, cancel := context.WithTimeout(context.Background(), brewUpdateTimeout)
	cmd := exec.CommandContext(ctx, "brew", "update", "--quiet")
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

	upCtx, upCancel := context.WithTimeout(context.Background(), brewUpgradeTimeout)
	defer upCancel()
	upCmd := exec.CommandContext(upCtx, "brew", "upgrade", brewFormula)
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
	cmd := exec.CommandContext(ctx, "brew", "info", "--json=v2", brewFormula)
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
func isBrewInstalled() bool {
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
