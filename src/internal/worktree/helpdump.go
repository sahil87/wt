package worktree

import (
	"bytes"
	"strings"

	"github.com/spf13/cobra"
)

// helpDumpSchemaVersion is the contract revision emitted in the help-dump
// envelope. It is frozen at 1 for this revision of the shll.ai help-dump
// contract — the upstream spec is sahil87/shll.ai docs/specs/help-dump-contract.md
// §8 (in-repo behavior contract: docs/memory/wt-cli/help-dump-contract.md). New
// fields and version bumps are a separate, deliberate change.
const helpDumpSchemaVersion = 1

// toolName is the binary name reported in the help-dump envelope's `tool` field.
// The contract requires the invoked binary name (not the file slug); for this
// repo that is the fixed constant "wt" — it is not derived from argv.
const toolName = "wt"

// HelpDoc is the top-level help-dump envelope emitted to stdout by
// `wt help-dump`. It marshals to EXACTLY {tool, version, schema_version, root}.
//
// It deliberately does NOT carry a captured_at field: per the shll.ai
// help-dump contract §3, captured_at is stamped by shll.ai post-capture, and
// the tool MUST NOT emit it. Adding the field here (even omitempty) would
// drift from the contract.
type HelpDoc struct {
	Tool          string   `json:"tool"`
	Version       string   `json:"version"`
	SchemaVersion int      `json:"schema_version"`
	Root          HelpNode `json:"root"`
}

// HelpNode is one command in the recursive help tree. commands holds child
// nodes (an empty, non-nil slice for a leaf so it marshals to [] not null,
// satisfying shll.ai's NodeSchema which requires z.array).
type HelpNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Short    string     `json:"short"`
	Usage    string     `json:"usage"`
	Text     string     `json:"text"`
	Commands []HelpNode `json:"commands"`
}

// BuildHelpDump walks the Cobra command tree rooted at root and builds the
// help-dump envelope. version is the built binary's version (from
// main.version / rootCmd.Version) and is never hardcoded by this package.
//
// The tree is discovered programmatically via root.Commands() recursively to
// full depth — never by parsing -h text. Cobra's auto-generated `completion`
// and `help` subcommands and any Hidden command (which includes `help-dump`
// itself) are dropped from the tree.
func BuildHelpDump(root *cobra.Command, version string) (HelpDoc, error) {
	// Cobra adds the `-h, --help` flag and the auto-generated `help` and
	// `completion` subcommands lazily — normally during Execute(). When
	// help-dump runs, the *root* has been initialized (it is the executed
	// command), but descendant commands have not had their help flag added,
	// and the `help`/`completion` subcommands may not yet exist. Without this
	// each leaf's rendered -h would omit the `-h, --help` line and drop the
	// `[flags]` suffix from its usage. Initialize the whole tree up front so
	// the render matches a real `command -h` invocation (and the reference
	// sample).
	initHelpTree(root)

	node, err := buildNode(root)
	if err != nil {
		return HelpDoc{}, err
	}
	return HelpDoc{
		Tool:          toolName,
		Version:       version,
		SchemaVersion: helpDumpSchemaVersion,
		Root:          node,
	}, nil
}

// initHelpTree initializes Cobra's lazily-added help affordances across the
// whole command tree so that buildNode's rendered -h matches a real
// `command -h` invocation: the `-h, --help` flag on every command (which also
// makes UseLine() append the `[flags]` suffix) and the auto-generated `help`
// and `completion` subcommands on commands that have children. The added
// `help`/`completion` commands are dropped from the tree by isFilteredCommand;
// initializing them here ensures they exist (and are thus consistently hidden)
// when rendering each parent's -h. All of these initializers are idempotent.
func initHelpTree(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	// When help-dump (not the root) is the executed command, Cobra never adds
	// the root's `-v, --version` flag — execute() only does so for the command
	// being run. Add it here so the root's rendered -h matches a real `wt -h`.
	// InitDefaultVersionFlag is a no-op when cmd.Version is empty.
	cmd.InitDefaultVersionFlag()
	if cmd.HasSubCommands() {
		cmd.InitDefaultHelpCmd()
	}
	if cmd.Root() == cmd {
		cmd.InitDefaultCompletionCmd()
	}
	for _, child := range cmd.Commands() {
		initHelpTree(child)
	}
}

// buildNode renders a single command into a HelpNode and recurses into its
// non-filtered children. text is the raw -h render for cmd, captured into a
// buffer (never composed by hand or regex-parsed), with the trailing newline
// trimmed to match the committed help/wt.json reference sample byte-for-byte.
func buildNode(cmd *cobra.Command) (HelpNode, error) {
	text, err := renderHelpText(cmd)
	if err != nil {
		return HelpNode{}, err
	}

	children := make([]HelpNode, 0, len(cmd.Commands()))
	for _, child := range cmd.Commands() {
		if isFilteredCommand(child) {
			continue
		}
		childNode, err := buildNode(child)
		if err != nil {
			return HelpNode{}, err
		}
		children = append(children, childNode)
	}

	return HelpNode{
		Name:     cmd.Name(),
		Path:     cmd.CommandPath(),
		Short:    cmd.Short,
		Usage:    cmd.UseLine(),
		Text:     text,
		Commands: children,
	}, nil
}

// isFilteredCommand reports whether a command must be dropped from the help
// tree: Cobra's auto-generated `completion` and `help` subcommands, plus any
// Hidden command. Because `help-dump` is itself Hidden, this rule self-filters
// it without a special case.
func isFilteredCommand(cmd *cobra.Command) bool {
	if cmd.Hidden {
		return true
	}
	switch cmd.Name() {
	case "completion", "help":
		return true
	}
	return false
}

// renderHelpText captures cmd's -h output into a buffer and returns it with the
// trailing newline trimmed.
//
// Filtered children (completion/help/Hidden) are temporarily detached from cmd
// during the render so the command's "Available Commands" listing reflects the
// dumped tree — matching the reference sample, which omits those entries.
// Detachment (rather than toggling Hidden) is required because Cobra's usage
// template special-cases the `help` command, listing it even when Hidden via an
// explicit `(eq .Name "help")` clause; only removing it from the slice keeps it
// out of the listing. The children are re-attached before returning (Cobra
// re-sorts on AddCommand, restoring order) so live `wt -h` for real users is
// unaffected, and the SetOut/SetErr overrides are likewise restored.
func renderHelpText(cmd *cobra.Command) (text string, err error) {
	var detached []*cobra.Command
	for _, child := range cmd.Commands() {
		if isFilteredCommand(child) {
			detached = append(detached, child)
		}
	}
	if len(detached) > 0 {
		cmd.RemoveCommand(detached...)
		defer cmd.AddCommand(detached...)
	}

	var buf bytes.Buffer
	prevOut, prevErr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	defer func() {
		cmd.SetOut(prevOut)
		cmd.SetErr(prevErr)
	}()

	if err := cmd.Help(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
