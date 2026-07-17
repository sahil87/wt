package main

import (
	"bytes"
	"os"
	"testing"
)

// canonicalSkillPath is the repo-relative canonical bundle, reached from the
// test's working dir (src/cmd/wt/) three levels up. Single literal, mirroring
// shll's standards_test.go precedent.
const canonicalSkillPath = "../../../docs/site/skill.md"

// runSkill drives skillCmd() with a bytes.Buffer for stdout/stderr, returning
// both — the testable seam, since the command reads only embedded bytes (no
// subprocess, no git state).
func runSkill(t *testing.T, args ...string) (stdout, stderr bytes.Buffer, err error) {
	t.Helper()
	cmd := skillCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout, stderr, err
}

// TestSkill_EmbedMatchesCanonical is the drift guard: the embedded bundle MUST
// equal the canonical docs/site/skill.md byte-for-byte. When someone edits the
// canonical file without re-running scripts/sync-skill.sh, this fails on the
// next `go test ./...` (and in CI), naming the fix. This is the load-bearing
// test of the whole embed mechanism.
func TestSkill_EmbedMatchesCanonical(t *testing.T) {
	canonical, err := os.ReadFile(canonicalSkillPath)
	if err != nil {
		t.Fatalf("read canonical %s: %v", canonicalSkillPath, err)
	}
	if !bytes.Equal(skillBundle, canonical) {
		t.Errorf("embedded skill.md has drifted from canonical %s — run `just sync-skill` (or scripts/sync-skill.sh) and commit the refreshed copy (embedded=%d bytes, canonical=%d bytes)",
			canonicalSkillPath, len(skillBundle), len(canonical))
	}
}

// TestSkill_LineBudget pins the standard's hard ≤150-line budget at build time.
// The bundle is loaded into agent context every session (and later aggregated
// across tools), so a bloated bundle taxes every conversation that pays for it.
func TestSkill_LineBudget(t *testing.T) {
	const maxLines = 150
	// Count newlines: a trailing newline yields the visible line count, and a
	// bundle with no trailing newline still counts its lines correctly for the
	// budget's purpose (never undercounting the visible lines).
	got := bytes.Count(skillBundle, []byte{'\n'})
	if !bytes.HasSuffix(skillBundle, []byte{'\n'}) && len(skillBundle) > 0 {
		got++ // final line without a trailing newline
	}
	if got > maxLines {
		t.Errorf("skill bundle is %d lines, want ≤%d (the standard's hard budget) — trim docs/site/skill.md", got, maxLines)
	}
}

// TestSkill_CommandContract asserts the invocation contract from the `skill`
// standard: raw embedded bytes to stdout, nothing to stderr, exit 0 (no error),
// and the command is visible (not Hidden).
func TestSkill_CommandContract(t *testing.T) {
	stdout, stderr, err := runSkill(t)
	if err != nil {
		t.Fatalf("wt skill err = %v, want nil", err)
	}
	if !bytes.Equal(stdout.Bytes(), skillBundle) {
		t.Errorf("stdout is not byte-identical to the embedded bundle (got %d bytes, want %d)",
			stdout.Len(), len(skillBundle))
	}
	if stderr.Len() != 0 {
		t.Errorf("skill wrote to stderr, want empty: %q", stderr.String())
	}
	if skillCmd().Hidden {
		t.Error("skill command must be visible (Hidden=false) per the standard's hidden-free requirement")
	}
}

// TestSkill_RejectsArgs asserts cobra.NoArgs enforcement — the failure path.
// The command returns an error on an unexpected positional arg and does NOT
// write the bundle. (Cobra prints its own usage text on the arg error here
// because SilenceUsage lives on the root command, not this standalone
// subcommand; the end-to-end no-stdout behavior via main.go's silenced root is
// covered by the integration harness.)
func TestSkill_RejectsArgs(t *testing.T) {
	stdout, _, err := runSkill(t, "extra")
	if err == nil {
		t.Fatal("wt skill extra err = nil, want a cobra.NoArgs error")
	}
	if bytes.Contains(stdout.Bytes(), skillBundle) {
		t.Errorf("rejected invocation must not emit the bundle, got:\n%s", stdout.String())
	}
}

// TestSkill_StaticOnly asserts the bundle is invariant across invocations — no
// dynamic, environment-derived content (contrast run-kit context).
func TestSkill_StaticOnly(t *testing.T) {
	out1, _, err1 := runSkill(t)
	out2, _, err2 := runSkill(t)
	if err1 != nil || err2 != nil {
		t.Fatalf("skill err = %v / %v, want nil", err1, err2)
	}
	if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
		t.Error("skill output differs across invocations; the bundle must be static-only")
	}
}
