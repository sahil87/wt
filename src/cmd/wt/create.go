package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// prNumberPattern is the accepted --pr value: a positive decimal PR number.
var prNumberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

func createCmd() *cobra.Command {
	var (
		worktreeName   string
		worktreeInit   string
		noInit         bool
		worktreeOpen   string
		reuse          bool
		nonInteractive bool
		base           string
		checkout       string
		pr             string
		temp           string
		initFlag       bool
		fetch          bool
		jsonOut        bool
	)

	cmd := &cobra.Command{
		Use:     "create [branch]",
		Aliases: []string{"new"},
		Short:   "Create a git worktree",
		Long: `Create a git worktree for parallel development.

When BRANCH is omitted, creates an exploratory worktree with a random name (or
the value of --name, if given) on a new branch of the same name.

When BRANCH is provided, it names a NEW branch to create (off --base, else
HEAD). If that branch already exists locally or remotely, the command fails —
use --checkout to put a worktree on an existing branch.

--checkout <branch> checks out an EXISTING branch (local as-is, or remote-only
fetched then checked out). --checkout cannot be combined with a positional
branch argument or with --base.

--reuse cannot be combined with --base or --fetch: a reused worktree keeps its
existing branch, so a start-point has nothing to apply to (and a fresh --reuse
worktree starts at HEAD, which a fetch never moves).

--pr <n> creates the worktree on pull request <n>'s head branch, resolved
with the GitHub CLI (gh pr view), then checked out exactly like --checkout.
Only same-repository PRs are supported; a PR from a fork exits 2. --pr cannot
be combined with a positional branch argument, --base, --checkout, --reuse,
or --temp.

--temp <ref> creates a DETACHED throwaway worktree at <ref> (branch, tag, or
commit SHA): no branch, no git config, name prefixed tmp-. The init script is
skipped unless --init is given. --temp cannot be combined with a positional
branch argument, --checkout, --reuse, --base, or --pr.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var branchArg string
			if len(args) > 0 {
				branchArg = args[0]
			}

			// doc guards the --json document: exactly one is ever printed, even
			// when the SIGINT/SIGTERM handler races a failure or success path.
			// printSuccess/printFailure are the main-path emitters; if the signal
			// handler already claimed the document they park forever so the
			// handler's exit 130 is the process's exit (the document says 130).
			var doc jsonDocGuard
			printSuccess := func(r wt.CreateReceipt) {
				if d, err := wt.FormatCreateSuccessJSON(r); err == nil && !doc.emit(d) {
					select {}
				}
			}
			printFailure := func(code int, message string, r *wt.CreateReceipt) {
				if d, err := wt.FormatCreateFailureJSON(code, message, r); err == nil && !doc.emit(d) {
					select {}
				}
			}

			// fail is the single failure exit for RunE: the structured error goes
			// to stderr as always, and under --json the failure document goes to
			// stdout (exactly one document on every exit path). It exits the
			// process with code.
			fail := func(code int, what, why, fix string) {
				wt.PrintError(what, why, fix)
				if jsonOut {
					printFailure(code, what, nil)
				}
				os.Exit(code)
			}

			// --json implies --non-interactive and defaults --open to skip; an
			// explicit non-skip --open is a usage error (opening an app can write
			// to stdout or prompt, and stdout is reserved for the one document).
			if jsonOut {
				if (cmd.Flags().Changed("open") || cmd.Flags().Changed("worktree-open")) && worktreeOpen != "skip" {
					fail(wt.ExitInvalidArgs,
						fmt.Sprintf("--json cannot be combined with --open %s", worktreeOpen),
						"--json reserves stdout for one JSON document; opening an app can write to stdout or prompt",
						"Drop --open (it defaults to skip under --json), or drop --json")
				}
				nonInteractive = true
			}

			// Apply defaults. --no-init (real bool) supersedes the deprecated
			// string --worktree-init when explicitly set: types differ, so the
			// two flags cannot share a variable — the new bool wins via
			// Changed(), else the old string path is honored. Both funnel into
			// the existing worktreeInit "true"/"false" string, so the rest of
			// the flow (reuse-init, runInit) is unchanged.
			if cmd.Flags().Changed("no-init") {
				if noInit {
					worktreeInit = "false"
				} else {
					worktreeInit = "true"
				}
			}
			if worktreeInit == "" {
				worktreeInit = "true"
			}
			if worktreeOpen == "" {
				if nonInteractive {
					worktreeOpen = "skip"
				} else {
					worktreeOpen = "prompt"
				}
			}

			tempMode := cmd.Flags().Changed("temp")
			checkModeConflicts(fail, tempMode, branchArg, checkout, reuse, base, cmd.Flags().Changed("pr"), initFlag, noInit)

			// Validate --reuse requires --name
			if reuse && worktreeName == "" {
				fail(wt.ExitInvalidArgs,
					"--reuse requires --name",
					"--reuse only works with an explicit worktree name",
					"Example: wt create --reuse --name my-feature branch-name")
			}

			// --checkout selects an existing branch; the positional creates a
			// new one. They are mutually exclusive modes, as are --checkout and
			// --base, and --reuse and --base (--base is the start-point for a
			// NEW branch only). --reuse and --fetch conflict too: a reuse keeps
			// its branch and a --reuse miss starts at HEAD, which a fetch never
			// moves.
			if checkout != "" && branchArg != "" {
				fail(wt.ExitInvalidArgs,
					"--checkout cannot be combined with a positional branch argument",
					"The positional creates a new branch; --checkout checks out an existing one",
					"Use one of: wt create <new-branch> | wt create --checkout <existing-branch>")
			}
			if checkout != "" && base != "" {
				fail(wt.ExitInvalidArgs,
					"--base cannot be combined with --checkout",
					"--base is the start-point for a NEW branch; --checkout targets an existing branch",
					"Drop --base, or create a new branch: wt create <name> --base <ref>")
			}
			if reuse && base != "" {
				fail(wt.ExitInvalidArgs,
					"--reuse and --base cannot be combined",
					"--reuse keeps the existing worktree's branch, so a start-point has nothing to apply to",
					"Drop --base, or drop --reuse to create a new worktree")
			}
			if reuse && fetch {
				fail(wt.ExitInvalidArgs,
					"--reuse and --fetch cannot be combined",
					"--reuse keeps the existing worktree's branch, and a new worktree from --reuse starts at HEAD, so there is no base for --fetch to refresh",
					"Drop --fetch, or drop --reuse to create a new worktree (wt create --name <name> --base origin/<branch> --fetch)")
			}

			// --pr resolves a PR's head branch and then runs the --checkout
			// path, so it excludes every other branch selector. Keyed on
			// Changed, not the value, so an explicit empty --pr "" (e.g. an
			// unset $PR in a script) is rejected rather than falling through
			// to a bare create.
			if cmd.Flags().Changed("pr") {
				if !prNumberPattern.MatchString(pr) {
					fail(wt.ExitInvalidArgs,
						fmt.Sprintf("Invalid --pr value: %s", pr),
						"--pr takes a pull request number",
						"Example: wt create --pr 42")
				}
				if branchArg != "" {
					fail(wt.ExitInvalidArgs,
						"--pr cannot be combined with a positional branch argument",
						"--pr checks out the PR's existing head branch; the positional creates a new branch",
						"Use one of: wt create <new-branch> | wt create --pr <n>")
				}
				if base != "" {
					fail(wt.ExitInvalidArgs,
						"--base cannot be combined with --pr",
						"--base is the start-point for a NEW branch; --pr targets the PR's existing head branch",
						"Drop --base, or create a new branch: wt create <name> --base <ref>")
				}
				if checkout != "" {
					fail(wt.ExitInvalidArgs,
						"--pr cannot be combined with --checkout",
						"Both select an existing branch to check out",
						"Use one of: wt create --pr <n> | wt create --checkout <branch>")
				}
				if reuse {
					fail(wt.ExitInvalidArgs,
						"--pr cannot be combined with --reuse",
						"--reuse keeps an existing worktree's branch, so there is no PR head to check out",
						"Drop --reuse, or reuse by name: wt create --reuse --name <name>")
				}
			}

			// Validate git repo
			if err := wt.ValidateGitRepo(); err != nil {
				fail(wt.ExitGitError,
					"Not a git repository",
					"This command requires a git repository",
					"Navigate to a git repository and try again")
			}

			ctx, err := wt.GetRepoContext()
			if err != nil {
				fail(wt.ExitGeneralError,
					"Not a git repository",
					"This command must be run from within a git repository",
					"Navigate to a git repository and try again")
			}

			// Resolve --pr to its head branch before any prompt or mutation, so
			// a failed lookup leaves nothing behind. From here on --pr is the
			// --checkout path.
			if pr != "" {
				head, err := wt.ResolvePRHead(pr)
				switch {
				case errors.Is(err, wt.ErrGhNotFound):
					fail(wt.ExitGeneralError,
						"GitHub CLI (gh) not found",
						"--pr resolves the pull request's head branch with gh",
						"Install gh (https://cli.github.com) and run gh auth login, or use: wt create --checkout <branch>")
				case errors.Is(err, wt.ErrPRCrossRepository):
					fail(wt.ExitInvalidArgs,
						fmt.Sprintf("PR #%s is from a fork (head branch '%s')", pr, head.Branch),
						"--pr only supports pull requests whose head branch is in this repository",
						"Add the fork as a remote and use --checkout")
				case err != nil:
					fail(wt.ExitGeneralError,
						fmt.Sprintf("Could not resolve PR #%s", pr),
						err.Error(),
						"Check the PR number and that gh is authenticated (gh auth status)")
				}
				checkout = head.Branch
			}

			// A --reuse create whose name already exists takes the reuse path:
			// no base resolution (--reuse + --base and --reuse + --fetch were
			// rejected above). --reuse requires --name, so the final name is known here.
			reuseHit := reuse && wt.CheckNameCollision(ctx.WorktreesDir, worktreeName)

			// --fetch: refresh the base's remote BEFORE the base (or the --temp
			// ref) is resolved so the worktree is cut from fresh refs. A no-op
			// for --checkout / --pr (the remote-only path already fetches).
			// All of this precedes the create dispatch (reinstall-window rule).
			if fetch && checkout == "" {
				fetchRef := base
				if tempMode {
					fetchRef = temp
				}
				remote := wt.RemoteForRef(fetchRef)
				if err := wt.FetchRemote(remote); err != nil {
					fail(wt.ExitGitError,
						fmt.Sprintf("Failed to fetch remote '%s'", remote),
						err.Error(),
						"Check the remote URL and your network/credentials (git remote -v), or drop --fetch to use the local ref")
				}
			}

			// --temp: resolve the ref to a commit once, up front (before any
			// prompt or mutation). tempSHA also feeds the "From:" summary label.
			var tempSHA string
			if tempMode {
				tempSHA, err = wt.ResolveCommit(temp)
				if err != nil {
					fail(wt.ExitInvalidArgs,
						fmt.Sprintf("Invalid --temp ref: %s", temp),
						fmt.Sprintf("'%s' does not resolve to a commit", temp),
						"Provide a valid branch name, tag, or commit SHA")
				}
			}

			// Set up rollback and signal handling
			rb := wt.NewRollback()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				if jsonOut {
					// stdout is reserved for the one document: emit the
					// interrupted failure instead of the framing newline.
					d, _ := wt.FormatCreateFailureJSON(130, "interrupted", nil)
					if !doc.emit(d) {
						// A document is already out: the main path owns the exit.
						return
					}
				} else {
					fmt.Println()
				}
				rb.Execute()
				os.Exit(130)
			}()
			defer func() {
				rb.Execute()
			}()

			// Validate branch name — the positional (new branch) or the
			// --checkout target (existing branch) are validated the same way.
			branchToValidate := branchArg
			if checkout != "" {
				branchToValidate = checkout
			}
			if branchToValidate != "" {
				if err := wt.ValidateBranchName(branchToValidate); err != nil {
					fail(wt.ExitInvalidArgs,
						"Invalid branch name",
						fmt.Sprintf("Branch name '%s' contains invalid characters", branchToValidate),
						"Use alphanumeric characters, hyphens, and single slashes")
				}
			}

			// Resolve the new-branch start-point ONCE to a full SHA and pin it:
			// the branch is cut from exactly this commit and the receipt's
			// base_sha reports it. --base always applies to a NEW branch
			// (--base + --checkout was rejected above); --checkout and a taken
			// --reuse never create a branch, and --temp detaches at its own
			// resolved ref, so they resolve nothing here.
			var baseSHA, baseUpstream string
			if checkout == "" && !reuseHit && !tempMode {
				if base != "" {
					sha, err := wt.ResolveCommit(base)
					if err != nil {
						fail(wt.ExitInvalidArgs,
							fmt.Sprintf("Invalid --base ref: %s", base),
							fmt.Sprintf("'%s' does not resolve to a valid git object", base),
							"Provide a valid branch name, tag, or commit SHA")
					}
					baseSHA = sha
				} else {
					sha, err := wt.ResolveCommit("HEAD")
					if err != nil {
						fail(wt.ExitGitError,
							"Failed to resolve HEAD",
							err.Error(),
							"Make an initial commit first, or pass --base <ref>")
					}
					baseSHA = sha
				}
			}

			// One MenuSession threaded through every interactive stdin consumer
			// in this flow (both menus + all three line prompts). Sharing one
			// reader is what prevents a menu's parked read-ahead pump from
			// stealing the next consumer's first line/keystroke — see the
			// MenuSession doc comment in internal/worktree/menu.go. Matches the
			// session-threading pattern already used by wt open/delete/go.
			session := wt.NewMenuSession()
			defer session.Close()

			// Dirty-state check
			if !nonInteractive && (wt.HasUncommittedChanges() || wt.HasUntrackedFiles()) {
				wt.Warn("current worktree has uncommitted changes")
				choice, err := session.Show("How to proceed?", []string{
					"Continue anyway",
					"Stash changes first",
					"Abort",
				}, -1)
				if err != nil {
					fail(wt.ExitGeneralError, "Menu error", err.Error(), "")
				}
				switch choice {
				case 1: // continue
				case 2:
					stashID, err := wt.StashCreate("wt-create: pre-creation stash")
					if err != nil {
						fail(wt.ExitGeneralError,
							"Failed to create stash",
							err.Error(),
							"Resolve any repository issues and try again")
					}
					if stashID != "" {
						fmt.Fprintf(os.Stderr, "Created stash %s for pre-creation changes.\n", stashID)
					}
				case 3, 0:
					rb.Disarm()
					return nil
				}
			}

			// Determine suggested name. Bare create → random unique name;
			// a named branch (new via positional, or existing via --checkout)
			// → derived from the branch name.
			var suggestedName string
			switch {
			case tempMode:
				suggestedName, err = wt.GenerateUniqueNameWithPrefix(ctx.WorktreesDir, wt.TempNamePrefix, 10)
				if err != nil {
					fail(wt.ExitRetryExhausted,
						"Could not find unique worktree name",
						"All 10 random name attempts collided with existing worktrees",
						fmt.Sprintf("Remove some worktrees from %s or increase retries", ctx.WorktreesDir))
				}
			case checkout != "":
				suggestedName = wt.DeriveWorktreeName(checkout)
			case branchArg != "":
				suggestedName = wt.DeriveWorktreeName(branchArg)
			default:
				suggestedName, err = wt.GenerateUniqueName(ctx.WorktreesDir, 10)
				if err != nil {
					fail(wt.ExitRetryExhausted,
						"Could not find unique worktree name",
						"All 10 random name attempts collided with existing worktrees",
						fmt.Sprintf("Remove some worktrees from %s or increase retries", ctx.WorktreesDir))
				}
			}

			// Resolve final name. A temp worktree needs no naming ceremony: the
			// prompt is skipped and the tmp- prefix is always applied (an
			// explicit --name included, without doubling an existing prefix).
			var finalName string
			if worktreeName != "" {
				finalName = worktreeName
			} else if nonInteractive || tempMode {
				finalName = suggestedName
			} else {
				finalName = session.PromptWithDefault("Worktree name", suggestedName)
			}
			if tempMode {
				finalName = wt.TempName(finalName)
			}

			// Check collision
			if wt.CheckNameCollision(ctx.WorktreesDir, finalName) {
				if reuse {
					fmt.Fprintf(os.Stderr, "Reusing existing worktree: %s\n", finalName)
					rb.Disarm()
					existingWtPath := filepath.Join(ctx.WorktreesDir, finalName)
					// Run init script on reuse — ensures skills are current even in existing worktrees.
					// Non-fatal: reuse proceeds even if init fails (existing worktree may be functional).
					reuseInit := wt.InitSkipped
					if worktreeInit == "true" {
						initScript, isDefault := wt.InitScriptPath()
						var err error
						reuseInit, err = wt.RunWorktreeSetupOutcome(existingWtPath, initScript, isDefault, ctx.RepoRoot, nil)
						if err != nil {
							wt.Warn("worktree init failed for reused worktree %q: %v", finalName, err)
						}
					}
					if jsonOut {
						// The reused worktree's branch is read best-effort;
						// unreadable or detached is null.
						var reuseBranch *string
						if b := wt.BranchForPath(existingWtPath); b != "unknown" && b != "HEAD" {
							reuseBranch = wt.StrPtr(b)
						}
						printSuccess(wt.CreateReceipt{
							Path: existingWtPath, Name: finalName, Branch: reuseBranch,
							Mode: wt.ModeReuse, Init: reuseInit,
						})
						return nil
					}
					fmt.Println(existingWtPath)
					return nil
				}
				fail(wt.ExitGeneralError,
					fmt.Sprintf("Worktree '%s' already exists", finalName),
					fmt.Sprintf("A worktree with this name already exists at %s/%s", ctx.WorktreesDir, finalName),
					"Remove the existing worktree or use a different branch name")
			}

			// Create worktree
			//
			// Reinstall-window contract (spec § Signal Handling During Init):
			// no I/O, user prompts, or nontrivial work between the worktree-add
			// returning and the signal.Reset below. The "Created worktree:"
			// summary lines are deferred until AFTER the signal swap. If init is
			// disabled (worktreeInit != "true"), the summary still gets printed
			// via the late-print path below.
			//
			// Three modes route here: bare create (exploratory new branch),
			// a positional (NEW branch — existing → ExitInvalidArgs pointing at
			// --checkout), and --checkout (EXISTING branch — missing →
			// ExitInvalidArgs pointing at create-new; --pr arrives here as
			// --checkout of the PR's head). The existence rules live
			// in internal/worktree; cmd/ maps the sentinel errors to exit codes.
			var wtPath string
			var createdSummaryBranch string // branch label shown in the summary

			// Resolve the "From:" summary label BEFORE the create dispatch, so
			// the one possible git query (DescribeHead) stays outside the tight
			// reinstall window between git worktree add and the init-phase
			// signal.Reset (see create-output-phases.md / the reinstall-window
			// contract). wt create never moves the invoking worktree's HEAD, so
			// HEAD here equals HEAD at summary time — resolving pre-dispatch is
			// equivalent.
			//   --pr       → fixed copy naming the PR and its head (no query)
			//   --temp     → the ref as typed plus the resolved short SHA (no query)
			//   --checkout → fixed copy naming the existing-branch path (no query)
			//   --base     → the ref as typed plus " @ <short-sha>" of the pinned
			//                start-point (no further query)
			//   else       → the resolved HEAD label (one query, best-effort)
			// baseRefLabel is the receipt's base_ref: the --base ref verbatim,
			// or the HEAD label when no --base was given.
			var createdSummaryFrom, baseRefLabel string
			switch {
			case tempMode:
				createdSummaryFrom = fmt.Sprintf("%s @ %s", temp, wt.ShortSHA(tempSHA))
			case pr != "":
				createdSummaryFrom = fmt.Sprintf("PR #%s (existing branch '%s')", pr, checkout)
			case checkout != "":
				createdSummaryFrom = fmt.Sprintf("existing branch '%s' (checked out directly)", checkout)
			case base != "":
				baseRefLabel = base
				createdSummaryFrom = base + " @ " + wt.ShortSHA(baseSHA)
			default:
				baseRefLabel = wt.DescribeHead()
				createdSummaryFrom = baseRefLabel
			}

			// failIfSetUpstream maps a post-add set-upstream failure (already
			// rolled back by internal/worktree) to its own exit-3 copy.
			failIfSetUpstream := func(err error) {
				var upErr *wt.SetUpstreamError
				if errors.As(err, &upErr) {
					why, _, _ := strings.Cut(upErr.Output, "\n")
					fail(wt.ExitGitError,
						fmt.Sprintf("Failed to set upstream '%s' for branch '%s'", upErr.Upstream, upErr.Branch),
						why,
						"Check that .git/config is writable (no stale config.lock), then retry")
				}
			}

			// failIfRecordBase maps a post-add record-base failure (already
			// rolled back by internal/worktree) to its own exit-3 copy.
			failIfRecordBase := func(err error) {
				var recErr *wt.RecordBaseError
				if errors.As(err, &recErr) {
					why, _, _ := strings.Cut(recErr.Output, "\n")
					fail(wt.ExitGitError,
						fmt.Sprintf("Failed to record base for branch '%s'", recErr.Branch),
						why,
						"Check that .git/config is writable (no stale config.lock), then retry")
				}
			}

			// The upstream git would have auto-tracked had it been given the ref
			// name rather than the pinned SHA: a pure lookup resolved here, before
			// the dispatch, so only the write happens after the worktree add.
			if checkout == "" && !tempMode {
				newBranch := branchArg
				if newBranch == "" {
					newBranch = finalName
				}
				baseUpstream = wt.UpstreamForBase(base, newBranch)
			}

			switch {
			case tempMode:
				wtPath, err = wt.CreateTempWorktree(tempSHA, finalName, ctx, rb)
				if err != nil {
					fail(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"Check that the commit is reachable and the worktrees directory is writable")
				}
				createdSummaryBranch = wt.DetachedBranchLabel
			case checkout != "":
				wtPath, err = wt.CheckoutBranchWorktree(checkout, finalName, ctx, rb)
				if err != nil {
					if errors.Is(err, wt.ErrBranchNotFound) && pr != "" {
						fail(wt.ExitInvalidArgs,
							fmt.Sprintf("PR #%s head branch '%s' not found", pr, checkout),
							"The branch exists neither locally nor on origin",
							fmt.Sprintf("Fetch it yourself, or check the PR: gh pr view %s", pr))
					}
					if errors.Is(err, wt.ErrBranchNotFound) {
						fail(wt.ExitInvalidArgs,
							fmt.Sprintf("Branch '%s' not found", checkout),
							"--checkout requires an existing local or remote branch",
							fmt.Sprintf("To create a new branch: wt create %s [--base <ref>]", checkout))
					}
					fail(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"The branch may already be checked out in another worktree")
				}
				createdSummaryBranch = checkout
			case branchArg != "":
				wtPath, err = wt.CreateNewBranchWorktree(branchArg, finalName, ctx, rb, baseRefLabel, baseSHA, baseUpstream)
				if err != nil {
					if errors.Is(err, wt.ErrBranchExists) {
						fail(wt.ExitInvalidArgs,
							fmt.Sprintf("Branch '%s' already exists", branchArg),
							"The positional argument only creates new branches",
							fmt.Sprintf("To put a worktree on the existing branch: wt create --checkout %s", branchArg))
					}
					failIfSetUpstream(err)
					failIfRecordBase(err)
					fail(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"The branch may already be checked out in another worktree")
				}
				createdSummaryBranch = branchArg
			default:
				wtPath, err = wt.CreateExploratoryWorktree(finalName, ctx, rb, baseRefLabel, baseSHA, baseUpstream)
				if err != nil {
					failIfSetUpstream(err)
					failIfRecordBase(err)
					fail(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"Check if the branch already exists or if there are permission issues")
				}
				createdSummaryBranch = finalName
			}

			// Assemble the receipt (pure data, no I/O — safe before the
			// init-phase signal.Reset). init is filled in once init has run.
			receipt := wt.CreateReceipt{
				Path:   wtPath,
				Name:   finalName,
				Branch: wt.StrPtr(createdSummaryBranch),
				Mode:   wt.ModeNew,
				Init:   wt.InitSkipped,
			}
			switch {
			case tempMode:
				receipt.Mode = wt.ModeTemp
				receipt.Branch = nil
				receipt.BaseRef = wt.StrPtr(temp)
				receipt.BaseSHA = wt.StrPtr(tempSHA)
			case checkout != "":
				receipt.Mode = wt.ModeCheckout
			default:
				receipt.BaseRef = wt.StrPtr(baseRefLabel)
				receipt.BaseSHA = wt.StrPtr(baseSHA)
			}
			// exitInitFailed ends the run for an init failure with the worktree
			// kept: exit 7, and under --json the failure document carries the
			// kept worktree's receipt.
			exitInitFailed := func(initErr error) {
				if jsonOut {
					printFailure(wt.ExitInitFailed, wt.InitFailureMessage(initErr), &receipt)
				}
				os.Exit(wt.ExitInitFailed)
			}

			// Setup
			//
			// The init-phase signal handler MUST be installed AFTER any
			// confirmation prompt completes (a SIGINT during the prompt
			// would otherwise be consumed by the init handler with no init
			// child to target — see Copilot review on PR #7). The flow:
			//   1. While the rollback handler is still active, optionally
			//      prompt the user (Ctrl-C during the prompt rolls back —
			//      correct: init hasn't started, abort the whole creation).
			//   2. Emit the deferred summary lines (still under the
			//      rollback handler).
			//   3. Swap to the init-phase handler with NO I/O in between.
			//   4. Run init via RunWorktreeSetupOutcome (no prompt
			//      inside the runner).
			// The confirm fires whenever a branch was explicitly named —
			// positional (new branch) or --checkout / --pr (existing branch) —
			// and is skipped for the bare exploratory create (and for --temp,
			// whose init is opt-in via --init — that flag is the consent, no confirm).
			runInit := worktreeInit == "true"
			if tempMode {
				runInit = initFlag
			}
			if runInit && !(nonInteractive || (branchArg == "" && checkout == "")) {
				runInit = session.ConfirmYesNo("Initialize worktree?")
			}

			// Emit deferred summary (still under rollback handler). The Git
			// phase separator joins this emission — it precedes the summary
			// (the Git phase's output) and stays before the init-phase
			// signal.Reset, so no new I/O enters the tight reinstall window.
			fmt.Fprintln(os.Stderr, wt.PhaseSeparator("Git"))
			fmt.Fprintf(os.Stderr, "Created worktree: %s\nPath: %s\nBranch: %s\nFrom: %s\n",
				finalName, wtPath, createdSummaryBranch, createdSummaryFrom)

			// initFailed records that the init script exited non-zero. It is
			// set in the failure branch below instead of an inline os.Exit so
			// the interactive "open anyway" path can fall through to the Open
			// phase (phase 5). It is read at the very end of the function to
			// force ExitInitFailed on ALL init-failure paths — a successful
			// open must NOT downgrade the exit to 0.
			var initFailed bool
			var initFailedErr error
			if runInit {
				initScript, isDefault := wt.InitScriptPath()

				// Terminal-foreground bookkeeping. wt runs the init child in
				// its own process group (Setpgid: true) while sharing wt's
				// controlling terminal. If the init script (or a descendant)
				// grabs terminal foreground and exits without restoring it, wt
				// is left in the background and the next TTY write (the
				// Open-phase menu render below) trips SIGTTOU and suspends the
				// process. We capture the foreground pgrp before init and
				// reclaim it after, on every exit path. No-op when stdin is not
				// a TTY (--non-interactive / piped / CI).
				//
				// tcgetpgrp is a single cheap syscall with no I/O or prompt, so
				// capturing it here — adjacent to the signal.Reset below — does
				// NOT widen the tight reinstall window (SIGINT Option B
				// contract). The captured pgrp IS wt's own process group
				// (equivalently unix.Getpgrp()), since wt is the terminal
				// foreground at this point — it is the source of truth for
				// "give the terminal back to me".
				ttyFd := int(os.Stdin.Fd())
				reclaimTTY := term.IsTerminal(ttyFd)
				var wtPgid int
				if reclaimTTY {
					fg, err := terminalForeground(ttyFd)
					if err != nil {
						// tcgetpgrp failed unexpectedly on a TTY — skip the
						// bookkeeping rather than reclaim to a bogus pgrp.
						reclaimTTY = false
					} else {
						wtPgid = fg
					}
				}
				// Best-effort restore: a panic/early-return safety net. The
				// single explicit reclaim below (run unconditionally after the
				// init call, before the banner / open-anyway prompt / Open menu)
				// is the load-bearing one — every init-failure path either
				// os.Exits (non-interactive) or falls through to Open, and
				// os.Exit skips deferred funcs, so this defer only actually
				// fires on a non-os.Exit return or a panic in this block. (The
				// pre-init SIGINT handler's exit-130 path runs before this defer
				// is installed and before any foreground was captured, so it has
				// nothing to reclaim.) It never blocks rollback or changes exit
				// codes.
				if reclaimTTY {
					defer reclaimTerminalForeground(ttyFd, wtPgid)
				}

				// SIGINT Option B: git ops are done AND any prompt has been
				// accepted. Reinstall the signal handler so SIGINT/SIGTERM
				// target the init child's process group (not rb.Execute) —
				// Ctrl-C while init is running means "stop this script", not
				// "nuke the worktree". Keep this window tight: NO I/O or user
				// prompts between this point and the new handler being
				// installed. The previous signal.Notify above is overridden
				// by signal.Reset.
				signal.Reset(syscall.SIGINT, syscall.SIGTERM)
				var initCmdPtr atomic.Pointer[exec.Cmd]
				initSigCh := make(chan os.Signal, 1)
				signal.Notify(initSigCh, syscall.SIGINT, syscall.SIGTERM)
				go func() {
					if _, ok := <-initSigCh; !ok {
						return
					}
					if c := initCmdPtr.Load(); c != nil && c.Process != nil {
						signalInitProcessGroup(c)
					}
				}()
				captureInit := func(c *exec.Cmd) { initCmdPtr.Store(c) }

				initOutcome, initErr := wt.RunWorktreeSetupOutcome(wtPath, initScript, isDefault, ctx.RepoRoot, captureInit)
				receipt.Init = initOutcome

				// SIGINT Option B teardown — tear down the init-child signal
				// handler before any further TTY work (banner, open-anyway
				// prompt, or the Open menu). This runs on EVERY init outcome
				// (success and the open-anyway fall-through), so the init
				// handler is never left armed once the init child has exited.
				signal.Stop(initSigCh)
				close(initSigCh)
				// Reclaim foreground before any further TTY write. This is the
				// load-bearing reclaim: it precedes the init-failure banner AND
				// the open-anyway prompt AND the Open menu (the menu is reached
				// either on init success or via the open-anyway Yes fall-through
				// below) — all are TTY writes that would SIGTTOU if foreground
				// were stranded by a shared-TTY init child.
				if reclaimTTY {
					reclaimTerminalForeground(ttyFd, wtPgid)
				}

				if initErr != nil {
					// Init-script non-zero exit: keep the worktree and print the
					// structured banner. We do NOT os.Exit inline — instead we
					// set initFailed and (when interactive) offer to open the
					// kept worktree anyway by falling through to the Open phase.
					// The function exits ExitInitFailed at the end on every
					// init-failure path (incl. a successful open-anyway open), so
					// operators can still distinguish "worktree exists, init
					// didn't complete" from other failures.
					initFailed = true
					initFailedErr = initErr
					// Worktree is KEPT on init-script non-zero exit: disarm the
					// rollback so the deferred rb.Execute() (which still fires on
					// any OTHER failure) does not remove the just-created worktree.
					rb.Disarm()
					wt.PrintInitFailureBanner(wtPath, finalName, initErr)

					// Open-anyway prompt is interactive-only, gated by the same
					// TTY / --non-interactive discipline the rest of create uses
					// (!nonInteractive AND stdin is a TTY — reclaimTTY is that
					// signal, already computed above). Non-interactive / piped /
					// CI keeps today's exact behavior: banner + exit 7, NO prompt.
					if !nonInteractive && reclaimTTY {
						if !session.ConfirmYesNo("Continue and open the worktree anyway?") {
							// No: do not open. The banner's Go: line already shows
							// how to reach the worktree, so no app menu is shown.
							worktreeOpen = "skip"
						}
						// Yes: fall through into the existing Open phase (foreground
						// already reclaimed above) so the user can open the kept
						// worktree. Either way the function exits 7 at the end via
						// the initFailed check.
					} else {
						// Non-interactive: exit 7 immediately, no prompt, no Open.
						exitInitFailed(initErr)
					}
				}
			}

			// Open
			//
			// The Open separator is emitted only when the open phase will
			// actually run — never for the skip case (incl. --non-interactive
			// defaulting to skip), since a separator must not precede a phase
			// that emits nothing.
			if worktreeOpen != "skip" {
				fmt.Fprintln(os.Stderr, wt.PhaseSeparator("Open"))
			}
			if worktreeOpen == "prompt" {
				apps := wt.BuildAvailableApps()
				if len(apps) > 0 {
					defaultIdx := wt.DetectDefaultApp(apps)
					appNames := make([]string, len(apps))
					for i, a := range apps {
						appNames[i] = a.Name
					}
					choice, err := session.Show("Open in:", appNames, defaultIdx)
					if err == nil && choice > 0 && choice <= len(apps) {
						selected := apps[choice-1]
						wt.SaveLastApp(selected.Cmd)
						if openErr := wt.OpenInApp(selected.Cmd, wtPath, ctx.RepoName, finalName); openErr != nil {
							wt.Warn("could not open in %s: %s", selected.Name, openErr)
						}
					}
				}
			} else if worktreeOpen == "default" {
				apps := wt.BuildAvailableApps()
				resolved, err := wt.ResolveDefaultApp(apps)
				if err != nil {
					wt.Warn("%s", err)
				} else {
					wt.SaveLastApp(resolved.Cmd)
					if openErr := wt.OpenInApp(resolved.Cmd, wtPath, ctx.RepoName, finalName); openErr != nil {
						wt.Warn("could not open in %s: %s", resolved.Name, openErr)
					}
				}
			} else if worktreeOpen != "skip" {
				apps := wt.BuildAvailableApps()
				resolved, err := wt.ResolveApp(worktreeOpen, apps)
				if err != nil {
					wt.Warn("%s", err)
				} else {
					wt.SaveLastApp(resolved.Cmd)
					if openErr := wt.OpenInApp(resolved.Cmd, wtPath, ctx.RepoName, finalName); openErr != nil {
						wt.Warn("could not open in %s: %s", resolved.Name, openErr)
					}
				}
			}

			// Success — disarm rollback (a no-op on the open-anyway path,
			// where rb was already disarmed in the init-failure branch — the
			// worktree was always kept).
			rb.Disarm()
			// Rollback is over: a late SIGINT/SIGTERM must not run the rollback
			// handler (or print the 130 document) for a worktree that is kept.
			signal.Stop(sigCh)

			// Init-failure override: when the interactive open-anyway path
			// fell through to Open, the process MUST still exit
			// ExitInitFailed — a successful open must NOT downgrade the exit
			// to 0 and erase the init-failure signal operators depend on.
			// This is the single exit point for the open-anyway path; it runs
			// regardless of whether the open succeeded or the user chose No.
			if initFailed {
				exitInitFailed(initFailedErr)
			}

			// Output the worktree path as the last line. This runs on every
			// success path — including an open_here launch, whose unified
			// NavigateTo emission already printed the same path (the stdout
			// contract is "the worktree path is always the last line", not
			// "exactly one line"). The former open_here suppression special
			// case is retired with the unified shell-cd contract.
			if jsonOut {
				printSuccess(receipt)
				return nil
			}
			fmt.Println(wtPath)
			return nil
		},
	}

	// --name / -n is primary; --worktree-name is the deprecated alias bound to
	// the same variable (same type, so a shared pointer is correct).
	cmd.Flags().StringVarP(&worktreeName, "name", "n", "", "Set worktree name (skips name prompt)")
	cmd.Flags().StringVar(&worktreeName, "worktree-name", "", "Set worktree name (skips name prompt)")
	cmd.Flags().MarkDeprecated("worktree-name", "use --name instead")
	// --open / -o is primary; --worktree-open is the deprecated alias.
	// --open deliberately has NO NoOptDefVal: bare `--open code` would parse
	// `code` as the positional [branch] argument (a silent footgun), so a value
	// is always required.
	cmd.Flags().StringVarP(&worktreeOpen, "open", "o", "", "After creation: prompt (menu), default (auto-detect app), skip, or an app name (e.g. code, cursor)")
	cmd.Flags().StringVar(&worktreeOpen, "worktree-open", "", "After creation: prompt (menu), default (auto-detect app), skip, or an app name (e.g. code, cursor)")
	cmd.Flags().MarkDeprecated("worktree-open", "use --open instead")
	// --no-init is a real bool superseding the deprecated string --worktree-init
	// (reconciled via Changed() in RunE — the types differ so they cannot share
	// a variable).
	cmd.Flags().BoolVar(&noInit, "no-init", false, "Skip the worktree init script (init runs by default)")
	cmd.Flags().StringVar(&worktreeInit, "worktree-init", "", "Run worktree init script: true (default) or false")
	cmd.Flags().MarkDeprecated("worktree-init", "use --no-init instead")
	cmd.Flags().BoolVar(&reuse, "reuse", false, "Reuse existing worktree if name collides (requires --name; not with --base or --fetch)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "No prompts; use defaults and skip menus")
	cmd.Flags().StringVar(&base, "base", "", "Git ref (branch, tag, SHA) to use as start-point for new branch (not with --checkout, --reuse, or --temp)")
	cmd.Flags().StringVar(&temp, "temp", "", "Create a detached throwaway worktree at <ref> (no branch; name prefixed tmp-; init skipped unless --init)")
	cmd.Flags().BoolVar(&initFlag, "init", false, "Run the worktree init script for --temp (skipped by default for temp worktrees)")
	cmd.Flags().BoolVar(&fetch, "fetch", false, "Fetch the base's remote (first --base segment if it is a remote, else origin) before resolving the base")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print one JSON receipt document on stdout (implies --non-interactive and --open skip)")
	cmd.Flags().StringVar(&checkout, "checkout", "", "Check out an EXISTING branch (local or remote) into the worktree instead of creating a new one")
	cmd.Flags().StringVar(&pr, "pr", "", "Create the worktree on pull request <n>'s head branch (resolved via gh; same-repository PRs only)")

	return cmd
}

// checkModeConflicts exits ExitInvalidArgs when --temp is combined with a flag
// or argument that selects a different creation mode, or when --init and
// --no-init contradict each other. It runs before any git work so a rejected
// invocation creates nothing. fail is create's failure exit (stderr error plus
// the --json failure document).
func checkModeConflicts(fail func(code int, what, why, fix string), tempMode bool, branchArg, checkout string, reuse bool, base string, prSet, initFlag, noInit bool) {
	if initFlag && noInit {
		fail(wt.ExitInvalidArgs,
			"--init cannot be combined with --no-init",
			"--init opts in to the init script for --temp; --no-init skips it",
			"Pass one of --init or --no-init")
	}
	if !tempMode {
		return
	}
	switch {
	case branchArg != "":
		fail(wt.ExitInvalidArgs,
			"--temp cannot be combined with a positional branch argument",
			"--temp creates a detached worktree with no branch; the positional names a new branch",
			"Use one of: wt create <new-branch> | wt create --temp <ref>")
	case checkout != "":
		fail(wt.ExitInvalidArgs,
			"--temp cannot be combined with --checkout",
			"--temp creates a detached worktree; --checkout puts a worktree on an existing branch",
			"Use one of: wt create --checkout <branch> | wt create --temp <ref>")
	case reuse:
		fail(wt.ExitInvalidArgs,
			"--temp cannot be combined with --reuse",
			"A temp worktree is always freshly created; --reuse returns an existing one",
			"Drop --reuse, or reuse by name without --temp: wt create --reuse --name <name>")
	case base != "":
		fail(wt.ExitInvalidArgs,
			"--temp cannot be combined with --base",
			"--temp's <ref> is already the commit to detach at; --base is the start-point for a NEW branch",
			"Pass the commit to --temp directly: wt create --temp <ref>")
	case prSet:
		fail(wt.ExitInvalidArgs,
			"--temp cannot be combined with --pr",
			"--pr checks out the PR's head branch; --temp detaches at a commit",
			"For a detached worktree at a PR head, pass its SHA: wt create --temp <sha>")
	}
}

// jsonDocGuard lets exactly one `--json` document be printed per run.
type jsonDocGuard struct {
	mu   sync.Mutex
	done bool
}

// emit prints doc (one line on stdout) and reports true, unless a document was
// already emitted, in which case it prints nothing and reports false.
func (g *jsonDocGuard) emit(doc string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.done {
		return false
	}
	g.done = true
	fmt.Println(doc)
	return true
}
