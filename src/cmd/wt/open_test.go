package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wt "github.com/sahil87/wt/internal/worktree"
)

func TestOpen_ErrorNonexistentWorktree(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--app", "code", "nonexistent-wt")
	if r.ExitCode == 0 {
		t.Error("expected failure for nonexistent worktree")
	}
	assertContains(t, r.Stderr, "not found")
}

func TestOpen_ErrorFromMainRepoWithoutTarget(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--app", "code")
	if r.ExitCode == 0 {
		t.Error("expected failure from main repo without target")
	}
	assertContains(t, r.Stderr, "No worktree specified")
}

// TestOpen_NoArgs_NonGit_OpensCwd verifies that running `wt open` from a
// non-git directory opens the current working directory (no longer fails
// with ExitGitError as it did pre-change).
func TestOpen_NoArgs_NonGit_OpensCwd(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	cdFile := filepath.Join(dir, "wt-cd")
	env := []string{
		"WT_CD_FILE=" + cdFile,
		"WT_WRAPPER=1",
	}

	r := runWt(t, dir, env, "open", "--app", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != dir {
		t.Errorf("expected cd file to contain %q, got %q", dir, string(data))
	}
}

// TestOpen_PathArg_NonGit_OpensPath verifies that a path arg works from a
// non-git cwd — the path may be unrelated to any repo.
func TestOpen_PathArg_NonGit_OpensPath(t *testing.T) {
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

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != target {
		t.Errorf("expected cd file to contain %q, got %q", target, string(data))
	}
}

// TestOpen_NameArg_NonGit_FailsWithGuidance verifies the spec-mandated error
// path: name args from a non-git cwd exit ExitGeneralError (1) with a clear
// message that suggests passing a path and does NOT suggest cd'ing.
func TestOpen_NameArg_NonGit_FailsWithGuidance(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "swift-fox")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "Cannot open 'swift-fox'")
	assertContains(t, r.Stderr, "name resolution requires a git repository")
	assertContains(t, r.Stderr, "wt open /absolute/path/to/dir")

	// Must NOT suggest cd'ing into a git repo (per spec requirement).
	if strings.Contains(r.Stderr, "Navigate to a git repository") {
		t.Errorf("error message should not suggest cd'ing into a git repo, got: %s", r.Stderr)
	}
	if strings.Contains(strings.ToLower(r.Stderr), "cd into") {
		t.Errorf("error message should not suggest cd'ing into a git repo, got: %s", r.Stderr)
	}
	if strings.Contains(strings.ToLower(r.Stderr), "run from a git repo") {
		t.Errorf("error message should not suggest running from a git repo, got: %s", r.Stderr)
	}
}

// TestOpen_NameArg_NotFound_InRepo verifies that asking for an unknown
// worktree name from inside a git repo exits ExitGeneralError (1, not
// ExitGitError) — the worktree list succeeded, the name simply didn't match.
// This pins the sentinel-error path that distinguishes "not found" (general
// error) from "git worktree list failed" (git error) per launcher-contract.md.
func TestOpen_NameArg_NotFound_InRepo(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "no-such-worktree")
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not found")
}

// TestOpen_PathArg_ExistsButNotDir verifies that passing an arg that exists
// on disk but is not a directory (e.g., a regular file) fails with a clear
// "not a directory" message — not the misleading "name resolution requires a
// git repository" error that would otherwise apply to non-existent args from
// a non-git cwd.
func TestOpen_PathArg_ExistsButNotDir(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "regular-file.txt")
	if err := os.WriteFile(filePath, []byte("hi"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := runWt(t, cwd, nil, "open", filePath)
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1 (ExitGeneralError), got %d\nstdout: %s\nstderr: %s",
			r.ExitCode, r.Stdout, r.Stderr)
	}
	assertContains(t, r.Stderr, "not a directory")
	// Must NOT fall through to the name-resolution error path.
	if strings.Contains(r.Stderr, "name resolution requires a git repository") {
		t.Errorf("file-arg error must not surface the name-resolution message, got: %s", r.Stderr)
	}
}

func TestOpen_ErrorUnknownApp(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "app-err")

	r := runWt(t, repo, nil, "open", "--app", "nonexistent-app", wtPath)
	if r.ExitCode == 0 {
		t.Error("expected failure for unknown app")
	}
	assertContains(t, r.Stderr, "Unknown app")
}

