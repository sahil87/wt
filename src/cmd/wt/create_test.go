package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreate_ExploratoryWorktree(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive")

	// stderr should have the "Created worktree:" message
	assertContains(t, r.Stderr, "Created worktree:")

	// stdout should be exactly one line: the worktree path
	path := strings.TrimSpace(r.Stdout)
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line of stdout, got %d: %q", len(lines), r.Stdout)
	}
	assertDirExists(t, path)
}

func TestCreate_ExploratoryWorktreeRandomName(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive")

	// Name should be adjective-noun format
	path := strings.TrimSpace(r.Stdout)
	name := filepath.Base(path)
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Errorf("expected adjective-noun name, got %q", name)
	}
}

func TestCreate_WorktreeNameFlag(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "custom-name")

	assertContains(t, r.Stderr, "custom-name")
	assertWorktreeExists(t, repo, "custom-name")
}

func TestCreate_BranchNameDerivation(t *testing.T) {
	repo := createTestRepo(t)

	// Create a local branch, then --checkout it (existing branch → derived name)
	gitRun(t, repo, "checkout", "-b", "feature/login")
	gitRun(t, repo, "checkout", "main")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--checkout", "feature/login")

	// Should derive "login" from "feature/login"
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "login")
}

func TestCreate_ExistingLocalBranch(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "feature/auth")
	gitRun(t, repo, "checkout", "main")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "my-feature", "--checkout", "feature/auth")

	assertContains(t, r.Stderr, "Created worktree: my-feature")
	assertContains(t, r.Stderr, "Branch: feature/auth")
	assertWorktreeExists(t, repo, "my-feature")
}

func TestCreate_RemoteBranch(t *testing.T) {
	repo := createTestRepo(t)

	// Create a branch, push it, then delete locally
	gitRun(t, repo, "checkout", "-b", "remote-feature")
	os.WriteFile(filepath.Join(repo, "remote-file.txt"), []byte("test"), 0644)
	gitRun(t, repo, "add", "remote-file.txt")
	gitRun(t, repo, "commit", "-q", "-m", "remote feature")
	gitRun(t, repo, "push", "-q", "-u", "origin", "remote-feature")
	gitRun(t, repo, "checkout", "main")
	gitRun(t, repo, "branch", "-D", "remote-feature")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "remote-wt", "--checkout", "remote-feature")

	assertContains(t, r.Stderr, "remote-wt")
	assertWorktreeExists(t, repo, "remote-wt")
}

func TestCreate_NewBranch(t *testing.T) {
	repo := createTestRepo(t)

	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "new-branch-wt", "new-feature")

	assertWorktreeExists(t, repo, "new-branch-wt")
	assertBranchExists(t, repo, "new-feature")
}

// TestCreate_SummaryFrom_NewBranchWithBase asserts the Git-phase summary's
// From: line shows the --base ref verbatim for a new positional branch.
func TestCreate_SummaryFrom_NewBranchWithBase(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive",
		"--worktree-name", "from-base-wt", "new-feature", "--base", "main")

	assertContains(t, r.Stderr, "From: main")
}

// TestCreate_SummaryFrom_NewBranchNoBase asserts the From: line shows the
// current branch label (the fixture repo is on "main") when no --base is given.
func TestCreate_SummaryFrom_NewBranchNoBase(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive",
		"--worktree-name", "from-head-wt", "new-feature")

	assertContains(t, r.Stderr, "From: main")
}

// TestCreate_SummaryFrom_Checkout asserts the From: line uses the fixed
// existing-branch copy on the --checkout path.
func TestCreate_SummaryFrom_Checkout(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "feature/auth")
	gitRun(t, repo, "checkout", "main")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive",
		"--worktree-name", "from-checkout-wt", "--checkout", "feature/auth")

	assertContains(t, r.Stderr, "From: existing branch 'feature/auth' (checked out directly)")
}

// TestCreate_SummaryFrom_BareExploratory asserts the From: line shows the
// current branch label for a bare exploratory create.
func TestCreate_SummaryFrom_BareExploratory(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive")

	assertContains(t, r.Stderr, "From: main")
}

// TestCreate_CheckoutMissingBranchRejected asserts --checkout on a branch that
// exists neither locally nor remotely fails with exit 2 and the create-new
// hint, leaving no worktree behind.
func TestCreate_CheckoutMissingBranchRejected(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "missing-wt",
		"--worktree-init", "false", "--checkout", "does-not-exist")

	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "Branch 'does-not-exist' not found")
	assertContains(t, r.Stderr, "wt create does-not-exist")
	assertWorktreeNotExists(t, repo, "missing-wt")
}

