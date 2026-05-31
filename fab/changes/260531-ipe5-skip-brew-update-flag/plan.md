# Plan: Add --skip-brew-update flag to update command

**Change**: 260531-ipe5-skip-brew-update-flag
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
