package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// ---------- Brew-handling safety (toolkit update standard, change 32su) ----------

// TestBrewBoundsAreGenerous pins the bound constants required by the toolkit
// update standard's brew-handling clause: any bound must be generous (sized
// for a network transfer). The former 30s bounds — and the 120s SIGKILL bound
// on `brew upgrade`, whose constant no longer exists (its removal is a
// compile-time guarantee) — are the regression this test guards against.
func TestBrewBoundsAreGenerous(t *testing.T) {
	if brewUpdateTimeout != 5*time.Minute {
		t.Errorf("brewUpdateTimeout = %v, want 5m (generous bound per the update standard)", brewUpdateTimeout)
	}
	if brewInfoTimeout != 60*time.Second {
		t.Errorf("brewInfoTimeout = %v, want 60s (network-tolerant)", brewInfoTimeout)
	}
	if brewGraceDelay != 10*time.Second {
		t.Errorf("brewGraceDelay = %v, want 10s (SIGTERM unwind grace)", brewGraceDelay)
	}
}

// TestNewBoundedBrewCmd_GracefulSIGTERM proves that a bounded brew call whose
// context expires receives a TRAPPABLE SIGTERM, never the exec.CommandContext
// default SIGKILL. A fake `brew` first on PATH traps TERM, writes a marker
// file from the trap handler, and exits — SIGKILL cannot be trapped, so the
// marker existing is proof the graceful path ran. This is the standard's
// "MUST NOT send SIGKILL to a package-manager subprocess mid-transaction"
// clause, pinned with a short test-injected context (the production bound is
// minutes; the helper takes the timeout from its caller's ctx).
func TestNewBoundedBrewCmd_GracefulSIGTERM(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, "sigterm-trapped")
	brewPath := filepath.Join(tmpDir, "brew")

	script := `#!/bin/sh
trap 'echo trapped > "` + markerPath + `"; exit 0' TERM
# Signal readiness, then wait long past the test context's deadline.
echo ready
i=0
while [ $i -lt 100 ]; do
  sleep 0.1
  i=$((i+1))
done
exit 1
`
	if err := os.WriteFile(brewPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake brew: %v", err)
	}
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := newBoundedBrewCmd(ctx, "update", "--quiet")

	start := time.Now()
	_, err := cmd.Output()
	elapsed := time.Since(start)

	// The fake brew exits 0 from its TERM trap after the context expired, so
	// the error (if any) reflects the cancellation, never a kill.
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected nil or context.DeadlineExceeded after graceful TERM, got: %v", err)
	}
	if _, statErr := os.Stat(markerPath); statErr != nil {
		t.Fatalf("marker file missing — fake brew's TERM trap never ran, so the process was not gracefully SIGTERMed (SIGKILL is untrappable): %v", statErr)
	}
	// Sanity: the trap fired promptly (well inside the WaitDelay grace), not
	// after the fake brew's full 10s sleep loop.
	if elapsed > 5*time.Second {
		t.Errorf("bounded command took %v; expected prompt graceful termination", elapsed)
	}
}

func TestIsBrewInstalledReturnsBool(t *testing.T) {
	// Smoke test: the function must not panic on whatever `os.Executable`
	// returns in the test process. The actual return value depends on the
	// environment — in CI it's false; on a developer machine running `go
	// test` from a brew install of go it's still false (the *go* test binary
	// lives under a temp dir, not /Cellar/). We just assert it doesn't crash.
	_ = isBrewInstalled()
}
