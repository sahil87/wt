package worktree

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestRoot builds a synthetic command tree resembling wt's: a root with a
// couple of visible leaves, a Hidden command (which must self-filter, like the
// real help-dump), and a nested child (to exercise recursive discovery). The
// auto-generated completion/help commands are added by BuildHelpDump's tree
// init, so this tree intentionally does not add them itself.
func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "wt",
		Short:   "Git worktree management",
		Long:    "Git worktree management — long description.",
		Version: "9.9.9",
	}
	root.AddCommand(&cobra.Command{
		Use:   "create [branch]",
		Short: "Create a git worktree",
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	})

	parent := &cobra.Command{
		Use:   "remote",
		Short: "Manage remotes",
	}
	parent.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a remote",
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	})
	root.AddCommand(parent)

	root.AddCommand(&cobra.Command{
		Use:    "help-dump",
		Short:  "Hidden dump command",
		Hidden: true,
		RunE:   func(cmd *cobra.Command, args []string) error { return nil },
	})
	return root
}

func TestBuildHelpDump_Envelope(t *testing.T) {
	root := newTestRoot()
	doc, err := BuildHelpDump(root, "1.2.3")
	if err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}

	if doc.Tool != "wt" {
		t.Errorf("tool = %q, want %q", doc.Tool, "wt")
	}
	if doc.Version != "1.2.3" {
		t.Errorf("version = %q, want %q (must come from the passed-in binary version)", doc.Version, "1.2.3")
	}
	if doc.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", doc.SchemaVersion)
	}
	if doc.Root.Name != "wt" {
		t.Errorf("root.name = %q, want %q", doc.Root.Name, "wt")
	}
}

// TestBuildHelpDump_OmitsCapturedAt asserts the marshaled envelope carries
// EXACTLY {tool, version, schema_version, root} and never captured_at — the
// tool must not emit captured_at (shll.ai stamps it post-capture).
func TestBuildHelpDump_OmitsCapturedAt(t *testing.T) {
	doc, err := BuildHelpDump(newTestRoot(), "1.2.3")
	if err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := top["captured_at"]; ok {
		t.Errorf("envelope must NOT contain captured_at, got keys: %v", keysOf(top))
	}
	want := map[string]bool{"tool": true, "version": true, "schema_version": true, "root": true}
	for k := range top {
		if !want[k] {
			t.Errorf("unexpected top-level key %q (envelope must be exactly tool/version/schema_version/root)", k)
		}
	}
	for k := range want {
		if _, ok := top[k]; !ok {
			t.Errorf("missing required top-level key %q", k)
		}
	}
}

// TestBuildHelpDump_FiltersCompletionHelpHidden asserts the tree drops the
// auto-generated completion/help subcommands and any Hidden command (the
// Hidden help-dump self-filters), while keeping the visible commands.
func TestBuildHelpDump_FiltersCompletionHelpHidden(t *testing.T) {
	doc, err := BuildHelpDump(newTestRoot(), "1.2.3")
	if err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}

	names := map[string]bool{}
	for _, c := range doc.Root.Commands {
		names[c.Name] = true
	}
	for _, banned := range []string{"completion", "help", "help-dump"} {
		if names[banned] {
			t.Errorf("tree must not contain %q, got commands: %v", banned, names)
		}
	}
	for _, want := range []string{"create", "remote"} {
		if !names[want] {
			t.Errorf("tree missing expected command %q, got: %v", want, names)
		}
	}
	if got := len(doc.Root.Commands); got != 2 {
		t.Errorf("root should have 2 visible subcommands (create, remote), got %d", got)
	}
}

// TestBuildHelpDump_RecursiveDiscovery asserts the walk recurses to full depth:
// the nested `remote add` child must be discovered under `remote`.
func TestBuildHelpDump_RecursiveDiscovery(t *testing.T) {
	doc, err := BuildHelpDump(newTestRoot(), "1.2.3")
	if err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}
	var remote *HelpNode
	for i := range doc.Root.Commands {
		if doc.Root.Commands[i].Name == "remote" {
			remote = &doc.Root.Commands[i]
		}
	}
	if remote == nil {
		t.Fatal("remote command not found")
	}
	if len(remote.Commands) != 1 || remote.Commands[0].Name != "add" {
		t.Errorf("expected nested `remote add`, got %+v", remote.Commands)
	}
	if got := remote.Commands[0].Path; got != "wt remote add" {
		t.Errorf("nested path = %q, want %q", got, "wt remote add")
	}
}

// TestBuildHelpDump_NodeShape asserts per-node fields and that a leaf's
// commands marshals to [] (not null), satisfying shll.ai's NodeSchema.
func TestBuildHelpDump_NodeShape(t *testing.T) {
	doc, err := BuildHelpDump(newTestRoot(), "1.2.3")
	if err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}

	var create *HelpNode
	for i := range doc.Root.Commands {
		if doc.Root.Commands[i].Name == "create" {
			create = &doc.Root.Commands[i]
		}
	}
	if create == nil {
		t.Fatal("create command not found")
	}
	if create.Path != "wt create" {
		t.Errorf("create.path = %q, want %q", create.Path, "wt create")
	}
	if create.Short != "Create a git worktree" {
		t.Errorf("create.short = %q", create.Short)
	}
	if !strings.HasPrefix(create.Usage, "wt create") {
		t.Errorf("create.usage = %q, want prefix %q", create.Usage, "wt create")
	}
	if create.Text == "" {
		t.Error("create.text must be the rendered -h output, got empty")
	}
	// The rendered -h MUST include the auto-added help flag (proves the help
	// flag is initialized across the tree, not just on the executed command).
	if !strings.Contains(create.Text, "-h, --help") {
		t.Errorf("create.text should contain the -h, --help flag line, got:\n%s", create.Text)
	}

	// Leaf commands marshal to "commands": [] (non-nil slice), never null.
	b, err := json.Marshal(create)
	if err != nil {
		t.Fatalf("marshal create: %v", err)
	}
	if !strings.Contains(string(b), `"commands":[]`) {
		t.Errorf("leaf commands must serialize as [] not null, got: %s", b)
	}
}

// TestBuildHelpDump_RestoresLiveTree asserts the render's temporary detachment
// of completion/help (and the SetOut/SetErr overrides) is restored, so a normal
// `wt -h` for real users is unaffected after a dump.
func TestBuildHelpDump_RestoresLiveTree(t *testing.T) {
	root := newTestRoot()
	if _, err := BuildHelpDump(root, "1.2.3"); err != nil {
		t.Fatalf("BuildHelpDump: %v", err)
	}

	var found []string
	for _, c := range root.Commands() {
		found = append(found, c.Name())
	}
	hasHelp, hasCompletion := false, false
	for _, n := range found {
		if n == "help" {
			hasHelp = true
		}
		if n == "completion" {
			hasCompletion = true
		}
	}
	if !hasHelp {
		t.Errorf("help command should be re-attached after a dump, got: %v", found)
	}
	if !hasCompletion {
		t.Errorf("completion command should be re-attached after a dump, got: %v", found)
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
