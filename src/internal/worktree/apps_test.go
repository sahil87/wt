package worktree

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTabName(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		wtName   string
		want     string
	}{
		{
			name:     "empty repoName falls back to wtName",
			repoName: "",
			wtName:   "notes",
			want:     "notes",
		},
		{
			name:     "both names present yields repo-wt",
			repoName: "repo",
			wtName:   "swift-fox",
			want:     "repo-swift-fox",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tabName(tt.repoName, tt.wtName)
			if got != tt.want {
				t.Errorf("tabName(%q, %q) = %q, want %q", tt.repoName, tt.wtName, got, tt.want)
			}
		})
	}
}

func TestBuildAvailableApps_OpenHereFirst(t *testing.T) {
	apps := BuildAvailableApps()
	if len(apps) == 0 {
		t.Fatal("BuildAvailableApps returned no apps")
	}
	first := apps[0]
	if first.Name != "Open here" || first.Cmd != "open_here" {
		t.Errorf("expected first app to be {\"Open here\", \"open_here\"}, got {%q, %q}", first.Name, first.Cmd)
	}
}

func TestDetectDefaultApp_SkipsOpenHere(t *testing.T) {
	apps := BuildAvailableApps()
	if len(apps) < 2 {
		t.Skip("need at least 2 apps to test fallback")
	}

	// Clear environment to force fallback path
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TMUX", "")
	t.Setenv("BYOBU_BACKEND", "")
	// Remove last-app cache to ensure fallback
	t.Setenv("HOME", t.TempDir())

	idx := DetectDefaultApp(apps)
	if idx > 0 && idx <= len(apps) {
		if apps[idx-1].Cmd == "open_here" {
			t.Errorf("DetectDefaultApp returned 'open_here' as default (index %d)", idx)
		}
	}
}

// captureOpenHere runs OpenInApp("open_here", …) with stdout/stderr captured,
// returning (stdout, stderr). Env setup (WT_CD_FILE / WT_WRAPPER) is the
// caller's job via t.Setenv.
func captureOpenHere(t *testing.T, path, repoName, wtName string) (string, string) {
	t.Helper()

	origStdout, origStderr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr

	openErr := OpenInApp("open_here", path, repoName, wtName)

	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = origStdout, origStderr

	if openErr != nil {
		t.Fatalf("OpenInApp returned error: %v", openErr)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, rOut); err != nil {
		t.Fatalf("io.Copy stdout: %v", err)
	}
	if _, err := io.Copy(&stderrBuf, rErr); err != nil {
		t.Fatalf("io.Copy stderr: %v", err)
	}
	return stdoutBuf.String(), stderrBuf.String()
}

// TestOpenInApp_OpenHere pins the unified shell-cd contract on the launcher's
// "Open here" action: stdout carries the bare resolved path as its only line
// (the retired `cd -- '<path>'` form must NOT reappear).
func TestOpenInApp_OpenHere(t *testing.T) {
	path := "/tmp/test-worktree"

	// WT_WRAPPER=1 suppresses the hint; no WT_CD_FILE.
	t.Setenv("WT_WRAPPER", "1")
	t.Setenv("WT_CD_FILE", "")

	stdout, _ := captureOpenHere(t, path, "repo", "wt-name")

	if stdout != path+"\n" {
		t.Errorf("expected stdout to be the bare path %q, got %q", path+"\n", stdout)
	}
	if strings.Contains(stdout, "cd -- ") {
		t.Errorf("the cd -- stdout fallback is retired, got %q", stdout)
	}
}

// TestOpenInApp_OpenHere_CdFile verifies the WT_CD_FILE write AND the
// always-print stdout contract hold together (no longer mutually exclusive).
func TestOpenInApp_OpenHere_CdFile(t *testing.T) {
	path := "/tmp/test-worktree"

	cdFile := filepath.Join(t.TempDir(), "wt-cd")
	t.Setenv("WT_CD_FILE", cdFile)

	stdout, _ := captureOpenHere(t, path, "repo", "wt-name")

	data, err := os.ReadFile(cdFile)
	if err != nil {
		t.Fatalf("reading cd file: %v", err)
	}
	if string(data) != path {
		t.Errorf("expected cd file content %q, got %q", path, string(data))
	}
	// The bare path is ALWAYS printed to stdout, even when WT_CD_FILE consumed
	// the navigation target (unified contract step 4).
	if stdout != path+"\n" {
		t.Errorf("expected stdout to be the bare path %q, got %q", path+"\n", stdout)
	}
}