func TestOpen_AppDefault(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := createWorktreeViaWt(t, repo, "default-test")

	// Clear environment to control detection path
	env := []string{
		"TERM_PROGRAM=",
		"TMUX=",
		"BYOBU_BACKEND=",
		"BYOBU_TTY=",
		"BYOBU_SESSION=",
		"BYOBU_CONFIG_DIR=",
		"HOME=" + t.TempDir(),
	}

	r := runWt(t, repo, env, "open", "--app", "default", wtPath)
	// Installed apps vary across environments (e.g., macOS always has Finder).
	// Accept either outcome, but verify the "default" keyword was recognized:
	// - exit 0: some default app resolved and reached OpenInApp (under the
	//   WT_TEST_NO_LAUNCH=1 guard, no real launch happens)
	// - non-zero: no default detected — should show our error, not "Unknown app"
	if r.ExitCode == 0 {
		// A resolved default app MUST go through OpenInApp; the test launch
		// guard emits the marker. Missing marker = a real launch leaked past
		// the seam.
		assertContains(t, r.Stderr, "[wt-test-no-launch]")
	} else {
		assertContains(t, r.Stderr, "No default app detected")
	}
	// "default" must never be treated as a literal app name
	if strings.Contains(r.Stderr, "Unknown app: default") {
		t.Errorf("'default' was treated as a literal app name instead of the keyword: %s", r.Stderr)
	}
	if strings.Contains(r.Stderr, "panic") {
		t.Errorf("command panicked: %s", r.Stderr)
	}
}

// menuOrder returns the worktree names from a selection-menu stdout in the
// order they were listed. It scans numbered menu lines ("  N) name (...)"),
// skipping the synthetic "All (...)" entry that the delete menu prepends.
func menuOrder(stdout string, want []string) []string {
	wantSet := make(map[string]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	var got []string
	for _, line := range strings.Split(stdout, "\n") {
		for w := range wantSet {
			// Menu entries render as "name (branch)"; match the leading token.
			if strings.Contains(line, ") "+w+" (") {
				got = append(got, w)
				break
			}
		}
	}
	return got
}

// chtimesWorktree sets a controlled mtime on a named worktree's directory so
// recency ordering is deterministic regardless of creation timing.
func chtimesWorktree(t *testing.T, repo, name string, mtime time.Time) {
	t.Helper()
	p := worktreePath(repo, name)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatalf("Chtimes %s: %v", name, err)
	}
}

// TestOpen_MenuOrdersNewestFirst verifies the open selection menu pins the main
// worktree to row 1 and lists non-main worktrees newest-first below it, with
// the newest non-main worktree (not main) as the marked default. Empty stdin
// makes ShowMenu print the menu then return on EOF; we assert only on the
// printed ordering and do not exercise any app launch.
func TestOpen_MenuOrdersNewestFirst(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")
	createWorktreeViaWt(t, repo, "bravo")
	createWorktreeViaWt(t, repo, "charlie")

	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	// charlie newest, then bravo, then alpha oldest.
	chtimesWorktree(t, repo, "alpha", base)
	chtimesWorktree(t, repo, "bravo", base.Add(time.Hour))
	chtimesWorktree(t, repo, "charlie", base.Add(2*time.Hour))

	r := runWt(t, repo, nil, "open")
	// main is pinned to row 1 (outside the recency ordering); non-main entries
	// follow newest-first.
	got := menuOrder(r.Stdout, []string{"main", "alpha", "bravo", "charlie"})
	want := []string{"main", "charlie", "bravo", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("expected %v in menu, got %v\nstdout:\n%s", want, got, r.Stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("open menu order = %v, want %v", got, want)
			break
		}
	}
	// The newest non-main worktree (row 2) is the marked default — NOT main.
	assertContains(t, r.Stdout, "charlie (charlie) (default)")
	assertNotContains(t, r.Stdout, "main (main) (default)")
}

