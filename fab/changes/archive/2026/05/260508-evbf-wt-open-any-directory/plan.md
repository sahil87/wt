# Plan: wt open — generalize to any directory

**Change**: 260508-evbf-wt-open-any-directory
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- Extracting the apps subsystem into a shared package — deferred; subprocess delegation continues
- Renaming `wt open` or introducing a `wt launch` verb — explicitly rejected during discussion
- Supporting non-directory targets (URLs, individual files) — `wt open` remains directory-only
- Changing the app catalog, menu UX, or default-detection logic
- Modifying `hop` — the existing subprocess + env-var integration is unchanged

## CLI: `wt open` precondition

### Requirement: Soft git-context detection

`wt open` SHALL NOT require the working directory to be a git repository. The git/worktree context SHALL be treated as enrichment, not a precondition. When git context is available, the command SHALL use it for smart defaults (worktree name resolution, tab-name composition); when absent, the command SHALL proceed with non-git fallbacks defined below.

#### Scenario: Open a path from outside any git repo

- **GIVEN** the user runs `wt open /tmp/notes` from a directory that is not a git repository
- **AND** `/tmp/notes` is an existing directory
- **WHEN** the command executes
- **THEN** the app menu is displayed (or `--app` resolution proceeds)
- **AND** the chosen app opens `/tmp/notes`
- **AND** the exit code is `ExitSuccess` (0)

#### Scenario: Backwards compatibility — open from inside a worktree

- **GIVEN** the user is inside a worktree at `/repo.worktrees/swift-fox`
- **WHEN** the user runs `wt open` with no args
- **THEN** the behavior is identical to the pre-change behavior (smart defaults, current worktree opened)
- **AND** the tmux/byobu tab name is composed as `{repoName}-{wtName}`

#### Scenario: Backwards compatibility — open from a main repo

- **GIVEN** the user is at the main repo root (not inside a worktree)
- **WHEN** the user runs `wt open` with no args
- **THEN** the worktree-selection menu is displayed (existing `selectAndOpen` flow)
- **AND** the behavior is identical to the pre-change behavior

### Requirement: No-args opens cwd in non-git context

When `wt open` is invoked with no positional argument from a directory that is not a git repository, it SHALL open the current working directory. This is equivalent to invoking `wt open .` from that directory.

#### Scenario: No args from a non-git directory

- **GIVEN** the user is at `/tmp/foo` and `/tmp/foo` is not a git repository
- **WHEN** the user runs `wt open` with no args
- **THEN** the app menu is displayed (or `--app` resolution proceeds)
- **AND** the chosen app opens `/tmp/foo`
- **AND** the exit code is `ExitSuccess` (0)

### Requirement: Path arg detection precedence

When a positional argument is supplied, `wt open` SHALL first attempt to resolve it as an existing directory via `os.Stat` followed by `IsDir()`. Only if the stat resolution fails or the entry is not a directory SHALL the command attempt name-based worktree resolution.

This SHALL match the existing detection pattern used by the worktree-name fallback in the current `openCmd`.

#### Scenario: Arg is an existing directory

- **GIVEN** the user supplies `wt open ./src`
- **AND** `./src` is an existing directory
- **WHEN** the command executes
- **THEN** the path is opened (no name resolution attempted)

#### Scenario: Arg is a worktree name (in a git repo)

- **GIVEN** the user is in a git repository and supplies `wt open swift-fox`
- **AND** no directory named `swift-fox` exists at cwd
- **AND** `swift-fox` is an existing worktree
- **WHEN** the command executes
- **THEN** the worktree path is resolved via `resolveWorktreeByName` and opened

#### Scenario: Arg is neither a path nor a known worktree (in a git repo)

- **GIVEN** the user is in a git repository and supplies `wt open xyz`
- **AND** no directory or worktree named `xyz` exists
- **WHEN** the command executes
- **THEN** the command exits with `ExitGeneralError` (1)
- **AND** stderr includes a "not found" message and the existing `wt list` suggestion

