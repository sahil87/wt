package worktree

import (
	"testing"
)

func TestValidateBranchName_Valid(t *testing.T) {
	valid := []string{
		"main",
		"feature/auth-login",
		"fix/bug-123",
		"release/v1.0",
		"my-branch",
		"some_branch",
		"a",
		"feature/nested/deep",
	}
	for _, name := range valid {
		if err := ValidateBranchName(name); err != nil {
			t.Errorf("ValidateBranchName(%q) returned error: %v", name, err)
		}
	}
}

func TestValidateBranchName_Invalid(t *testing.T) {
	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"foo bar", "space"},
		{"foo~bar", "tilde"},
		{"foo^bar", "caret"},
		{"foo:bar", "colon"},
		{"foo?bar", "question mark"},
		{"foo*bar", "asterisk"},
		{"foo[bar", "bracket"},
		{"foo..bar", "double dot"},
		{"foo.lock", ".lock suffix"},
		{".hidden", "leading dot"},
		{"foo/.hidden", "slash-dot"},
	}
	for _, tc := range invalid {
		if err := ValidateBranchName(tc.name); err == nil {
			t.Errorf("ValidateBranchName(%q) should fail (%s) but returned nil", tc.name, tc.reason)
		}
	}
}

func TestDeriveWorktreeName(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"feature/login", "login"},
		{"feature/auth-login", "auth-login"},
		{"simple", "simple"},
		{"deep/nested/branch", "branch"},
		{"dots.in.name", "dots-in-name"},
		{"under_score", "under_score"},
	}
	for _, tc := range tests {
		got := DeriveWorktreeName(tc.branch)
		if got != tc.expected {
			t.Errorf("DeriveWorktreeName(%q) = %q, want %q", tc.branch, got, tc.expected)
		}
	}
}

func TestInitScriptPath_Default(t *testing.T) {
	// When WORKTREE_INIT_SCRIPT is unset/empty, returns the built-in default
	// with isDefault=true (provenance).
	t.Setenv("WORKTREE_INIT_SCRIPT", "")
	got, isDefault := InitScriptPath()
	if got != "fab sync" {
		t.Errorf("InitScriptPath() script = %q, want %q", got, "fab sync")
	}
	if !isDefault {
		t.Errorf("InitScriptPath() isDefault = false, want true (unset env)")
	}
}

func TestInitScriptPath_Custom(t *testing.T) {
	t.Setenv("WORKTREE_INIT_SCRIPT", "custom/init.sh")
	got, isDefault := InitScriptPath()
	if got != "custom/init.sh" {
		t.Errorf("InitScriptPath() script = %q, want %q", got, "custom/init.sh")
	}
	if isDefault {
		t.Errorf("InitScriptPath() isDefault = true, want false (explicit env)")
	}
}

// TestInitScriptPath_ExplicitFabSync verifies provenance keys on env-var
// presence, NOT string equality: an explicit WORKTREE_INIT_SCRIPT="fab sync"
// yields isDefault=false even though the value matches the built-in default.
func TestInitScriptPath_ExplicitFabSync(t *testing.T) {
	t.Setenv("WORKTREE_INIT_SCRIPT", "fab sync")
	got, isDefault := InitScriptPath()
	if got != "fab sync" {
		t.Errorf("InitScriptPath() script = %q, want %q", got, "fab sync")
	}
	if isDefault {
		t.Errorf("InitScriptPath() isDefault = true, want false (explicit \"fab sync\" is not the built-in default)")
	}
}
