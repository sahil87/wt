package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestList_ShowsRepoNameAndLocation(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "Worktrees for:")
	assertContains(t, r.Stdout, filepath.Base(repo))
	assertContains(t, r.Stdout, "Location:")
}

func TestList_ShowsMainRepo(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "(main)")
	assertContains(t, r.Stdout, "main")
}

func TestList_ShowsTotalCount(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "Total: 1 worktree(s)")
}

func TestList_MultipleWorktrees(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "test-wt1")
	createWorktreeViaWt(t, repo, "test-wt2")

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "test-wt1")
	assertContains(t, r.Stdout, "test-wt2")
	assertContains(t, r.Stdout, "Total: 3 worktree(s)")
}

func TestList_ShowsBranchNames(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "feature/test")
	gitRun(t, repo, "checkout", "main")
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "my-feature", "feature/test")

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "my-feature")
	assertContains(t, r.Stdout, "feature/test")
}

func TestList_SucceedsWithNoWorktrees(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "Total: 1 worktree(s)")
}

func TestList_ErrorOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	r := runWt(t, dir, nil, "list")
	if r.ExitCode == 0 {
		t.Error("expected failure outside git repo")
	}
	assertContains(t, r.Stderr, "Not a git repository")
}

// --path flag tests

func TestList_PathReturnsAbsolutePath(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "path-test")

	r := runWtSuccess(t, repo, nil, "list", "--path", "path-test")
	path := strings.TrimSpace(r.Stdout)
	if !strings.HasSuffix(path, "/path-test") {
		t.Errorf("expected path ending in /path-test, got %q", path)
	}
	assertDirExists(t, path)
}

func TestList_PathSingleLineOnly(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "single-line-test")

	r := runWtSuccess(t, repo, nil, "list", "--path", "single-line-test")
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}

func TestList_PathNonexistent(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "list", "--path", "nonexistent")
	if r.ExitCode == 0 {
		t.Error("expected failure for nonexistent worktree --path lookup")
	}
	assertContains(t, r.Stderr, "not found")
}

// --json flag tests

func TestList_JSONOutputValid(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list", "--json")
	entries := parseJSONList(t, r.Stdout)
	if len(entries) == 0 {
		t.Error("expected at least 1 entry in JSON output")
	}
}

func TestList_JSONIncludesMainRepo(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list", "--json")
	entries := parseJSONList(t, r.Stdout)

	mainCount := 0
	for _, e := range entries {
		if isMain, ok := e["is_main"].(bool); ok && isMain {
			mainCount++
		}
	}
	if mainCount != 1 {
		t.Errorf("expected exactly 1 main entry, got %d", mainCount)
	}
}

func TestList_JSONDefaultFields(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "json-fields-test")

	r := runWtSuccess(t, repo, nil, "list", "--json")
	entries := parseJSONList(t, r.Stdout)

	found := false
	for _, e := range entries {
		if name, ok := e["name"].(string); ok && name == "json-fields-test" {
			found = true
			// Default mode: only the non-status fields are present.
			requiredFields := []string{"name", "branch", "path", "is_main", "is_current"}
			for _, f := range requiredFields {
				if _, ok := e[f]; !ok {
					t.Errorf("missing field %q in JSON entry", f)
				}
			}
			if _, ok := e["is_main"].(bool); !ok {
				t.Error("is_main should be boolean")
			}
			// dirty/unpushed must be absent without --status.
			if _, ok := e["dirty"]; ok {
				t.Error("dirty key should be absent without --status")
			}
			if _, ok := e["unpushed"]; ok {
				t.Error("unpushed key should be absent without --status")
			}
		}
	}
	if !found {
		t.Error("json-fields-test not found in JSON output")
	}
}

func TestList_JSONStatusFields(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "json-status-test")

	r := runWtSuccess(t, repo, nil, "list", "--status", "--json")
	entries := parseJSONList(t, r.Stdout)

	found := false
	for _, e := range entries {
		if name, ok := e["name"].(string); ok && name == "json-status-test" {
			found = true
			requiredFields := []string{"name", "branch", "path", "is_main", "is_current", "dirty", "unpushed"}
			for _, f := range requiredFields {
				if _, ok := e[f]; !ok {
					t.Errorf("missing field %q in JSON entry under --status", f)
				}
			}
			if _, ok := e["dirty"].(bool); !ok {
				t.Error("dirty should be boolean")
			}
			if _, ok := e["unpushed"].(float64); !ok {
				t.Error("unpushed should be number")
			}
		}
	}
	if !found {
		t.Error("json-status-test not found in JSON output")
	}
}

func TestList_JSONDetectsDirty(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dirty-json-test")

	// Make the worktree dirty
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0644)

	r := runWtSuccess(t, repo, nil, "list", "--status", "--json")
	entries := parseJSONList(t, r.Stdout)

	for _, e := range entries {
		if name, ok := e["name"].(string); ok && name == "dirty-json-test" {
			if dirty, ok := e["dirty"].(bool); !ok || !dirty {
				t.Error("expected dirty=true for dirty worktree under --status")
			}
			return
		}
	}
	t.Error("dirty-json-test not found in JSON output")
}

func TestList_JSONIsCurrentField(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list", "--json")
	entries := parseJSONList(t, r.Stdout)

	for _, e := range entries {
		if name, ok := e["name"].(string); ok && name == "main" {
			if isCurrent, ok := e["is_current"].(bool); !ok || !isCurrent {
				t.Error("expected is_current=true for main when running from main repo")
			}
			return
		}
	}
	t.Error("main not found in JSON output")
}

// mutual exclusivity

