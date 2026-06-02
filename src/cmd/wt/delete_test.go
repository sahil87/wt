package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDelete_ByName(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "test-wt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "test-wt")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
	assertWorktreeNotExists(t, repo, "test-wt")
}

func TestDelete_BranchDeletedByDefault(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "branch-test")

	assertBranchExists(t, repo, "branch-test")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "branch-test")

	assertWorktreeNotExists(t, repo, "branch-test")
	assertBranchNotExists(t, repo, "branch-test")
}

func TestDelete_PreservesBranchWhenFalse(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "keep-branch")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "keep-branch", "--delete-branch", "false")

	assertWorktreeNotExists(t, repo, "keep-branch")
	assertBranchExists(t, repo, "keep-branch")
}

func TestDelete_ErrorNonexistent(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "nonexistent")
	if r.ExitCode == 0 {
		t.Error("expected failure for nonexistent worktree")
	}
	assertContains(t, r.Stderr, "not found")
}

func TestDelete_ErrorNoWorktreeSpecifiedNonInteractive(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "delete", "--non-interactive")
	if r.ExitCode == 0 {
		t.Error("expected failure when no worktree specified in non-interactive mode")
	}
	assertContains(t, r.Stderr, "No worktree specified")
}

func TestDelete_StashFlag(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "stash-test")

	// Create uncommitted changes in the worktree
	os.WriteFile(filepath.Join(wtPath, "dirty-file.txt"), []byte("uncommitted"), 0644)
	gitRun(t, wtPath, "add", "dirty-file.txt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "stash-test", "--stash")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Stashing changes")
	assertContains(t, combined, "Deleted worktree")

	// Verify stash exists
	stashOut := gitRun(t, repo, "stash", "list")
	assertContains(t, stashOut, "wt-delete")
}

func TestDelete_DiscardsInNonInteractive(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "discard-test")

	os.WriteFile(filepath.Join(wtPath, "dirty-file.txt"), []byte("will-be-discarded"), 0644)

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "discard-test")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
	assertWorktreeNotExists(t, repo, "discard-test")
}

func TestDelete_All(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "wt-all-1")
	createWorktreeViaWt(t, repo, "wt-all-2")
	createWorktreeViaWt(t, repo, "wt-all-3")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all")

	assertWorktreeNotExists(t, repo, "wt-all-1")
	assertWorktreeNotExists(t, repo, "wt-all-2")
	assertWorktreeNotExists(t, repo, "wt-all-3")
}

func TestDelete_AllNoWorktrees(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "No worktrees found")
}

func TestDelete_AllCleansBranches(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "all-branch-1")
	createWorktreeViaWt(t, repo, "all-branch-2")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all", "--delete-branch", "true")

	assertBranchNotExists(t, repo, "all-branch-1")
	assertBranchNotExists(t, repo, "all-branch-2")
}

func TestDelete_DirectoryRemoved(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dir-check")

	assertDirExists(t, wtPath)

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "dir-check")

	assertDirNotExists(t, wtPath)
}

func TestDelete_NotInListAfterDeletion(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "list-check")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "list-check")

	r := runWtSuccess(t, repo, nil, "list")
	combined := r.Stdout + r.Stderr
	assertNotContains(t, combined, "list-check")
}

func TestDelete_ErrorOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	r := runWt(t, dir, nil, "delete")
	if r.ExitCode == 0 {
		t.Error("expected failure outside git repo")
	}
	assertContains(t, r.Stderr, "Not a git repository")
}

func TestDelete_UnpushedCommitsNonInteractive(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "unpushed-test")

	// Create unpushed commits
	os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("change"), 0644)
	gitRun(t, wtPath, "add", ".")
	gitRun(t, wtPath, "commit", "-q", "-m", "unpushed")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "unpushed-test")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
}

