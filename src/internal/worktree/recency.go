package worktree

import (
	"os"
	"sort"
	"time"
)

// RecencyOf returns the recency signal for a worktree: the mtime of its
// working-directory root. Returns the zero time.Time if the path cannot be
// stat'd (vanished worktree, permissions error) rather than an error — recency
// is an ordering hint, not an operation that should fail a command.
func RecencyOf(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// RecencyLess reports whether the entry described by (aRecency, aName) should
// sort before (bRecency, bName) in recency order: most-recent first, with a
// deterministic Name-ascending tie-break when recencies are equal (including
// two zero-time entries). This single definition of "newer first" is shared by
// wt list, wt open, and wt delete so the three never drift apart.
func RecencyLess(aRecency time.Time, aName string, bRecency time.Time, bName string) bool {
	if aRecency.Equal(bRecency) {
		return aName < bName
	}
	return aRecency.After(bRecency)
}

// SortByRecency orders items newest-first in place using RecencyLess. The
// pathOf and nameOf accessors adapt each caller's own struct to the shared
// (RecencyOf(path), Name) key, so heterogeneous consumers (list's listEntry,
// open/delete's local wtOption) reuse one comparator without converting to a
// common type. The sort is stable so equal-key items keep their input order
// before the Name tie-break decides.
func SortByRecency[T any](items []T, pathOf func(T) string, nameOf func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		return RecencyLess(
			RecencyOf(pathOf(items[i])), nameOf(items[i]),
			RecencyOf(pathOf(items[j])), nameOf(items[j]),
		)
	})
}