func TestList_PathAndJSONMutuallyExclusive(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "list", "--path", "foo", "--json")
	if r.ExitCode == 0 {
		t.Error("expected failure for --path and --json together")
	}
	assertContains(t, r.Stderr, "mutually exclusive")
}

// dirty/status indicators

func TestList_DefaultModeNoDirtyIndicator(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dirty-default-test")

	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0644)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "dirty-default-test")
	// In default mode no `*` dirty indicator appears on the dirty worktree line.
	// The leading current-worktree marker column is not on the data row, so a
	// `*` on this line would be a status marker.
	for _, line := range strings.Split(r.Stdout, "\n") {
		if !strings.Contains(line, "dirty-default-test") {
			continue
		}
		if strings.Contains(line, "*") {
			t.Errorf("expected NO dirty indicator '*' on default-mode line, got: %s", line)
		}
		return
	}
	t.Fatal("dirty-default-test line not found in output")
}

func TestList_StatusModeShowsDirty(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dirty-status-test")

	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0644)

	r := runWtSuccess(t, repo, nil, "list", "--status")
	assertContains(t, r.Stdout, "dirty-status-test")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "dirty-status-test") {
			if !strings.Contains(line, "*") {
				t.Errorf("expected dirty indicator '*' on dirty-status-test line under --status, got: %s", line)
			}
			return
		}
	}
	t.Fatal("dirty-status-test line not found in output")
}

// formatted output layout

func TestList_DefaultHeader(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "fmt-test")

	r := runWtSuccess(t, repo, nil, "list")
	// Default header must contain Name/Branch/Path but NOT Status.
	assertContains(t, r.Stdout, "Name")
	assertContains(t, r.Stdout, "Branch")
	assertContains(t, r.Stdout, "Path")
	assertNotContains(t, r.Stdout, "Status")

	// Separator row must be absent.
	assertNotContains(t, r.Stdout, "----")

	// Paths should be relative (contain ".worktrees/" segment, no leading "/")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "fmt-test") && !strings.HasPrefix(line, "Worktrees") && !strings.HasPrefix(line, "Location") {
			if strings.Contains(line, ".worktrees/") {
				return
			}
		}
	}
	t.Error("expected relative path with .worktrees/ segment for fmt-test worktree")
}

func TestList_StatusHeader(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "fmt-status-test")

	r := runWtSuccess(t, repo, nil, "list", "--status")
	assertContains(t, r.Stdout, "Name")
	assertContains(t, r.Stdout, "Branch")
	assertContains(t, r.Stdout, "Status")
	assertContains(t, r.Stdout, "Path")
}

// --status flag tests

func TestList_StatusFlagInHelp(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "list", "--help")
	assertContains(t, r.Stdout, "--status")
}

func TestList_StatusAndPathMutuallyExclusive(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "list", "--status", "--path", "foo")
	if r.ExitCode == 0 {
		t.Error("expected failure for --status and --path together")
	}
	assertContains(t, r.Stderr, "mutually exclusive")
}

func TestList_StatusFlagShowsUnpushed(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "unpushed-test")

	// Push the worktree branch to origin so it has an upstream, then commit
	// locally without pushing to create unpushed commits.
	gitRun(t, wtPath, "push", "-q", "-u", "origin", "unpushed-test")
	os.WriteFile(filepath.Join(wtPath, "ahead1.txt"), []byte("ahead"), 0644)
	gitRun(t, wtPath, "add", "ahead1.txt")
	gitRun(t, wtPath, "commit", "-q", "-m", "first ahead commit")
	os.WriteFile(filepath.Join(wtPath, "ahead2.txt"), []byte("ahead2"), 0644)
	gitRun(t, wtPath, "add", "ahead2.txt")
	gitRun(t, wtPath, "commit", "-q", "-m", "second ahead commit")

	r := runWtSuccess(t, repo, nil, "list", "--status")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "unpushed-test") {
			if !strings.Contains(line, "↑2") {
				t.Errorf("expected '↑2' on unpushed-test line, got: %s", line)
			}
			return
		}
	}
	t.Fatal("unpushed-test line not found in output")
}

func TestList_StatusOrderingPreserved(t *testing.T) {
	repo := createTestRepo(t)
	// Create several worktrees so parallel enrichment has work to spread across workers.
	names := []string{"order-a", "order-b", "order-c", "order-d", "order-e"}
	for _, n := range names {
		createWorktreeViaWt(t, repo, n)
	}

	r := runWtSuccess(t, repo, nil, "list", "--status")

	// Capture the order in which worktree names appear in stdout, comparing
	// against the porcelain order (which lists main first, then others in
	// the order git tracks them).
	porcelainOut, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git worktree list --porcelain: %v", err)
	}
	var expected []string
	for _, line := range strings.Split(string(porcelainOut), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		p := strings.TrimPrefix(line, "worktree ")
		base := filepath.Base(p)
		if base == filepath.Base(repo) {
			expected = append(expected, "(main)")
		} else {
			expected = append(expected, base)
		}
	}

	var got []string
	for _, line := range strings.Split(r.Stdout, "\n") {
		for _, exp := range expected {
			if strings.Contains(line, exp) && !strings.HasPrefix(line, "Worktrees") && !strings.HasPrefix(line, "Location") {
				got = append(got, exp)
				break
			}
		}
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d rows, got %d (got=%v expected=%v)", len(expected), len(got), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("row %d: expected %q, got %q (full got=%v)", i, expected[i], got[i], got)
		}
	}
}

// NO_COLOR support

func TestList_NoColorSupport(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, []string{"NO_COLOR=1"}, "list")
	// Should not contain ANSI escape codes
	if strings.Contains(r.Stdout, "\033[") {
		t.Error("output contains ANSI color codes despite NO_COLOR=1")
	}
}
