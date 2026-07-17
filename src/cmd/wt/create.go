package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	wt "github.com/sahil87/wt/internal/worktree"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
branch argument or with --base.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var branchArg string
			if len(args) > 0 {
				branchArg = args[0]
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

			// Validate --reuse requires --name
			if reuse && worktreeName == "" {
				wt.PrintError(
					"--reuse requires --name",
					"--reuse only works with an explicit worktree name",
					"Example: wt create --reuse --name my-feature branch-name")
				os.Exit(wt.ExitInvalidArgs)
			}

			// --checkout selects an existing branch; the positional creates a
			// new one. They are mutually exclusive modes, as are --checkout and
			// --base (--base is the start-point for a NEW branch only).
			if checkout != "" && branchArg != "" {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--checkout cannot be combined with a positional branch argument",
					"The positional creates a new branch; --checkout checks out an existing one",
					"Use one of: wt create <new-branch> | wt create --checkout <existing-branch>")
			}
			if checkout != "" && base != "" {
				wt.ExitWithError(wt.ExitInvalidArgs,
					"--base cannot be combined with --checkout",
					"--base is the start-point for a NEW branch; --checkout targets an existing branch",
					"Drop --base, or create a new branch: wt create <name> --base <ref>")
			}

			// Validate git repo
			if err := wt.ValidateGitRepo(); err != nil {
				wt.ExitWithError(wt.ExitGitError,
					"Not a git repository",
					"This command requires a git repository",
					"Navigate to a git repository and try again")
			}

			ctx, err := wt.GetRepoContext()
			if err != nil {
				wt.ExitWithError(wt.ExitGeneralError,
					"Not a git repository",
					"This command must be run from within a git repository",
					"Navigate to a git repository and try again")
			}

			// Set up rollback and signal handling
			rb := wt.NewRollback()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Println()
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
					wt.ExitWithError(wt.ExitInvalidArgs,
						"Invalid branch name",
						fmt.Sprintf("Branch name '%s' contains invalid characters", branchToValidate),
						"Use alphanumeric characters, hyphens, and single slashes")
				}
			}

			// Validate --base ref whenever it is set and --reuse is not.
			// The positional is always a NEW branch now, so --base always
			// applies to it; only --reuse short-circuits before --base is used
			// (so `wt create --reuse --base <bad>` must not fail on the ref).
			// (--base + --checkout was already rejected above.)
			if base != "" && !reuse {
				if err := exec.Command("git", "rev-parse", "--verify", base).Run(); err != nil {
					wt.ExitWithError(wt.ExitInvalidArgs,
						fmt.Sprintf("Invalid --base ref: %s", base),
						fmt.Sprintf("'%s' does not resolve to a valid git object", base),
						"Provide a valid branch name, tag, or commit SHA")
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
					wt.ExitWithError(wt.ExitGeneralError, "Menu error", err.Error(), "")
				}
				switch choice {
				case 1: // continue
				case 2:
					stashID, err := wt.StashCreate("wt-create: pre-creation stash")
					if err != nil {
						wt.ExitWithError(wt.ExitGeneralError,
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
			case checkout != "":
				suggestedName = wt.DeriveWorktreeName(checkout)
			case branchArg != "":
				suggestedName = wt.DeriveWorktreeName(branchArg)
			default:
				suggestedName, err = wt.GenerateUniqueName(ctx.WorktreesDir, 10)
				if err != nil {
					wt.ExitWithError(wt.ExitRetryExhausted,
						"Could not find unique worktree name",
						"All 10 random name attempts collided with existing worktrees",
						fmt.Sprintf("Remove some worktrees from %s or increase retries", ctx.WorktreesDir))
				}
			}

			// Resolve final name
			var finalName string
			if worktreeName != "" {
				finalName = worktreeName
			} else if nonInteractive {
				finalName = suggestedName
			} else {
				finalName = session.PromptWithDefault("Worktree name", suggestedName)
			}

			// Check collision
			if wt.CheckNameCollision(ctx.WorktreesDir, finalName) {
				if reuse {
					fmt.Fprintf(os.Stderr, "Reusing existing worktree: %s\n", finalName)
					rb.Disarm()
					existingWtPath := filepath.Join(ctx.WorktreesDir, finalName)
					// Run init script on reuse — ensures skills are current even in existing worktrees.
					// Non-fatal: reuse proceeds even if init fails (existing worktree may be functional).
					if worktreeInit == "true" {
						initScript, isDefault := wt.InitScriptPath()
						if err := wt.RunWorktreeSetup(existingWtPath, initScript, isDefault, ctx.RepoRoot); err != nil {
							wt.Warn("worktree init failed for reused worktree %q: %v", finalName, err)
						}
					}
					fmt.Println(existingWtPath)
					return nil
				}
				wt.ExitWithError(wt.ExitGeneralError,
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
			// ExitInvalidArgs pointing at create-new). The existence rules live
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
			//   --checkout → fixed copy naming the existing-branch path (no query)
			//   --base     → the ref verbatim as typed (no query)
			//   else       → the resolved HEAD label (one query, best-effort)
			var createdSummaryFrom string
			switch {
			case checkout != "":
				createdSummaryFrom = fmt.Sprintf("existing branch '%s' (checked out directly)", checkout)
			case base != "":
				createdSummaryFrom = base
			default:
				createdSummaryFrom = wt.DescribeHead()
			}

			switch {
			case checkout != "":
				wtPath, err = wt.CheckoutBranchWorktree(checkout, finalName, ctx, rb)
				if err != nil {
					if errors.Is(err, wt.ErrBranchNotFound) {
						wt.ExitWithError(wt.ExitInvalidArgs,
							fmt.Sprintf("Branch '%s' not found", checkout),
							"--checkout requires an existing local or remote branch",
							fmt.Sprintf("To create a new branch: wt create %s [--base <ref>]", checkout))
					}
					wt.ExitWithError(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"The branch may already be checked out in another worktree")
				}
				createdSummaryBranch = checkout
			case branchArg != "":
				wtPath, err = wt.CreateNewBranchWorktree(branchArg, finalName, ctx, rb, base)
				if err != nil {
					if errors.Is(err, wt.ErrBranchExists) {
						wt.ExitWithError(wt.ExitInvalidArgs,
							fmt.Sprintf("Branch '%s' already exists", branchArg),
							"The positional argument only creates new branches",
							fmt.Sprintf("To put a worktree on the existing branch: wt create --checkout %s", branchArg))
					}
					wt.ExitWithError(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"The branch may already be checked out in another worktree")
				}
				createdSummaryBranch = branchArg
			default:
				wtPath, err = wt.CreateExploratoryWorktree(finalName, ctx, rb, base)
				if err != nil {
					wt.ExitWithError(wt.ExitGitError, "Failed to create worktree", err.Error(),
						"Check if the branch already exists or if there are permission issues")
				}
				createdSummaryBranch = finalName
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
			//   4. Run init via RunWorktreeSetupWithObserver (no prompt
			//      inside the runner).
			// The confirm fires whenever a branch was explicitly named —
			// positional (new branch) or --checkout (existing branch) — and is
			// skipped for the bare exploratory create.
			runInit := worktreeInit == "true"
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

				initErr := wt.RunWorktreeSetupWithObserver(wtPath, initScript, isDefault, ctx.RepoRoot, captureInit)

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
						os.Exit(wt.ExitInitFailed)
					}
				}
			}

			// Open
			//
			// The Open separator is emitted only when the open phase will
			// actually run — never for the skip case (incl. --non-interactive
			// defaulting to skip), since a separator must not precede a phase
			// that emits nothing.
			var suppressPath bool
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
						if selected.Cmd == "open_here" {
							suppressPath = true
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
					if resolved.Cmd == "open_here" {
						suppressPath = true
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
					if resolved.Cmd == "open_here" {
						suppressPath = true
					}
				}
			}

			// Success — disarm rollback (a no-op on the open-anyway path,
			// where rb was already disarmed in the init-failure branch — the
			// worktree was always kept).
			rb.Disarm()

			// Init-failure override: when the interactive open-anyway path
			// fell through to Open, the process MUST still exit
			// ExitInitFailed — a successful open must NOT downgrade the exit
			// to 0 and erase the init-failure signal operators depend on.
			// This is the single exit point for the open-anyway path; it runs
			// regardless of whether the open succeeded or the user chose No.
			if initFailed {
				os.Exit(wt.ExitInitFailed)
			}

			// Output the worktree path as the last line
			if !suppressPath {
				fmt.Println(wtPath)
			}
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
	cmd.Flags().BoolVar(&reuse, "reuse", false, "Reuse existing worktree if name collides (requires --name)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "No prompts; use defaults and skip menus")
	cmd.Flags().StringVar(&base, "base", "", "Git ref (branch, tag, SHA) to use as start-point for new branch")
	cmd.Flags().StringVar(&checkout, "checkout", "", "Check out an EXISTING branch (local or remote) into the worktree instead of creating a new one")

	return cmd
}
