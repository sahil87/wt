# Intake: Isolate tmux/byobu env in test harness

**Change**: 260508-suln-isolate-tmux-env-in-tests
**Created**: 2026-05-08
**Status**: Draft

## Origin

> Tests that exercise the default-app open codepath leak real tmux windows because the test env doesn't suppress `tmux new-window` invocations. Observed: 9 leaked tmux windows named `001-default-open-test` after a single `go test ./...` run.

This bug was discovered immediately after shipping `260508-evbf-wt-open-any-directory` (PR #2). The user's tmux session showed 9 leftover windows named `001-default-open-test`, all created by tests that exercise `wt create --worktree-open default` or `wt open --app default` without isolating the tmux env.

The bug is **pre-existing** — it is not caused by the `evbf` change. The `evbf` change does add new tests (e.g., `TestOpen_PathArg_NonGit_OpensPath`, `TestIntegration_LauncherContract_NonGitTempDir`), but those use `--app open_here` and do not hit the tmux codepath; they did not contribute to the observed leaks.

The leak makes the tests "pass" (`go test` exit 0) while leaving real tmux state behind — which is exactly the kind of failure mode the constitution's Test Integrity clause and Test What the User Sees principle (IV) are meant to surface, but they don't catch side-effect leaks because Go's test framework does not inspect the world outside the test process.

## Why

**1. The bug exists.** Concrete evidence: 9 windows named `001-default-open-test` in the user's tmux session after a single test run. Each represents a `tmux new-window -n <repo>-<wtName> -c <path>` call that succeeded but was never reaped.

**2. The bug worsens over time.** Every `go test` cycle adds more windows. CI doesn't see them (CI's tmux session, if any, dies with the runner), but local-dev sessions accumulate them. A developer running tests during the day can end up with 50+ orphan windows.

**3. The bug masks regressions.** Today the leaked windows are harmless. But if a test starts checking "no extra tmux windows were created" — a perfectly reasonable thing to want for the new `tabName` helper — it will pass for the wrong reason (the leaks pre-date the test setup) or fail for the wrong reason (a leak from a prior test contaminates this test). Sandbox isolation is a precondition for any future test that wants to assert side-effect absence.

**4. The bug is a footgun for new tests.** A contributor adding a test that calls `wt create --worktree-open` will reproduce the bug without realizing — `runWt` happily inherits `TMUX` from the parent shell and the test env doesn't telegraph the danger. Default-isolation in `runWt` removes the footgun at the chokepoint.

**5. The fix is small and reversible.** Env clearing is ~10 lines of code in `runWt` plus opt-in env passthrough for the (currently zero) tests that legitimately need a tmux session. No architectural change, no new abstraction.

Doing nothing means: the leak grows linearly with developer testing activity, future tests can't reliably assert side-effect absence, and the next contributor to add a default-app test reproduces the same bug.

## What Changes

### 1. Default-isolate launcher env in `runWt`

**File**: `src/cmd/wt/testutil_test.go`

`runWt` currently does:

```go
cmd.Env = append(os.Environ(),
    "NO_COLOR=1",
    "WORKTREE_INIT_SCRIPT=__wt_test_noinit__ noop",
)
cmd.Env = append(cmd.Env, env...)  // user-supplied vars override defaults
```

Change to:

```go
cmd.Env = append(os.Environ(),
    "NO_COLOR=1",
    "WORKTREE_INIT_SCRIPT=__wt_test_noinit__ noop",
    // Default-isolate launcher-affecting env so tests that invoke
    // `wt create --worktree-open default` or `wt open --app default`
    // do NOT shell out to tmux/byobu and leak real windows. Tests that
    // legitimately need to exercise the tmux/byobu codepaths must
    // explicitly re-enable via the env parameter.
    "TMUX=",
    "BYOBU_BACKEND=",
    "BYOBU_TTY=",
    "BYOBU_SESSION=",
    "BYOBU_CONFIG_DIR=",
    "TERM_PROGRAM=",
)
cmd.Env = append(cmd.Env, env...)  // user-supplied vars still override
```

The list of cleared vars matches what `IsTmuxSession`, `IsByobuSession`, and `DetectDefaultApp` consult:

- `IsTmuxSession()` reads `TMUX`
- `IsByobuSession()` reads `BYOBU_BACKEND`, `BYOBU_TTY`, `BYOBU_SESSION`, `BYOBU_CONFIG_DIR`
- `DetectDefaultApp()` reads `TERM_PROGRAM`