// TestOpen_MainKey_ResolvesToRepoRoot verifies `wt open main` resolves the
// stable "main" key to the repo root via the shared resolveWorktreeByName. The
// resolved path is launched via open_here (WT_CD_FILE), proving the key routes
// through the same resolver `wt go` uses.
func TestOpen_MainKey_ResolvesToRepoRoot(t *testing.T) {
	repo := createTestRepo(t)
	createWorktreeViaWt(t, repo, "alpha")

	cdFile := filepath.Join(repo, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWt(t, repo, env, "open", "main", "-a", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != repo {
		t.Errorf("expected cd file to contain repo root %q, got %q", repo, string(data))
	}
}

// NOTE: Testing actual app opening (code, cursor, etc.) requires mock binaries
// on PATH that log their invocations. We test the error paths here; the
// open-by-name success path is tested via the worktree resolution logic
// (which is shared with other commands).

// ---------- Intuitive flag names (change 59u8) ----------

// TestOpen_AppShortFlag verifies the new -a short for --app opens a path in the
// named app (open_here → WT_CD_FILE), parity with --app.
func TestOpen_AppShortFlag(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	cdFile := filepath.Join(dir, "wt-cd")
	env := []string{"WT_CD_FILE=" + cdFile, "WT_WRAPPER=1"}

	r := runWt(t, dir, env, "open", "-a", "open_here")
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", r.ExitCode, r.Stderr)
	}
	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != dir {
		t.Errorf("expected cd file to contain %q, got %q", dir, string(data))
	}
}

// TestOpen_HelpHidesGoShowsSelect verifies `wt open --help` shows --select and
// hides the deprecated --go alias, and shows the -a short for --app.
func TestOpen_HelpHidesGoShowsSelect(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "--help")
	if r.ExitCode != 0 {
		t.Fatalf("wt open --help failed (exit %d)\nstderr: %s", r.ExitCode, r.Stderr)
	}
	assertContains(t, r.Stdout, "--select")
	assertContains(t, r.Stdout, "-a, --app")
	assertNotContains(t, r.Stdout, "--go")
}

// ---------- wt open --list ----------

// TestOpen_List_HumanTable verifies the human-mode listing: an aligned
// Id/Label/Kind table of launchable host apps with action rows excluded.
func TestOpen_List_HumanTable(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--list")
	assertExitCode(t, r, 0)

	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least a header line")
	}
	header := lines[0]
	for _, col := range []string{"Id", "Label", "Kind"} {
		if !strings.Contains(header, col) && !strings.Contains(r.Stdout, "No launchable applications detected.") {
			t.Errorf("expected header column %q in %q", col, header)
		}
	}

	// Action rows must never appear in the listing.
	for _, actionID := range []string{"open_here", "copy_macos", "copy_linux", "byobu_tab", "tmux_window", "tmux_session"} {
		if strings.Contains(r.Stdout, actionID) {
			t.Errorf("action row %q leaked into --list output:\n%s", actionID, r.Stdout)
		}
	}
}

// TestOpen_List_NoGitRequired verifies that --list works from a non-git cwd:
// app detection is host-only, so the branch runs before git-context detection.
func TestOpen_List_NoGitRequired(t *testing.T) {
	dir := t.TempDir() // not a git repository

	r := runWt(t, dir, nil, "open", "--list")
	assertExitCode(t, r, 0)

	rj := runWt(t, dir, nil, "open", "--list", "--json")
	assertExitCode(t, rj, 0)
	var records []map[string]string
	if err := json.Unmarshal([]byte(rj.Stdout), &records); err != nil {
		t.Fatalf("cannot parse --list --json output from non-git cwd: %v\n%s", err, rj.Stdout)
	}
}