### Requirement: Name resolution requires git context

When `wt open <arg>` is invoked from a non-git directory and `<arg>` is not an existing directory path, the command SHALL exit with `ExitGeneralError` (1). The error message SHALL state that name resolution requires a git repository and SHALL suggest passing a path. The error message SHALL NOT suggest cd'ing to a git repository.

#### Scenario: Name arg from non-git cwd

- **GIVEN** the user is at `/tmp/foo` (non-git)
- **WHEN** the user runs `wt open swift-fox`
- **AND** `/tmp/foo/swift-fox` does not exist as a directory
- **THEN** the command exits with `ExitGeneralError` (1)
- **AND** stderr includes a message indicating name resolution requires a git repository
- **AND** stderr suggests passing a path (e.g., `Example: wt open /absolute/path/to/dir`)
- **AND** stderr does NOT suggest cd'ing into a git repository

### Requirement: `--app` flag works in non-git context

The `--app <name>` flag SHALL function identically in both git and non-git invocations. When combined with a path argument or no-args-in-non-git-cwd, the named app SHALL be resolved and opened without invoking the interactive menu.

#### Scenario: `--app` with explicit path from non-git cwd

- **GIVEN** the user is at `/tmp/foo` (non-git)
- **WHEN** the user runs `wt open ./bar --app code` and `./bar` is an existing directory
- **THEN** the command resolves `code` via `ResolveApp` and opens `/tmp/foo/bar` in VSCode without showing the menu
- **AND** the exit code is `ExitSuccess` (0)

## App layer: tab-name composition

### Requirement: Tab name fallback when no repo context

`OpenInApp` in `src/internal/worktree/apps.go` SHALL compose tmux/byobu tab names using a helper `tabName(repoName, wtName string) string` defined in the same file. When `repoName` is empty (non-git invocation), the helper SHALL return `wtName` unmodified. When `repoName` is non-empty, the helper SHALL return `repoName + "-" + wtName` (preserving current behavior).

The helper SHALL replace the three call sites in the `byobu_tab`, `tmux_window`, and `tmux_session` cases. No other behavior in `OpenInApp` SHALL change.

#### Scenario: Tab name with empty repo name

- **GIVEN** `OpenInApp` is invoked with `appCmd="tmux_window"`, `path="/tmp/notes"`, `repoName=""`, `wtName="notes"`
- **WHEN** the helper composes the tab name
- **THEN** the tab name is `notes`

#### Scenario: Tab name with both names present (backwards compat)

- **GIVEN** `OpenInApp` is invoked with `appCmd="byobu_tab"`, `path="/repo.worktrees/swift-fox"`, `repoName="repo"`, `wtName="swift-fox"`
- **WHEN** the helper composes the tab name
- **THEN** the tab name is `repo-swift-fox` (identical to pre-change behavior)

## Launcher contract

### Requirement: Document the `WT_CD_FILE` and `WT_WRAPPER` env-var contract

A new specification file `docs/specs/launcher-contract.md` SHALL be created. It SHALL document the env-var contract that external callers (notably `hop`) rely on when delegating to `wt open` via subprocess invocation.

The spec SHALL include:

