package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/sahil87/wt/internal/update"
	wt "github.com/sahil87/wt/internal/worktree"
)

func updateCmd() *cobra.Command {
	var skipBrewUpdate bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Self-update the wt binary via Homebrew",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := update.Run(skipBrewUpdate, version, cmd.OutOrStdout(), cmd.ErrOrStderr())
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
	// --skip-brew-update is the toolkit contract flag: the update standard
	// freezes the literal substring `--skip-brew-update` in `update --help`
	// (shll probes it via strings.Contains before every toolkit-wide run), so
	// it MUST stay visible and non-deprecated. --no-brew-update is a visible
	// alias bound to the same bool variable (same type — a shared pointer is
	// correct); behavior is identical whichever is passed, and neither prints
	// a warning.
	cmd.Flags().BoolVar(&skipBrewUpdate, "skip-brew-update", false,
		"skip the internal `brew update` tap-metadata refresh (toolkit contract flag; version check and upgrade still run)")
	cmd.Flags().BoolVar(&skipBrewUpdate, "no-brew-update", false,
		"alias for --skip-brew-update")
	return cmd
}
