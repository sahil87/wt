package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AppInfo describes an application that can open a worktree.
type AppInfo struct {
	Name string // Display name (e.g., "VSCode")
	Cmd  string // Command key (e.g., "code")
	Kind string // Launchable-app kind (AppKind* constant); empty for action rows
}

// AppKind* classify launchable host applications for `wt open --list`.
// Action rows (open_here, copy_*, byobu/tmux) carry an empty Kind — they are
// not host applications an external consumer can launch, so they are excluded
// from the listing while remaining in the interactive menu and valid as
// `-a` values.
const (
	AppKindEditor      = "editor"
	AppKindTerminal    = "terminal"
	AppKindFileManager = "file-manager"
)

// BuildAvailableApps detects which apps are available on the current system.
func BuildAvailableApps() []AppInfo {
	osType := DetectOS()
	var apps []AppInfo

	// Open here — always available, no detection needed
	apps = append(apps, AppInfo{Name: "Open here", Cmd: "open_here"})

	// VSCode
	if appAvailable("code", "com.microsoft.VSCode", "code.desktop", osType) {
		apps = append(apps, AppInfo{Name: "VSCode", Cmd: "code", Kind: AppKindEditor})
	}

	// Cursor
	if appAvailable("cursor", "com.todesktop.230313mzl4w4u92", "cursor.desktop", osType) {
		apps = append(apps, AppInfo{Name: "Cursor", Cmd: "cursor", Kind: AppKindEditor})
	}

	// Ghostty
	if osType == "macos" {
		if appAvailable("ghostty", "com.mitchellh.ghostty", "", osType) {
			apps = append(apps, AppInfo{Name: "Ghostty", Cmd: "ghostty_macos", Kind: AppKindTerminal})
		}
	} else if osType == "linux" {
		if appAvailable("ghostty", "", "com.mitchellh.ghostty.desktop", osType) {
			apps = append(apps, AppInfo{Name: "Ghostty", Cmd: "ghostty_linux", Kind: AppKindTerminal})
		}
	}

	// macOS-only terminals
	if osType == "macos" {
		if appAvailable("", "com.googlecode.iterm2", "", osType) {
			apps = append(apps, AppInfo{Name: "iTerm2", Cmd: "iterm", Kind: AppKindTerminal})
		}
		if appAvailable("", "com.apple.Terminal", "", osType) {
			apps = append(apps, AppInfo{Name: "Terminal.app", Cmd: "terminal_app", Kind: AppKindTerminal})
		}
	}

	// Linux-only terminals
	if osType == "linux" {
		if appAvailable("gnome-terminal", "", "org.gnome.Terminal.desktop", osType) {
			apps = append(apps, AppInfo{Name: "GNOME Terminal", Cmd: "gnome_terminal", Kind: AppKindTerminal})
		}
		if appAvailable("konsole", "", "org.kde.konsole.desktop", osType) {
			apps = append(apps, AppInfo{Name: "Konsole", Cmd: "konsole", Kind: AppKindTerminal})
		}
	}

	// File managers
	if osType == "macos" {
		apps = append(apps, AppInfo{Name: "Finder", Cmd: "finder", Kind: AppKindFileManager})
	} else if osType == "linux" {
		if appAvailable("nautilus", "", "org.gnome.Nautilus.desktop", osType) {
			apps = append(apps, AppInfo{Name: "Nautilus", Cmd: "nautilus", Kind: AppKindFileManager})
		}
		if appAvailable("dolphin", "", "org.kde.dolphin.desktop", osType) {
			apps = append(apps, AppInfo{Name: "Dolphin", Cmd: "dolphin", Kind: AppKindFileManager})
		}
	}

	// Copy path
	if osType == "macos" {
		if _, err := exec.LookPath("pbcopy"); err == nil {
			apps = append(apps, AppInfo{Name: "Copy path", Cmd: "copy_macos"})
		}
	} else if osType == "linux" {
		if _, err := exec.LookPath("xclip"); err == nil {
			apps = append(apps, AppInfo{Name: "Copy path", Cmd: "copy_linux"})
		}
	}

	// Byobu tab (only in byobu session)
	if IsByobuSession() {
		apps = append(apps, AppInfo{Name: "Byobu tab", Cmd: "byobu_tab"})
	}

	// tmux window (only in plain tmux session)
	if IsTmuxSession() {
		apps = append(apps, AppInfo{Name: "tmux window", Cmd: "tmux_window"})
	}

	// tmux session (only in plain tmux session)
	if IsTmuxSession() {
		apps = append(apps, AppInfo{Name: "tmux session", Cmd: "tmux_session"})
	}

	return apps
}

