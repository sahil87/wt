# Plan: wt update subcommand

**Change**: 260508-4c7g-wt-update-subcommand
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- A `--check` flag (print latest, exit non-zero if behind, no upgrade) — out of scope; preserve parity with `hop update`.
- A self-replacing-binary update path (download tarball, swap in place) — Homebrew is the only supported install channel covered here.
- Automatic background update checks at startup — explicit user invocation only.
- Refactoring existing wt subprocess call sites to a shared `internal/proc` wrapper — see Design Decision 1.

## CLI: `wt update`

### Requirement: Subcommand registration
The binary SHALL register an `update` subcommand on the root cobra command. The subcommand MUST set `Args = cobra.NoArgs`, MUST inherit the root's `SilenceUsage` and `SilenceErrors` settings, and MUST be visible in `wt --help` output.

#### Scenario: Help output lists the subcommand
- **GIVEN** the `wt` binary is built
- **WHEN** the user runs `wt --help`
- **THEN** the output SHALL include a line for the `update` subcommand with its short description (`self-update the wt binary via Homebrew`)

#### Scenario: Extra positional args rejected
- **GIVEN** the `wt update` subcommand is registered
- **WHEN** the user runs `wt update extra-arg`
- **THEN** the subcommand SHALL return a non-nil error from cobra (matching `cobra.NoArgs` enforcement)
- **AND** the binary SHALL exit with `ExitGeneralError`

### Requirement: Brew-install detection
The implementation SHALL detect whether the running binary was installed via Homebrew by resolving `os.Executable()` to its real path (via `filepath.EvalSymlinks`) and checking whether the resolved path contains the substring `/Cellar/`.

#### Scenario: Test binary is not under /Cellar/
- **GIVEN** the running binary is a `go test` artifact (e.g., a tempdir-built test binary)
- **WHEN** the brew-install detector runs
- **THEN** it SHALL return `false`
- **AND** SHALL NOT panic or error

#### Scenario: Symlink resolution failure
- **GIVEN** `os.Executable()` succeeds but `filepath.EvalSymlinks` fails (e.g., the binary was deleted while running)
- **WHEN** the brew-install detector runs
- **THEN** it SHALL return `false` rather than propagating the error

### Requirement: Non-Homebrew install fast-path
The subcommand SHALL detect non-Homebrew installs (e.g., `just local-install` builds in `~/.local/bin`) and return immediately with a manual-update hint. No `brew` invocation MAY occur in this path.

#### Scenario: Locally-built binary prints manual-update hint
- **GIVEN** the running binary's resolved path does not contain `/Cellar/`
- **WHEN** the user runs `wt update`
- **THEN** stdout SHALL contain the line `wt {currentVersion} was not installed via Homebrew.`
- **AND** stdout SHALL contain the line `Update manually, or reinstall with: brew install sahil87/tap/wt`
- **AND** the subcommand SHALL return `nil` (exit code 0)
- **AND** no `brew` subprocess SHALL be invoked

### Requirement: Brew-not-found handling
When the detector reports a Homebrew install but `brew` itself is not on `PATH`, the subcommand SHALL print a single-line hint to stderr and return without attempting further brew calls. The cobra wrapper MUST suppress the redundant cobra error print so the user sees only one line.

#### Scenario: Brew binary missing
- **GIVEN** the binary's resolved path contains `/Cellar/` (Homebrew install detected)
- **AND** the `brew` executable is not on `PATH`
- **WHEN** the user runs `wt update`
- **THEN** stderr SHALL contain exactly one line: `wt update: brew not found on PATH.`
- **AND** stderr SHALL NOT contain a duplicate cobra-printed error
- **AND** the binary SHALL exit with `ExitGeneralError`

### Requirement: Brew tap formula identifier
The implementation SHALL reference the tap formula by its fully-qualified name `sahil87/tap/wt` in every brew invocation that needs it (`brew info`, `brew upgrade`) and in the manual-install hint. The formula identifier MUST be a single named constant in `src/internal/update/`.

#### Scenario: Formula constant used in all references
- **GIVEN** the implementation is built
- **WHEN** the source for `internal/update` is grepped for the literal `sahil87/tap`
- **THEN** every occurrence SHALL be the named constant or a reference to it (no inline string duplication)

