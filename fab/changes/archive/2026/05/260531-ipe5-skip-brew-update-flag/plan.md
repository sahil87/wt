# Plan: Add --skip-brew-update flag to update command

**Change**: 260531-ipe5-skip-brew-update-flag
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- Changing `brew info` version-check behavior, the up-to-date short-circuit, or `brew upgrade` behavior — only the `brew update` refresh is made conditional.
- Refactoring subprocess invocation into a runner/interface — direct `exec.CommandContext` calls are retained per the existing codebase convention.
- Adding a short flag, flag aliases, environment-variable binding, or config-file binding for the new flag.
- Changing the `Run` writer parameters' stream routing or the wrapper messages emitted on `out`/`errOut`.

## wt update: --skip-brew-update flag

### Requirement: Flag definition and default
The `wt update` command SHALL expose a boolean flag named exactly `--skip-brew-update`, registered as a cobra `BoolVar` with default value `false` and long-form name only (no short flag). The command SHALL retain `Args: cobra.NoArgs`. The flag's value SHALL be passed into `update.Run` as a `skipBrewUpdate bool` parameter.

#### Scenario: Flag present and defaults to false
- **GIVEN** the `wt` binary built from this change
- **WHEN** the user runs `wt update --help`
- **THEN** the help output SHALL list a `--skip-brew-update` flag
- **AND** no single-letter short alias is registered for it
- **AND** when the flag is omitted, the value passed to `update.Run` SHALL be `false`

#### Scenario: Flag rejects positional arguments
- **GIVEN** the `wt update` command
- **WHEN** the user runs `wt update some-positional-arg`
- **THEN** the command SHALL reject the call as an argument error (`cobra.NoArgs` behavior is unchanged)

### Requirement: Skipping the brew update refresh
When `skipBrewUpdate` is `true`, `update.Run` SHALL NOT invoke `brew update`. The `brew update` subprocess block — its `context.WithTimeout`, the `exec.CommandContext(ctx, "brew", "update", "--quiet")` call, and that call's error handling — SHALL be skipped in its entirety. No other step SHALL be altered: the `brew info` version check (`brewLatestVersion`), the up-to-date short-circuit, and the interactive `brew upgrade` SHALL execute exactly as in the default path.

#### Scenario: Flag set skips brew update but still upgrades
- **GIVEN** a brew-installed `wt` whose current version differs from the latest reported by `brew info`
- **WHEN** `update.Run` is invoked with `skipBrewUpdate = true`
- **THEN** `brew update` SHALL NOT be executed
- **AND** `brew info` SHALL be executed to determine the latest version
- **AND** `brew upgrade sahil87/tap/wt` SHALL be executed because the versions differ

#### Scenario: Flag set, already up to date
- **GIVEN** a brew-installed `wt` whose current version equals the latest reported by `brew info`
- **WHEN** `update.Run` is invoked with `skipBrewUpdate = true`
- **THEN** `brew update` SHALL NOT be executed
- **AND** `brew info` SHALL be executed
- **AND** the up-to-date short-circuit SHALL fire ("Already up to date") and `brew upgrade` SHALL NOT run

### Requirement: Default behavior preserved exactly
When `skipBrewUpdate` is `false` (the default, flag absent), `update.Run` SHALL behave byte-for-byte as it did before this change: `brew update --quiet` runs first, then `brew info`, then the up-to-date short-circuit, then `brew upgrade` when versions differ.

#### Scenario: Flag absent preserves brew update
- **GIVEN** a brew-installed `wt` whose current version differs from the latest reported by `brew info`
- **WHEN** `update.Run` is invoked with `skipBrewUpdate = false`
- **THEN** `brew update` SHALL be executed first
- **AND** `brew info` SHALL be executed
- **AND** `brew upgrade sahil87/tap/wt` SHALL be executed

