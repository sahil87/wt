package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "my-feature", "--checkout", "feature/test")

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
	// Structured ExitWithError(ExitGeneralError, ...) — byte-parity with the
	// open.go/go.go not-found case (what/why/fix on stderr, exit 1).
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
	assertContains(t, r.Stderr, "Error: Worktree 'nonexistent' not found")
	assertContains(t, r.Stderr, "Why: No worktree with that name in this repository")
	assertContains(t, r.Stderr, "Fix: Use 'wt list' to see available worktrees")
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

// TestList_StatusOrderingPreserved verifies that under a STABLE sort mode
// (--status --sort=name), parallel enrichment does not reorder rows relative to
// the chosen order. With the audience-split default now in effect, the stable
// order is asserted explicitly via --sort=name (main pinned first, then names
// ascending) rather than against raw porcelain order. The worker-pool indexed-
// write invariant is the property under test: the deterministic post-enrichment
// sort must produce the same order on every run.
func TestList_StatusOrderingPreserved(t *testing.T) {
	repo := createTestRepo(t)
	// Create several worktrees so parallel enrichment has work to spread across
	// workers. Create them in non-sorted order to prove the sort, not creation
	// order, decides the output.
	names := []string{"order-c", "order-a", "order-e", "order-b", "order-d"}
	for _, n := range names {
		createWorktreeViaWt(t, repo, n)
	}

	// Stable mode: main pinned first, then non-main entries name-ascending.
	expected := []string{"(main)", "order-a", "order-b", "order-c", "order-d", "order-e"}

	r := runWtSuccess(t, repo, nil, "list", "--status", "--sort=name")

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

// ---------- --sort flag + audience-split default ----------

// chtimesWt sets a controlled mtime on a named worktree directory so recency
// ordering is deterministic in list tests.
func chtimesWt(t *testing.T, repo, name string, mtime time.Time) {
	t.Helper()
	p := worktreePath(repo, name)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatalf("Chtimes %s: %v", name, err)
	}
}

// jsonNonMainOrder returns the non-main worktree names from --json output in
// the order they appear in the array.
func jsonNonMainOrder(t *testing.T, jsonStr string) []string {
	t.Helper()
	entries := parseJSONList(t, jsonStr)
	var names []string
	for _, e := range entries {
		isMain, _ := e["is_main"].(bool)
		if isMain {
			continue
		}
		if n, ok := e["name"].(string); ok {
			names = append(names, n)
		}
	}
	return names
}

// humanNonMainOrder returns the non-main worktree names from human list output
// in the order their rows appear, given the set of names to look for.
func humanNonMainOrder(stdout string, candidates []string) []string {
	var order []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Worktrees") || strings.HasPrefix(line, "Location") || strings.HasPrefix(line, "Total") {
			continue
		}
		for _, c := range candidates {
			if strings.Contains(line, c) {
				order = append(order, c)
				break
			}
		}
	}
	return order
}

func assertOrder(t *testing.T, got, want []string, ctx string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected %v, got %v", ctx, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: expected %v, got %v", ctx, want, got)
		}
	}
}

func TestList_SortRecent(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWt(t, repo, "alpha", base)
	chtimesWt(t, repo, "bravo", base.Add(time.Hour))
	chtimesWt(t, repo, "charlie", base.Add(2*time.Hour))

	r := runWtSuccess(t, repo, nil, "list", "--sort=recent")
	got := humanNonMainOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	assertOrder(t, got, []string{"charlie", "bravo", "alpha"}, "--sort=recent")
}

func TestList_SortName(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"charlie", "alpha", "bravo"} {
		createWorktreeViaWt(t, repo, n)
	}
	r := runWtSuccess(t, repo, nil, "list", "--sort=name")
	got := humanNonMainOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	assertOrder(t, got, []string{"alpha", "bravo", "charlie"}, "--sort=name")
}

func TestList_SortBranch(t *testing.T) {
	repo := createTestRepo(t)
	// Branch names sort in a different order than the worktree (creation) names:
	// worktrees wt-x/wt-y/wt-z carry branches charlie/alpha/bravo, so
	// branch-ascending order is wt-y(alpha), wt-z(bravo), wt-x(charlie).
	for _, b := range []string{"charlie", "alpha", "bravo"} {
		gitRun(t, repo, "branch", b)
	}
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "wt-x", "--worktree-init", "false", "--checkout", "charlie")
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "wt-y", "--worktree-init", "false", "--checkout", "alpha")
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "wt-z", "--worktree-init", "false", "--checkout", "bravo")

	r := runWtSuccess(t, repo, nil, "list", "--sort=branch")
	got := humanNonMainOrder(r.Stdout, []string{"wt-x", "wt-y", "wt-z"})
	assertOrder(t, got, []string{"wt-y", "wt-z", "wt-x"}, "--sort=branch")
}

