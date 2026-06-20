# Spec: Improve DX of `wt create` when init script fails

**Change**: 260516-g5e7-wt-create-init-failure-dx
**Created**: 2026-05-16
**Affected memory**: _none — `docs/memory/index.md` has no domain rows; no memory files to update_

<!--
  Source intake: intake.md in this folder. The intake is fully clarified (12 Certain
  assumptions, 0 Confident/Tentative/Unresolved). This spec confirms each intake
  assumption and adds spec-level decisions for resolver placement, helper names,
  and signal-handler reinstallation ordering.

  Recorded change_type in .status.yaml is `fix`. The intake (assumption #1) notes
  the work is `refactor`-shaped (formalizes a duplicated protocol + adds typed
  exit code). Per the intake decision, the recorded change_type is NOT amended —
  this note exists so reviewers can locate the discrepancy if scoring thresholds
  ever matter.
-->

## Non-Goals

- **No opt-in `--keep-on-init-failure` flag**. The kept-worktree behavior is the unconditional new default; the rollback-on-init-failure path is deleted, not gated.
- **No flag conversion of `WORKTREE_INIT_SCRIPT`**. The env var remains the sole way to override the default `fab sync` invocation. No new long-form flag is introduced (Constitution Principle II).
- **No change to `wt create --reuse` init semantics**. The reuse path keeps its current "warn-but-continue on init failure" behavior and does NOT adopt `ExitInitFailed`. A reused worktree is presumed functional pre-existing; the init step there is a refresh, not a gate.
- **No backward-compat env var or shim** for restoring rollback-on-init-failure. The previous behavior is removed wholesale.
- **No reordering or renumbering of existing exit codes** in `internal/worktree/errors.go`. `ExitInitFailed` is appended after the current last constant.
- **No init-protocol semantic change**. The wire contract between `wt` and the init script (env vars passed, working directory, stdio streaming) is unchanged. Only the resolver code path and the failure-handling code path are touched.

## Init Invocation Resolution

### Requirement: Unified resolver function

The `internal/worktree` package SHALL expose a single function that resolves an init-script string into either a runnable `*exec.Cmd` or a structured "not found" reason. The function signature SHALL be:

```go
func ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error)
```

Both `cmd/wt/init.go` (the `wt init` subcommand) and `internal/worktree/crud.go` (the `wt create` init step) SHALL consume this function. No other code in the repo MAY duplicate the command-vs-path detection logic, the `exec.LookPath` check, or the file-existence check that this function performs. Duplicating this logic is an explicit anti-pattern under code-quality.md ("Duplicating existing utilities instead of reusing them").

Return-value contract:
- On successful resolution: `(*exec.Cmd, nil, nil)`. The returned `*exec.Cmd` SHALL have `Dir` left unset by the resolver (callers set the working directory because `wt init` and `wt create` use different directories); `Stdout`/`Stderr` left unset (callers wire streaming).
- On structured not-found: `(nil, *InitNotFound, nil)`. The not-found reason is a successful resolution outcome, not an error.
- On unexpected I/O or parsing error: `(nil, nil, error)`. Reserved for failures the caller cannot recover from (e.g., malformed init script string that cannot be tokenized).

#### Scenario: Command resolves via PATH

- **GIVEN** `WORKTREE_INIT_SCRIPT` is unset (so `initScript == "fab sync"`)
- **AND** `fab` is on the user's `PATH`
- **WHEN** `ResolveInitInvocation("fab sync", repoRoot)` is called
- **THEN** it returns a non-nil `*exec.Cmd` configured to run `fab sync`
- **AND** the `*InitNotFound` return value is `nil`
- **AND** the `error` return value is `nil`

#### Scenario: Command not on PATH

- **GIVEN** `initScript == "fab sync"`
- **AND** `fab` is NOT on `PATH`
- **WHEN** `ResolveInitInvocation` is called
- **THEN** it returns `(nil, &InitNotFound{Kind: CommandNotOnPath, Name: "fab"}, nil)`

#### Scenario: File path resolves

- **GIVEN** `initScript == "./scripts/init.sh"`
- **AND** the file exists at `<repoRoot>/scripts/init.sh`
- **WHEN** `ResolveInitInvocation` is called with that `repoRoot`
- **THEN** it returns a non-nil `*exec.Cmd` configured to execute that file
- **AND** the `*InitNotFound` return value is `nil`

#### Scenario: File path does not exist

- **GIVEN** `initScript == "./scripts/missing.sh"`
- **AND** no such file exists under `repoRoot`
- **WHEN** `ResolveInitInvocation` is called
- **THEN** it returns `(nil, &InitNotFound{Kind: FileNotFound, Path: "<absolute>/scripts/missing.sh", RelPath: "./scripts/missing.sh"}, nil)`

### Requirement: `InitNotFound` typed struct

The not-found reason SHALL be a typed struct (NOT a sentinel error usable with `errors.Is`). The struct SHALL include:

- A `Kind` field of an exported enumerated type with values `CommandNotOnPath` and `FileNotFound`. These two constants are the only valid `Kind` values.
- A `Name` field (string) populated when `Kind == CommandNotOnPath` — the first whitespace-separated token of the init-script string.
- A `Path` field (string) populated when `Kind == FileNotFound` — the resolved absolute path that was checked.
- A `RelPath` field (string) populated when `Kind == FileNotFound` — the path string as the user supplied it (so warning messages can echo it back literally).

Rationale (locked from intake assumption #11): downstream message rendering is a single `switch notFound.Kind` at each of the two call sites. Sentinel errors would force `errors.As` plus a type assertion per case, with no readability benefit. The struct also makes the contract explicit at the type level — callers cannot accidentally ignore a `Kind` value.

`Kind` MUST be defined as a named type (not an untyped string or int) so the Go compiler can flag unhandled cases when new kinds are added.

#### Scenario: CommandNotOnPath struct shape

- **GIVEN** the resolver returns `CommandNotOnPath` for `initScript == "fab sync"`
- **THEN** the returned struct has `Kind == CommandNotOnPath`, `Name == "fab"`, and `Path`/`RelPath` are empty strings

#### Scenario: FileNotFound struct shape

- **GIVEN** the resolver returns `FileNotFound` for `initScript == "./missing.sh"` and `repoRoot == "/tmp/repo"`
- **THEN** the returned struct has `Kind == FileNotFound`, `Path == "/tmp/repo/missing.sh"`, `RelPath == "./missing.sh"`, and `Name` is the empty string

### Requirement: Unified not-found warning

Both `wt init` and `wt create`'s init step SHALL print the **same** verbose, helpful warning when the resolver returns `*InitNotFound`. The message text SHALL be produced by a single rendering helper in `internal/worktree/` (e.g., `(InitNotFound).RenderWarning() string`) so the two call sites cannot drift.

For `Kind == CommandNotOnPath`, the warning SHALL include:
- The fact that the named command is not on `PATH`
- The command name (`Name`)
- A short hint that the user can either install the command, adjust `PATH`, or set `WORKTREE_INIT_SCRIPT` to a different invocation

For `Kind == FileNotFound`, the warning SHALL include:
- The fact that the file path does not exist
- The path as the user provided it (`RelPath`)
- The absolute path that was checked (`Path`)
- A short hint that the user can create the file or set `WORKTREE_INIT_SCRIPT` to a different invocation

The current silent-skip behavior at `internal/worktree/crud.go:133` and `:139` SHALL be removed. When `wt create`'s init step encounters a not-found init script, the user sees the same warning as if they had run `wt init` directly.

This warning is non-fatal: after printing, both call sites continue successfully (the init step is treated as a no-op, the worktree is kept, and `wt create` returns exit 0). The not-found warning does NOT trigger `ExitInitFailed`.

#### Scenario: `wt create` shows command-not-found warning

- **GIVEN** `WORKTREE_INIT_SCRIPT=missing-bin` and `missing-bin` is not on `PATH`
- **WHEN** the user runs `wt create newbranch`
- **THEN** the git operations succeed
- **AND** stderr contains the verbose "missing-bin is not on PATH" warning identical to what `wt init` prints under the same condition
- **AND** the command exits 0
- **AND** the worktree directory and branch are kept

#### Scenario: `wt init` shows file-not-found warning

- **GIVEN** `WORKTREE_INIT_SCRIPT=./does-not-exist.sh`
- **AND** the user is inside a worktree
- **WHEN** the user runs `wt init`
- **THEN** stderr contains the verbose "./does-not-exist.sh does not exist (checked <absolute>)" warning
- **AND** the command exits 0

#### Scenario: Warning text matches across call sites

- **GIVEN** the same `WORKTREE_INIT_SCRIPT` value that resolves to `*InitNotFound`
- **WHEN** the warning is rendered for `wt init` and for `wt create`'s init step
- **THEN** the rendered string is byte-identical between the two call sites (because both call the same rendering helper)

### Requirement: `RunWorktreeSetup` thinned

After this change, `internal/worktree/crud.go`'s `RunWorktreeSetup` SHALL contain only:

1. A call to `ResolveInitInvocation`
2. Not-found warning printing (via the unified helper) and an early return when `*InitNotFound` is non-nil
3. The interactive confirmation prompt (when `mode != "force"`)
4. Setting `cmd.Dir = wtPath` and wiring `cmd.Stdout`/`cmd.Stderr`/`cmd.Stdin` to the user terminal
5. `cmd.Run()` and returning its error verbatim

The function MUST NOT exceed 50 lines (code-quality.md anti-pattern: "God functions (>50 lines without clear reason)"). All command-vs-path detection, file-existence checking, and `exec.Cmd` construction SHALL live in `ResolveInitInvocation`, not in `RunWorktreeSetup`.

#### Scenario: RunWorktreeSetup delegates resolution

- **GIVEN** `RunWorktreeSetup(wtPath, "force", "fab sync", repoRoot)` is called
- **AND** `fab` is on PATH
- **WHEN** the function executes
- **THEN** the first non-trivial operation it performs is a call to `ResolveInitInvocation("fab sync", repoRoot)`
- **AND** the function does not itself call `exec.LookPath` or `os.Stat` on the init script

## Init Failure Handling

### Requirement: Worktree kept on init-script non-zero exit

When `wt create` runs the init script and the script exits non-zero (i.e., `RunWorktreeSetup` returns a non-nil error), the worktree directory, the branch, and any fetched refs SHALL be kept. The rollback SHALL be disarmed via `rb.Disarm()` (not `rb.Execute()`).

This applies to both code paths in `cmd/wt/create.go` that invoke `RunWorktreeSetup`:
- The auto-init branch (current line 235, taken when `nonInteractive || branchArg == ""`)
- The prompted branch (current line 243, taken in interactive mode after the user confirms init)

The existing `defer rb.Execute()` (current lines 87–89) MUST still fire on any other failure path between worktree creation and the init step — only init-script non-zero exit is exempt from rollback.

Init-script non-zero exit is distinct from init-script not-found (which is handled per "Unified not-found warning" above and is non-fatal). Only the case where the resolver succeeded, the command was executed, and the process exited with a non-zero status triggers this requirement.

#### Scenario: Failing init script in non-interactive mode

- **GIVEN** `WORKTREE_INIT_SCRIPT` points at a script that exits 1
- **WHEN** the user runs `wt create --non-interactive newbranch`
- **THEN** the worktree directory still exists on disk after the command completes
- **AND** the branch `newbranch` still exists in the repository
- **AND** the rollback function was NOT executed

#### Scenario: Failing init script in interactive mode (prompted)

- **GIVEN** `WORKTREE_INIT_SCRIPT` points at a script that exits 1
- **AND** the user runs `wt create` (no branch arg, interactive)
- **WHEN** the user confirms the init prompt and the script then fails
- **THEN** the worktree directory still exists
- **AND** the branch still exists
- **AND** the rollback was disarmed, not executed

#### Scenario: Non-init failure still triggers rollback

- **GIVEN** `wt create` is mid-flight after the worktree directory has been created
- **WHEN** the optional fetch step (between worktree creation and init invocation) fails for a reason unrelated to the init script
- **THEN** the rollback `defer rb.Execute()` fires
- **AND** the worktree is removed (current behavior preserved for non-init failures)

### Requirement: Init-failure banner

When the init script exits non-zero, `cmd/wt/create.go` SHALL print a structured banner to stderr via a new helper in `internal/worktree/errors.go` (or a sibling file in the same package). The helper signature SHALL be (or be equivalent to):

```go
func PrintInitFailureBanner(wtPath, name string, err error)
```

The banner SHALL contain, in order:

1. **A status line** that names the failure (init script) and includes the extracted exit code when available. The exit code SHALL be extracted from `err` via `errors.As(err, &exitErr)` against `*exec.ExitError`. When `errors.As` succeeds, the banner SHALL include the numeric exit code (e.g., `exited with status 2`). When `errors.As` does NOT succeed (e.g., the error is a kill-signal error or an I/O error from before exec completed), the banner SHALL use a generic phrase (e.g., `did not complete`) instead of a numeric status. The banner MUST also remind the user that the init output was streamed above (since by the time the banner appears, the script's stderr/stdout has already scrolled past).
2. **A kept-worktree marker line** showing the absolute worktree path (`wtPath`) and a short note that it was kept because the git operations succeeded. The path SHALL be rendered absolute with no `~`/`$HOME` expansion games.
3. **A retry hint line** showing the literal copy-paste-ready command `cd <wtPath> && wt init`. The `&&` chaining (not `;`) SHALL be used so the hint works identically in bash, zsh, and fish.
4. **A remove hint line** showing `wt delete <name>`, where `<name>` is the worktree name (not the absolute path) so it composes with `wt delete`'s name resolution.

The banner SHALL use the existing color helpers (`ColorRed`, `ColorBold`, `ColorReset` from `internal/worktree`) for visual consistency with `WtError`. No magic strings: any literal labels (e.g., the words for the status line, "Worktree:", "Retry:", "Remove:") SHALL be either inline string literals confined to this one helper OR named constants in the same file. They MUST NOT be duplicated in any other file.

Exact wording of the banner labels is intentionally not pinned in this spec — tests SHALL assert presence of the worktree path, the `wt init` retry hint, and the `wt delete <name>` remove hint, NOT byte-equality of the banner template. The plan stage MAY refine wording.

#### Scenario: Banner for `*exec.ExitError`

- **GIVEN** the init script exits with status 2
- **WHEN** the banner is printed
- **THEN** the banner contains the substring `status 2` (or equivalent rendering of the exit code)
- **AND** the banner contains the absolute worktree path
- **AND** the banner contains the substring `wt init`
- **AND** the banner contains the substring `wt delete <name>` where `<name>` is the worktree name

#### Scenario: Banner for non-ExitError failure

- **GIVEN** the init invocation fails with an error that does NOT unwrap to `*exec.ExitError` (e.g., the process was killed before exit)
- **WHEN** the banner is printed
- **THEN** the banner does NOT include a numeric exit status
- **AND** the banner still includes the worktree path, the retry hint, and the remove hint

#### Scenario: Retry hint is copy-paste safe across shells

- **GIVEN** any init failure banner
- **WHEN** a user copies the retry line
- **THEN** the command uses `&&` (not `;` or `(... ; ...)`) so it parses identically in bash, zsh, and fish

### Requirement: `ExitInitFailed = 7` exit code

`internal/worktree/errors.go` SHALL declare a new exit code constant:

```go
ExitInitFailed = 7
```

The constant SHALL be appended after the current last constant `ExitTmuxWindowError = 6`. No existing exit-code constants may be renumbered, reordered, or removed.

`cmd/wt/create.go` SHALL exit with `ExitInitFailed` (via `os.Exit(wt.ExitInitFailed)`) — NOT via `wt.ExitWithError(...)` with `ExitGeneralError` — when the init script exits non-zero. This satisfies Constitution Principle III ("Typed Exit Codes"): operators (shell wrappers, fab-kit, `hop`) can programmatically distinguish "worktree exists, init didn't complete" from any other generic failure.

The `--reuse` code path (current `cmd/wt/create.go` lines 184–193) SHALL NOT adopt `ExitInitFailed`. Per intake assumption #8, the reuse path keeps its existing warn-but-continue semantics; init failure during reuse is a refresh-failure, not a creation-failure, and operators do not need to distinguish it.

`docs/specs/init-protocol.md` SHALL be updated in the same change. The "Script failure semantics" section (current lines 78–86) SHALL be rewritten to document: the new exit code `ExitInitFailed = 7`, the kept-worktree semantics, the retry/remove hints, and the SIGINT-during-init behavior described below. The spec update SHALL also mention `ResolveInitInvocation` as the single resolution contract used by both `wt init` and `wt create`'s init step.

#### Scenario: Exit code on init failure

- **GIVEN** `WORKTREE_INIT_SCRIPT` points at a script that exits 1
- **WHEN** the user runs `wt create --non-interactive newbranch`
- **THEN** the `wt` process exits with status 7

#### Scenario: Exit code unchanged for `--reuse`

- **GIVEN** an existing worktree and `wt create --reuse <name>` is run
- **AND** the init script exits non-zero
- **THEN** the reuse path's existing behavior is preserved (warn-but-continue; exit code is NOT `ExitInitFailed`)

#### Scenario: Existing exit codes unchanged

- **GIVEN** the post-change `internal/worktree/errors.go`
- **THEN** `ExitGeneralError == 1`, `ExitUserAbort == 2`, ... `ExitTmuxWindowError == 6` are all unchanged
- **AND** `ExitInitFailed == 7` is the new last constant

## Signal Handling During Init

### Requirement: SIGINT during init kills only the init child

When the user sends SIGINT (or SIGTERM) **after** the worktree has been successfully created and **while** the init script is running, the signal SHALL be delivered to the init process group only. The worktree, branch, and fetched refs SHALL be kept. The signal SHALL then route through the same code path as a natural init-script failure: the banner from "Init-failure banner" is printed and the process exits with `ExitInitFailed`.

When the user sends SIGINT **during** the git-operations phase (everything from the start of `wt create` up to and including successful worktree creation), the current behavior is preserved: the rollback is executed and the process exits 130.

Implementation SHALL use **Option B from the intake**: after the worktree-creation phase succeeds and immediately before the init step starts, the existing signal handler is reset (`signal.Reset(syscall.SIGINT, syscall.SIGTERM)`) and a new handler is installed that:

1. Calls `cmd.Process.Signal(syscall.SIGINT)` (or equivalent) against the init `*exec.Cmd`, targeting the child's process group so the script's own children also receive the signal. On Unix, this requires `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` to be set when the cmd is constructed (the resolver MAY set this, or `RunWorktreeSetup` MAY set it just before `cmd.Run()`; the plan decides).
2. Waits for the child to exit (the existing `cmd.Run()` returns naturally once the child terminates).
3. Falls through to the natural init-failure code path: `rb.Disarm()` was already called (or equivalently, the init-failure handler in `cmd/wt/create.go` checks for the error and prints the banner + exits `ExitInitFailed`).

Option A (phase flag + shared `currentInitCmd` reference guarded by a mutex) was explicitly rejected — see Design Decisions.

The reinstallation has a small ordering window between worktree creation and signal-handler swap. The implementation SHALL minimize the time spent in this window: no I/O calls, no user prompts, no nontrivial work between `git worktree add` returning successfully and the new handler being installed. If a panic occurs in this window, the original `defer rb.Execute()` still fires — losing the worktree is the correct fallback because the signal handler is in an inconsistent state.

#### Scenario: SIGINT during git operations (phase A)

- **GIVEN** `wt create newbranch` is mid-flight during `git worktree add`
- **WHEN** the user presses Ctrl-C
- **THEN** the rollback executes
- **AND** the worktree directory is removed
- **AND** the branch is removed (if it was created by this invocation)
- **AND** the process exits with status 130

#### Scenario: SIGINT during init (phase B)

- **GIVEN** `wt create newbranch` has successfully created the worktree and is now running the init script
- **WHEN** the user presses Ctrl-C
- **THEN** the init child process (and its process group) receives SIGINT
- **AND** the worktree directory is kept
- **AND** the branch is kept
- **AND** stderr contains the init-failure banner (worktree path, retry hint, remove hint)
- **AND** the process exits with status 7 (`ExitInitFailed`)

#### Scenario: Process group signaling

- **GIVEN** the init script spawns a long-running child of its own
- **WHEN** SIGINT is delivered to the init `*exec.Cmd`
- **THEN** the script's child process(es) also receive SIGINT (because the cmd's process group was targeted, not just the cmd's PID)

## Testing Requirements

### Requirement: Integration test for init failure

`src/cmd/integration_test.go` SHALL contain at least one integration test that:

1. Builds the `wt` binary into a `t.TempDir()` directory (or uses the existing build harness in that file).
2. Initializes a real git repository in `t.TempDir()` (with at least one commit so `wt create` has a branch to fork from).
3. Sets `WORKTREE_INIT_SCRIPT` to point at a script that writes a marker line to stderr and exits 1.
4. Invokes the built `wt` binary with `wt create --non-interactive testbranch`.
5. Asserts the process exit code is exactly `7`.
6. Asserts the worktree directory at the expected path still exists on disk.
7. Asserts the branch `testbranch` exists in the repository (via `git branch --list` or equivalent).
8. Asserts the captured stderr contains the worktree path, the substring `wt init` (the retry hint), and the substring `wt delete` (the remove hint).

The test name SHOULD be self-describing (e.g., `TestIntegration_CreateInitFailure_KeepsWorktreeAndExits7`). This test is mandatory per Constitution Principle IV.

#### Scenario: End-to-end init failure preserves worktree

- **GIVEN** the integration test above is run
- **WHEN** the test process exits
- **THEN** the assertions in steps 5–8 all pass

### Requirement: Unit tests for resolver and banner

`internal/worktree/` SHALL contain unit tests covering:

- `ResolveInitInvocation` returns the expected `*exec.Cmd` for a command on PATH (use a binary known to exist, e.g., the path to `true` discovered via `exec.LookPath`).
- `ResolveInitInvocation` returns `&InitNotFound{Kind: CommandNotOnPath, Name: ...}` for a command not on PATH (use a randomly generated nonexistent name).
- `ResolveInitInvocation` returns `&InitNotFound{Kind: FileNotFound, Path: ..., RelPath: ...}` for a path that does not exist.
- `(InitNotFound).RenderWarning()` produces a non-empty string for both `Kind` values and includes the relevant context fields.
- The init-failure banner helper (`PrintInitFailureBanner`) produces output that contains the worktree path, the `wt init` retry hint, and the `wt delete <name>` remove hint, for both the `*exec.ExitError` case and the non-`ExitError` case.

`src/cmd/wt/create_test.go` SHALL contain unit tests covering:

- `TestCreate_InitFailureKeepsWorktree` — both the existing-branch path and the exploratory (no-branch-arg) path. The test SHALL stub or set `WORKTREE_INIT_SCRIPT` so the init step fails, then assert the worktree directory and branch survive and the exit code is `ExitInitFailed`.
- `TestCreate_InitFailureBannerHasRetryHint` — assert stderr contains the worktree path and the `wt init` retry hint string.

These unit tests are mandatory per Constitution Principle IV.

#### Scenario: Resolver unit tests cover all four branches

- **GIVEN** the resolver test suite runs
- **THEN** there is at least one test case for each of: command-on-PATH success, command-not-on-PATH `*InitNotFound`, file-exists success, file-missing `*InitNotFound`

### Requirement: Automated SIGINT integration test

`src/cmd/integration_test.go` SHALL contain an integration test that exercises SIGINT delivery during the init phase. The test:

1. Builds the `wt` binary.
2. Sets `WORKTREE_INIT_SCRIPT` to a slow script (e.g., `sleep 30`) so the init phase is reliably in-flight when SIGINT is delivered.
3. Starts the `wt create --non-interactive testbranch` process via `exec.Cmd`.
4. Waits a short, deterministic interval (e.g., poll for the worktree directory's existence, with a timeout cap) to ensure the worktree has been created and init has started.
5. Calls `cmd.Process.Signal(syscall.SIGINT)` against the spawned binary.
6. Waits for the process to exit (with a generous timeout — at least 10 seconds).
7. Asserts the process exit code is `7` (`ExitInitFailed`).
8. Asserts the worktree directory exists.
9. Asserts the branch exists.

The test MAY call `t.Skip` on Windows (where `syscall.SIGINT` semantics differ). On Unix the test SHALL run by default.

This test is mandatory per intake assumption #10 (clarified) and Constitution Principle IV.

#### Scenario: SIGINT during slow init keeps worktree

- **GIVEN** the SIGINT integration test above runs on Linux or macOS
- **WHEN** the test sends `SIGINT` to the in-flight `wt create`
- **THEN** the assertions in steps 7–9 all pass
- **AND** the test completes within the configured timeout

#### Scenario: Test skipped on Windows

- **GIVEN** the test runs on `GOOS=windows`
- **THEN** the test calls `t.Skip` with a brief reason and reports as skipped (not failed)

### Requirement: Test launch guard prevents host side-effects

> Added mid-apply after discovery that the pre-existing default-app tests
> (`TestCreate_WorktreeOpenDefault`, `TestOpen_AppDefault`) leaked real VSCode
> windows during `go test ./...` runs. The intake's "no host side-effects"
> theme (Constitution / `code-review.md`) extends naturally to GUI launchers
> beyond the tmux/byobu env isolation that already existed.

`internal/worktree/apps.go` `OpenInApp` SHALL honor a `WT_TEST_NO_LAUNCH` environment-variable seam: when set to `"1"`, every `appCmd` except `"open_here"` MUST short-circuit — emit a `[wt-test-no-launch]` marker line to stderr and return `nil` without exec'ing any GUI/terminal/clipboard binary (`code`, `cursor`, `iterm`, `terminal_app`, `finder`, `ghostty_*`, `gnome_terminal`, `konsole`, `nautilus`, `dolphin`, `copy_macos`, `copy_linux`). The `"open_here"` case is exempt because it is cooperative (writes to `WT_CD_FILE` or stdout) and produces no host side effect.

The `runWt` test helper in `src/cmd/wt/testutil_test.go` SHALL default this env var to `"1"`, appended before the trailing user-env override so individual tests can opt out via the `env` parameter (last-wins). The `--worktree-open=default` and `wt open --app default` tests SHALL assert the marker is present in stderr — if a real launch ever leaks past the seam, these assertions fail loudly.

#### Scenario: Default-app codepath does not launch real apps under test

- **GIVEN** a test runs `wt create --worktree-open=default` via `runWt`
- **AND** `WT_TEST_NO_LAUNCH=1` is in the test env (the `runWt` default)
- **WHEN** `OpenInApp` is reached with a resolved `appCmd` like `"code"`
- **THEN** no `code` (or other GUI) process is exec'd
- **AND** stderr contains `[wt-test-no-launch] would open ... in "code" ...`
- **AND** `OpenInApp` returns `nil`

#### Scenario: `open_here` is exempt from the seam

- **GIVEN** `WT_TEST_NO_LAUNCH=1` is set
- **WHEN** `OpenInApp` is called with `appCmd="open_here"`
- **THEN** the existing `WT_CD_FILE` / stdout-`cd` behavior runs unchanged
- **AND** no `[wt-test-no-launch]` marker is emitted (the short-circuit is skipped)

## Deprecated Requirements

### Rollback-on-init-failure

**Reason**: The current behavior in `src/cmd/wt/create.go` (lines 235–239 and 243–247) executes the rollback when the init script exits non-zero, destroying a worktree whose git operations all succeeded. This is being treated as a bug, not a configurable preference (per intake; the user explicitly confirmed "This is just the new default.").

**Migration**: Replaced by "Worktree kept on init-script non-zero exit" (above). No flag, no env var, no shim restores the old behavior. The code paths at the cited line ranges are deleted in favor of `rb.Disarm()` + banner + `os.Exit(ExitInitFailed)`.

### Two-line init-failure banner

**Reason**: The current banner —

```
Error: Init script failed
  Why: exit status 1
  Fix: Check the init script for errors
```

— is rendered via `wt.ExitWithError(wt.ExitGeneralError, "Init script failed", err.Error(), "Check the init script for errors")`. The "Why" line is the raw `(*exec.ExitError).Error()` text, which is just `exit status N`. The "Fix" line provides no path, no rerun command, no remove hint. By the time the banner appears, the init script's real diagnostic output has already streamed past and may be out of scrollback.

**Migration**: Replaced by "Init-failure banner" (above), which includes the absolute worktree path, the `cd <path> && wt init` retry hint, and the `wt delete <name>` remove hint, and which is rendered via the new `PrintInitFailureBanner` helper.

### Silent skip on init-script not-found in `RunWorktreeSetup`

**Reason**: `internal/worktree/crud.go:133` and `:139` currently return `nil` silently when the init script's command isn't on PATH or the file doesn't exist. `cmd/wt/init.go:73–87` prints a verbose, helpful warning under the same conditions. The two paths disagree on user-visible behavior for the same protocol contract.

**Migration**: Replaced by "Unified not-found warning" (above). The verbose warning becomes the canonical output at both call sites, rendered by a single helper.

### Duplicated init-protocol resolver

**Reason**: `cmd/wt/init.go:36–104` (`runInitScript`) and `internal/worktree/crud.go:122–160` (`RunWorktreeSetup`) each independently parse the init-script string, do command-vs-path detection, and run `exec.LookPath`/`os.Stat`. Two implementations of the same protocol drift over time and have already drifted on user-visible not-found behavior.

**Migration**: Replaced by `ResolveInitInvocation` (above). Both call sites consume the single resolver function.

### Unconditional rollback-on-SIGINT during init

**Reason**: The signal handler at `src/cmd/wt/create.go:80–86` unconditionally calls `rb.Execute()` on SIGINT/SIGTERM. After the worktree has been built and init is running, Ctrl-C from the user almost always means "stop this script, I'll fix it" — not "nuke the just-created worktree."

**Migration**: Replaced by "SIGINT during init kills only the init child" (above). The handler is reinstalled after worktree creation (Option B) to route SIGINT through the init child and then into the natural init-failure code path.

## Design Decisions

1. **Resolver placement: new file vs. extend `crud.go`** — Place `ResolveInitInvocation`, `InitNotFound`, and the `RenderWarning` helper in a new file `src/internal/worktree/init.go` (separate from `crud.go`).
   - *Why*: Keeps `crud.go` focused on git CRUD (the existing `RunWorktreeSetup` slims down to a thin wrapper). The new resolver + struct + rendering helper form a cohesive unit ~80–120 lines that earn their own file. Tests live in `src/internal/worktree/init_test.go`.
   - *Rejected*: Extending `crud.go` in place. Would push the file past 250 lines and conflate two responsibilities (git CRUD vs. external-script resolution).

2. **`InitNotFound` as typed struct vs. sentinel errors** — Typed struct with a `Kind` discriminator and per-kind contextual fields.
   - *Why*: Locked by intake assumption #11. Message rendering at the two call sites becomes a single `switch notFound.Kind`. The `Kind` named type lets the compiler flag unhandled cases if new kinds are added later. Sentinel errors would force `errors.As` + a type assertion per case, with no readability gain.
   - *Rejected*: Sentinel errors (`var ErrCommandNotOnPath = errors.New(...)`). Loses contextual fields (would need a custom error type anyway); awkward for the dual-call-site rendering switch.

3. **No opt-in flag for kept-worktree behavior** — The kept-worktree behavior is the unconditional new default; no `--keep-on-init-failure` flag.
   - *Why*: Locked by intake assumption #2 (user explicitly said "this is just the new default"). The previous rollback-on-init-failure is being treated as a bug, not a preference. Adding a flag would split the user base and create migration burden for fab-kit operators with no compensating benefit — no one will ever prefer "destroy my just-created worktree because my init script has a typo."
   - *Rejected*: An opt-in `--keep-on-init-failure` boolean flag (or `WORKTREE_KEEP_ON_INIT_FAILURE` env var). Discussed and rejected during intake.

4. **Bundle all five changes vs. land incrementally** — Land all five changes (kept worktree, banner, unified resolver, exit code, SIGINT) as a single spec/plan/PR.
   - *Why*: The five changes are mutually reinforcing. Keeping the worktree without the new banner leaves users staring at a useless "exit status 1" message with no rerun hint. The new banner without the unified resolver means `wt init` and `wt create`'s init step still diverge on not-found behavior. The exit code is the operator-facing complement to the banner. SIGINT is the same fix as kept-worktree applied to a different trigger. Partial landing would create transitional states where the failure path is internally inconsistent.
   - *Rejected*: Incremental landing across multiple PRs. Would force users through 2–4 transitional behaviors over weeks; each transitional state is harder to document than the final state.

5. **`ExitInitFailed = 7` vs. overloading `ExitGitError`** — New typed exit code.
   - *Why*: Locked by Constitution Principle III ("Typed Exit Codes"). Init failure is semantically distinct from git failure — by the time init runs, all git operations have succeeded. Operators (fab-kit, `hop`, shell wrappers) want to programmatically detect "worktree exists, init didn't complete" and offer a "retry init" affordance. Overloading `ExitGitError` would conflate these.
   - *Rejected*: Reusing `ExitGitError` or `ExitGeneralError`. Loses the operator-facing signal. Violates Constitution III.

6. **Unify resolver vs. patch `RunWorktreeSetup` in place** — Extract `ResolveInitInvocation` as the single resolution contract.
   - *Why*: `docs/specs/init-protocol.md` describes a single contract; the code should expose it through a single function. Two implementations have already drifted (silent-skip vs. verbose-warning). Patching the message in `RunWorktreeSetup` separately preserves the duplication and the latent drift risk for the next change.
   - *Rejected*: Just adding the verbose warning to `RunWorktreeSetup`'s current code in place. Fixes the immediate symptom but leaves the duplicated parsing/lookup logic, which is what created the divergence in the first place.

7. **SIGINT Option B (reinstall handler) vs. Option A (phase flag + shared cmd ref)** — Option B.
   - *Why*: Locked by intake assumption #9. Cleaner control flow (no shared mutable state across goroutines, no mutex), no `currentInitCmd *exec.Cmd` package-level variable. The cost — a small ordering window between worktree creation and handler reinstallation — is bounded and the fallback (original `defer rb.Execute()`) is correct.
   - *Rejected*: Option A (phase flag + shared cmd reference + mutex). More defensive against the ordering window but adds shared mutable state and a goroutine-correctness obligation that's easy to break in future edits.

## Assumptions

<!-- Confirmed/upgraded from intake; new spec-level assumptions appended at #13+.
     The intake's table is fully clarified (12 Certain, 0 Confident/Tentative/Unresolved)
     so the spec confirms each row and records new spec-stage decisions for resolver
     file placement, banner-label naming convention, and rendering-helper API shape. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Change is `refactor`-shaped (formalizes duplicated resolver, adds typed exit code) but `.status.yaml` records `fix`. Recorded `change_type` is NOT amended. | Confirmed from intake #1. Both classifications are defensible; preserving the recorded value avoids churn. | S:90 R:90 A:85 D:80 |
| 2 | Certain | No opt-in flag, no backward-compat env var. Kept-worktree is the unconditional new default. | Confirmed from intake #2. User confirmed "This is just the new default." | S:100 R:80 A:95 D:95 |
| 3 | Certain | All five recommendations (kept worktree, banner, resolver, exit code, SIGINT) land as a single change. | Confirmed from intake #3. Mutually reinforcing — partial landings create inconsistent transitional states. | S:95 R:85 A:90 D:90 |
| 4 | Certain | `ExitInitFailed = 7`, appended after `ExitTmuxWindowError = 6`. No reordering of existing constants. | Confirmed from intake #4. Existing codes are stable and referenced by shell wrappers; appending is the only safe option. | S:100 R:60 A:100 D:100 |
| 5 | Certain | Banner contains, in order: status line (with extracted exit code when available) → kept-worktree marker (absolute path) → retry hint (`cd <path> && wt init`) → remove hint (`wt delete <name>`). Tests assert presence of path/retry/remove, not exact byte equality. | Confirmed from intake #5. Exact wording is a Confident-level decision deferred to plan; spec describes shape and intent. | S:95 R:90 A:85 D:75 |
| 6 | Certain | Resolver signature is `ResolveInitInvocation(initScript, repoRoot string) (*exec.Cmd, *InitNotFound, error)`. | Confirmed from intake #6. | S:95 R:90 A:80 D:70 |
| 7 | Certain | `wt init` and `wt create`'s init step print byte-identical not-found warnings, rendered via a single helper. The silent-skip in `RunWorktreeSetup` is removed. | Confirmed from intake #7. | S:95 R:80 A:90 D:80 |
| 8 | Certain | `wt create --reuse` keeps its current "warn-but-continue on init failure" semantics; does NOT adopt `ExitInitFailed`. | Confirmed from intake #8. Reuse implies the worktree was already functional; init there is a refresh, not a gate. | S:95 R:80 A:90 D:85 |
| 9 | Certain | SIGINT-during-init uses Option B (`signal.Reset` + reinstall handler after worktree creation). The reinstall window MUST contain no I/O and no user prompts. | Confirmed from intake #9. Cleaner control flow than Option A; bounded fallback if the window is interrupted. | S:95 R:70 A:60 D:55 |
| 10 | Certain | SIGINT-during-init has an automated integration test using `cmd.Process.Signal(syscall.SIGINT)` against the spawned `wt` binary with a slow init script. MAY `t.Skip` on Windows. | Confirmed from intake #10. Aligns with Constitution Principle IV. | S:95 R:80 A:50 D:50 |
| 11 | Certain | `InitNotFound` is a typed struct with `Kind` field (`CommandNotOnPath` / `FileNotFound`) plus `Name`, `Path`, `RelPath` context fields. `Kind` is a named type so unhandled cases are compiler-visible. | Confirmed from intake #11. | S:95 R:75 A:55 D:55 |
| 12 | Certain | Integration test for failing init script in `src/cmd/integration_test.go` + unit tests in `src/cmd/wt/create_test.go` and `src/internal/worktree/init_test.go` are mandatory. | Confirmed from intake #12. Constitution Principle IV. | S:95 R:85 A:95 D:90 |
| 13 | Certain | The resolver, `InitNotFound`, the warning-rendering helper, and the init-related unit tests live in a NEW file `src/internal/worktree/init.go` (not in `crud.go`). | New spec-stage decision. Keeps `crud.go` focused on git CRUD; the resolver + struct + helper form a cohesive ~80–120-line unit. Tests in `src/internal/worktree/init_test.go`. | S:80 R:85 A:90 D:85 |
| 14 | Certain | The init-failure banner helper is `PrintInitFailureBanner(wtPath, name string, err error)` and lives in `src/internal/worktree/errors.go` (or a sibling file in the same package). Wording is set during plan. | New spec-stage decision. The intake gave the helper name as a working name; spec confirms it. Co-locating with existing `WtError` rendering and color helpers keeps banner styling consistent. | S:90 R:90 A:85 D:80 |
| 15 | Certain | The not-found warning rendering helper is a method on `InitNotFound` (e.g., `RenderWarning() string`), not a free function. Single source of truth for both call sites. | New spec-stage decision. A method binds the rendering to the data type and prevents future drift; consistent with the typed-struct decision (#11). | S:85 R:85 A:85 D:75 |
| 16 | Certain | Process-group setup for SIGINT (the `Setpgid: true` `SysProcAttr` on the init cmd) is set during cmd construction or just before `cmd.Run()`. Plan stage picks the exact spot. | New spec-stage decision. Required so the signal handler can deliver SIGINT to the whole process group (intake §5). The "where" is a small detail deferred to plan. | S:80 R:85 A:80 D:70 |

16 assumptions (16 certain, 0 confident, 0 tentative, 0 unresolved).
<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
