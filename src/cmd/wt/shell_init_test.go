package main

import (
	"os"
	"os/exec"
	"testing"
)

// The shell-init contract under test (toolkit shell-init standard):
//   - `wt shell-init zsh` / `wt shell-init bash` → ONLY the eval-safe wrapper
//     on stdout, exit 0.
//   - Missing or unsupported shell argument → usage error: exit 2
//     (wt.ExitInvalidArgs), usage message on stderr, EMPTY stdout ($SHELL is
//     never consulted — the inference path was removed).
// Exit-code assertions run against the built binary via runWt, since the
// usage paths exit via os.Exit and cannot be asserted in-process.

func TestShellInit_ShellArg_Zsh(t *testing.T) {
	repo := createTestRepo(t)

	// SHELL deliberately set to an unsupported shell: the explicit arg is the
	// only input; the env var must not matter.
	r := runWt(t, repo, []string{"SHELL=/usr/bin/fish"}, "shell-init", "zsh")
	assertExitCode(t, r, 0)

	// Verify the wrapper function is present in stdout
	assertContains(t, r.Stdout, "wt() {")
	assertContains(t, r.Stdout, `command wt "$@"`)
	assertContains(t, r.Stdout, `cd -- "$_wt_dir"`)
	assertContains(t, r.Stdout, "export WT_WRAPPER=1")

	// stdout is the wrapper byte-for-byte — nothing else may ride along,
	// because every byte is eval'ed.
	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	if r.Stderr != "" {
		t.Errorf("expected no stderr for explicit zsh arg, got %q", r.Stderr)
	}
}

func TestShellInit_ShellArg_Bash(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL="}, "shell-init", "bash")
	assertExitCode(t, r, 0)

	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	if r.Stderr != "" {
		t.Errorf("expected no stderr for explicit bash arg, got %q", r.Stderr)
	}
}

// assertShellInitUsageError asserts the strict usage-error contract: exit 2,
// EMPTY stdout (eval-safety on the error path), and a usage message on stderr
// naming the fix.
func assertShellInitUsageError(t *testing.T, r wtResult) {
	t.Helper()
	assertExitCode(t, r, 2)
	if r.Stdout != "" {
		t.Errorf("usage-error stdout MUST be empty (it is eval'ed verbatim), got %q", r.Stdout)
	}
	assertContains(t, r.Stderr, "wt shell-init zsh")
	assertContains(t, r.Stderr, "bash")
}

func TestShellInit_NoArg_UsageError(t *testing.T) {
	repo := createTestRepo(t)

	// Even with a supported shell in $SHELL: no inference — the argument is
	// required.
	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init")
	assertShellInitUsageError(t, r)
}

func TestShellInit_NoArg_EmptyShellEnv_UsageError(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL="}, "shell-init")
	assertShellInitUsageError(t, r)
}

func TestShellInit_NoArg_UnsupportedShellEnv_UsageError(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL=/usr/bin/fish"}, "shell-init")
	assertShellInitUsageError(t, r)
}

func TestShellInit_ShellArg_Unsupported_UsageError(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init", "fish")
	assertShellInitUsageError(t, r)
	assertContains(t, r.Stderr, "fish")
}

func TestShellInit_ShellArg_TooMany(t *testing.T) {
	repo := createTestRepo(t)

	// More than one positional arg should fail (cobra.MaximumNArgs(1)).
	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init", "zsh", "extra")
	if r.ExitCode == 0 {
		t.Errorf("expected non-zero exit for too many args, got 0; stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
	if r.Stdout != "" {
		t.Errorf("expected empty stdout for too many args, got %q", r.Stdout)
	}
}

// evalWrapperInShell evals the emitted wrapper source in a real subshell and
// returns the subshell's error — the shell-init standard's recommended
// cheapest guard against a poisoned blob.
func evalWrapperInShell(t *testing.T, shellBin, wrapper string) error {
	t.Helper()
	cmd := exec.Command(shellBin, "-c", `eval "$WT_TEST_WRAPPER"`)
	cmd.Env = append(os.Environ(), "WT_TEST_WRAPPER="+wrapper)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("%s eval output:\n%s", shellBin, out)
	}
	return err
}

func TestShellInit_EvalInSubshell_Bash(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "shell-init", "bash")
	assertExitCode(t, r, 0)
	if err := evalWrapperInShell(t, "bash", r.Stdout); err != nil {
		t.Errorf("bash failed to eval shell-init output cleanly: %v", err)
	}
}

func TestShellInit_EvalInSubshell_Zsh(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not on PATH; skipping zsh eval check")
	}
	repo := createTestRepo(t)

	r := runWt(t, repo, nil, "shell-init", "zsh")
	assertExitCode(t, r, 0)
	if err := evalWrapperInShell(t, "zsh", r.Stdout); err != nil {
		t.Errorf("zsh failed to eval shell-init output cleanly: %v", err)
	}
}
