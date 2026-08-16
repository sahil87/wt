package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGo_NameArg_NavigatesToWorktree verifies the happy path: `wt go <name>`
// resolves a worktree and writes its absolute path to WT_CD_FILE while also
// printing it to stdout as the last line. No application is launched.
func TestGo_NameArg_NavigatesToWorktree(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "swift-fox")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWtSuccess(t, repo, env, "go", "swift-fox")

	// WT_CD_FILE holds the resolved worktree path.
	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
	// launcher-contract.md §3: mode 0600.
	info, err := os.Stat(cdFile)
	if err != nil {
		t.Fatalf("stat cd file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected cd file mode 0600, got %o", mode)
	}

	// stdout's last non-empty line is the resolved path (scripting form).
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	last := lines[len(lines)-1]
	if last != wtPath {
		t.Errorf("expected stdout last line %q, got %q (full stdout: %q)", wtPath, last, r.Stdout)
	}

	// No app launch leaked through (the test seam marker would appear).
	if strings.Contains(r.Stderr, "[wt-test-no-launch]") {
		t.Errorf("wt go must not launch an app, got stderr: %q", r.Stderr)
	}
}

// TestGo_NameArg_StderrConfirmation_StdoutStaysBarePath verifies the navigation
// confirmation block lands on STDERR (repo / worktree / branch + indented path)
// while STDOUT stays EXACTLY the bare resolved path — the critical regression
// guard for the stdout machine contract (cd "$(command wt go ...)").
func TestGo_NameArg_StderrConfirmation_StdoutStaysBarePath(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "frosted-jaguar")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, repo, env, "go", "frosted-jaguar")

	// STDOUT must be EXACTLY the bare path (single line, no confirmation text).
	if got := strings.TrimRight(r.Stdout, "\n"); got != wtPath {
		t.Errorf("stdout must be exactly the bare path %q, got %q", wtPath, got)
	}
	if strings.Contains(r.Stdout, "→") {
		t.Errorf("confirmation arrow must NOT appear on stdout, got: %q", r.Stdout)
	}

	// STDERR carries the compact-arrow confirmation block.
	assertContains(t, r.Stderr, "→")
	assertContains(t, r.Stderr, filepath.Base(repo)) // repo name
	assertContains(t, r.Stderr, "frosted-jaguar")    // worktree basename
	assertContains(t, r.Stderr, "frosted-jaguar)")   // branch (in parens; wt create names branch == worktree)
	assertContains(t, r.Stderr, wtPath)              // indented absolute path line
}

// TestGo_OnlyMain_ShowsOneRowMenu verifies that with no non-main worktrees the
// menu still shows the one-row "main (branch)" entry (main is always present
// in-repo) — the old "No worktrees found." path is retired. Empty stdin drives
// the non-TTY fallback to its EOF refusal (exit 1, per the 260717-6end
// contract), so no navigation happens and the confirmation arrow must NOT
// appear — the menu having rendered the main row is what this asserts.
func TestGo_OnlyMain_ShowsOneRowMenu(t *testing.T) {
	repo := createTestRepo(t)

	// No extra worktrees created: selectWorktree pins only the main row.
	r := runWt(t, repo, nil, "go")

	// Empty stdin (non-TTY) drives the fallback menu to its EOF refusal: it
	// renders the menu, then refuses with ExitGeneralError (1) because no choice
	// could be read. This pins the exit code the doc comment above claims.
	assertExitCode(t, r, 1)

	// The one-row menu shows main; the retired "No worktrees found." is gone.
	// With main the sole row, defaultIdx = 1, so the "(default)" marker renders
	// on the main row (NO_COLOR blanks the color codes, leaving the bare text).
	assertContains(t, r.Stdout, "main (main) (default)")
	assertNotContains(t, r.Stdout, "No worktrees found.")
	// No selection was made (empty-stdin EOF), so no navigation confirmation.
	assertNotContains(t, r.Stdout, "→")
	assertNotContains(t, r.Stderr, "→")
}

// TestGo_MainKey_NavigatesToRepoRoot verifies the stable "main" key resolves to
// the main worktree (the repo root): `wt go main` writes the repo root to
// WT_CD_FILE and prints it to stdout, even though no worktree directory is
// literally named "main".
func TestGo_MainKey_NavigatesToRepoRoot(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "swift-fox")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, repo, env, "go", "main")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != repo {
		t.Errorf("expected cd file to contain repo root %q, got %q", repo, string(data))
	}
	if last := strings.TrimSpace(r.Stdout); last != repo {
		t.Errorf("expected stdout to be repo root %q, got %q", repo, r.Stdout)
	}
}

