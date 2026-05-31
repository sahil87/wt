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
//
// Each item's recency is stat'd exactly once, before sorting. Computing
// RecencyOf inside the comparator would re-stat the same path O(log n) times
// and — worse — let the comparator observe a path's mtime changing mid-sort,
// breaking the total-order invariant sort.SliceStable relies on. We sort an
// index permutation (whose comparator reads the precomputed keys) and then
// apply that permutation to items, so each item keeps its own key throughout.
func SortByRecency[T any](items []T, pathOf func(T) string, nameOf func(T) string) {
	keys := make([]time.Time, len(items))
	for i := range items {
		keys[i] = RecencyOf(pathOf(items[i]))
	}
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		return RecencyLess(keys[a], nameOf(items[a]), keys[b], nameOf(items[b]))
	})
	sorted := make([]T, len(items))
	for i, idx := range order {
		sorted[i] = items[idx]
	}
	copy(items, sorted)
}
