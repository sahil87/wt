# Spec: Add --skip-brew-update flag to update command

**Change**: 260531-ipe5-skip-brew-update-flag
**Created**: 2026-05-31
**Affected memory**: `docs/memory/wt-cli/update-command-contract.md`

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Flag name EXACTLY `--skip-brew-update`, cobra `BoolVar`, default `false`, long-form only, `Args: cobra.NoArgs` retained | Confirmed from intake #1; fixed by cross-toolkit contract and constitution principle II | S:98 R:90 A:95 D:98 |
| 2 | Certain | When set, skip ONLY the `brew update` block (ctx + exec + error handling); `brew info`, up-to-date short-circuit, `brew upgrade` unchanged; default preserved exactly | Confirmed from intake #2; explicit and repeated in the contract | S:98 R:78 A:95 D:95 |
| 3 | Certain | Preserve output routing: brew update/info capture stdout (stderr→os.Stderr), brew upgrade inherits stdin/stdout/stderr | Confirmed from intake #3; documented in current `Run` doc comment | S:95 R:80 A:95 D:95 |
| 4 | Confident | `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` — new param leads, writers stay paired at tail | Confirmed from intake #4; one internal caller, trivially updated | S:82 R:80 A:82 D:72 |
| 5 | Confident | brew-not-found stays detected via `brew info`'s existing `exec.ErrNotFound`→`ErrBrewNotFound` handling; single-line stderr contract preserved | Confirmed from intake #5; update.go L82–85 shows identical handling already present | S:78 R:78 A:88 D:78 |
| 6 | Certain | Test bypasses `isBrewInstalled()` via the test-only `WT_TEST_FORCE_BREW=1` env-var seam mirroring `apps.go:201`; fake `brew` on PATH records invocations | Clarified (auto) — env-var name fixed; `WT_TEST_NO_LAUNCH=1` precedent dictates the name unambiguously, leaving no open choice; reversible, test-only | S:95 R:82 A:62 D:58 |
| 7 | Confident | New memory file `wt-cli/update-command-contract` created at hydrate documenting the flag contract | Confirmed from intake #7; wt-cli domain houses command contracts | S:72 R:85 A:80 D:75 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
