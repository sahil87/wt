package main

import (
	"fmt"
	"os"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

// version is the binary version, overridden via -ldflags "-X main.version=..." at build time.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "wt",
		Short: "Git worktree management — create, list, open, delete worktrees",
		Long: `Git worktree management — create, list, open, delete worktrees.

Shell wrapper (recommended):
  To enable the "Open here" menu option (cd into a worktree in the current
  shell), add this to your shell profile (~/.bashrc or ~/.zshrc):

    eval "$(wt shell-init)"`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		createCmd(),
		listCmd(),
		openCmd(),
		goCmd(),
		deleteCmd(),
		initCmd(),
		shellInitCmd(),
		updateCmd(),
		skillCmd(),
		helpDumpCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(wt.ExitGeneralError)
	}
}
