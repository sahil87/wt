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
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "auto-skip-wt", "--checkout", "feature/different-branch")

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

// TestDelete_UncommittedWarning_OnStderr verifies the interactive
// "uncommitted changes" warning now lands on STDERR (not stdout), and that the
// menu prompt itself still renders on stdout. Empty stdin makes the menu return
// on EOF after the warning has printed; we assert only on the warning stream.
func TestDelete_UncommittedWarning_OnStderr(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dirty-current")

	// Make the worktree dirty so handleUncommittedChanges fires.
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("uncommitted"), 0644)
	gitRun(t, wtPath, "add", "dirty.txt")

	// Interactive (no --non-interactive), run from INSIDE the worktree with no
	// name → handleDeleteCurrent → handleUncommittedChanges. Empty stdin makes
	// the subsequent menu return on EOF after the warning prints.
	r := runWt(t, wtPath, nil, "delete")

	// The warning is a diagnostic and MUST be on stderr, never stdout.
	assertContains(t, r.Stderr, "Warning: Worktree has uncommitted changes")
	assertNotContains(t, r.Stdout, "Worktree has uncommitted changes")
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

// ---------- Intuitive flag names (change 59u8) ----------

// TestDelete_AllFlagAndShort verifies --all and -a delete every worktree
// (parity with the deprecated --delete-all).
func TestDelete_AllFlagAndShort(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "all-new-1")
	createWorktreeViaWt(t, repo, "all-new-2")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--all")
	assertNotContains(t, r.Stderr, "deprecated")
	assertWorktreeNotExists(t, repo, "all-new-1")
	assertWorktreeNotExists(t, repo, "all-new-2")

	createWorktreeViaWt(t, repo, "all-short-1")
	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "-a")
	assertWorktreeNotExists(t, repo, "all-short-1")
}

// TestDelete_RmAlias verifies `wt rm` invokes `wt delete` identically.
func TestDelete_RmAlias(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "via-rm")

	r := runWtSuccess(t, repo, nil, "rm", "--non-interactive", "via-rm")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")
	assertWorktreeNotExists(t, repo, "via-rm")
}

// TestDelete_BranchFlagForceDeletes verifies the new --branch string flag
// force-deletes the branch (parity with the deprecated --delete-branch true).
func TestDelete_BranchFlagForceDeletes(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "branch-new")
	assertBranchExists(t, repo, "branch-new")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--branch", "true", "branch-new")
	assertNotContains(t, r.Stderr, "deprecated")
	assertBranchNotExists(t, repo, "branch-new")
}

// TestDelete_NoRemoteSuppressesRemoteDeletion verifies the new real-bool
// --no-remote keeps the origin branch even when the local branch is deleted.
// Without --no-remote (the default), the remote branch would be deleted too.
func TestDelete_NoRemoteSuppressesRemoteDeletion(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "keep-remote")
	// Push the branch to origin so remote deletion would otherwise apply.
	gitRun(t, worktreePath(repo, "keep-remote"), "push", "-q", "-u", "origin", "keep-remote")
	assertRemoteBranchExists(t, repo, "keep-remote")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--branch", "true", "--no-remote", "keep-remote")

	assertBranchNotExists(t, repo, "keep-remote")
	// --no-remote means the remote branch survives.
	assertRemoteBranchExists(t, repo, "keep-remote")
}

// TestDelete_DefaultDeletesRemote verifies that WITHOUT --no-remote (and without
// the old --delete-remote string) the remote branch IS deleted — the string→bool
// conversion preserves default behavior.
func TestDelete_DefaultDeletesRemote(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "drop-remote")
	gitRun(t, worktreePath(repo, "drop-remote"), "push", "-q", "-u", "origin", "drop-remote")
	assertRemoteBranchExists(t, repo, "drop-remote")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--branch", "true", "drop-remote")

	assertBranchNotExists(t, repo, "drop-remote")
	assertRemoteBranchNotExists(t, repo, "drop-remote")
}