// TestCreate_CheckoutWithPositionalConflict asserts --checkout and a positional
// branch argument are mutually exclusive.
func TestCreate_CheckoutWithPositionalConflict(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "existing-branch")
	gitRun(t, repo, "checkout", "main")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "conflict-pos",
		"--worktree-init", "false", "--checkout", "existing-branch", "new-branch")

	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "--checkout cannot be combined with a positional branch argument")
	assertWorktreeNotExists(t, repo, "conflict-pos")
}

func TestCreate_NameCollision(t *testing.T) {
	repo := createTestRepo(t)

	createWorktreeViaWt(t, repo, "collision-test")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "collision-test")
	if r.ExitCode == 0 {
		t.Error("expected failure on name collision")
	}
	assertContains(t, r.Stderr, "already exists")
}

func TestCreate_ReuseExisting(t *testing.T) {
	repo := createTestRepo(t)

	firstPath := createWorktreeViaWt(t, repo, "reuse-test")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--reuse", "--worktree-name", "reuse-test")

	reusePath := strings.TrimSpace(r.Stdout)
	if reusePath != firstPath {
		t.Errorf("--reuse path mismatch: got %q, want %q", reusePath, firstPath)
	}
}

func TestCreate_ReuseCreatesWhenNoCollision(t *testing.T) {
	repo := createTestRepo(t)

	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--reuse", "--worktree-name", "reuse-fresh", "--worktree-init", "false")
	assertWorktreeExists(t, repo, "reuse-fresh")
}

func TestCreate_ReuseRequiresWorktreeName(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "create", "--non-interactive", "--reuse")
	if r.ExitCode == 0 {
		t.Error("expected failure: --reuse without --name")
	}
	assertContains(t, r.Stderr, "--reuse requires --name")
}

func TestCreate_ErrorOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	r := runWt(t, dir, nil, "create")
	if r.ExitCode == 0 {
		t.Error("expected failure outside git repo")
	}
	assertContains(t, r.Stderr, "Not a git repository")
}

func TestCreate_InvalidBranchName(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "bad-branch", "refs/invalid..name")
	if r.ExitCode == 0 {
		t.Error("expected failure with invalid branch name")
	}
	// No partial worktree directory should be left behind
	assertDirNotExists(t, worktreePath(repo, "bad-branch"))
}

func TestCreate_BranchesOffCurrentBranch(t *testing.T) {
	repo := createTestRepo(t)

	// Create a feature branch with a unique commit
	gitRun(t, repo, "checkout", "-b", "feature/has-marker")
	os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("marker"), 0644)
	gitRun(t, repo, "add", "marker.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add marker")
	featureCommit := gitRun(t, repo, "rev-parse", "HEAD")

	// Stay on feature branch and create exploratory worktree
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "from-feature", "--worktree-init", "false")

	wtPath := strings.TrimSpace(r.Stdout)

	// The worktree should have the marker file (branched off feature, not main)
	assertFileExists(t, filepath.Join(wtPath, "marker.txt"))

	// The worktree's HEAD should match the feature commit
	wtCommit := gitRun(t, wtPath, "rev-parse", "HEAD")
	if wtCommit != featureCommit {
		t.Errorf("worktree HEAD %s != feature commit %s", wtCommit, featureCommit)
	}
}

func TestCreate_ExploratoryFromMainStillWorks(t *testing.T) {
	repo := createTestRepo(t)

	mainCommit := gitRun(t, repo, "rev-parse", "HEAD")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "from-main", "--worktree-init", "false")

	wtPath := strings.TrimSpace(r.Stdout)
	wtCommit := gitRun(t, wtPath, "rev-parse", "HEAD")
	if wtCommit != mainCommit {
		t.Errorf("worktree HEAD %s != main commit %s", wtCommit, mainCommit)
	}
}

func TestCreate_ExistingBranchUnaffectedByCurrentBranch(t *testing.T) {
	repo := createTestRepo(t)

	// Create branch-a with unique content
	gitRun(t, repo, "checkout", "-b", "branch-a")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("branch-a content"), 0644)
	gitRun(t, repo, "add", "a.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add a.txt")
	gitRun(t, repo, "checkout", "main")

	// Create branch-b with different content
	gitRun(t, repo, "checkout", "-b", "branch-b")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("branch-b content"), 0644)
	gitRun(t, repo, "add", "b.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add b.txt")

	// While on branch-b, check out branch-a into a worktree
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "checkout-a", "--checkout", "branch-a")

	wtPath := strings.TrimSpace(r.Stdout)
	assertFileExists(t, filepath.Join(wtPath, "a.txt"))
	if _, err := os.Stat(filepath.Join(wtPath, "b.txt")); err == nil {
		t.Error("worktree should not have b.txt (it's from branch-b)")
	}
}

