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

// ---------- Toolkit update-standard flag surface (change 32su) ----------

// TestUpdate_HelpContainsBothFlags asserts both flags are registered and
// visible in `wt update --help`. The literal substring `--skip-brew-update`
// is a frozen textual contract from the toolkit update standard: shll
// discovers the flag with a strings.Contains probe on the help text before
// every toolkit-wide run, so this assertion mirrors that probe exactly.
// --no-brew-update stays visible as the alias.
func TestUpdate_HelpContainsBothFlags(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --help failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	// The exact probe shll runs (strings.Contains on the help text).
	if !strings.Contains(r.Stdout, "--skip-brew-update") {
		t.Fatalf("expected literal `--skip-brew-update` in `wt update --help` (toolkit contract substring), got:\n%s", r.Stdout)
	}
	if !strings.Contains(r.Stdout, "--no-brew-update") {
		t.Fatalf("expected `--no-brew-update` in `wt update --help`, got:\n%s", r.Stdout)
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

// TestUpdate_SkipBrewUpdateNoDeprecationWarning asserts the contract flag is
// accepted WITHOUT any deprecation warning: shll passes --skip-brew-update on
// every toolkit-wide run, so a per-run stderr warning on the standard's own
// flag would be noise (the flag was previously MarkDeprecated'd; that is
// undone by the update standard's frozen-substring contract).
func TestUpdate_SkipBrewUpdateNoDeprecationWarning(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "update", "--skip-brew-update")
	if r.ExitCode != 0 {
		t.Fatalf("wt update --skip-brew-update failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	if strings.Contains(r.Stderr, "deprecated") {
		t.Fatalf("--skip-brew-update must not emit a deprecation warning (toolkit contract flag), stderr:\n%s", r.Stderr)
	}
}
