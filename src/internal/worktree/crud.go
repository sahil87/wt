package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsureWorktreesDir creates the worktrees directory if it doesn't exist.
func EnsureWorktreesDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create worktrees directory at %s: %w", dir, err)
		}
	}
	return nil
}

// CheckNameCollision returns true if a worktree with the given name already exists.
func CheckNameCollision(worktreesDir, name string) bool {
	path := filepath.Join(worktreesDir, name)
	_, err := os.Stat(path)
	return err == nil
}

// CreateWorktree creates a git worktree at the given path for the given branch.
// If newBranch is true, it creates a new branch with -b flag.
// When startPoint is non-empty and newBranch is true, the branch is created from the given start-point.
func CreateWorktree(path, branch string, newBranch bool, startPoint string) error {
	var cmd *exec.Cmd
	if newBranch {
		if startPoint != "" {
			cmd = exec.Command("git", "worktree", "add", "-b", branch, path, startPoint)
		} else {
			cmd = exec.Command("git", "worktree", "add", "-b", branch, path)
		}
	} else {
		cmd = exec.Command("git", "worktree", "add", path, branch)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add failed for branch '%s' at '%s': %s",
			branch, path, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveWorktree removes a git worktree at the given path, then prunes.
// If force is true, uses --force flag.
func RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %s", strings.TrimSpace(string(out)))
	}

	// Prune stale worktree metadata
	exec.Command("git", "worktree", "prune").Run()
	return nil
}

// CreateBranchWorktree creates a worktree for a specified branch (local, remote, or new).
// startPoint is only used when creating a new branch; it is ignored for existing local/remote branches.
// Returns the worktree path.
func CreateBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint string) (string, error) {
	if err := EnsureWorktreesDir(ctx.WorktreesDir); err != nil {
		return "", err
	}

	wtPath := filepath.Join(ctx.WorktreesDir, name)

	if BranchExistsLocally(branch) {
		if err := CreateWorktree(wtPath, branch, false, ""); err != nil {
			return "", err
		}
		rb.Register("git", "worktree", "remove", "--force", wtPath)
	} else if BranchExistsRemotely(branch) {
		if err := FetchRemoteBranch(branch); err != nil {
			return "", err
		}
		if err := CreateWorktree(wtPath, branch, false, ""); err != nil {
			return "", err
		}
		rb.Register("git", "worktree", "remove", "--force", wtPath)
	} else {
		if err := CreateWorktree(wtPath, branch, true, startPoint); err != nil {
			return "", err
		}
		rb.Register("git", "worktree", "remove", "--force", wtPath)
		rb.Register("git", "branch", "-D", branch)
	}

	return wtPath, nil
}

// CreateExploratoryWorktree creates an exploratory worktree with a new branch matching the name.
// When startPoint is non-empty, the new branch is created from the given start-point.
// Returns the worktree path.
func CreateExploratoryWorktree(name string, ctx *RepoContext, rb *Rollback, startPoint string) (string, error) {
	if err := EnsureWorktreesDir(ctx.WorktreesDir); err != nil {
		return "", err
	}

	wtPath := filepath.Join(ctx.WorktreesDir, name)
	branch := name

	if err := CreateWorktree(wtPath, branch, true, startPoint); err != nil {
		return "", err
	}
	rb.Register("git", "worktree", "remove", "--force", wtPath)
	rb.Register("git", "branch", "-D", branch)

	return wtPath, nil
}

// RunWorktreeSetup resolves the init script via ResolveInitInvocation and
// runs it in the worktree directory.
//   - On structured not-found, prints the unified warning to stderr and
//     returns nil (init step is treated as a no-op).
//   - Returns the exec error verbatim on init-script non-zero exit so callers
//     can extract *exec.ExitError via errors.As.
//
// Callers that want to confirm with the user before running MUST call
// ConfirmYesNo themselves first — confirmation is no longer part of the
// runner so wt create's SIGINT-during-init handler can be installed AFTER
// the prompt completes (installing it before would consume Ctrl-C during
// the prompt with no init child to target).
func RunWorktreeSetup(wtPath, initScript, repoRoot string) error {
	return RunWorktreeSetupWithObserver(wtPath, initScript, repoRoot, nil)
}

// RunWorktreeSetupWithObserver is like RunWorktreeSetup but invokes observer
// with the resolved *exec.Cmd immediately before cmd.Run(). The observer
// lets wt create's SIGINT-during-init handler capture a reference to the
// in-flight init child without growing the public API surface. Pass nil to
// behave identically to RunWorktreeSetup.
func RunWorktreeSetupWithObserver(wtPath, initScript, repoRoot string, observer func(cmd *exec.Cmd)) error {
	cmd, notFound, err := ResolveInitInvocation(initScript, repoRoot)
	if err != nil {
		return err
	}
	if notFound != nil {
		fmt.Fprintln(os.Stderr, notFound.RenderWarning())
		return nil
	}

	fmt.Fprintln(os.Stderr, "Running worktree init...")
	cmd.Dir = wtPath
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if observer != nil {
		observer(cmd)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("init script failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Worktree init complete.")
	return nil
}