func TestCreate_CorrectDirectoryStructure(t *testing.T) {
	repo := createTestRepo(t)

	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "test-structure", "--worktree-init", "false")

	expected := worktreePath(repo, "test-structure")
	assertDirExists(t, expected)
}

func TestCreate_PorcelainStdoutOnlyPath(t *testing.T) {
	repo := createTestRepo(t)

	cmd := exec.Command(wtBinary, "create", "--non-interactive", "--worktree-name", "porcelain-test", "--worktree-init", "false")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "NO_COLOR=1")

	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("wt create failed (exit %d): %s", exitErr.ExitCode(), exitErr.Stderr)
		}
		t.Fatalf("wt create failed: %v", err)
	}

	// stdout should be exactly one line: the worktree path
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line of stdout, got %d: %q", len(lines), string(stdout))
	}
	assertDirExists(t, strings.TrimSpace(string(stdout)))
}

func TestCreate_InitScriptRuns(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)

	// Commit init script so worktrees see it
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	r := runWtSuccess(t, repo, []string{"WORKTREE_INIT_SCRIPT=scripts/worktree-init.sh"}, "create", "--non-interactive", "--worktree-name", "init-run-test")

	wtPath := strings.TrimSpace(r.Stdout)
	assertFileExists(t, filepath.Join(wtPath, ".init-script-ran"))
}

func TestCreate_ReuseRunsInitScript(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)

	// Commit init script so worktrees see it
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	// Pre-create the worktree (without init script so .init-script-ran is absent)
	existingPath := createWorktreeViaWt(t, repo, "reuse-init-test")

	// Verify .init-script-ran is NOT present yet (init was skipped during pre-create)
	if _, err := os.Stat(filepath.Join(existingPath, ".init-script-ran")); err == nil {
		t.Fatal("expected .init-script-ran to be absent before reuse")
	}

	// Now run wt create --reuse — should run the init script on the existing worktree
	r := runWtSuccess(t, repo, []string{"WORKTREE_INIT_SCRIPT=scripts/worktree-init.sh"},
		"create", "--non-interactive", "--reuse", "--worktree-name", "reuse-init-test")

	// Stdout should still print the worktree path (porcelain contract)
	reusedPath := strings.TrimSpace(r.Stdout)
	if reusedPath != existingPath {
		t.Errorf("--reuse path mismatch: got %q, want %q", reusedPath, existingPath)
	}

	// Init script should have run and created .init-script-ran
	assertFileExists(t, filepath.Join(existingPath, ".init-script-ran"))
}

func TestCreate_ReuseInitSkippedWhenWorktreeInitFalse(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)

	// Commit init script so worktrees see it
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	// Pre-create the worktree (without init script so .init-script-ran is absent)
	existingPath := createWorktreeViaWt(t, repo, "reuse-noinit-test")

	// Run wt create --reuse with --worktree-init false — init should NOT run
	runWtSuccess(t, repo, []string{"WORKTREE_INIT_SCRIPT=scripts/worktree-init.sh"},
		"create", "--non-interactive", "--reuse", "--worktree-name", "reuse-noinit-test",
		"--worktree-init", "false")

	// Init script should NOT have run
	if _, err := os.Stat(filepath.Join(existingPath, ".init-script-ran")); err == nil {
		t.Error("init script should not have run with --worktree-init false on --reuse path")
	}
}

func TestCreate_InitScriptSkippedWhenFalse(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "no-init-test", "--worktree-init", "false")

	wtPath := strings.TrimSpace(r.Stdout)
	if _, err := os.Stat(filepath.Join(wtPath, ".init-script-ran")); err == nil {
		t.Error("init script should not have run with --worktree-init false")
	}
}

func TestCreate_ImmediatelyListable(t *testing.T) {
	repo := createTestRepo(t)

	createWorktreeViaWt(t, repo, "immediate-list")

	r := runWtSuccess(t, repo, nil, "list")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "immediate-list")
}

