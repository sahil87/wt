package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/spf13/cobra"
	wt "github.com/sahil87/wt/internal/worktree"
)

// ansiPattern matches ANSI escape sequences for display width calculation.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// listEntry holds worktree info for the list command. Dirty and Unpushed are
// pointer types so JSON output can distinguish "status not computed" (nil →
// key omitted via omitempty) from "status computed and clean / 0 unpushed"
// (non-nil → key present with the explicit value).
type listEntry struct {
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	IsMain    bool   `json:"is_main"`
	IsCurrent bool   `json:"is_current"`
	Dirty     *bool  `json:"dirty,omitempty"`
	Unpushed  *int   `json:"unpushed,omitempty"`
}

// maxListConcurrency caps the worker pool for --status enrichment regardless of
// host CPU count. 8 is sufficient for the expected ≤100-worktree scale.
const maxListConcurrency = 8

func listCmd() *cobra.Command {
	var (
		pathName   string
		jsonOut    bool
		statusFlag bool
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

			if jsonOut {
				return handleJSONOutput(entries)
			}

			return handleFormattedOutput(entries, ctx, statusFlag)
		},
	}

	cmd.Flags().StringVar(&pathName, "path", "", "Output just the absolute path for a named worktree")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output worktree data as a JSON array")
	cmd.Flags().BoolVar(&statusFlag, "status", false, "Show dirty/unpushed status for each worktree (slower)")

	return cmd
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

func handleFormattedOutput(entries []listEntry, ctx *wt.RepoContext, showStatus bool) error {
	fmt.Printf("Worktrees for: %s%s%s\n", wt.ColorBold, ctx.RepoName, wt.ColorReset)
	fmt.Printf("Location: %s\n\n", ctx.WorktreesDir)

	type displayRow struct {
		name   string
		branch string
		status string
		path   string
	}
	rows := make([]displayRow, len(entries))
	for i, e := range entries {
		name := e.Name
		if e.IsMain {
			name = wt.ColorBold + "(main)" + wt.ColorReset
		}

		var status string
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

		rows[i] = displayRow{
			name:   name,
			branch: e.Branch,
			status: status,
			path:   relativePath(e.Path, ctx),
		}
	}

	if showStatus {
		headers := [4]string{"Name", "Branch", "Status", "Path"}
		colWidths := [4]int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
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
			statusPad := colWidths[2] - displayWidth(r.status)

			fmt.Printf("%s%s%s  %-*s  %s%s  %s\n",
				marker,
				r.name, strings.Repeat(" ", namePad),
				colWidths[1], r.branch,
				r.status, strings.Repeat(" ", statusPad),
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

// listEntriesEnriched returns worktree entries with dirty/unpushed status
// computed in parallel via a bounded worker pool. Output order matches the
// porcelain ordering.
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
		// Initialize pointer fields so JSON output emits dirty/unpushed keys
		// even when enrichment is skipped (vanished worktree) or all-zero
		// (clean and up-to-date). The contract for --status is "keys present
		// regardless of value".
		zeroDirty := false
		zeroUnpushed := 0
		entries[i].Dirty = &zeroDirty
		entries[i].Unpushed = &zeroUnpushed

		if _, statErr := os.Stat(r.path); statErr != nil {
			continue
		}
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
