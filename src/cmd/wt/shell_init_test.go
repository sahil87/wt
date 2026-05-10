package main

import (
	"testing"
)

func TestShellInit_OutputsWrapperFunction(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init")
	assertExitCode(t, r, 0)

	// Verify the wrapper function is present in stdout
	assertContains(t, r.Stdout, "wt() {")
	assertContains(t, r.Stdout, `command wt "$@"`)
	assertContains(t, r.Stdout, `cd -- "$_wt_dir"`)
	assertContains(t, r.Stdout, "export WT_WRAPPER=1")

	// Verify the full output matches the expected wrapper
	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	// No stderr for recognized shell
	if r.Stderr != "" {
		t.Errorf("expected no stderr for zsh, got %q", r.Stderr)
	}
}

func TestShellInit_BashShell(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL=/bin/bash"}, "shell-init")
	assertExitCode(t, r, 0)

	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	if r.Stderr != "" {
		t.Errorf("expected no stderr for bash, got %q", r.Stderr)
	}
}

func TestShellInit_EmptyShell(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL="}, "shell-init")
	assertExitCode(t, r, 0)

	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	// No warning for empty SHELL
	if r.Stderr != "" {
		t.Errorf("expected no stderr for empty SHELL, got %q", r.Stderr)
	}
}

func TestShellInit_UnsupportedShell(t *testing.T) {
	repo := createTestRepo(t)

	r := runWt(t, repo, []string{"SHELL=/usr/bin/fish"}, "shell-init")
	assertExitCode(t, r, 0)

	// Still outputs the bash/zsh wrapper
	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	// Warning on stderr
	assertContains(t, r.Stderr, `warning: unsupported shell "fish"`)
	assertContains(t, r.Stderr, "outputting bash/zsh wrapper")
}

func TestShellInit_ShellArg_Zsh(t *testing.T) {
	repo := createTestRepo(t)

	// Explicit shell arg overrides $SHELL — even if $SHELL is unsupported,
	// the arg should win and produce no warning.
	r := runWt(t, repo, []string{"SHELL=/usr/bin/fish"}, "shell-init", "zsh")
	assertExitCode(t, r, 0)

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

func TestShellInit_ShellArg_Unsupported(t *testing.T) {
	repo := createTestRepo(t)

	// Unsupported shell passed as arg — warn but still output wrapper.
	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init", "fish")
	assertExitCode(t, r, 0)

	if r.Stdout != ShellWrapperFunc {
		t.Errorf("stdout does not match expected wrapper function.\nExpected:\n%s\nGot:\n%s", ShellWrapperFunc, r.Stdout)
	}

	assertContains(t, r.Stderr, `warning: unsupported shell "fish"`)
	assertContains(t, r.Stderr, "outputting bash/zsh wrapper")
}

func TestShellInit_ShellArg_TooMany(t *testing.T) {
	repo := createTestRepo(t)

	// More than one positional arg should fail.
	r := runWt(t, repo, []string{"SHELL=/bin/zsh"}, "shell-init", "zsh", "extra")
	if r.ExitCode == 0 {
		t.Errorf("expected non-zero exit for too many args, got 0; stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
}