func TestList_SortInvalidValue(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "list", "--sort=bogus")
	assertExitCode(t, r, 2) // ExitInvalidArgs
	assertContains(t, r.Stderr, "recent")
	assertContains(t, r.Stderr, "name")
	assertContains(t, r.Stderr, "branch")
}

func TestList_SortAndPathMutuallyExclusive(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "list", "--path", "foo", "--sort=recent")
	assertExitCode(t, r, 2) // ExitInvalidArgs
	assertContains(t, r.Stderr, "--path and --sort are mutually exclusive")
}

func TestList_MainPinnedFirstUnderRecency(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo"} {
		createWorktreeViaWt(t, repo, n)
	}
	// Make the non-main worktrees newer than main so a naive recency sort would
	// push main down; it must still be first.
	future := time.Now().Add(48 * time.Hour)
	chtimesWt(t, repo, "alpha", future)
	chtimesWt(t, repo, "bravo", future.Add(time.Hour))

	r := runWtSuccess(t, repo, nil, "list", "--sort=recent")
	// The first data row (after the header) must be (main).
	var dataRows []string
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.HasPrefix(line, "Worktrees") || strings.HasPrefix(line, "Location") ||
			strings.HasPrefix(line, "Total") || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "Name") && strings.Contains(line, "Branch") && strings.Contains(line, "Path") {
			continue // header
		}
		dataRows = append(dataRows, line)
	}
	if len(dataRows) == 0 || !strings.Contains(dataRows[0], "(main)") {
		t.Fatalf("expected (main) as first data row under --sort=recent, got rows:\n%v", dataRows)
	}
}

func TestList_JSONDefaultIsStableName(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	// Recency that would invert name order if recency leaked into JSON default.
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWt(t, repo, "alpha", base.Add(2*time.Hour)) // newest
	chtimesWt(t, repo, "bravo", base.Add(time.Hour))
	chtimesWt(t, repo, "charlie", base) // oldest

	r := runWtSuccess(t, repo, nil, "list", "--json")
	got := jsonNonMainOrder(t, r.Stdout)
	assertOrder(t, got, []string{"alpha", "bravo", "charlie"}, "--json default (stable name)")
}

func TestList_JSONExplicitSortOverridesDefault(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWt(t, repo, "alpha", base)
	chtimesWt(t, repo, "bravo", base.Add(time.Hour))
	chtimesWt(t, repo, "charlie", base.Add(2*time.Hour)) // newest

	r := runWtSuccess(t, repo, nil, "list", "--json", "--sort=recent")
	got := jsonNonMainOrder(t, r.Stdout)
	assertOrder(t, got, []string{"charlie", "bravo", "alpha"}, "--json --sort=recent")
}

func TestList_NonInteractiveDefaultIsStableName(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWt(t, repo, "alpha", base.Add(2*time.Hour)) // newest
	chtimesWt(t, repo, "bravo", base.Add(time.Hour))
	chtimesWt(t, repo, "charlie", base)

	r := runWtSuccess(t, repo, nil, "list", "--non-interactive")
	got := humanNonMainOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	assertOrder(t, got, []string{"alpha", "bravo", "charlie"}, "--non-interactive default (stable name)")
}

func TestList_HumanDefaultIsRecency(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	// "alpha" is days old; the others are recent so the 4-column Last Active
	// column carries a relative-time bucket on a non-main row.
	chtimesWt(t, repo, "alpha", time.Now().Add(-72*time.Hour))
	chtimesWt(t, repo, "bravo", time.Now().Add(-time.Hour))
	chtimesWt(t, repo, "charlie", time.Now()) // newest

	r := runWtSuccess(t, repo, nil, "list")
	got := humanNonMainOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	assertOrder(t, got, []string{"charlie", "bravo", "alpha"}, "human default (recency)")

	// The recency-ordered human view is now 4-column: assert the Last Active
	// header and a relative-time value on the oldest non-main row.
	assertContains(t, r.Stdout, "Last Active")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "alpha") {
			if !strings.Contains(line, "3d ago") {
				t.Errorf("expected '3d ago' on alpha row in recency human view, got: %s", line)
			}
			return
		}
	}
	t.Fatal("alpha row not found in recency human view")
}

