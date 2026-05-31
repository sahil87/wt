package worktree

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecencyOf_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	// Pin a known mtime so the assertion is deterministic.
	want := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dir, want, want); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got := RecencyOf(dir)
	if !got.Equal(want) {
		t.Errorf("RecencyOf(%q) = %v, want %v", dir, got, want)
	}
}

func TestRecencyOf_VanishedPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := RecencyOf(missing)
	if !got.IsZero() {
		t.Errorf("RecencyOf(missing) = %v, want zero time", got)
	}
}

func TestRecencyLess_NewestFirst(t *testing.T) {
	t1 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if !RecencyLess(t2, "any", t1, "any") {
		t.Error("expected newer (t2) to sort before older (t1)")
	}
	if RecencyLess(t1, "any", t2, "any") {
		t.Error("expected older (t1) NOT to sort before newer (t2)")
	}
}

func TestRecencyLess_NameTieBreak(t *testing.T) {
	now := time.Now()
	// Equal recency → ascending Name decides.
	if !RecencyLess(now, "alpha", now, "bravo") {
		t.Error("expected 'alpha' to sort before 'bravo' on equal mtime")
	}
	if RecencyLess(now, "bravo", now, "alpha") {
		t.Error("expected 'bravo' NOT to sort before 'alpha' on equal mtime")
	}
	// Two zero-time entries also tie-break by Name.
	var zero time.Time
	if !RecencyLess(zero, "alpha", zero, "bravo") {
		t.Error("expected zero-time 'alpha' before zero-time 'bravo'")
	}
}

func TestSortByRecency_OrdersNewestFirstWithTieBreak(t *testing.T) {
	base := t.TempDir()

	// Three worktrees with distinct mtimes t1 < t2 < t3, plus two with an
	// identical mtime to exercise the Name tie-break.
	type wt struct{ name string }
	mk := func(name string, mtime time.Time) wt {
		p := filepath.Join(base, name)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatalf("Mkdir %s: %v", name, err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatalf("Chtimes %s: %v", name, err)
		}
		return wt{name: name}
	}

	t1 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour)
	tied := t1.Add(30 * time.Minute)

	items := []wt{
		mk("oldest", t1),
		mk("newest", t3),
		mk("middle", t2),
		mk("bravo", tied),
		mk("alpha", tied),
	}

	SortByRecency(items,
		func(w wt) string { return filepath.Join(base, w.name) },
		func(w wt) string { return w.name },
	)

	var got []string
	for _, w := range items {
		got = append(got, w.name)
	}
	// newest(t3) > middle(t2) > [tied: alpha before bravo] > oldest(t1).
	want := []string{"newest", "middle", "alpha", "bravo", "oldest"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortByRecency order = %v, want %v", got, want)
		}
	}
}
