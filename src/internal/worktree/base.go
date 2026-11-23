package worktree

import (
	"fmt"
	"os/exec"
	"strings"
)

// Branch-config key suffixes for a branch wt created: `branch.<name>.wt-base`
// holds the base ref as the user typed it (or the HEAD label), and
// `branch.<name>.wt-base-sha` the full SHA it was pinned to.
const (
	baseRefKey = "wt-base"
	baseSHAKey = "wt-base-sha"

	// recordedBasesPattern selects every recorded-base key in one
	// `git config --get-regexp` call.
	recordedBasesPattern = `^branch\..*\.wt-base(-sha)?$`
)

// RecordedBase is the base a wt-created branch was started from.
type RecordedBase struct {
	Ref, SHA string
}

// RecordBaseError is returned when writing a branch's recorded base fails.
type RecordBaseError struct {
	Branch, Output string
}

func (e *RecordBaseError) Error() string {
	return fmt.Sprintf("failed to record base for branch %s: %s", e.Branch, e.Output)
}

// RecordBase writes the recorded base for branch to the repository's own
// config (never global): ref as typed (or the HEAD label) and the pinned sha.
func RecordBase(branch, ref, sha string) error {
	for _, kv := range [][2]string{{baseRefKey, ref}, {baseSHAKey, sha}} {
		cmd := exec.Command("git", "config", "branch."+branch+"."+kv[0], kv[1])
		if out, err := cmd.CombinedOutput(); err != nil {
			return &RecordBaseError{Branch: branch, Output: strings.TrimSpace(string(out))}
		}
	}
	return nil
}

// RecordedBases reads every recorded base in one `git config --get-regexp`
// call, keyed by branch name. Any failure — including exit 1 for no matches —
// yields an empty map: a missing base is never an error.
func RecordedBases() map[string]RecordedBase {
	out, err := exec.Command("git", "config", "-z", "--get-regexp", recordedBasesPattern).Output()
	if err != nil {
		return map[string]RecordedBase{}
	}
	return parseRecordedBases(string(out))
}

// parseRecordedBases parses `git config -z --get-regexp` output: NUL-separated
// "key\nvalue" records. Branch names may contain dots and slashes, so the
// branch is whatever lies between the "branch." prefix and the key suffix.
func parseRecordedBases(out string) map[string]RecordedBase {
	bases := map[string]RecordedBase{}
	for _, rec := range strings.Split(out, "\x00") {
		key, val, _ := strings.Cut(rec, "\n")
		name, ok := strings.CutPrefix(key, "branch.")
		if !ok {
			continue
		}
		if branch, ok := strings.CutSuffix(name, "."+baseSHAKey); ok {
			b := bases[branch]
			b.SHA = val
			bases[branch] = b
		} else if branch, ok := strings.CutSuffix(name, "."+baseRefKey); ok {
			b := bases[branch]
			b.Ref = val
			bases[branch] = b
		}
	}
	return bases
}