// ListableApps filters apps to launchable host applications — entries with a
// non-empty Kind — preserving detection order. Action rows (open_here,
// copy_macos/copy_linux, byobu_tab, tmux_window, tmux_session) are excluded:
// they depend on wt's own process environment (shell wrapper, clipboard,
// multiplexer session) and are wrong signals for an external consumer. They
// remain in the interactive menu and remain valid `-a` values, unchanged.
// The returned slice is always non-nil so JSON consumers get `[]`, not `null`.
func ListableApps(apps []AppInfo) []AppInfo {
	listable := make([]AppInfo, 0, len(apps))
	for _, a := range apps {
		if a.Kind != "" {
			listable = append(listable, a)
		}
	}
	return listable
}

// ResolveApp resolves a user-provided app name to an AppInfo.
// Matches command keys directly, then display names case-insensitively.
func ResolveApp(input string, apps []AppInfo) (*AppInfo, error) {
	// Exact match on command key
	for i := range apps {
		if apps[i].Cmd == input {
			return &apps[i], nil
		}
	}

	// Case-insensitive match on display name
	inputLower := strings.ToLower(input)
	for i := range apps {
		if strings.ToLower(apps[i].Name) == inputLower {
			return &apps[i], nil
		}
	}

	return nil, fmt.Errorf("app '%s' not found or not available", input)
}

// DetectDefaultApp returns the index (1-based) of the best default app based on context.
func DetectDefaultApp(apps []AppInfo) int {
	var suggestedCmd string

	switch os.Getenv("TERM_PROGRAM") {
	case "vscode":
		suggestedCmd = "code"
	case "cursor":
		suggestedCmd = "cursor"
	}

	if suggestedCmd == "" {
		if IsByobuSession() {
			suggestedCmd = "byobu_tab"
		} else if IsTmuxSession() {
			suggestedCmd = "tmux_window"
		}
	}

	if suggestedCmd == "" {
		data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".cache", "wt", "last-app"))
		if err == nil {
			suggestedCmd = strings.TrimSpace(string(data))
		}
	}

	if suggestedCmd != "" {
		for i, app := range apps {
			if app.Cmd == suggestedCmd {
				return i + 1
			}
		}
	}

	// Skip "open_here" in the fallback — it should never be the default
	for i, app := range apps {
		if app.Cmd != "open_here" {
			return i + 1
		}
	}
	return -1
}

// ResolveDefaultApp resolves the "default" keyword to an app using DetectDefaultApp.
// Returns the resolved AppInfo or an error if no default can be determined.
func ResolveDefaultApp(apps []AppInfo) (*AppInfo, error) {
	idx := DetectDefaultApp(apps)
	if idx < 1 || idx > len(apps) {
		return nil, fmt.Errorf("no default app detected")
	}
	return &apps[idx-1], nil
}

// tabName composes a tmux/byobu tab or session name from repo + worktree names.
// Returns wtName alone when repoName is empty (non-git invocation), avoiding a
// leading dash. Otherwise preserves the historical "{repo}-{wt}" format.
func tabName(repoName, wtName string) string {
	if repoName == "" {
		return wtName
	}
	return repoName + "-" + wtName
}