1. **Purpose** — `wt open` is the canonical directory launcher; external callers MAY delegate via subprocess
2. **Invocation surface** — `wt open [<path>|<name>] [--app <app>]` with the precondition relaxation defined above
3. **`WT_CD_FILE` semantics** — when set to a writable file path, "Open here" (and `--app open_here`) writes the resolved directory path to that file (mode 0600) instead of printing a `cd --` shell line. Consumers read the file after a zero-exit `wt open` and `cd` the parent shell themselves
4. **`WT_WRAPPER` semantics** — when set to `1`, suppresses the "shell wrapper not loaded" hint that would otherwise print to stderr. Consumers handling their own `cd` SHOULD set this to avoid confusing users
5. **Exit-code contract** — `ExitSuccess` (0): success; `ExitGeneralError` (1): arg-resolution failures (unknown name, unknown app); `ExitGitError` (3): only when a git operation was actually attempted (i.e., name resolution in a git repo where `git worktree list` failed); `ExitByobuTabError` (5) / `ExitTmuxWindowError` (6): tab-creation failures. Non-zero exit means the consumer MUST NOT trust the contents of `WT_CD_FILE`.
6. **Stability guarantees** — env-var names, exit-code semantics, and "Open here" file-write behavior are stable. Internal additions (new app types, new menu items, new internal flags) do NOT count as breaking changes. Changes to the documented contract require a constitution amendment per Module Path Stability precedent.
7. **Non-goals** — `wt open` is not a general-purpose `xdg-open` replacement; only opens directories, not URLs or arbitrary files.

#### Scenario: Spec file exists with all required sections

- **GIVEN** the change has shipped
- **WHEN** a developer reads `docs/specs/launcher-contract.md`
- **THEN** the file contains sections covering the seven points listed above

### Requirement: Cross-link from `cli-surface.md`

The existing `docs/specs/cli-surface.md` `wt open` entry SHALL be updated to:

1. Reflect the loosened precondition (positional path arg works without a git repo; no-args opens cwd in non-git context; name resolution still requires a git repo)
2. Cross-reference `launcher-contract.md`
3. Update the "Exit codes" line to reflect that `ExitGitError` no longer applies to all `wt open` invocations — only to those that attempt git operations

## Tests

### Requirement: Integration test for the launcher contract

The integration test suite SHALL include at least one test that exercises the `WT_CD_FILE` + `WT_WRAPPER` contract end-to-end against a non-git directory.

The test SHALL:

1. Create a non-git temporary directory via `t.TempDir()`
2. Set `WT_CD_FILE` to a writable file path inside the temp dir, `WT_WRAPPER=1`
3. Invoke the built `wt` binary as a subprocess with `wt open <temp-dir> --app open_here`
4. Assert the exit code is 0
5. Assert the contents of the `WT_CD_FILE` file equal the temp-dir path
6. Assert no "shell wrapper not loaded" hint appears on stderr

#### Scenario: Contract test passes against a non-git temp dir

- **GIVEN** a non-git temp directory and a writable cd-file path
- **WHEN** `wt open <temp-dir> --app open_here` is invoked with `WT_CD_FILE` and `WT_WRAPPER=1` set
- **THEN** the exit code is 0
- **AND** the cd-file contains the temp-dir path
- **AND** stderr is empty (or at minimum, contains no "shell wrapper" hint)

### Requirement: Unit tests for non-git paths in `openCmd`

`src/cmd/wt/open_test.go` SHALL include unit tests covering each new flow branch:

1. No args from non-git cwd → opens cwd
2. Path arg from non-git cwd → opens path
3. Name arg from non-git cwd → exits `ExitGeneralError` with the correct message
4. Path arg from git cwd (existing branch) → still opens path
5. Name arg from git cwd (existing branch) → still resolves via `resolveWorktreeByName`

#### Scenario: Each new branch has at least one unit test

- **GIVEN** the change has shipped
- **WHEN** `go test ./src/cmd/wt -run TestOpen` runs
- **THEN** all new branches (1-3 above) are exercised by named test cases
- **AND** existing branches (4-5 above) continue to be exercised and pass

### Requirement: Unit tests for `tabName` helper

`src/internal/worktree/apps_test.go` SHALL include unit tests for the new `tabName(repoName, wtName) string` helper covering both branches (empty and non-empty `repoName`).

#### Scenario: tabName helper has table-driven coverage

- **GIVEN** the change has shipped
- **WHEN** `go test ./src/internal/worktree -run TestTabName` runs
- **THEN** at least two cases pass: empty `repoName` returns `wtName`; non-empty `repoName` returns `repoName + "-" + wtName`