// TestDelete_DeprecatedFlagsStillWork verifies the deprecated delete flags still
// behave and emit a stderr deprecation warning.
func TestDelete_DeprecatedFlagsStillWork(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "legacy-del-1")
	createWorktreeViaWt(t, repo, "legacy-del-2")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all", "--delete-branch", "true", "--delete-remote", "false")
	assertContains(t, r.Stderr, "deprecated")
	assertWorktreeNotExists(t, repo, "legacy-del-1")
	assertWorktreeNotExists(t, repo, "legacy-del-2")
}

// TestDelete_HelpHidesDeprecatedShowsNew verifies `wt delete --help` shows the
// new flags and hides the deprecated aliases.
func TestDelete_HelpHidesDeprecatedShowsNew(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "delete", "--help")
	for _, want := range []string{"--all", "--branch", "--no-remote"} {
		assertContains(t, r.Stdout, want)
	}
	for _, hidden := range []string{"--delete-all", "--delete-branch", "--delete-remote"} {
		assertNotContains(t, r.Stdout, hidden)
	}
}

// ---------- --dry-run preview (change p5m9) ----------

// TestDelete_DryRunHelpMentionsFlag verifies `--dry-run` is on the delete help
// surface (R10).
func TestDelete_DryRunHelpMentionsFlag(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "delete", "--help")
	assertContains(t, r.Stdout, "--dry-run")
}

// TestDelete_DryRunByNamePreviewsNoMutation is the core single-target preview:
// a worktree whose branch matches its name and exists on origin, deleted by
// positional name under --dry-run, prints the Would-lines to stdout and leaves
// the worktree, local branch, and remote branch byte-identical (R3, R4, R8).
func TestDelete_DryRunByNamePreviewsNoMutation(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "preview-wt")
	// Push so the remote branch exists — the preview must report it without
	// deleting it.
	gitRun(t, worktreePath(repo, "preview-wt"), "push", "-q", "-u", "origin", "preview-wt")
	assertRemoteBranchExists(t, repo, "preview-wt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "preview-wt", "--dry-run", "--branch", "true")

	assertContains(t, r.Stdout, "Dry run — no changes will be made.")
	assertContains(t, r.Stdout, "Would remove worktree: preview-wt")
	assertContains(t, r.Stdout, "Would delete branch: preview-wt (local)")
	assertContains(t, r.Stdout, "Would delete branch: preview-wt (remote)")
	// The live completion messages must NOT appear — nothing was mutated.
	assertNotContains(t, r.Stdout, "Deleted worktree")
	assertNotContains(t, r.Stdout, "Removing worktree...")

	// State is byte-identical.
	assertWorktreeExists(t, repo, "preview-wt")
	assertBranchExists(t, repo, "preview-wt")
	assertRemoteBranchExists(t, repo, "preview-wt")
}

// TestDelete_DryRunAutoModeSkipMessageStillShows verifies the --branch auto
// tri-state decision runs live under dry-run: a name-mismatched branch is
// reported as skipped, not previewed as deleted (R3).
func TestDelete_DryRunAutoModeSkipMessageStillShows(t *testing.T) {
	repo := createTestRepo(t)
	gitRun(t, repo, "checkout", "-b", "feature/mismatch")
	gitRun(t, repo, "checkout", "main")
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "auto-dry", "--checkout", "feature/mismatch")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "auto-dry", "--dry-run")
	assertContains(t, r.Stdout, "Would remove worktree: auto-dry")
	assertContains(t, r.Stdout, "Skipped branch deletion")
	// Auto mode skips the mismatched branch → no local-delete preview line.
	assertNotContains(t, r.Stdout, "Would delete branch: feature/mismatch")

	assertWorktreeExists(t, repo, "auto-dry")
	assertBranchExists(t, repo, "feature/mismatch")
}

// TestDelete_DryRunNoRemoteSuppressesRemotePreview verifies --no-remote alters
// the previewed branch actions exactly as it alters the live run (R3).
func TestDelete_DryRunNoRemoteSuppressesRemotePreview(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "no-remote-dry")
	gitRun(t, worktreePath(repo, "no-remote-dry"), "push", "-q", "-u", "origin", "no-remote-dry")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "no-remote-dry", "--dry-run", "--branch", "true", "--no-remote")
	assertContains(t, r.Stdout, "Would delete branch: no-remote-dry (local)")
	// --no-remote means no remote preview line.
	assertNotContains(t, r.Stdout, "Would delete branch: no-remote-dry (remote)")

	assertWorktreeExists(t, repo, "no-remote-dry")
	assertRemoteBranchExists(t, repo, "no-remote-dry")
}

