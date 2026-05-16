# Intake: Improve DX of `wt create` when init script fails

**Change**: 260516-g5e7-wt-create-init-failure-dx
**Created**: 2026-05-16
**Status**: Draft

## Origin

This change came from a `/fab-discuss` session followed by `/fab-proceed`. The conversation surveyed the current failure path of `wt create` when the worktree init script (default `fab sync`, overridable via `WORKTREE_INIT_SCRIPT`) exits non-zero, and identified five interlocking DX problems. The user explicitly confirmed:

> "This is just the new default."

Meaning: no opt-in flag, no backward-compat toggle. The current rollback-on-init-failure behavior is being treated as a bug fix, not a feature. All five recommendations land together as one change so the resulting failure path is internally consistent (kept-worktree + helpful banner + new exit code + restored signal semantics + unified resolver all reinforce one another).

The five problems identified in discussion:

1. **Destroyed worktree on init failure**: `create.go` lines 237 and 244 call `rb.Execute()` then `wt.ExitWithError(...)`, nuking the worktree even though every git operation (branch creation, `git worktree add`, optional fetch) succeeded. Only the user-supplied init script failed.
2. **Useless error banner**: User sees `Error: Init script failed / Why: exit status 1 / Fix: Check the init script for errors`. The "Why" is just `(*exec.ExitError).Error()` text. The real diagnostic output streamed to terminal during init may already be out of scrollback. The "Fix" provides no path, no rerun command, no remove hint.
3. **Duplicated init-protocol resolver**: `init.go:36-104` (`runInitScript`) and `crud.go:122-160` (`RunWorktreeSetup`) each independently parse the init-script string, do command-vs-path detection, do PATH/file existence checks. The two paths disagree on user-visible behavior — `wt init` prints a verbose, helpful "not found" message; `RunWorktreeSetup` returns `nil` silently.
4. **Exit code collision**: Init failure shares `ExitGeneralError(1)` with unrelated errors (not-a-git-repo, menu error, stash failure). Shell wrappers and operators (fab-kit, `hop`) can't programmatically distinguish "worktree exists, init didn't complete" from any other generic failure.
5. **SIGINT destroys worktree**: The signal handler in `create.go:80-86` always calls `rb.Execute()`. During the git-operations phase this is correct. After the worktree is built and init is running, Ctrl-C likely means "stop this script, I'll fix it" not "nuke everything I just created."

## Why

**Problem.** Today, when `wt create <branch>` (or exploratory `wt create`) runs the init script and the script exits non-zero, the user loses their worktree, their branch, and any fetched refs — and gets a generic two-line error pointing them nowhere. The git operations all succeeded; only the user-supplied init script failed. From the user's perspective, `wt create` is destroying their work in response to a problem in their own script. The fix often is "fix one typo and retry," but there's nothing left to retry against.

**Consequence if unfixed.** Users hit this whenever their `fab sync` (or custom `WORKTREE_INIT_SCRIPT`) has a transient failure: missing dependency, network blip, stale credential, typo in a recently-edited init script. Each failure forces them to re-run `wt create`, re-fetch the branch, re-do anything they cancelled the init to fix. The current error banner actively misleads — the "Why: exit status 1" line implies that's the useful diagnostic, when in fact the diagnostic was streamed above and may already be lost.

**Why this approach over alternatives.**

