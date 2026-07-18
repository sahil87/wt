package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	var appFlag string
	var goFlag bool

	cmd := &cobra.Command{
		Use:   "open [name|path]",
		Short: "Open a directory or worktree in an application",
		Long: `Open a directory in a detected application (editor, terminal, file manager).

When called without arguments from a worktree, opens the current worktree.
When called without arguments from the main repo, shows a worktree-selection menu.
When called without arguments from a non-git directory, opens the current working directory.

Path arguments are accepted regardless of git context. Worktree-name resolution
requires a git repository.

With --select, "wt open" first performs "wt go"'s worktree selection (a menu when
no name is given, or resolve-by-name when a name is given) and then launches the
selected worktree — composing the selector and the launcher. --select requires a
git repository and composes with --app.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var target string
			if len(args) > 0 {
				target = args[0]
			}

			// --select (deprecated alias: --go): compose "wt go"'s selection with
			// "wt open"'s launcher. Self-contained so the non-select paths below
			// stay untouched. goFlag holds either flag (shared variable).
			if goFlag {
				return openGo(target, appFlag)
			}

			// Soft git-context detection: git context enriches resolution but is
			// no longer a precondition. ValidateGitRepo only gates branches that
			// genuinely require a repo (worktree-name resolution, in-worktree
			// "open current" defaults, the main-repo selection menu).
			inRepo := wt.ValidateGitRepo() == nil
			var ctx *wt.RepoContext
			if inRepo {
				var err error
				ctx, err = wt.GetRepoContext()
				if err != nil {
					wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
				}
			}

			var wtPath, wtName, repoName string

			switch {
			case target != "":
				// Path-first: an existing directory always wins. When in a git
				// repo, preserve the historical tab-name format (repo-basename)
				// regardless of whether the path is actually a worktree of this
				// repo; outside a git repo, leave repoName empty and the
				// tab-name fallback in OpenInApp will use just the basename.
				info, statErr := os.Stat(target)
				switch {
				case statErr == nil && info.IsDir():
					wtPath = target
					wtName = filepath.Base(wtPath)
					if inRepo {
						repoName = ctx.RepoName
					}
				case statErr == nil && !info.IsDir():
					// Target exists but is a file (or other non-directory).
					// Don't fall through to name resolution — that would
					// produce a misleading "name resolution requires a git
					// repository" message. wt open is directory-only.
					wt.ExitWithError(wt.ExitGeneralError,
						fmt.Sprintf("Cannot open '%s'", target),
						"target exists but is not a directory; wt open accepts directories only",
						"Pass a directory path or a worktree name (in a git repo)")
				case inRepo:
					// Try as worktree name (requires git context to walk worktrees).
					path, err := resolveWorktreeByName(target, ctx)
					if err != nil {
						if errors.Is(err, errWorktreeNotFound) {
							wt.ExitWithError(wt.ExitGeneralError,
								fmt.Sprintf("Worktree '%s' not found", target),
								"No worktree with that name and not an existing directory",
								"Use 'wt list' to see available worktrees")
						}
						// listWorktreeEntries failed — a real git operation
						// error; map to ExitGitError per launcher-contract.md §5.
						wt.ExitWithError(wt.ExitGitError,
							"git worktree list failed",
							err.Error(),
							"Check 'git worktree list' from this repo")
					}
					wtPath = path
					wtName = target
					repoName = ctx.RepoName
				default:
					// Outside a git repo with a non-path arg: name resolution
					// would require the worktree list, which isn't reachable.
					wt.ExitWithError(wt.ExitGeneralError,
						fmt.Sprintf("Cannot open '%s'", target),
						"name resolution requires a git repository",
						"Example: wt open /absolute/path/to/dir")
				}
			case inRepo && wt.IsWorktree():
				// In a worktree — open it.
				var err error
				wtPath, err = wt.CurrentWorktreeTopLevel()
				if err != nil {
					wt.ExitWithError(wt.ExitGeneralError, "Cannot determine worktree root", err.Error(), "")
				}
				wtName = filepath.Base(wtPath)
				repoName = ctx.RepoName
			case inRepo:
				// In main repo — show selection menu. --app is incompatible with
				// the menu (preserved from pre-change behavior).
				if appFlag != "" {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"No worktree specified",
						"--app requires a worktree name or path, or run from within a worktree",
						"Example: wt open --app code my-worktree")
				}
				return selectAndOpen(ctx)
			default:
				// No git context, no target — open cwd.
				cwd, err := os.Getwd()
				if err != nil {
					wt.ExitWithError(wt.ExitGeneralError, "Cannot determine current directory", err.Error(), "")
				}
				wtPath = cwd
				wtName = filepath.Base(wtPath)
			}

			// Open with specified app or show menu
			if appFlag != "" {
				return openInNamedApp(appFlag, wtPath, repoName, wtName)
			}
			return handleAppMenu(wtPath, repoName, wtName)
		},
	}

	cmd.Flags().StringVarP(&appFlag, "app", "a", "", "Open in specified app, skipping the menu")
	// --select is primary; --go is the deprecated alias bound to the same bool
	// variable. No short flag for --select. --select says what it does (run the
	// worktree selector first) rather than naming the sibling `wt go` command.
	cmd.Flags().BoolVar(&goFlag, "select", false, "Select a worktree (menu or by name) first, then launch it")
	cmd.Flags().BoolVar(&goFlag, "go", false, "Select a worktree (menu or by name) first, then launch it")
	cmd.Flags().MarkDeprecated("go", "use --select instead")

	return cmd
}

// openGo implements `wt open --go`: it composes `wt go`'s worktree selection
// with `wt open`'s launcher. It resolves a worktree path (by name when target
// is non-empty, otherwise via the shared selection menu) and launches it via
// the existing launcher path (--app direct, or the "Open in:" app menu). Like
// `wt go`, --go requires a git repository.
//
// Selection and the subsequent app menu share ONE MenuSession (single stdin
// reader) — see wt.MenuSession for why chaining menus on separate readers
// steals keystrokes.
func openGo(target, appFlag string) error {
	if wt.ValidateGitRepo() != nil {
		wt.ExitWithError(wt.ExitGitError,
			"Not a git repository",
			"wt open --go selects a worktree of the current repo and needs a git repository",
			"Run wt open --go from inside a git repository")
	}

	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	var wtPath, wtName string

	if target != "" {
		path, resErr := resolveWorktreeByName(target, ctx)
		if resErr != nil {
			if errors.Is(resErr, errWorktreeNotFound) {
				wt.ExitWithError(wt.ExitGeneralError,
					fmt.Sprintf("Worktree '%s' not found", target),
					"No worktree with that name in this repository",
					"Use 'wt list' to see available worktrees")
			}
			wt.ExitWithError(wt.ExitGitError,
				"git worktree list failed",
				resErr.Error(),
				"Check 'git worktree list' from this repo")
		}
		wtPath = path
		wtName = target
	}

	// One session spans the selection menu and the "Open in:" menu.
	session := wt.NewMenuSession()
	defer session.Close()

	if target == "" {
		path, name, cancelled, selErr := selectWorktree(session, "Select worktree to open:")
		if selErr != nil {
			return selErr
		}
		if cancelled {
			fmt.Println("Cancelled.")
			return nil
		}
		wtPath = path
		wtName = name
	}

	// Launch the selected worktree. --app opens directly; otherwise the
	// "Open in:" menu runs on the same session as the selection menu.
	if appFlag != "" {
		return openInNamedApp(appFlag, wtPath, ctx.RepoName, wtName)
	}
	return handleAppMenuWithSession(session, wtPath, ctx.RepoName, wtName)
}

// openInNamedApp resolves appFlag (or the "default" keyword) against the
// available apps and launches wtPath in it. Extracted from openCmd's --app
// branch so `wt open --go --app <app>` reuses the identical resolution and
// error-mapping logic (the launcher-contract exit-code surface).
func openInNamedApp(appFlag, wtPath, repoName, wtName string) error {
	apps := wt.BuildAvailableApps()
	var resolved *wt.AppInfo
	var err error
	if appFlag == "default" {
		resolved, err = wt.ResolveDefaultApp(apps)
		if err != nil {
			wt.ExitWithError(wt.ExitGeneralError,
				"No default app detected",
				"Could not determine a default application for the current environment",
				"Use 'wt open' without --app to see the menu")
		}
	} else {
		resolved, err = wt.ResolveApp(appFlag, apps)
		if err != nil {
			wt.ExitWithError(wt.ExitGeneralError,
				fmt.Sprintf("Unknown app: %s", appFlag),
				fmt.Sprintf("App '%s' is not available on this system", appFlag),
				"Available apps can be seen with: wt open (then check the menu)")
		}
	}
	wt.SaveLastApp(resolved.Cmd)
	if openErr := wt.OpenInApp(resolved.Cmd, wtPath, repoName, wtName); openErr != nil {
		exitCode := wt.ExitGeneralError
		if strings.Contains(resolved.Cmd, "byobu") {
			exitCode = wt.ExitByobuTabError
		} else if strings.Contains(resolved.Cmd, "tmux") {
			exitCode = wt.ExitTmuxWindowError
		}
		wt.ExitWithError(exitCode,
			fmt.Sprintf("Failed to open in %s", resolved.Name),
			openErr.Error(),
			"Verify the application is running and retry")
	}
	return nil
}

// errWorktreeNotFound is returned by resolveWorktreeByName when the worktree
// list was retrieved successfully but no entry matched the requested name.
// Distinct from a git-operation failure (which propagates up unchanged) so the
// caller can map to ExitGeneralError vs. ExitGitError per launcher-contract.md.
var errWorktreeNotFound = fmt.Errorf("worktree not found")

func resolveWorktreeByName(name string, ctx *wt.RepoContext) (string, error) {
	entries, err := listWorktreeEntries()
	if err != nil {
		return "", err
	}

	for _, e := range entries {
		entryName := filepath.Base(e.path)
		if strings.EqualFold(entryName, name) {
			return e.path, nil
		}
	}

	// Stable "main" key: after the exact-basename loop finds no match, resolve
	// "main" (case-insensitive) to the porcelain-first entry, which is always
	// the main worktree (the same convention list.go uses: mainPath = raw[0]).
	// Exact-basename match above takes precedence, so a worktree directory
	// literally named "main" keeps resolving to that worktree. This matches the
	// name `wt list` displays for the main entry and fixes `wt go main` /
	// `wt open main` / `wt open --select main` in one place.
	if strings.EqualFold(name, "main") && len(entries) > 0 {
		return entries[0].path, nil
	}

	return "", errWorktreeNotFound
}

func handleAppMenu(wtPath, repoName, wtName string) error {
	session := wt.NewMenuSession()
	defer session.Close()
	return handleAppMenuWithSession(session, wtPath, repoName, wtName)
}

// handleAppMenuWithSession renders the "Open in:" menu against an existing
// terminal session. selectAndOpen passes the same session it used for the
// worktree-selection menu so the two menus share one stdin reader — see
// MenuSession for why a shared reader is required (otherwise the first menu's
// orphaned read steals this menu's first keystroke).
func handleAppMenuWithSession(session *wt.MenuSession, wtPath, repoName, wtName string) error {
	apps := wt.BuildAvailableApps()
	if len(apps) == 0 {
		fmt.Println("No supported applications detected.")
		return nil
	}

	defaultIdx := wt.DetectDefaultApp(apps)
	appNames := make([]string, len(apps))
	for i, a := range apps {
		appNames[i] = a.Name
	}

	choice, err := session.Show("Open in:", appNames, defaultIdx)
	if err != nil {
		return err
	}
	if choice == 0 {
		return nil
	}

	selected := apps[choice-1]
	wt.SaveLastApp(selected.Cmd)

	if openErr := wt.OpenInApp(selected.Cmd, wtPath, repoName, wtName); openErr != nil {
		exitCode := wt.ExitGeneralError
		if strings.Contains(selected.Cmd, "byobu") {
			exitCode = wt.ExitByobuTabError
		} else if strings.Contains(selected.Cmd, "tmux") {
			exitCode = wt.ExitTmuxWindowError
		}
		wt.ExitWithError(exitCode,
			fmt.Sprintf("Failed to open in %s", selected.Name),
			openErr.Error(),
			"Verify the application is running and retry")
	}

	return nil
}

// selectWorktree renders the current repo's worktree-selection menu against the
// provided session and returns the chosen worktree's (path, name). It is the
// single source of truth for worktree selection shared by `wt open` (main-repo
// no-arg menu), `wt go`, and `wt open --go`: it pins the main worktree to row 1
// (rendered "main (branch)"), orders the non-main entries newest-first via the
// shared recency comparator below it, displays the branch per entry, and
// pre-selects the newest non-main worktree as the default (main only when it is
// the sole row).
//
// The main worktree is the porcelain-first entry (entries[0]); it is pinned
// OUTSIDE the recency ordering, mirroring `wt list`'s sortEntries pin-first
// convention. Its returned name is the stable key "main" (the same name
// `wt list` displays), so `wt open` launch flows tab-name it {repo}/main. In a
// validated git repo `git worktree list --porcelain` always yields ≥1 entry
// (the main worktree), so the menu always has at least the pinned main row —
// there is no "no worktrees" case for this helper to signal.
//
// The caller supplies the MenuSession so that select-then-launch flows
// (`wt open` / `wt open --go`) can chain the subsequent "Open in:" menu on the
// SAME stdin reader — see wt.MenuSession for why a single reader across menus
// is required (otherwise the first menu's orphaned read-ahead pump steals the
// next menu's first keystroke).
//
// Returns cancelled=true only when the user picks Cancel (choice 0). The
// per-caller "Cancelled." message is the caller's to print. A nil error with
// cancelled=false guarantees path and name are populated.
func selectWorktree(session *wt.MenuSession, prompt string) (path, name string, cancelled bool, err error) {
	entries, err := listWorktreeEntries()
	if err != nil {
		return "", "", false, err
	}

	type wtOption struct {
		path string
		name string
	}

	// Partition out the porcelain-first entry (entries[0]), which is always the
	// main worktree — the same convention list.go's sortEntries/buildBaseEntry
	// uses (mainPath = raw[0].path). In a validated git repo porcelain always
	// yields ≥1 entry, but guard the slice defensively so an empty list can't
	// panic. The non-main entries are ordered newest-first via the shared
	// recency comparator.
	var nonMain []wtOption
	if len(entries) > 0 {
		for _, e := range entries[1:] {
			nonMain = append(nonMain, wtOption{path: e.path, name: filepath.Base(e.path)})
		}
	}
	wt.SortByRecency(nonMain,
		func(o wtOption) string { return o.path },
		func(o wtOption) string { return o.name },
	)

	// Pin the main worktree to row 1, OUTSIDE the recency ordering (mirroring
	// `wt list`'s sortEntries pin-first convention). The main entry is
	// entries[0] (porcelain-first); rendered with the stable name "main".
	options := make([]wtOption, 0, len(nonMain)+1)
	if len(entries) > 0 {
		options = append(options, wtOption{path: entries[0].path, name: "main"})
	}
	options = append(options, nonMain...)

	// The pre-selected default is the newest non-main worktree (row 2), keeping
	// the create → go → newest enter-key muscle memory — not main. When main is
	// the only row, the default falls back to it (row 1).
	defaultIdx := 1
	if len(nonMain) > 0 {
		defaultIdx = 2
	}

	// Build menu rows: "name (branch)".
	menuNames := make([]string, len(options))
	for i, o := range options {
		menuNames[i] = fmt.Sprintf("%s (%s)", o.name, getBranchForPath(o.path))
	}

	choice, err := session.Show(prompt, menuNames, defaultIdx)
	if err != nil {
		return "", "", false, err
	}
	if choice == 0 {
		return "", "", true, nil
	}

	selected := options[choice-1]
	return selected.path, selected.name, false, nil
}

func selectAndOpen(ctx *wt.RepoContext) error {
	// One terminal session spans both menus ("Select worktree to open:" then
	// "Open in:") so they share a single stdin reader. Without this, the first
	// menu's read-ahead pump is left orphaned on stdin and steals the second
	// menu's first keystroke (see wt.MenuSession).
	session := wt.NewMenuSession()
	defer session.Close()

	path, name, cancelled, err := selectWorktree(session, "Select worktree to open:")
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Println("Cancelled.")
		return nil
	}

	return handleAppMenuWithSession(session, path, ctx.RepoName, name)
}

func getBranchForPath(wtPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
