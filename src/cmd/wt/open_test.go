package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpen_ErrorNonexistentWorktree(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--app", "code", "nonexistent-wt")
	if r.ExitCode == 0 {
		t.Error("expected failure for nonexistent worktree")
	}
	assertContains(t, r.Stderr, "not found")
}

func TestOpen_ErrorFromMainRepoWithoutTarget(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--app", "code")
	if r.ExitCode == 0 {
		t.Error("expected failure from main repo without target")
	}
	assertContains(t, r.Stderr, "No worktree specified")
}

// TestOpen_NoArgs_NonGit_OpensCwd verifies that running `wt open` from a
// non-git directory opens the current working directory (no longer fails
// with ExitGitError as it did pre-change).
func TestOpen_NoArgs_NonGit_OpensCwd(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	cdFile := filepath.Join(dir, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWt(t, dir, env, "open", "--app", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != dir {
		t.Errorf("expected cd file to contain %q, got %q", dir, string(data))
	}
}

// TestOpen_PathArg_NonGit_OpensPath verifies that a path arg works from a
// non-git cwd — the path may be unrelated to any repo.
func TestOpen_PathArg_NonGit_OpensPath(t *testing.T) {
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks cwd: %v", err)
	}
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks target: %v", err)
	}

	cdFile := filepath.Join(cwd, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWt(t, cwd, env, "open", target, "--app", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != target {
		t.Errorf("expected cd file to contain %q, got %q", target, string(data))
	}
}

// TestOpen_NameArg_NonGit_FailsWithGuidance verifies the spec-mandated error
// path: name args from a non-git cwd exit ExitGeneralError (1) with a clear
// message that suggests passing a path and does NOT suggest cd'ing.
func TestOpen_NameArg_NonGit_FailsWithGuidance(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "swift-fox")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "Cannot open 'swift-fox'")
	assertContains(t, r.Stderr, "name resolution requires a git repository")
	assertContains(t, r.Stderr, "wt open /absolute/path/to/dir")

	// Must NOT suggest cd'ing into a git repo (per spec requirement).
	if strings.Contains(r.Stderr, "Navigate to a git repository") {
		t.Errorf("error message should not suggest cd'ing into a git repo, got: %s", r.Stderr)
	}
	if strings.Contains(strings.ToLower(r.Stderr), "cd into") {
		t.Errorf("error message should not suggest cd'ing into a git repo, got: %s", r.Stderr)
	}
	if strings.Contains(strings.ToLower(r.Stderr), "run from a git repo") {
		t.Errorf("error message should not suggest running from a git repo, got: %s", r.Stderr)
	}
}

// TestOpen_NameArg_NotFound_InRepo verifies that asking for an unknown
// worktree name from inside a git repo exits ExitGeneralError (1, not
// ExitGitError) — the worktree list succeeded, the name simply didn't match.
// This pins the sentinel-error path that distinguishes "not found" (general
// error) from "git worktree list failed" (git error) per launcher-contract.md.
func TestOpen_NameArg_NotFound_InRepo(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "no-such-worktree")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not found")
}

// TestOpen_PathArg_ExistsButNotDir verifies that passing an arg that exists
// on disk but is not a directory (e.g., a regular file) fails with a clear
// "not a directory" message — not the misleading "name resolution requires a
// git repository" error that would otherwise apply to non-existent args from
// a non-git cwd.
func TestOpen_PathArg_ExistsButNotDir(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "regular-file.txt")
	if err := os.WriteFile(filePath, []byte("hi"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := runWt(t, cwd, nil, "open", filePath)
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not a directory")
	// Must NOT fall through to the name-resolution error path.
	if strings.Contains(r.Stderr, "name resolution requires a git repository") {
		t.Errorf("file-arg error must not surface the name-resolution message, got: %s", r.Stderr)
	}
}

func TestOpen_ErrorUnknownApp(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "app-err")

	r := runWt(t, repo, nil, "open", "--app", "nonexistent-app", wtPath)
	if r.ExitCode == 0 {
		t.Error("expected failure for unknown app")
	}
	assertContains(t, r.Stderr, "Unknown app")
}

func TestOpen_AppDefault(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "default-test")

	// Clear environment to control detection path
	env := []string{
		"TERM_PROGRAM=",
		"TMUX=",
		"BYOBU_BACKEND=",
		"BYOBU_TTY=",
		"BYOBU_SESSION=",
		"BYOBU_CONFIG_DIR=",
		"HOME=" + t.TempDir(),
	}

	r := runWt(t, repo, env, "open", "--app", "default", wtPath)
	// Installed apps vary across environments (e.g., macOS always has Finder).
	// Accept either outcome, but verify the "default" keyword was recognized:
	// - exit 0: some default app resolved and reached OpenInApp (under the
	//   WT_TEST_NO_LAUNCH=1 guard, no real launch happens)
	// - non-zero: no default detected — should show our error, not "Unknown app"
	if r.ExitCode == 0 {
		// A resolved default app MUST go through OpenInApp; the test launch
		// guard emits the marker. Missing marker = a real launch leaked past
		// the seam.
		assertContains(t, r.Stderr, "[wt-test-no-launch]")
	} else {
		assertContains(t, r.Stderr, "No default app detected")
	}
	// "default" must never be treated as a literal app name
	if strings.Contains(r.Stderr, "Unknown app: default") {
		t.Errorf("'default' was treated as a literal app name instead of the keyword: %s", r.Stderr)
	}
	if strings.Contains(r.Stderr, "panic") {
		t.Errorf("command panicked: %s", r.Stderr)
	}
}

// NOTE: Testing actual app opening (code, cursor, etc.) requires mock binaries
// on PATH that log their invocations. We test the error paths here; the
// open-by-name success path is tested via the worktree resolution logic
// (which is shared with other commands).