## Backwards compatibility

### Requirement: Every prior `wt open` invocation behaves identically

Every invocation of `wt open` that produced a successful outcome before this change SHALL continue to produce the identical outcome after this change. The change SHALL NOT alter:

1. The app menu contents or ordering
2. The default-app detection logic (`DetectDefaultApp`, `last-app` cache, `TERM_PROGRAM`-based hints)
3. The tab-name format when both `repoName` and `wtName` are present
4. Exit codes for any pre-existing failure mode (the only new exit-code path is the non-git + name-arg case, which previously failed with `ExitGitError` and now fails with `ExitGeneralError` — see Design Decisions)
5. The "Open here" `WT_CD_FILE` write behavior
6. The shell wrapper hint when `WT_WRAPPER` is unset

#### Scenario: Existing integration tests continue to pass

- **GIVEN** the change has shipped
- **WHEN** the existing integration test suite (`src/cmd/wt/integration_test.go`) runs
- **THEN** every test that passed before the change continues to pass

## Design Decisions

1. **Soft detection at `RunE` entry, not via a separate command**: integrating within `wt open` (vs. adding `wt launch`) preserves the menu-defaults state in one place and avoids fragmenting the launcher UX. Discussed and confirmed.
   - *Why*: `wt open` is the verb users already type; "launch this directory" is not a verb in their vocabulary
   - *Rejected*: A `wt launch <path>` verb that `wt open` would call into. Fragments the `last-app` cache, `TERM_PROGRAM` detection, and tmux/byobu session awareness across two commands.

2. **Subprocess delegation, not a shared library**: `hop` continues to invoke `wt open` as a subprocess via `WT_CD_FILE` + `WT_WRAPPER`, rather than importing a shared apps package.
   - *Why*: Subprocess delegation lets `hop` inherit `wt`'s view of the world (cached defaults, session detection) for free; no config-resolution duplication
   - *Rejected*: Extracting `internal/worktree/apps.go` into a public package. Bigger refactor with a duplication tax (every consumer must re-derive defaults). Left as a future option (Option 3 in `/fab-discuss`); this change does not preclude it.

3. **`ExitGeneralError` (not `ExitGitError`) for non-git + name-arg failure**: when a user is at a non-git directory and supplies a name arg that isn't an existing directory, no git operation has been attempted — the error is "we cannot resolve this argument," not "git failed."
   - *Why*: Constitution III requires typed exit codes that map to actual failure modes. `ExitGitError` is reserved for actual git invocation failures.
   - *Rejected*: Reusing `ExitGitError` for all non-git scenarios. Misleading; breaks the semantic contract documented in `cli-surface.md`'s exit-code table.

4. **Tab-name fallback uses `wtName` alone (not `wtName + "-" + filepath.Dir`)**: when `repoName` is empty, the tab name is just the basename of the opened directory.
   - *Why*: Simplest sensible default; matches user expectation (the tab is "the thing I opened"). Two-segment names risk confusion (which segment is the worktree?).
   - *Rejected*: Composing parent-base (e.g., `notes-foo` for `/tmp/notes/foo`). More information but no clear win; users can rename tabs themselves.

5. **`tabName` helper lives in `apps.go` next to `OpenInApp`**: co-located with the only caller.
   - *Why*: Existing project pattern (helpers live next to their callers); one small function does not warrant its own file.
   - *Rejected*: Putting it in `names.go`. That file is currently scoped to worktree-folder naming (random adjective-noun); adding tab-naming to it would broaden its scope and reduce thematic clarity.

6. **Path detection: `os.Stat` + `IsDir()` first, then name resolution**: matches the existing fallback pattern in `openCmd`.
   - *Why*: Reuses the existing pattern, minimizes risk of regression, preserves the rule that an existing directory always wins over a coincidentally-named worktree.
   - *Rejected*: Trying git resolution first. Inverts the current code structure, adds latency for path-only invocations, and changes a documented behavior (existing dir takes precedence in `cli-surface.md`).