func TestCreate_BaseNewBranch(t *testing.T) {
	repo := createTestRepo(t)

	// Create a feature branch with a marker commit
	gitRun(t, repo, "checkout", "-b", "feature-A")
	os.WriteFile(filepath.Join(repo, "marker-A.txt"), []byte("from feature-A"), 0644)
	gitRun(t, repo, "add", "marker-A.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add marker-A")
	featureATip := gitRun(t, repo, "rev-parse", "HEAD")
	gitRun(t, repo, "checkout", "main")

	// Create a new branch based on feature-A
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "base-new",
		"--worktree-init", "false", "feature-B", "--base", "feature-A")

	wtPath := strings.TrimSpace(r.Stdout)

	// The worktree should have the marker file from feature-A
	assertFileExists(t, filepath.Join(wtPath, "marker-A.txt"))

	// The worktree's HEAD should match feature-A's tip
	wtCommit := gitRun(t, wtPath, "rev-parse", "HEAD")
	if wtCommit != featureATip {
		t.Errorf("worktree HEAD %s != feature-A tip %s", wtCommit, featureATip)
	}

	// The branch name should be feature-B
	wtBranch := gitRun(t, wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	if wtBranch != "feature-B" {
		t.Errorf("expected branch feature-B, got %s", wtBranch)
	}
}

func TestCreate_BaseExploratoryWorktree(t *testing.T) {
	repo := createTestRepo(t)

	// Create a feature branch with a marker commit
	gitRun(t, repo, "checkout", "-b", "feature-A")
	os.WriteFile(filepath.Join(repo, "marker-A.txt"), []byte("from feature-A"), 0644)
	gitRun(t, repo, "add", "marker-A.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add marker-A")
	featureATip := gitRun(t, repo, "rev-parse", "HEAD")
	gitRun(t, repo, "checkout", "main")

	// Create an exploratory worktree based on feature-A
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "explore-base",
		"--worktree-init", "false", "--base", "feature-A")

	wtPath := strings.TrimSpace(r.Stdout)

	// The worktree should have the marker file from feature-A
	assertFileExists(t, filepath.Join(wtPath, "marker-A.txt"))

	// The worktree's HEAD should match feature-A's tip
	wtCommit := gitRun(t, wtPath, "rev-parse", "HEAD")
	if wtCommit != featureATip {
		t.Errorf("worktree HEAD %s != feature-A tip %s", wtCommit, featureATip)
	}
}

// TestCreate_CheckoutWithBaseConflict asserts --checkout and --base are
// mutually exclusive (--base is the start-point for a NEW branch; --checkout
// targets an EXISTING one). This replaces the removed warn-and-ignore behavior
// that --base-on-an-existing-branch used to have.
func TestCreate_CheckoutWithBaseConflict(t *testing.T) {
	repo := createTestRepo(t)

	// Create an existing local branch to --checkout
	gitRun(t, repo, "checkout", "-b", "existing-branch")
	gitRun(t, repo, "checkout", "main")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "conflict-wt",
		"--worktree-init", "false", "--checkout", "existing-branch", "--base", "main")

	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "--base cannot be combined with --checkout")
	// No worktree should have been created.
	assertWorktreeNotExists(t, repo, "conflict-wt")
}

// TestCreate_PositionalExistingLocalBranchRejected asserts that naming an
// existing local branch positionally fails with exit 2 and the --checkout
// hint, leaving no state behind (the positional creates NEW branches only).
func TestCreate_PositionalExistingLocalBranchRejected(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "existing-branch")
	gitRun(t, repo, "checkout", "main")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "reject-local",
		"--worktree-init", "false", "existing-branch")

	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "Branch 'existing-branch' already exists")
	assertContains(t, r.Stderr, "wt create --checkout existing-branch")
	// No worktree left behind.
	assertWorktreeNotExists(t, repo, "reject-local")
}

// TestCreate_PositionalExistingRemoteBranchRejected asserts the exact danger
// case from the backlog: a positional naming a remote-only shared branch is
// rejected (remote-only existence counts), pointing at --checkout.
func TestCreate_PositionalExistingRemoteBranchRejected(t *testing.T) {
	repo := createTestRepo(t)

	// Create a branch, push it, then delete locally (remote-only).
	gitRun(t, repo, "checkout", "-b", "remote-only")
	os.WriteFile(filepath.Join(repo, "remote-file.txt"), []byte("remote"), 0644)
	gitRun(t, repo, "add", "remote-file.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Remote commit")
	gitRun(t, repo, "push", "-q", "-u", "origin", "remote-only")
	gitRun(t, repo, "checkout", "main")
	gitRun(t, repo, "branch", "-D", "remote-only")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "reject-remote",
		"--worktree-init", "false", "remote-only")

	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "Branch 'remote-only' already exists")
	assertContains(t, r.Stderr, "wt create --checkout remote-only")
	assertWorktreeNotExists(t, repo, "reject-remote")
}