`HOME` is intentionally NOT defaulted here — tests that want a clean `~/.cache/wt` already pass `HOME=t.TempDir()` explicitly, and centralizing it would either break tests that rely on the real HOME or duplicate `t.TempDir` logic in the helper.

User-supplied env vars (the `env` parameter) continue to be appended last, preserving the existing override semantics. A test that needs to exercise tmux can pass `[]string{"TMUX=/tmp/fake", "BYOBU_BACKEND=tmux"}` and `runWt`'s defaults will be overridden.

### 2. Audit and clean up existing tests

**File**: `src/cmd/wt/create_test.go`

`TestCreate_WorktreeOpenDefault` (line 586) currently passes `--worktree-open default` without clearing `TMUX`. After change #1, the test inherits `TMUX=` from `runWt`'s defaults, so no test-side change is required — but the test should be reviewed to confirm its intent is "verify the keyword resolution, not actual window creation." The test's existing assertions (`assertContains(t, r.Stderr, "Created worktree:")` and `Unknown app: default` negative check) cover only the keyword path, so the env clearing change is sufficient.

**File**: `src/cmd/wt/open_test.go`

`TestOpen_AppDefault` (line 171) already explicitly clears `TMUX=` and `BYOBU_*=`. After change #1, those explicit clears become redundant (defaults already cover them). Two options:

- **A**: leave the explicit clears in place (defense-in-depth; documents intent at the test site)
- **B**: remove the redundant clears and add a comment noting they are now defaulted by `runWt`

Either is acceptable. Recommend **A** for clarity — explicit > implicit at the test site, especially since this is the only test today that documents the pattern.

### 3. Audit other tests touching `--worktree-open` or `--app`

**Approach**: grep for `--worktree-open` and `--app` across all `*_test.go` files. For each occurrence, classify:

- **Safe**: `--worktree-open=skip`, `--app=open_here`, `--app=copy_*`, `--app=code` (when `code` not on PATH, fails before shelling out), explicit error-path tests with unknown apps
- **Needs isolation**: `--worktree-open=default`, `--worktree-open=<actual app>`, `--app=default`, `--app=tmux_window`, `--app=byobu_tab`, `--app=tmux_session`

After change #1, all tests that go through `runWt` inherit the isolation, so most "needs isolation" cases become "safe by default." Document the audit findings in spec; no further code changes expected unless an outlier is found.

### 4. Update review policy

**File**: `fab/project/code-review.md`

Append to the **Project-Specific Review Rules** section:

```markdown
- Tests that invoke the binary MUST NOT leak side-effects to the host system. Specifically: tests that exercise `--worktree-open` or `--app` codepaths SHALL either (a) use a non-side-effecting target (e.g., `--worktree-open=skip`, `--app=open_here`, `--app=copy_*`), (b) rely on `runWt`'s default env isolation (which clears `TMUX`, `BYOBU_*`, `TERM_PROGRAM`), or (c) explicitly register `t.Cleanup` to reap created windows/sessions/tabs. The first two are strongly preferred — actual window creation should be tested only by hand or via dedicated end-to-end fixtures, not in the standard unit-test suite.
```

### 5. (Optional) Add a regression test