func TestDelete_CreateThenDeleteWithAllOptions(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "full-delete-test")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive",
		"--worktree-name", "full-delete-test",
		"--delete-branch", "true",
		"--delete-remote", "true")

	assertWorktreeNotExists(t, repo, "full-delete-test")
	assertBranchNotExists(t, repo, "full-delete-test")
}

func TestDelete_LifecycleStashAndCleanup(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "lifecycle-test")

	// Make uncommitted changes
	os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("important work"), 0644)
	gitRun(t, wtPath, "add", "work.txt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "lifecycle-test", "--stash")
	_ = r

	assertWorktreeNotExists(t, repo, "lifecycle-test")

	stashOut := gitRun(t, repo, "stash", "list")
	if !strings.Contains(stashOut, "lifecycle-test") {
		t.Error("expected stash to contain lifecycle-test reference")
	}
}

// ---------- Multi-Delete Tests ----------

func TestDelete_MultipleByPositionalArgs(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "multi-a")
	createWorktreeViaWt(t, repo, "multi-b")
	createWorktreeViaWt(t, repo, "multi-c")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "multi-a", "multi-b")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")

	assertWorktreeNotExists(t, repo, "multi-a")
	assertWorktreeNotExists(t, repo, "multi-b")
	assertWorktreeExists(t, repo, "multi-c")
}

func TestDelete_MultipleFailFastOnInvalidName(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "valid-one")

	r := runWt(t, repo, nil, "delete", "--non-interactive", "valid-one", "typo-name")
	if r.ExitCode == 0 {
		t.Error("expected failure when one name is invalid")
	}
	assertContains(t, r.Stderr, "Worktree 'typo-name' not found")
	// valid-one must NOT have been deleted (fail-fast)
	assertWorktreeExists(t, repo, "valid-one")
}

func TestDelete_MultipleDeduplication(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "dedup-wt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "dedup-wt", "dedup-wt")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")

	assertWorktreeNotExists(t, repo, "dedup-wt")
}

func TestDelete_MultipleBranchCleanup(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "bc-alpha")
	createWorktreeViaWt(t, repo, "bc-bravo")

	assertBranchExists(t, repo, "bc-alpha")
	assertBranchExists(t, repo, "bc-bravo")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-branch", "true", "bc-alpha", "bc-bravo")

	assertWorktreeNotExists(t, repo, "bc-alpha")
	assertWorktreeNotExists(t, repo, "bc-bravo")
	assertBranchNotExists(t, repo, "bc-alpha")
	assertBranchNotExists(t, repo, "bc-bravo")
}

func TestDelete_MixPositionalAndFlagError(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "mix-alpha")
	createWorktreeViaWt(t, repo, "mix-bravo")

	r := runWt(t, repo, nil, "delete", "--non-interactive", "mix-alpha", "--worktree-name", "mix-bravo")
	if r.ExitCode == 0 {
		t.Error("expected failure when mixing positional args and --worktree-name")
	}
	assertContains(t, r.Stderr, "Cannot mix positional arguments and --worktree-name")
}

func TestDelete_SinglePositionalArg(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "single-pos")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "single-pos")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")

	assertWorktreeNotExists(t, repo, "single-pos")
}

func TestDelete_DeprecatedFlagStillWorks(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "deprecated-wt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "deprecated-wt")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
	assertContains(t, r.Stderr, "deprecated")

	assertWorktreeNotExists(t, repo, "deprecated-wt")
}

func TestDelete_MultipleAllNamesInvalid(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "delete", "--non-interactive", "foo", "bar")
	if r.ExitCode == 0 {
		t.Error("expected failure when all names are invalid")
	}
	assertContains(t, r.Stderr, "Worktree 'foo' not found")
	assertContains(t, r.Stderr, "Worktree 'bar' not found")
}