func TestCreate_BaseInvalidRef(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "bad-base",
		"--worktree-init", "false", "new-branch", "--base", "nonexistent-ref")

	if r.ExitCode == 0 {
		t.Error("expected failure with invalid --base ref")
	}
	assertContains(t, r.Stderr, "Invalid --base ref")

	// No worktree should have been created
	assertDirNotExists(t, worktreePath(repo, "bad-base"))
	assertBranchNotExists(t, repo, "new-branch")
}

// TestCreate_BaseValidatedEvenWithExistingPositional asserts --base is now
// validated whenever it is set (and --reuse is not), even when the positional
// names an already-existing branch. The old warn-and-ignore behavior (which
// skipped --base validation for existing branches) is gone — the positional is
// always a NEW branch, so --base always applies and an invalid ref fails.
func TestCreate_BaseValidatedEvenWithExistingPositional(t *testing.T) {
	repo := createTestRepo(t)

	// Create an existing branch with a commit
	gitRun(t, repo, "checkout", "-b", "existing-branch")
	os.WriteFile(filepath.Join(repo, "existing.txt"), []byte("existing"), 0644)
	gitRun(t, repo, "add", "existing.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add existing.txt")
	gitRun(t, repo, "checkout", "main")

	r := runWt(t, repo, nil, "create", "--non-interactive", "--worktree-name", "exist-branch",
		"--worktree-init", "false", "existing-branch", "--base", "nonexistent-ref")

	// --base is validated up front now, so this fails on the invalid ref.
	assertExitCode(t, r, 2)
	assertContains(t, r.Stderr, "Invalid --base ref")
	assertWorktreeNotExists(t, repo, "exist-branch")
}

func TestCreate_BaseInvalidRefWithReuse(t *testing.T) {
	repo := createTestRepo(t)

	// Create a worktree first
	firstPath := createWorktreeViaWt(t, repo, "reuse-invalid-base")

	// Attempt to reuse the existing worktree with an invalid --base; --reuse should take precedence
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--reuse", "--worktree-name", "reuse-invalid-base",
		"--base", "nonexistent-ref")

	reusedPath := strings.TrimSpace(r.Stdout)
	if reusedPath != firstPath {
		t.Errorf("--reuse path mismatch with invalid --base: got %q, want %q", reusedPath, firstPath)
	}
}

func TestCreate_BaseWithReuse(t *testing.T) {
	repo := createTestRepo(t)

	// Create a worktree first
	firstPath := createWorktreeViaWt(t, repo, "reuse-base")

	// Create a branch to use as --base
	gitRun(t, repo, "checkout", "-b", "base-branch")
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base"), 0644)
	gitRun(t, repo, "add", "base.txt")
	gitRun(t, repo, "commit", "-q", "-m", "Add base.txt")
	gitRun(t, repo, "checkout", "main")

	// Try to create with --reuse and --base — reuse should take precedence
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--reuse", "--worktree-name", "reuse-base",
		"--base", "base-branch")

	reusePath := strings.TrimSpace(r.Stdout)
	if reusePath != firstPath {
		t.Errorf("--reuse path mismatch: got %q, want %q", reusePath, firstPath)
	}
}

func TestCreate_BaseDoesNotAffectExistingBehavior(t *testing.T) {
	repo := createTestRepo(t)

	mainCommit := gitRun(t, repo, "rev-parse", "HEAD")

	// Create a new branch WITHOUT --base — should branch from HEAD (main)
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "no-base",
		"--worktree-init", "false", "no-base-branch")

	wtPath := strings.TrimSpace(r.Stdout)
	wtCommit := gitRun(t, wtPath, "rev-parse", "HEAD")
	if wtCommit != mainCommit {
		t.Errorf("worktree HEAD %s != main commit %s (without --base, should branch from HEAD)", wtCommit, mainCommit)
	}
}

// createFailingInitScript writes a committed init script that streams a
// marker line and exits 1. Returns the env override caller should pass to
// runWt so WORKTREE_INIT_SCRIPT points at it.
func createFailingInitScript(t *testing.T, repo string) []string {
	t.Helper()
	scriptDir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	script := filepath.Join(scriptDir, "init-fail.sh")
	content := "#!/usr/bin/env bash\necho 'INIT_FAIL_MARKER' >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitRun(t, repo, "add", "scripts/init-fail.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add failing init script")
	return []string{"WORKTREE_INIT_SCRIPT=scripts/init-fail.sh"}
}

