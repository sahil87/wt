package worktree

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureNavigateTo runs NavigateTo with stdout/stderr captured, returning
// (stdout, stderr, err). Env setup is the caller's job (t.Setenv).
func captureNavigateTo(t *testing.T, path, repoName, branch string) (string, string, error) {
	t.Helper()

	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW

	navErr := NavigateTo(path, repoName, branch)

	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	return string(outBytes), string(errBytes), navErr
}

// TestNavigateTo_WritesCdFileAndAlwaysPrintsPath pins the unified contract's
// happy path with WT_CD_FILE set: the file holds the path (mode 0600), stdout
// is exactly the bare path as the last (only) line, the stderr confirmation
// carries repo/basename/branch plus the indented path, and no wrapper hint is
// printed (WT_CD_FILE is set, so the wrapper is handling the cd).
func TestNavigateTo_WritesCdFileAndAlwaysPrintsPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "swift-fox")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	cdFile := filepath.Join(dir, "wt-cd")
	t.Setenv("WT_CD_FILE", cdFile)
	t.Setenv("WT_WRAPPER", "")

	stdout, stderr, err := captureNavigateTo(t, target, "myrepo", "feature-x")
	if err != nil {
		t.Fatalf("NavigateTo returned error: %v", err)
	}

	data, readErr := os.ReadFile(cdFile)
	if readErr != nil {
		t.Fatalf("reading cd file: %v", readErr)
	}
	if string(data) != target {
		t.Errorf("cd file = %q, want %q", string(data), target)
	}
	info, statErr := os.Stat(cdFile)
	if statErr != nil {
		t.Fatalf("stat cd file: %v", statErr)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("cd file mode = %o, want 0600", mode)
	}

	// stdout is exactly the bare path (the machine contract).
	if got := strings.TrimRight(stdout, "\n"); got != target {
		t.Errorf("stdout = %q, want exactly the bare path %q", got, target)
	}

	// stderr carries the confirmation, never the hint.
	if !strings.Contains(stderr, "→ myrepo / swift-fox  (feature-x)") {
		t.Errorf("stderr missing confirmation line, got: %q", stderr)
	}
	if !strings.Contains(stderr, "  "+target) {
		t.Errorf("stderr missing indented path line, got: %q", stderr)
	}
	if strings.Contains(stderr, "shell wrapper") {
		t.Errorf("hint must not print when WT_CD_FILE is set, got: %q", stderr)
	}
}

// TestNavigateTo_TruncatesExistingCdFile verifies truncate-on-write semantics:
// longer pre-existing content is fully replaced.
func TestNavigateTo_TruncatesExistingCdFile(t *testing.T) {
	dir := t.TempDir()
	cdFile := filepath.Join(dir, "wt-cd")
	longContent := strings.Repeat("x", 500)
	if err := os.WriteFile(cdFile, []byte(longContent), 0600); err != nil {
		t.Fatalf("seeding cd file: %v", err)
	}
	t.Setenv("WT_CD_FILE", cdFile)

	if _, _, err := captureNavigateTo(t, dir, "repo", "main"); err != nil {
		t.Fatalf("NavigateTo returned error: %v", err)
	}

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != dir {
		t.Errorf("cd file = %q, want %q (stale content must be truncated)", string(data), dir)
	}
}

// TestNavigateTo_NonGitDegrade verifies the confirmation's non-git form: with
// repoName empty the first line is "→ {basename}" — no slash, no branch parens.
func TestNavigateTo_NonGitDegrade(t *testing.T) {
	dir := t.TempDir()
	cdFile := filepath.Join(dir, "wt-cd")
	t.Setenv("WT_CD_FILE", cdFile)

	_, stderr, err := captureNavigateTo(t, dir, "", "unknown")
	if err != nil {
		t.Fatalf("NavigateTo returned error: %v", err)
	}

	wantLine := "→ " + filepath.Base(dir) + "\n"
	if !strings.Contains(stderr, wantLine) {
		t.Errorf("stderr missing degraded confirmation %q, got: %q", wantLine, stderr)
	}
	if strings.Contains(stderr, " / ") || strings.Contains(stderr, "(unknown)") {
		t.Errorf("degraded confirmation must carry no repo/branch fields, got: %q", stderr)
	}
}

// TestNavigateTo_HintGating verifies the WT_WRAPPER-gated hint: printed when
// neither WT_CD_FILE nor WT_WRAPPER=1 is present, suppressed under WT_WRAPPER=1.
func TestNavigateTo_HintGating(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("WT_CD_FILE", "")
	t.Setenv("WT_WRAPPER", "")
	stdout, stderr, err := captureNavigateTo(t, dir, "repo", "main")
	if err != nil {
		t.Fatalf("NavigateTo returned error: %v", err)
	}
	if !strings.Contains(stderr, "shell wrapper") || !strings.Contains(stderr, "wt shell-init zsh") {
		t.Errorf("expected wrapper hint on stderr, got: %q", stderr)
	}
	// The stdout machine contract holds on the hint path too.
	if got := strings.TrimRight(stdout, "\n"); got != dir {
		t.Errorf("stdout = %q, want %q", got, dir)
	}

	t.Setenv("WT_WRAPPER", "1")
	_, stderr, err = captureNavigateTo(t, dir, "repo", "main")
	if err != nil {
		t.Fatalf("NavigateTo returned error: %v", err)
	}
	if strings.Contains(stderr, "shell wrapper") {
		t.Errorf("WT_WRAPPER=1 must suppress the hint, got: %q", stderr)
	}
}

// TestNavigateTo_WriteFailureBeforeAnyOutput verifies the failure ordering: an
// unwritable WT_CD_FILE returns an error and emits NOTHING — no confirmation,
// no stdout path (a success message before a failed write would mislead).
func TestNavigateTo_WriteFailureBeforeAnyOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WT_CD_FILE", filepath.Join(dir, "no-such-subdir", "wt-cd"))

	stdout, stderr, err := captureNavigateTo(t, dir, "repo", "main")
	if err == nil {
		t.Fatal("expected an error for an unwritable WT_CD_FILE, got nil")
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on write failure, got: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr must be empty on write failure, got: %q", stderr)
	}
}