func TestDelete_AllTakesPrecedenceOverPositionalArgs(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "prec-alpha")
	createWorktreeViaWt(t, repo, "prec-bravo")
	createWorktreeViaWt(t, repo, "prec-charlie")

	// --delete-all should delete ALL worktrees, not just the named one
	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all", "prec-alpha")

	assertWorktreeNotExists(t, repo, "prec-alpha")
	assertWorktreeNotExists(t, repo, "prec-bravo")
	assertWorktreeNotExists(t, repo, "prec-charlie")
}

func TestDelete_AutoModeSkipsBranchWhenNameMismatch(t *testing.T) {
	repo := createTestRepo(t)

	// Create a branch with a different name than the worktree
	gitRun(t, repo, "checkout", "-b", "feature/different-branch")
	gitRun(t, repo, "checkout", "main")
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "auto-skip-wt", "feature/different-branch")

	// Delete the worktree WITHOUT --delete-branch (auto mode)
	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "auto-skip-wt")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
	// Auto mode should skip branch deletion and print the skip message
	assertContains(t, combined, "Skipped branch deletion")

	// The branch should still exist because auto mode only deletes when branch == wtName
	assertWorktreeNotExists(t, repo, "auto-skip-wt")
	assertBranchExists(t, repo, "feature/different-branch")
}

func TestDelete_AutoModeDeletesBranchWhenNameMatches(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "auto-match-wt")

	// Branch name == worktree name (created by wt create)
	assertBranchExists(t, repo, "auto-match-wt")

	// Delete without --delete-branch (auto mode should delete because names match)
	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "auto-match-wt")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")

	assertWorktreeNotExists(t, repo, "auto-match-wt")
	assertBranchNotExists(t, repo, "auto-match-wt")
}

func TestDelete_MultipleBranchPreservation(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "bp-alpha")
	createWorktreeViaWt(t, repo, "bp-bravo")

	assertBranchExists(t, repo, "bp-alpha")
	assertBranchExists(t, repo, "bp-bravo")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-branch", "false", "bp-alpha", "bp-bravo")

	assertWorktreeNotExists(t, repo, "bp-alpha")
	assertWorktreeNotExists(t, repo, "bp-bravo")
	// Branches should still exist
	assertBranchExists(t, repo, "bp-alpha")
	assertBranchExists(t, repo, "bp-bravo")
}

func TestDelete_MultipleWithStash(t *testing.T) {
	repo := createTestRepo(t)
	wtPathA := createWorktreeViaWt(t, repo, "stash-alpha")
	wtPathB := createWorktreeViaWt(t, repo, "stash-bravo")

	// Create uncommitted changes in both worktrees
	os.WriteFile(filepath.Join(wtPathA, "dirty-a.txt"), []byte("alpha changes"), 0644)
	gitRun(t, wtPathA, "add", "dirty-a.txt")
	os.WriteFile(filepath.Join(wtPathB, "dirty-b.txt"), []byte("bravo changes"), 0644)
	gitRun(t, wtPathB, "add", "dirty-b.txt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--stash", "stash-alpha", "stash-bravo")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Stashing changes")

	assertWorktreeNotExists(t, repo, "stash-alpha")
	assertWorktreeNotExists(t, repo, "stash-bravo")

	// Verify stashes exist
	stashOut := gitRun(t, repo, "stash", "list")
	assertContains(t, stashOut, "stash-alpha")
	assertContains(t, stashOut, "stash-bravo")
}