func TestCreate_InitFailureKeepsWorktree_Exploratory(t *testing.T) {
	repo := createTestRepo(t)
	env := createFailingInitScript(t, repo)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "explore-fail",
		"--worktree-open", "skip")

	// Exit code must be ExitInitFailed (7), not the legacy ExitGeneralError (1).
	assertExitCode(t, r, 7)
	// Worktree directory survives.
	assertWorktreeExists(t, repo, "explore-fail")
	// Branch (matches worktree name for exploratory) survives.
	assertBranchExists(t, repo, "explore-fail")
}

func TestCreate_InitFailureKeepsWorktree_ExistingBranch(t *testing.T) {
	repo := createTestRepo(t)
	env := createFailingInitScript(t, repo)

	// Pre-create the branch so we go through the existing-local-branch path.
	gitRun(t, repo, "branch", "feature/keep-on-fail")

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "branch-fail",
		"--worktree-open", "skip",
		"--checkout", "feature/keep-on-fail")

	assertExitCode(t, r, 7)
	assertWorktreeExists(t, repo, "branch-fail")
	assertBranchExists(t, repo, "feature/keep-on-fail")
}

func TestCreate_InitFailureBannerHasRetryHint(t *testing.T) {
	repo := createTestRepo(t)
	env := createFailingInitScript(t, repo)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "banner-test",
		"--worktree-open", "skip")

	assertExitCode(t, r, 7)
	wtPath := worktreePath(repo, "banner-test")
	// Banner contents — shape, not byte-equality per spec.
	assertContains(t, r.Stderr, wtPath)
	assertContains(t, r.Stderr, "wt init")
	assertContains(t, r.Stderr, "wt delete 'banner-test'")
	assertContains(t, r.Stderr, "&&")
}

// TestCreate_InitFailureBannerHasGoHint asserts the banner now points the user
// at `wt go '<name>'` for the kept worktree, on the non-interactive path. The
// hint lives in PrintInitFailureBanner so it appears on every caller path.
func TestCreate_InitFailureBannerHasGoHint(t *testing.T) {
	repo := createTestRepo(t)
	env := createFailingInitScript(t, repo)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "go-hint-test",
		"--worktree-open", "skip")

	assertExitCode(t, r, 7)
	assertContains(t, r.Stderr, "wt go 'go-hint-test'")
}

// TestCreate_InitFailureNonInteractive_NoPrompt asserts the non-interactive
// init-failure path preserves today's exact behavior: banner + exit 7 with NO
// open-anyway prompt. The interactivity gate is !nonInteractive AND a TTY, so
// --non-interactive must never reach ConfirmYesNo.
func TestCreate_InitFailureNonInteractive_NoPrompt(t *testing.T) {
	repo := createTestRepo(t)
	env := createFailingInitScript(t, repo)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "noprompt-test",
		"--worktree-open", "skip")

	assertExitCode(t, r, 7)
	// The banner is shown (with the go hint)...
	assertContains(t, r.Stderr, "wt go 'noprompt-test'")
	// ...but the open-anyway prompt is NOT, and the Open phase did not run.
	assertNotContains(t, r.Stdout, "Continue and open the worktree anyway?")
	assertNotContains(t, r.Stderr, "Continue and open the worktree anyway?")
	assertNotContains(t, r.Stdout, "cd -- '")
	// Worktree survives.
	assertWorktreeExists(t, repo, "noprompt-test")
}

func TestCreate_OpenHereSuppressesPath(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, []string{"HOME=" + t.TempDir()}, "create", "--non-interactive",
		"--worktree-name", "open-here-test",
		"--worktree-init", "false",
		"--worktree-open", "open_here")

	// stdout should contain the cd line
	assertContains(t, r.Stdout, `cd -- '`)

	// stdout should NOT contain a trailing bare path line (the suppressed fmt.Println)
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) != 1 {
		t.Errorf("expected exactly 1 stdout line (the cd command), got %d: %q", len(lines), r.Stdout)
	}

	// The single line should start with "cd -- "
	if !strings.HasPrefix(lines[0], "cd -- ") {
		t.Errorf("expected stdout line to start with 'cd -- ', got %q", lines[0])
	}

	// stderr should still contain the creation message
	assertContains(t, r.Stderr, "Created worktree:")
}

