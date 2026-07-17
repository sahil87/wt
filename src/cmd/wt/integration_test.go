package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIntegration_CreateListDeleteLifecycle(t *testing.T) {
	repo := createTestRepo(t)

	// Create
	r := runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "lifecycle-test", "--worktree-init", "false")
	assertContains(t, r.Stderr, "Created worktree: lifecycle-test")

	// List
	r = runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "lifecycle-test")

	// Delete
	r = runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "lifecycle-test")
	combined := r.Stdout + r.Stderr
	assertContains(t, combined, "Deleted worktree")

	// Verify gone from list
	r = runWtSuccess(t, repo, nil, "list")
	assertNotContains(t, r.Stdout, "lifecycle-test")
}

func TestIntegration_CreateMultipleDeleteAll(t *testing.T) {
	repo := createTestRepo(t)

	createWorktreeViaWt(t, repo, "multi-1")
	createWorktreeViaWt(t, repo, "multi-2")
	createWorktreeViaWt(t, repo, "multi-3")

	// Verify all exist
	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "multi-1")
	assertContains(t, r.Stdout, "multi-2")
	assertContains(t, r.Stdout, "multi-3")

	// Delete all
	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--delete-all")

	// Verify all gone
	r = runWtSuccess(t, repo, nil, "list")
	assertNotContains(t, r.Stdout, "multi-1")
	assertNotContains(t, r.Stdout, "multi-2")
	assertNotContains(t, r.Stdout, "multi-3")

	// Verify branches cleaned up
	assertBranchNotExists(t, repo, "multi-1")
	assertBranchNotExists(t, repo, "multi-2")
	assertBranchNotExists(t, repo, "multi-3")
}

func TestIntegration_NonInteractiveAutomation(t *testing.T) {
	repo := createTestRepo(t)

	// Full lifecycle using only --non-interactive flags
	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "auto-test", "--worktree-init", "false")

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "auto-test")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "auto-test", "--delete-branch", "true", "--delete-remote", "true")

	r = runWtSuccess(t, repo, nil, "list")
	assertNotContains(t, r.Stdout, "auto-test")
}

func TestIntegration_CreatedWorktreeHasCorrectBranch(t *testing.T) {
	repo := createTestRepo(t)

	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "branch-verify", "--worktree-init", "false")
	assertBranchExists(t, repo, "branch-verify")

	r := runWtSuccess(t, repo, nil, "list")
	assertContains(t, r.Stdout, "branch-verify")
}

func TestIntegration_BranchDeletePreservesOthers(t *testing.T) {
	repo := createTestRepo(t)

	gitRun(t, repo, "checkout", "-b", "feature/keep-me")
	os.WriteFile(filepath.Join(repo, "keep.txt"), []byte("keep"), 0644)
	gitRun(t, repo, "add", "keep.txt")
	gitRun(t, repo, "commit", "-q", "-m", "keep")
	gitRun(t, repo, "checkout", "main")

	gitRun(t, repo, "checkout", "-b", "feature/delete-me")
	gitRun(t, repo, "checkout", "main")

	runWtSuccess(t, repo, nil, "create", "--non-interactive", "--worktree-name", "del-branch", "--checkout", "feature/delete-me")

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "del-branch", "--delete-branch", "true")

	// delete-me should be gone
	assertBranchNotExists(t, repo, "feature/delete-me")

	// keep-me should still exist
	assertBranchExists(t, repo, "feature/keep-me")
}

func TestIntegration_RapidCreateDeleteCycle(t *testing.T) {
	repo := createTestRepo(t)

	for i := 1; i <= 3; i++ {
		name := "cycle-" + strings.Repeat("x", i) // cycle-x, cycle-xx, cycle-xxx for uniqueness
		createWorktreeViaWt(t, repo, name)
		runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", name)
	}

	// All should be gone
	r := runWtSuccess(t, repo, nil, "list")
	assertNotContains(t, r.Stdout, "cycle-")
}

func TestIntegration_GitStateCleanAfterCreateDelete(t *testing.T) {
	repo := createTestRepo(t)

	createWorktreeViaWt(t, repo, "integrity-test")
	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "integrity-test")

	assertGitStateClean(t, repo)
}