### Requirement: brew update execution
The subcommand SHALL run `brew update --quiet` with a 30-second timeout before checking for the latest version. Subprocess stdout SHALL be captured (and discarded — output is not user-visible). Subprocess stderr SHALL pass through to the parent process's stderr.

#### Scenario: brew update timeout
- **GIVEN** `brew update --quiet` has been started
- **WHEN** 30 seconds elapse without completion
- **THEN** the context SHALL be cancelled
- **AND** the subprocess SHALL be killed
- **AND** the subcommand SHALL return a wrapped error of the form `brew update failed: {underlying}`

### Requirement: Latest-version query
The subcommand SHALL query `brew info --json=v2 sahil87/tap/wt` with a 30-second timeout and parse `formulae[0].versions.stable` from the JSON output. If the array is empty or `stable` is empty, the subcommand SHALL return an error of the form `could not determine latest version: no stable version found in brew info output`.

#### Scenario: Successful version query
- **GIVEN** `brew info --json=v2 sahil87/tap/wt` returns valid JSON with a populated `versions.stable`
- **WHEN** the latest-version query runs
- **THEN** the subcommand SHALL receive the bare version string (e.g., `0.1.1`, no leading `v`)

#### Scenario: Empty formulae array
- **GIVEN** `brew info` returns valid JSON but `formulae` is empty
- **WHEN** the latest-version query runs
- **THEN** the subcommand SHALL return an error containing `no stable version found in brew info output`

#### Scenario: Malformed JSON
- **GIVEN** `brew info` returns non-JSON output
- **WHEN** the latest-version query runs
- **THEN** the subcommand SHALL return an error wrapping the JSON unmarshal failure

### Requirement: Version comparison and no-op
The subcommand SHALL compare the current binary version (from `main.version`, stamped via `-ldflags "-X main.version=..."`) against the brew-reported latest after stripping at most one leading `v` from each. If the normalized strings are equal, the subcommand SHALL print `Already up to date ({currentVersion}).` to stdout and return `nil` without invoking `brew upgrade`.

#### Scenario: Versions equal after normalization
- **GIVEN** `currentVersion = "v0.1.0"` and brew-reported latest `"0.1.0"`
- **WHEN** version comparison runs
- **THEN** stdout SHALL contain `Already up to date (v0.1.0).`
- **AND** `brew upgrade` SHALL NOT be invoked
- **AND** the subcommand SHALL return `nil`

#### Scenario: Single leading v stripped
- **GIVEN** input `"vvv1.0.0"`
- **WHEN** version normalization runs
- **THEN** the result SHALL be `"vv1.0.0"` (only one leading `v` stripped)

### Requirement: brew upgrade execution
When the brew-reported latest differs from the normalized current version, the subcommand SHALL print `Updating {currentVersion} → v{normalizedLatest}...` to stdout, then invoke `brew upgrade sahil87/tap/wt` with a 120-second timeout. The subprocess MUST inherit `os.Stdin`, `os.Stdout`, and `os.Stderr` so brew's tty-aware progress output renders inline.

#### Scenario: Successful upgrade
- **GIVEN** the latest version differs from the current
- **AND** `brew upgrade sahil87/tap/wt` exits with code 0
- **WHEN** the upgrade step runs
- **THEN** brew's progress output SHALL render to the user's terminal as the upgrade proceeds
- **AND** stdout SHALL contain `Updated to v{normalizedLatest}.` after completion
- **AND** the subcommand SHALL return `nil`

#### Scenario: Upgrade non-zero exit
- **GIVEN** `brew upgrade` exits with a non-zero code (e.g., 1)
- **WHEN** the upgrade step runs
- **THEN** the subcommand SHALL return an error of the form `brew upgrade exited with code {code}`

#### Scenario: Upgrade subprocess fails to start
- **GIVEN** `brew upgrade` cannot be started (e.g., `brew` removed from PATH between calls)
- **WHEN** the upgrade step runs
- **THEN** the subcommand SHALL return an error of the form `brew upgrade failed: {underlying}`
- **AND** if the underlying error matches "executable not found on PATH", the cobra wrapper SHALL print the brew-not-found hint and suppress the cobra error print