// ---------- --status last_active column ----------

func TestList_LastActiveOmittedInDefaultMode(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "la-default")

	r := runWtSuccess(t, repo, nil, "list", "--json")
	entries := parseJSONList(t, r.Stdout)
	for _, e := range entries {
		if _, ok := e["last_active"]; ok {
			t.Errorf("last_active key should be absent without --status, entry: %v", e)
		}
	}
}

func TestList_LastActivePresentUnderStatus(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "la-status")

	r := runWtSuccess(t, repo, nil, "list", "--status", "--json")
	entries := parseJSONList(t, r.Stdout)
	if len(entries) == 0 {
		t.Fatal("no entries in JSON output")
	}
	for _, e := range entries {
		v, ok := e["last_active"]
		if !ok {
			t.Errorf("last_active key missing under --status for entry: %v", e)
			continue
		}
		ts, ok := v.(string)
		if !ok {
			t.Errorf("last_active should be a string timestamp, got %T", v)
			continue
		}
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("last_active %q is not RFC3339: %v", ts, err)
		}
	}
}

// TestList_LastActiveRelativeTimeInHumanStatus asserts the --status 5-column
// view renders Last Active with the expected relative time. The Last Active
// column is no longer status-only — it also appears in the recency human view
// (4-column), covered by TestList_HumanDefaultIsRecency and
// TestList_RecentHumanShowsLastActiveColumn. This test pins the --status view's
// 5-column rendering specifically, which is unchanged by this change.
func TestList_LastActiveRelativeTimeInHumanStatus(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "la-rel")
	// ~2 hours ago.
	chtimesWt(t, repo, "la-rel", time.Now().Add(-2*time.Hour))

	r := runWtSuccess(t, repo, nil, "list", "--status")
	assertContains(t, r.Stdout, "Last Active")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "la-rel") {
			if !strings.Contains(line, "2h ago") {
				t.Errorf("expected '2h ago' on la-rel row, got: %s", line)
			}
			return
		}
	}
	t.Fatal("la-rel row not found")
}

// ---------- recency human view: 4-column Last Active ----------

// TestList_RecentHumanShowsLastActiveColumn asserts the default human view
// emits the 4-column Last Active header and distinct relative-time buckets on
// non-main rows, with newest-first order preserved.
func TestList_RecentHumanShowsLastActiveColumn(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"fresh", "stale"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWt(t, repo, "fresh", time.Now())                    // just now
	chtimesWt(t, repo, "stale", time.Now().Add(-96*time.Hour)) // 4d ago

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "Last Active")

	// Newest-first ordering preserved.
	got := humanNonMainOrder(r.Stdout, []string{"fresh", "stale"})
	assertOrder(t, got, []string{"fresh", "stale"}, "recency human view order")

	// Distinct relative-time buckets on the respective rows.
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "fresh") && !strings.Contains(line, "just now") {
			t.Errorf("expected 'just now' on fresh row, got: %s", line)
		}
		if strings.Contains(line, "stale") && !strings.Contains(line, "4d ago") {
			t.Errorf("expected '4d ago' on stale row, got: %s", line)
		}
	}
}

// TestList_NameBranchModesNoLastActiveColumn asserts --sort=name and
// --sort=branch human output stay 3-column with no Last Active header.
func TestList_NameBranchModesNoLastActiveColumn(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo"} {
		createWorktreeViaWt(t, repo, n)
	}

	for _, mode := range []string{"name", "branch"} {
		r := runWtSuccess(t, repo, nil, "list", "--sort="+mode)
		assertContains(t, r.Stdout, "Name")
		assertContains(t, r.Stdout, "Branch")
		assertContains(t, r.Stdout, "Path")
		assertNotContains(t, r.Stdout, "Last Active")
	}
}

