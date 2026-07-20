---
type: memory
description: "`wt update` self-upgrade contract — the toolkit brew-metadata-refresh flag (`--skip-brew-update` visible contract flag + `--no-brew-update` visible alias, both bind one bool) and the brew-handling safety contract (unbounded `brew upgrade`, `brew update`/`brew info` bounded via a shared SIGTERM+grace helper — no code path can SIGKILL brew)."
---
# wt-cli: Update Command Contract

> Post-implementation behavior capture for the `wt update` self-upgrade flow, the brew-metadata-refresh flag, and the brew-handling safety contract.

This file documents the contract that `wt update` honors. Future changes touching `cmd/wt/update.go` or `src/internal/update/update.go` should preserve these invariants unless an explicit spec amendment supersedes them.

The brew-metadata-refresh flag implements a **cross-toolkit contract** shared across the toolkit: the *semantics* and *default behavior* are fixed by that contract and are NOT open to local reinterpretation. `--skip-brew-update` is the toolkit **contract flag** — the shll `update` standard freezes the literal substring `--skip-brew-update` in `update --help` (shll probes it via `strings.Contains` before every toolkit-wide run), so it MUST stay **visible and non-deprecated**. `--no-brew-update` is a **visible alias** bound to the same bool, kept for this repo's `--no-*` negation convention (see [flag-naming-conventions](/wt-cli/flag-naming-conventions.md)). What is forbidden is *re-scoping* the flag's semantics, *removing* the `--no-brew-update` alias, or *hiding* the `--skip-brew-update` contract flag — see § Design Decisions.

## Requirements

### Flag definition — `--skip-brew-update` visible contract flag, `--no-brew-update` visible alias