## Tasks

### Phase 1: Setup

<!-- This is a refactor in an existing codebase; no setup tasks required. -->

_(none — refactor in an existing codebase)_

### Phase 2: Core Implementation

- [x] T001 Add `tabName(repoName, wtName string) string` helper in `src/internal/worktree/apps.go`. When `repoName == ""`, return `wtName`; otherwise return `repoName + "-" + wtName`.
- [x] T002 Replace the three `repoName + "-" + wtName` (or `tabName := repoName + "-" + wtName`) call sites in `src/internal/worktree/apps.go` (`byobu_tab`, `tmux_window`, `tmux_session` cases) with `tabName(repoName, wtName)` / `sessionName := tabName(...)`.
- [x] T003 Refactor `openCmd().RunE` in `src/cmd/wt/open.go`: replace the hard `wt.ValidateGitRepo()` failure with soft detection (`inRepo := wt.ValidateGitRepo() == nil`), and only call `wt.GetRepoContext()` when `inRepo` is true.
- [x] T004 In `src/cmd/wt/open.go` `RunE`, restructure the resolution flow to handle (a) target as path (always wins, regardless of git context; `repoName` left empty when arg is a path), (b) target as name (requires `inRepo`; otherwise fail with `ExitGeneralError` and the spec-mandated message), (c) no-args + worktree (existing flow), (d) no-args + main repo (existing `selectAndOpen`), (e) no-args + non-git (open cwd). Pass `repoName` (possibly empty) through to `OpenInApp` / `handleAppMenu`.
- [x] T005 Preserve the existing `--app` + selection-menu guard in `src/cmd/wt/open.go`: when `inRepo && !IsWorktree && target == "" && appFlag != ""`, still exit `ExitInvalidArgs` with "No worktree specified" (backwards-compat).

### Phase 3: Integration & Edge Cases

- [x] T006 Update `docs/specs/cli-surface.md` `wt open` entry: positional arg description (path arg works without git; no-args opens cwd in non-git context; name arg still requires git); update Exit codes line so `ExitGitError` only applies when git operations are attempted (not blanket "not a git repo"); add cross-reference to `launcher-contract.md`.
- [x] T007 Create `docs/specs/launcher-contract.md` covering the seven sections required by spec: Purpose, Invocation surface, `WT_CD_FILE` semantics, `WT_WRAPPER` semantics, Exit-code contract, Stability guarantees, Non-goals.
- [x] T008 Add the new `launcher-contract.md` row to `docs/specs/index.md` so it is discoverable.

### Phase 4: Polish (Tests)

- [x] T009 [P] Add table-driven unit tests for `tabName` in `src/internal/worktree/apps_test.go`: empty `repoName` → `wtName`; non-empty `repoName` → `repoName + "-" + wtName`.
- [x] T010 [P] Extend `src/cmd/wt/open_test.go` with integration-style tests (using the existing `runWt` harness) for new branches: (a) no-args from non-git tempdir → opens cwd via `--app open_here` (succeeds, exit 0), (b) path arg from non-git tempdir → opens that path via `--app open_here` (succeeds, exit 0), (c) name arg from non-git tempdir → exits `ExitGeneralError` (1) with "name resolution requires a git repository" message and no "cd" suggestion.
- [x] T011 [P] Add an integration test in `src/cmd/wt/integration_test.go` that exercises the full `WT_CD_FILE` + `WT_WRAPPER=1` contract end-to-end against a non-git tempdir: `wt open <tempdir> --app open_here` → exit 0, cd-file contents == tempdir, no shell-wrapper hint on stderr.
- [x] T012 Run `go test ./...` from `src/` and confirm all tests pass (existing + new).

## Execution Order