// TestList_JSONOmitsLastActiveWithAndWithoutRecentSort asserts neither --json
// nor --json --sort=recent emit a last_active key, while preserving each mode's
// ordering contract (bare --json name-ordered; --json --sort=recent recent).
func TestList_JSONOmitsLastActiveWithAndWithoutRecentSort(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		createWorktreeViaWt(t, repo, n)
	}
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWt(t, repo, "alpha", base)
	chtimesWt(t, repo, "bravo", base.Add(time.Hour))
	chtimesWt(t, repo, "charlie", base.Add(2*time.Hour)) // newest

	// Bare --json: no last_active, stable name order.
	rPlain := runWtSuccess(t, repo, nil, "list", "--json")
	for _, e := range parseJSONList(t, rPlain.Stdout) {
		if _, ok := e["last_active"]; ok {
			t.Errorf("last_active key present in --json output, entry: %v", e)
		}
	}
	assertOrder(t, jsonNonMainOrder(t, rPlain.Stdout),
		[]string{"alpha", "bravo", "charlie"}, "--json (stable name)")

	// --json --sort=recent: still no last_active, recency-ordered.
	rRecent := runWtSuccess(t, repo, nil, "list", "--json", "--sort=recent")
	for _, e := range parseJSONList(t, rRecent.Stdout) {
		if _, ok := e["last_active"]; ok {
			t.Errorf("last_active key present in --json --sort=recent output, entry: %v", e)
		}
	}
	assertOrder(t, jsonNonMainOrder(t, rRecent.Stdout),
		[]string{"charlie", "bravo", "alpha"}, "--json --sort=recent")
}

// TestList_MainRowShowsOwnLastActiveInRecentMode asserts the main worktree row
// in recent human mode shows its own relative-time Last Active, not "-".
func TestList_MainRowShowsOwnLastActiveInRecentMode(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "other")
	// Set the main repo dir mtime to ~3 hours ago.
	mainMtime := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(repo, mainMtime, mainMtime); err != nil {
		t.Fatalf("Chtimes main repo: %v", err)
	}

	r := runWtSuccess(t, repo, nil, "list")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "(main)") {
			if !strings.Contains(line, "3h ago") {
				t.Errorf("expected main row to show its own relative time '3h ago', got: %s", line)
			}
			if strings.Contains(line, " - ") {
				t.Errorf("expected main row NOT to render '-' for Last Active, got: %s", line)
			}
			return
		}
	}
	t.Fatal("(main) row not found in recency human view")
}

// TestList_VanishedWorktreeRendersDashInRecentMode asserts a worktree directory
// that cannot be stat'd renders "-" in the recent-mode Last Active column (zero
// time.Time → "-" via relativeTime).
func TestList_VanishedWorktreeRendersDashInRecentMode(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "vanished")
	// Remove the worktree directory from disk (but leave it registered in git)
	// so RecencyOf cannot stat it and yields the zero time.
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("RemoveAll worktree dir: %v", err)
	}

	r := runWtSuccess(t, repo, nil, "list")
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "vanished") {
			// A zero recency key renders "-"; no relative-time bucket should
			// appear on the row (which would mean a real mtime was read).
			if strings.Contains(line, "ago") || strings.Contains(line, "just now") {
				t.Errorf("expected no relative time on vanished worktree row, got: %s", line)
			}
			if !strings.Contains(line, "-") {
				t.Errorf("expected '-' Last Active on vanished worktree row, got: %s", line)
			}
			return
		}
	}
	t.Fatal("vanished worktree row not found")
}

// TestList_MainOnlyRepoRendersRecentHeader asserts a repository with only the
// main worktree renders the 4-column Last Active header, the (main) row, and the
// total line without panic or misalignment.
func TestList_MainOnlyRepoRendersRecentHeader(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "Last Active")
	assertContains(t, r.Stdout, "(main)")
	assertContains(t, r.Stdout, "Total: 1 worktree(s)")
}

// ---------- idle marker (260530-5fyu) ----------

// idleLine finds the human-output row containing name and returns it.
func idleLine(t *testing.T, stdout, name string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	t.Fatalf("row for %q not found in:\n%s", name, stdout)
	return ""
}