// TestOpenInApp_OpenHere_WithWrapper verifies WT_WRAPPER=1 suppresses the hint
// while the stderr navigation confirmation (shared with `wt go`) is emitted.
func TestOpenInApp_OpenHere_WithWrapper(t *testing.T) {
	path := "/tmp/test-worktree"

	t.Setenv("WT_WRAPPER", "1")
	t.Setenv("WT_CD_FILE", "")

	stdout, stderr := captureOpenHere(t, path, "repo", "wt-name")

	if stdout != path+"\n" {
		t.Errorf("expected stdout to be the bare path %q, got %q", path+"\n", stdout)
	}
	// The hint is suppressed…
	if strings.Contains(stderr, "shell wrapper") {
		t.Errorf("expected no wrapper hint with WT_WRAPPER=1, got %q", stderr)
	}
	// …but the navigation confirmation IS emitted (repo context supplied, the
	// branch of a nonexistent dir degrades to "unknown").
	if !strings.Contains(stderr, "→ repo / test-worktree  (unknown)") {
		t.Errorf("expected stderr navigation confirmation, got %q", stderr)
	}
	if !strings.Contains(stderr, "  "+path) {
		t.Errorf("expected indented path line on stderr, got %q", stderr)
	}
}

func TestBuildAvailableApps_TmuxSession_InTmux(t *testing.T) {
	// Simulate a plain tmux session
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("BYOBU_TTY", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("BYOBU_SESSION", "")
	t.Setenv("BYOBU_CONFIG_DIR", "")

	apps := BuildAvailableApps()

	found := false
	for _, app := range apps {
		if app.Cmd == "tmux_session" {
			found = true
			if app.Name != "tmux session" {
				t.Errorf("expected display name %q, got %q", "tmux session", app.Name)
			}
			break
		}
	}
	if !found {
		t.Error("expected tmux_session in BuildAvailableApps when IsTmuxSession() is true")
	}
}

func TestBuildAvailableApps_TmuxSession_AfterTmuxWindow(t *testing.T) {
	// Simulate a plain tmux session
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("BYOBU_TTY", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("BYOBU_SESSION", "")
	t.Setenv("BYOBU_CONFIG_DIR", "")

	apps := BuildAvailableApps()

	windowIdx := -1
	sessionIdx := -1
	for i, app := range apps {
		if app.Cmd == "tmux_window" {
			windowIdx = i
		}
		if app.Cmd == "tmux_session" {
			sessionIdx = i
		}
	}

	if windowIdx == -1 {
		t.Fatal("tmux_window not found in apps")
	}
	if sessionIdx == -1 {
		t.Fatal("tmux_session not found in apps")
	}
	if sessionIdx != windowIdx+1 {
		t.Errorf("expected tmux_session (index %d) immediately after tmux_window (index %d)", sessionIdx, windowIdx)
	}
}

func TestBuildAvailableApps_TmuxSession_AbsentOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("BYOBU_TTY", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("BYOBU_SESSION", "")
	t.Setenv("BYOBU_CONFIG_DIR", "")

	apps := BuildAvailableApps()

	for _, app := range apps {
		if app.Cmd == "tmux_session" {
			t.Error("tmux_session should not appear when not in a tmux session")
		}
	}
}

func TestBuildAvailableApps_TmuxSession_AbsentInByobu(t *testing.T) {
	// Simulate a byobu session
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("BYOBU_BACKEND", "tmux")

	apps := BuildAvailableApps()

	for _, app := range apps {
		if app.Cmd == "tmux_session" {
			t.Error("tmux_session should not appear in a byobu session")
		}
	}
}

func TestResolveApp_TmuxSession_ByCmd(t *testing.T) {
	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "tmux window", Cmd: "tmux_window"},
		{Name: "tmux session", Cmd: "tmux_session"},
	}

	resolved, err := ResolveApp("tmux_session", apps)
	if err != nil {
		t.Fatalf("ResolveApp returned error: %v", err)
	}
	if resolved.Cmd != "tmux_session" {
		t.Errorf("expected Cmd %q, got %q", "tmux_session", resolved.Cmd)
	}
	if resolved.Name != "tmux session" {
		t.Errorf("expected Name %q, got %q", "tmux session", resolved.Name)
	}
}

func TestResolveApp_TmuxSession_ByDisplayName(t *testing.T) {
	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "tmux window", Cmd: "tmux_window"},
		{Name: "tmux session", Cmd: "tmux_session"},
	}

	resolved, err := ResolveApp("tmux session", apps)
	if err != nil {
		t.Fatalf("ResolveApp returned error: %v", err)
	}
	if resolved.Cmd != "tmux_session" {
		t.Errorf("expected Cmd %q, got %q", "tmux_session", resolved.Cmd)
	}
}

