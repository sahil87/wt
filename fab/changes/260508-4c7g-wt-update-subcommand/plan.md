# Plan: wt update subcommand

**Change**: 260508-4c7g-wt-update-subcommand
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Tasks

### Phase 1: Setup

- [x] T001 Create new package directory `src/internal/update/` (no scaffolding files — will be populated in Phase 2 by T002)

### Phase 2: Core Implementation

- [x] T002 Implement `src/internal/update/update.go`: package doc comment (wt-flavored, drops hop's Constitution Principle I citation), `brewFormula = "sahil87/tap/wt"` constant, timeout constants (30s/30s/120s), exported sentinel `ErrBrewNotFound`, `Run(currentVersion string, out, errOut io.Writer) error`, `brewLatestVersion()`, `isBrewInstalled()`, and `normalizeVersion(v string) string`. Use `os/exec` directly (no `internal/proc` wrapper). For `brew update --quiet` and `brew info --json=v2`: `cmd.Stderr = os.Stderr; out, _ := cmd.Output()` (capture stdout, pass stderr through). For `brew upgrade`: wire `cmd.Stdin/Stdout/Stderr` to `os.Stdin/Stdout/Stderr` (inherit all three). Map `errors.Is(err, exec.ErrNotFound)` from `cmd.Run()`/`cmd.Output()` to the package-local `ErrBrewNotFound` sentinel.
- [x] T003 Implement `src/cmd/wt/update.go`: `updateCmd()` returning `*cobra.Command` with `Use: "update"`, `Short: "self-update the wt binary via Homebrew"`, `Args: cobra.NoArgs`, and `RunE` that calls `update.Run(version, cmd.OutOrStdout(), cmd.ErrOrStderr())`. On `errors.Is(err, update.ErrBrewNotFound)`, call `wt.ExitWithError(wt.ExitGeneralError, ...)` directly to bypass cobra's error print (process terminates). Otherwise propagate the error verbatim.
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
