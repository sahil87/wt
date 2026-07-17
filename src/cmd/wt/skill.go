package main

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:generate ../../../scripts/sync-skill.sh

// skillBundle is the canonical agent usage bundle, copied into this package dir
// by scripts/sync-skill.sh and embedded at build time. The Go module root is
// src/ and docs/site/ sits above it, so //go:embed cannot reach the canonical
// file directly — the sync step copies it here first (see
// scripts/sync-skill.sh). The committed copy is what a clean `go build ./...`
// compiles; TestSkill_EmbedMatchesCanonical keeps it byte-honest against
// docs/site/skill.md on every `go test`. A single file needs no embed.FS.
//
//go:embed skill.md
var skillBundle []byte

// skillCmd builds the visible `wt skill` subcommand — the agent-facing usage
// bundle mandated by the sahil87 toolkit's `skill` standard. It prints the
// embedded bundle as raw markdown to stdout, byte-identical to the canonical
// docs/site/skill.md, with nothing on stderr and exit 0 — no rendering, no
// pager, no added framing (principle №2: stdout is data). It is visible (not
// Hidden) so it appears in `wt -h` and the help-dump tree.
//
// Per Constitution V this stays in cmd/: writing embedded bytes to stdout is
// trivial orchestration with no non-trivial logic to push into
// internal/worktree. SilenceUsage/SilenceErrors are set on the root command,
// so no per-command override is needed.
func skillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent usage bundle (static markdown)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write(skillBundle)
			return err
		},
	}
}