- T001 blocks T002 (call sites depend on the helper existing)
- T003 blocks T004 (T004 restructures the body that T003 enters with soft detection)
- T004 blocks T005 (T005 preserves a sub-branch within the restructured flow)
- T009, T010, T011 can run in parallel after T002/T005 are done
- T012 runs last

## Acceptance

### Functional Completeness

- [x] A-001 Soft git-context detection: `wt open` no longer fails with `ExitGitError` when run outside a git repo; git context is treated as enrichment, not a precondition. (open.go:39 `inRepo := wt.ValidateGitRepo() == nil`)
- [x] A-002 No-args opens cwd in non-git context: `wt open` with no positional argument from a non-git directory opens the current working directory. (open.go:99-107 default branch)
- [x] A-003 Path arg detection precedence: when a positional arg is supplied, `os.Stat` + `IsDir()` is attempted first; only on failure does the command try name-based resolution. (open.go:57-79)
- [x] A-004 Name resolution requires git context: `wt open <name>` from a non-git cwd, where `<name>` is not an existing directory, exits with `ExitGeneralError` (1) and a message stating name resolution requires a git repository. (open.go:72-79)
- [x] A-005 Error message guidance: the non-git + name-arg error suggests passing a path (e.g., `Example: wt open /absolute/path/to/dir`) and does NOT suggest cd'ing into a git repository (post-rework: trailing "or run from a git repo" clause removed; test asserts its absence). (open.go:77; TestOpen_NameArg_NonGit_FailsWithGuidance)
- [x] A-039 Sentinel error `errWorktreeNotFound` distinguishes "not found" (ExitGeneralError) from "git worktree list failed" (ExitGitError) per launcher-contract.md §5. (open.go:160 sentinel + open.go:62-79 caller; TestOpen_NameArg_NotFound_InRepo)
- [x] A-040 Integration test asserts `WT_CD_FILE` is written with mode 0600 per launcher-contract.md §3 stability guarantee. (integration_test.go:177-184)
- [x] A-006 `--app` flag works in non-git context: `--app <name>` resolves and opens without invoking the menu when combined with a path arg or no-args-in-non-git-cwd. (open.go:110-143; tested by TestOpen_NoArgs_NonGit_OpensCwd, TestOpen_PathArg_NonGit_OpensPath)
- [x] A-007 `tabName` helper exists in `src/internal/worktree/apps.go` and is used by all three of `byobu_tab`, `tmux_window`, `tmux_session` cases. (apps.go:185-190 helper; apps.go:250, 265, 275 call sites)
- [x] A-008 `tabName` returns `wtName` when `repoName == ""` and `repoName + "-" + wtName` otherwise. (apps.go:185-190; tested by TestTabName)
- [x] A-009 `docs/specs/launcher-contract.md` exists and contains all seven required sections (Purpose, Invocation, `WT_CD_FILE`, `WT_WRAPPER`, Exit codes, Stability, Non-goals).
- [x] A-010 `docs/specs/cli-surface.md` `wt open` entry reflects the loosened precondition, updates the exit-code line, and cross-references `launcher-contract.md`.

### Behavioral Correctness

- [x] A-011 Tab-name format unchanged when both names present: `repoName-wtName` (e.g., `repo-swift-fox`) is preserved for all in-worktree invocations. (apps.go:189; tested in TestTabName)
- [x] A-012 Non-git + name-arg now returns `ExitGeneralError` (1), where the pre-change behavior returned `ExitGitError` (3). Documented in spec Design Decision 3. (open.go:75; tested by TestOpen_NameArg_NonGit_FailsWithGuidance)

### Removal Verification

- [x] A-013 The hard `wt.ValidateGitRepo()` early-exit at the top of `openCmd().RunE` is removed; no remaining "Not a git repository" `ExitGitError` exit in `open.go`. (verified — only soft check remains at open.go:39)

### Scenario Coverage