// TestDelete_MenuOrdersNewestFirst verifies the delete selection menu lists
// non-main worktrees newest-first (after the prepended "All" entry), mirroring
// the open menu. Empty stdin makes ShowMenu print the menu then return on EOF;
// we assert only on the printed ordering and delete nothing.
func TestDelete_MenuOrdersNewestFirst(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	createWorktreeViaWt(t, repo, "bravo")
	createWorktreeViaWt(t, repo, "charlie")

	// Recent, distinct mtimes so ordering is deterministic AND none of the
	// worktrees are idle — this test isolates newest-first ordering and the
	// unshifted default (no "All idle" entry). Idle annotation/shift behavior is
	// covered by the dedicated stale-aware menu tests below.
	now := time.Now()
	chtimesWorktree(t, repo, "alpha", now.Add(-2*time.Hour))
	chtimesWorktree(t, repo, "bravo", now.Add(-time.Hour))
	chtimesWorktree(t, repo, "charlie", now)

	r := runWt(t, repo, nil, "delete")
	got := menuOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	want := []string{"charlie", "bravo", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("expected %v in menu, got %v\nstdout:\n%s", want, got, r.Stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("delete menu order = %v, want %v", got, want)
			break
		}
	}
	// The newest worktree must be the marked default (offset by the "All" entry).
	assertContains(t, r.Stdout, "charlie (charlie) (default)")
	// No idle worktrees → no "All idle" entry.
	assertNotContains(t, r.Stdout, "All idle")

	// Nothing was deleted (menu was cancelled via EOF).
	assertWorktreeExists(t, repo, "alpha")
	assertWorktreeExists(t, repo, "bravo")
	assertWorktreeExists(t, repo, "charlie")
}

// ---------- stale-aware menu + --stale selector (260530-5fyu) ----------

// daysAgo returns a time N days before now, for controlled idle/fresh mtimes.
func daysAgo(n int) time.Time {
	return time.Now().Add(-time.Duration(n) * 24 * time.Hour)
}

// TestDelete_MenuAnnotatesIdleAndAllIdleEntry verifies the interactive menu
// (printed then cancelled via EOF) annotates idle rows with ", idle", includes
// an "All idle (N)" entry counting only idle worktrees, and pre-selects the
// newest worktree row at defaultIdx 3 (shifted from 2 because "All idle" is
// present). Empty stdin makes ShowMenu print the menu then return on EOF;
// nothing is deleted.
func TestDelete_MenuAnnotatesIdleAndAllIdleEntry(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"recent-a", "old-b", "old-c"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWorktree(t, repo, "recent-a", time.Now()) // fresh, newest
	chtimesWorktree(t, repo, "old-b", daysAgo(20))   // idle
	chtimesWorktree(t, repo, "old-c", daysAgo(40))   // idle

	r := runWt(t, repo, nil, "delete")

	assertContains(t, r.Stdout, "All idle (2)")
	// Idle rows annotated.
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.Contains(line, "old-b") && !strings.Contains(line, ", idle") {
			t.Errorf("expected old-b row annotated ', idle', got: %s", line)
		}
		if strings.Contains(line, "old-c") && !strings.Contains(line, ", idle") {
			t.Errorf("expected old-c row annotated ', idle', got: %s", line)
		}
		if strings.Contains(line, "recent-a") && strings.Contains(line, ", idle") {
			t.Errorf("expected fresh recent-a row NOT annotated idle, got: %s", line)
		}
	}
	// Newest worktree is the marked default (shifted past All + All idle).
	assertContains(t, r.Stdout, "recent-a (recent-a) (default)")

	// Nothing deleted (EOF cancel).
	assertWorktreeExists(t, repo, "recent-a")
	assertWorktreeExists(t, repo, "old-b")
	assertWorktreeExists(t, repo, "old-c")
}

// TestDelete_MenuNoAllIdleWhenNoneIdle verifies the "All idle" entry is omitted
// when no worktree is idle, and the default remains the newest worktree at the
// unshifted defaultIdx 2.
func TestDelete_MenuNoAllIdleWhenNoneIdle(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"alpha", "bravo"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWorktree(t, repo, "alpha", time.Now())
	chtimesWorktree(t, repo, "bravo", daysAgo(1)) // fresh, newest is alpha

	r := runWt(t, repo, nil, "delete")
	assertNotContains(t, r.Stdout, "All idle")
	// Newest (alpha) is the default at the unshifted index.
	assertContains(t, r.Stdout, "alpha (alpha) (default)")
}

