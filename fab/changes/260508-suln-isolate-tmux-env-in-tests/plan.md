# Plan: Isolate tmux/byobu env in test harness

**Change**: 260508-suln-isolate-tmux-env-in-tests
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
