package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGo_NameArg_NavigatesToWorktree verifies the happy path: `wt go <name>`
// resolves a worktree and writes its absolute path to WT_CD_FILE while also
// printing it to stdout as the last line. No application is launched.
func TestGo_NameArg_NavigatesToWorktree(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "swift-fox")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWtSuccess(t, repo, env, "go", "swift-fox")

	// WT_CD_FILE holds the resolved worktree path.
	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
	// launcher-contract.md §3: mode 0600.
	info, err := os.Stat(cdFile)
	if err != nil {
		t.Fatalf("stat cd file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected cd file mode 0600, got %o", mode)
	}

	// stdout's last non-empty line is the resolved path (scripting form).
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	last := lines[len(lines)-1]
	if last != wtPath {
		t.Errorf("expected stdout last line %q, got %q (full stdout: %q)", wtPath, last, r.Stdout)
	}

	// No app launch leaked through (the test seam marker would appear).
	if strings.Contains(r.Stderr, "[wt-test-no-launch]") {
		t.Errorf("wt go must not launch an app, got stderr: %q", r.Stderr)
	}
}

// TestGo_NameArg_StderrConfirmation_StdoutStaysBarePath verifies the navigation
// confirmation block lands on STDERR (repo / worktree / branch + indented path)
// while STDOUT stays EXACTLY the bare resolved path — the critical regression
// guard for the stdout machine contract (cd "$(command wt go ...)").
func TestGo_NameArg_StderrConfirmation_StdoutStaysBarePath(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "frosted-jaguar")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, repo, env, "go", "frosted-jaguar")

	// STDOUT must be EXACTLY the bare path (single line, no confirmation text).
	if got := strings.TrimRight(r.Stdout, "\n"); got != wtPath {
		t.Errorf("stdout must be exactly the bare path %q, got %q", wtPath, got)
	}
	if strings.Contains(r.Stdout, "→") {
		t.Errorf("confirmation arrow must NOT appear on stdout, got: %q", r.Stdout)
	}

	// STDERR carries the compact-arrow confirmation block.
	assertContains(t, r.Stderr, "→")
	assertContains(t, r.Stderr, filepath.Base(repo)) // repo name
	assertContains(t, r.Stderr, "frosted-jaguar")    // worktree basename
	assertContains(t, r.Stderr, "frosted-jaguar)")   // branch (in parens; wt create names branch == worktree)
	assertContains(t, r.Stderr, wtPath)              // indented absolute path line
}

// TestGo_NoWorktrees_NoConfirmation verifies the confirmation block is NOT
// emitted when there is nothing to navigate to (no non-main worktrees) — the
// arrow must only appear on the success path.
func TestGo_NoWorktrees_NoConfirmation(t *testing.T) {
	repo := createTestRepo(t)

	// No extra worktrees created: selectWorktree finds zero options, prints
	// "No worktrees found." and returns without navigating.
	r := runWtSuccess(t, repo, nil, "go")

	assertContains(t, r.Stdout, "No worktrees found.")
	assertNotContains(t, r.Stdout, "→")
	assertNotContains(t, r.Stderr, "→")
}

// TestGo_NameArg_CaseInsensitive verifies name resolution is case-insensitive,
// matching resolveWorktreeByName's contract shared with `wt open`.
func TestGo_NameArg_CaseInsensitive(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "alpha")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	runWtSuccess(t, repo, env, "go", "ALPHA")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
}

// TestGo_UnknownName_ExitsGeneralError verifies an unresolved name exits
// ExitGeneralError (1) with a "not found" message — the worktree list
// succeeded, the name simply didn't match.
func TestGo_UnknownName_ExitsGeneralError(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "go", "no-such-worktree")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not found")
	assertContains(t, r.Stderr, "wt list")
}

// TestGo_NonGit_ExitsGitError verifies that running `wt go` (and `wt go
// <name>`) from a non-git cwd exits ExitGitError (3).
func TestGo_NonGit_ExitsGitError(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "go")
	if r.ExitCode != 3 {
		t.Fatalf("expected exit 3 (ExitGitError) for no-arg, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}

	r = runWt(t, dir, nil, "go", "some-name")
	if r.ExitCode != 3 {
		t.Fatalf("expected exit 3 (ExitGitError) for name-arg, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
}

// TestGo_NoArg_NonInteractive_ExitsGeneralError verifies that `wt go
// --non-interactive` with no name refuses deterministically (exit 1) rather
// than prompting — a no-arg selection menu has no non-interactive default.
func TestGo_NoArg_NonInteractive_ExitsGeneralError(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")

	r := runWt(t, repo, nil, "go", "--non-interactive")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "No worktree specified")
	// Must not have prompted (no menu rendered).
	assertNotContains(t, r.Stdout, "Select worktree")
}