#### Scenario: Non-brew install unaffected by flag
- **GIVEN** a `wt` binary NOT installed via Homebrew (not under `/Cellar/`)
- **WHEN** `update.Run` is invoked with any value of `skipBrewUpdate`
- **THEN** the manual-update hint SHALL be printed to `out` and `Run` SHALL return `nil` without invoking any brew subcommand (the `isBrewInstalled()` short-circuit is unchanged)

### Requirement: Output routing preserved
`update.Run` SHALL preserve the existing stream routing for every subprocess regardless of `skipBrewUpdate`: `brew update` and `brew info` capture stdout (for parse/discard) while routing stderr to `os.Stderr`; `brew upgrade` inherits `os.Stdin`, `os.Stdout`, and `os.Stderr` so its tty-aware progress renders inline. The wrapper messages written to the `out` and `errOut` writers SHALL be unchanged.

#### Scenario: Upgrade remains interactive when flag is set
- **GIVEN** `update.Run` invoked with `skipBrewUpdate = true` and a version mismatch
- **WHEN** the `brew upgrade` subprocess is constructed
- **THEN** its `Stdin`, `Stdout`, and `Stderr` SHALL be `os.Stdin`, `os.Stdout`, `os.Stderr` respectively (interactive routing unchanged)

### Requirement: Brew-not-found detection preserved
With the `brew update` call skipped, brew-not-found detection SHALL still occur. The first brew invocation that runs (`brew info` via `brewLatestVersion`) already maps `exec.ErrNotFound` to `ErrBrewNotFound`, and `update.Run` SHALL return that sentinel so the cobra wrapper maps it to the typed exit. The single-line stderr contract (exactly one "brew not found" hint) SHALL be preserved.

#### Scenario: Brew missing with flag set
- **GIVEN** a brew-installed-looking `wt` (passes `isBrewInstalled`) but `brew` is absent from PATH
- **WHEN** `update.Run` is invoked with `skipBrewUpdate = true`
- **THEN** `brew info` SHALL surface `exec.ErrNotFound`
- **AND** `update.Run` SHALL return `ErrBrewNotFound`
- **AND** exactly one "brew not found" hint SHALL be written to `errOut`

### Requirement: Test coverage for skip-vs-run behavior
The update package test suite SHALL include a test asserting that `update.Run` with `skipBrewUpdate = true` does NOT run `brew update` but DOES run `brew upgrade` (and `brew info`), and that with `skipBrewUpdate = false` it DOES run `brew update`. The test SHALL follow the repository's existing subprocess-testing convention (the test-only `WT_TEST_FORCE_BREW=1` environment-variable seam that forces `isBrewInstalled()` true, plus a fake `brew` executable on `PATH`) rather than introducing subprocess-runner indirection. Existing update-package tests SHALL be updated to the new `Run` signature and SHALL continue to pass. `go build ./...` and `go test ./internal/update/...` SHALL succeed before the PR is opened.

#### Scenario: Test observes subcommand invocation
- **GIVEN** a fake `brew` executable placed first on `PATH` that records each invocation's first argument and emits valid `brew info --json=v2` output
- **AND** `isBrewInstalled()` forced true via the test-only `WT_TEST_FORCE_BREW=1` env-var seam
- **WHEN** the test invokes `Run` with `skipBrewUpdate = true`, then again with `skipBrewUpdate = false`
- **THEN** the skip run's recorded invocations SHALL contain `info` and `upgrade` but NOT `update`
- **AND** the default run's recorded invocations SHALL contain `update`

#### Scenario: Existing tests pass under new signature
- **GIVEN** the updated `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` signature
- **WHEN** `go test ./internal/update/...` runs
- **THEN** all existing tests (`TestNormalizeVersion`, `TestRunNonBrewInstall`, `TestIsBrewInstalledReturnsBool`) SHALL pass with their call sites updated to the new signature

## Design Decisions

1. **`skipBrewUpdate` is the leading parameter of `Run`**: `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)`.
   - *Why*: Keeps the two `io.Writer` parameters adjacent at the tail (the existing convention) and reads as a mode flag preceding the operands. Only one caller exists (`cmd/wt/update.go`), so the signature change is trivially absorbed.
   - *Rejected*: Appending `skipBrewUpdate` after the writers (`Run(currentVersion, out, errOut, skipBrewUpdate)`) splits the natural writer pairing and buries the mode flag behind the I/O operands.