// TestDelete_StaleSelectsIdleNonInteractive verifies `wt delete --stale
// --non-interactive` deletes exactly the idle worktrees and leaves fresh ones.
func TestDelete_StaleSelectsIdleNonInteractive(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"fresh", "idle-x", "idle-y"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWorktree(t, repo, "fresh", time.Now())
	chtimesWorktree(t, repo, "idle-x", daysAgo(10))
	chtimesWorktree(t, repo, "idle-y", daysAgo(30))

	r := runWtSuccess(t, repo, nil, "delete", "--stale", "--non-interactive")
	_ = r

	assertWorktreeExists(t, repo, "fresh")
	assertWorktreeNotExists(t, repo, "idle-x")
	assertWorktreeNotExists(t, repo, "idle-y")
}

// TestDelete_StaleThresholdOverride verifies `--stale=Nd` overrides the default
// threshold for this invocation.
func TestDelete_StaleThresholdOverride(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"within", "beyond"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWorktree(t, repo, "within", daysAgo(20)) // older than 7d, younger than 30d
	chtimesWorktree(t, repo, "beyond", daysAgo(40)) // older than 30d

	runWtSuccess(t, repo, nil, "delete", "--stale=30d", "--non-interactive")

	// Only the 40d worktree exceeds the 30d threshold.
	assertWorktreeExists(t, repo, "within")
	assertWorktreeNotExists(t, repo, "beyond")
}

// TestDelete_StaleNoMatchesPrintsMessage verifies zero idle matches prints the
// informational message and exits 0 (not an error).
func TestDelete_StaleNoMatchesPrintsMessage(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "fresh")
	chtimesWorktree(t, repo, "fresh", time.Now())

	r := runWtSuccess(t, repo, nil, "delete", "--stale", "--non-interactive")
	assertContains(t, r.Stdout, "No idle worktrees (threshold: 7d).")
	assertWorktreeExists(t, repo, "fresh")
}

// TestDelete_StalePositionalMutex verifies `--stale <name>` exits ExitInvalidArgs
// with "mutually exclusive" and deletes nothing.
func TestDelete_StalePositionalMutex(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "feature-x")

	r := runWt(t, repo, nil, "delete", "--stale", "feature-x", "--non-interactive")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "mutually exclusive")
	assertWorktreeExists(t, repo, "feature-x")
}

// TestDelete_StaleDeleteAllMutex verifies `--stale --delete-all` exits
// ExitInvalidArgs with "mutually exclusive".
func TestDelete_StaleDeleteAllMutex(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "feature-x")

	r := runWt(t, repo, nil, "delete", "--stale", "--delete-all", "--non-interactive")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "mutually exclusive")
	assertWorktreeExists(t, repo, "feature-x")
}

// TestDelete_StaleInvalidThreshold verifies a malformed --stale value exits
// ExitInvalidArgs naming the accepted Nd form.
func TestDelete_StaleInvalidThreshold(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "feature-x")

	r := runWt(t, repo, nil, "delete", "--stale=banana", "--non-interactive")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "30d")
	assertWorktreeExists(t, repo, "feature-x")
}

// TestDelete_StaleNeverTargetsMain verifies the --stale selector never picks the
// main worktree even when main's dir mtime is past the threshold.
func TestDelete_StaleNeverTargetsMain(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "fresh")
	chtimesWorktree(t, repo, "fresh", time.Now())
	// Age main past the threshold.
	old := daysAgo(40)
	if err := os.Chtimes(repo, old, old); err != nil {
		t.Fatalf("Chtimes main repo: %v", err)
	}

	r := runWtSuccess(t, repo, nil, "delete", "--stale", "--non-interactive")
	// Main is excluded; only fresh would be eligible and it is not idle, so no
	// matches. Main repo dir still present.
	assertContains(t, r.Stdout, "No idle worktrees")
	assertDirExists(t, repo)
}
