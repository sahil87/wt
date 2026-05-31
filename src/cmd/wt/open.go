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

	cmd := &cobra.Command{
		Use:   "open [name|path]",
		Short: "Open a directory or worktree in an application",
		Long: `Open a directory in a detected application (editor, terminal, file manager).

When called without arguments from a worktree, opens the current worktree.
When called without arguments from the main repo, shows a worktree-selection menu.
When called without arguments from a non-git directory, opens the current working directory.

Path arguments are accepted regardless of git context. Worktree-name resolution
requires a git repository.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var target string
			if len(args) > 0 {
				target = args[0]
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
			} else {
				return handleAppMenu(wtPath, repoName, wtName)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&appFlag, "app", "", "Open in specified app, skipping the menu")

	return cmd
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

func selectAndOpen(ctx *wt.RepoContext) error {
	entries, err := listWorktreeEntries()
	if err != nil {
		return err
	}

	type wtOption struct {
		path string
		name string
	}

	var options []wtOption
	var newestPath string
	var newestTime int64

	for _, e := range entries {
		if e.path == ctx.RepoRoot {
			continue
		}
		name := filepath.Base(e.path)
		options = append(options, wtOption{path: e.path, name: name})

		// Track most recently modified
		if info, err := os.Stat(e.path); err == nil {
			mtime := info.ModTime().Unix()
			if mtime > newestTime {
				newestTime = mtime
				newestPath = e.path
			}
		}
	}

	if len(options) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	// Find default index
	defaultIdx := 1
	for i, o := range options {
		if o.path == newestPath {
			defaultIdx = i + 1
			break
		}
	}

	// Build menu
	menuNames := make([]string, len(options))
	for i, o := range options {
		// Get branch for display
		branch := getBranchForPath(o.path)
		menuNames[i] = fmt.Sprintf("%s (%s)", o.name, branch)
	}

	// One terminal session spans both menus ("Select worktree to open:" then
	// "Open in:") so they share a single stdin reader. Without this, the first
	// menu's read-ahead pump is left orphaned on stdin and steals the second
	// menu's first keystroke (see wt.MenuSession).
	session := wt.NewMenuSession()
	defer session.Close()

	choice, err := session.Show("Select worktree to open:", menuNames, defaultIdx)
	if err != nil {
		return err
	}
	if choice == 0 {
		fmt.Println("Cancelled.")
		return nil
	}

	selected := options[choice-1]
	return handleAppMenuWithSession(session, selected.path, ctx.RepoName, selected.name)
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
