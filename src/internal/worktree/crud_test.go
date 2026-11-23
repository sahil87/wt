package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOut runs git in dir and returns trimmed stdout, failing the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// addCommit makes an empty commit in dir.
func addCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %s: %v", out, err)
	}
}

func TestResolveCommit(t *testing.T) {
	dir := setupGitRepo(t)
	first := gitOut(t, dir, "rev-parse", "HEAD")
	addCommit(t, dir, "second")
	head := gitOut(t, dir, "rev-parse", "HEAD")
	gitOut(t, dir, "tag", "v1", first)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cases := map[string]string{
		first:    first,
		"HEAD":   head,
		"HEAD~1": first,
		"main":   head,
		"v1":     first,
	}
	for ref, want := range cases {
		got, err := ResolveCommit(ref)
		if err != nil {
			t.Errorf("ResolveCommit(%q) error: %v", ref, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveCommit(%q) = %q, want %q", ref, got, want)
		}
	}

	for _, bad := range []string{"no-such-ref", "deadbeefdeadbeef", ""} {
		if _, err := ResolveCommit(bad); err == nil {
			t.Errorf("ResolveCommit(%q) = nil error, want failure", bad)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := ShortSHA("0123456789abcdef"); got != "0123456" {
		t.Errorf("ShortSHA = %q, want 0123456", got)
	}
	if got := ShortSHA("abc"); got != "abc" {
		t.Errorf("ShortSHA(short) = %q, want abc", got)
	}
}

func TestCreateTempWorktree_DetachedAtSHA(t *testing.T) {
	dir := setupGitRepo(t)
	first := gitOut(t, dir, "rev-parse", "HEAD")
	addCommit(t, dir, "second")
	branchesBefore := gitOut(t, dir, "branch", "--list")

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	ctx, err := GetRepoContext()
	if err != nil {
		t.Fatal(err)
	}
	rb := NewRollback()
	path, err := CreateTempWorktree(first, "tmp-x", ctx, rb)
	if err != nil {
		t.Fatalf("CreateTempWorktree: %v", err)
	}
	if want := filepath.Join(ctx.WorktreesDir, "tmp-x"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if got := gitOut(t, path, "rev-parse", "HEAD"); got != first {
		t.Errorf("worktree HEAD = %q, want %q", got, first)
	}
	if err := exec.Command("git", "-C", path, "symbolic-ref", "-q", "HEAD").Run(); err == nil {
		t.Error("worktree HEAD is on a branch; want detached")
	}
	if got := gitOut(t, dir, "branch", "--list"); got != branchesBefore {
		t.Errorf("branches changed: before %q, after %q", branchesBefore, got)
	}
	if cfg := gitOut(t, dir, "config", "--local", "--list"); strings.Contains(cfg, "tmp-x") {
		t.Errorf("git config mentions the temp worktree:\n%s", cfg)
	}

	// Rollback registers only the worktree removal — no `branch -D`.
	cmds := rb.Commands()
	if len(cmds) != 1 || cmds[0][1] != "worktree" || cmds[0][2] != "remove" {
		t.Errorf("rollback commands = %v, want a single `git worktree remove`", cmds)
	}
	rb.Execute()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("rollback left %s behind", path)
	}
}

func TestCreateTempWorktree_GitFailureLeavesNothing(t *testing.T) {
	dir := setupGitRepo(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	ctx, err := GetRepoContext()
	if err != nil {
		t.Fatal(err)
	}
	rb := NewRollback()
	// A syntactically valid but nonexistent object fails `git worktree add`.
	_, err = CreateTempWorktree(strings.Repeat("0", 40), "tmp-bad", ctx, rb)
	if err == nil {
		t.Fatal("expected error for nonexistent commit")
	}
	if len(rb.Commands()) != 0 {
		t.Errorf("rollback registered %v for a failed add", rb.Commands())
	}
	if _, err := os.Stat(filepath.Join(ctx.WorktreesDir, "tmp-bad")); !os.IsNotExist(err) {
		t.Error("failed add left a directory behind")
	}
}

func TestRunWorktreeSetupOutcome(t *testing.T) {
	repo := t.TempDir()
	wtDir := t.TempDir()
	writeScript := func(name, body string) string {
		p := filepath.Join(repo, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return name // init scripts resolve relative to the repo root
	}
	okScript := writeScript("ok.sh", "exit 0")
	failScript := writeScript("fail.sh", "exit 1")

	if out, err := RunWorktreeSetupOutcome(wtDir, "__wt_missing_script__ noop", false, repo, nil); out != InitNotFoundSkip || err != nil {
		t.Errorf("not-found: got (%q, %v)", out, err)
	}
	if out, err := RunWorktreeSetupOutcome(wtDir, okScript, false, repo, nil); out != InitOK || err != nil {
		t.Errorf("ok: got (%q, %v)", out, err)
	}
	if out, err := RunWorktreeSetupOutcome(wtDir, failScript, false, repo, nil); out != InitFailed || err == nil {
		t.Errorf("failed: got (%q, %v)", out, err)
	}
	// The thin wrapper keeps its error-only contract.
	if err := RunWorktreeSetupWithObserver(wtDir, failScript, false, repo, nil); err == nil {
		t.Error("RunWorktreeSetupWithObserver should still return the init error")
	}
}

// newRollbackFixture returns a repo, a RepoContext for it, and chdirs into it
// (restored on cleanup) since the create helpers run git in the cwd.
func newRollbackFixture(t *testing.T) (string, *RepoContext) {
	t.Helper()
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	return dir, &RepoContext{RepoRoot: dir, RepoName: filepath.Base(dir), WorktreesDir: dir + ".worktrees"}
}

func branchExists(t *testing.T, dir, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestCreateWorktree_RollbackRemovesWorktreeBeforeBranch proves the registered
// undo leaves neither the worktree nor the branch behind: Rollback is LIFO and
// git refuses to delete a branch that is still checked out, so the worktree
// must go first.
func TestCreateWorktree_RollbackRemovesWorktreeBeforeBranch(t *testing.T) {
	dir, ctx := newRollbackFixture(t)

	for name, create := range map[string]func(rb *Rollback) (string, error){
		"positional": func(rb *Rollback) (string, error) {
			return CreateNewBranchWorktree("nb", "wt-nb", ctx, rb, "HEAD", "HEAD", "")
		},
		"exploratory": func(rb *Rollback) (string, error) {
			return CreateExploratoryWorktree("wt-ex", ctx, rb, "HEAD", "HEAD", "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			rb := NewRollback()
			path, err := create(rb)
			if err != nil {
				t.Fatal(err)
			}
			cmds := rb.Commands()
			wtIdx, brIdx := -1, -1
			for i, c := range cmds {
				if len(c) > 2 && c[1] == "worktree" && c[2] == "remove" {
					wtIdx = i
				}
				if len(c) > 2 && c[1] == "branch" && c[2] == "-D" {
					brIdx = i
				}
			}
			if wtIdx < 0 || brIdx < 0 || wtIdx < brIdx {
				t.Fatalf("worktree remove must be registered after branch -D (LIFO runs it first): %v", cmds)
			}
			rb.Execute()
			if _, err := os.Stat(path); err == nil {
				t.Errorf("worktree %s survived rollback", path)
			}
			branch := filepath.Base(path)[len("wt-"):]
			if name == "exploratory" {
				branch = filepath.Base(path)
			}
			if branchExists(t, dir, branch) {
				t.Errorf("branch %q survived rollback", branch)
			}
		})
	}
}

// TestCreateWorktree_SetUpstreamFailureRollsBack forces SetUpstream to fail
// (the upstream ref does not exist) and asserts the create leaves nothing.
func TestCreateWorktree_SetUpstreamFailureRollsBack(t *testing.T) {
	dir, ctx := newRollbackFixture(t)

	rb := NewRollback()
	path, err := CreateNewBranchWorktree("nb", "wt-nb", ctx, rb, "HEAD", "HEAD", "no-such-upstream")
	if err == nil {
		t.Fatal("expected set-upstream failure")
	}
	if path != "" {
		t.Errorf("path = %q, want empty on failure", path)
	}
	if _, statErr := os.Stat(filepath.Join(ctx.WorktreesDir, "wt-nb")); statErr == nil {
		t.Error("worktree dir survived the failed create")
	}
	if branchExists(t, dir, "nb") {
		t.Error("branch nb survived the failed create")
	}
}

// configGet returns a git config value, or "" and false when the key is unset.
func configGet(t *testing.T, dir, key string) (string, bool) {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err == nil
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestCreateWorktree_RecordsBase(t *testing.T) {
	dir, ctx := newRollbackFixture(t)
	sha := headSHA(t, dir)

	rb := NewRollback()
	if _, err := CreateNewBranchWorktree("nb", "wt-nb", ctx, rb, "main", sha, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateExploratoryWorktree("wt-ex", ctx, rb, "HEAD-label", sha, ""); err != nil {
		t.Fatal(err)
	}

	for branch, ref := range map[string]string{"nb": "main", "wt-ex": "HEAD-label"} {
		if got, ok := configGet(t, dir, "branch."+branch+".wt-base"); !ok || got != ref {
			t.Errorf("%s wt-base = %q (set=%v), want %q", branch, got, ok, ref)
		}
		if got, ok := configGet(t, dir, "branch."+branch+".wt-base-sha"); !ok || got != sha {
			t.Errorf("%s wt-base-sha = %q (set=%v), want %q", branch, got, ok, sha)
		}
	}
}

func TestCreateWorktree_EmptyStartPointRecordsNothing(t *testing.T) {
	dir, ctx := newRollbackFixture(t)

	rb := NewRollback()
	if _, err := CreateNewBranchWorktree("nb", "wt-nb", ctx, rb, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateExploratoryWorktree("wt-ex", ctx, rb, "", "", ""); err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"nb", "wt-ex"} {
		for _, key := range []string{"wt-base", "wt-base-sha"} {
			if got, ok := configGet(t, dir, "branch."+branch+"."+key); ok {
				t.Errorf("branch.%s.%s = %q, want unset", branch, key, got)
			}
		}
	}
}

func TestCheckoutBranchWorktree_RecordsNothing(t *testing.T) {
	dir, ctx := newRollbackFixture(t)
	if out, err := exec.Command("git", "-C", dir, "branch", "feat").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}

	if _, err := CheckoutBranchWorktree("feat", "wt-feat", ctx, NewRollback()); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"wt-base", "wt-base-sha"} {
		if got, ok := configGet(t, dir, "branch.feat."+key); ok {
			t.Errorf("branch.feat.%s = %q, want unset", key, got)
		}
	}
}

// TestCreateWorktree_RecordBaseFailureRollsBack forces the record write to
// fail (a stale .git/config.lock; a local base sets no upstream first) and
// asserts a *RecordBaseError with nothing left behind.
func TestCreateWorktree_RecordBaseFailureRollsBack(t *testing.T) {
	dir, ctx := newRollbackFixture(t)
	sha := headSHA(t, dir)

	for name, create := range map[string]func(rb *Rollback) (string, error){
		"positional": func(rb *Rollback) (string, error) {
			return CreateNewBranchWorktree("nb", "wt-nb", ctx, rb, "main", sha, "")
		},
		"exploratory": func(rb *Rollback) (string, error) {
			return CreateExploratoryWorktree("nb", ctx, rb, "main", sha, "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			lock := filepath.Join(dir, ".git", "config.lock")
			if err := os.WriteFile(lock, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(lock)

			path, err := create(NewRollback())
			var recErr *RecordBaseError
			if !errors.As(err, &recErr) || recErr.Branch != "nb" {
				t.Fatalf("err = %v, want *RecordBaseError for nb", err)
			}
			if path != "" {
				t.Errorf("path = %q, want empty on failure", path)
			}
			if _, statErr := os.Stat(filepath.Join(ctx.WorktreesDir, "wt-nb")); statErr == nil {
				t.Error("worktree dir survived the failed create")
			}
			if _, statErr := os.Stat(filepath.Join(ctx.WorktreesDir, "nb")); statErr == nil {
				t.Error("worktree dir survived the failed create")
			}
			if branchExists(t, dir, "nb") {
				t.Error("branch nb survived the failed create")
			}
			os.Remove(lock)
			for _, key := range []string{"wt-base", "wt-base-sha"} {
				if got, ok := configGet(t, dir, "branch.nb."+key); ok {
					t.Errorf("branch.nb.%s = %q survived rollback", key, got)
				}
			}
		})
	}
}