// TestOpen_ListJSON_ShapeAndOrder verifies the machine-readable registry:
// a JSON array of {id, label, kind} records — all three keys always present,
// kind in the closed enum, no action rows, detection order preserved.
func TestOpen_ListJSON_ShapeAndOrder(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "open", "--list", "--json")
	assertExitCode(t, r, 0)

	var records []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Stdout), &records); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, r.Stdout)
	}

	validKinds := map[string]bool{"editor": true, "terminal": true, "file-manager": true}
	actionIDs := map[string]bool{
		"open_here": true, "copy_macos": true, "copy_linux": true,
		"byobu_tab": true, "tmux_window": true, "tmux_session": true,
	}

	var gotIDs []string
	for i, rec := range records {
		if len(rec) != 3 {
			t.Errorf("record %d has %d keys, want exactly 3 (id, label, kind): %v", i, len(rec), rec)
		}
		var id, label, kind string
		for key, dst := range map[string]*string{"id": &id, "label": &label, "kind": &kind} {
			raw, ok := rec[key]
			if !ok {
				t.Fatalf("record %d missing key %q: %v", i, key, rec)
			}
			if err := json.Unmarshal(raw, dst); err != nil {
				t.Fatalf("record %d key %q is not a string: %v", i, key, err)
			}
		}
		if id == "" || label == "" {
			t.Errorf("record %d has empty id/label: id=%q label=%q", i, id, label)
		}
		if !validKinds[kind] {
			t.Errorf("record %d kind %q not in editor|terminal|file-manager", i, kind)
		}
		if actionIDs[id] {
			t.Errorf("action row %q leaked into --list --json output", id)
		}
		gotIDs = append(gotIDs, id)
	}

	// Order preserves BuildAvailableApps() detection order minus filtered
	// rows. The test process and the child binary see the same host apps
	// (tmux/byobu rows differ by env, but those carry empty Kind and are
	// filtered either way).
	wantIDs := []string{}
	for _, a := range wt.ListableApps(wt.BuildAvailableApps()) {
		wantIDs = append(wantIDs, a.Cmd)
	}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("got %d records %v, want %d %v", len(gotIDs), gotIDs, len(wantIDs), wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Errorf("record %d id = %q, want %q (detection order must be preserved)", i, gotIDs[i], wantIDs[i])
		}
	}
}

// TestOpen_ListJSON_IDsRoundTrip verifies the validation-source guarantee:
// every id the registry emits is accepted by `wt open <path> -a <id>`.
// WT_TEST_NO_LAUNCH=1 (runWt default) short-circuits the actual launch.
func TestOpen_ListJSON_IDsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "--list", "--json")
	assertExitCode(t, r, 0)

	var records []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &records); err != nil {
		t.Fatalf("cannot parse --list --json output: %v\n%s", err, r.Stdout)
	}

	for _, rec := range records {
		launch := runWt(t, dir, nil, "open", dir, "-a", rec.ID)
		if launch.ExitCode != 0 {
			t.Errorf("id %q from --list --json was rejected by `wt open <dir> -a %s`: exit %d\nstderr: %s",
				rec.ID, rec.ID, launch.ExitCode, launch.Stderr)
		}
	}
}

// TestPrintOpenListJSON_EmptyEmitsArray verifies the zero-apps machine output
// is `[]` (a non-nil empty array), never `null` — direct unit test of the
// emitter since a host with zero detected apps cannot be forced end-to-end.
func TestPrintOpenListJSON_EmptyEmitsArray(t *testing.T) {
	old := os.Stdout
	rp, wp, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = wp
	emitErr := printOpenListJSON(wt.ListableApps(nil))
	wp.Close()
	os.Stdout = old

	out, err := io.ReadAll(rp)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	if emitErr != nil {
		t.Fatalf("printOpenListJSON returned error: %v", emitErr)
	}
	if got := strings.TrimSpace(string(out)); got != "[]" {
		t.Errorf("empty registry emitted %q, want %q (null would break machine consumers)", got, "[]")
	}
}

// TestOpen_List_FlagExclusivity verifies each invalid combination exits
// ExitInvalidArgs (2) at flag-check time — from a non-git cwd, proving the
// validation needs no detection or git work.
func TestOpen_List_FlagExclusivity(t *testing.T) {
	dir := t.TempDir() // not a git repository

	tests := []struct {
		name      string
		args      []string
		wantOnErr string
	}{
		{"list with positional target", []string{"open", "--list", "some-target"}, "mutually exclusive"},
		{"list with app", []string{"open", "--list", "--app", "code"}, "mutually exclusive"},
		{"list with select", []string{"open", "--list", "--select"}, "mutually exclusive"},
		{"list with deprecated go alias", []string{"open", "--list", "--go"}, "mutually exclusive"},
		{"json without list", []string{"open", "--json"}, "requires --list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runWt(t, dir, nil, tt.args...)
			assertExitCode(t, r, 2)
			assertContains(t, r.Stderr, tt.wantOnErr)
		})
	}
}

// TestOpen_HelpShowsListAndJSON pins the new flags into the visible help
// surface (both are script-facing contract flags, never hidden).
func TestOpen_HelpShowsListAndJSON(t *testing.T) {
	dir := t.TempDir()

	r := runWt(t, dir, nil, "open", "--help")
	assertExitCode(t, r, 0)
	assertContains(t, r.Stdout, "--list")
	assertContains(t, r.Stdout, "--json")
}