### Requirement: Output stream contract
The `Run` function SHALL accept `out io.Writer` and `errOut io.Writer` parameters. Wrapper messages emitted by `internal/update` itself ("Current version:", "Checking for updates...", "Already up to date", "Updating ...", "Updated to ...", and the manual-install / brew-not-found hints) MUST be routed through `out` or `errOut` per their nature. Subprocess streams (brew's stdout/stderr from `brew update`, `brew info`, and `brew upgrade`) MUST NOT be routed through `out`/`errOut` — they go directly to `os.Stdout`/`os.Stderr` (or are captured for parsing in the case of `brew info`).

#### Scenario: Test injects writers
- **GIVEN** a test calls `update.Run("v0.0.3", &stdout, &stderr)` on a non-brew binary
- **WHEN** the function runs
- **THEN** the manual-install hint SHALL appear in the test's `stdout` buffer
- **AND** the test's `stderr` buffer SHALL be empty
- **AND** no actual brew subprocess SHALL be invoked

### Requirement: Test coverage
The change SHALL include a test file at `src/cmd/wt/update_test.go` covering cobra wiring (subcommand reachable via the existing `runWt` helper, `--help` lists it, `cobra.NoArgs` rejects extras) and a test file at `src/internal/update/update_test.go` covering version normalization (table-driven), the non-brew-install branch, and a smoke test for the brew-install detector.
<!-- clarified: wt's actual cmd test helper is `runWt(t, dir, env, args...)` (in `src/cmd/wt/testutil_test.go`), not hop's `runArgs(t, args...)`. The intake's Open Questions section flagged this divergence; verified by inspecting `testutil_test.go` and existing tests (e.g., `init_test.go`, which calls `runWt(t, repo, nil, "init")`). Spec scenarios below use `runWt` accordingly. -->

#### Scenario: Cobra wiring test exercises non-brew branch
- **GIVEN** the test binary is not located under `/Cellar/`
- **WHEN** `runWt(t, repo, nil, "update")` is invoked (where `repo` is from `createTestRepo(t)`)
- **THEN** the captured stdout SHALL contain `was not installed via Homebrew`
- **AND** the test SHALL NOT depend on `brew` being installed

#### Scenario: Help test
- **GIVEN** the test binary is built
- **WHEN** `runWt(t, repo, nil, "--help")` is invoked
- **THEN** the captured stdout SHALL contain a line matching `wt update`

#### Scenario: NoArgs test
- **GIVEN** the `update` subcommand is registered with `cobra.NoArgs`
- **WHEN** `runWt(t, repo, nil, "update", "extra")` is invoked
- **THEN** the result SHALL exit with a non-zero code (cobra's `NoArgs` validation failure surfaces via `main.go`'s error path, not a direct `error` return — `runWt` invokes the built binary as a subprocess)

#### Scenario: normalizeVersion table cases
- **GIVEN** the input table `{"v0.0.3"→"0.0.3", "0.0.3"→"0.0.3", ""→"", "v"→"", "vvv1.0.0"→"vv1.0.0"}`
- **WHEN** the table-driven test runs
- **THEN** every case SHALL pass

### Requirement: Exit-code mapping
The subcommand SHALL map outcomes to exit codes via the existing `main.go` error path. Successful operations (upgrade, no-op, non-brew install) MUST return `nil` from `RunE` and exit with `ExitSuccess` (0). Brew failures and JSON parse failures MUST return non-nil errors that exit with `ExitGeneralError` (1). The brew-not-found case MUST exit with `ExitGeneralError` (1) AND MUST NOT produce a duplicate cobra error line.

#### Scenario: Successful no-op exits 0
- **GIVEN** the user is already at the latest version
- **WHEN** `wt update` runs
- **THEN** the binary SHALL exit with code 0

#### Scenario: Brew failure exits 1
- **GIVEN** `brew update --quiet` fails (e.g., network error)
- **WHEN** `wt update` runs
- **THEN** the binary SHALL exit with code 1
- **AND** stderr SHALL contain the wrapped error message

## Documentation: `docs/specs/cli-surface.md`

### Requirement: Add `wt update` entry to CLI surface spec
The change SHALL append a new section `## wt update` to `docs/specs/cli-surface.md` between the existing subcommand sections (alphabetical order is not currently enforced; placement near `init` or at end of subcommand sections is acceptable). The section MUST document: no flags, no positional args, behavior summary (manual-install hint, brew-not-found hint, no-op when up to date, upgrade flow, exit-code mapping), and a pointer to `internal/update` for implementation detail.

#### Scenario: Spec section exists and matches behavior
- **GIVEN** `docs/specs/cli-surface.md` is read
- **WHEN** the file content is searched for `## wt update`
- **THEN** exactly one section matches
- **AND** the section content SHALL describe the four user-facing outcomes (Homebrew upgrade succeeds, already up to date, not installed via Homebrew, brew not on PATH) and SHALL list the relevant exit codes

## Design Decisions

1. **No `internal/proc` wrapper for wt**:
   - *Why*: Existing wt code uses `os/exec` directly (context.go, stash.go, apps.go, cmd/wt/init.go), and the constitution does not mandate centralized subprocess routing. Introducing an `internal/proc` package solely for one new command (`update`) would be over-scoped relative to current usage patterns.
   - *Rejected*: Mirroring hop's `internal/proc` package — would require either porting `proc` and refactoring existing call sites for consistency (out of scope for this change) or creating a one-off wrapper used only by `update` (worse than direct `os/exec` calls because it adds a layer for one consumer).

2. **Brew-not-found exits directly from cobra wrapper**:
   - *Why*: hop suppresses double-printing via an `errSilent` sentinel; wt's `main.go` has no such sentinel. The cleanest port is for the `update.Run` function to write the user-visible hint to its `errOut` parameter and return a sentinel error (`update.ErrBrewNotFound`); the cobra wrapper in `cmd/wt/update.go` detects the sentinel and exits directly with the typed code, bypassing both cobra's automatic error printing and `main.go`'s error formatter. The user sees exactly one hint line; the binary exits with `ExitGeneralError` (1).
   - *Decision detail*: To preserve the "exactly one stderr line" requirement, the cobra wrapper SHALL call `os.Exit(wt.ExitGeneralError)` after `update.Run` has already printed the hint. `wt.ExitWithError(...)` is NOT used here because it always prints a structured error first, which would violate the single-line contract. Implementation detail belongs in `cmd/wt/update.go`'s `RunE`.
   - *Rejected*: Returning the sentinel error from `RunE` and accepting one extra cobra-printed error line — noisier, contradicts the user-facing hint that already appears.
   - *Rejected*: Using `wt.ExitWithError(wt.ExitGeneralError, ...)` from the cobra wrapper — emits a second structured error line, violating the single-line stderr contract.

3. **Reuse existing `main.version` package var**:
   - *Why*: `main.version` is already declared in `src/cmd/wt/main.go` and stamped via `-ldflags "-X main.version=..."` per `docs/specs/build-and-release.md`. No build wiring changes are needed; the cobra wrapper passes `version` directly to `update.Run`.
   - *Rejected*: A new `internal/version` package — unwarranted abstraction for a single consumer.

4. **File layout mirrors hop**: `src/cmd/wt/update.go` (cobra wiring) + `src/internal/update/update.go` (logic), with paired `_test.go` files in the same packages.
   - *Why*: Maintains the wt convention (`cmd/` thin, logic under `internal/`) and matches hop's split for easy cross-reference during future maintenance.
   - *Rejected*: Single file under `cmd/wt/update.go` containing all logic — violates Constitution Principle V (Internal Package Boundary).


## Tasks

### Phase 1: Setup

- [x] T001 Create new package directory `src/internal/update/` (no scaffolding files — will be populated in Phase 2 by T002)

### Phase 2: Core Implementation

- [x] T002 Implement `src/internal/update/update.go`: package doc comment (wt-flavored, drops hop's Constitution Principle I citation), `brewFormula = "sahil87/tap/wt"` constant, timeout constants (30s/30s/120s), exported sentinel `ErrBrewNotFound`, `Run(currentVersion string, out, errOut io.Writer) error`, `brewLatestVersion()`, `isBrewInstalled()`, and `normalizeVersion(v string) string`. Use `os/exec` directly (no `internal/proc` wrapper). For `brew update --quiet` and `brew info --json=v2`: `cmd.Stderr = os.Stderr; out, _ := cmd.Output()` (capture stdout, pass stderr through). For `brew upgrade`: wire `cmd.Stdin/Stdout/Stderr` to `os.Stdin/Stdout/Stderr` (inherit all three). Map `errors.Is(err, exec.ErrNotFound)` from `cmd.Run()`/`cmd.Output()` to the package-local `ErrBrewNotFound` sentinel.
- [x] T003 Implement `src/cmd/wt/update.go`: `updateCmd()` returning `*cobra.Command` with `Use: "update"`, `Short: "self-update the wt binary via Homebrew"`, `Args: cobra.NoArgs`, and `RunE` that calls `update.Run(version, cmd.OutOrStdout(), cmd.ErrOrStderr())`. On `errors.Is(err, update.ErrBrewNotFound)`, call `os.Exit(wt.ExitGeneralError)` directly to bypass both cobra's error print and `main.go`'s error formatter — `wt.ExitWithError` is NOT used here because it would emit a second structured stderr line, violating the spec's "exactly one line" contract. Otherwise propagate the error verbatim.
- [x] T004 Register `updateCmd()` in `src/cmd/wt/main.go` by adding it to the `root.AddCommand(...)` block (append at end after `shellSetupCmd()`).

### Phase 3: Integration & Edge Cases

- [x] T005 [P] Write `src/internal/update/update_test.go` with: `TestNormalizeVersion` (table-driven cases from spec — `v0.0.3→0.0.3`, `0.0.3→0.0.3`, `""→""`, `v→""`, `vvv1.0.0→vv1.0.0`); `TestRunNonBrewInstall` (skip-if-brew-installed; assert manual-update hint contains `v0.0.3 was not installed via Homebrew` and `brew install sahil87/tap/wt`, stderr empty); `TestIsBrewInstalledReturnsBool` (smoke test — must not panic).
- [x] T006 [P] Write `src/cmd/wt/update_test.go` using the `runWt(t, dir, env, args...)` helper with `createTestRepo(t)` providing `dir`: `TestUpdate_NonBrewBranch` (runs `wt update`; asserts stdout contains `was not installed via Homebrew`); `TestUpdate_RejectsArgs` (runs `wt update extra`; asserts non-zero exit); `TestUpdate_AppearsInHelp` (runs `wt --help`; asserts stdout contains `update`).

### Phase 4: Polish

- [x] T007 Add `## wt update` section to `docs/specs/cli-surface.md` (placed after `wt init`, before `wt shell-setup`) documenting: no flags, no positional args, four user-facing outcomes (Homebrew upgrade succeeds, already up to date, not installed via Homebrew, brew not on PATH), exit-code mapping (`ExitSuccess` on success/no-op/non-brew install, `ExitGeneralError` on brew failure / brew-not-found / JSON parse failure), and a pointer to `src/internal/update/` for implementation detail.
- [x] T008 Run targeted tests `go test ./src/internal/update/... ./src/cmd/wt/...` and the full suite `go test ./src/...` to confirm no regressions.

## Execution Order

- T001 → T002 → T003 → T004 (sequential — each depends on the previous)
- T005 and T006 are independent ([P]) — can be written in any order after T002 and T004 land
- T007 is independent of code (docs only)
- T008 runs last to validate everything

## Acceptance

### Functional Completeness

- [ ] A-001 Subcommand registration: `wt update` is registered on the root cobra command, `Args = cobra.NoArgs`, inherits `SilenceUsage`/`SilenceErrors` from root, appears in `wt --help`.
- [ ] A-002 Brew-install detection: `isBrewInstalled()` resolves `os.Executable()` via `filepath.EvalSymlinks` and returns true iff resolved path contains `/Cellar/`; returns false on any error rather than panicking.
- [ ] A-003 Non-Homebrew install fast-path: When detection returns false, `Run` writes `wt {version} was not installed via Homebrew.` and `Update manually, or reinstall with: brew install sahil87/tap/wt` to `out` and returns `nil` without invoking any `brew` subprocess.
- [ ] A-004 Brew-not-found handling: When `brew` is missing on PATH, `Run` writes `wt update: brew not found on PATH.` to `errOut` exactly once, returns `ErrBrewNotFound`; cobra wrapper detects sentinel and exits via `wt.ExitWithError(ExitGeneralError, ...)` — no duplicate cobra error line.
- [ ] A-005 Brew tap formula identifier: `sahil87/tap/wt` is referenced via a single named constant `brewFormula` in `src/internal/update/update.go`; no inline string duplication.
- [ ] A-006 brew update execution: `brew update --quiet` invoked with 30s context timeout; stdout captured (discarded), stderr passed to parent's stderr; failure returns `brew update failed: {underlying}`.
- [ ] A-007 Latest-version query: `brew info --json=v2 sahil87/tap/wt` invoked with 30s timeout; parses `formulae[0].versions.stable`; empty/missing returns `no stable version found in brew info output`; malformed JSON returns wrapped unmarshal error.
- [ ] A-008 Version comparison and no-op: After stripping at most one leading `v` from each side, equal versions print `Already up to date ({currentVersion}).` to `out` and return `nil` without invoking `brew upgrade`.
- [ ] A-009 brew upgrade execution: When versions differ, prints `Updating {currentVersion} → v{normalizedLatest}...` then runs `brew upgrade sahil87/tap/wt` with 120s timeout, inheriting all three streams (`os.Stdin`, `os.Stdout`, `os.Stderr`); on success prints `Updated to v{normalizedLatest}.` and returns `nil`; non-zero exit returns `brew upgrade exited with code {code}`; start failure returns `brew upgrade failed: {underlying}`.
- [ ] A-010 Output stream contract: Wrapper messages route through `out`/`errOut`; subprocess streams (brew's stdout/stderr) go directly to `os.Stdout`/`os.Stderr` (or are captured for parsing in `brew info`). Tests can inject buffer writers and observe wrapper output without invoking real subprocesses.
- [ ] A-011 Test coverage: `src/cmd/wt/update_test.go` and `src/internal/update/update_test.go` exist and pass; cobra wiring tests use the `runWt` helper; `TestNormalizeVersion` covers all five table cases.
- [ ] A-012 Exit-code mapping: Successful (upgrade, no-op, non-brew install) returns `nil` from `RunE` → exit 0; brew failures, JSON parse failures return non-nil error → exit `ExitGeneralError` (1); brew-not-found exits 1 via `wt.ExitWithError` (no duplicate cobra error line).
- [ ] A-013 CLI surface spec: `docs/specs/cli-surface.md` contains exactly one `## wt update` section describing the four user-facing outcomes and listing relevant exit codes.

### Scenario Coverage

- [ ] A-014 Help output scenario: `wt --help` includes a line for `update` with its short description.
- [ ] A-015 NoArgs scenario: `wt update extra-arg` returns non-zero exit.
- [ ] A-016 Test binary not under /Cellar/: `isBrewInstalled()` returns false in `go test` runs.
- [ ] A-017 Symlink resolution failure: `isBrewInstalled()` returns false when `EvalSymlinks` errors.
- [ ] A-018 Locally-built fast-path scenario: stdout contains both required lines, exit 0, no brew invocation.
- [ ] A-019 normalizeVersion table cases: All five cases pass.

### Edge Cases & Error Handling

- [ ] A-020 brew update timeout: 30s elapsed without completion → context cancelled, subprocess killed, error wrapped as `brew update failed: {underlying}`.
- [ ] A-021 Empty formulae array: `brew info` returns valid JSON with empty `formulae` → error contains `no stable version found in brew info output`.
- [ ] A-022 Malformed JSON: `brew info` returns non-JSON → error wraps the unmarshal failure.
- [ ] A-023 Single leading v stripped: `vvv1.0.0` → `vv1.0.0` (only one `v` removed).
- [ ] A-024 Upgrade subprocess fails to start: `exec.ErrNotFound` from `brew upgrade` mapped to `ErrBrewNotFound`; cobra wrapper handles via `ExitWithError`.
- [ ] A-025 Upgrade non-zero exit: Returns `brew upgrade exited with code {code}`.

### Code Quality

- [ ] A-026 Pattern consistency: New code follows naming and structural patterns of surrounding code (lowercase package name, `RunE` returning errors, `wt.ExitWithError` for typed exits, doc comments on exported symbols).
- [ ] A-027 No unnecessary duplication: Existing utilities reused where applicable — `version` package var reused (no new `internal/version`); `wt.ExitWithError` reused for the brew-not-found exit; no new `internal/proc` wrapper.
- [ ] A-028 Readability: Functions stay focused (no god functions >50 lines without reason); the formula identifier is a named constant; timeouts are named constants.
- [ ] A-029 Anti-patterns avoided: No magic strings (formula, timeouts, error messages) — all use named constants or formatted output.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
