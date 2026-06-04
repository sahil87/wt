package main

import (
	"encoding/json"

	"github.com/spf13/cobra"

	wt "github.com/sahil87/wt/internal/worktree"
)

// helpDumpCmd builds the Hidden `wt help-dump` subcommand. It emits a single
// JSON help-dump envelope to stdout for shll.ai's scheduled puller to capture
// (see the shll.ai help-dump contract). The command is Hidden so it never
// appears in `wt -h`, and — being Hidden — self-filters from its own dump.
//
// Per Constitution V the tree-walk and envelope-building logic lives in
// internal/worktree (BuildHelpDump); this layer only wires Cobra to the
// builder and writes JSON to stdout, returning errors via RunE so main.go maps
// them to a typed exit code.
func helpDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "help-dump",
		Short:  "Emit the CLI help tree as JSON (for shll.ai capture)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := wt.BuildHelpDump(cmd.Root(), version)
			if err != nil {
				return err
			}
			out, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return err
			}
			out = append(out, '\n')
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
}
