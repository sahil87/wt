package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRandomName_Format(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := GenerateRandomName()
		parts := strings.SplitN(name, "-", 2)
		if len(parts) != 2 {
			t.Errorf("GenerateRandomName() = %q, want adjective-noun format", name)
		}
		if parts[0] == "" || parts[1] == "" {
			t.Errorf("GenerateRandomName() = %q, parts cannot be empty", name)
		}
	}
}

func TestGenerateRandomName_Variety(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		name := GenerateRandomName()
		seen[name] = true
	}
	// With 200*200=40000 combos, 100 draws should produce at least 50 unique names
	if len(seen) < 50 {
		t.Errorf("GenerateRandomName() produced only %d unique names in 100 draws, expected more variety", len(seen))
	}
}

func TestGenerateUniqueName_Success(t *testing.T) {
	dir := t.TempDir()
	name, err := GenerateUniqueName(dir, 10)
	if err != nil {
		t.Fatalf("GenerateUniqueName() error: %v", err)
	}
	if name == "" {
		t.Error("GenerateUniqueName() returned empty name")
	}
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		t.Errorf("GenerateUniqueName() = %q, want adjective-noun format", name)
	}
}

func TestGenerateUniqueName_RetryExhaustion(t *testing.T) {
	dir := t.TempDir()

	// Temporarily replace word lists with a tiny set so we can exhaust all
	// combinations without creating 40,000+ directories.
	origAdj := adjectives
	origNoun := nouns
	adjectives = []string{"alpha", "beta"}
	nouns = []string{"one", "two"}
	t.Cleanup(func() {
		adjectives = origAdj
		nouns = origNoun
	})

	// Create directories for all 2*2=4 combinations
	for _, adj := range adjectives {
		for _, noun := range nouns {
			name := adj + "-" + noun
			if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
				t.Fatalf("failed to create directory %s: %v", name, err)
			}
		}
	}

	_, err := GenerateUniqueName(dir, 100)
	if err == nil {
		t.Error("GenerateUniqueName() should fail when all names collide")
	}
}

func TestWordListsNonEmpty(t *testing.T) {
	if len(adjectives) == 0 {
		t.Error("adjectives list is empty")
	}
	if len(nouns) == 0 {
		t.Error("nouns list is empty")
	}
	if len(adjectives) < 200 {
		t.Errorf("adjectives list has only %d entries, expected >= 200", len(adjectives))
	}
	if len(nouns) < 200 {
		t.Errorf("nouns list has only %d entries, expected >= 200", len(nouns))
	}
}

func TestGenerateUniqueNameWithPrefix_Prefixed(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		name, err := GenerateUniqueNameWithPrefix(dir, TempNamePrefix, 10)
		if err != nil {
			t.Fatalf("GenerateUniqueNameWithPrefix() error: %v", err)
		}
		if !strings.HasPrefix(name, TempNamePrefix) {
			t.Errorf("name %q missing prefix %q", name, TempNamePrefix)
		}
		rest := strings.TrimPrefix(name, TempNamePrefix)
		if parts := strings.Split(rest, "-"); len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			t.Errorf("name %q: want tmp-<adj>-<noun>", name)
		}
	}
}

func TestGenerateUniqueNameWithPrefix_CollisionChecksPrefixedName(t *testing.T) {
	dir := t.TempDir()
	origAdj, origNoun := adjectives, nouns
	adjectives = []string{"alpha"}
	nouns = []string{"one"}
	t.Cleanup(func() { adjectives, nouns = origAdj, origNoun })

	// The UNprefixed name existing must not block the prefixed generator.
	if err := os.MkdirAll(filepath.Join(dir, "alpha-one"), 0755); err != nil {
		t.Fatal(err)
	}
	name, err := GenerateUniqueNameWithPrefix(dir, TempNamePrefix, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "tmp-alpha-one" {
		t.Errorf("name = %q, want tmp-alpha-one", name)
	}

	// The prefixed name existing exhausts retries.
	if err := os.MkdirAll(filepath.Join(dir, "tmp-alpha-one"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateUniqueNameWithPrefix(dir, TempNamePrefix, 5); err == nil {
		t.Error("expected retry exhaustion when the prefixed name collides")
	}
}

func TestTempName(t *testing.T) {
	cases := map[string]string{
		"foo":      "tmp-foo",
		"tmp-foo":  "tmp-foo",
		"tmp-":     "tmp-",
		"tmpfoo":   "tmp-tmpfoo",
		"a-tmp-b":  "tmp-a-tmp-b",
		"swift-fx": "tmp-swift-fx",
	}
	for in, want := range cases {
		if got := TempName(in); got != want {
			t.Errorf("TempName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTempWorktree(t *testing.T) {
	cases := []struct {
		name     string
		detached bool
		want     bool
	}{
		{"tmp-swift-fox", true, true},
		{"tmp-swift-fox", false, false}, // tmp- name on a branch
		{"raw-det", true, false},        // detached without the prefix
		{"feature", false, false},
		{"", true, false},
	}
	for _, c := range cases {
		if got := IsTempWorktree(c.name, c.detached); got != c.want {
			t.Errorf("IsTempWorktree(%q, %v) = %v, want %v", c.name, c.detached, got, c.want)
		}
	}
}