A regression test for the env isolation itself: invoke `wt create --worktree-open default` via `runWt` from inside a tmux session (simulated by setting `TMUX=` then *re-overriding* with a fake value would defeat the purpose; instead, the test verifies `runWt`'s effective env contains `TMUX=` empty by reading `cmd.Env` post-construction).

This is overkill — the audit + review policy is sufficient. Leaving as a nice-to-have, not a requirement.

## Affected Memory

- (none) — no memory files exist yet (`docs/memory/index.md` is empty). The change touches `fab/project/code-review.md` (project-level review policy, not memory) and source code/tests. No spec-level behavioral changes for the runtime binary.

## Impact

- **Source code** (none modified — runtime is unchanged):
  - The runtime binary's behavior is identical pre- and post-change. Only test scaffolding is modified.
- **Tests**:
  - `src/cmd/wt/testutil_test.go` — `runWt` env defaults extended (~6 lines)
  - `src/cmd/wt/create_test.go` — review only; possibly no change after #1 lands
  - `src/cmd/wt/open_test.go` — review only; possibly no change after #1 lands
- **Project policy**:
  - `fab/project/code-review.md` — append project-specific rule about test side-effect isolation
- **Specs**:
  - (none) — runtime contract is unchanged. The launcher-contract.md spec from `evbf` is not affected.
- **External**: nothing. `hop` is unaffected. CI is unaffected (CI's tmux session, if any, is ephemeral).
- **Backwards compatibility**: existing tests must continue to pass. Tests that pass `TMUX=...` explicitly (currently zero, but possible in future) continue to work because user env overrides defaults.

## Open Questions

- (none) — both prior open questions resolved during `/fab-clarify` (see Clarifications below).

## Clarifications

### Session 2026-05-08 (tentative resolution)

| # | Action | Detail |
|---|--------|--------|
| 12 | Confirmed | Skip the optional regression test — audit + policy is sufficient |
| 13 | Confirmed | Audit produces zero code changes (assuming all tests use `runWt`); document in spec, not code |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Env-based isolation in `runWt`, not refactoring `OpenInApp`. | Per scope item 1 + Out of scope; reuses existing isolation pattern. | S:95 R:90 A:90 D:95 |
| 2 | Certain | Change type is `fix` — test-harness bug that leaks side-effects beyond the test process. | Per scope; matches keyword inference rule (#1 "fix"). | S:95 R:90 A:95 D:95 |
| 3 | Certain | The runtime binary is unchanged; only test scaffolding and review policy. | Bug is in test isolation, not in runtime behavior. Constitution IV (test what the user sees) is preserved — tests still use the real binary; only the env they pass to it is sanitized. | S:100 R:95 A:95 D:95 |
| 4 | Certain | Backwards compat: existing tests continue to pass after env defaults are added. | User-supplied env still overrides defaults (last-wins in Go's `exec.Cmd`); existing tests that explicitly clear vars become redundant-but-correct. | S:95 R:80 A:90 D:90 |
| 5 | Certain | Constitution IV is preserved — tests still use the real binary. | The change clears env that affects which codepath the binary chooses, not what the binary does. The "real binary" is still under test. | S:95 R:90 A:95 D:95 |
| 6 | Certain | The list of vars to clear matches what `IsTmuxSession`/`IsByobuSession`/`DetectDefaultApp` consult. | Verified by reading `src/internal/worktree/platform.go` and `apps.go` during intake; six vars total. | S:95 R:85 A:95 D:95 |
| 7 | Certain | Out of scope: refactoring `OpenInApp`, changes to `hop`, changes to launcher-contract.md. | Per scope's Out of scope section. | S:100 R:90 A:95 D:100 |
| 8 | Confident | `runWt` is the right chokepoint (vs. per-test isolation). | Test helpers are the natural place for cross-test invariants; per-test isolation duplicates the pattern and risks new tests forgetting it. Codebase already centralizes test setup in `testutil_test.go`. | S:85 R:85 A:90 D:85 |
| 9 | Confident | `HOME` is not defaulted by `runWt`; tests that need `HOME=t.TempDir()` continue to set it explicitly. | Different concern (HOME isolates `~/.cache/wt/last-app`, not the launcher choice). Centralizing it would either break real-HOME tests or duplicate `t.TempDir` logic in the helper. Worth revisiting if `last-app` cache pollution becomes an observed issue. | S:80 R:80 A:85 D:80 |
| 10 | Confident | Existing test `TestOpen_AppDefault` keeps its explicit env clears (option A) for clarity. | Defense-in-depth + documents intent at the test site. Cost is ~6 lines of redundancy vs. clarity benefit. | S:80 R:95 A:85 D:75 |
| 11 | Confident | Code-review.md gets a new project-specific rule about test side-effect isolation. | Per scope item 4; codifies the lesson so future contributors don't reintroduce the bug. Aligns with existing `code-review.md` structure. | S:90 R:95 A:90 D:90 |
| 12 | Certain | The optional regression test (#5) is NOT included. | Clarified — user confirmed. Brittle (asserts on internals); audit + policy is sufficient coverage. | S:95 R:90 A:80 D:75 |
| 13 | Certain | Audit of existing `--worktree-open`/`--app` tests is documented in the spec but does not produce code changes (assuming all are covered by #1's defaults). | Clarified — user confirmed. If audit surfaces a test that bypasses `runWt`, it becomes a discovered task. | S:95 R:85 A:80 D:75 |

13 assumptions (9 certain, 4 confident, 0 tentative, 0 unresolved).