// TestGo_MainKey_CaseInsensitive verifies the "main" key match is
// case-insensitive, matching the exact-basename resolver's contract.
func TestGo_MainKey_CaseInsensitive(t *testing.T) {
	repo := createTestRepo(t)

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	runWtSuccess(t, repo, env, "go", "MAIN")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != repo {
		t.Errorf("expected cd file to contain repo root %q, got %q", repo, string(data))
	}
}

// TestGo_MainKey_ExactBasenamePrecedence pins R4's precedence rule: when a
// worktree directory is literally named "main", `wt go main` resolves to THAT
// worktree via the exact-basename loop, NOT to the repo root via the stable
// "main" key. The exact-basename match runs first, so the additive "main" key
// never fires for it — the accidental-basename behavior is preserved.
func TestGo_MainKey_ExactBasenamePrecedence(t *testing.T) {
	repo := createTestRepo(t)

	// Create a linked worktree whose directory basename is literally "main", on
	// a distinct branch (the repo root already holds the "main" branch). Raw git
	// is used because `wt create` would name the branch after the worktree and
	// collide with the existing main branch — this test targets the resolver's
	// precedence, not the create flow.
	mainWtPath := worktreePath(repo, "main")
	gitRun(t, repo, "worktree", "add", "-b", "main-wt-branch", mainWtPath)

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, repo, env, "go", "main")

	// Resolution lands on the "main"-named worktree, not the repo root.
	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != mainWtPath {
		t.Errorf("expected exact-basename precedence: cd file should be the 'main' worktree %q, got %q (repo root is %q)",
			mainWtPath, string(data), repo)
	}
	if last := strings.TrimSpace(r.Stdout); last != mainWtPath {
		t.Errorf("expected stdout to be the 'main' worktree %q, got %q", mainWtPath, last)
	}
}

// TestGo_NameArg_CaseInsensitive verifies name resolution is case-insensitive,
// matching resolveWorktreeByName's contract shared with `wt open`.
func TestGo_NameArg_CaseInsensitive(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "alpha")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	runWtSuccess(t, repo, env, "go", "ALPHA")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
}

// TestGo_UnknownName_ExitsGeneralError verifies an unresolved name exits
// ExitGeneralError (1) with a "not found" message — the worktree list
// succeeded, the name simply didn't match.
func TestGo_UnknownName_ExitsGeneralError(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "go", "no-such-worktree")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not found")
	assertContains(t, r.Stderr, "wt list")
}

// TestGo_NonGit_ExitsGitError verifies that running `wt go` (and `wt go
// <name>`) from a non-git cwd exits ExitGitError (3).
func TestGo_NonGit_ExitsGitError(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "go")
	if r.ExitCode != 3 {
		t.Fatalf("expected exit 3 (ExitGitError) for no-arg, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}

	r = runWt(t, dir, nil, "go", "some-name")
	if r.ExitCode != 3 {
		t.Fatalf("expected exit 3 (ExitGitError) for name-arg, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
}

// TestGo_NoArg_NonInteractive_ExitsGeneralError verifies that `wt go
// --non-interactive` with no name refuses deterministically (exit 1) rather
// than prompting — a no-arg selection menu has no non-interactive default.
func TestGo_NoArg_NonInteractive_ExitsGeneralError(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")

	r := runWt(t, repo, nil, "go", "--non-interactive")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "No worktree specified")
	// Must not have prompted (no menu rendered).
	assertNotContains(t, r.Stdout, "Select worktree")
}

// ---------- wt go --open (change 0is3) ----------

// TestGo_OpenSkip_EqualsBareNavigate verifies the grammar-parity value: `wt go
// <name> --open skip` navigates exactly as bare `wt go <name>` does — WT_CD_FILE
// written, bare path on stdout, no app launch.
func TestGo_OpenSkip_EqualsBareNavigate(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "skip-nav")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWtSuccess(t, repo, env, "go", "skip-nav", "--open", "skip")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
	if got := strings.TrimRight(r.Stdout, "\n"); got != wtPath {
		t.Errorf("expected stdout to be the bare path %q, got %q", wtPath, got)
	}
	if strings.Contains(r.Stderr, "[wt-test-no-launch]") {
		t.Errorf("--open skip must not launch an app, got stderr: %q", r.Stderr)
	}
}

// TestGo_OpenOpenHere_UnifiedNavigation verifies `wt go <name> --open open_here`
// routes through the unified shell-cd contract: WT_CD_FILE written, bare path as
// the last stdout line, stderr confirmation present — navigation in effect, so
// bare `wt go` ≡ `wt go --open open_here`.
func TestGo_OpenOpenHere_UnifiedNavigation(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "open-here-nav")

	cdFile := filepath.Join(repo, "wt-cd")
	// HOME is overridden so SaveLastApp (the launcher path) cannot touch the
	// real user cache.
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1", "HOME=" + t.TempDir()}

	r := runWtSuccess(t, repo, env, "go", "open-here-nav", "--open", "open_here")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != wtPath {
		t.Errorf("expected cd file to contain %q, got %q", wtPath, string(data))
	}
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	if last := lines[len(lines)-1]; last != wtPath {
		t.Errorf("expected stdout last line %q, got %q", wtPath, last)
	}
	assertContains(t, r.Stderr, "→")
	assertContains(t, r.Stderr, wtPath)
}

