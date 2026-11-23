package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installGhStub puts a fake `gh` first on PATH that prints stdout / stderr,
// exits with code, and records its argv (one per line) to the returned file.
func installGhStub(t *testing.T, stdout, stderr string, code int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gh stub is a POSIX shell script")
	}
	dir := t.TempDir()
	for name, content := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	argsFile := filepath.Join(dir, "args")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > '%s'\ncat '%s'\ncat '%s' >&2\nexit %d\n",
		argsFile, filepath.Join(dir, "stdout"), filepath.Join(dir, "stderr"), code)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func TestResolvePRHead_SameRepository(t *testing.T) {
	argsFile := installGhStub(t, `{"headRefName":"feature/x","isCrossRepository":false}`, "", 0)

	head, err := ResolvePRHead("42")
	if err != nil {
		t.Fatalf("ResolvePRHead: %v", err)
	}
	if head.Branch != "feature/x" || head.CrossRepository {
		t.Errorf("head = %+v, want {Branch: feature/x, CrossRepository: false}", head)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile args: %v", err)
	}
	want := "pr\nview\n42\n--json\nheadRefName,isCrossRepository\n"
	if string(args) != want {
		t.Errorf("gh argv = %q, want %q", args, want)
	}
}

func TestResolvePRHead_CrossRepository(t *testing.T) {
	installGhStub(t, `{"headRefName":"patch-1","isCrossRepository":true}`, "", 0)

	head, err := ResolvePRHead("7")
	if !errors.Is(err, ErrPRCrossRepository) {
		t.Fatalf("err = %v, want ErrPRCrossRepository", err)
	}
	if head.Branch != "patch-1" {
		t.Errorf("head.Branch = %q, want patch-1 (named in the fork error)", head.Branch)
	}
}

func TestResolvePRHead_GhFailureReturnsFirstStderrLine(t *testing.T) {
	installGhStub(t, "", "\nno pull requests found for branch \"99\"\nsecond line\n", 1)

	_, err := ResolvePRHead("99")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got, want := err.Error(), `no pull requests found for branch "99"`; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}

func TestResolvePRHead_MalformedOutput(t *testing.T) {
	for name, stdout := range map[string]string{
		"not json":   "not json",
		"empty head": `{"headRefName":"","isCrossRepository":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			installGhStub(t, stdout, "", 0)
			_, err := ResolvePRHead("1")
			if err == nil || !strings.Contains(err.Error(), "unexpected gh output") {
				t.Errorf("err = %v, want an unexpected-gh-output error", err)
			}
		})
	}
}

func TestResolvePRHead_GhMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := ResolvePRHead("1")
	if !errors.Is(err, ErrGhNotFound) {
		t.Errorf("err = %v, want ErrGhNotFound", err)
	}
}
