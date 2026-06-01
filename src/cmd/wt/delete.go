package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

func deleteCmd() *cobra.Command {
	var (
		worktreeName   string
		deleteBranch   string
		deleteRemote   string
		deleteAll      bool
		stashFlag      bool
		nonInteractive bool
		staleFlag      string
	)

	cmd := &cobra.Command{
		Use:   "delete [worktree-names...]",
		Short: "Delete a git worktree",
		Long: `Delete one or more git worktrees with optional branch cleanup.

Positional arguments are interpreted as worktree names to delete.
Resolution order: --stale, --delete-all, positional args, --worktree-name (deprecated), current worktree, interactive selection.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Apply defaults (deleteBranch "" = auto mode, handled by handleBranchCleanup)
			if deleteRemote == "" {
				deleteRemote = "true"
			}

			if err := wt.ValidateGitRepo(); err != nil {
				wt.ExitWithError(wt.ExitGitError,
					"Not a git repository",
					"This command requires a git repository",
					"Navigate to a git repository and try again")
			}

			// Set up signal handling
			rb := wt.NewRollback()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Println()
				rb.Execute()
				os.Exit(130)
			}()

			stashMode := ""
			if stashFlag {
				stashMode = "stash"
			}

			// --stale mutex checks run before any git work or session setup so a
			// mis-typed threshold fails fast. cmd.Flags().Changed("stale") detects
			// the flag's presence (the bare form sets the 7d NoOptDefVal, so a nil
			// check on the value would not distinguish "absent" from "bare").
			staleRequested := cmd.Flags().Changed("stale")
			if staleRequested {
				// --stale + positional names: the space-separated --stale Nd form
				// parses Nd as a positional worktree name, so a positional alongside
				// --stale is almost always a mis-typed threshold. Convert it into a
				// loud, recoverable error (matching the --path↔--status idiom in
				// wt list) rather than silently mis-targeting.
				if len(args) > 0 {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"--stale and worktree names are mutually exclusive",
						"--stale selects idle worktrees automatically; passing names alongside it is ambiguous",
						"Use '--stale=Nd' for the threshold (the '=' is required), e.g. wt delete --stale=30d")
				}
				// --stale + --delete-all: --delete-all already targets every
				// worktree, so combining the two is contradictory.
				if deleteAll {
					wt.ExitWithError(wt.ExitInvalidArgs,
						"--stale and --delete-all are mutually exclusive",
						"--delete-all already targets every worktree; --stale is a narrowing selector",
						"Use one of them, not both")
				}
			}

			// One terminal session spans every menu this invocation shows. The
			// delete flow chains menus (e.g. selection → uncommitted-changes →
			// unpushed-commits → final confirm), and a fresh reader per menu
			// would leave the previous menu's read-ahead pump orphaned on stdin,
			// stealing the next menu's first keystroke (see wt.MenuSession).
			session := wt.NewMenuSession()
			defer session.Close()

			if staleRequested {
				return handleDeleteStale(session, staleFlag, nonInteractive, deleteBranch, deleteRemote, stashMode)
			}

			if deleteAll {
				return handleDeleteAll(session, nonInteractive, deleteBranch, deleteRemote, stashMode)
			}

			if len(args) > 0 && worktreeName != "" {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"Cannot mix positional arguments and --worktree-name",
					"Use either positional arguments or --worktree-name, not both",
					"Example: wt delete alpha bravo --non-interactive")
			}

			if len(args) > 0 {
				return handleDeleteMultiple(session, args, nonInteractive, deleteBranch, deleteRemote, stashMode)
			}

			if worktreeName != "" {
				return handleDeleteByName(session, worktreeName, nonInteractive, deleteBranch, deleteRemote, stashMode, rb)
			}

			if wt.IsWorktree() {
				return handleDeleteCurrent(session, nonInteractive, deleteBranch, deleteRemote, stashMode, rb)
			}

			if nonInteractive {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"No worktree specified",
					"In non-interactive mode, specify worktree names as arguments (or run from within a worktree)",
					"Example: wt delete my-feature --non-interactive")
			}

			return handleDeleteMenu(session, nonInteractive, deleteBranch, deleteRemote, stashMode)
		},
	}

	cmd.Flags().StringVar(&worktreeName, "worktree-name", "", "Worktree to delete")
	cmd.Flags().StringVar(&deleteBranch, "delete-branch", "", "Delete associated branch: true, false, or auto (default: auto — deletes only when branch matches worktree name)")
	cmd.Flags().StringVar(&deleteRemote, "delete-remote", "", "Delete remote branch: true (default) or false")
	cmd.Flags().BoolVar(&deleteAll, "delete-all", false, "Delete all worktrees")
	cmd.Flags().BoolVarP(&stashFlag, "stash", "s", false, "Stash uncommitted changes before deleting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "No prompts, use defaults")
	cmd.Flags().StringVar(&staleFlag, "stale", "", "Select idle worktrees (filesystem mtime older than the threshold) for deletion. Bare --stale uses the 7d default; --stale=Nd overrides (e.g. --stale=30d). The '=' is required.")
	// NoOptDefVal lets bare `--stale` carry the 7d default without an argument;
	// `--stale=Nd` overrides. The value MUST use `=` — `--stale 30d` would parse
	// 30d as a positional worktree name (cobra.ArbitraryArgs), which the
	// stale↔positional mutex below converts into a loud error rather than a
	// silent mis-target.
	cmd.Flags().Lookup("stale").NoOptDefVal = "7d"

	cmd.Flags().MarkDeprecated("worktree-name", "use positional arguments instead")

	return cmd
}

func handleDeleteCurrent(session *wt.MenuSession, nonInteractive bool, deleteBranch, deleteRemote, stashMode string, rb *wt.Rollback) error {
	if !wt.IsWorktree() {
		wt.ExitWithError(wt.ExitGeneralError,
			"Not in a worktree",
			"wt delete without --worktree-name only works from within a worktree",
			"Specify a worktree: wt delete --worktree-name <name>")
	}

	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	wtPath, err := wt.CurrentWorktreeTopLevel()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot determine worktree path", err.Error(), "")
	}
	wtName := filepath.Base(wtPath)

	branch, err := wt.CurrentBranch()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot determine current branch", err.Error(), "")
	}

	fmt.Printf("Worktree: %s%s%s\n", wt.ColorBold, wtName, wt.ColorReset)
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("Path: %s\n\n", wtPath)

	// Handle uncommitted changes
	if wt.HasUncommittedChanges() || wt.HasUntrackedFiles() {
		if err := handleUncommittedChanges(session, wtName, stashMode, nonInteractive, rb); err != nil {
			return err
		}
	}

	// Check for unpushed commits
	if wt.HasUnpushedCommits(branch) {
		if err := handleUnpushedCommits(session, branch, nonInteractive); err != nil {
			return err
		}
	}

	// Confirmation
	if !nonInteractive {
		choice, err := session.Show("Delete this worktree?", []string{"Yes, delete"}, 0)
		if err != nil {
			return err
		}
		if choice == 0 {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Change to main repo before deletion
	if err := os.Chdir(ctx.RepoRoot); err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot change to main repo",
			fmt.Sprintf("Failed to cd to %s", ctx.RepoRoot),
			"Check if the main repository still exists")
	}

	fmt.Println("Removing worktree...")
	if err := wt.RemoveWorktree(wtPath, true); err != nil {
		wt.ExitWithError(wt.ExitGitError, "Failed to remove worktree", err.Error(), "")
	}
	fmt.Printf("Deleted worktree: %s%s%s\n", wt.ColorGreen, wtName, wt.ColorReset)

	handleBranchCleanup(branch, wtName, deleteBranch, deleteRemote)

	fmt.Println()
	fmt.Println("You are no longer in a valid directory.")
	fmt.Printf("Run: %scd %s%s\n", wt.ColorBold, ctx.RepoRoot, wt.ColorReset)

	return nil
}

func handleDeleteByName(session *wt.MenuSession, name string, nonInteractive bool, deleteBranch, deleteRemote, stashMode string, rb *wt.Rollback) error {
	if err := wt.ValidateGitRepo(); err != nil {
		wt.ExitWithError(wt.ExitGitError, "Not a git repository", err.Error(), "")
	}

	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	var wtPath, branch string
	for _, e := range entries {
		if filepath.Base(e.path) == name {
			wtPath = e.path
			branch = e.branch
			break
		}
	}

	if wtPath == "" {
		wt.PrintError(
			fmt.Sprintf("Worktree '%s' not found", name),
			"No worktree with that name exists",
			"Use 'wt list' to see available worktrees")
		os.Exit(wt.ExitGeneralError)
	}

	fmt.Printf("Worktree: %s%s%s\n", wt.ColorBold, name, wt.ColorReset)
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("Path: %s\n\n", wtPath)

	// Handle stash
	if stashMode == "stash" {
		handleStashInDir(wtPath, name)
	}

	// Confirmation
	if !nonInteractive {
		choice, err := session.Show("Delete this worktree?", []string{"Yes, delete"}, 0)
		if err != nil {
			return err
		}
		if choice == 0 {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	fmt.Println("Removing worktree...")
	if err := wt.RemoveWorktree(wtPath, true); err != nil {
		wt.ExitWithError(wt.ExitGitError, "Failed to remove worktree", err.Error(), "")
	}
	fmt.Printf("Deleted worktree: %s%s%s\n", wt.ColorGreen, name, wt.ColorReset)

	handleBranchCleanup(branch, name, deleteBranch, deleteRemote)

	return nil
}

func handleDeleteMultiple(session *wt.MenuSession, names []string, nonInteractive bool, deleteBranch, deleteRemote, stashMode string) error {
	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	// If running from inside a worktree that may be deleted, chdir to main repo first
	if wt.IsWorktree() {
		if err := os.Chdir(ctx.RepoRoot); err != nil {
			wt.ExitWithError(wt.ExitGeneralError, "Cannot change to main repo",
				fmt.Sprintf("Failed to cd to %s", ctx.RepoRoot),
				"Check if the main repository still exists")
		}
	}

	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	// Build lookup map: name -> rawEntry (excluding main worktree)
	entryMap := make(map[string]rawEntry)
	for _, e := range entries {
		if e.path == ctx.RepoRoot {
			continue
		}
		entryMap[filepath.Base(e.path)] = e
	}

	// Deduplicate names preserving order
	seen := make(map[string]bool)
	var unique []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}

	// Resolve all names upfront — fail-fast if any are invalid
	type wtInfo struct {
		name   string
		path   string
		branch string
	}
	var resolved []wtInfo
	var unresolved []string
	for _, n := range unique {
		if e, ok := entryMap[n]; ok {
			resolved = append(resolved, wtInfo{
				name:   n,
				path:   e.path,
				branch: e.branch,
			})
		} else {
			unresolved = append(unresolved, n)
		}
	}

	if len(unresolved) > 0 {
		for _, n := range unresolved {
			wt.PrintError(
				fmt.Sprintf("Worktree '%s' not found", n),
				"No worktree with that name exists",
				"Use 'wt list' to see available worktrees")
		}
		os.Exit(wt.ExitGeneralError)
	}

	// Display summary
	fmt.Printf("Worktrees to delete (%d):\n", len(resolved))
	for _, w := range resolved {
		fmt.Printf("  %s%s%s  (branch: %s, path: %s)\n", wt.ColorBold, w.name, wt.ColorReset, w.branch, w.path)
	}
	fmt.Println()

	// Single confirmation prompt
	if !nonInteractive {
		choice, err := session.Show(
			fmt.Sprintf("Delete these %d worktrees?", len(resolved)),
			[]string{"Yes, delete all"},
			0)
		if err != nil {
			return err
		}
		if choice == 0 {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Sequential deletion with continue-on-error
	for _, w := range resolved {
		fmt.Printf("\n--- Deleting: %s ---\n", w.name)
		fmt.Printf("Worktree: %s%s%s\n", wt.ColorBold, w.name, wt.ColorReset)
		fmt.Printf("Branch: %s\n", w.branch)
		fmt.Printf("Path: %s\n\n", w.path)

		// Handle stash per worktree
		if stashMode == "stash" {
			handleStashInDir(w.path, w.name)
		}

		fmt.Println("Removing worktree...")
		if err := wt.RemoveWorktree(w.path, true); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %s\n", w.name, err)
			continue
		}
		fmt.Printf("Deleted worktree: %s%s%s\n", wt.ColorGreen, w.name, wt.ColorReset)
		handleBranchCleanup(w.branch, w.name, deleteBranch, deleteRemote)
	}

	return nil
}

func handleDeleteAll(session *wt.MenuSession, nonInteractive bool, deleteBranch, deleteRemote, stashMode string) error {
	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	// If in a worktree, cd to main repo
	if wt.IsWorktree() {
		if err := os.Chdir(ctx.RepoRoot); err != nil {
			wt.ExitWithError(wt.ExitGeneralError, "Cannot change to main repo",
				fmt.Sprintf("Failed to cd to %s", ctx.RepoRoot), "")
		}
	}

	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	// Collect non-main worktrees
	type wtInfo struct {
		name   string
		path   string
		branch string
	}
	var worktrees []wtInfo
	for _, e := range entries {
		if e.path != ctx.RepoRoot {
			worktrees = append(worktrees, wtInfo{
				name:   filepath.Base(e.path),
				path:   e.path,
				branch: e.branch,
			})
		}
	}

	if len(worktrees) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	fmt.Printf("Found %d worktree(s):\n", len(worktrees))
	for _, w := range worktrees {
		fmt.Printf("  %s\n", w.name)
	}
	fmt.Println()

	// Confirmation
	if !nonInteractive {
		choice, err := session.Show(
			fmt.Sprintf("Delete ALL %d worktree(s)?", len(worktrees)),
			[]string{"Yes, delete all"},
			0)
		if err != nil {
			return err
		}
		if choice == 0 {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	for _, w := range worktrees {
		fmt.Printf("\n--- Deleting: %s ---\n", w.name)
		fmt.Printf("Worktree: %s%s%s\n", wt.ColorBold, w.name, wt.ColorReset)
		fmt.Printf("Branch: %s\n", w.branch)
		fmt.Printf("Path: %s\n\n", w.path)

		fmt.Println("Removing worktree...")
		if err := wt.RemoveWorktree(w.path, true); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %s\n", w.name, err)
			continue
		}
		fmt.Printf("Deleted worktree: %s%s%s\n", wt.ColorGreen, w.name, wt.ColorReset)
		handleBranchCleanup(w.branch, w.name, deleteBranch, deleteRemote)
	}

	return nil
}

// handleDeleteStale selects every non-main worktree that is idle (filesystem
// mtime older than the resolved threshold) and routes the selection through the
// existing handleDeleteMultiple flow — so each removal still passes through the
// per-worktree stash/unpushed safety prompts and rollback. Idleness only
// pre-selects candidates; it is never the sole gate for removal (R20).
//
// staleValue is the raw flag value: "" / "7d" (bare --stale via NoOptDefVal) or
// an "Nd" override. An invalid value exits ExitInvalidArgs. Zero matches is not
// an error — it prints an informational message and exits ExitSuccess.
func handleDeleteStale(session *wt.MenuSession, staleValue string, nonInteractive bool, deleteBranch, deleteRemote, stashMode string) error {
	threshold, err := wt.ParseIdleThreshold(staleValue)
	if err != nil {
		wt.ExitWithError(wt.ExitInvalidArgs,
			"Invalid --stale threshold",
			err.Error(),
			"Use a day-suffixed integer, e.g. --stale=30d")
	}

	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	now := time.Now()
	var idleNames []string
	for _, e := range entries {
		if e.path == ctx.RepoRoot {
			continue // main worktree is never an idle candidate (R21)
		}
		if wt.IsIdle(wt.RecencyOf(e.path), now, threshold) {
			idleNames = append(idleNames, filepath.Base(e.path))
		}
	}

	if len(idleNames) == 0 {
		fmt.Printf("No idle worktrees (threshold: %s).\n", formatThreshold(threshold))
		return nil
	}

	return handleDeleteMultiple(session, idleNames, nonInteractive, deleteBranch, deleteRemote, stashMode)
}

// formatThreshold renders a whole-day duration back as an Nd string for the
// informational empty-state message, matching the --stale=Nd input form.
func formatThreshold(d time.Duration) string {
	return fmt.Sprintf("%dd", int(d.Hours())/24)
}

func handleDeleteMenu(session *wt.MenuSession, nonInteractive bool, deleteBranch, deleteRemote, stashMode string) error {
	ctx, err := wt.GetRepoContext()
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
	}

	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	type wtOption struct {
		name   string
		path   string
		branch string
	}

	var options []wtOption

	for _, e := range entries {
		if e.path == ctx.RepoRoot {
			continue
		}
		name := filepath.Base(e.path)
		options = append(options, wtOption{name: name, path: e.path, branch: e.branch})
	}

	if len(options) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	// Order newest-first via the shared recency comparator, matching the
	// wt open menu. The newest worktree lands first among worktrees and stays
	// the pre-selected default (offset by the prepended "All"/"All idle" entries).
	wt.SortByRecency(options,
		func(o wtOption) string { return o.path },
		func(o wtOption) string { return o.name },
	)

	// Determine idleness per option from the recency signal. This re-stats each
	// path once (SortByRecency does not expose its internal keys), which is
	// acceptable on the interactive menu path at the ≤100-worktree scale and keeps
	// the annotation consistent with --stale. Note R6's no-extra-stat guarantee
	// governs only the wt list path, not this menu. Idle options are annotated
	// with a trailing ", idle"; the count drives the optional "All idle (N)" entry.
	now := time.Now()
	idle := make([]bool, len(options))
	var idleNames []string
	for i, o := range options {
		if wt.IsIdle(wt.RecencyOf(o.path), now, wt.DefaultIdleThreshold) {
			idle[i] = true
			idleNames = append(idleNames, o.name)
		}
	}

	// Build the menu: "All (N worktrees)" at index 1, then "All idle (M)" at
	// index 2 ONLY when at least one worktree is idle (no "All idle (0)" row).
	// The first worktree row — and the pre-selected default — is index 2 when
	// "All idle" is absent, and shifts to 3 when it is present (amending the
	// recency-ordering contract's documented defaultIdx = 2).
	allLabel := fmt.Sprintf("All (%d worktrees)", len(options))
	menuNames := []string{allLabel}
	hasAllIdle := len(idleNames) > 0
	if hasAllIdle {
		menuNames = append(menuNames, fmt.Sprintf("All idle (%d)", len(idleNames)))
	}
	for i, o := range options {
		label := fmt.Sprintf("%s (%s)", o.name, o.branch)
		if idle[i] {
			label += ", idle"
		}
		menuNames = append(menuNames, label)
	}

	// firstWorktreeIdx is the menu index of the first (newest) worktree row.
	firstWorktreeIdx := 2
	if hasAllIdle {
		firstWorktreeIdx = 3
	}
	defaultIdx := firstWorktreeIdx

	choice, err := session.Show("Select worktree to delete:", menuNames, defaultIdx)
	if err != nil {
		return err
	}
	if choice == 0 {
		fmt.Println("Cancelled.")
		return nil
	}

	if choice == 1 {
		return handleDeleteAll(session, nonInteractive, deleteBranch, deleteRemote, stashMode)
	}

	if hasAllIdle && choice == 2 {
		// Route the idle subset through the existing multi-delete flow — no new
		// deletion code path; per-worktree safety prompts/rollback still run.
		return handleDeleteMultiple(session, idleNames, nonInteractive, deleteBranch, deleteRemote, stashMode)
	}

	selected := options[choice-firstWorktreeIdx]
	rb := wt.NewRollback()
	return handleDeleteByName(session, selected.name, nonInteractive, deleteBranch, deleteRemote, stashMode, rb)
}

func handleUncommittedChanges(session *wt.MenuSession, wtName, stashMode string, nonInteractive bool, rb *wt.Rollback) error {
	dateStr := time.Now().Format("2006-01-02")

	if stashMode == "stash" {
		fmt.Println("Stashing changes...")
		hash, err := wt.StashCreate(fmt.Sprintf("wt-delete: saved from worktree '%s' on %s", wtName, dateStr))
		if err != nil {
			return err
		}
		if hash != "" {
			fmt.Printf("Changes stashed (hash: %s). Recover with 'git stash list' or 'git stash apply %s'\n", hash, hash)
		}
		return nil
	}

	if nonInteractive {
		fmt.Println("Discarding uncommitted changes...")
		return nil
	}

	fmt.Printf("\n%sWarning:%s Worktree has uncommitted changes\n\n", wt.ColorYellow, wt.ColorReset)

	choice, err := session.Show("What would you like to do?", []string{
		"Stash changes and delete (Recommended)",
		"Discard changes and delete",
	}, 1)
	if err != nil {
		return err
	}

	switch choice {
	case 1:
		fmt.Println("Stashing changes...")
		hash, err := wt.StashCreate(fmt.Sprintf("wt-delete: saved from worktree '%s' on %s", wtName, dateStr))
		if err != nil {
			return err
		}
		if hash != "" {
			fmt.Printf("Changes stashed (hash: %s). Recover with 'git stash list' or 'git stash apply %s'\n", hash, hash)
		}
	case 2:
		fmt.Println("Discarding changes...")
	case 0:
		fmt.Println("Cancelled.")
		os.Exit(wt.ExitSuccess)
	}
	return nil
}

func handleUnpushedCommits(session *wt.MenuSession, branch string, nonInteractive bool) error {
	if nonInteractive {
		return nil
	}

	count := wt.GetUnpushedCount(branch)
	fmt.Printf("\n%sWarning:%s Branch has %d unpushed commit(s)\n\n", wt.ColorYellow, wt.ColorReset, count)

	fmt.Println("Commits that will be lost:")
	lines := wt.GetUnpushedCommitLines(branch, 5)
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
	if count > 5 {
		fmt.Printf("  ... and %d more\n", count-5)
	}
	fmt.Println()

	choice, err := session.Show("Continue anyway?", []string{
		"Yes, delete (commits will be lost)",
	}, 0)
	if err != nil {
		return err
	}
	if choice == 0 {
		fmt.Println("Cancelled.")
		os.Exit(wt.ExitSuccess)
	}
	return nil
}

func handleBranchCleanup(branch, wtName, deleteBranch, deleteRemote string) {
	if branch == "" {
		return
	}

	// Tri-state logic for deleteBranch:
	//   ""      = auto mode: delete only if branch == wtName
	//   "true"  = force delete regardless of name match
	//   "false" = skip deletion
	shouldDelete := false
	switch deleteBranch {
	case "true":
		shouldDelete = true
	case "false":
		shouldDelete = false
	default: // "" = auto mode
		if branch == wtName {
			shouldDelete = true
		} else {
			fmt.Printf("Skipped branch deletion: %s ≠ worktree name (%s). Use --delete-branch true to force.\n", branch, wtName)
		}
	}

	if shouldDelete {
		if err := wt.DeleteLocalBranch(branch, true); err == nil {
			fmt.Printf("Deleted branch: %s (local)\n", branch)
		}

		if deleteRemote == "true" && wt.BranchExistsRemotely(branch) {
			if err := wt.DeleteRemoteBranch(branch); err == nil {
				fmt.Printf("Deleted branch: %s (remote)\n", branch)
			} else {
				fmt.Printf("%sNote:%s Could not delete remote branch\n", wt.ColorYellow, wt.ColorReset)
			}
		}
	}

	// Clean up orphaned wt/ branch (always runs regardless of deleteBranch)
	wtOriginBranch := "wt/" + wtName
	if wtOriginBranch != branch {
		if err := wt.DeleteLocalBranch(wtOriginBranch, true); err == nil {
			fmt.Printf("Deleted branch: %s (local)\n", wtOriginBranch)
		}
		if deleteRemote == "true" && wt.BranchExistsRemotely(wtOriginBranch) {
			if err := wt.DeleteRemoteBranch(wtOriginBranch); err == nil {
				fmt.Printf("Deleted branch: %s (remote)\n", wtOriginBranch)
			}
		}
	}
}

func handleStashInDir(wtPath, name string) {
	// Check if there are changes to stash
	cmd := exec.Command("git", "diff", "--quiet", "HEAD")
	cmd.Dir = wtPath
	hasChanges := cmd.Run() != nil

	if !hasChanges {
		cmd = exec.Command("git", "diff", "--cached", "--quiet", "HEAD")
		cmd.Dir = wtPath
		hasChanges = cmd.Run() != nil
	}

	if !hasChanges {
		cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
		cmd.Dir = wtPath
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			hasChanges = true
		}
	}

	if !hasChanges {
		return
	}

	fmt.Println("Stashing changes...")
	dateStr := time.Now().Format("2006-01-02")
	msg := fmt.Sprintf("wt-delete: saved from worktree '%s' on %s", name, dateStr)

	// Run stash in the worktree directory
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = wtPath
	addCmd.Run()

	createCmd := exec.Command("git", "stash", "create", msg)
	createCmd.Dir = wtPath
	out, err := createCmd.Output()
	if err != nil {
		return
	}

	hash := strings.TrimSpace(string(out))
	if hash == "" {
		return
	}

	storeCmd := exec.Command("git", "stash", "store", hash, "-m", msg)
	storeCmd.Dir = wtPath
	storeCmd.Run()

	resetCmd := exec.Command("git", "reset", "--hard", "HEAD")
	resetCmd.Dir = wtPath
	resetCmd.Run()

	cleanCmd := exec.Command("git", "clean", "-fd")
	cleanCmd.Dir = wtPath
	cleanCmd.Run()

	fmt.Printf("Changes stashed (hash: %s). Recover with 'git stash list' or 'git stash apply %s'\n", hash, hash)
}
