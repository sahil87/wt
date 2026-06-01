package worktree

import (
	"testing"
	"time"
)

func TestIsIdle_OlderThanThresholdIsIdle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	recency := now.Add(-8 * 24 * time.Hour) // 8 days ago

	if !IsIdle(recency, now, DefaultIdleThreshold) {
		t.Errorf("expected 8d-old worktree to be idle against 7d threshold")
	}
}

func TestIsIdle_NewerThanThresholdIsNotIdle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	recency := now.Add(-6 * 24 * time.Hour) // 6 days ago

	if IsIdle(recency, now, DefaultIdleThreshold) {
		t.Errorf("expected 6d-old worktree NOT to be idle against 7d threshold")
	}
}

// TestIsIdle_ExactlyAtThresholdIsNotIdle pins the strict boundary: age == threshold
// is NOT idle (only age > threshold is).
func TestIsIdle_ExactlyAtThresholdIsNotIdle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	recency := now.Add(-DefaultIdleThreshold) // exactly 7 days ago

	if IsIdle(recency, now, DefaultIdleThreshold) {
		t.Errorf("expected worktree exactly at threshold NOT to be idle (strict >, not >=)")
	}
}

// TestIsIdle_JustOverThresholdIsIdle pins the boundary one nanosecond past it.
func TestIsIdle_JustOverThresholdIsIdle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	recency := now.Add(-DefaultIdleThreshold - time.Nanosecond)

	if !IsIdle(recency, now, DefaultIdleThreshold) {
		t.Errorf("expected worktree one nanosecond past threshold to be idle")
	}
}

// TestIsIdle_ZeroRecencyIsIdle pins R2: a zero recency (vanished/unstattable
// worktree, what RecencyOf returns) is treated as idle against any positive
// threshold.
func TestIsIdle_ZeroRecencyIsIdle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	var zero time.Time

	if !IsIdle(zero, now, DefaultIdleThreshold) {
		t.Errorf("expected zero-recency (vanished) worktree to be idle")
	}
}

func TestParseIdleThreshold_EmptyResolvesToDefault(t *testing.T) {
	got, err := ParseIdleThreshold("")
	if err != nil {
		t.Fatalf("ParseIdleThreshold(\"\") returned error: %v", err)
	}
	if got != DefaultIdleThreshold {
		t.Errorf("ParseIdleThreshold(\"\") = %v, want %v", got, DefaultIdleThreshold)
	}
}

func TestParseIdleThreshold_ValidDayForms(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"365d", 365 * 24 * time.Hour},
	}
	for _, c := range cases {
		got, err := ParseIdleThreshold(c.in)
		if err != nil {
			t.Errorf("ParseIdleThreshold(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseIdleThreshold(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseIdleThreshold_RejectsInvalid(t *testing.T) {
	cases := []string{
		"banana", // non-numeric, no d suffix
		"30",     // no d suffix
		"7h",     // wrong unit
		"2w",     // wrong unit
		"dd",     // d suffix but non-integer
		"0d",     // non-positive
		"-5d",    // non-positive
		"d",      // empty number part
	}
	for _, in := range cases {
		if _, err := ParseIdleThreshold(in); err == nil {
			t.Errorf("ParseIdleThreshold(%q) expected error, got nil", in)
		}
	}
}
