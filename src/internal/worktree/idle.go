package worktree

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultIdleThreshold is the built-in age past which a worktree is considered
// idle. It is a named constant — not an environment variable or config knob —
// mirroring the maxListConcurrency "named constant, not a knob" precedent. The
// only per-invocation override is `wt delete --stale=Nd`.
const DefaultIdleThreshold = 7 * 24 * time.Hour

// IsIdle reports whether a worktree with the given recency time is idle as of
// now, against threshold. It is the single definition of "idle" shared by
// wt list and wt delete.
//
// The boundary is strict: a worktree whose age is exactly threshold is NOT idle;
// only age > threshold is idle. recency is taken as a value (not a path) so
// callers reuse a recency time they have already computed (e.g. listEntry's
// LastActive, or a RecencyOf result) — the predicate itself never stats.
//
// A zero recency (time.Time{}, what RecencyOf returns for a vanished or
// unstattable worktree) is treated as idle against any positive threshold:
// now.Sub(zeroTime) is an enormous positive duration, so this falls out of the
// comparison naturally. An unstattable worktree is, if anything, a stronger
// cleanup candidate — never a fresh one.
func IsIdle(recency time.Time, now time.Time, threshold time.Duration) bool {
	return now.Sub(recency) > threshold
}

// ParseIdleThreshold parses a day-suffixed integer threshold string of the form
// `Nd` (e.g. "7d", "30d") into a time.Duration. An empty string (bare
// `--stale` via pflag NoOptDefVal) resolves to DefaultIdleThreshold.
//
// A value with no `d` suffix, a non-integer day count, or a non-positive value
// is rejected with an error naming the accepted form. Only the `d` (day) suffix
// is supported — hours and weeks are deliberately out of scope.
func ParseIdleThreshold(s string) (time.Duration, error) {
	if s == "" {
		return DefaultIdleThreshold, nil
	}

	if !strings.HasSuffix(s, "d") {
		return 0, fmt.Errorf("invalid threshold %q: expected a day-suffixed integer like 7d or 30d", s)
	}

	numPart := strings.TrimSuffix(s, "d")
	days, err := strconv.Atoi(numPart)
	if err != nil {
		return 0, fmt.Errorf("invalid threshold %q: expected a day-suffixed integer like 7d or 30d", s)
	}
	if days <= 0 {
		return 0, fmt.Errorf("invalid threshold %q: day count must be a positive integer like 7d or 30d", s)
	}

	// Guard against int64 overflow: time.Duration is int64 nanoseconds, capping
	// the representable range at ~106751 days. Without this check a too-large day
	// count wraps to a negative duration, which would make IsIdle treat every
	// worktree as idle — dangerous for --stale selection (it would target all
	// worktrees for deletion). Reject anything that does not round-trip.
	const maxDays = int(int64(^uint64(0)>>1) / int64(24*time.Hour)) // MaxInt64 / 24h
	if days > maxDays {
		return 0, fmt.Errorf("invalid threshold %q: day count too large (max %dd)", s, maxDays)
	}

	return time.Duration(days) * 24 * time.Hour, nil
}