// TestGo_OpenUnknownApp_ExitsGeneralError verifies `go --open` carries the
// launcher's unknown-app mapping: ExitGeneralError (1), not a menu and not a
// silent navigation.
func TestGo_OpenUnknownApp_ExitsGeneralError(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "app-err-go")

	r := runWt(t, repo, []string{"HOME=" + t.TempDir()}, "go", "app-err-go", "--open", "nonexistent-app")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
	assertContains(t, r.Stderr, "Unknown app")
}

// TestGo_NameArg_OpenPrompt_RendersAppMenu verifies the composition `wt go
// <name> --open prompt`: no worktree menu (a name was given), the "Open in:"
// app menu renders. Empty stdin drives the non-TTY fallback to its EOF refusal
// (exit 1), which is fine — the menu having rendered is what this asserts.
func TestGo_NameArg_OpenPrompt_RendersAppMenu(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "prompt-target")

	r := runWt(t, repo, []string{"HOME=" + t.TempDir()}, "go", "prompt-target", "--open", "prompt")

	assertExitCode(t, r, 1)
	assertContains(t, r.Stdout, "Open in:")
	assertNotContains(t, r.Stdout, "Select worktree")
}

// TestGo_NoArg_OpenPrompt_ChainsBothMenus verifies the two-menu chain on one
// session: no-arg `wt go --open prompt` renders the worktree-selection menu
// (with the launch-mode prompt wording) and — after a piped selection — the
// "Open in:" app menu on the same stdin.
func TestGo_NoArg_OpenPrompt_ChainsBothMenus(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "chain-a")

	// Piped stdin: choose row 2 (newest non-main worktree) in the selection
	// menu; then EOF lands the app menu on its refusal after rendering.
	r := runWtStdin(t, repo, []string{"HOME=" + t.TempDir()}, "2\n", "go", "--open", "prompt")

	assertContains(t, r.Stdout, "Select worktree to open:")
	assertContains(t, r.Stdout, "Open in:")
}

// TestGo_NoArg_NonInteractive_WithOpen_StillRefuses verifies the selection
// precondition beats the launch flag: no-name + --non-interactive refuses
// deterministically regardless of --open.
func TestGo_NoArg_NonInteractive_WithOpen_StillRefuses(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha-ni")

	r := runWt(t, repo, nil, "go", "--non-interactive", "--open", "code")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "No worktree specified")
	assertNotContains(t, r.Stdout, "Select worktree")
	assertNotContains(t, r.Stdout, "Open in:")
}

// TestGo_OpenPrompt_MenuOrdersNewestFirst pins the selection-menu content on
// the composition flow (the menu now lives only in `wt go`): main pinned to
// row 1, non-main newest-first, newest non-main as the marked default. Adopted
// from the retired TestOpen_MenuOrdersNewestFirst.
func TestGo_OpenPrompt_MenuOrdersNewestFirst(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	createWorktreeViaWt(t, repo, "bravo")
	createWorktreeViaWt(t, repo, "charlie")

	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	chtimesWorktree(t, repo, "alpha", base)
	chtimesWorktree(t, repo, "bravo", base.Add(time.Hour))
	chtimesWorktree(t, repo, "charlie", base.Add(2*time.Hour))

	r := runWt(t, repo, []string{"HOME=" + t.TempDir()}, "go", "--open", "prompt")

	got := menuOrder(r.Stdout, []string{"main", "alpha", "bravo", "charlie"})
	want := []string{"main", "charlie", "bravo", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("expected %v in menu, got %v\nstdout:\n%s", want, got, r.Stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("go --open prompt menu order = %v, want %v", got, want)
			break
		}
	}
	assertContains(t, r.Stdout, "charlie (charlie) (default)")
	assertNotContains(t, r.Stdout, "main (main) (default)")
	assertContains(t, r.Stdout, "Select worktree to open:")
}

// TestGo_HelpShowsOpen pins --open into the visible help surface with no -o
// short (the composition flag is long-form-only, unlike wt create's -o).
func TestGo_HelpShowsOpen(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "go", "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt go --help failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	assertContains(t, r.Stdout, "--open")
	assertNotContains(t, r.Stdout, "-o, --open")
}
