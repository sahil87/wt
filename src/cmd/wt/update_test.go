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

// ---------- Intuitive flag names (change 59u8) ----------

// TestUpdate_NoBrewUpdateFlagInHelp asserts the new --no-brew-update flag is
// registered and visible in `wt update --help`, while the deprecated
// --skip-brew-update alias is hidden.
func TestUpdate_NoBrewUpdateFlagInHelp(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --help failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "--no-brew-update") {
		t.Fatalf("expected `--no-brew-update` in `wt update --help`, got:\n%s", r.Stdout)
	}
	if strings.Contains(r.Stdout, "--skip-brew-update") {
		t.Fatalf("expected deprecated `--skip-brew-update` to be hidden from `wt update --help`, got:\n%s", r.Stdout)
	}
}

// TestUpdate_NoBrewUpdateFlagAccepted asserts `wt update --no-brew-update` is
// parsed and threaded into internal/update.Run (reaching the non-brew
// short-circuit on the go-test binary, which never lives under /Cellar/).
func TestUpdate_NoBrewUpdateFlagAccepted(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--no-brew-update")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --no-brew-update failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	if !strings.Contains(r.Stdout, "was not installed via Homebrew") {
		t.Fatalf("expected non-brew hint in stdout (flag accepted, Run reached), got:\n%s", r.Stdout)
	}
	if strings.Contains(r.Stderr, "deprecated") {
		t.Fatalf("new --no-brew-update should not emit a deprecation warning, stderr:\n%s", r.Stderr)
	}
}

// TestUpdate_SkipBrewUpdateDeprecated asserts the deprecated --skip-brew-update
// alias is still accepted and emits a stderr deprecation warning naming the new
// flag.
func TestUpdate_SkipBrewUpdateDeprecated(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--skip-brew-update")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --skip-brew-update failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	if !strings.Contains(r.Stderr, "deprecated") {
		t.Fatalf("expected deprecation warning on stderr for --skip-brew-update, got:\n%s", r.Stderr)
	}
}