2. **Test seam via an environment-variable bypass of `isBrewInstalled()`, not a subprocess-runner refactor**: the test-only env var `WT_TEST_FORCE_BREW` (value `"1"`) makes `isBrewInstalled()` return true; a fake `brew` on `PATH` records invocations.
   <!-- clarified: env-var name fixed to WT_TEST_FORCE_BREW=1 — mirrors the WT_TEST_NO_LAUNCH=1 convention at src/internal/worktree/apps.go:201 (checked via os.Getenv(...) == "1"); name was previously deferred to apply but the precedent dictates it unambiguously, so it is settled here to remove the only Tentative item -->
   - *Note*: applies the same `os.Getenv("WT_TEST_FORCE_BREW") == "1"` gate shape as the `apps.go` precedent; never set in production.
   - *Why*: Mirrors the established codebase convention at `src/internal/worktree/apps.go:201` (`WT_TEST_NO_LAUNCH=1` short-circuits real launches for tests). Keeps every `exec.CommandContext` call site untouched, adds zero production indirection (the env var is never set in production), and satisfies the contract's "match existing subprocess convention (do NOT refactor)" requirement.
   - *Rejected*: `var execCommand = exec.Command` injection or an `interface`-based runner — these are the very refactor the contract forbids. A build-tag stub file is heavier and splits the logic across build configurations.

## Clarifications

### Session 2026-05-31 (auto)

| Item | Action | Detail |
|------|--------|--------|
| Assumption #6 | Resolved | Test-seam env-var name fixed to `WT_TEST_FORCE_BREW=1` (gated via `os.Getenv(...) == "1"`), mirroring the `WT_TEST_NO_LAUNCH=1` precedent at `src/internal/worktree/apps.go:201`. Tentative → Certain. |


## Tasks

### Phase 1: Core Implementation

- [x] T001 In `src/internal/update/update.go`, change `func Run(currentVersion string, out, errOut io.Writer) error` to `func Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer) error` (new `skipBrewUpdate` param leads). Update the `Run` doc comment to mention the new parameter and that it skips only the `brew update` metadata refresh.
- [x] T002 In `src/internal/update/update.go`, wrap ONLY the `brew update` block (the `context.WithTimeout` + `exec.CommandContext(ctx, "brew", "update", "--quiet")` + its error handling, ~L67-78) in `if !skipBrewUpdate { ... }`. `brew info`, the up-to-date short-circuit, and the interactive `brew upgrade` stay exactly as-is.
- [x] T003 In `src/internal/update/update.go`, add a test-only seam to `isBrewInstalled()`: return `true` when `os.Getenv("WT_TEST_FORCE_BREW") == "1"`, otherwise the current `/Cellar/` logic. Add a short "Test seam:" comment mirroring the `WT_TEST_NO_LAUNCH` seam at `src/internal/worktree/apps.go:201`.
- [x] T004 In `src/cmd/wt/update.go`, register a real cobra bool flag `--skip-brew-update` (default `false`, long-form only). Refactor the constructor to `var skipBrewUpdate bool` + `cmd := &cobra.Command{...}` + `cmd.Flags().BoolVar(&skipBrewUpdate, "skip-brew-update", false, "<description>")` + `return cmd`. Pass `skipBrewUpdate` as the FIRST arg to `update.Run(...)`. Keep `Args: cobra.NoArgs` and the existing `ErrBrewNotFound` handling.

### Phase 2: Tests