- `cmd/wt/update.go` registers **both** `--skip-brew-update` and `--no-brew-update` as visible cobra `BoolVar` flags, default `false`, long-form only (no single-letter short alias), per constitution Principle II. Both bind the **same** bool variable (`skipBrewUpdate`) — a shared pointer is correct because both are the same type — so behavior is identical whichever is passed.
- **Neither flag is deprecated or hidden.** There is no `MarkDeprecated` call on the update command. `--skip-brew-update` MUST appear literally in `wt update --help` (the standard's frozen textual contract, probed by `strings.Contains`); a deprecation warning on the contract flag would be wrong because shll passes it on every toolkit-wide run. Passing either flag prints no warning on stdout or stderr.
- Help strings: `--skip-brew-update` carries the full description (`skip the internal `+"`brew update`"+` tap-metadata refresh (toolkit contract flag; version check and upgrade still run)`); `--no-brew-update` reads `alias for --skip-brew-update`.
- `Args: cobra.NoArgs` is retained — `wt update` rejects positional arguments.
- The resolved flag value is threaded into `update.Run` as the LEADING parameter: `update.Run(skipBrewUpdate, version, cmd.OutOrStdout(), cmd.ErrOrStderr())`. Because both flag names bind the same variable, the internal `update.Run` signature and its `skipBrewUpdate` parameter name are unchanged — the flag surface is a pure `cmd/` concern.
- The constructor uses the standard cobra flag-registration shape: `var skipBrewUpdate bool` + `cmd := &cobra.Command{...}` + two `cmd.Flags().BoolVar(&skipBrewUpdate, ...)` calls (contract flag then alias) + `return cmd` — not a bare `return &cobra.Command{...}`.

### Flag skips ONLY the `brew update` metadata refresh

- When `skipBrewUpdate` is `true` (set via either `--skip-brew-update` or `--no-brew-update`), `update.Run` does NOT invoke `brew update`. The entire `brew update` block — its `context.WithTimeout(...)`, the bounded `brew update --quiet` call built by `newBoundedBrewCmd` (see § Brew-handling safety), and that call's error handling — is skipped via an `if !skipBrewUpdate { ... }` guard.
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
- Setting either `--skip-brew-update` or `--no-brew-update` changes none of this routing.

### Brew-not-found detection is preserved (one call later)

- In the default path, `brew update` is the first brew invocation, so it is the one that surfaces `exec.ErrNotFound` and maps it to the `ErrBrewNotFound` sentinel.
- When the flag (either `--skip-brew-update` or `--no-brew-update`) elides that call, the NEXT brew invocation (`brew info` inside `brewLatestVersion`) surfaces `exec.ErrNotFound` and maps it via the IDENTICAL `errors.Is(err, exec.ErrNotFound)` → `ErrBrewNotFound` handling. No new handling is added — the detection just shifts one call later.
- `ErrBrewNotFound` is an exported sentinel (`var ErrBrewNotFound = errors.New("brew not found on PATH")`). `update.Run` prints exactly one `wt update: brew not found on PATH.` line to `errOut`, then returns the sentinel.
- The cobra wrapper in `cmd/wt/update.go` detects the sentinel via `errors.Is(err, update.ErrBrewNotFound)` and calls `os.Exit(wt.ExitGeneralError)` directly — bypassing both cobra's automatic error print and `main.go`'s error formatter so stderr contains EXACTLY one "brew not found" line (the single-line stderr contract). Per constitution Principle III, this is a typed exit, not an ad-hoc integer.

### Brew-handling safety — no code path can SIGKILL brew

Per the shll `update` standard's brew-handling clause, a package-manager mutation must never be SIGKILLed mid-transaction (a kill landing between `brew unlink` and `brew link` corrupts the keg), and `brew upgrade` must not carry a short hard timeout.

- **`brew upgrade` is unbounded.** It runs via plain `exec.Command("brew", "upgrade", brewFormula)` — no `context`, no timer, no `brewUpgradeTimeout` constant. brew can legitimately block for minutes (an un-timed GitHub API call inside brew), the call inherits the user's stdin/stdout/stderr, and Ctrl-C (SIGINT, which brew traps and unwinds) is the user's escape.
- **Bounded metadata calls terminate gracefully.** `brew update` keeps a **5-minute** bound (`brewUpdateTimeout`) and `brew info` a **60-second** bound (`brewInfoTimeout`) — both generous, sized for a network transfer. Both are built through the shared `newBoundedBrewCmd(ctx, args...)` helper, which sets `cmd.Cancel` to send `syscall.SIGTERM` (trappable — brew can finish or roll back) instead of `exec.CommandContext`'s default SIGKILL, and sets `cmd.WaitDelay = brewGraceDelay` (**10 seconds**) so brew has a grace window to unwind before the runtime's forced kill.
- **The helper makes the no-SIGKILL property structural.** Every bounded brew call site MUST go through `newBoundedBrewCmd`, so no call site can regress to the raw `exec.CommandContext` SIGKILL default. A process that exits promptly after SIGTERM is never force-killed.

#### Scenario: a bounded brew call's context expiry delivers a trappable SIGTERM

- **GIVEN** a bounded brew subprocess whose context expires
- **WHEN** the cancel fires
- **THEN** the process receives SIGTERM (trappable — brew can unwind), never an immediate SIGKILL
- **AND** a process that exits promptly after SIGTERM is never force-killed

## Internal API

- `update.Run` signature is `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer) error`.
  - `skipBrewUpdate` leads so the two `io.Writer` parameters stay adjacent at the tail (the existing convention) and the flag reads as a mode flag preceding the operands.
  - `currentVersion` is the binary's reported version (e.g. `v0.1.0`); `normalizeVersion` strips a single leading `v` before comparison since `brew info` reports the bare form.
- `update.Run` is internal to this module; the only caller is `cmd/wt/update.go`. No public API surface changed.
- Subprocess invocation uses `os/exec` directly (no `internal/proc` wrapper) — consistent with the rest of the wt codebase, which has no centralized subprocess routing (deliberate — see Design Decisions).

### `WT_TEST_FORCE_BREW=1` test seam in `isBrewInstalled()`

- `isBrewInstalled()` returns `true` unconditionally when `os.Getenv("WT_TEST_FORCE_BREW") == "1"`, so tests can exercise the brew code paths without a real Homebrew install (the `go test` binary never lives under `/Cellar/`).
- The env var is NEVER set in production, so production behavior (the `/Cellar/` resolution via `os.Executable()` + `filepath.EvalSymlinks`) is unchanged.
- This mirrors the `WT_TEST_NO_LAUNCH=1` seam at `src/internal/worktree/apps.go` — the established codebase convention for keeping `exec.Command` call sites direct yet testable. It is NOT a `var execCommand = exec.Command` injection nor an interface-based runner refactor (both explicitly rejected — see Design Decisions).

## Design Decisions

### `skipBrewUpdate` is the leading parameter of `Run`

`Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` places the new flag first to keep the two `io.Writer` parameters adjacent at the tail (the existing convention) and to read as a mode flag preceding the operands. Appending it after the writers (`Run(currentVersion, out, errOut, skipBrewUpdate)`) was rejected because it splits the natural writer pairing and buries the mode flag behind the I/O operands. Only one caller exists (`cmd/wt/update.go`), so the signature change is trivially absorbed. (Source: spec ipe5 Design Decision 1.)

### Env-var test seam over a subprocess-runner refactor

The test seam is the test-only `WT_TEST_FORCE_BREW=1` env var that forces `isBrewInstalled()` true, combined with a fake `brew` executable first on `PATH` that records each invocation's first argument. This mirrors the `WT_TEST_NO_LAUNCH=1` precedent at `apps.go` and keeps the brew call sites (direct `exec.Command` for the unbounded upgrade, the shared `newBoundedBrewCmd` for the bounded metadata calls) exercisable with zero production indirection. A `var execCommand = exec.Command` injection or an interface-based runner was rejected — those are the very refactor the cross-toolkit contract forbids ("match existing subprocess convention; do NOT refactor"). A build-tag stub file was also rejected as heavier and splitting logic across build configurations.

### Brew-not-found detection deliberately shifts to `brew info`, no new code

Rather than adding a pre-flight `brew` existence check to preserve not-found detection when `brew update` is skipped, the design relies on `brew info` (`brewLatestVersion`) having identical `exec.ErrNotFound` → `ErrBrewNotFound` handling. Detection still happens, just one call later, and the single-line stderr contract is preserved with zero added code. (Source: spec ipe5 "Brew-not-found detection preserved".)

### `--skip-brew-update` visible contract flag; `--no-brew-update` visible alias
**Decision**: both `--skip-brew-update` and `--no-brew-update` are visible, non-deprecated flags bound to one bool; `--skip-brew-update` carries the full help text, `--no-brew-update` reads `alias for --skip-brew-update`; there is no `MarkDeprecated` on the update command.
**Why**: the shll `update` standard freezes the literal substring `--skip-brew-update` in `update --help` as a MUST (probed by `strings.Contains`), and shll passes the flag on every toolkit-wide run — a deprecation warning on the contract flag every run would be wrong. Keeping `--no-brew-update` visible preserves this repo's own documented `--no-*` surface, so no caller breaks.
**Rejected**: keeping `MarkDeprecated` but un-hiding via `flag.Hidden = false` (pflag still prints the deprecation warning on every use); dropping `--no-brew-update` (breaks the repo's own documented surface); keeping `--no-brew-update` as the primary and `--skip-brew-update` deprecated (the deprecation warning + help-hiding violate the standard's substring MUST — the standard postdates that arrangement and, per Constitution § Toolkit Standards, binds without amendment).
*Introduced by*: `260719-32su-conform-update-version-shell-init`

### Graceful-bound helper for brew metadata calls; upgrade unbounded
**Decision**: one shared `newBoundedBrewCmd` constructor builds every bounded brew command with `cmd.Cancel` = SIGTERM and `cmd.WaitDelay` = 10s; `brew update` is bounded 5 min, `brew info` 60s; `brew upgrade` gets no bound at all (plain `exec.Command`).
**Why**: the standard's brew-handling MUSTs — no SIGKILL mid-transaction (a kill between `brew unlink`/`brew link` corrupts the keg), and no short hard timeout on `brew upgrade`. A single helper makes the no-SIGKILL property structural rather than per-call-site.
**Rejected**: a generous bound on `brew upgrade` (also conformant, but adds a failure mode for zero benefit on an interactive call the user can Ctrl-C); `HOMEBREW_NO_GITHUB_API=1` injection as the upgrade alternative (unbounded is simpler); per-call-site `cmd.Cancel` wiring (regression-prone — a new call site could forget it and regress to the SIGKILL default).
*Introduced by*: `260719-32su-conform-update-version-shell-init`

## Cross-references

- Source: `src/internal/update/update.go` — `Run`, `brewLatestVersion`, `isBrewInstalled` (with `WT_TEST_FORCE_BREW` seam), `normalizeVersion`, `ErrBrewNotFound`, `brewFormula`, the `newBoundedBrewCmd` graceful-bound helper (SIGTERM `cmd.Cancel` + `brewGraceDelay` `WaitDelay`), and the `brewUpdateTimeout` (5 min) / `brewInfoTimeout` (60s) / `brewGraceDelay` (10s) constants. `src/cmd/wt/update.go` — `updateCmd` (both `--skip-brew-update` contract flag and `--no-brew-update` alias registered visible on the shared `skipBrewUpdate` var, no `MarkDeprecated`, plus the `ErrBrewNotFound`→typed-exit wrapper).
- Tests: `src/internal/update/update_test.go` — `TestRunSkipBrewUpdate` (fake `brew` on `PATH` + `WT_TEST_FORCE_BREW=1`, asserts skip omits `update` but keeps `info`/`upgrade`), the bounds/constants pins, and the graceful-cancel test (a fake slow `brew` on `PATH` that traps SIGTERM under a short test-injected context, proving SIGTERM — not SIGKILL — delivery via `newBoundedBrewCmd`), `TestRunNonBrewInstall`, `TestNormalizeVersion`, `TestIsBrewInstalledReturnsBool`. `src/cmd/wt/update_test.go` — the `--skip-brew-update` literal-substring help probe (mirroring shll's `strings.Contains`), both-flags-visible assertion, and the no-deprecation-warning assertion.
- Constitution: Principle II (Cobra command surface — long-form flag names, `RunE`), Principle III (Typed exit codes — `ErrBrewNotFound` → `ExitGeneralError`).
- Sibling memory: `wt-cli/init-failure-contract.md`, `wt-cli/list-status-contract.md` — same pattern of post-change invariant capture for other `wt` subcommands. The `WT_TEST_FORCE_BREW` seam is a sibling of the `WT_TEST_NO_LAUNCH` seam documented in `init-failure-contract.md`.
- Cross-toolkit: the flag's **semantics and default** MUST stay identical to the other tools implementing the same contract. `--skip-brew-update` is the toolkit contract flag (frozen as a `--help` substring by the shll `update` standard) and MUST stay visible; `--no-brew-update` is a visible alias — both bind one bool, so the cross-tool name works and the behavioral contract holds (see § Design Decisions).
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md) — the `update` standard's PASS surface (flag-substring contract + brew-handling safety) under the shll v0.1.7 re-audit; [flag-naming-conventions](/wt-cli/flag-naming-conventions.md) — the `--no-*` negation convention and the carve-out that a toolkit-standard frozen contract flag may not be hidden via `MarkDeprecated`.
