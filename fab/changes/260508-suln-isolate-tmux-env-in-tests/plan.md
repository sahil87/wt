# Plan: Isolate tmux/byobu env in test harness

**Change**: 260508-suln-isolate-tmux-env-in-tests
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- Refactoring `OpenInApp` or its callers — no runtime code changes
- Adding a "dry-run" mode or test-injection seam to the binary
- Changes to `hop` or any external project
- Changes to `docs/specs/launcher-contract.md` (the contract is correct; the bug is in the test harness)
- A regression test that asserts on `runWt`'s constructed `cmd.Env` — explicitly rejected during clarify (brittle, asserts on internals)
- Explicit per-test env clears at every test site that uses `--worktree-open=default` / `--app=default` (defense-in-depth was rejected; chokepoint isolation in `runWt` is the chosen approach)

## Test harness: env isolation in `runWt`

### Requirement: Default-isolate launcher-affecting env in `runWt`

The `runWt` test helper in `src/cmd/wt/testutil_test.go` SHALL clear the following environment variables in the constructed `cmd.Env` before appending caller-supplied env vars:

- `TMUX`
- `BYOBU_BACKEND`
- `BYOBU_TTY`
- `BYOBU_SESSION`
- `BYOBU_CONFIG_DIR`
- `TERM_PROGRAM`

These are precisely the variables consulted by `IsTmuxSession()`, `IsByobuSession()`, and `DetectDefaultApp()` in `src/internal/worktree/`. Clearing them prevents the binary under test from resolving the default app to `tmux_window`, `tmux_session`, or `byobu_tab` — the codepaths that shell out to real `tmux new-window` / `tmux new-session` / `byobu new-window` and create real, leaked windows in the host system.

The clearing SHALL happen in the same `cmd.Env = append(...)` call that already sets `NO_COLOR=1` and `WORKTREE_INIT_SCRIPT=__wt_test_noinit__ noop`. The caller-supplied `env` slice SHALL continue to be appended last so that callers can override defaults when needed.

#### Scenario: Default isolation prevents tmux window creation

- **GIVEN** the parent shell has `TMUX=/tmp/tmux-1000/default,12345,0` set
- **AND** a test invokes `runWt(t, dir, nil, "create", "--worktree-open", "default", ...)`
- **WHEN** the test runs to completion with exit code 0
- **THEN** no new tmux windows are created in the parent tmux session
- **AND** the binary's `IsTmuxSession()` evaluates to false during the test
- **AND** `DetectDefaultApp()` does not resolve to `tmux_window`

#### Scenario: Caller can override the default isolation

