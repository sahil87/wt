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
	if err := Run("v0.0.3", false, &stdout, &stderr); err != nil {
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

// fakeBrew installs a stub `brew` executable on PATH for the duration of the
// test. The stub appends each invocation's subcommand (its first argument) to
// logPath, one per line, and answers `brew info --json=v2` with a stable
// version payload so Run's version check resolves to v9.9.9 — always newer than
// the test's "v0.0.3", which drives Run all the way through `brew upgrade`.
// Using a real on-PATH binary (rather than a Go-level seam over exec) keeps the
// production `exec.CommandContext(ctx, "brew", ...)` calls completely unchanged.
func fakeBrew(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "brew-invocations.log")

	// The stub logs $1 (the brew subcommand: update / info / upgrade) and emits
	// the JSON brew info shape only for the `info` subcommand.
	script := "#!/usr/bin/env bash\n" +
		"echo \"$1\" >> " + logPath + "\n" +
		"if [ \"$1\" = \"info\" ]; then\n" +
		"  echo '{\"formulae\":[{\"versions\":{\"stable\":\"9.9.9\"}}]}'\n" +
		"fi\n" +
		"exit 0\n"
	brewPath := filepath.Join(dir, "brew")
	if err := os.WriteFile(brewPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile fake brew: %v", err)
	}

	// Prepend our temp dir so the stub shadows any real brew on PATH.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Force the brew-install gate true; the test binary does not live under
	// /Cellar/. Restored automatically after the test.
	prev := brewInstalled
	brewInstalled = func() bool { return true }
	t.Cleanup(func() { brewInstalled = prev })

	return logPath
}

// brewSubcommands returns the ordered list of brew subcommands the stub
// recorded (e.g. ["update", "info", "upgrade"]). Missing log => no invocations.
func brewSubcommands(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadFile invocation log: %v", err)
	}
	var subs []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			subs = append(subs, line)
		}
	}
	return subs
}

func contains(subs []string, want string) bool {
	for _, s := range subs {
		if s == want {
			return true
		}
	}
	return false
}

// TestRunSkipBrewUpdate asserts the --skip-brew-update contract: with the flag
// set, the internal `brew update` tap-metadata refresh is NOT invoked, while
// the `brew info` version check AND `brew upgrade` still run unchanged.
func TestRunSkipBrewUpdate(t *testing.T) {
	logPath := fakeBrew(t)

	var stdout, stderr bytes.Buffer
	if err := Run("v0.0.3", true, &stdout, &stderr); err != nil {
		t.Fatalf("Run(skipBrewUpdate=true) returned err: %v\nstderr: %s", err, stderr.String())
	}

	subs := brewSubcommands(t, logPath)
	if contains(subs, "update") {
		t.Errorf("expected `brew update` NOT to be invoked with --skip-brew-update, got invocations: %v", subs)
	}
	if !contains(subs, "info") {
		t.Errorf("expected `brew info` version check to still run, got invocations: %v", subs)
	}
	if !contains(subs, "upgrade") {
		t.Errorf("expected `brew upgrade` to still run, got invocations: %v", subs)
	}
	if !strings.Contains(stdout.String(), "Updated to v9.9.9.") {
		t.Errorf("expected success message, got stdout:\n%s", stdout.String())
	}
}

// TestRunDefaultRunsBrewUpdate is the control: with the flag absent (default
// false), the internal `brew update` refresh runs alongside info and upgrade —
// confirming the flag changes only that one step.
func TestRunDefaultRunsBrewUpdate(t *testing.T) {
	logPath := fakeBrew(t)

	var stdout, stderr bytes.Buffer
	if err := Run("v0.0.3", false, &stdout, &stderr); err != nil {
		t.Fatalf("Run(skipBrewUpdate=false) returned err: %v\nstderr: %s", err, stderr.String())
	}

	subs := brewSubcommands(t, logPath)
	for _, want := range []string{"update", "info", "upgrade"} {
		if !contains(subs, want) {
			t.Errorf("expected `brew %s` to be invoked by default, got invocations: %v", want, subs)
		}
	}
}

// TestRunSkipBrewUpdateAlreadyCurrent confirms the "already up to date"
// short-circuit is preserved under --skip-brew-update: when brew info reports
// the current version, Run stops after the version check without invoking
// `brew upgrade` (and never invokes `brew update`).
func TestRunSkipBrewUpdateAlreadyCurrent(t *testing.T) {
	logPath := fakeBrew(t)

	var stdout, stderr bytes.Buffer
	// Stub reports 9.9.9, so passing v9.9.9 makes normalizeVersion match.
	if err := Run("v9.9.9", true, &stdout, &stderr); err != nil {
		t.Fatalf("Run returned err: %v\nstderr: %s", err, stderr.String())
	}

	subs := brewSubcommands(t, logPath)
	if contains(subs, "update") {
		t.Errorf("expected `brew update` NOT invoked with --skip-brew-update, got: %v", subs)
	}
	if !contains(subs, "info") {
		t.Errorf("expected `brew info` version check to still run, got: %v", subs)
	}
	if contains(subs, "upgrade") {
		t.Errorf("expected `brew upgrade` NOT invoked when already up to date, got: %v", subs)
	}
	if !strings.Contains(stdout.String(), "Already up to date") {
		t.Errorf("expected already-up-to-date message, got stdout:\n%s", stdout.String())
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
