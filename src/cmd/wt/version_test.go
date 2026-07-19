package main

import (
	"regexp"
	"strings"
	"testing"
)

// The toolkit version standard's parse contract, mirrored from shll:
//
//   - versionTokenRE — a bare version token: an optional leading `v`, at
//     least one numeric component, optional dotted components, optional
//     `[.-]<suffix>` pre-release/build metadata.
//   - versionPrefixRE — a `<word> version <rest>` first line ("version"
//     case-insensitive), from which shll takes <rest>.
//
// shll scans ONLY the first non-empty line of `<tool> --version` output,
// under a 2-second timeout. Cobra's default template emits the RECOMMENDED
// canonical shape `wt version <v>`, which satisfies the prefix rule (and the
// token rule too on release builds, where -ldflags stamps a real semver; the
// test harness's plain `go build` stamps "dev").
var (
	versionTokenRE  = regexp.MustCompile(`^v?\d+(\.\d+)*([.-][\w.+-]+)?$`)
	versionPrefixRE = regexp.MustCompile(`(?i)^(\S+) version (\S+)$`)
)

// TestVersion_FlagContract pins the version standard's invocation contract:
// `wt --version` exits 0, writes the version to stdout (stderr empty), and
// the version appears on the FIRST non-empty line in the canonical
// `wt version <rest>` shape — no banner, copyright, or update-check line may
// ever be printed above it.
func TestVersion_FlagContract(t *testing.T) {
	repo := createTestRepo(t)
	r := runWt(t, repo, nil, "--version")

	assertExitCode(t, r, 0)
	if r.Stderr != "" {
		t.Errorf("expected empty stderr from --version, got %q", r.Stderr)
	}

	var firstLine string
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			firstLine = strings.TrimSpace(line)
			break
		}
	}
	if firstLine == "" {
		t.Fatalf("--version produced no non-empty stdout line; stdout=%q", r.Stdout)
	}

	m := versionPrefixRE.FindStringSubmatch(firstLine)
	if m == nil {
		t.Fatalf("first non-empty line %q does not match the `<word> version <rest>` shape", firstLine)
	}
	if m[1] != "wt" {
		t.Errorf("version line names %q, want the tool name %q (first line: %q)", m[1], "wt", firstLine)
	}

	// The harness builds with a plain `go build`, so <rest> is "dev"; release
	// builds stamp a semver via -ldflags. Whenever <rest> is not the dev
	// placeholder it must be a valid bare version token.
	if rest := m[2]; rest != "dev" && !versionTokenRE.MatchString(rest) {
		t.Errorf("version token %q matches neither the dev placeholder nor versionTokenRE", rest)
	}
}
