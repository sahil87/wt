# Plan: wt open — generalize to any directory

**Change**: 260508-evbf-wt-open-any-directory
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
