package update

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"v0.0.3", "0.0.3"},
		{"0.0.3", "0.0.3"},
		{"", ""},
		{"v", ""},
		{"vvv1.0.0", "vv1.0.0"}, // only one leading "v" is stripped
	}
	for _, c := range cases {
		if got := normalizeVersion(c.in); got != c.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRunNonBrewInstall confirms that when the running binary is NOT installed
// via Homebrew, Run prints a manual-update hint to its `out` writer and
// returns nil without invoking brew. We cannot easily simulate "brew install"
// inside the test process, but we CAN observe that go's `go test` binary
// doesn't live under /Cellar/, so isBrewInstalled returns false here — making
// this assertion stable in CI and on developer machines.
func TestRunNonBrewInstall(t *testing.T) {
	if isBrewInstalled() {
		t.Skip("test binary appears to be brew-installed; non-brew code path not exercised")
	}
	var stdout, stderr bytes.Buffer
	if err := Run(false, "v0.0.3", &stdout, &stderr); err != nil {
		t.Fatalf("Run on non-brew install returned err: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"v0.0.3 was not installed via Homebrew",
		"brew install sahil87/tap/wt",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected stdout to contain %q, got:\n%s", want, out)
		}
	}
	if got := stderr.String(); got != "" {
		t.Errorf("expected empty stderr, got: %q", got)
	}
}

// TestRunSkipBrewUpdate verifies that skipBrewUpdate gates ONLY the `brew
// update` refresh. A fake `brew` script is placed first on PATH; it appends its
// first argument to a log file so the test can observe which subcommands ran.
// For `info` it emits valid `brew info --json=v2` JSON with a stable version
// that differs from currentVersion, so the upgrade path is reached. The
// WT_TEST_FORCE_BREW=1 seam forces isBrewInstalled() true so the brew code path
// is exercised even though the test binary is not under /Cellar/.
func TestRunSkipBrewUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "brew.log")
	brewPath := filepath.Join(tmpDir, "brew")

	// Fake brew: log the first arg, and for `info` print valid --json=v2 output
	// with a stable version of 9.9.9 (differs from the v0.0.0 currentVersion).
	script := `#!/bin/sh
echo "$1" >> "` + logPath + `"
if [ "$1" = "info" ]; then
  cat <<'JSON'
{"formulae":[{"versions":{"stable":"9.9.9"}}]}
JSON
fi
exit 0
`
	if err := os.WriteFile(brewPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake brew: %v", err)
	}

	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WT_TEST_FORCE_BREW", "1")

	readLog := func() string {
		b, err := os.ReadFile(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ""
			}
			t.Fatalf("reading brew log: %v", err)
		}
		return string(b)
	}

	// Skip case: brew update must NOT run; info + upgrade must run.
	t.Run("skip true omits update but upgrades", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := Run(true, "v0.0.0", &stdout, &stderr); err != nil {
			t.Fatalf("Run(skip=true) returned err: %v", err)
		}
		log := readLog()
		if strings.Contains(log, "update") {
			t.Errorf("expected log NOT to contain %q, got:\n%s", "update", log)
		}
		for _, want := range []string{"info", "upgrade"} {
			if !strings.Contains(log, want) {
				t.Errorf("expected log to contain %q, got:\n%s", want, log)
			}
		}
	})

	// Reset the log before the default-path run.
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("resetting brew log: %v", err)
	}

	// Default case: brew update MUST run.
	t.Run("skip false runs update", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := Run(false, "v0.0.0", &stdout, &stderr); err != nil {
			t.Fatalf("Run(skip=false) returned err: %v", err)
		}
		log := readLog()
		if !strings.Contains(log, "update") {
			t.Errorf("expected log to contain %q, got:\n%s", "update", log)
		}
	})
}

func TestIsBrewInstalledReturnsBool(t *testing.T) {
	// Smoke test: the function must not panic on whatever `os.Executable`
	// returns in the test process. The actual return value depends on the
	// environment — in CI it's false; on a developer machine running `go
	// test` from a brew install of go it's still false (the *go* test binary
	// lives under a temp dir, not /Cellar/). We just assert it doesn't crash.
	_ = isBrewInstalled()
}