- **GIVEN** a future test legitimately needs to exercise the tmux codepath
- **AND** that test passes `env: []string{"TMUX=/tmp/fake", "BYOBU_BACKEND=tmux"}` to `runWt`
- **WHEN** the test runs
- **THEN** the binary observes `TMUX=/tmp/fake` and `BYOBU_BACKEND=tmux` (caller's values, not the cleared defaults)
- **AND** `IsTmuxSession()` and `IsByobuSession()` evaluate accordingly

### Requirement: No regression in existing tests

Every test in `src/cmd/wt/` and `src/internal/worktree/` that passed before this change SHALL continue to pass after the change. In particular:

- `TestCreate_WorktreeOpenDefault` (in `create_test.go`) SHALL continue to pass — it asserts on stderr containing "Created worktree:" and the absence of "Unknown app: default", neither of which depends on the binary actually opening a tmux window.
- `TestOpen_AppDefault` (in `open_test.go`) SHALL continue to pass — it explicitly clears the launcher-affecting env vars in its own `env` slice; after this change those clears become redundant-but-correct (caller-supplied vars still override `runWt`'s defaults to the same empty value).
- All tests in `apps_test.go` that set `t.Setenv("TMUX", ...)` to test the catalog/detection logic SHALL continue to work — `t.Setenv` mutates the parent process env, not `cmd.Env`, so they are unaffected by the `runWt` change.

#### Scenario: Existing test suite passes after change

- **GIVEN** the change has shipped
- **WHEN** `go test -count=1 ./...` runs from `src/`
- **THEN** every test that passed before the change continues to pass

### Requirement: No new tmux windows are created during test execution

After this change, a complete `go test -count=1 ./...` run from `src/` SHALL NOT create any new windows in the host's tmux session, regardless of whether the test runner is itself running inside a tmux session. This is the observable behavior the bug reports against today; the change closes it.

#### Scenario: Test run from inside a tmux session leaks no windows

- **GIVEN** the test runner is running inside a tmux session (host has `TMUX` set)
- **AND** `tmux list-windows | wc -l` reports `N` before the test run
- **WHEN** `go test -count=1 ./...` runs to completion
- **THEN** `tmux list-windows | wc -l` still reports `N` after the test run
- **AND** no windows match the pattern `*-default-open-test*` or `*-default-test*`

## Audit findings

### Requirement: Document tests touching `--worktree-open` / `--app` codepaths

The change SHALL document an audit of tests that invoke `--worktree-open` (with a value other than `skip`) or `--app` (with a value that could resolve to `tmux_*` / `byobu_tab`). This audit lives in this spec's **Audit Results** section below; it does not produce code changes beyond Requirement 1, on the assumption that every audited `--worktree-open` / `--app` invocation goes through `runWt` and inherits its default isolation. (Other tests in `src/cmd/wt/` may invoke the binary directly via `exec.Command(wtBinary, ...)` — e.g., `TestCreate_PorcelainStdoutOnlyPath` — but they do not exercise the launcher codepaths and are out of scope for this audit.)

If the audit surfaces a test that bypasses `runWt` and directly invokes the binary with non-isolated env, that becomes a discovered task and is added to the plan. Per intake clarification, the expectation is **zero discovered tasks**.

### Requirement: Audit covers all `--worktree-open` and `--app` test invocations

The audit SHALL grep for `--worktree-open` and `--app` across all `*_test.go` files in `src/cmd/wt/` and `src/internal/worktree/`. For each occurrence, the audit SHALL classify the invocation as one of:

- **Safe-by-target**: the value is non-side-effecting (`--worktree-open=skip`, `--app=open_here`, `--app=copy_*`, `--app=<unknown-name>` for negative tests, or any value that fails before shelling out)
- **Safe-by-isolation**: the value could side-effect, but the test goes through `runWt` and inherits the new default isolation (`--worktree-open=default`, `--app=default`)
- **Needs explicit handling**: the test bypasses `runWt` OR explicitly re-enables the launcher env, AND the value could create real windows/sessions

#### Scenario: Audit table is present in spec

- **GIVEN** the change has shipped
- **WHEN** a developer reads this spec's `## Audit Results` section
- **THEN** the section contains a table of every `--worktree-open` and `--app` test invocation with its classification
- **AND** any "Needs explicit handling" rows have linked plan tasks that resolve them

## Audit Results

Generated during apply via `grep -rn '--worktree-open\|--app' src/cmd/wt/*_test.go src/internal/worktree/*_test.go`. Every occurrence is classified per the requirement above. No `--worktree-open` or `--app` invocations exist in `src/internal/worktree/*_test.go` (which test pure functions and do not invoke the binary). Every audited `--worktree-open` / `--app` invocation routes through `runWt`, so they inherit the new default isolation. (A small number of `cmd/wt` tests bypass `runWt` and use `exec.Command(wtBinary, ...)` directly — e.g., `TestCreate_PorcelainStdoutOnlyPath` — but none of them pass `--worktree-open` or `--app`, so they are outside the audit scope.)

| Test | File | Invocation | Classification |
|------|------|------------|----------------|
| `TestOpen_ErrorNonexistentWorktree` | `open_test.go:13` | `--app code` (target worktree does not exist) | Safe-by-target — fails on worktree resolution before `OpenInApp` |
| `TestOpen_ErrorFromMainRepoWithoutTarget` | `open_test.go:23` | `--app code` (no target supplied) | Safe-by-target — fails on missing target before `OpenInApp` |
| `TestOpen_NoArgs_NonGit_OpensCwd` | `open_test.go:45` | `--app open_here` | Safe-by-target — `open_here` writes to `WT_CD_FILE`, no external shell-out |
| `TestOpen_PathArg_NonGit_OpensPath` | `open_test.go:78` | `--app open_here` | Safe-by-target — `open_here` writes to `WT_CD_FILE`, no external shell-out |
| `TestOpen_ErrorUnknownApp` | `open_test.go:164` | `--app nonexistent-app` | Safe-by-target — `ResolveApp` fails before any shell-out |
| `TestOpen_AppDefault` | `open_test.go:186` | `--app default` | Safe-by-isolation + explicit clears (defense-in-depth) |
| `TestCreate_OpenHereSuppressesPath` | `create_test.go:566` | `--worktree-open open_here` | Safe-by-target — `open_here` does not shell out |
| `TestCreate_WorktreeOpenDefault` | `create_test.go:595` | `--worktree-open default` | Safe-by-isolation (relies on `runWt` defaults) |
| `TestIntegration_LauncherContract_NonGitTempDir` | `integration_test.go:174` | `--app open_here` | Safe-by-target — `open_here` writes to `WT_CD_FILE` only |

Zero "Needs explicit handling" rows — every invocation is either Safe-by-target (the value cannot reach `tmux_*` / `byobu_tab` / GUI shell-out paths) or Safe-by-isolation (the value could resolve to a side-effecting app, but `runWt`'s default env clears prevent it). No discovered tasks. The audit confirms Assumption 9 ("audit produces zero code changes").

## Project policy: code-review.md

### Requirement: Add test-side-effect-isolation rule to `code-review.md`

`fab/project/code-review.md` SHALL be updated to include a new project-specific review rule under the **Project-Specific Review Rules** section. The rule SHALL state that tests invoking the binary MUST NOT leak side-effects to the host system, with explicit guidance for `--worktree-open` and `--app` tests:

- Use a non-side-effecting target (`--worktree-open=skip`, `--app=open_here`, `--app=copy_*`), OR
- Rely on `runWt`'s default env isolation (which clears `TMUX`, `BYOBU_*`, `TERM_PROGRAM`), OR
- Explicitly register `t.Cleanup` to reap created windows/sessions/tabs.

The rule SHALL flag option (a) and (b) as preferred; (c) as last-resort for tests that genuinely need to verify the side-effecting codepath.

#### Scenario: Reviewer applies the new rule

- **GIVEN** a future change that adds a test invoking `wt create --worktree-open default`
- **WHEN** a reviewer (human or sub-agent) reads `fab/project/code-review.md`
- **THEN** the reviewer can flag missing isolation as a should-fix or must-fix finding per the documented rule

## Design Decisions

1. **Chokepoint isolation in `runWt`, not per-test env clears**: every test in `src/cmd/wt/` goes through `runWt`. Centralizing the env defaults at the chokepoint prevents the bug from reappearing via new tests.
   - *Why*: New contributors won't know they need to clear `TMUX` unless the harness telegraphs it. Defaults shift the burden from "remember to clear" to "remember to override (when you actually need it)."
   - *Rejected*: Per-test explicit clears (defense-in-depth). Costs ~6 lines per test for redundancy, and a single forgotten clear regenerates the bug. The single failure mode (a test that overrides `TMUX` to a real value, e.g., `TMUX=/tmp/real-session`) is uncommon and visible at the test site.

2. **Clear, don't unset**: the new env defaults set `TMUX=`, `BYOBU_*=`, `TERM_PROGRAM=` (empty values), not absent values.
   - *Why*: Go's `exec.Cmd.Env` is positional; setting empty values is the idiomatic way to override the parent's value. Removing keys would require iterating the env list, which is more code and less clear.
   - *Rejected*: Filtering `os.Environ()` to remove the keys before appending. More code, harder to read.

3. **Don't include `HOME` in the default isolation**: tests that need a clean `~/.cache/wt` already pass `HOME=t.TempDir()` explicitly.
   - *Why*: `HOME` affects `last-app` cache pollution, not the launcher choice. Centralizing it would either break tests that intentionally use the real `HOME`, or duplicate `t.TempDir()` logic in the helper. Different concern, different fix.
   - *Rejected*: Including `HOME` in the cleared list. Creates new bugs (tests reading from real `HOME` would suddenly see `~/.cache/wt` go missing).

4. **Skip the regression test for `runWt`'s env construction**: per intake clarification, no test asserts on `runWt`'s constructed `cmd.Env`.
   - *Why*: The audit + the new code-review.md rule provide policy-level coverage. A test asserting on `cmd.Env` internals is brittle (fails if `runWt` ever rearranges its env list).
   - *Rejected*: Adding a unit test for `runWt`'s env list. Brittle; tests internals not behavior.

5. **Document the audit in the spec, not as a separate file**: the audit findings live in this spec's `## Audit Results` section.
   - *Why*: The audit is a snapshot in time tied to this change. Embedding it in the spec keeps it discoverable alongside the rationale and avoids creating yet another doc file.
   - *Rejected*: A separate `docs/specs/test-env-audit.md`. Over-structured for a one-time snapshot.


## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 2: Core Implementation

<!-- Default-isolate launcher-affecting env in `runWt` so tests do not leak tmux/byobu state. -->

- [x] T001 Extend `runWt` in `src/cmd/wt/testutil_test.go` (around lines 134-148): add `TMUX=`, `BYOBU_BACKEND=`, `BYOBU_TTY=`, `BYOBU_SESSION=`, `BYOBU_CONFIG_DIR=`, `TERM_PROGRAM=` to the `cmd.Env = append(os.Environ(), ...)` block, immediately after the existing `NO_COLOR=1` and `WORKTREE_INIT_SCRIPT=...` entries. Preserve the trailing `cmd.Env = append(cmd.Env, env...)` so caller-supplied env vars still override defaults (last-wins). Add a code comment referencing launcher-contract.md / explaining the test-side-effect isolation purpose.

### Phase 3: Integration & Edge Cases

<!-- Audit existing test surface, codify the new policy. -->

- [x] T002 Regenerate the audit table in `fab/changes/260508-suln-isolate-tmux-env-in-tests/spec.md` `## Audit Results`: re-run `grep -rn '\-\-worktree-open\|\-\-app' src/cmd/wt/*_test.go src/internal/worktree/*_test.go`, classify every actual occurrence, and replace the spec's table. Drop illustrative placeholder rows (e.g., "TestCreate_WorktreeOpenSkip (if present)") — only list tests that actually exist.
- [x] T003 Append the project-specific test-side-effect-isolation rule to `fab/project/code-review.md` under `## Project-Specific Review Rules`. Match existing markdown bullet style. Cover the three permitted patterns from spec §"Project policy: code-review.md" (non-side-effecting target, runWt default isolation, explicit `t.Cleanup`) with (a)/(b) preferred and (c) last-resort.

### Phase 4: Polish

<!-- Validate the change holistically: tests pass and no tmux windows leak. -->

- [x] T004 Run the full test suite from `src/`: `cd src && go test -count=1 ./...`. Confirm all tests pass. Capture `tmux list-windows | wc -l` BEFORE and AFTER the run; assert BEFORE == AFTER, and verify no windows match `*default-open-test*` or `*default-test*`. Report both counts and any matches. **Result**: `ok cmd/wt 6.088s`, `ok internal/worktree 0.095s`; 169 PASS / 0 FAIL across 175 RUN entries (169 tests, the rest are subtests). BEFORE=3, AFTER=3, zero matches for `*default-open-test*` / `*default-test*`.

## Execution Order

- T001 must complete before T004 (T004 verifies T001's runtime effect).
- T002 and T003 are documentation-only and parallelizable with each other; both should complete before T004's verification (so any audit-discovered task is folded in first).

## Acceptance

<!-- Declarative acceptance criteria used by the review stage. -->

### Functional Completeness

- [x] A-001 runWt env defaults: `runWt` clears `TMUX`, `BYOBU_BACKEND`, `BYOBU_TTY`, `BYOBU_SESSION`, `BYOBU_CONFIG_DIR`, `TERM_PROGRAM` in the constructed `cmd.Env`, in the same `append(os.Environ(), ...)` call that already sets `NO_COLOR` and `WORKTREE_INIT_SCRIPT`.
- [x] A-002 Caller override preserved: `cmd.Env = append(cmd.Env, env...)` remains the final assignment so caller-supplied env vars (including `TMUX=/tmp/fake`, `BYOBU_BACKEND=tmux`) override the cleared defaults via Go's `exec.Cmd` last-wins semantics.
- [x] A-003 Audit table regenerated: `spec.md` `## Audit Results` reflects the actual current `--worktree-open` and `--app` invocations across `src/cmd/wt/*_test.go` and `src/internal/worktree/*_test.go`, with each classified as Safe-by-target / Safe-by-isolation / Needs explicit handling. No illustrative placeholder rows remain.
- [x] A-004 Code-review.md rule added: `fab/project/code-review.md` `## Project-Specific Review Rules` section contains a new bullet codifying the test-side-effect-isolation rule for `--worktree-open` and `--app` codepaths, naming the three permitted patterns (non-side-effecting target / runWt default isolation / explicit `t.Cleanup`) with (a)/(b) preferred over (c).

### Behavioral Correctness

- [x] A-005 No regression in existing tests: `cd src && go test -count=1 ./...` exits 0 — every test that passed before this change continues to pass after.
- [x] A-006 `TestOpen_AppDefault` keeps its explicit clears (defense-in-depth): per Design Decision in spec, the explicit `TMUX=`/`BYOBU_*=` clears at the test site are NOT removed.

### Scenario Coverage

- [x] A-007 Default isolation prevents tmux window creation: when the parent shell has `TMUX=...` set and `go test ./...` runs, no new tmux windows are created in the parent session (the binary's `IsTmuxSession()` evaluates false and `DetectDefaultApp()` does not resolve to `tmux_window`).
- [x] A-008 Test run from inside a tmux session leaks no windows: `tmux list-windows | wc -l` BEFORE the test run equals the count AFTER; no windows match `*default-open-test*` or `*default-test*`.

### Edge Cases & Error Handling

- [x] A-009 Caller-side override semantics work: a hypothetical test passing `env: []string{"TMUX=/tmp/fake", "BYOBU_BACKEND=tmux"}` to `runWt` would observe those values inside the binary (the cleared defaults are overridden, not silently dropped).

### Code Quality

- [x] A-010 Pattern consistency: the new env entries are added inline in the existing `cmd.Env = append(os.Environ(), ...)` call, matching the surrounding style. No separate helper function or new abstraction is introduced.
- [x] A-011 No unnecessary duplication: existing `runWt` chokepoint is reused; per-test env clears are NOT added (per spec's Design Decision rejecting defense-in-depth at the test site).
- [x] A-012 Readability over cleverness: the new env clears are accompanied by an inline comment explaining intent (test-side-effect isolation, opt-in override path) so future contributors understand the chokepoint pattern.
- [x] A-013 No god functions: `runWt` remains under 50 lines after the change (currently ~36 lines; six new env entries + one comment block keep it well under the threshold).
- [x] A-014 No magic strings: the cleared env var names are well-known shell environment variables consumed by `IsTmuxSession`/`IsByobuSession`/`DetectDefaultApp`; they are documented in the spec and apps.go/platform.go. Inlining them here is the idiomatic Go pattern for `exec.Cmd.Env` setup.

## Notes

- Acceptance items A-013/A-014 reference Code Quality anti-patterns (god functions, magic strings) — both apply to scope but are easily satisfied here.
- No regression test for `cmd.Env` internals per spec Design Decision 4 (rejected as brittle; audit + policy provides coverage).
- Per Constitution IV and spec Assumption 5, the runtime binary is unchanged; only test scaffolding and review policy are modified.