// TestList_IdleMarkerInRecentHuman asserts the default 4-column human view marks
// a non-main worktree older than the 7d threshold with "⚠ idle" on its Last
// Active cell, while a fresh worktree shows no marker.
func TestList_IdleMarkerInRecentHuman(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"fresh", "ancient"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWt(t, repo, "fresh", time.Now())                         // just now
	chtimesWt(t, repo, "ancient", time.Now().Add(-40*24*time.Hour)) // 40 days ago

	r := runWtSuccess(t, repo, nil, "list")
	ancientRow := idleLine(t, r.Stdout, "ancient")
	if !strings.Contains(ancientRow, "idle") {
		t.Errorf("expected '⚠ idle' marker on 40d-old worktree row, got: %s", ancientRow)
	}
	if !strings.Contains(ancientRow, "40d ago") {
		t.Errorf("expected '40d ago' relative time on ancient row, got: %s", ancientRow)
	}
	freshRow := idleLine(t, r.Stdout, "fresh")
	if strings.Contains(freshRow, "idle") {
		t.Errorf("expected NO idle marker on fresh worktree row, got: %s", freshRow)
	}
}

// TestList_IdleMarkerInStatusHuman asserts the 5-column --status view also marks
// idle worktrees on the Last Active cell.
func TestList_IdleMarkerInStatusHuman(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "ancient")
	chtimesWt(t, repo, "ancient", time.Now().Add(-40*24*time.Hour))

	r := runWtSuccess(t, repo, nil, "list", "--status")
	ancientRow := idleLine(t, r.Stdout, "ancient")
	if !strings.Contains(ancientRow, "idle") {
		t.Errorf("expected '⚠ idle' marker under --status, got: %s", ancientRow)
	}
}

// TestList_MainNeverMarkedIdle asserts the main worktree is never annotated idle
// even when its directory mtime is well past the threshold.
func TestList_MainNeverMarkedIdle(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "other")
	// Age the main repo dir past the threshold.
	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(repo, old, old); err != nil {
		t.Fatalf("Chtimes main repo: %v", err)
	}

	r := runWtSuccess(t, repo, nil, "list")
	mainRow := idleLine(t, r.Stdout, "(main)")
	if strings.Contains(mainRow, "idle") {
		t.Errorf("main worktree must never be marked idle, got: %s", mainRow)
	}
}

// TestList_NameModeNoIdleMarker asserts the 3-column name/branch human modes show
// no idle marker (they do no per-worktree stat).
func TestList_NameModeNoIdleMarker(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "ancient")
	chtimesWt(t, repo, "ancient", time.Now().Add(-40*24*time.Hour))

	for _, mode := range []string{"name", "branch"} {
		r := runWtSuccess(t, repo, nil, "list", "--sort="+mode)
		assertNotContains(t, r.Stdout, "idle")
	}
}

// TestList_JSONIdleAbsentInDefault asserts the default machine path emits no
// "idle" key (LastActive stays nil → omitempty omits idle too).
func TestList_JSONIdleAbsentInDefault(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "ancient")
	chtimesWt(t, repo, "ancient", time.Now().Add(-40*24*time.Hour))

	// Bare --json and --json --sort=recent both leave LastActive nil.
	for _, args := range [][]string{{"list", "--json"}, {"list", "--json", "--sort=recent"}} {
		r := runWtSuccess(t, repo, nil, args...)
		for _, e := range parseJSONList(t, r.Stdout) {
			if _, ok := e["idle"]; ok {
				t.Errorf("idle key present in %v output, entry: %v", args, e)
			}
		}
	}
}

// TestList_JSONIdlePresentUnderStatus asserts every object carries a boolean
// "idle" key under --status: true for an old non-main worktree, false for main.
func TestList_JSONIdlePresentUnderStatus(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "ancient")
	chtimesWt(t, repo, "ancient", time.Now().Add(-40*24*time.Hour))
	// Keep main fresh so its idle would be false even ignoring the main override.
	now := time.Now()
	if err := os.Chtimes(repo, now, now); err != nil {
		t.Fatalf("Chtimes main repo: %v", err)
	}

	r := runWtSuccess(t, repo, nil, "list", "--status", "--json")
	for _, e := range parseJSONList(t, r.Stdout) {
		v, ok := e["idle"]
		if !ok {
			t.Errorf("idle key missing under --status for entry: %v", e)
			continue
		}
		b, ok := v.(bool)
		if !ok {
			t.Errorf("idle should be a bool, got %T for entry: %v", v, e)
			continue
		}
		isMain, _ := e["is_main"].(bool)
		name, _ := e["name"].(string)
		if isMain && b {
			t.Errorf("main worktree must have idle:false, got true")
		}
		if name == "ancient" && !b {
			t.Errorf("40d-old worktree must have idle:true, got false")
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