// TestDelete_DryRunCurrentDiscardsHazardReport verifies the current-worktree
// path reports the uncommitted-changes hazard as a Would-line without
// discarding (R6). Run from inside a dirty worktree with no name.
func TestDelete_DryRunCurrentDiscardsHazardReport(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "dirty-dry")
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("uncommitted"), 0644)

	r := runWtSuccess(t, wtPath, nil, "delete", "--non-interactive", "--dry-run")
	assertContains(t, r.Stdout, "Would discard uncommitted changes (use --stash to preserve them)")
	assertContains(t, r.Stdout, "Would remove worktree: dirty-dry")
	assertNotContains(t, r.Stdout, "Discarding uncommitted changes...")

	assertWorktreeExists(t, repo, "dirty-dry")
	// The dirty file survives — nothing was discarded.
	assertFileExists(t, filepath.Join(wtPath, "dirty.txt"))
}

// TestDelete_DryRunCurrentStashHazardReport verifies --stash × --dry-run
// previews the stash action without stashing (R6, assumption #7).
func TestDelete_DryRunCurrentStashHazardReport(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "stash-dry")
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("uncommitted"), 0644)

	r := runWtSuccess(t, wtPath, nil, "delete", "--non-interactive", "--dry-run", "--stash")
	assertContains(t, r.Stdout, "Would stash uncommitted changes")
	assertNotContains(t, r.Stdout, "Stashing changes...")

	assertWorktreeExists(t, repo, "stash-dry")
	// No stash was actually created.
	stashOut := gitRun(t, repo, "stash", "list")
	assertNotContains(t, stashOut, "wt-delete")
}

// TestDelete_DryRunUnpushedReport verifies the unpushed-commits hazard is
// reported as a Would-line (R6). Uses the current-worktree path (the only path
// that runs unpushed detection).
func TestDelete_DryRunUnpushedReport(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "unpushed-dry")
	// Push to establish upstream, then make one unpushed commit.
	gitRun(t, wtPath, "push", "-q", "-u", "origin", "unpushed-dry")
	os.WriteFile(filepath.Join(wtPath, "v2.txt"), []byte("v2"), 0644)
	gitRun(t, wtPath, "add", ".")
	gitRun(t, wtPath, "commit", "-q", "-m", "unpushed")

	r := runWtSuccess(t, wtPath, nil, "delete", "--non-interactive", "--dry-run")
	assertContains(t, r.Stdout, "Would lose 1 unpushed commit(s) on branch unpushed-dry")

	assertWorktreeExists(t, repo, "unpushed-dry")
}

// TestDelete_DryRunStashByNameNoStash verifies the byName/multiple stash path
// (handleStashInDir) previews the stash without mutating (R4).
func TestDelete_DryRunStashByNameNoStash(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "stash-name-dry")
	os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("uncommitted"), 0644)
	gitRun(t, wtPath, "add", "dirty.txt")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "stash-name-dry", "--dry-run", "--stash")
	assertContains(t, r.Stdout, "Would stash uncommitted changes")
	assertNotContains(t, r.Stdout, "Stashing changes...")

	assertWorktreeExists(t, repo, "stash-name-dry")
	stashOut := gitRun(t, repo, "stash", "list")
	assertNotContains(t, stashOut, "wt-delete")
}

// TestDelete_DryRunAllPreviewsAllNoMutation verifies --all × --dry-run keeps
// the per-worktree block structure and mutates nothing (R2, R8).
func TestDelete_DryRunAllPreviewsAllNoMutation(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "all-dry-1")
	createWorktreeViaWt(t, repo, "all-dry-2")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--all", "--dry-run")
	assertContains(t, r.Stdout, "Dry run — no changes will be made.")
	assertContains(t, r.Stdout, "Would remove worktree: all-dry-1")
	assertContains(t, r.Stdout, "Would remove worktree: all-dry-2")
	assertNotContains(t, r.Stdout, "Deleted worktree")

	assertWorktreeExists(t, repo, "all-dry-1")
	assertWorktreeExists(t, repo, "all-dry-2")
}