- **Why not add an opt-in flag (`--keep-on-init-failure`)?** Discussed and rejected. The current behavior is being treated as a bug, not a configurable preference. Flags create migration burden and split the user base. No one will ever say "actually, I prefer my just-created worktree to be destroyed when my init script has a typo."
- **Why bundle all five changes instead of landing them incrementally?** They are mutually reinforcing. Keeping the worktree (#1) without the new banner (#2) leaves users staring at a useless error with no rerun hint. The new banner (#2) without the unified resolver (#3) means `wt init` and `wt create`'s init step still diverge in not-found behavior. The exit code (#4) is the operator-facing complement to the banner. SIGINT (#5) is the same fix as #1 applied to a different trigger. A partial landing would create transitional states where the failure path is inconsistent.
- **Why a separate exit code rather than overloading `ExitGitError`?** Init failure is semantically distinct from git failure — by the time init runs, all git operations succeeded. Operators want to detect "worktree exists, init didn't complete" and offer a "retry init" affordance. Constitution Principle III ("Typed Exit Codes") directly calls this out.
- **Why unify the resolver rather than just patching the message in `RunWorktreeSetup`?** Two implementations of the same protocol drift over time. The init-protocol spec (`docs/specs/init-protocol.md`) describes a single contract; the code should expose it through a single function. Patching `RunWorktreeSetup` separately preserves the duplication and the latent drift risk.

## What Changes

### 1. Keep the worktree on init failure (no rollback on init-script exit)

**File**: [src/cmd/wt/create.go](../../../src/cmd/wt/create.go), lines 230-249.

On init-script non-zero exit (both the auto-init branch at line 235 and the prompted branch at line 243): **disarm the rollback** (`rb.Disarm()`) instead of executing it. Print the new structured banner (see #2). Exit with the new code `ExitInitFailed` (see #4). The worktree directory, the branch, and any fetched refs all survive.

Before (current):
```go
if err := wt.RunWorktreeSetup(wtPath, "force", initScript, ctx.RepoRoot); err != nil {
    // Init failure triggers rollback — must execute before os.Exit
    rb.Execute()
    wt.ExitWithError(wt.ExitGeneralError, "Init script failed", err.Error(),
        "Check the init script for errors")
}
```

After (target):
```go
if err := wt.RunWorktreeSetup(wtPath, "force", initScript, ctx.RepoRoot); err != nil {
    rb.Disarm()
    wt.PrintInitFailureBanner(wtPath, finalName, err)  // exact API up to spec
    os.Exit(wt.ExitInitFailed)
}
```

Both the `nonInteractive || branchArg == ""` branch (line 235) and the prompted branch (line 243) follow the same pattern.

The `defer rb.Execute()` at line 87-89 still fires on any other failure path between worktree creation and successful init — those paths still rollback. Only init-script failure is exempt.

### 2. New init-failure banner

**File**: [src/internal/worktree/errors.go](../../../src/internal/worktree/errors.go) — add a new helper, e.g. `PrintInitFailureBanner(wtPath, name string, err error)`.

Replace:
```
Error: Init script failed
  Why: exit status 1
  Fix: Check the init script for errors
```

With a banner shaped roughly like:
```
Error: Init script exited with status N (output is above)
  Worktree:  <wtPath>  (kept — git operations succeeded)
  Retry:     cd <wtPath> && wt init
  Remove:    wt delete <name>
```

Behavioral requirements:
- The exit status (`N`) extracted from the underlying `*exec.ExitError` when available; fall back to a generic phrase if `errors.As` doesn't unwrap to `ExitError` (e.g., kill signal, IO error before exec).
- The worktree path is shown absolute (no `~` expansion games).
- The retry command uses `cd <path> && wt init` rather than `(cd <path>; wt init)` to keep it copy-paste-friendly across `bash`/`zsh`/`fish`.
- The remove hint uses the worktree name (which `wt delete` resolves), not the full path.
- Uses the existing color helpers (`ColorRed`, `ColorBold`, `ColorReset`) for consistency with `WtError`.
- Exact text wording is a Confident-level decision — the spec should describe shape and intent, not pixel-perfect strings. Tests should assert presence of the path, the retry hint, and the remove hint, not exact byte equality.

### 3. Unify the init-protocol resolver

**New function** in `src/internal/worktree/` (file choice up to spec — likely a new `init.go` or extend `crud.go`):

```go
// ResolveInitInvocation returns either a runnable *exec.Cmd, or a
// structured "not found" reason if the init script cannot be located.
// Callers decide how to surface the not-found case.
func ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error)
```

Where `InitNotFound` is a small struct distinguishing:
- **CommandNotOnPath**: `Name` (e.g., `"fab"`) — the first whitespace-separated token wasn't found by `exec.LookPath`.
- **FileNotFound**: `Path` (resolved absolute path), `RelPath` (the value as provided) — the file path didn't exist.

Both [src/cmd/wt/init.go:36-104](../../../src/cmd/wt/init.go) (`runInitScript`) and [src/internal/worktree/crud.go:122-160](../../../src/internal/worktree/crud.go) (`RunWorktreeSetup`) consume this resolver.

**Unified not-found messaging**: The verbose `wt init`-style message (current `init.go:73-75` for command-not-on-path, and `init.go:82-87` for file-not-found) becomes the canonical message printed from **both** call sites. The current silent-skip behavior in `RunWorktreeSetup` (`crud.go:133, 139`) is removed — when `wt create` runs init and the script isn't found, the user sees the same helpful warning as if they'd run `wt init` directly.

`RunWorktreeSetup` becomes thin: call resolver → if `notFound`, print warning and return nil → confirm prompt (if `mode != "force"`) → set `cmd.Dir` to `wtPath`, wire streams, run.

### 4. New exit code `ExitInitFailed`

**File**: [src/internal/worktree/errors.go](../../../src/internal/worktree/errors.go).

Add the constant:
```go
ExitInitFailed = 7
```

Slotted after the current last constant (`ExitTmuxWindowError = 6`). No reordering, no renumbering — just append.

**Spec update**: [docs/specs/init-protocol.md](../../../docs/specs/init-protocol.md), the "Script failure semantics" section (current lines 78-86). Replace the current paragraph (which says "maps to `ExitGeneralError` (1)" and notes the rollback) with the new contract: failure exits with `ExitInitFailed (7)`, the worktree is kept, the user is shown retry/remove hints, and the rollback is disarmed.

### 5. SIGINT during init kills only the init process

**File**: [src/cmd/wt/create.go](../../../src/cmd/wt/create.go), lines 78-89 (signal handler) and lines 230-249 (init invocation).

The signal handler today is unconditional: it always calls `rb.Execute()` on `SIGINT`/`SIGTERM` and exits 130. Behavior needs to fork based on lifecycle phase:

- **Phase A — git operations** (everything from line 91 through the successful return of `CreateExploratoryWorktree` or `CreateBranchWorktree`): current behavior. SIGINT triggers `rb.Execute()`, worktree is removed, exit 130.
- **Phase B — init step** (after the worktree is built, while `RunWorktreeSetup` is running): SIGINT delivers the signal to the init child process group (so the script's children also stop), waits for the child to exit, then routes the exit through the same "worktree kept" code path as a natural init failure (banner from #2, exit code from #4). The rollback is disarmed.

Two viable implementations:

- **Option A: Phase flag + shared cmd reference.** A `currentInitCmd *exec.Cmd` variable (guarded by a mutex), set by `RunWorktreeSetup` via callback or returned-handle, read by the signal handler. The signal handler checks `if currentInitCmd != nil { signal child } else { rb.Execute() }`.
- **Option B: Reinstall handler after worktree creation.** After git operations succeed, `signal.Reset(SIGINT, SIGTERM)` and reinstall a different handler that kills the init child and falls through to the init-failure code path.

Both are fine — pick one in the spec. Option B is cleaner but requires careful ordering (any panic between worktree creation and reinstall would leave the worktree without rollback). Option A is more defensive but adds shared mutable state.

**Note on testability**: SIGINT tests are awkward — they need a subprocess (the test cannot send SIGINT to itself without affecting `go test`'s own process). The spec should acknowledge this and either (a) accept a manual-test note in the spec rather than an automated test, or (b) write an integration test that uses `cmd.Process.Signal(syscall.SIGINT)` against the spawned `wt` binary. Plan stage decides.

## Affected Memory

This repo's `docs/memory/` is empty (memory index has no domain rows). No memory file updates required. Spec doc updates (under `docs/specs/`) are tracked in **Impact** below.

## Impact

**Code surface**:

- [src/cmd/wt/create.go](../../../src/cmd/wt/create.go) — disarm rollback on init failure (lines 235-239, 243-247); call new banner helper; use `ExitInitFailed`; SIGINT rework (lines 78-89).
- [src/cmd/wt/init.go](../../../src/cmd/wt/init.go) — refactor `runInitScript` to consume `ResolveInitInvocation`; not-found messaging moves into the resolver path.
- [src/internal/worktree/crud.go](../../../src/internal/worktree/crud.go) — extract the resolver out of `RunWorktreeSetup`; make `RunWorktreeSetup` a thin wrapper (confirm prompt + streaming exec); not-found warning routes through the unified path.
- [src/internal/worktree/errors.go](../../../src/internal/worktree/errors.go) — add `ExitInitFailed = 7`; add `PrintInitFailureBanner(wtPath, name string, err error)` helper (or equivalent name).
- (Possibly) **new file** under `src/internal/worktree/` for `ResolveInitInvocation` + `InitNotFound` if `crud.go` becomes too dense — spec decides.

**Spec surface**:

- [docs/specs/init-protocol.md](../../../docs/specs/init-protocol.md) — rewrite "Script failure semantics" section. Document the new exit code, kept-worktree semantics, retry/remove hints, and SIGINT-during-init behavior. Mention the unified resolver as the single resolution contract for both `wt init` and `wt create`'s init step.

**Test surface**:

- [src/cmd/wt/create_test.go](../../../src/cmd/wt/create_test.go) — new tests:
  - `TestCreate_InitFailureKeepsWorktree` (existing-branch + exploratory variants): point `WORKTREE_INIT_SCRIPT` at a script that exits 1; assert the worktree directory still exists, the branch still exists, and exit code is `ExitInitFailed`.
  - `TestCreate_InitFailureBannerHasRetryHint`: assert stderr contains the worktree path and the `wt init` retry hint.
- [src/cmd/wt/init_test.go](../../../src/cmd/wt/init_test.go) — adjust if the resolver refactor changes test seams (likely small).
- [src/cmd/wt/integration_test.go](../../../src/cmd/wt/integration_test.go) — end-to-end: failing init script via `WORKTREE_INIT_SCRIPT` pointing at a script exiting 1; assert worktree dir + branch survive; assert exit code `7` (`ExitInitFailed`).
- SIGINT test: best-effort. May require a separate fixture or be deferred to manual testing — plan stage decides.

**No backward-compat shim**. No flag, no env var to restore the old behavior. The change is a default-behavior fix.

**Constitution alignment**:

- **Principle II (Cobra Command Surface)**: no new subcommand, no new long-form flag introduced.
- **Principle III (Typed Exit Codes)**: directly served by `ExitInitFailed`.
- **Principle IV (Test What the User Sees)**: integration test for end-to-end init-failure-keeps-worktree is required.
- **Principle V (Internal Package Boundary)**: the resolver and banner helpers live in `internal/worktree/`; `cmd/wt/create.go` only orchestrates.

## Open Questions

- Exact wording and ordering of the banner lines (Confident — spec picks).
- Resolver function name (`ResolveInitInvocation` is a working name; spec may rename).

> Resolved during /fab-clarify (2026-05-16): `InitNotFound` is a typed struct with Kind field (assumption #11); SIGINT uses Option B reinstall handler (assumption #9); SIGINT has an automated integration test (assumption #10). See ## Clarifications below.

## Clarifications

### Session 2026-05-16

| # | Question | Answer |
|---|----------|--------|
| 9 | SIGINT implementation: Option B (reinstall handler) vs Option A (phase flag + shared cmd ref)? | Option B — reinstall handler after worktree creation. Spec/plan stage must handle the small reinstall ordering window carefully. |
| 10 | SIGINT integration test: automated subprocess-signal test vs manual-only with documented note? | Automated integration test via `cmd.Process.Signal(syscall.SIGINT)` against the spawned `wt` binary, with a slow init script. Plan stage adds generous timeouts and may `t.Skip` on Windows. |
| 11 | `InitNotFound` shape: typed struct with Kind field vs sentinel errors (`errors.Is`)? | Typed struct with `Kind` discriminator (`CommandNotOnPath` / `FileNotFound`) and contextual fields (`Name`, `Path`, `RelPath`). Rendering at the two call sites becomes a single switch. |

### Session 2026-05-16 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 5 | Confirmed | Banner shape (status + kept marker + retry + remove). Exact wording at spec. |
| 6 | Confirmed | Resolver name `ResolveInitInvocation` + signature `(*exec.Cmd, *InitNotFound, error)`. |
| 7 | Confirmed | Unified verbose not-found warning across `wt init` and `wt create`'s init step. |
| 8 | Confirmed | `wt create --reuse` keeps warn-but-continue semantics; doesn't adopt `ExitInitFailed`. |
| 12 | Confirmed | Integration test for failing init script + unit tests are mandatory per Constitution IV. |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Change type is `refactor` (default-behavior fix that formalizes a duplicated protocol implementation, no new user verb, no new command). The `/fab-new` taxonomy may classify this as `fix` instead based on keyword matching — both defensible, no behavior change. | Discussed — user described it as "treating the current behavior as a bug" but the scope formalizes existing protocol (#3 resolver unification) and adds typed semantics (#4 exit code), making `refactor` the better taxonomy fit. | S:90 R:90 A:85 D:80 |
| 2 | Certain | No opt-in flag, no backward-compat env var. Current rollback-on-init-failure behavior is replaced wholesale. | Discussed — user explicitly said "This is just the new default." | S:100 R:80 A:95 D:95 |
| 3 | Certain | All five recommendations land as a single change, not split across multiple changes. | Discussed — recommendations are mutually reinforcing (banner depends on kept-worktree; exit code is the operator complement to banner; unified resolver eliminates drift between init.go and crud.go; SIGINT is the same fix as #1 for a different trigger). | S:95 R:85 A:90 D:90 |
| 4 | Certain | New exit code is `ExitInitFailed = 7`, appended after `ExitTmuxWindowError = 6`. No renumbering of existing codes. | Existing codes in `errors.go` are stable and referenced by shell wrappers; appending is the only safe option. | S:100 R:60 A:100 D:100 |
| 5 | Certain | Banner shape is: status line with extracted exit code → kept-worktree marker → retry hint (`cd <path> && wt init`) → remove hint (`wt delete <name>`). Exact wording finalized at spec stage. | Clarified — user confirmed. | S:95 R:90 A:85 D:75 |
| 6 | Certain | Resolver function is named `ResolveInitInvocation` (working name) and returns `(*exec.Cmd, *InitNotFound, error)` or equivalent. Spec may rename. | Clarified — user confirmed. | S:95 R:90 A:80 D:70 |
| 7 | Certain | `RunWorktreeSetup`'s current silent-skip-on-not-found behavior is replaced by the same verbose warning `wt init` shows today. `wt create`'s init step and `wt init` print identical messages for the same not-found condition. | Clarified — user confirmed. | S:95 R:80 A:90 D:80 |
| 8 | Certain | `wt create --reuse` path (line 184-193) keeps its current "warn-but-continue on init failure" semantics (does not adopt the new `ExitInitFailed` exit). Rationale: a reused worktree is presumed functional pre-existing; init is a refresh, not a blocker. | Clarified — user confirmed. | S:95 R:80 A:90 D:85 |
| 9 | Certain | SIGINT implementation uses **Option B** (reinstall handler after worktree creation) rather than Option A (phase flag + shared cmd reference). Cleaner control flow, no shared mutable state across goroutines. Spec/plan stage MUST handle the reinstall ordering window carefully. <!-- clarified: user confirmed Option B during /fab-clarify --> | Clarified — user confirmed Option B (reinstall handler) over Option A (phase flag). | S:95 R:70 A:60 D:55 |
| 10 | Certain | SIGINT-during-init has an automated integration test (using `cmd.Process.Signal(syscall.SIGINT)` against the spawned `wt` binary, with a slow init script as the sentinel sleep). Plan stage adds generous timeouts and may `t.Skip` on Windows. <!-- clarified: user confirmed automated test during /fab-clarify --> | Clarified — user confirmed automated test over manual-only note. Aligns with Constitution Principle IV ("Test What the User Sees"). | S:95 R:80 A:50 D:50 |
| 11 | Certain | `InitNotFound` is a typed struct with `Kind` field (`CommandNotOnPath` / `FileNotFound`) and contextual fields (`Name`, `Path`, `RelPath`) rather than sentinel errors. Message rendering at the two call sites is a single switch on `Kind`. <!-- clarified: user confirmed typed struct during /fab-clarify --> | Clarified — user confirmed typed struct over sentinel errors. Simpler for the dual-call-site message rendering. | S:95 R:75 A:55 D:55 |
| 12 | Certain | The integration test for failing init script (in `integration_test.go`) is mandatory per Constitution Principle IV. The unit tests in `create_test.go` are also mandatory. | Clarified — user confirmed. | S:95 R:85 A:95 D:90 |

12 assumptions (12 certain, 0 confident, 0 tentative, 0 unresolved).
