package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sentinel errors for the branch-selection contract. cmd/create.go maps these
// (via errors.Is) to ExitInvalidArgs with the user-facing fix hint pointing at
// the correct flag, keeping the existence business-rule inside internal/
// (Constitution V) while cmd/ only routes flags.
var (
	// ErrBranchExists is returned by CreateNewBranchWorktree when the named
	// branch already exists locally or remotely. The positional argument only
	// creates NEW branches; an existing branch is a --checkout job.
	ErrBranchExists = errors.New("branch already exists")

	// ErrBranchNotFound is returned by CheckoutBranchWorktree when the named
	// branch exists neither locally nor remotely. --checkout requires an
	// existing branch; a missing branch is a create-new job.
	ErrBranchNotFound = errors.New("branch not found")
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

// CreateNewBranchWorktree creates a worktree on a NEW branch. If the branch
// already exists locally or remotely it returns ErrBranchExists and creates
// nothing — checking out an existing branch is CheckoutBranchWorktree's job
// (the positional argument only creates new branches). startPoint, when
// non-empty, is the git start-point for the new branch (else HEAD).
// Returns the worktree path.
func CreateNewBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback, startPoint string) (string, error) {
	if BranchExistsLocally(branch) || BranchExistsRemotely(branch) {
		return "", ErrBranchExists
	}

	if err := EnsureWorktreesDir(ctx.WorktreesDir); err != nil {
		return "", err
	}

	wtPath := filepath.Join(ctx.WorktreesDir, name)

	if err := CreateWorktree(wtPath, branch, true, startPoint); err != nil {
		return "", err
	}
	rb.Register("git", "worktree", "remove", "--force", wtPath)
	rb.Register("git", "branch", "-D", branch)

	return wtPath, nil
}

// CheckoutBranchWorktree creates a worktree on an EXISTING branch — a local
// branch is checked out as-is; a remote-only branch is fetched then checked
// out. If the branch exists neither locally nor remotely it returns
// ErrBranchNotFound and creates nothing (--checkout requires an existing
// branch). Returns the worktree path.
func CheckoutBranchWorktree(branch, name string, ctx *RepoContext, rb *Rollback) (string, error) {
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
		return "", ErrBranchNotFound
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
//   - On the default-not-applicable skip (isDefault AND exit 3, per
//     DefaultNotApplicable), prints the skip warning to stderr and returns nil.
//   - Wraps the exec error (via %w) on any other init-script non-zero exit so
//     callers can still extract *exec.ExitError via errors.As.
//
// isDefault carries InitScriptPath's provenance (true only for the built-in
// "fab sync" default) so an explicitly configured script always fails hard.
//
// Callers that want to confirm with the user before running MUST call
// ConfirmYesNo themselves first — confirmation is no longer part of the
// runner so wt create's SIGINT-during-init handler can be installed AFTER
// the prompt completes (installing it before would consume Ctrl-C during
// the prompt with no init child to target).
func RunWorktreeSetup(wtPath, initScript string, isDefault bool, repoRoot string) error {
	return RunWorktreeSetupWithObserver(wtPath, initScript, isDefault, repoRoot, nil)
}

// RunWorktreeSetupWithObserver is like RunWorktreeSetup but invokes observer
// with the resolved *exec.Cmd immediately before cmd.Run(). The observer
// lets wt create's SIGINT-during-init handler capture a reference to the
// in-flight init child without growing the public API surface. Pass nil to
// behave identically to RunWorktreeSetup.
func RunWorktreeSetupWithObserver(wtPath, initScript string, isDefault bool, repoRoot string, observer func(cmd *exec.Cmd)) error {
	cmd, notFound, err := ResolveInitInvocation(initScript, repoRoot)
	if err != nil {
		return err
	}
	if notFound != nil {
		fmt.Fprintln(os.Stderr, notFound.RenderWarning())
		return nil
	}

	// Init phase separator, labeled with the resolved init command as the
	// user would recognize it (e.g. "fab sync" or a script path). Emitted on
	// stderr immediately before the existing "Running worktree init..." line
	// so captured logs can attribute the init script's output to this phase.
	fmt.Fprintln(os.Stderr, PhaseSeparator("Init ("+initScript+")"))
	fmt.Fprintln(os.Stderr, "Running worktree init...")
	cmd.Dir = wtPath
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if observer != nil {
		observer(cmd)
	}
	if err := cmd.Run(); err != nil {
		// Default-not-applicable skip: the built-in "fab sync" default ran in a
		// repo that is not fab-managed and exited ExitNotManaged (3). Treat this
		// as an init no-op — warn on stderr and return nil so wt create proceeds
		// to the Open phase exactly as on success (no banner, no prompt, exit 0).
		// An explicitly configured script (isDefault=false) never reaches this
		// branch; it falls through to the hard-failure return below.
		if DefaultNotApplicable(err, isDefault) {
			fmt.Fprintln(os.Stderr, RenderDefaultSkipWarning())
			return nil
		}
		return fmt.Errorf("init script failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Worktree init complete.")
	return nil
}