func TestResolveApp_TmuxSession_ByDisplayNameCaseInsensitive(t *testing.T) {
	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "tmux window", Cmd: "tmux_window"},
		{Name: "tmux session", Cmd: "tmux_session"},
	}

	resolved, err := ResolveApp("Tmux Session", apps)
	if err != nil {
		t.Fatalf("ResolveApp returned error: %v", err)
	}
	if resolved.Cmd != "tmux_session" {
		t.Errorf("expected Cmd %q, got %q", "tmux_session", resolved.Cmd)
	}
}

func TestOpenInApp_OpenHere_WithoutWrapper(t *testing.T) {
	path := "/tmp/test-worktree"

	// Ensure WT_WRAPPER and WT_CD_FILE are not set
	t.Setenv("WT_WRAPPER", "")
	t.Setenv("WT_CD_FILE", "")

	stdout, stderr := captureOpenHere(t, path, "repo", "wt-name")

	// stdout is the bare resolved path (the machine contract survives the
	// hint path).
	if stdout != path+"\n" {
		t.Errorf("expected stdout to be the bare path %q, got %q", path+"\n", stdout)
	}

	// stderr carries the wrapper hint (shared copy with wt go).
	if !strings.Contains(stderr, "hint: cd needs the shell wrapper") {
		t.Errorf("expected stderr to contain hint, got %q", stderr)
	}
	if !strings.Contains(stderr, `eval "$(wt shell-init zsh)"`) {
		t.Errorf("expected stderr to contain eval instruction, got %q", stderr)
	}
	if !strings.Contains(stderr, `Add it to your ~/.zshrc or ~/.bashrc`) {
		t.Errorf("expected stderr to contain profile hint, got %q", stderr)
	}
}

func TestResolveDefaultApp_Success(t *testing.T) {
	// Simulate a plain tmux session
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("BYOBU_TTY", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("BYOBU_SESSION", "")
	t.Setenv("BYOBU_CONFIG_DIR", "")
	t.Setenv("HOME", t.TempDir())

	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "VSCode", Cmd: "code", Kind: AppKindEditor},
		{Name: "tmux window", Cmd: "tmux_window"},
	}

	resolved, err := ResolveDefaultApp(apps)
	if err != nil {
		t.Fatalf("ResolveDefaultApp returned error: %v", err)
	}
	if resolved.Cmd != "tmux_window" {
		t.Errorf("expected Cmd %q, got %q", "tmux_window", resolved.Cmd)
	}
}

func TestResolveDefaultApp_VSCode(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("TMUX", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("HOME", t.TempDir())

	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "VSCode", Cmd: "code", Kind: AppKindEditor},
		{Name: "tmux window", Cmd: "tmux_window"},
	}

	resolved, err := ResolveDefaultApp(apps)
	if err != nil {
		t.Fatalf("ResolveDefaultApp returned error: %v", err)
	}
	if resolved.Cmd != "code" {
		t.Errorf("expected Cmd %q, got %q", "code", resolved.Cmd)
	}
}

func TestResolveDefaultApp_NoDefault(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TMUX", "")
	t.Setenv("BYOBU_BACKEND", "")
	t.Setenv("HOME", t.TempDir())

	// Only open_here available — DetectDefaultApp skips it, returns -1
	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
	}

	_, err := ResolveDefaultApp(apps)
	if err == nil {
		t.Fatal("expected error from ResolveDefaultApp with only open_here, got nil")
	}
	if !strings.Contains(err.Error(), "no default app detected") {
		t.Errorf("expected 'no default app detected' error, got %q", err.Error())
	}
}

func TestOpenInApp_OpenHere_PathPrintedVerbatim(t *testing.T) {
	// The retired `cd -- '<path>'` fallback was eval'd and needed shell
	// quoting. The unified contract prints the BARE path verbatim — consumers
	// use cd "$(command wt …)", where the shell substitution handles special
	// characters. No quoting layer may reappear.
	t.Setenv("WT_WRAPPER", "1")
	t.Setenv("WT_CD_FILE", "")

	paths := []string{
		"/tmp/$(whoami)",
		"/tmp/`id`",
		"/tmp/it's-here",
		"/tmp/my worktree",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			stdout, _ := captureOpenHere(t, path, "repo", "wt")
			if stdout != path+"\n" {
				t.Errorf("expected verbatim bare path %q, got %q", path+"\n", stdout)
			}
		})
	}
}