// OpenInApp opens the given path in the specified application.
//
// Test seam: when WT_TEST_NO_LAUNCH=1 is set in the environment, every appCmd
// except "open_here" short-circuits — the function prints a marker line to
// stderr and returns nil instead of exec'ing a GUI/terminal/clipboard binary.
// Defaulted ON in cmd/wt's runWt test helper so `go test ./...` cannot leak
// real VSCode/iTerm/etc. windows onto the developer's host. The "open_here"
// case is exempt because it is cooperative (writes to WT_CD_FILE or stdout)
// and has no host side effect.
func OpenInApp(appCmd, path, repoName, wtName string) error {
	if appCmd != "open_here" && os.Getenv("WT_TEST_NO_LAUNCH") == "1" {
		fmt.Fprintf(os.Stderr, "[wt-test-no-launch] would open %q in %q (repo=%q wt=%q)\n",
			path, appCmd, repoName, wtName)
		return nil
	}
	switch appCmd {
	case "open_here":
		cdFile := os.Getenv("WT_CD_FILE")
		if cdFile != "" {
			return os.WriteFile(cdFile, []byte(path), 0600)
		}
		if os.Getenv("WT_WRAPPER") != "1" {
			fmt.Fprintln(os.Stderr, `hint: "Open here" requires the shell wrapper to cd. Run: eval "$(wt shell-init zsh)" (or bash)`)
			fmt.Fprintln(os.Stderr, `      Add it to your ~/.zshrc or ~/.bashrc to make it permanent.`)
		}
		fmt.Printf("cd -- '%s'\n", shellQuoteSingle(path))
		return nil
	case "code":
		return runCommand("code", path)
	case "cursor":
		return runCommand("cursor", path)
	case "ghostty_macos":
		return runCommand("open", "-a", "Ghostty", path)
	case "ghostty_linux":
		cmd := exec.Command("ghostty", "-e", "bash", "-c", fmt.Sprintf("cd %q && exec \"$SHELL\"", path))
		return cmd.Start()
	case "iterm":
		return runCommand("open", "-a", "iTerm", path)
	case "terminal_app":
		return runCommand("open", "-a", "Terminal", path)
	case "gnome_terminal":
		cmd := exec.Command("gnome-terminal", "--working-directory="+path)
		return cmd.Start()
	case "konsole":
		cmd := exec.Command("konsole", "--workdir", path)
		return cmd.Start()
	case "finder":
		return runCommand("open", path)
	case "nautilus":
		cmd := exec.Command("nautilus", path)
		return cmd.Start()
	case "dolphin":
		cmd := exec.Command("dolphin", path)
		return cmd.Start()
	case "copy_macos":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(path)
		if err := cmd.Run(); err != nil {
			return err
		}
		fmt.Println("Path copied to clipboard")
		return nil
	case "copy_linux":
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(path)
		if err := cmd.Run(); err != nil {
			return err
		}
		fmt.Println("Path copied to clipboard")
		return nil
	case "byobu_tab":
		name := tabName(repoName, wtName)
		if _, err := exec.LookPath("byobu"); err != nil {
			return fmt.Errorf("byobu is not available on this system")
		}
		// Clean up corrupted byobu cache
		byobuCache := filepath.Join(os.Getenv("HOME"), ".cache", "byobu", ".last.tmux")
		if info, err := os.Stat(byobuCache); err == nil && info.IsDir() {
			os.RemoveAll(byobuCache)
		}
		cmd := exec.Command("byobu", "new-window", "-n", name, "-c", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("byobu new-window failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "tmux_window":
		name := tabName(repoName, wtName)
		if _, err := exec.LookPath("tmux"); err != nil {
			return fmt.Errorf("tmux is not available on this system")
		}
		cmd := exec.Command("tmux", "new-window", "-n", name, "-c", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-window failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "tmux_session":
		sessionName := tabName(repoName, wtName)
		if _, err := exec.LookPath("tmux"); err != nil {
			return fmt.Errorf("tmux is not available on this system")
		}
		cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-session failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("unknown application: %s", appCmd)
	}
}

// SaveLastApp saves the last-used app command key to cache.
func SaveLastApp(cmd string) {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "wt")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "last-app"), []byte(cmd), 0644)
}

// appAvailable checks if an application is available on the system.
func appAvailable(cli, bundleID, desktopFile, osType string) bool {
	// Check CLI command first
	if cli != "" {
		if _, err := exec.LookPath(cli); err == nil {
			return true
		}
	}

	// OS-specific detection
	if osType == "macos" && bundleID != "" {
		out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == '"+bundleID+"'").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return true
		}
	} else if osType == "linux" && desktopFile != "" {
		if _, err := os.Stat("/usr/share/applications/" + desktopFile); err == nil {
			return true
		}
		home := os.Getenv("HOME")
		if _, err := os.Stat(filepath.Join(home, ".local/share/applications", desktopFile)); err == nil {
			return true
		}
	}

	return false
}

// shellQuoteSingle escapes a string for use inside shell single quotes.
// Single quotes prevent all shell expansion ($, `, \, etc.).
// The only character needing escaping is the single quote itself, which is
// replaced with the standard close-quote/escaped-quote/reopen-quote sequence.
func shellQuoteSingle(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// runCommand runs a command and waits for completion.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
