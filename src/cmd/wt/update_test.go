package main

import (
	"strings"
	"testing"
)

// TestUpdate_NonBrewBranch asserts that `wt update` is registered, accepts no
// args, and reaches the internal/update.Run code path. We exploit the fact
// that `go test` binaries do not live under /Cellar/, so the function
// short-circuits to the "not installed via Homebrew" branch — exercising the
// cobra plumbing without hitting brew.
func TestUpdate_NonBrewBranch(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update")
	if r.ExitCode != 0 {
		t.Fatalf("wt update on non-brew binary failed (exit %d)\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "was not installed via Homebrew") {
		t.Fatalf("expected non-brew hint in stdout, got:\n%s", r.Stdout)
	}
}

// TestUpdate_RejectsArgs verifies cobra.NoArgs enforcement on the update
// subcommand: passing an extra positional arg surfaces a non-zero exit via
// main.go's error path (runWt invokes the built binary as a subprocess).
func TestUpdate_RejectsArgs(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "extra")
	if r.ExitCode == 0 {
		t.Fatalf("expected non-zero exit from `wt update extra` (cobra.NoArgs)\nstdout: %s\nstderr: %s",
			r.Stdout, r.Stderr)
	}
}

// TestUpdate_AppearsInHelp asserts the new subcommand is visible in `wt --help`.
func TestUpdate_AppearsInHelp(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt --help failed (exit %d)\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "update") {
		t.Fatalf("expected `update` in --help output, got:\n%s", r.Stdout)
	}
}

// TestUpdate_SkipBrewUpdateFlagInHelp asserts the --skip-brew-update flag is
// registered on the subcommand and visible in `wt update --help`. This guards
// the cobra plumbing end-to-end — a misspelled flag name or a flag that was
// never registered would be caught here, whereas the internal/update.Run tests
// exercise Run directly and cannot see the cobra wiring.
func TestUpdate_SkipBrewUpdateFlagInHelp(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --help failed (exit %d)\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "--skip-brew-update") {
		t.Fatalf("expected `--skip-brew-update` in `wt update --help` output, got:\n%s", r.Stdout)
	}
}

// TestUpdate_SkipBrewUpdateFlagAccepted asserts that `wt update
// --skip-brew-update` is parsed and accepted by cobra (no unknown-flag error),
// reaching the internal/update.Run code path. As with TestUpdate_NonBrewBranch,
// the `go test` binary does not live under /Cellar/, so Run short-circuits to
// the "not installed via Homebrew" branch — confirming the parsed flag value is
// threaded into Run without hitting brew.
func TestUpdate_SkipBrewUpdateFlagAccepted(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--skip-brew-update")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --skip-brew-update failed (exit %d)\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "was not installed via Homebrew") {
		t.Fatalf("expected non-brew hint in stdout (flag accepted, Run reached), got:\n%s", r.Stdout)
	}
}
