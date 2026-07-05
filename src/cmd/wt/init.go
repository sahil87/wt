package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Run worktree init script",
		Long: `Run the worktree init script for the current repository.
If the init script doesn't exist, exits with guidance.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := wt.ValidateGitRepo(); err != nil {
				wt.ExitWithError(wt.ExitGitError,
					"Not a git repository",
					"This command requires a git repository",
					"Navigate to a git repository and try again")
			}

			return runInitScript()
		},
	}

	return cmd
}

func runInitScript() error {
	// Resolve main repo root using git-common-dir
	out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot determine git common dir", err.Error(), "")
	}

	gitCommonDir := strings.TrimSpace(string(out))
	absPath, err := filepath.Abs(gitCommonDir)
	if err != nil {
		wt.ExitWithError(wt.ExitGeneralError, "Cannot resolve path", err.Error(), "")
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		resolved = absPath
	}

	repoRoot := strings.TrimSuffix(resolved, string(filepath.Separator)+".git")

	// Get current toplevel (worktree or main repo)
	topOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot determine toplevel", err.Error(), "")
	}
	currentRoot := strings.TrimSpace(string(topOut))

	initScriptRel, isDefault := wt.InitScriptPath()

	// Single resolution contract — same helper that wt create's init step uses.
	cmd, notFound, err := wt.ResolveInitInvocation(initScriptRel, repoRoot)
	if err != nil {
		return fmt.Errorf("resolve init script: %w", err)
	}
	if notFound != nil {
		// No command to label — emit the canonical warning, no separator.
		fmt.Fprintln(os.Stderr, notFound.RenderWarning())
		return nil
	}

	// wt init has no machine-readable result, so all of its output is
	// diagnostic and belongs on stderr (matching wt create's init runner).
	// The Init separator is labeled with the resolved init-script value.
	fmt.Fprintln(os.Stderr, wt.PhaseSeparator("Init ("+initScriptRel+")"))
	fmt.Fprintln(os.Stderr, "Running worktree init...")
	fmt.Fprintln(os.Stderr)

	cmd.Dir = currentRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Terminal-foreground bookkeeping — same contract as wt create's init
	// step. The init child runs in its own process group (Setpgid: true) on a
	// shared controlling terminal; if it (or a descendant) grabs terminal
	// foreground and exits without restoring it, wt would be left in the
	// background and the user's shell stranded at a suspended prompt after
	// wt init returns. Capture the foreground pgrp before the child and
	// reclaim it after, on both the success and failure exit paths. No-op when
	// stdin is not a TTY (piped / non-interactive / CI).
	ttyFd := int(os.Stdin.Fd())
	reclaimTTY := term.IsTerminal(ttyFd)
	var wtPgid int
	if reclaimTTY {
		// The captured foreground pgrp IS wt's own process group, since wt is
		// the terminal foreground at this point.
		fg, err := terminalForeground(ttyFd)
		if err != nil {
			reclaimTTY = false
		} else {
			wtPgid = fg
		}
	}
	if reclaimTTY {
		// Best-effort safety net for any exit path of this function; the
		// explicit reclaims below are the ordered, load-bearing ones.
		defer reclaimTerminalForeground(ttyFd, wtPgid)
	}

	if err := cmd.Run(); err != nil {
		// Reclaim the terminal foreground BEFORE any further stderr write so it
		// cannot SIGTTOU — this ordering is load-bearing for both the skip
		// warning and the failure trailer below.
		if reclaimTTY {
			reclaimTerminalForeground(ttyFd, wtPgid)
		}
		// Default-not-applicable skip: the built-in "fab sync" default ran in a
		// repo that is not fab-managed and exited ExitNotManaged (3). Treat it as
		// success — warn on stderr and return nil (exit 0), no ExitInitFailed, no
		// "Worktree init complete." trailer. An explicitly configured script
		// (isDefault=false) or any other exit code falls through to the hard
		// failure below.
		if wt.DefaultNotApplicable(err, isDefault) {
			fmt.Fprintln(os.Stderr, wt.RenderDefaultSkipWarning())
			return nil
		}
		// Use the typed ExitInitFailed exit code so operators / shell
		// wrappers can distinguish "init script failed" from generic
		// errors — matches the contract `wt create` uses. The actual
		// init-script output streamed to stderr above; we add a one-line
		// trailer with the underlying error and exit.
		fmt.Fprintf(os.Stderr, "\nInit script failed: %v\n", err)
		os.Exit(wt.ExitInitFailed)
	}

	if reclaimTTY {
		// Reclaim before the completion trailer write.
		reclaimTerminalForeground(ttyFd, wtPgid)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Worktree init complete.")
	return nil
}
