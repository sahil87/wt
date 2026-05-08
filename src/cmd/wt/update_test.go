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