func TestCreate_WorktreeOpenDefault(t *testing.T) {
	repo := createTestRepo(t)

	// Seed a default app deterministically so this test does not depend on
	// what happens to be installed on the host (a bare CI runner has no
	// editor/terminal/clipboard, so DetectDefaultApp would otherwise return
	// "no default app detected" and the marker assertion below would fail).
	//
	// On Linux, appAvailable() treats an app as installed when a matching
	// .desktop file exists under $HOME/.local/share/applications. We point
	// HOME at a temp dir and plant code.desktop there, making VSCode the
	// resolved default regardless of the real environment. The WT_TEST_NO_LAUNCH
	// seam (default-on in runWt) still prevents any real launch.
	//
	// On macOS the .desktop seam does not apply (detection uses mdfind on a
	// bundle ID); developer machines there generally have a real editor
	// installed, so the default resolves anyway. If no default can be resolved
	// on any platform, the test skips rather than producing a misleading
	// failure — the marker assertion only carries weight once a default exists.
	fakeHome := t.TempDir()
	if runtime.GOOS != "windows" {
		appsDir := filepath.Join(fakeHome, ".local", "share", "applications")
		if err := os.MkdirAll(appsDir, 0o755); err != nil {
			t.Fatalf("seed apps dir: %v", err)
		}
		desktop := "[Desktop Entry]\nName=Visual Studio Code\nExec=code\nType=Application\n"
		if err := os.WriteFile(filepath.Join(appsDir, "code.desktop"), []byte(desktop), 0o644); err != nil {
			t.Fatalf("seed code.desktop: %v", err)
		}
	}

	// --worktree-open default should resolve via DetectDefaultApp and reach
	// OpenInApp under the launch guard — it must not panic or treat "default"
	// as a literal app name.
	r := runWt(t, repo, []string{"HOME=" + fakeHome}, "create", "--non-interactive",
		"--worktree-name", "default-open-test",
		"--worktree-init", "false",
		"--worktree-open", "default")

	// The worktree should be created regardless of whether the default app opened
	assertContains(t, r.Stderr, "Created worktree:")

	// Guard against the old behavior where "default" would be treated as a
	// literal app name and produce a ResolveApp warning
	if strings.Contains(r.Stderr, "app 'default' not found") {
		t.Errorf("expected --worktree-open=default to use the default-app code path, got stderr: %q", r.Stderr)
	}

	// If no default app could be resolved on this platform, the marker
	// assertion is meaningless — skip rather than fail. On Linux/CI the seeded
	// code.desktop guarantees a default, so this skip should not trigger there.
	if strings.Contains(r.Stderr, "no default app detected") {
		t.Skip("no default app resolvable on this platform; skipping launch-guard marker assertion")
	}

	// The default-app path must reach OpenInApp under the test launch guard.
	// If a real launch ever leaks past the WT_TEST_NO_LAUNCH=1 seam, the
	// marker will be missing and this test will fail — preventing the
	// VSCode-during-test regression class.
	assertContains(t, r.Stderr, "[wt-test-no-launch]")
}

// TestCreate_DirtyStateWarningCopy asserts the corrected dirty-state warning
// string. The checks (HasUncommittedChanges/HasUntrackedFiles) run in the
// process CWD — any checkout, linked or main — so the copy must describe the
// "current worktree", not the "main repo". Running interactively (no
// --non-interactive) with a dirty repo reaches the dirty-state menu; feeding
// "3\n" (Abort) via the non-TTY fallback menu returns cleanly BEFORE any
// worktree is created, so the test leaks no side effects to the host.
func TestCreate_DirtyStateWarningCopy(t *testing.T) {
	repo := createTestRepo(t)

	// Make the current checkout dirty with an untracked file.
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("uncommitted\n"), 0644); err != nil {
		t.Fatalf("WriteFile dirty.txt: %v", err)
	}

	r := runWtStdin(t, repo, nil, "3\n", "create", "--worktree-name", "dirty-abort-test")

	// The corrected copy is on stderr (human-facing); the stale copy is gone.
	assertContains(t, r.Stderr, "current worktree has uncommitted changes")
	assertNotContains(t, r.Stderr, "main repo has uncommitted changes")

	// Abort returns before creating anything — no worktree leaked.
	assertWorktreeNotExists(t, repo, "dirty-abort-test")
}

// ---------- Intuitive flag names (change 59u8) ----------

