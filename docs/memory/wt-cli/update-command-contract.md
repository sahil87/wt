# wt-cli: Update Command Contract

> Post-implementation behavior capture for the `wt update` self-upgrade flow and the `--skip-brew-update` flag.
> Source change: `260531-ipe5-skip-brew-update-flag`.

This file documents the contract that `wt update` honors. Future changes touching `cmd/wt/update.go` or `src/internal/update/update.go` should preserve these invariants unless an explicit spec amendment supersedes them.

The `--skip-brew-update` flag is one tool's implementation of a **cross-toolkit contract** shared by 6 tools: the flag name, semantics, and default behavior are fixed by that contract and are NOT open to local reinterpretation. Renaming, aliasing, or re-scoping the flag in this repo alone is a contract violation.

## Requirements

### `--skip-brew-update` flag definition

- `cmd/wt/update.go` registers a cobra `BoolVar` flag named EXACTLY `--skip-brew-update`, default `false`, long-form only (no single-letter short alias), per constitution Principle II.
- `Args: cobra.NoArgs` is retained — `wt update` rejects positional arguments.
- The flag value is threaded into `update.Run` as the LEADING parameter: `update.Run(skipBrewUpdate, version, cmd.OutOrStdout(), cmd.ErrOrStderr())`.
- The constructor uses the standard cobra flag-registration shape: `var skipBrewUpdate bool` + `cmd := &cobra.Command{...}` + `cmd.Flags().BoolVar(&skipBrewUpdate, "skip-brew-update", false, "...")` + `return cmd` — not a bare `return &cobra.Command{...}`.

### Flag skips ONLY the `brew update` metadata refresh

- When `skipBrewUpdate` is `true`, `update.Run` does NOT invoke `brew update`. The entire `brew update` block — its `context.WithTimeout(...)`, the `exec.CommandContext(ctx, "brew", "update", "--quiet")` call, and that call's error handling — is skipped via an `if !skipBrewUpdate { ... }` guard.
- Nothing else moves. When the flag is set, these still run exactly as in the default path:
  - the `brew info --json=v2 sahil87/tap/wt` version check (`brewLatestVersion`),
  - the up-to-date short-circuit (`normalizeVersion(latest) == normalizeVersion(currentVersion)` → prints `Already up to date (...)` and returns `nil`),
  - the interactive `brew upgrade sahil87/tap/wt` when versions differ.

### Default behavior (flag absent) is preserved byte-for-byte

- When `skipBrewUpdate` is `false` (the default — flag omitted), `update.Run` behaves exactly as before this flag existed: `brew update --quiet` runs first, then `brew info`, then the up-to-date short-circuit, then `brew upgrade` when versions differ.
- The flag is a pure additive opt-out; existing callers and scripts that never pass it are unaffected.

### Non-brew install short-circuit is unaffected by the flag

- When the running binary is NOT a Homebrew install (does not resolve under `/Cellar/`, per `isBrewInstalled()`), `update.Run` prints the manual-update hint (`wt <version> was not installed via Homebrew.` + `Update manually, or reinstall with: brew install sahil87/tap/wt`) to `out` and returns `nil` WITHOUT invoking any brew subcommand.
- This `isBrewInstalled()` short-circuit fires before any brew call and is independent of `skipBrewUpdate` — the flag value never reaches a brew invocation in this path.

### Output routing is preserved for every subprocess

- `brew update` and `brew info` capture stdout (for parse/discard via `cmd.Output()`) and route stderr to `os.Stderr` (`cmd.Stderr = os.Stderr`).
- `brew upgrade` inherits all three standard streams — `cmd.Stdin = os.Stdin`, `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr` — so brew's tty-aware progress renders inline (interactive).
- The `out` and `errOut` writers receive ONLY the wrapper messages this package emits (`Current version:`, `Checking for updates...`, `Already up to date`, `Updating ... → ...`, `Updated to ...`, and error hints). Subprocess stdout/stderr is intentionally NOT routed through them.
- Setting `--skip-brew-update` changes none of this routing.

### Brew-not-found detection is preserved (one call later)

- In the default path, `brew update` is the first brew invocation, so it is the one that surfaces `exec.ErrNotFound` and maps it to the `ErrBrewNotFound` sentinel.
- When `--skip-brew-update` elides that call, the NEXT brew invocation (`brew info` inside `brewLatestVersion`) surfaces `exec.ErrNotFound` and maps it via the IDENTICAL `errors.Is(err, exec.ErrNotFound)` → `ErrBrewNotFound` handling. No new handling is added — the detection just shifts one call later.
- `ErrBrewNotFound` is an exported sentinel (`var ErrBrewNotFound = errors.New("brew not found on PATH")`). `update.Run` prints exactly one `wt update: brew not found on PATH.` line to `errOut`, then returns the sentinel.
- The cobra wrapper in `cmd/wt/update.go` detects the sentinel via `errors.Is(err, update.ErrBrewNotFound)` and calls `os.Exit(wt.ExitGeneralError)` directly — bypassing both cobra's automatic error print and `main.go`'s error formatter so stderr contains EXACTLY one "brew not found" line (the single-line stderr contract). Per constitution Principle III, this is a typed exit, not an ad-hoc integer.

