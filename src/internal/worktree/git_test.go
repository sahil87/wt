package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupGitRepo creates a temporary git repository for testing.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %s: %v", args, out, err)
		}
	}

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	os.WriteFile(testFile, []byte("# Test\n"), 0644)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initial commit: %s: %v", out, err)
	}

	return dir
}

func TestHasUncommittedChanges_Clean(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if HasUncommittedChanges() {
		t.Error("HasUncommittedChanges() = true, want false for clean repo")
	}
}

func TestHasUncommittedChanges_Modified(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Modify a tracked file
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0644)

	if !HasUncommittedChanges() {
		t.Error("HasUncommittedChanges() = false, want true for modified repo")
	}
}

func TestHasUntrackedFiles_Clean(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if HasUntrackedFiles() {
		t.Error("HasUntrackedFiles() = true, want false for clean repo")
	}
}

func TestHasUntrackedFiles_WithNew(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create an untracked file
	os.WriteFile(filepath.Join(dir, "new-file.txt"), []byte("new\n"), 0644)

	if !HasUntrackedFiles() {
		t.Error("HasUntrackedFiles() = false, want true with untracked file")
	}
}

func TestBranchExistsLocally(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if !BranchExistsLocally("main") {
		t.Error("BranchExistsLocally('main') = false, want true")
	}

	if BranchExistsLocally("nonexistent-branch") {
		t.Error("BranchExistsLocally('nonexistent-branch') = true, want false")
	}
}

func TestRemoteForRef(t *testing.T) {
	dir := setupGitRepo(t)
	for _, args := range [][]string{
		{"git", "remote", "add", "origin", "/nonexistent/origin"},
		{"git", "remote", "add", "upstream", "/nonexistent/upstream"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s: %v", args, out, err)
		}
	}
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	tests := []struct{ ref, want string }{
		{"origin/main", "origin"},
		{"upstream/main", "upstream"},
		{"feature/x", "origin"}, // slash branch, no "feature" remote
		{"main", "origin"},
		{"abc1234", "origin"}, // bare SHA
		{"", "origin"},
		{"/leading", "origin"},
	}
	for _, tt := range tests {
		if got := RemoteForRef(tt.ref); got != tt.want {
			t.Errorf("RemoteForRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestFetchRemote_Error(t *testing.T) {
	dir := setupGitRepo(t)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if err := FetchRemote("no-such-remote"); err == nil {
		t.Error("FetchRemote(no-such-remote) = nil error, want error")
	}
}

func TestUpstreamForBase(t *testing.T) {
	dir := setupGitRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("remote", "add", "origin", "/nonexistent/origin")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("update-ref", "refs/remotes/ghost/main", "HEAD") // no ghost remote / refspec
	run("tag", "v1")
	// "tracked" is a local branch that itself tracks origin/main (inherit source).
	run("branch", "tracked")
	run("branch", "--set-upstream-to=origin/main", "tracked")
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	sha, _ := ResolveCommit("HEAD")
	tests := []struct {
		name, cfg, base, branch string
		want                    string
	}{
		{"remote default", "", "origin/main", "nb", "origin/main"},
		{"remote true", "true", "origin/main", "nb", "origin/main"},
		{"remote false", "false", "origin/main", "nb", ""},
		{"local default", "", "main", "nb", ""},
		{"local always", "always", "main", "nb", "main"},
		{"tag always", "always", "v1", "nb", ""},
		{"sha", "", sha, "nb", ""},
		{"HEAD always", "always", "HEAD", "nb", ""},
		{"empty", "always", "", "nb", ""},
		{"unresolvable", "", "no-such", "nb", ""},
		{"simple matching name", "simple", "origin/main", "main", "origin/main"},
		{"simple other name", "simple", "origin/main", "nb", ""},
		{"inherit copies base's upstream", "inherit", "tracked", "nb", "origin/main"},
		{"inherit base without upstream", "inherit", "main", "nb", ""},
		{"inherit remote base", "inherit", "origin/main", "nb", ""},
		{"stray remote-tracking ref, no remote", "", "ghost/main", "nb", ""},
		{"stray remote-tracking ref, always", "always", "ghost/main", "nb", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg == "" {
				exec.Command("git", "config", "--unset", "branch.autoSetupMerge").Run()
			} else {
				run("config", "branch.autoSetupMerge", tt.cfg)
			}
			if got := UpstreamForBase(tt.base, tt.branch); got != tt.want {
				t.Errorf("UpstreamForBase(%q, %q) cfg=%q = %q, want %q", tt.base, tt.branch, tt.cfg, got, tt.want)
			}
		})
	}
}

func TestRefspecDstMatches(t *testing.T) {
	tests := []struct {
		spec, ref string
		want      bool
	}{
		{"+refs/heads/*:refs/remotes/origin/*", "refs/remotes/origin/main", true},
		{"refs/heads/*:refs/remotes/origin/*", "refs/remotes/origin/feat/x", true},
		{"+refs/heads/*:refs/remotes/origin/*", "refs/remotes/ghost/main", false},
		{"+refs/heads/main:refs/remotes/origin/main", "refs/remotes/origin/main", true},
		{"+refs/heads/main:refs/remotes/origin/main", "refs/remotes/origin/dev", false},
		{"+refs/heads/*:refs/remotes/origin/*", "refs/remotes/origin", false},
		{"refs/heads/main", "refs/remotes/origin/main", false}, // no dst
		{"", "refs/remotes/origin/main", false},
	}
	for _, tt := range tests {
		if got := refspecDstMatches(tt.spec, tt.ref); got != tt.want {
			t.Errorf("refspecDstMatches(%q, %q) = %v, want %v", tt.spec, tt.ref, got, tt.want)
		}
	}
}