- [x] T005 In `src/internal/update/update_test.go`, update the existing `Run("v0.0.3", &stdout, &stderr)` call site in `TestRunNonBrewInstall` to the new signature `Run(false, "v0.0.3", &stdout, &stderr)`.
- [x] T006 In `src/internal/update/update_test.go`, add `TestRunSkipBrewUpdate`: create a temp dir; write an executable (0755) fake `brew` shell script that appends its first arg to a log file and, for `info`, prints valid `brew info --json=v2` JSON for `sahil87/tap/wt` with a stable version `9.9.9` and exits 0; for `update`/`upgrade` just logs and exits 0. Set `t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))` and `t.Setenv("WT_TEST_FORCE_BREW", "1")`. Pass `currentVersion` `"v0.0.0"`. Run with `skipBrewUpdate=true` → assert log contains `info` and `upgrade` but NOT `update`; reset log, run with `skipBrewUpdate=false` → assert log contains `update`.

### Phase 3: Verification

- [x] T007 From `src/`, run `go build ./...` and `go test ./internal/update/...`; fix any failures at the root cause. Optionally run `go vet ./internal/update/...` and `gofmt -l` on changed files.

## Execution Order

- T001 → T002 → T003 are sequential edits within `update.go`.
- T004 depends on T001 (new signature must exist before the caller passes the extra arg).
- T005, T006 depend on T001-T004.
- T007 runs last.

## Acceptance

### Functional Completeness

- [ ] A-001 Flag definition and default: `wt update` registers a cobra `BoolVar` flag named exactly `--skip-brew-update` (default `false`, no short alias); `Args: cobra.NoArgs` retained; the value is passed as the leading `skipBrewUpdate bool` arg to `update.Run`.
- [ ] A-002 Skipping the brew update refresh: when `skipBrewUpdate` is `true`, the `brew update` block (ctx + `exec.CommandContext` + error handling) is skipped in its entirety while `brew info`, the up-to-date short-circuit, and `brew upgrade` are unchanged.
- [ ] A-003 Default behavior preserved: when `skipBrewUpdate` is `false`, `brew update --quiet` runs first, then `brew info`, then short-circuit, then `brew upgrade` on version mismatch — byte-for-byte as before.
- [ ] A-004 Output routing preserved: `brew update`/`brew info` capture stdout with stderr→`os.Stderr`; `brew upgrade` inherits `os.Stdin/os.Stdout/os.Stderr`; wrapper messages on `out`/`errOut` unchanged.

### Behavioral Correctness

- [ ] A-005 `Run` signature is `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` with the doc comment updated to describe the new parameter and that it skips only the metadata refresh.
- [ ] A-006 `isBrewInstalled()` returns `true` when `os.Getenv("WT_TEST_FORCE_BREW") == "1"`, otherwise its current `/Cellar/` logic; production behavior is unchanged (env var never set in production); a "Test seam:" comment documents it.

### Scenario Coverage

- [ ] A-007 `TestRunSkipBrewUpdate` exists: with `skipBrewUpdate=true` the recorded brew invocations contain `info` and `upgrade` but NOT `update`; with `skipBrewUpdate=false` they contain `update`. The test uses a fake `brew` on `PATH` + `WT_TEST_FORCE_BREW=1`.
- [ ] A-008 Existing tests pass under the new signature: `TestRunNonBrewInstall` call site updated to `Run(false, ...)`; `TestNormalizeVersion`, `TestIsBrewInstalledReturnsBool` still pass.

### Edge Cases & Error Handling

- [ ] A-009 Brew-not-found detection preserved: with `brew update` skipped, the first brew call (`brew info`) still maps `exec.ErrNotFound` to `ErrBrewNotFound`; no new handling added.

### Code Quality

- [ ] A-010 Pattern consistency: new code follows the naming and structural patterns of surrounding code (env-var seam mirrors `apps.go:201`; cobra flag registration follows the standard pattern).
- [ ] A-011 No unnecessary duplication: existing utilities reused; no subprocess-runner indirection introduced (direct `exec.CommandContext` retained).
- [ ] A-012 No magic strings: `WT_TEST_FORCE_BREW` / `--skip-brew-update` used consistently; no unexplained literals introduced.

## Notes

- `go build ./...` and `go test ./internal/update/...` are run from `src/` (module root with `go.mod`).
- The env-var seam is test-only and never set in production, so production behavior is unchanged.
