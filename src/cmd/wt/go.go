package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

func goCmd() *cobra.Command {
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "go [name]",
		Short: "Select a worktree of the current repo and navigate there",
		Long: `Select a worktree of the current repository and navigate there.

Unlike "wt open", "wt go" does not launch any application — it only changes the
shell's working directory to the selected worktree (open=launcher, go=selector).

When called without arguments, shows a worktree-selection menu for the current
repo (newest-first, branch shown per entry). The menu is reachable from anywhere
in the repository — the main repo or inside another worktree.

When called with a name, resolves it as a worktree (case-insensitive) and
navigates there directly, with no menu.

Navigation reuses the same shell-cd plumbing as the "Open here" launcher option:
the resolved absolute path is written to WT_CD_FILE (when set) and also printed
to stdout as the last line, so both the shell wrapper (eval "$(wt shell-init)")
and the scripting form (cd "$(command wt go some-name)") work.

Requires a git repository — worktree resolution walks the repo's worktree list.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Worktree resolution always requires a git repo (it walks the
			// repo's worktree list). Mirrors open.go's git-context gating.
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
				return navigateTo(ctx, path)
			}

			// No name. A no-arg selection menu has no sensible non-interactive
			// default, so refuse deterministically rather than prompt or guess.
			if nonInteractive {
				wt.ExitWithError(wt.ExitGeneralError,
					"No worktree specified",
					"wt go with no name shows a selection menu, which has no non-interactive default",
					"Pass a worktree name: wt go <name>")
			}

			session := wt.NewMenuSession()
			defer session.Close()

			path, _, cancelled, noWorktrees, err := selectWorktree(ctx, session, "Select worktree to go to:")
			if err != nil {
				return err
			}
			if cancelled {
				// "No worktrees found." is printed by selectWorktree; only the
				// explicit Cancel path needs the "Cancelled." line.
				if !noWorktrees {
					fmt.Println("Cancelled.")
				}
				return nil
			}

			return navigateTo(ctx, path)
		},
	}

	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "No prompts; require a worktree name")

	return cmd
}

// navigateTo records the navigation target for the shell-cd contract. It writes
// the resolved absolute path to WT_CD_FILE (when set) so the wt() shell wrapper
// cd's the parent shell there, and always prints the path to stdout as the last
// line so the no-wrapper scripting form (cd "$(command wt go ...)") works.
//
// A compact-arrow navigation confirmation (repo / worktree / branch + the
// absolute path) is written to STDERR so the user can see where they are landing
// without polluting the stdout machine contract. stdout stays the bare resolved
// path only — this preserves cd "$(command wt go ...)" and the WT_CD_FILE write.
//
// Per Constitution VII, wt never cd's the parent shell directly — it cooperates
// via WT_CD_FILE / stdout and the shell wrapper evaluates the result. The
// WT_CD_FILE semantics (mode 0600, truncate-on-write, contents = resolved dir
// path) are the same ones documented in launcher-contract.md §3 for "Open here".
func navigateTo(ctx *wt.RepoContext, path string) error {
	// The WT_CD_FILE write is the operation that can still fail, so it runs
	// BEFORE the success confirmation. Emitting the "→ repo / …" line first
	// would print a misleading success message even when the write then errors
	// out and exits non-zero.
	if cdFile := os.Getenv("WT_CD_FILE"); cdFile != "" {
		if err := os.WriteFile(cdFile, []byte(path), 0600); err != nil {
			wt.ExitWithError(wt.ExitGeneralError,
				"Cannot write navigation target",
				err.Error(),
				"Check that WT_CD_FILE points to a writable path")
		}
	} else if os.Getenv("WT_WRAPPER") != "1" {
		fmt.Fprintln(os.Stderr, `hint: wt go requires the shell wrapper to cd. Run: eval "$(wt shell-init)"`)
		fmt.Fprintln(os.Stderr, `      Add it to your ~/.zshrc or ~/.bashrc to make it permanent.`)
	}

	// Confirmation block (stderr, human copy). getBranchForPath reuses the same
	// single git rev-parse the open/go menus use. Emits no color, so it is
	// NO_COLOR-safe by construction. Printed only after the WT_CD_FILE write
	// (above) has succeeded — there are no further error/exit conditions below.
	fmt.Fprintf(os.Stderr, "→ %s / %s  (%s)\n", ctx.RepoName, filepath.Base(path), getBranchForPath(path))
	fmt.Fprintf(os.Stderr, "  %s\n", path)

	// Always emit the resolved path as the last stdout line for the scripting
	// path: cd "$(command wt go some-name)".
	fmt.Println(path)
	return nil
}