// TestCreate_NameFlagAndShort verifies the new --name flag and its -n short
// create a named worktree (equivalent to the deprecated --worktree-name), with
// no deprecation warning on the happy path.
func TestCreate_NameFlagAndShort(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "-n", "short-name", "--no-init", "-o", "skip")
	assertWorktreeExists(t, repo, "short-name")
	assertNotContains(t, r.Stderr, "deprecated")

	r = runWtSuccess(t, repo, nil, "create", "--non-interactive", "--name", "long-name", "--no-init", "-o", "skip")
	assertWorktreeExists(t, repo, "long-name")
	assertNotContains(t, r.Stderr, "deprecated")
}

// TestCreate_NewAlias verifies `wt new` invokes `wt create` identically.
func TestCreate_NewAlias(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "new", "--non-interactive", "-n", "via-new", "--no-init", "-o", "skip")
	assertWorktreeExists(t, repo, "via-new")
	// stdout is the machine path (porcelain contract), unchanged by the alias.
	if strings.TrimSpace(r.Stdout) != worktreePath(repo, "via-new") {
		t.Errorf("wt new stdout path mismatch: got %q", strings.TrimSpace(r.Stdout))
	}
}

// TestCreate_NoInitFlagSkipsInit verifies the new real-bool --no-init skips the
// init script (parity with the deprecated --worktree-init false).
func TestCreate_NoInitFlagSkipsInit(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	r := runWtSuccess(t, repo, []string{"WORKTREE_INIT_SCRIPT=scripts/worktree-init.sh"},
		"create", "--non-interactive", "-n", "noinit-flag", "--no-init", "-o", "skip")
	wtPath := strings.TrimSpace(r.Stdout)
	if _, err := os.Stat(filepath.Join(wtPath, ".init-script-ran")); err == nil {
		t.Error("init script should not have run with --no-init")
	}
}

// TestCreate_NoInitDefaultRunsInit verifies that WITHOUT --no-init (and without
// the old string flag) the init script still runs — the string→bool conversion
// preserves default behavior.
func TestCreate_NoInitDefaultRunsInit(t *testing.T) {
	repo := createTestRepo(t)
	createInitScript(t, repo)
	gitRun(t, repo, "add", "scripts/worktree-init.sh")
	gitRun(t, repo, "commit", "-q", "-m", "Add init script")

	r := runWtSuccess(t, repo, []string{"WORKTREE_INIT_SCRIPT=scripts/worktree-init.sh"},
		"create", "--non-interactive", "-n", "init-default", "-o", "skip")
	wtPath := strings.TrimSpace(r.Stdout)
	assertFileExists(t, filepath.Join(wtPath, ".init-script-ran"))
}

// TestCreate_OpenFlagShort verifies the new --open/-o flag controls the open
// phase (skip here) equivalently to the deprecated --worktree-open.
func TestCreate_OpenFlagShort(t *testing.T) {
	repo := createTestRepo(t)

	// -o skip suppresses the Open phase separator (no open phase runs).
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "-n", "open-skip", "--no-init", "-o", "skip")
	assertNotContains(t, r.Stderr, "-- Open")
	assertWorktreeExists(t, repo, "open-skip")
}

// TestCreate_DeprecatedFlagsStillWork verifies the deprecated create flags still
// behave as before AND emit a stderr deprecation warning naming the new flag.
func TestCreate_DeprecatedFlagsStillWork(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--non-interactive",
		"--worktree-name", "legacy-create", "--worktree-init", "false", "--worktree-open", "skip")
	assertWorktreeExists(t, repo, "legacy-create")
	assertContains(t, r.Stderr, "deprecated")
	// stdout stays the clean machine path — warnings are stderr-only.
	if strings.TrimSpace(r.Stdout) != worktreePath(repo, "legacy-create") {
		t.Errorf("stdout should be the bare path; got %q", strings.TrimSpace(r.Stdout))
	}
}

// TestCreate_HelpHidesDeprecatedShowsNew verifies `wt create --help` shows the
// new flags and hides the deprecated aliases.
func TestCreate_HelpHidesDeprecatedShowsNew(t *testing.T) {
	repo := createTestRepo(t)

	r := runWtSuccess(t, repo, nil, "create", "--help")
	for _, want := range []string{"--name", "--open", "--no-init"} {
		assertContains(t, r.Stdout, want)
	}
	for _, hidden := range []string{"--worktree-name", "--worktree-open", "--worktree-init"} {
		assertNotContains(t, r.Stdout, hidden)
	}
}
