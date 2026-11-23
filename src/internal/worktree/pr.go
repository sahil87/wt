package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Sentinel errors for `wt create --pr`. cmd/create.go maps these (via
// errors.Is) to typed exits: a missing gh is ExitGeneralError, a fork PR is
// ExitInvalidArgs. Any other ResolvePRHead error is a failed lookup
// (ExitGeneralError) carrying gh's own first error line.
var (
	// ErrGhNotFound is returned by ResolvePRHead when the `gh` executable is
	// not on PATH. gh is an optional dependency, needed only for --pr.
	ErrGhNotFound = errors.New("gh not found on PATH")

	// ErrPRCrossRepository is returned (alongside the parsed PRHead) when the
	// PR's head branch lives in another repository (a fork). Its branch is
	// not on origin, and wt does not manage remotes.
	ErrPRCrossRepository = errors.New("pull request head is in another repository")
)

// prLookupTimeout bounds the `gh pr view` call so a hung network or auth
// prompt cannot stall wt create indefinitely.
const prLookupTimeout = 15 * time.Second

// prLookupWaitDelay caps how long Wait lingers after the timeout kill for a
// gh descendant still holding the output pipes, keeping the bound honest.
const prLookupWaitDelay = 2 * time.Second

// PRHead is what `gh pr view` reports about a pull request's head.
type PRHead struct {
	Branch          string // headRefName
	CrossRepository bool   // isCrossRepository
}

// ResolvePRHead resolves pull request number's head branch by running
// `gh pr view <number> --json headRefName,isCrossRepository` (argv exec, in
// the current directory so gh picks the repository from its git remotes).
// A cross-repository head returns the parsed PRHead together with
// ErrPRCrossRepository so the caller can name the branch in its error.
func ResolvePRHead(number string) (PRHead, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return PRHead{}, ErrGhNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), prLookupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", number, "--json", "headRefName,isCrossRepository")
	cmd.WaitDelay = prLookupWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return PRHead{}, fmt.Errorf("gh pr view timed out after %s", prLookupTimeout)
		}
		if line := firstNonEmptyLine(stderr.String()); line != "" {
			return PRHead{}, errors.New(line)
		}
		return PRHead{}, err
	}

	var out struct {
		HeadRefName       string `json:"headRefName"`
		IsCrossRepository bool   `json:"isCrossRepository"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return PRHead{}, fmt.Errorf("unexpected gh output: %v", err)
	}
	if out.HeadRefName == "" {
		return PRHead{}, errors.New("unexpected gh output: no headRefName")
	}

	head := PRHead{Branch: out.HeadRefName, CrossRepository: out.IsCrossRepository}
	if head.CrossRepository {
		return head, ErrPRCrossRepository
	}
	return head, nil
}

// firstNonEmptyLine returns the first line of s with content, trimmed.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
