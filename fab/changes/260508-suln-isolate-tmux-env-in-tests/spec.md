# Spec: Isolate tmux/byobu env in test harness

**Change**: 260508-suln-isolate-tmux-env-in-tests
**Created**: 2026-05-08
**Affected memory**: (none — `docs/memory/` is empty; runtime contract unchanged)

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Env-based isolation in `runWt`; no runtime code changes. | Confirmed from intake #1; reaffirmed via Design Decision 1. | S:95 R:90 A:90 D:95 |
| 2 | Certain | Change type is `fix` — test-harness bug. | Confirmed from intake #2. | S:95 R:90 A:95 D:95 |
| 3 | Certain | Runtime binary is unchanged; only test scaffolding and review policy. | Confirmed from intake #3; encoded as "no source files modified" — only test files and `fab/project/code-review.md`. | S:100 R:95 A:95 D:95 |
| 4 | Certain | Backwards compat: existing tests pass after env defaults are added. | Confirmed from intake #4; Go's last-wins semantics for `exec.Cmd.Env` ensures caller env overrides defaults. | S:95 R:80 A:90 D:90 |
| 5 | Certain | Constitution IV preserved — tests use the real binary. | Confirmed from intake #5. | S:95 R:90 A:95 D:95 |
| 6 | Certain | Cleared vars match what `IsTmuxSession`/`IsByobuSession`/`DetectDefaultApp` consult. | Confirmed from intake #6; verified by reading `src/internal/worktree/platform.go` and `apps.go`. | S:95 R:85 A:95 D:95 |
| 7 | Certain | Out of scope: `OpenInApp` refactoring, `hop` changes, launcher-contract.md changes. | Confirmed from intake #7; encoded in Non-Goals. | S:100 R:90 A:95 D:100 |
| 8 | Certain | Skip the optional regression test (no `cmd.Env` assertion). | Clarified — user confirmed (intake #12). | S:95 R:90 A:80 D:75 |
| 9 | Certain | Audit produces zero code changes; documented in spec only. | Clarified — user confirmed (intake #13). If a discovered task surfaces, it joins the plan. | S:95 R:85 A:80 D:75 |
| 10 | Confident | `runWt` is the right chokepoint (vs. per-test isolation). | Confirmed from intake #8; reaffirmed via Design Decision 1. | S:85 R:85 A:90 D:85 |
| 11 | Confident | `HOME` is NOT defaulted by `runWt`. | Confirmed from intake #9; reaffirmed via Design Decision 3. | S:80 R:80 A:85 D:80 |
| 12 | Confident | `TestOpen_AppDefault` keeps its explicit env clears (option A). | Confirmed from intake #10; defense-in-depth + documents intent. | S:80 R:95 A:85 D:75 |
| 13 | Confident | `code-review.md` gets a new project-specific rule. | Confirmed from intake #11; encoded as a dedicated requirement. | S:90 R:95 A:90 D:90 |
| 14 | Certain | Audit scope: `src/cmd/wt/` and `src/internal/worktree/` `*_test.go` files. | Discovered during spec generation; matches the codebase layout. | S:95 R:90 A:95 D:95 |
| 15 | Certain | Empty-value env clears (`TMUX=`) over key removal (`os.Environ` filtering). | Discovered during spec generation; reaffirmed via Design Decision 2. | S:90 R:90 A:90 D:90 |

15 assumptions (11 certain, 4 confident, 0 tentative, 0 unresolved). <!-- clarified: corrected count — rows 1-9 + 14-15 are Certain (11), 10-13 are Confident (4) -->

