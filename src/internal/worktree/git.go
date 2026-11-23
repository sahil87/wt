package worktree

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// shortSHALen is the length of the abbreviated commit SHA shown in summaries.
const shortSHALen = 7

// ResolveCommit resolves ref (branch, tag, or commit SHA) to the full SHA of
// the commit it points at, via `git rev-parse --verify --quiet <ref>^{commit}`.
// Returns an error when ref does not resolve to a commit.
func ResolveCommit(ref string) (string, error) {
	out, err := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}").Output()
	if err != nil {
		return "", fmt.Errorf("'%s' does not resolve to a commit", ref)
	}
	return strings.TrimSpace(string(out)), nil
}

// ShortSHA abbreviates a full commit SHA to the 7 characters used in summaries.
func ShortSHA(sha string) string {
	if len(sha) > shortSHALen {
		return sha[:shortSHALen]
	}
	return sha
}

// HasUncommittedChanges returns true if the current directory has staged or unstaged changes.
func HasUncommittedChanges() bool {
	if err := exec.Command("git", "diff", "--quiet").Run(); err != nil {
		return true
	}
	if err := exec.Command("git", "diff", "--cached", "--quiet").Run(); err != nil {
		return true
	}
	return false
}

// HasUntrackedFiles returns true if there are untracked files in the current directory.
func HasUntrackedFiles() bool {
	out, err := exec.Command("git", "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// HasUnpushedCommits returns true if the branch has unpushed commits.
func HasUnpushedCommits(branch string) bool {
	upstream, err := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{upstream}").Output()
	if err != nil {
		return false
	}
	up := strings.TrimSpace(string(upstream))
	out, err := exec.Command("git", "log", up+".."+branch, "--oneline").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// GetUnpushedCount returns the number of unpushed commits for the branch.
func GetUnpushedCount(branch string) int {
	upstream, err := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{upstream}").Output()
	if err != nil {
		return 0
	}
	up := strings.TrimSpace(string(upstream))
	out, err := exec.Command("git", "rev-list", "--count", up+".."+branch).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// BranchExistsLocally returns true if the branch exists as a local ref.
func BranchExistsLocally(branch string) bool {
	return exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// BranchExistsRemotely returns true if the branch exists on origin.
func BranchExistsRemotely(branch string) bool {
	out, err := exec.Command("git", "ls-remote", "--heads", "origin", branch).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), branch)
}

// FetchRemoteBranch fetches a branch from origin into a local branch.
func FetchRemoteBranch(branch string) error {
	cmd := exec.Command("git", "fetch", "origin", branch+":"+branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch origin %s failed: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoteForRef returns the remote a base ref should be fetched from: the first
// "/"-separated segment of ref when (and only when) that segment is a configured
// remote, otherwise "origin". A branch such as "feature/x" with no "feature"
// remote therefore maps to origin, as does an empty ref or a bare SHA.
func RemoteForRef(ref string) string {
	const defaultRemote = "origin"
	prefix, _, found := strings.Cut(ref, "/")
	if !found || prefix == "" {
		return defaultRemote
	}
	out, err := exec.Command("git", "remote").Output()
	if err != nil {
		return defaultRemote
	}
	for _, r := range strings.Fields(string(out)) {
		if r == prefix {
			return prefix
		}
	}
	return defaultRemote
}

// FetchRemote runs `git fetch <remote>`. Output is captured (never streamed to
// stdout); on failure the returned error wraps git's trimmed output.
func FetchRemote(remote string) error {
	cmd := exec.Command("git", "fetch", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// UpstreamForBase returns the ref a new branch cut from the typed base should
// track, or "" for none. It mirrors what `git worktree add -b <branch> <path>
// <base>` does via branch.autoSetupMerge when given a ref name (wt passes a
// pinned SHA, which git never auto-tracks). It is a pure lookup — callers
// resolve it before creating anything and apply it with SetUpstream afterwards.
//
//   - unset / true: a remote-tracking base (refs/remotes/...) is tracked.
//   - always: a local branch (refs/heads/...) is tracked too.
//   - simple: a remote-tracking base only when its branch name equals the new
//     branch's.
//   - inherit: copy the base branch's own upstream when it has one, else none.
//   - false: never.
//
// A SHA, tag, HEAD or empty base never tracks.
func UpstreamForBase(base, branch string) string {
	if base == "" || base == "HEAD" {
		return ""
	}
	out, err := exec.Command("git", "rev-parse", "--symbolic-full-name", base).Output()
	if err != nil {
		return ""
	}
	full := strings.TrimSpace(string(out))
	remote := strings.HasPrefix(full, "refs/remotes/") && remoteTrackingRefMapped(full)
	local := strings.HasPrefix(full, "refs/heads/")
	if !remote && !local {
		return ""
	}
	cfg, _ := exec.Command("git", "config", "--get", "branch.autoSetupMerge").Output()
	switch strings.ToLower(strings.TrimSpace(string(cfg))) {
	case "false", "no", "off", "0":
		return ""
	case "always":
		return base
	case "simple":
		if remote {
			rest := strings.TrimPrefix(full, "refs/remotes/")
			if _, name, ok := strings.Cut(rest, "/"); ok && name == branch {
				return base
			}
		}
		return ""
	case "inherit":
		if !local {
			return ""
		}
		up, err := exec.Command("git", "rev-parse", "--abbrev-ref", base+"@{upstream}").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(up))
	default:
		if remote {
			return base
		}
		return ""
	}
}

// remoteTrackingRefMapped reports whether some configured remote's fetch
// refspec maps onto the full remote-tracking ref. git only auto-tracks (and
// `--set-upstream-to` only accepts) a remote-tracking ref a remote would
// produce; a stray refs/remotes/ghost/main with no covering refspec is not a
// tracking candidate.
func remoteTrackingRefMapped(fullRef string) bool {
	out, err := exec.Command("git", "remote").Output()
	if err != nil {
		return false
	}
	for _, r := range strings.Fields(string(out)) {
		specs, err := exec.Command("git", "config", "--get-all", "remote."+r+".fetch").Output()
		if err != nil {
			continue
		}
		for _, spec := range strings.Split(string(specs), "\n") {
			if refspecDstMatches(strings.TrimSpace(spec), fullRef) {
				return true
			}
		}
	}
	return false
}

// refspecDstMatches reports whether the destination side of a fetch refspec
// ("[+]src:dst", dst holding at most one "*") matches ref.
func refspecDstMatches(spec, ref string) bool {
	spec = strings.TrimPrefix(spec, "+")
	_, dst, ok := strings.Cut(spec, ":")
	if !ok || dst == "" {
		return false
	}
	prefix, suffix, glob := strings.Cut(dst, "*")
	if !glob {
		return dst == ref
	}
	return len(ref) >= len(prefix)+len(suffix) &&
		strings.HasPrefix(ref, prefix) && strings.HasSuffix(ref, suffix)
}

// SetUpstreamError is returned when `git branch --set-upstream-to` fails.
type SetUpstreamError struct {
	Branch, Upstream, Output string
}

func (e *SetUpstreamError) Error() string {
	return fmt.Sprintf("git branch --set-upstream-to=%s %s failed: %s", e.Upstream, e.Branch, e.Output)
}

// SetUpstream runs `git branch --set-upstream-to=<upstream> <branch>`.
func SetUpstream(branch, upstream string) error {
	cmd := exec.Command("git", "branch", "--set-upstream-to="+upstream, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &SetUpstreamError{Branch: branch, Upstream: upstream, Output: strings.TrimSpace(string(out))}
	}
	return nil
}

// DeleteLocalBranch deletes a local branch. If force is true, uses -D instead of -d.
func DeleteLocalBranch(branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.Command("git", "branch", flag, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s %s: %s", flag, branch, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteRemoteBranch deletes a branch on origin.
func DeleteRemoteBranch(branch string) error {
	cmd := exec.Command("git", "push", "origin", "--delete", branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push origin --delete %s: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}

// GetUnpushedCommitLines returns the oneline log of unpushed commits for display.
func GetUnpushedCommitLines(branch string, limit int) []string {
	upstream, err := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{upstream}").Output()
	if err != nil {
		return nil
	}
	up := strings.TrimSpace(string(upstream))
	out, err := exec.Command("git", "log", up+".."+branch, "--oneline").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if limit > 0 && len(lines) > limit {
		return lines[:limit]
	}
	return lines
}
