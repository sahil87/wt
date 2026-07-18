package main

import (
	"encoding/json"
	"testing"
)

// helpDumpDoc mirrors the help-dump envelope for decoding in tests. captured_at
// is deliberately typed so we can assert its ABSENCE separately via a raw map.
type helpDumpDoc struct {
	Tool          string       `json:"tool"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schema_version"`
	Root          helpDumpNode `json:"root"`
}

type helpDumpNode struct {
	Name     string         `json:"name"`
	Aliases  []string       `json:"aliases,omitempty"`
	Path     string         `json:"path"`
	Short    string         `json:"short"`
	Usage    string         `json:"usage"`
	Text     string         `json:"text"`
	Commands []helpDumpNode `json:"commands"`
}

// TestHelpDump_EmitsValidEnvelope runs the built binary's `wt help-dump` and
// asserts the contract: exit 0, empty stderr, valid JSON, tool=="wt",
// schema_version==1, no captured_at key, and the auto-generated
// completion/help plus the Hidden help-dump itself are absent from the tree.
func TestHelpDump_EmitsValidEnvelope(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "help-dump")

	if r.Stderr != "" {
		t.Errorf("expected empty stderr on success, got: %q", r.Stderr)
	}

	// Top-level shape: exactly tool/version/schema_version/root, no captured_at.
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Stdout), &top); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, r.Stdout)
	}
	if _, ok := top["captured_at"]; ok {
		t.Error("envelope must NOT contain captured_at (shll.ai stamps it post-capture)")
	}
	allowed := map[string]bool{"tool": true, "version": true, "schema_version": true, "root": true}
	for k := range top {
		if !allowed[k] {
			t.Errorf("unexpected top-level key %q", k)
		}
	}

	var doc helpDumpDoc
	if err := json.Unmarshal([]byte(r.Stdout), &doc); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if doc.Tool != "wt" {
		t.Errorf("tool = %q, want %q", doc.Tool, "wt")
	}
	if doc.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", doc.SchemaVersion)
	}
	if doc.Version == "" {
		t.Error("version must be populated from the built binary, got empty")
	}
	if doc.Root.Name != "wt" || doc.Root.Path != "wt" {
		t.Errorf("root name/path = %q/%q, want wt/wt", doc.Root.Name, doc.Root.Path)
	}

	names := map[string]bool{}
	for _, c := range doc.Root.Commands {
		names[c.Name] = true
	}
	for _, banned := range []string{"completion", "help", "help-dump"} {
		if names[banned] {
			t.Errorf("tree must not contain %q", banned)
		}
	}
	// wt currently exposes exactly these 9 visible subcommands.
	for _, want := range []string{"create", "delete", "go", "init", "list", "open", "shell-init", "skill", "update"} {
		if !names[want] {
			t.Errorf("tree missing expected subcommand %q, got: %v", want, names)
		}
	}
	if got := len(doc.Root.Commands); got != 9 {
		t.Errorf("expected 9 visible subcommands, got %d: %v", got, names)
	}
}

// TestHelpDump_EmitsAliases asserts the emitted tree surfaces each aliased
// command's registered Cobra aliases in the structured `aliases` field:
// list→[ls], create→[new], delete→[rm]. Non-aliased nodes and the root MUST
// emit no `aliases` key at all — asserted against the raw JSON to pin omitempty.
func TestHelpDump_EmitsAliases(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "help-dump")

	// Decoded view: assert the exact alias lists on the three aliased nodes.
	var doc helpDumpDoc
	if err := json.Unmarshal([]byte(r.Stdout), &doc); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	byName := map[string]helpDumpNode{}
	for _, c := range doc.Root.Commands {
		byName[c.Name] = c
	}
	wantAliases := map[string][]string{
		"list":   {"ls"},
		"create": {"new"},
		"delete": {"rm"},
	}
	for name, want := range wantAliases {
		got := byName[name].Aliases
		if len(got) != len(want) || (len(want) == 1 && got[0] != want[0]) {
			t.Errorf("%s.aliases = %v, want %v", name, got, want)
		}
	}

	// Raw view: aliased children carry the key; non-aliased children and the
	// root do NOT (pins omitempty on the emitted JSON, not just the struct).
	var top struct {
		Root struct {
			Aliases  json.RawMessage `json:"aliases"`
			Commands []struct {
				Name    string          `json:"name"`
				Aliases json.RawMessage `json:"aliases"`
			} `json:"commands"`
		} `json:"root"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &top); err != nil {
		t.Fatalf("raw decode: %v", err)
	}
	if top.Root.Aliases != nil {
		t.Errorf("root must NOT carry an `aliases` key, got: %s", top.Root.Aliases)
	}
	aliased := map[string]bool{"list": true, "create": true, "delete": true}
	for _, c := range top.Root.Commands {
		if aliased[c.Name] {
			if c.Aliases == nil {
				t.Errorf("aliased node %q must carry an `aliases` key in the emitted JSON", c.Name)
			}
		} else if c.Aliases != nil {
			t.Errorf("non-aliased node %q must NOT carry an `aliases` key, got: %s", c.Name, c.Aliases)
		}
	}
}

// TestHelpDump_HiddenFromRootHelp asserts help-dump never appears in `wt -h`
// (it is declared Hidden).
func TestHelpDump_HiddenFromRootHelp(t *testing.T) {
	repo := createTestRepo(t)
	r := runWtSuccess(t, repo, nil, "-h")
	assertNotContains(t, r.Stdout, "help-dump")
}

// TestHelpDump_RejectsArgs asserts cobra.NoArgs enforcement: a positional arg
// surfaces a non-zero exit via main.go's error path.
func TestHelpDump_RejectsArgs(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "help-dump", "extra")
	if r.ExitCode == 0 {
		t.Fatalf("expected non-zero exit from `wt help-dump extra` (cobra.NoArgs)\nstdout: %s\nstderr: %s",
			r.Stdout, r.Stderr)
	}
}
