package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
)

// ansiPattern matches ANSI escape sequences for display width calculation.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// listEntry holds worktree info for the list command. Dirty, Unpushed, and
// LastActive are pointer types so JSON output can distinguish "status not
// computed" (nil → key omitted via omitempty) from "status computed" (non-nil →
// key present with the explicit value, including zero/clean values).
type listEntry struct {
	Name       string     `json:"name"`
	Branch     string     `json:"branch"`
	Path       string     `json:"path"`
	IsMain     bool       `json:"is_main"`
	IsCurrent  bool       `json:"is_current"`
	Dirty      *bool      `json:"dirty,omitempty"`
	Unpushed   *int       `json:"unpushed,omitempty"`
	LastActive *time.Time `json:"last_active,omitempty"`
	Idle       *bool      `json:"idle,omitempty"`
}

// maxListConcurrency caps the worker pool for --status enrichment regardless of
// host CPU count. 8 is sufficient for the expected ≤100-worktree scale.
const maxListConcurrency = 8

func listCmd() *cobra.Command {
	var (
		pathName       string
		jsonOut        bool
		statusFlag     bool
		sortFlag       string
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all git worktrees",
		Long: `List all git worktrees for the current repository.

The current worktree is marked with a green asterisk (*).

By default, output shows Name, Branch, and Path only — no per-worktree git
invocations occur, so the command is O(1) regardless of worktree count. Pass
--status to enable dirty/unpushed enrichment; this is the slower mode because
it forks 2 git subprocesses per worktree (parallelized).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if pathName != "" && jsonOut {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--path and --json are mutually exclusive",
					"Use one output mode at a time",
					"Run 'wt list --help' for usage information")
			}
			if pathName != "" && statusFlag {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--path and --status are mutually exclusive",
					"--path skips enrichment; --status forces it",
					"Run 'wt list --help' for usage information")
			}
			if pathName != "" && sortFlag != "" {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--path and --sort are mutually exclusive",
					"--path is a single-worktree lookup; ordering is meaningless",
					"Run 'wt list --help' for usage information")
			}
			if sortFlag != "" && !isValidSort(sortFlag) {
				wt.ExitWithError(wt.ExitInvalidArgs,
					fmt.Sprintf("Invalid --sort value: %q", sortFlag),
					"--sort accepts: recent, name, branch",
					"Run 'wt list --help' for usage information")
			}

			if err := wt.ValidateGitRepo(); err != nil {
				wt.ExitWithError(wt.ExitGitError,
					"Not a git repository",
					"This command requires a git repository",
					"Navigate to a git repository and try again")
			}

			ctx, err := wt.GetRepoContext()
			if err != nil {
				wt.ExitWithError(wt.ExitGeneralError, "Cannot get repo context", err.Error(), "")
			}

			if pathName != "" {
				return handlePathLookup(pathName, ctx)
			}

			var entries []listEntry
			if statusFlag {
				entries, err = listEntriesEnriched(ctx)
			} else {
				entries, err = listEntriesBasic(ctx)
			}
			if err != nil {
				wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
			}

			// Ordering is a deterministic post-step applied to the final slice,
			// independent of enrichment (which preserves porcelain order via
			// indexed writes). Default order is audience-split by flag: recent
			// for human output, stable name order for --json/--non-interactive;
			// an explicit --sort overrides in any mode. The main worktree is
			// always pinned to the first row.
			//
			// persistKey is set on the human path only (!jsonOut): recent mode
			// writes the computed recency key back into entries[i].LastActive so
			// the renderer can display it without a second stat. The JSON path
			// passes false, leaving LastActive nil so omitempty keeps last_active
			// out of --json --sort=recent (Constitution VI machine-output contract).
			mode := resolveSort(sortFlag, jsonOut, nonInteractive)
			sortEntries(entries, mode, !jsonOut)

			// Derive the idle marker from the recency key already persisted into
			// LastActive (by --status enrichment or recent-mode sortEntries) — no
			// new os.Stat, no git subprocess. The Idle pointer is set non-nil
			// exactly when LastActive is non-nil, so JSON emits "idle" only when
			// last_active is present (i.e. under --status); the main worktree is
			// never idle.
			populateIdle(entries, time.Now())

			if jsonOut {
				return handleJSONOutput(entries)
			}

			return handleFormattedOutput(entries, ctx, statusFlag, mode)
		},
	}

	cmd.Flags().StringVar(&pathName, "path", "", "Output just the absolute path for a named worktree")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output worktree data as a JSON array")
	cmd.Flags().BoolVar(&statusFlag, "status", false, "Show dirty/unpushed status for each worktree (slower)")
	cmd.Flags().StringVar(&sortFlag, "sort", "", "Order non-main worktrees by: recent, name, or branch")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Use stable (name) default ordering for scripts")

	return cmd
}

// sortMode is the resolved ordering applied to non-main list entries.
type sortMode int

const (
	sortRecent sortMode = iota
	sortName
	sortBranch
)

// isValidSort reports whether s is an accepted --sort value.
func isValidSort(s string) bool {
	switch s {
	case "recent", "name", "branch":
		return true
	}
	return false
}

// resolveSort maps the --sort flag and output-mode flags to the effective
// ordering. An explicit --sort always wins. Otherwise the default is recent for
// human output and stable name order whenever --json or --non-interactive is
// set, preserving deterministic machine-readable output (Constitution VI).
func resolveSort(sortFlag string, jsonOut, nonInteractive bool) sortMode {
	switch sortFlag {
	case "recent":
		return sortRecent
	case "name":
		return sortName
	case "branch":
		return sortBranch
	}
	if jsonOut || nonInteractive {
		return sortName
	}
	return sortRecent
}

// sortEntries reorders entries in place per mode, pinning the main worktree to
// the first row and reordering only the non-main entries below it. The main
// worktree is the porcelain-first entry (IsMain); its position is stable across
// all sort modes per the recency-ordering contract.
//
// persistKey, when true (the human-output path, !jsonOut), causes recent mode
// to write the recency key it already computes back into each entry's nil
// LastActive so the renderer can display it without a second os.Stat. It is
// false on the JSON path, where leaving LastActive nil keeps last_active out of
// the output via omitempty. A non-nil LastActive (the --status path) is always
// left untouched.
func sortEntries(entries []listEntry, mode sortMode, persistKey bool) {
	// Partition out the main worktree (always first per porcelain convention)
	// so only non-main entries are reordered.
	start := 0
	if len(entries) > 0 && entries[0].IsMain {
		start = 1
	}
	rest := entries[start:]

	switch mode {
	case sortName:
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].Name < rest[j].Name })
	case sortBranch:
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].Branch < rest[j].Branch })
	case sortRecent:
		// Prefer the already-computed LastActive (set under --status) as the
		// recency key over a fresh os.Stat: this keeps the sort key consistent
		// with the displayed value (no TOCTOU) and avoids redundant stats on the
		// --status path. In default/basic mode LastActive is nil, so we fall back
		// to RecencyOf(e.Path) — the non-status path still stats, but exactly
		// once per entry here, before sorting. Computing the key inside the
		// comparator would re-stat each path O(log n) times and let the
		// comparator observe a changing mtime mid-sort.
		keys := make([]time.Time, len(rest))
		for i, e := range rest {
			if e.LastActive != nil {
				keys[i] = *e.LastActive
			} else {
				keys[i] = wt.RecencyOf(e.Path)
			}
		}
		order := make([]int, len(rest))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			a, b := order[i], order[j]
			return wt.RecencyLess(keys[a], rest[a].Name, keys[b], rest[b].Name)
		})
		sorted := make([]listEntry, len(rest))
		for i, idx := range order {
			sorted[i] = rest[idx]
		}
		copy(rest, sorted)

		// On the human path, persist the recency key we already computed into
		// each non-main entry's LastActive (only when nil — the --status path's
		// non-nil value is the source of truth and must not be clobbered). This
		// reuses the stat already paid for the sort key: no second os.Stat, no
		// git subprocess. The keys[] slice is indexed by the pre-sort position,
		// so write back through the permutation.
		if persistKey {
			for i, idx := range order {
				if rest[i].LastActive == nil {
					k := keys[idx]
					rest[i].LastActive = &k
				}
			}
			// The pinned main worktree is partitioned out of rest above and is
			// never stat'd in basic mode, so its LastActive would render "-".
			// Populate it via a single RecencyOf when nil (one stat for main,
			// no git subprocess); a non-nil --status value is left as-is.
			if start == 1 && entries[0].LastActive == nil {
				k := wt.RecencyOf(entries[0].Path)
				entries[0].LastActive = &k
			}
		}
	}
}

// populateIdle sets the Idle pointer on each entry whose LastActive is non-nil,
// reusing the already-computed recency value as the idleness input (no new
// os.Stat, no git subprocess). Idle is set non-nil exactly when LastActive is
// non-nil, so JSON output emits the "idle" key only when "last_active" is also
// present (i.e. under --status). The main worktree is never marked idle: its
// Idle is forced to false whenever the field is present. Idleness is evaluated
// against the built-in DefaultIdleThreshold; wt list has no per-invocation
// override (that lives on wt delete --stale).
func populateIdle(entries []listEntry, now time.Time) {
	for i := range entries {
		if entries[i].LastActive == nil {
			continue
		}
		idle := !entries[i].IsMain && wt.IsIdle(*entries[i].LastActive, now, wt.DefaultIdleThreshold)
		entries[i].Idle = &idle
	}
}

func handlePathLookup(name string, ctx *wt.RepoContext) error {
	entries, err := listWorktreeEntries()
	if err != nil {
		wt.ExitWithError(wt.ExitGitError, "Cannot list worktrees", err.Error(), "")
	}

	for _, e := range entries {
		entryName := filepath.Base(e.path)
		if e.path == ctx.RepoRoot {
			entryName = "main"
		}
		if strings.EqualFold(entryName, name) {
			fmt.Println(e.path)
			return nil
		}
	}

	fmt.Fprintf(os.Stderr, "Worktree '%s' not found. Use 'wt list' to see available worktrees.\n", name)
	os.Exit(wt.ExitGeneralError)
	return nil
}

func handleJSONOutput(entries []listEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON encoding: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// displayWidth returns the visible width of s, excluding ANSI escape sequences.
// Uses RuneCountInString to correctly count multi-byte characters (e.g. "↑").
func displayWidth(s string) int {
	return utf8.RuneCountInString(ansiPattern.ReplaceAllString(s, ""))
}

// relativeTime renders a worktree's last-active time as a coarse, human-
// friendly relative string (e.g. "just now", "2h ago", "3d ago"). A zero time
// (vanished worktree, never stat'd) renders as "-" so the column stays aligned
// without implying a real timestamp. JSON output never uses this — it emits the
// raw RFC3339 timestamp via the *time.Time field.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		// Clock skew or a future mtime; treat as current.
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// relativePath computes a short relative path for display.
// Main worktree: "{repoName}/"
// Other worktrees: "{repoName}.worktrees/{wtName}/"
func relativePath(entryPath string, ctx *wt.RepoContext) string {
	parent := filepath.Dir(ctx.WorktreesDir)
	rel, err := filepath.Rel(parent, entryPath)
	if err != nil {
		return entryPath
	}
	return rel + "/"
}

func handleFormattedOutput(entries []listEntry, ctx *wt.RepoContext, showStatus bool, mode sortMode) error {
	fmt.Printf("Worktrees for: %s%s%s\n", wt.ColorBold, ctx.RepoName, wt.ColorReset)
	fmt.Printf("Location: %s\n\n", ctx.WorktreesDir)

	// recentLayout selects the 4-column Name/Branch/Last Active/Path table for the
	// recency-ordered human view. --status keeps its own 5-column layout; name/
	// branch modes stay 3-column. Layout is keyed on the resolved sort mode, not
	// on showStatus alone.
	recentLayout := mode == sortRecent && !showStatus

	type displayRow struct {
		name       string
		branch     string
		status     string
		lastActive string
		path       string
	}
	rows := make([]displayRow, len(entries))
	for i, e := range entries {
		name := e.Name
		if e.IsMain {
			name = wt.ColorBold + "(main)" + wt.ColorReset
		}

		var status, lastActive string
		if showStatus {
			var dirtyMarker, unpushedMarker string
			if e.Dirty != nil && *e.Dirty {
				dirtyMarker = wt.ColorYellow + "*" + wt.ColorReset
			}
			if e.Unpushed != nil && *e.Unpushed > 0 {
				unpushedMarker = wt.ColorYellow + "↑" + strconv.Itoa(*e.Unpushed) + wt.ColorReset
			}
			switch {
			case dirtyMarker != "" && unpushedMarker != "":
				status = dirtyMarker + " " + unpushedMarker
			case dirtyMarker != "":
				status = dirtyMarker
			case unpushedMarker != "":
				status = unpushedMarker
			}
		}
		// Last Active is rendered under --status (5-column) and in recent human
		// mode (4-column). sortEntries persisted the recency key into LastActive
		// on the human path, so this reuses it — no second os.Stat. A nil pointer
		// falls back to the zero time, which relativeTime renders as "-". An idle
		// non-main worktree gets a trailing " ⚠ idle" marker on the same cell,
		// reusing the already-computed Idle flag (no new stat). The main worktree
		// is never marked (its Idle is false when set).
		if showStatus || recentLayout {
			var t time.Time
			if e.LastActive != nil {
				t = *e.LastActive
			}
			lastActive = relativeTime(t)
			if e.Idle != nil && *e.Idle {
				lastActive += " " + wt.ColorYellow + "⚠ idle" + wt.ColorReset
			}
		}

		rows[i] = displayRow{
			name:       name,
			branch:     e.Branch,
			status:     status,
			lastActive: lastActive,
			path:       relativePath(e.Path, ctx),
		}
	}

	if showStatus {
		headers := [5]string{"Name", "Branch", "Status", "Last Active", "Path"}
		colWidths := [5]int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3]), len(headers[4])}
		for _, r := range rows {
			if w := displayWidth(r.name); w > colWidths[0] {
				colWidths[0] = w
			}
			if w := displayWidth(r.branch); w > colWidths[1] {
				colWidths[1] = w
			}
			if w := displayWidth(r.status); w > colWidths[2] {
				colWidths[2] = w
			}
			if w := displayWidth(r.lastActive); w > colWidths[3] {
				colWidths[3] = w
			}
			if w := displayWidth(r.path); w > colWidths[4] {
				colWidths[4] = w
			}
		}

		fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n",
			colWidths[0], headers[0],
			colWidths[1], headers[1],
			colWidths[2], headers[2],
			colWidths[3], headers[3],
			headers[4])

		for i, r := range rows {
			marker := "  "
			if entries[i].IsCurrent {
				marker = wt.ColorGreen + "*" + wt.ColorReset + " "
			}
			namePad := colWidths[0] - displayWidth(r.name)
			statusPad := colWidths[2] - displayWidth(r.status)
			// lastActive may carry ANSI codes and the multi-byte "⚠" marker, so
			// pad by display width like Status above — %-*s pads by byte count and
			// would leave the Path column ragged when the idle marker is present.
			lastActivePad := colWidths[3] - displayWidth(r.lastActive)

			fmt.Printf("%s%s%s  %-*s  %s%s  %s%s  %s\n",
				marker,
				r.name, strings.Repeat(" ", namePad),
				colWidths[1], r.branch,
				r.status, strings.Repeat(" ", statusPad),
				r.lastActive, strings.Repeat(" ", lastActivePad),
				r.path)
		}
	} else if recentLayout {
		headers := [4]string{"Name", "Branch", "Last Active", "Path"}
		colWidths := [4]int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
		for _, r := range rows {
			if w := displayWidth(r.name); w > colWidths[0] {
				colWidths[0] = w
			}
			if w := displayWidth(r.branch); w > colWidths[1] {
				colWidths[1] = w
			}
			if w := displayWidth(r.lastActive); w > colWidths[2] {
				colWidths[2] = w
			}
			if w := displayWidth(r.path); w > colWidths[3] {
				colWidths[3] = w
			}
		}

		fmt.Printf("  %-*s  %-*s  %-*s  %s\n",
			colWidths[0], headers[0],
			colWidths[1], headers[1],
			colWidths[2], headers[2],
			headers[3])

		for i, r := range rows {
			marker := "  "
			if entries[i].IsCurrent {
				marker = wt.ColorGreen + "*" + wt.ColorReset + " "
			}
			namePad := colWidths[0] - displayWidth(r.name)
			// lastActive may carry ANSI codes and the multi-byte "⚠" marker, so
			// pad by display width — %-*s pads by byte count and would leave the
			// Path column ragged when the idle marker is present.
			lastActivePad := colWidths[2] - displayWidth(r.lastActive)

			fmt.Printf("%s%s%s  %-*s  %s%s  %s\n",
				marker,
				r.name, strings.Repeat(" ", namePad),
				colWidths[1], r.branch,
				r.lastActive, strings.Repeat(" ", lastActivePad),
				r.path)
		}
	} else {
		headers := [3]string{"Name", "Branch", "Path"}
		colWidths := [3]int{len(headers[0]), len(headers[1]), len(headers[2])}
		for _, r := range rows {
			if w := displayWidth(r.name); w > colWidths[0] {
				colWidths[0] = w
			}
			if w := displayWidth(r.branch); w > colWidths[1] {
				colWidths[1] = w
			}
			if w := displayWidth(r.path); w > colWidths[2] {
				colWidths[2] = w
			}
		}

		fmt.Printf("  %-*s  %-*s  %s\n",
			colWidths[0], headers[0],
			colWidths[1], headers[1],
			headers[2])

		for i, r := range rows {
			marker := "  "
			if entries[i].IsCurrent {
				marker = wt.ColorGreen + "*" + wt.ColorReset + " "
			}
			namePad := colWidths[0] - displayWidth(r.name)

			fmt.Printf("%s%s%s  %-*s  %s\n",
				marker,
				r.name, strings.Repeat(" ", namePad),
				colWidths[1], r.branch,
				r.path)
		}
	}

	fmt.Printf("\nTotal: %d worktree(s)\n", len(entries))
	return nil
}

type rawEntry struct {
	path   string
	branch string
}

func listWorktreeEntries() ([]rawEntry, error) {
	out, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var entries []rawEntry
	var current rawEntry

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.path != "" {
				entries = append(entries, current)
			}
			current = rawEntry{path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			current.branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "detached" {
			current.branch = "(detached)"
		}
	}
	if current.path != "" {
		entries = append(entries, current)
	}
	return entries, nil
}

// buildBaseEntry populates the cheap fields of a listEntry (name, branch, path,
// is_main, is_current). It does NOT spawn any per-worktree git subprocesses.
func buildBaseEntry(r rawEntry, mainPath, currentDir string) listEntry {
	e := listEntry{
		Path:   r.path,
		Branch: r.branch,
		IsMain: r.path == mainPath,
	}
	if e.IsMain {
		e.Name = "main"
	} else {
		e.Name = filepath.Base(r.path)
	}
	resolvedPath, _ := filepath.EvalSymlinks(r.path)
	if resolvedPath == currentDir || strings.HasPrefix(currentDir, resolvedPath+string(filepath.Separator)) {
		e.IsCurrent = true
	}
	return e
}

// listEntriesBasic returns worktree entries without dirty/unpushed enrichment.
// One git subprocess (`git worktree list --porcelain`) regardless of count.
func listEntriesBasic(ctx *wt.RepoContext) ([]listEntry, error) {
	raw, err := listWorktreeEntries()
	if err != nil {
		return nil, err
	}

	currentDir, _ := os.Getwd()
	currentDir, _ = filepath.EvalSymlinks(currentDir)

	var mainPath string
	if len(raw) > 0 {
		mainPath = raw[0].path
	}

	entries := make([]listEntry, len(raw))
	for i, r := range raw {
		entries[i] = buildBaseEntry(r, mainPath, currentDir)
	}
	return entries, nil
}

// listEntriesEnriched returns worktree entries with dirty/unpushed/last_active
// status computed in parallel via a bounded worker pool. The slice is returned
// in porcelain order; the worker pool preserves that order via indexed writes.
// Final display ordering is applied separately by sortEntries in listCmd.
func listEntriesEnriched(ctx *wt.RepoContext) ([]listEntry, error) {
	raw, err := listWorktreeEntries()
	if err != nil {
		return nil, err
	}

	currentDir, _ := os.Getwd()
	currentDir, _ = filepath.EvalSymlinks(currentDir)

	var mainPath string
	if len(raw) > 0 {
		mainPath = raw[0].path
	}

	entries := make([]listEntry, len(raw))
	for i, r := range raw {
		entries[i] = buildBaseEntry(r, mainPath, currentDir)
	}

	concurrency := runtime.NumCPU()
	if concurrency > maxListConcurrency {
		concurrency = maxListConcurrency
	}
	if concurrency < 1 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, r := range raw {
		// Initialize pointer fields so JSON output emits dirty/unpushed/
		// last_active keys even when enrichment is skipped (vanished worktree)
		// or all-zero (clean and up-to-date). The contract for --status is
		// "keys present regardless of value"; a vanished worktree keeps the
		// zero time.Time as its last_active.
		zeroDirty := false
		zeroUnpushed := 0
		var zeroActive time.Time
		entries[i].Dirty = &zeroDirty
		entries[i].Unpushed = &zeroUnpushed
		entries[i].LastActive = &zeroActive

		info, statErr := os.Stat(r.path)
		if statErr != nil {
			continue
		}
		// Reuse the gate's stat result for recency — no extra os.Stat and never
		// a git subprocess (preserves the single-subprocess list contract).
		*entries[i].LastActive = info.ModTime()
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r rawEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			dirty := checkDirty(r.path)
			*entries[i].Dirty = dirty
			if r.branch != "(detached)" {
				*entries[i].Unpushed = getUnpushedInDir(r.path)
			}
		}(i, r)
	}
	wg.Wait()

	return entries, nil
}

// checkDirty reports whether the worktree at wtPath has any staged, unstaged,
// or untracked changes. A single `git status --porcelain` captures all three.
// Non-zero exit (e.g., corrupted index) is treated as clean — failure modes
// are non-actionable for a list command.
func checkDirty(wtPath string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// getUnpushedInDir returns the number of commits on HEAD ahead of its upstream.
// `@{u}` resolves the upstream inline; if no upstream is configured the command
// exits non-zero and we return 0.
func getUnpushedInDir(wtPath string) int {
	cmd := exec.Command("git", "rev-list", "--count", "@{u}..HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}
