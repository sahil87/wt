package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	var appFlag string
	var goFlag bool
	var listFlag bool
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "open [name|path]",
		Short: "Open a directory or worktree in an application",
		Long: `Open a directory in a detected application (editor, terminal, file manager).

"wt open" is the pure launcher half of the go/open split: go owns the "which
worktree?" menu, open owns the "which app?" menu. To pick a worktree and then
launch it, compose the two with "wt go --open".

When called without arguments, opens the current context: the current worktree
(inside a worktree), the repo root (in the main repo), or the current working
directory (outside git).

Path arguments are accepted regardless of git context. Worktree-name resolution
requires a git repository. --app works with every form and skips the app menu.

With --list, "wt open" prints the detected launchable host applications
(editors, terminals, file managers) and exits — no menu, no launch, no git
repository required. Add --json for a machine-readable JSON array of
{id, label, kind} records; each id is accepted by "wt open <path> -a <id>".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var target string
			if len(args) > 0 {
				target = args[0]
			}

			// --list is a pure query: validate flag exclusivity first, before
			// any detection or git work, then list and exit. It runs before the
			// soft git-context detection below so `wt open --list` works from
			// any cwd (external consumers may invoke it from anywhere).
			if listFlag {
				if target != "" {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"--list and a target are mutually exclusive",
						"--list queries the detected apps; it does not open anything",
						"Run 'wt open --list' with no target, or drop --list to open the target")
				}
				if appFlag != "" {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"--list and --app are mutually exclusive",
						"--list queries the detected apps; --app launches one",
						"Run 'wt open --list' to see valid --app values")
				}
				if goFlag {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"--list and --select are mutually exclusive",
						"--list queries the detected apps; --select picks a worktree to launch",
						"Run 'wt open --list' on its own")
				}
				return handleOpenList(jsonFlag)
			}
			if jsonFlag {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--json requires --list",
					"wt open has no JSON output surface besides the --list app registry",
					"Run 'wt open --list --json'")
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
				// In the main repo — open the repo root (the current context).
				// The former worktree-selection menu moved to `wt go` /
				// `wt go --open`; --app composes with this branch like every
				// other selection mode. The stable name "main" matches
				// `wt open main` and the menu's pinned main row, so launch
				// flows tab-name it {repo}-main.
				//
				// Transitional (one release): this is the invocation whose
				// behavior visibly changed, so point old muscle memory at the
				// selection verb.
				fmt.Fprintln(os.Stderr, "tip: to pick a worktree, use wt go (or wt go --open)")
				wtPath = ctx.RepoRoot
				wtName = "main"
				repoName = ctx.RepoName
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
	// --select and --go are both functional deprecated aliases for the
	// `wt go --open` composition (the selection menu moved to the selection
	// verb). Both bind the same bool; MarkDeprecated auto-hides them from
	// --help and prints a stderr warning on use. The openGo path they drive is
	// retained internally until a later removal change.
	cmd.Flags().BoolVar(&goFlag, "select", false, "Select a worktree (menu or by name) first, then launch it")
	cmd.Flags().BoolVar(&goFlag, "go", false, "Select a worktree (menu or by name) first, then launch it")
	cmd.Flags().MarkDeprecated("select", `use "wt go --open" instead`)
	cmd.Flags().MarkDeprecated("go", `use "wt go --open" instead`)
	// --list/--json are script-facing query flags: long-form only, no shorts.
	cmd.Flags().BoolVar(&listFlag, "list", false, "List detected launchable apps instead of opening anything")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "With --list, output the app registry as a JSON array")

	return cmd
}

// openAppRecord is the machine-readable record `wt open --list --json` emits
// per detected app. All three keys are always present: id is the internal
// command key (AppInfo.Cmd — the exact token `wt open <path> -a <id>`
// accepts), label the display name (AppInfo.Name), kind the closed enum
// editor|terminal|file-manager (AppInfo.Kind).
type openAppRecord struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// handleOpenList implements `wt open --list [--json]`: it lists the launchable
// host applications from the same BuildAvailableApps() catalog the interactive
// menu and -a resolution use (filtered to non-empty Kind via ListableApps,
// detection order preserved) and exits without launching anything. No git
// repository is required — app detection is host-only.
func handleOpenList(jsonOut bool) error {
	apps := wt.ListableApps(wt.BuildAvailableApps())
	if jsonOut {
		return printOpenListJSON(apps)
	}
	return printOpenListTable(apps)
}

// printOpenListJSON emits the app registry as a JSON array, mirroring
// `wt list --json`'s encoding (MarshalIndent, two-space indent, trailing
// newline). The records slice is initialized non-nil so zero detected apps
// emit `[]`, never `null` (a nil Go slice marshals to null).
func printOpenListJSON(apps []wt.AppInfo) error {
	records := make([]openAppRecord, 0, len(apps))
	for _, a := range apps {
		records = append(records, openAppRecord{ID: a.Cmd, Label: a.Name, Kind: a.Kind})
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON encoding: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printOpenListTable renders the human-mode aligned Id / Label / Kind table,
// mirroring `wt list`'s human-default/--json-opt-in split.
func printOpenListTable(apps []wt.AppInfo) error {
	if len(apps) == 0 {
		fmt.Println("No launchable applications detected.")
		return nil
	}

	idWidth, labelWidth := len("Id"), len("Label")
	for _, a := range apps {
		if l := len(a.Cmd); l > idWidth {
			idWidth = l
		}
		if l := len(a.Name); l > labelWidth {
			labelWidth = l
		}
	}

	fmt.Printf("%-*s  %-*s  %s\n", idWidth, "Id", labelWidth, "Label", "Kind")
	for _, a := range apps {
		fmt.Printf("%-*s  %-*s  %s\n", idWidth, a.Cmd, labelWidth, a.Name, a.Kind)
	}
	return nil
}

// openGo implements the deprecated `wt open --select` / `--go` composition:
// it resolves a worktree path (by name when target is non-empty, otherwise via
// the shared selection menu) and launches it via the existing launcher path
// (--app direct, or the "Open in:" app menu). Retained for back-compat — the
// primary composition is now `wt go --open`. Like `wt go`, it requires a git
// repository.
//
// Selection and the subsequent app menu share ONE MenuSession (single stdin
// reader) — see wt.MenuSession for why chaining menus on separate readers
// steals keystrokes.
func openGo(target, appFlag string) error {
	if wt.ValidateGitRepo() != nil {
		wt.ExitWithError(wt.ExitGitError,
			"Not a git repository",
			"wt open --select picks a worktree of the current repo and needs a git repository",
			"Run it from inside a git repository, or use wt go --open")
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