func TestIntegration_MainRepoUnaffectedByWorktreeOps(t *testing.T) {
	repo := createTestRepo(t)

	initialCommit := gitRun(t, repo, "rev-parse", "HEAD")

	// Create, modify, and delete a worktree
	wtPath := createWorktreeViaWt(t, repo, "no-affect")
	os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("new"), 0644)
	gitRun(t, wtPath, "add", ".")
	gitRun(t, wtPath, "commit", "-q", "-m", "worktree commit")
	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "no-affect")

	currentCommit := gitRun(t, repo, "rev-parse", "HEAD")
	if initialCommit != currentCommit {
		t.Errorf("main repo HEAD changed: %s -> %s", initialCommit, currentCommit)
	}
}

// TestIntegration_LauncherContract_NonGitTempDir exercises the full
// WT_CD_FILE + WT_WRAPPER contract end-to-end against a non-git directory.
// This is the spec-mandated regression test for the launcher contract:
// callers like `hop` rely on `wt open <path> --app open_here` working from
// any cwd and writing the resolved path to WT_CD_FILE.
func TestIntegration_LauncherContract_NonGitTempDir(t *testing.T) {
	// Resolve symlinks (macOS /tmp -> /private/tmp) so the path written to
	// WT_CD_FILE — the full resolved directory path passed to OpenInApp —
	// matches what we assert below. On macOS the kernel hands back
	// /var/folders/... while user-space sees /private/var/folders/...; we
	// normalize to one form here so the equality check is stable.
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks cwd: %v", err)
	}
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks target: %v", err)
	}

	cdFile := filepath.Join(cwd, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWt(t, cwd, env, "open", target, "--app", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}

	info, err := os.Stat(cdFile)
	if err != nil {
		t.Fatalf("stat cd file %s: %v", cdFile, err)
	}
	// launcher-contract.md §3 documents 0600 as a stability guarantee.
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected cd file mode 0600, got %o", mode)
	}

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file %s: %v", cdFile, err)
	}
	if string(data) != target {
		t.Errorf("expected cd file to contain %q, got %q", target, string(data))
	}

	// stderr must NOT contain the shell-wrapper hint (WT_WRAPPER=1 suppresses it).
	if strings.Contains(r.Stderr, "shell wrapper") {
		t.Errorf("expected no shell-wrapper hint with WT_WRAPPER=1, got stderr: %q", r.Stderr)
	}
}

// writeFailingInitScript writes a committed init script that exits 1 and
// returns the env override pointing WORKTREE_INIT_SCRIPT at it. Mirrors the
// helper in create_test.go but lives here so the integration tests stay
// self-contained.
func writeFailingInitScript(t *testing.T, repo string) []string {
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

// TestIntegration_CreateInitFailure_KeepsWorktreeAndExits7 exercises the
// full kept-worktree-on-init-failure contract end-to-end against the built
// binary. Required by spec.md "Requirement: Integration test for init failure".
func TestIntegration_CreateInitFailure_KeepsWorktreeAndExits7(t *testing.T) {
	repo := createTestRepo(t)
	env := writeFailingInitScript(t, repo)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "testbranch",
		"--worktree-open", "skip")

	// 1. Process exit code is exactly 7 (ExitInitFailed).
	assertExitCode(t, r, 7)

	// 2. Worktree directory still exists on disk.
	wtPath := worktreePath(repo, "testbranch")
	assertDirExists(t, wtPath)

	// 3. Branch still exists in the repository.
	assertBranchExists(t, repo, "testbranch")

	// 4. Worktree appears in `git worktree list`.
	out, err := exec.Command("git", "-C", repo, "worktree", "list").CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), wtPath) {
		t.Errorf("expected worktree %q in `git worktree list`:\n%s", wtPath, out)
	}

	// 5. Stderr contains the worktree path.
	assertContains(t, r.Stderr, wtPath)

	// 6. Stderr contains the `wt init` retry hint.
	assertContains(t, r.Stderr, "wt init")

	// 7. Stderr contains the `wt delete` remove hint.
	assertContains(t, r.Stderr, "wt delete")

	// 8. The failing init script's stderr marker streamed through.
	assertContains(t, r.Stderr, "INIT_FAIL_MARKER")
}

// stubFabEnv writes a stub `fab` executable that exits with exitCode into a
// fresh t.TempDir(), and returns env overrides that (1) prepend that dir to
// PATH so the stub shadows any real fab and (2) clear WORKTREE_INIT_SCRIPT so
// the default "fab sync" init runs. The stub emits a fab-like error line on
// stderr so the streamed-through-output behavior is exercised. All state lives
// under t.TempDir() with no host side-effects (per code-review.md).
func stubFabEnv(t *testing.T, exitCode int) []string {
	t.Helper()
	binDir := t.TempDir()
	stub := filepath.Join(binDir, "fab")
	// The stub echoes a fab-like message and exits with the requested code,
	// mirroring `fab sync`'s "not in a fab-managed repo" behavior (exit 3) or an
	// older fab-kit's generic failure (exit 1).
	content := "#!/usr/bin/env bash\n" +
		"echo 'ERROR: not in a fab-managed repo (stub)' >&2\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(stub, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile stub fab: %v", err)
	}
	return []string{
		// Prepend binDir so the stub shadows any real fab on the dev/CI host.
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		// Clear the test-default no-init override so InitScriptPath falls through
		// to the built-in default "fab sync" (isDefault=true).
		"WORKTREE_INIT_SCRIPT=",
	}
}

