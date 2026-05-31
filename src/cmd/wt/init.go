package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
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

	initScriptRel := wt.InitScriptPath()

	fmt.Println("Running worktree init...")
	fmt.Println()

	// Single resolution contract — same helper that wt create's init step uses.
	cmd, notFound, err := wt.ResolveInitInvocation(initScriptRel, repoRoot)
	if err != nil {
		return fmt.Errorf("resolve init script: %w", err)
	}
	if notFound != nil {
		fmt.Println(notFound.RenderWarning())
		return nil
	}

	cmd.Dir = currentRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		// Use the typed ExitInitFailed exit code so operators / shell
		// wrappers can distinguish "init script failed" from generic
		// errors — matches the contract `wt create` uses. The actual
		// init-script output streamed to stderr above; we add a one-line
		// trailer with the underlying error and exit.
		fmt.Fprintf(os.Stderr, "\nInit script failed: %v\n", err)
		os.Exit(wt.ExitInitFailed)
	}

	fmt.Println()
	fmt.Println("Worktree init complete.")
	return nil
}
