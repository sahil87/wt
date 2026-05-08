package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/sahil87/wt/internal/update"
	wt "github.com/sahil87/wt/internal/worktree"
)

func updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "self-update the wt binary via Homebrew",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := update.Run(version, cmd.OutOrStdout(), cmd.ErrOrStderr())
			// internal/update writes its own "brew not found" hint to errOut
			// before returning ErrBrewNotFound. Exit directly with the typed
			// exit code so the user sees only the single hint — bypassing both
			// cobra's automatic error print and main.go's error formatter.
			// (Per spec Requirement: Brew-not-found handling, stderr must
			// contain exactly one line.)
			if errors.Is(err, update.ErrBrewNotFound) {
				os.Exit(wt.ExitGeneralError)
			}
			return err
		},
	}
}