// TestDelete_DryRunStaleSelectsIdlePreviewsNoMutation verifies --stale ×
// --dry-run computes the idle target set live and previews only (R2, R3).
func TestDelete_DryRunStaleSelectsIdlePreviewsNoMutation(t *testing.T) {
	repo := createTestRepo(t)
	for _, n := range []string{"fresh-dry", "idle-dry"} {
		createWorktreeViaWt(t, repo, n)
	}
	chtimesWorktree(t, repo, "fresh-dry", time.Now())
	chtimesWorktree(t, repo, "idle-dry", daysAgo(30))

	r := runWtSuccess(t, repo, nil, "delete", "--stale", "--non-interactive", "--dry-run")
	assertContains(t, r.Stdout, "Would remove worktree: idle-dry")
	assertNotContains(t, r.Stdout, "Would remove worktree: fresh-dry")

	// Nothing deleted — both survive.
	assertWorktreeExists(t, repo, "fresh-dry")
	assertWorktreeExists(t, repo, "idle-dry")
}

// TestDelete_DryRunStaleNoMatchesKeepsEmptyState verifies the --stale zero-match
// empty-state message is preserved under dry-run (R8).
func TestDelete_DryRunStaleNoMatchesKeepsEmptyState(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "fresh-only")
	chtimesWorktree(t, repo, "fresh-only", time.Now())

	r := runWtSuccess(t, repo, nil, "delete", "--stale", "--non-interactive", "--dry-run")
	assertContains(t, r.Stdout, "No idle worktrees (threshold: 7d).")
	assertWorktreeExists(t, repo, "fresh-only")
}

// TestDelete_DryRunNoTargetNonInteractiveRefusal verifies the non-interactive
// no-target refusal is unchanged under --dry-run (R7, R9).
func TestDelete_DryRunNoTargetNonInteractiveRefusal(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "delete", "--non-interactive", "--dry-run")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "No worktree specified")
}

// TestDelete_DryRunUnknownNameFails verifies dry-run fails on exactly the inputs
// the live run fails on: an unknown name exits ExitGeneralError (R9).
func TestDelete_DryRunUnknownNameFails(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "delete", "--non-interactive", "nonexistent-dry", "--dry-run")
	if r.ExitCode == 0 {
		t.Error("expected failure for nonexistent worktree under --dry-run")
	}
	assertContains(t, r.Stderr, "not found")
}

// TestDelete_DryRunStaleMutexStillFails verifies argument validation precedes
// dry-run branching: --stale + positional still exits ExitInvalidArgs (R9).
func TestDelete_DryRunStaleMutexStillFails(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "feature-x")

	r := runWt(t, repo, nil, "delete", "--stale", "feature-x", "--non-interactive", "--dry-run")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "mutually exclusive")
	assertWorktreeExists(t, repo, "feature-x")
}

// TestDelete_DryRunInvalidThresholdFails verifies a malformed --stale value
// still exits ExitInvalidArgs under --dry-run (R9).
func TestDelete_DryRunInvalidThresholdFails(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "feature-x")

	r := runWt(t, repo, nil, "delete", "--stale=banana", "--non-interactive", "--dry-run")
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "30d")
	assertWorktreeExists(t, repo, "feature-x")
}