// TestIntegration_CreateDefaultInitSkip_NonFabRepo_Exit3 verifies the graceful
// default-init skip: when the built-in default `fab sync` exits ExitNotManaged
// (3) in a non-fab-managed repo, `wt create` treats it as an init no-op —
// worktree kept, skip warning on stderr, NO failure banner, exit 0.
func TestIntegration_CreateDefaultInitSkip_NonFabRepo_Exit3(t *testing.T) {
	repo := createTestRepo(t)
	env := stubFabEnv(t, 3)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "skip-default",
		"--worktree-open", "skip")

	// Exit 0 — the default does not apply, so it is not a failure.
	assertExitCode(t, r, 0)

	// Worktree was created and registered.
	assertWorktreeExists(t, repo, "skip-default")

	// The canonical skip warning is on stderr.
	assertContains(t, r.Stderr, "not a fab-managed repo")
	assertContains(t, r.Stderr, "WORKTREE_INIT_SCRIPT")

	// No init-failure banner / hard-failure artifacts.
	assertNotContains(t, r.Stderr, "Worktree init complete.")
	assertNotContains(t, r.Stderr, "wt delete")
	assertNotContains(t, r.Stderr, "init script exited with status")

	// stdout carries the final worktree path (unaffected by the skip).
	if strings.TrimSpace(r.Stdout) != worktreePath(repo, "skip-default") {
		t.Errorf("expected stdout final path %q, got %q",
			worktreePath(repo, "skip-default"), r.Stdout)
	}
}

// TestIntegration_InitDefaultSkip_NonFabRepo_Exit3 verifies `wt init` exits 0
// (skip warning, no ExitInitFailed) when the default `fab sync` exits 3.
func TestIntegration_InitDefaultSkip_NonFabRepo_Exit3(t *testing.T) {
	repo := createTestRepo(t)
	env := stubFabEnv(t, 3)

	r := runWt(t, repo, env, "init")

	assertExitCode(t, r, 0)
	assertContains(t, r.Stderr, "not a fab-managed repo")
	assertNotContains(t, r.Stderr, "Worktree init complete.")
	assertNotContains(t, r.Stderr, "Init script failed")
}

// TestIntegration_CreateDefaultInit_OldFabKit_Exit1_HardFails verifies the
// no-regression posture: an older fab-kit exits 1 (not 3), so the default init
// hard-fails exactly as today — exit 7 with the failure banner.
func TestIntegration_CreateDefaultInit_OldFabKit_Exit1_HardFails(t *testing.T) {
	repo := createTestRepo(t)
	env := stubFabEnv(t, 1)

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "oldfab",
		"--worktree-open", "skip")

	// Exit 7 (ExitInitFailed), unchanged from today.
	assertExitCode(t, r, 7)

	// Worktree is kept, banner is shown.
	assertWorktreeExists(t, repo, "oldfab")
	assertContains(t, r.Stderr, worktreePath(repo, "oldfab"))
	assertContains(t, r.Stderr, "wt delete")

	// The graceful-skip path must NOT engage for a non-3 exit.
	assertNotContains(t, r.Stderr, "not a fab-managed repo — skipping init")
}

// TestIntegration_CreateExplicitFabSync_Exit3_HardFails verifies the provenance
// rule: an EXPLICIT WORKTREE_INIT_SCRIPT="fab sync" still hard-fails on exit 3
// (the user opted into the script), even though the string matches the default.
func TestIntegration_CreateExplicitFabSync_Exit3_HardFails(t *testing.T) {
	repo := createTestRepo(t)
	// Start from the exit-3 stub, then override WORKTREE_INIT_SCRIPT to an
	// explicit "fab sync" (last-wins) — the stub still shadows fab on PATH.
	env := append(stubFabEnv(t, 3), "WORKTREE_INIT_SCRIPT=fab sync")

	r := runWt(t, repo, env, "create", "--non-interactive",
		"--worktree-name", "explicit-fabsync",
		"--worktree-open", "skip")

	// Exit 7 — explicit script provenance means the failure is the user's to see.
	assertExitCode(t, r, 7)
	assertWorktreeExists(t, repo, "explicit-fabsync")
	assertContains(t, r.Stderr, "wt delete")

	// The graceful-skip path must NOT engage for an explicitly-configured script.
	assertNotContains(t, r.Stderr, "not a fab-managed repo — skipping init")
}

