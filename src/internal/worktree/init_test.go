package worktree

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// randomMissingCommand returns a random byte-suffixed command name unlikely
// to exist on PATH. Used so the CommandNotOnPath branch is not contaminated
// by environment differences (e.g., a dev box that has "fab" installed).
func randomMissingCommand(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return "__wt_missing_" + hex.EncodeToString(buf) + "__"
}

func TestResolveInitInvocation_CommandOnPath(t *testing.T) {
	// "true" is on POSIX systems; resolver should construct a runnable cmd.
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not on PATH; skipping")
	}
	cmd, notFound, err := ResolveInitInvocation("true ignored-arg", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Fatalf("expected nil InitNotFound, got %+v", notFound)
	}
	if cmd == nil {
		t.Fatal("expected non-nil *exec.Cmd")
	}
	if cmd.Dir != "" {
		t.Errorf("expected cmd.Dir empty (callers wire it), got %q", cmd.Dir)
	}
	// The Setpgid assertion lives in init_unix_test.go (build !windows) —
	// syscall.SysProcAttr does not expose Setpgid on Windows, so the
	// assertion cannot live in a cross-platform file.
	assertInitProcessGroupSet(t, cmd)
}

func TestResolveInitInvocation_CommandNotOnPath(t *testing.T) {
	name := randomMissingCommand(t)
	cmd, notFound, err := ResolveInitInvocation(name+" sync", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %+v", cmd)
	}
	if notFound == nil {
		t.Fatal("expected non-nil InitNotFound")
	}
	if notFound.Kind != CommandNotOnPath {
		t.Errorf("expected Kind=CommandNotOnPath, got %v", notFound.Kind)
	}
	if notFound.Name != name {
		t.Errorf("expected Name=%q, got %q", name, notFound.Name)
	}
}

func TestResolveInitInvocation_FileExists(t *testing.T) {
	repoRoot := t.TempDir()
	rel := "scripts/init.sh"
	abs := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd, notFound, err := ResolveInitInvocation(rel, repoRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Fatalf("expected nil InitNotFound, got %+v", notFound)
	}
	if cmd == nil {
		t.Fatal("expected non-nil *exec.Cmd")
	}
	// The first arg should be the bash interpreter; the resolved script
	// path should appear in Args.
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, abs) {
		t.Errorf("expected cmd.Args to reference %q, got %q", abs, joined)
	}
}

func TestResolveInitInvocation_FileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	rel := "scripts/missing.sh"
	cmd, notFound, err := ResolveInitInvocation(rel, repoRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %+v", cmd)
	}
	if notFound == nil {
		t.Fatal("expected non-nil InitNotFound")
	}
	if notFound.Kind != FileNotFound {
		t.Errorf("expected Kind=FileNotFound, got %v", notFound.Kind)
	}
	if notFound.RelPath != rel {
		t.Errorf("expected RelPath=%q, got %q", rel, notFound.RelPath)
	}
	expectedAbs := filepath.Join(repoRoot, rel)
	if notFound.Path != expectedAbs {
		t.Errorf("expected Path=%q, got %q", expectedAbs, notFound.Path)
	}
}

// exitErrorWithCode runs a trivial command that exits with the given code and
// returns the resulting error, which unwraps to an *exec.ExitError carrying
// that code. This produces a real exit error (as cmd.Run does at the run sites)
// rather than a hand-forged one, so DefaultNotApplicable is exercised through
// the same errors.As path it uses in production.
func exitErrorWithCode(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("bash", "-c", "exit "+strconv.Itoa(code)).Run()
	if err == nil {
		t.Fatalf("expected non-nil error for exit %d", code)
	}
	// Confirm the fixture actually carries the code we asked for.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
	if exitErr.ExitCode() != code {
		t.Fatalf("fixture exit code = %d, want %d", exitErr.ExitCode(), code)
	}
	return err
}

func TestDefaultNotApplicable(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("`bash` not on PATH; skipping")
	}
	exit3 := exitErrorWithCode(t, 3)
	exit1 := exitErrorWithCode(t, 1)

	tests := []struct {
		name      string
		err       error
		isDefault bool
		want      bool
	}{
		{"default + exit 3 → skip", exit3, true, true},
		{"explicit + exit 3 → no skip (provenance)", exit3, false, false},
		{"default + exit 1 → no skip (only 3)", exit1, true, false},
		{"default + nil error → no skip", nil, true, false},
		{"default + non-ExitError → no skip", errors.New("boom"), true, false},
		{"explicit + exit 1 → no skip", exit1, false, false},
		{"explicit + nil error → no skip", nil, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultNotApplicable(tc.err, tc.isDefault); got != tc.want {
				t.Errorf("DefaultNotApplicable(%v, %v) = %v, want %v",
					tc.err, tc.isDefault, got, tc.want)
			}
		})
	}
}

func TestRenderDefaultSkipWarning(t *testing.T) {
	out := RenderDefaultSkipWarning()
	if out == "" {
		t.Fatal("expected non-empty warning")
	}
	for _, want := range []string{
		"not a fab-managed repo",
		"skipping init",
		"WORKTREE_INIT_SCRIPT",
		"fab init",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected skip warning to contain %q, got:\n%s", want, out)
		}
	}
	// Two lines: the skip statement and the escape-hatch hint.
	if got := strings.Count(out, "\n"); got != 1 {
		t.Errorf("expected exactly one newline (two lines), got %d:\n%s", got, out)
	}
}

func TestInitNotFound_RenderWarning_CommandNotOnPath(t *testing.T) {
	n := InitNotFound{Kind: CommandNotOnPath, Name: "fab"}
	out := n.RenderWarning()
	if out == "" {
		t.Fatal("expected non-empty warning")
	}
	if !strings.Contains(out, "fab") {
		t.Errorf("expected warning to mention command name, got: %s", out)
	}
	if !strings.Contains(out, "PATH") {
		t.Errorf("expected warning to mention PATH, got: %s", out)
	}
	if !strings.Contains(out, "WORKTREE_INIT_SCRIPT") {
		t.Errorf("expected warning to mention WORKTREE_INIT_SCRIPT, got: %s", out)
	}
}

func TestInitNotFound_RenderWarning_FileNotFound(t *testing.T) {
	n := InitNotFound{
		Kind:    FileNotFound,
		Path:    "/tmp/repo/scripts/init.sh",
		RelPath: "scripts/init.sh",
	}
	out := n.RenderWarning()
	if out == "" {
		t.Fatal("expected non-empty warning")
	}
	if !strings.Contains(out, "/tmp/repo/scripts/init.sh") {
		t.Errorf("expected warning to include absolute Path, got: %s", out)
	}
	if !strings.Contains(out, "scripts/init.sh") {
		t.Errorf("expected warning to include RelPath, got: %s", out)
	}
	if !strings.Contains(out, "WORKTREE_INIT_SCRIPT") {
		t.Errorf("expected warning to mention WORKTREE_INIT_SCRIPT, got: %s", out)
	}
}
