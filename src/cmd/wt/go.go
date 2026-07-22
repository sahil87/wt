package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

func goCmd() *cobra.Command {
	var nonInteractive bool
	var openFlag string

	cmd := &cobra.Command{
		Use:   "go [name]",
		Short: "Select a worktree of the current repo and navigate there",
		Long: `Select a worktree of the current repository and navigate there.

"wt go" and "wt open" split along two axes — selection (which directory) and
action (navigate vs. launch). Each menu lives in exactly one verb: go owns the
"which worktree?" menu, open owns the "which app?" menu. Compose the two with
"wt go --open".

When called without arguments, shows the worktree-selection menu for the current
repo (main pinned to row 1, non-main newest-first, branch shown per entry). The
menu is reachable from anywhere in the repository — the main repo or inside
another worktree.

When called with a name, resolves it as a worktree (case-insensitive; the name
"main" resolves to the main worktree) and acts on it directly, with no
worktree menu.

By default the selection is navigated to: the resolved absolute path is written
to WT_CD_FILE (when set) and also printed to stdout as the last line, so both
the shell wrapper (eval "$(wt shell-init zsh)") and the scripting form
(cd "$(command wt go some-name)") work.

With --open, the selection is launched instead of navigated to (mirroring
"wt create --open"): --open prompt shows the "Open in:" app menu, --open default
launches the auto-detected default app, --open <app> launches the named app
(e.g. code, cursor, tmux_window), and --open skip is equivalent to a bare
"wt go". A value is always required (no bare --open).

Requires a git repository — worktree resolution walks the repo's worktree list.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Worktree resolution always requires a git repo (it walks the
			// repo's worktree list). Mirrors open.go's --select gating.
			if wt.ValidateGitRepo() != nil {
				wt.ExitWithError(wt.ExitGitError,
					"Not a git repository",
					"wt go resolves worktrees of the current repo and needs a git repository",
					"Run wt go from inside a git repository")
			}

			ctx, err := wt.GetRepoContext()
			if err != nil {
				wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
			}

			var target string
			if len(args) > 0 {
				target = args[0]
			}

			// A non-"skip" --open replaces navigation with launch (mirroring
			// wt create --open's grammar: prompt | default | skip | <app>).
			// "skip" and unset both mean plain navigation.
			launch := openFlag != "" && openFlag != "skip"

			if target != "" {
				path, err := resolveWorktreeByName(target, ctx)
				if err != nil {
					if errors.Is(err, errWorktreeNotFound) {
						wt.ExitWithError(wt.ExitGeneralError,
							fmt.Sprintf("Worktree '%s' not found", target),
							"No worktree with that name in this repository",
							"Use 'wt list' to see available worktrees")
					}
					// listWorktreeEntries failed — a real git operation error.
					wt.ExitWithError(wt.ExitGitError,
						"git worktree list failed",
						err.Error(),
						"Check 'git worktree list' from this repo")
				}
				if !launch {
					return navigateTo(ctx, path)
				}
				return launchSelection(nil, openFlag, path, ctx.RepoName, target)
			}

			// No name. A no-arg selection menu has no sensible non-interactive
			// default, so refuse deterministically rather than prompt or guess.
			// This refusal runs BEFORE any launch logic — it is selection's
			// precondition, independent of --open.
			if nonInteractive {
				wt.ExitWithError(wt.ExitGeneralError,
					"No worktree specified",
					"wt go with no name shows a selection menu, which has no non-interactive default",
					"Pass a worktree name: wt go <name>")
			}

			// One session spans the selection menu and (for --open prompt) the
			// "Open in:" menu — the two consecutive menus must share a single
			// stdin reader (see wt.MenuSession).
			session := wt.NewMenuSession()
			defer session.Close()

			prompt := "Select worktree to go to:"
			if launch {
				prompt = "Select worktree to open:"
			}
			path, name, cancelled, err := selectWorktree(session, prompt)
			if err != nil {
				return err
			}
			if cancelled {
				fmt.Println("Cancelled.")
				return nil
			}

			if !launch {
				return navigateTo(ctx, path)
			}
			return launchSelection(session, openFlag, path, ctx.RepoName, name)
		},
	}

	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "No prompts; require a worktree name")
	// --open deliberately has NO NoOptDefVal and no short: bare `--open code`
	// would parse `code` as the positional [name] argument (the same silent
	// footgun wt create --open avoids), so a value is always required.
	cmd.Flags().StringVar(&openFlag, "open", "", "After selection: prompt (app menu), default (auto-detect app), skip (navigate, the default), or an app name (e.g. code, cursor)")

	return cmd
}

// launchSelection launches the selected worktree via the existing launcher
// path, per the --open value: "prompt" renders the "Open in:" app menu (on the
// provided session when one is passed — required when a selection menu already
// ran on the same stdin; a fresh one-shot session otherwise), any other value
// (including "default") opens directly via openInNamedApp. This is what gives
// `wt go --open` the launcher exit codes (ExitByobuTabError,
// ExitTmuxWindowError, ExitGeneralError for unknown apps).
func launchSelection(session *wt.MenuSession, openValue, path, repoName, wtName string) error {
	if openValue == "prompt" {
		if session == nil {
			return handleAppMenu(path, repoName, wtName)
		}
		return handleAppMenuWithSession(session, path, repoName, wtName)
	}
	return openInNamedApp(openValue, path, repoName, wtName)
}

// navigateTo records the navigation target for the shell-cd contract via the
// shared wt.NavigateTo helper — the single unified implementation also used by
// the launcher's "Open here" action (launcher-contract.md §3, v2): WT_CD_FILE
// write (when set), stderr confirmation, and the bare resolved path as the
// last stdout line. Per Constitution VII, wt never cd's the parent shell
// directly; cmd/ only maps the write failure to the typed exit code.
func navigateTo(ctx *wt.RepoContext, path string) error {
	if err := wt.NavigateTo(path, ctx.RepoName, wt.BranchForPath(path)); err != nil {
		wt.ExitWithError(wt.ExitGeneralError,
			"Cannot write navigation target",
			err.Error(),
			"Check that WT_CD_FILE points to a writable path")
	}
	return nil
}

// errWorktreeNotFound is returned by resolveWorktreeByName when the worktree
// list was retrieved successfully but no entry matched the requested name.
// Distinct from a git-operation failure (which propagates up unchanged) so the
// caller can map to ExitGeneralError vs. ExitGitError per launcher-contract.md.
var errWorktreeNotFound = fmt.Errorf("worktree not found")

// resolveWorktreeByName resolves a worktree of the current repo by name,
// case-insensitively. It is the single shared resolver behind `wt go <name>`,
// `wt open <name>`, and the deprecated `wt open --select <name>`.
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

// selectWorktree renders the current repo's worktree-selection menu against the
// provided session and returns the chosen worktree's (path, name). It is the
// single source of truth for worktree selection — owned by `wt go` (the
// selection verb), with the deprecated `wt open --select` path as its other
// caller: it pins the main worktree to row 1 (rendered "main (branch)"),
// orders the non-main entries newest-first via the shared recency comparator
// below it, displays the branch per entry, and pre-selects the newest non-main
// worktree as the default (main only when it is the sole row).
//
// The main worktree is the porcelain-first entry (entries[0]); it is pinned
// OUTSIDE the recency ordering, mirroring `wt list`'s sortEntries pin-first
// convention. Its returned name is the stable key "main" (the same name
// `wt list` displays), so launch flows tab-name it {repo}/main. In a
// validated git repo `git worktree list --porcelain` always yields ≥1 entry
// (the main worktree), so the menu always has at least the pinned main row. The
// empty-list case is therefore unreachable in normal use, but the helper still
// fails fast on it (returning an error) rather than building a zero-option menu
// whose empty-input default would panic at options[choice-1].
//
// The caller supplies the MenuSession so that select-then-launch flows
// (`wt go --open prompt` / `wt open --select`) can chain the subsequent
// "Open in:" menu on the SAME stdin reader — see wt.MenuSession for why a
// single reader across menus is required (otherwise the first menu's orphaned
// read-ahead pump steals the next menu's first keystroke).
//
// Returns cancelled=true only when the user picks Cancel (choice 0). The
// per-caller "Cancelled." message is the caller's to print. A nil error with
// cancelled=false guarantees path and name are populated.
func selectWorktree(session *wt.MenuSession, prompt string) (path, name string, cancelled bool, err error) {
	entries, err := listWorktreeEntries()
	if err != nil {
		return "", "", false, err
	}

	// Fail fast on an empty worktree list. In a validated git repo
	// `git worktree list --porcelain` always yields ≥1 entry (the main
	// worktree), so this is unreachable in normal use — but building a menu with
	// zero options and defaultIdx=1 would let the empty-input default return
	// choice 1, panicking at `options[choice-1]` below. Refusing here keeps the
	// helper from ever reaching that invalid menu state.
	if len(entries) == 0 {
		return "", "", false, fmt.Errorf("%s", wt.WtError(
			"No worktrees found",
			"git worktree list returned no entries, so there is nothing to select",
			"Run this from inside a git repository with at least one worktree"))
	}

	type wtOption struct {
		path string
		name string
	}

	// Partition out the porcelain-first entry (entries[0]), which is always the
	// main worktree — the same convention list.go's sortEntries/buildBaseEntry
	// uses (mainPath = raw[0].path). entries is guaranteed non-empty by the
	// fail-fast guard above. The non-main entries are ordered newest-first via
	// the shared recency comparator.
	var nonMain []wtOption
	for _, e := range entries[1:] {
		nonMain = append(nonMain, wtOption{path: e.path, name: filepath.Base(e.path)})
	}
	wt.SortByRecency(nonMain,
		func(o wtOption) string { return o.path },
		func(o wtOption) string { return o.name },
	)

	// Pin the main worktree to row 1, OUTSIDE the recency ordering (mirroring
	// `wt list`'s sortEntries pin-first convention). The main entry is
	// entries[0] (porcelain-first); rendered with the stable name "main".
	options := make([]wtOption, 0, len(nonMain)+1)
	options = append(options, wtOption{path: entries[0].path, name: "main"})
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
		menuNames[i] = fmt.Sprintf("%s (%s)", o.name, wt.BranchForPath(o.path))
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