// TestIntegration_Go_FromSiblingWorktree exercises the core gap this change
// closes: navigating to a sibling worktree from inside another worktree.
// `wt go <name>` run from inside worktree A must resolve sibling B and write
// B's absolute path to WT_CD_FILE (and stdout), launching nothing.
func TestIntegration_Go_FromSiblingWorktree(t *testing.T) {
	repo := createTestRepo(t)
	pathA := createWorktreeViaWt(t, repo, "alpha")
	pathB := createWorktreeViaWt(t, repo, "bravo")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	// cwd is INSIDE worktree alpha — the menu/resolution must still see bravo.
	r := runWtSuccess(t, pathA, env, "go", "bravo")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != pathB {
		t.Errorf("expected cd file to contain sibling %q, got %q", pathB, string(data))
	}
	if last := strings.TrimSpace(r.Stdout); last != pathB {
		t.Errorf("expected stdout to be sibling path %q, got %q", pathB, r.Stdout)
	}
	if strings.Contains(r.Stderr, "[wt-test-no-launch]") {
		t.Errorf("wt go must not launch an app, got stderr: %q", r.Stderr)
	}
}

// TestIntegration_Go_MenuOrdersNewestFirst verifies the no-arg `wt go` menu
// lists the repo's worktrees newest-first with the newest as the default —
// the shared selection helper used by `wt open`. Mirrors
// TestOpen_MenuOrdersNewestFirst.
func TestIntegration_Go_MenuOrdersNewestFirst(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	createWorktreeViaWt(t, repo, "bravo")
	createWorktreeViaWt(t, repo, "charlie")

	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWorktree(t, repo, "alpha", base)
	chtimesWorktree(t, repo, "bravo", base.Add(time.Hour))
	chtimesWorktree(t, repo, "charlie", base.Add(2*time.Hour))

	r := runWt(t, repo, nil, "go")
	got := menuOrder(r.Stdout, []string{"alpha", "bravo", "charlie"})
	want := []string{"charlie", "bravo", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("expected %v in menu, got %v\nstdout:\n%s", want, got, r.Stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("go menu order = %v, want %v", got, want)
			break
		}
	}
	assertContains(t, r.Stdout, "charlie (charlie) (default)")
	assertContains(t, r.Stdout, "Select worktree to go to:")
}

// TestIntegration_OpenGo_NameArg_ResolvesAndLaunches exercises `wt open --go
// <name> --app open_here`: it composes selection (resolve-by-name) with the
// launcher, writing the resolved path to WT_CD_FILE via the open_here app.
func TestIntegration_OpenGo_NameArg_ResolvesAndLaunches(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	pathB := createWorktreeViaWt(t, repo, "bravo")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	// Run from inside alpha to prove cross-worktree selection works here too.
	r := runWtSuccess(t, worktreePath(repo, "alpha"), env, "open", "--go", "bravo", "--app", "open_here")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != pathB {
		t.Errorf("expected cd file to contain %q, got %q", pathB, string(data))
	}
	_ = r
}

// TestIntegration_OpenGo_NonGit_ExitsGitError verifies `wt open --go` from a
// non-git cwd exits ExitGitError (3), the same precondition as `wt go`.
func TestIntegration_OpenGo_NonGit_ExitsGitError(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "--go")
	if r.ExitCode != 3 {
		t.Fatalf("expected exit 3 (ExitGitError), got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
}

func TestIntegration_WorktreeCommitIndependent(t *testing.T) {
	repo := createTestRepo(t)

	wtPath := createWorktreeViaWt(t, repo, "independent")

	// Commit in worktree
	os.WriteFile(filepath.Join(wtPath, "wt-file.txt"), []byte("wt content"), 0644)
	gitRun(t, wtPath, "add", ".")
	gitRun(t, wtPath, "commit", "-q", "-m", "wt change")

	// File should not exist in main repo
	if _, err := os.Stat(filepath.Join(repo, "wt-file.txt")); err == nil {
		t.Error("wt-file.txt should not exist in main repo")
	}

	runWtSuccess(t, repo, nil, "delete", "--non-interactive", "--worktree-name", "independent")
}

// TestIntegration_OpenSelect_NameArg_ResolvesAndLaunches exercises the new
// --select flag (renamed from --go): `wt open --select <name> --app open_here`
// composes resolve-by-name selection with the launcher, writing the resolved
// path to WT_CD_FILE via open_here. No deprecation warning on the new flag.
func TestIntegration_OpenSelect_NameArg_ResolvesAndLaunches(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	pathB := createWorktreeViaWt(t, repo, "bravo")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, worktreePath(repo, "alpha"), env, "open", "--select", "bravo", "--app", "open_here")
	assertNotContains(t, r.Stderr, "deprecated")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != pathB {
		t.Errorf("expected cd file to contain %q, got %q", pathB, string(data))
	}
}