- [x] A-014 Scenario "Open a path from outside any git repo": exercised by an automated test (path arg from non-git tempdir, exit 0). (TestOpen_PathArg_NonGit_OpensPath)
- [x] A-015 Scenario "Backwards compatibility — open from inside a worktree": existing tests continue to pass (no regression).
- [x] A-016 Scenario "Backwards compatibility — open from a main repo": existing tests continue to pass (no regression). (TestOpen_ErrorFromMainRepoWithoutTarget)
- [x] A-017 Scenario "No args from a non-git directory": exercised by an automated test. (TestOpen_NoArgs_NonGit_OpensCwd)
- [x] A-018 Scenario "Arg is an existing directory": existing branch covered (path-first precedence preserved). (open.go:57)
- [x] A-019 Scenario "Arg is a worktree name (in a git repo)": existing branch covered. (open.go:60-71)
- [x] A-020 Scenario "Arg is neither a path nor a known worktree (in a git repo)": existing test (`TestOpen_ErrorNonexistentWorktree`) continues to pass.
- [x] A-021 Scenario "Name arg from non-git cwd": exercised by a new automated test. (TestOpen_NameArg_NonGit_FailsWithGuidance)
- [x] A-022 Scenario "`--app` with explicit path from non-git cwd": exercised by the integration test. (TestIntegration_LauncherContract_NonGitTempDir)
- [x] A-023 Scenario "Tab name with empty repo name": covered by `tabName` unit test.
- [x] A-024 Scenario "Tab name with both names present (backwards compat)": covered by `tabName` unit test.
- [x] A-025 Scenario "Contract test passes against a non-git temp dir": new integration test exists and passes.

### Edge Cases & Error Handling

- [x] A-026 Non-zero exit reliably means cd-file MUST NOT be trusted by consumers — documented in `launcher-contract.md` Exit-code section. (launcher-contract.md §5 "Critical rule for consumers")
- [x] A-027 `--app` validation block in main repo (no target, no worktree) still exits `ExitInvalidArgs` — backwards-compat preserved. (open.go:91-97)

### Code Quality

- [x] A-028 Pattern consistency: new code follows the existing `cmd/`-thin / `internal/worktree`-thick separation; no business logic moved into `cmd/`. (`tabName` lives in internal/worktree/apps.go)
- [x] A-029 No unnecessary duplication: existing `os.Stat`+`IsDir()`, `resolveWorktreeByName`, and `wt.ExitWithError` helpers are reused; no shadow re-implementations.
- [x] A-030 Readability/maintainability: refactored `RunE` has clearly labelled resolution branches with explanatory comments.
- [x] A-031 Existing project patterns followed: `tabName` lives next to `OpenInApp`; errors use `wt.ExitWithError` with what/why/fix; tests follow file-per-source pattern.
- [x] A-032 No god functions: RunE is 121 lines (matching the pre-change ~95-line size); branches are clearly labelled. Pre-existing pattern, not a regression introduced by this change. Sub-handlers `selectAndOpen` and `handleAppMenu` already exist for menu paths. Mark with caveat — see should-fix.
- [x] A-033 No magic strings: error messages and exit-code constants are referenced via existing named constants (`wt.ExitGeneralError` etc.).

### Backwards Compatibility

- [x] A-034 Every prior `wt open` invocation that worked before continues to work identically: existing integration tests pass without modification.
- [x] A-035 Tab-name format with both names present is unchanged: `repoName-wtName` (e.g., `repo-swift-fox`). All in-repo invocations — worktree, main-repo selection, AND path-arg — produce the historical `repo-basename` format. Only non-git invocations produce `basename` alone (no repo context to use).
- [x] A-036 "Open here" `WT_CD_FILE` write behavior is unchanged. (apps.go:196-199)
- [x] A-037 Shell wrapper hint on stderr (when `WT_WRAPPER` is unset) is unchanged. (apps.go:200-203)
- [x] A-038 App menu contents, ordering, and default-detection logic are unchanged.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