## Internal API

- `update.Run` signature is `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer) error`.
  - `skipBrewUpdate` leads so the two `io.Writer` parameters stay adjacent at the tail (the existing convention) and the flag reads as a mode flag preceding the operands.
  - `currentVersion` is the binary's reported version (e.g. `v0.1.0`); `normalizeVersion` strips a single leading `v` before comparison since `brew info` reports the bare form.
- `update.Run` is internal to this module; the only caller is `cmd/wt/update.go`. No public API surface changed.
- Subprocess invocation uses `os/exec` directly (no `internal/proc` wrapper) — consistent with the rest of the wt codebase, which has no centralized subprocess routing. This change did NOT refactor that into a runner/interface.

### `WT_TEST_FORCE_BREW=1` test seam in `isBrewInstalled()`

- `isBrewInstalled()` returns `true` unconditionally when `os.Getenv("WT_TEST_FORCE_BREW") == "1"`, so tests can exercise the brew code paths without a real Homebrew install (the `go test` binary never lives under `/Cellar/`).
- The env var is NEVER set in production, so production behavior (the `/Cellar/` resolution via `os.Executable()` + `filepath.EvalSymlinks`) is unchanged.
- This mirrors the `WT_TEST_NO_LAUNCH=1` seam at `src/internal/worktree/apps.go` — the established codebase convention for keeping `exec.Command` call sites direct yet testable. It is NOT a `var execCommand = exec.Command` injection nor an interface-based runner refactor (both explicitly rejected — see Design Decisions).

## Design Decisions

### `skipBrewUpdate` is the leading parameter of `Run`

`Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` places the new flag first to keep the two `io.Writer` parameters adjacent at the tail (the existing convention) and to read as a mode flag preceding the operands. Appending it after the writers (`Run(currentVersion, out, errOut, skipBrewUpdate)`) was rejected because it splits the natural writer pairing and buries the mode flag behind the I/O operands. Only one caller exists (`cmd/wt/update.go`), so the signature change is trivially absorbed. (Source: spec ipe5 Design Decision 1.)

### Env-var test seam over a subprocess-runner refactor

The test seam is the test-only `WT_TEST_FORCE_BREW=1` env var that forces `isBrewInstalled()` true, combined with a fake `brew` executable first on `PATH` that records each invocation's first argument. This mirrors the `WT_TEST_NO_LAUNCH=1` precedent at `apps.go` and keeps every `exec.CommandContext` call site untouched with zero production indirection. A `var execCommand = exec.Command` injection or an interface-based runner was rejected — those are the very refactor the cross-toolkit contract forbids ("match existing subprocess convention; do NOT refactor"). A build-tag stub file was also rejected as heavier and splitting logic across build configurations. (Source: spec ipe5 Design Decision 2.)

### Brew-not-found detection deliberately shifts to `brew info`, no new code

Rather than adding a pre-flight `brew` existence check to preserve not-found detection when `brew update` is skipped, the change relies on the fact that `brew info` (`brewLatestVersion`) already has identical `exec.ErrNotFound` → `ErrBrewNotFound` handling. Detection still happens, just one call later, and the single-line stderr contract is preserved with zero added code. (Source: spec ipe5 "Brew-not-found detection preserved".)

## Cross-references

- Source: `src/internal/update/update.go` — `Run`, `brewLatestVersion`, `isBrewInstalled` (with `WT_TEST_FORCE_BREW` seam), `normalizeVersion`, `ErrBrewNotFound`, `brewFormula`. `src/cmd/wt/update.go` — `updateCmd` (flag registration + `ErrBrewNotFound`→typed-exit wrapper).
- Tests: `src/internal/update/update_test.go` — `TestRunSkipBrewUpdate` (fake `brew` on `PATH` + `WT_TEST_FORCE_BREW=1`, asserts skip omits `update` but keeps `info`/`upgrade`), `TestRunNonBrewInstall`, `TestNormalizeVersion`, `TestIsBrewInstalledReturnsBool`.
- Constitution: Principle II (Cobra command surface — long-form flag name, `RunE`), Principle III (Typed exit codes — `ErrBrewNotFound` → `ExitGeneralError`).
- Sibling memory: `wt-cli/init-failure-contract.md`, `wt-cli/list-status-contract.md` — same pattern of post-change invariant capture for other `wt` subcommands. The `WT_TEST_FORCE_BREW` seam is a sibling of the `WT_TEST_NO_LAUNCH` seam documented in `init-failure-contract.md`.
- Cross-toolkit: `--skip-brew-update` flag name/semantics MUST stay identical to the other 5 tools implementing the same contract.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260531-ipe5-skip-brew-update-flag` | 2026-05-31 | Added the `--skip-brew-update` cobra bool flag (default false, long-form only); threaded `skipBrewUpdate` as the leading parameter of `update.Run`; guarded ONLY the `brew update` block behind `if !skipBrewUpdate` (info/short-circuit/upgrade unchanged); preserved output routing and the non-brew-install short-circuit; relied on `brew info`'s existing `ErrBrewNotFound` mapping for not-found detection; added the `WT_TEST_FORCE_BREW=1` test seam in `isBrewInstalled()`. |