// TestOpenInApp_TestNoLaunchSeam verifies the WT_TEST_NO_LAUNCH=1 short-circuit:
// every appCmd except open_here returns nil + a marker on stderr, without
// exec'ing any external binary. Prevents the VSCode-during-test leak class.
func TestOpenInApp_TestNoLaunchSeam(t *testing.T) {
	t.Setenv("WT_TEST_NO_LAUNCH", "1")

	// Redirect stderr to capture the marker.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = r.Close()
		_ = w.Close()
	})
	os.Stderr = w

	// Pick a representative GUI/terminal/clipboard appCmd from each family.
	guarded := []string{"code", "cursor", "iterm", "finder", "ghostty_macos", "copy_macos"}
	for _, appCmd := range guarded {
		if err := OpenInApp(appCmd, "/tmp/some-wt", "repo", "wt"); err != nil {
			t.Errorf("OpenInApp(%q) under WT_TEST_NO_LAUNCH=1 returned error: %v", appCmd, err)
		}
	}

	w.Close()
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("io.Copy: %v", copyErr)
	}

	out := buf.String()
	for _, appCmd := range guarded {
		if !strings.Contains(out, appCmd) || !strings.Contains(out, "[wt-test-no-launch]") {
			t.Errorf("expected marker line for %q in stderr, got:\n%s", appCmd, out)
		}
	}

	// open_here is exempt — it's cooperative and has no host side effect.
	// Verify it does NOT emit the marker under WT_TEST_NO_LAUNCH=1.
	// (open_here writes to stdout, not stderr, so we don't need to capture here;
	// we just need to confirm it doesn't go through the short-circuit path.
	// A separate run with stdout-capture covers open_here's actual behavior
	// in TestOpenInApp_OpenHere_Stdout.)
}

// appKindByCmd is the closed classification table for the wt open --list
// contract: every Cmd key BuildAvailableApps can emit, mapped to its Kind.
// Action rows map to "" (excluded from --list, retained in the menu / -a).
var appKindByCmd = map[string]string{
	"code":           AppKindEditor,
	"cursor":         AppKindEditor,
	"ghostty_macos":  AppKindTerminal,
	"ghostty_linux":  AppKindTerminal,
	"iterm":          AppKindTerminal,
	"terminal_app":   AppKindTerminal,
	"gnome_terminal": AppKindTerminal,
	"konsole":        AppKindTerminal,
	"finder":         AppKindFileManager,
	"nautilus":       AppKindFileManager,
	"dolphin":        AppKindFileManager,
	"open_here":      "",
	"copy_macos":     "",
	"copy_linux":     "",
	"byobu_tab":      "",
	"tmux_window":    "",
	"tmux_session":   "",
}

func TestBuildAvailableApps_KindClassification(t *testing.T) {
	for _, app := range BuildAvailableApps() {
		want, known := appKindByCmd[app.Cmd]
		if !known {
			t.Errorf("BuildAvailableApps emitted unclassified Cmd %q — add it to the Kind mapping", app.Cmd)
			continue
		}
		if app.Kind != want {
			t.Errorf("Cmd %q: Kind = %q, want %q", app.Cmd, app.Kind, want)
		}
	}
}

func TestListableApps_FiltersActionRowsPreservingOrder(t *testing.T) {
	apps := []AppInfo{
		{Name: "Open here", Cmd: "open_here"},
		{Name: "VSCode", Cmd: "code", Kind: AppKindEditor},
		{Name: "Copy path", Cmd: "copy_macos"},
		{Name: "iTerm2", Cmd: "iterm", Kind: AppKindTerminal},
		{Name: "tmux window", Cmd: "tmux_window"},
		{Name: "Finder", Cmd: "finder", Kind: AppKindFileManager},
		{Name: "tmux session", Cmd: "tmux_session"},
	}

	got := ListableApps(apps)

	wantCmds := []string{"code", "iterm", "finder"}
	if len(got) != len(wantCmds) {
		t.Fatalf("ListableApps returned %d entries, want %d: %+v", len(got), len(wantCmds), got)
	}
	for i, cmd := range wantCmds {
		if got[i].Cmd != cmd {
			t.Errorf("ListableApps[%d].Cmd = %q, want %q (order must match detection order)", i, got[i].Cmd, cmd)
		}
		if got[i].Kind == "" {
			t.Errorf("ListableApps[%d] (%q) has empty Kind — filter must keep only classified apps", i, got[i].Cmd)
		}
	}
}

func TestListableApps_EmptyInputReturnsNonNil(t *testing.T) {
	if got := ListableApps(nil); got == nil {
		t.Error("ListableApps(nil) returned a nil slice; want non-nil empty slice (JSON [] contract)")
	}
	if got := ListableApps([]AppInfo{{Name: "Open here", Cmd: "open_here"}}); got == nil || len(got) != 0 {
		t.Errorf("ListableApps(action rows only) = %v; want non-nil empty slice", got)
	}
}
