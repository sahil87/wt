package worktree

import (
	"fmt"
	"os/exec"
	"strings"
)

// StashCreate creates a stash commit using the hash-based approach (git stash create)
// for concurrency safety. It adds all files, creates the stash, stores it in the reflog,
// then resets and cleans. Returns the stash hash (empty string if no changes).
//
// Errors from `git stash create`, `git stash store`, `git reset --hard`, and
// `git clean -fd` are propagated to the caller. The destructive `reset` and
// `clean` steps are only executed after a successful `stash store`, so callers
// cannot end up with a wiped working tree and no recoverable stash.
func StashCreate(msg string) (string, error) {
	// Stage all files
	if err := exec.Command("git", "add", "-A").Run(); err != nil {
		// Not fatal — might just have nothing to add
	}

	// Create a stash commit (does not modify ref). A non-zero exit here
	// indicates a real git failure (not a repo, index corruption, permission
	// issues) — propagate it rather than silently treating as "no changes".
	out, err := exec.Command("git", "stash", "create", msg).Output()
	if err != nil {
		return "", fmt.Errorf("git stash create: %w", err)
	}

	hash := strings.TrimSpace(string(out))
	if hash == "" {
		return "", nil // No changes to stash
	}

	// Store the stash in the reflog for recovery. Must succeed before we
	// destructively reset/clean — otherwise the stash hash is orphaned and
	// the working tree gets wiped with no recovery path.
	if err := exec.Command("git", "stash", "store", hash, "-m", msg).Run(); err != nil {
		return "", fmt.Errorf("git stash store: %w", err)
	}

	// Reset and clean — only after stash store succeeded.
	if err := exec.Command("git", "reset", "--hard", "HEAD").Run(); err != nil {
		return "", fmt.Errorf("git reset --hard: %w", err)
	}
	if err := exec.Command("git", "clean", "-fd").Run(); err != nil {
		return "", fmt.Errorf("git clean -fd: %w", err)
	}

	return hash, nil
}

// StashApply applies a stash by hash. No-op if hash is empty.
func StashApply(hash string) error {
	if hash == "" {
		return nil
	}
	cmd := exec.Command("git", "stash", "apply", hash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &StashApplyError{Hash: hash, Output: strings.TrimSpace(string(out))}
	}
	return nil
}

// StashApplyError represents a failed stash apply.
type StashApplyError struct {
	Hash   string
	Output string
}

func (e *StashApplyError) Error() string {
	return "git stash apply " + e.Hash + ": " + e.Output
}