// TestDelete_DryRunRemoteOnlyOrphanPreviewsRemote is the regression test for the
// orphan `wt/<name>` preview↔live drift: the live path deletes a remote-only
// `refs/heads/wt/<name>` orphan independently of local existence, so the dry-run
// preview MUST print the remote `Would …` line for that orphan even when no
// local `wt/<name>` branch exists (R3, R4). --branch=false suppresses the
// primary branch preview so the only branch lines that can appear come from the
// orphan cleanup, isolating the case under test.
func TestDelete_DryRunRemoteOnlyOrphanPreviewsRemote(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "orphan-dry")

	// Create a `wt/orphan-dry` orphan that exists ONLY on origin: push it, then
	// delete the local ref so refs/heads/wt/orphan-dry is remote-only.
	gitRun(t, repo, "branch", "wt/orphan-dry")
	gitRun(t, repo, "push", "-q", "origin", "wt/orphan-dry")
	gitRun(t, repo, "branch", "-D", "wt/orphan-dry")
	assertRemoteBranchExists(t, repo, "wt/orphan-dry")
	assertBranchNotExists(t, repo, "wt/orphan-dry")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "orphan-dry", "--dry-run", "--branch", "false")

	// The remote orphan is previewed even though it is absent locally — mirroring
	// the live path, which deletes it independently of local existence.
	assertContains(t, r.Stdout, "Would delete branch: wt/orphan-dry (remote)")
	// No local orphan exists, so its local preview line must NOT appear.
	assertNotContains(t, r.Stdout, "Would delete branch: wt/orphan-dry (local)")
	// --branch=false suppresses the primary branch preview entirely.
	assertNotContains(t, r.Stdout, "Would delete branch: orphan-dry (local)")

	// State is byte-identical — the remote orphan survives the preview.
	assertWorktreeExists(t, repo, "orphan-dry")
	assertRemoteBranchExists(t, repo, "wt/orphan-dry")
	assertBranchNotExists(t, repo, "wt/orphan-dry")
}

// TestDelete_DryRunByWorktreeNameFlagPreviewsNoMutation exercises the
// handleDeleteByName dry-run path directly via the deprecated `--worktree-name`
// flag (positional names route through handleDeleteMultiple, so the byName
// branches were previously uncovered). The deprecation warning on stderr is
// expected and does not affect the preview on stdout (R2, R3, R4).
func TestDelete_DryRunByWorktreeNameFlagPreviewsNoMutation(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "byname-dry")
	gitRun(t, worktreePath(repo, "byname-dry"), "push", "-q", "-u", "origin", "byname-dry")
	assertRemoteBranchExists(t, repo, "byname-dry")

	r := runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "byname-dry", "--dry-run", "--branch", "true")

	assertContains(t, r.Stdout, "Dry run — no changes will be made.")
	assertContains(t, r.Stdout, "Would remove worktree: byname-dry")
	assertContains(t, r.Stdout, "Would delete branch: byname-dry (local)")
	assertContains(t, r.Stdout, "Would delete branch: byname-dry (remote)")
	// Nothing was mutated — the live completion messages are absent.
	assertNotContains(t, r.Stdout, "Deleted worktree")

	// State is byte-identical.
	assertWorktreeExists(t, repo, "byname-dry")
	assertBranchExists(t, repo, "byname-dry")
	assertRemoteBranchExists(t, repo, "byname-dry")
}

// TestDelete_DryRunMenuRoutePreviewsNoMutation drives the interactive
// target-selection menu (handleDeleteMenu → handleDeleteByName) under --dry-run
// via the non-TTY fallback numbered menu: with a single (non-idle) worktree the
// menu is `0) Cancel`, `1) All`, `2) <name>`, so feeding "2\n" selects the
// worktree. Dry-run skips the confirmation prompt, so the selection is the only
// menu interaction. Selection is not consent (R7) and nothing is mutated (R3).
func TestDelete_DryRunMenuRoutePreviewsNoMutation(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "menu-dry")

	r := runWtStdin(t, repo, nil, "2\n", "delete", "--dry-run", "--branch", "true")
	if r.ExitCode != 0 {
		t.Fatalf("menu-route dry-run failed (exit %d):\nstdout: %s\nstderr: %s", r.ExitCode, r.Stdout, r.Stderr)
	}

	assertContains(t, r.Stdout, "Would remove worktree: menu-dry")
	assertNotContains(t, r.Stdout, "Deleted worktree")

	// Nothing mutated — the worktree survives the preview.
	assertWorktreeExists(t, repo, "menu-dry")
	assertBranchExists(t, repo, "menu-dry")
}