// TestIntegration_OpenGo_Deprecated verifies the deprecated --go alias still
// composes select-then-launch AND emits a stderr deprecation warning naming
// --select. stdout/WT_CD_FILE machine contract is unchanged.
func TestIntegration_OpenGo_Deprecated(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	pathB := createWorktreeViaWt(t, repo, "bravo")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, worktreePath(repo, "alpha"), env, "open", "--go", "bravo", "--app", "open_here")
	assertContains(t, r.Stderr, "deprecated")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != pathB {
		t.Errorf("expected cd file to contain %q, got %q", pathB, string(data))
	}
}

// TestIntegration_NonTTYMenuActionableRefusal asserts the toolkit-standards
// principles №1 (non-interactive by default) / №4 (fail fast with actionable
// errors): when an interactive selection menu is reached with stdin that is not
// a terminal (runWt feeds an empty, immediate-EOF reader), the command MUST
// refuse with an actionable, flag-naming error on stderr — never hang, and never
// surface the bare "reading input: EOF". Covers the three menu entry points
// (wt open main-repo menu, wt go no-name, wt delete no-name) through the single
// shared fallback-menu choke point.
func TestIntegration_NonTTYMenuActionableRefusal(t *testing.T) {
	repo := createTestRepo(t)
	// Two worktrees so each menu has entries (an empty menu short-circuits before
	// the prompt — we want to exercise the prompt-then-EOF refusal path).
	createWorktreeViaWt(t, repo, "menu-a")
	createWorktreeViaWt(t, repo, "menu-b")

	cases := []struct {
		name string
		args []string
	}{
		{"open", []string{"open"}},
		{"go", []string{"go"}},
		{"delete", []string{"delete"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// runWt feeds an empty stdin (immediate EOF, never a TTY).
			r := runWt(t, repo, nil, tc.args...)
			// ExitGeneralError (1): the menu returns an error, main.go maps it.
			assertExitCode(t, r, 1)
			// Actionable structured refusal on stderr, naming the escape.
			assertContains(t, r.Stderr, "end of input")
			assertContains(t, r.Stderr, "--non-interactive")
			// The bare cause must NOT be surfaced.
			assertNotContains(t, r.Stderr, "reading input: EOF")
		})
	}
}

// TestIntegration_SkillBundle runs the built binary and asserts the `skill`
// standard's invocation contract end-to-end: `wt skill` exits 0, writes the
// bundle to stdout byte-identical to the canonical docs/site/skill.md, and
// leaves stderr empty. No git state is needed, but the case uses runWt's env
// isolation like every other end-to-end case.
func TestIntegration_SkillBundle(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "skill")

	if r.Stderr != "" {
		t.Errorf("expected empty stderr on success, got: %q", r.Stderr)
	}

	// The canonical bundle lives at the repo root; the test binary runs from
	// src/cmd/wt/, so reach three levels up.
	canonical, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "site", "skill.md"))
	if err != nil {
		t.Fatalf("read canonical docs/site/skill.md: %v", err)
	}
	if r.Stdout != string(canonical) {
		t.Errorf("stdout is not byte-identical to docs/site/skill.md (got %d bytes, want %d)",
			len(r.Stdout), len(canonical))
	}
}

// TestIntegration_SkillRejectsArgs asserts cobra.NoArgs end-to-end against the
// built binary: an unexpected positional arg surfaces a non-zero exit via
// main.go's error path (mirroring TestHelpDump_RejectsArgs).
func TestIntegration_SkillRejectsArgs(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "skill", "extra")
	if r.ExitCode == 0 {
		t.Fatalf("expected non-zero exit from `wt skill extra` (cobra.NoArgs)\nstdout: %s\nstderr: %s",
			r.Stdout, r.Stderr)
	}
}
