# Intake: Add --skip-brew-update flag to update command

**Change**: 260531-ipe5-skip-brew-update-flag
**Created**: 2026-05-31
**Status**: Draft

## Origin

> One-shot invocation via `/fab-new`. Raw input:
>
> Add a boolean `--skip-brew-update` flag to the `update` command. **CONTRACT** (cross-toolkit, identical in 6 tools): flag name EXACTLY `--skip-brew-update`. When set, skip ONLY the internal `brew update` tap-metadata refresh. Everything else unchanged: `brew info` version check, up-to-date short-circuit, `brew upgrade`. Default (absent) = current behavior exactly preserved.
>
> THIS REPO (wt): update logic in `src/internal/update/update.go` (func `Run`, the `brew update` call ~L68); wire a real cobra bool flag in `cmd/wt/update.go` and pass it into `Run()`. Thread `skipBrewUpdate bool` through `Run()`. Preserve the intentional output routing (brew update/info capture stdout, upgrade is interactive). Match existing subprocess convention (do NOT refactor). Add a test asserting `--skip-brew-update` omits `brew update` but still runs `brew upgrade`, following the repo test pattern. Build + run the update package tests before the PR.
>
> PR title (exact): `feat: add --skip-brew-update flag to update command`. Do NOT merge.

This is one tool's implementation of a cross-toolkit contract shared by 6 tools. The flag name, semantics, and default behavior are fixed by the contract and not open to local reinterpretation.

## Why

1. **Problem**: `wt update` always runs `brew update` first — a full Homebrew tap-metadata refresh that hits the network and can take many seconds even when only the `sahil87/tap/wt` formula needs checking. In automation, CI, or rapid local iteration, this refresh is redundant: the caller may have just run `brew update` moments ago, or may deliberately want to upgrade against already-fetched tap metadata.

2. **Consequence if not fixed**: Every `wt update` invocation pays the `brew update` cost unconditionally. Scripted/batch upgrades across many tools (the cross-toolkit motivation) multiply this cost N times. There is no escape hatch to skip just the metadata refresh while still performing the actual version check and upgrade.

3. **Why this approach**: A single boolean opt-out flag (`--skip-brew-update`) is the minimal, predictable surface. It maps cleanly onto the one subprocess call we want to elide (`brew update`) and leaves the rest of the flow (`brew info` version check, up-to-date short-circuit, `brew upgrade`) byte-for-byte unchanged. The default-absent path preserves today's behavior exactly, so existing callers are unaffected. The flag name and semantics are dictated by the cross-toolkit contract — consistency across all 6 tools is the point, so the local implementation conforms rather than innovates.

## What Changes

### 1. Thread `skipBrewUpdate bool` through `update.Run`

`src/internal/update/update.go` — `Run` gains a leading `skipBrewUpdate bool` parameter:

```go
func Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer) error {
```

The signature is `(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)`. Placing `skipBrewUpdate` first keeps the two writers adjacent at the tail (the existing convention) and reads naturally as a mode flag.

### 2. Gate ONLY the `brew update` block

The current `brew update` block (update.go ~L67–78):

```go
ctx, cancel := context.WithTimeout(context.Background(), brewUpdateTimeout)
cmd := exec.CommandContext(ctx, "brew", "update", "--quiet")
cmd.Stderr = os.Stderr
_, err := cmd.Output()
cancel()
if err != nil {
    if errors.Is(err, exec.ErrNotFound) {
        fmt.Fprintln(errOut, "wt update: brew not found on PATH.")
        return ErrBrewNotFound
    }
    return fmt.Errorf("brew update failed: %w", err)
}
```

is wrapped in `if !skipBrewUpdate { ... }`. When `skipBrewUpdate` is true the entire block — context, subprocess, error handling — is skipped. **Nothing else moves.** The `brew info` version check (`brewLatestVersion`), the up-to-date short-circuit (`normalizeVersion` equality → "Already up to date"), and the interactive `brew upgrade` all execute exactly as today.

**Output routing is preserved exactly**: `brew update` keeps `cmd.Stderr = os.Stderr` and `cmd.Output()` (capture stdout for discard); `brew info` keeps the same; `brew upgrade` keeps `Stdin/Stdout/Stderr = os.Stdin/os.Stdout/os.Stderr` (interactive). The wrapper messages on `out`/`errOut` are unchanged. The `Run` doc comment is updated to note the new parameter and that skipping affects only the metadata refresh.

**Edge case — `brew not found` detection**: today, if `brew` is missing on PATH, the `brew update` call is the first to surface `exec.ErrNotFound` and map it to `ErrBrewNotFound`. When `--skip-brew-update` skips that call, the NEXT brew invocation (`brew info` inside `brewLatestVersion`) already has identical `errors.Is(err, exec.ErrNotFound)` → `ErrBrewNotFound` handling (update.go L82–85). So brew-not-found is still detected and still maps to the typed exit — just one call later. The single-line stderr contract is preserved. No new handling needed.

### 3. Wire the cobra bool flag in `cmd/wt/update.go`

`src/cmd/wt/update.go` — register a real cobra bool flag and pass its value into `Run`:

```go
func updateCmd() *cobra.Command {
    var skipBrewUpdate bool
    cmd := &cobra.Command{
        Use:   "update",
        Short: "self-update the wt binary via Homebrew",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            err := update.Run(skipBrewUpdate, version, cmd.OutOrStdout(), cmd.ErrOrStderr())
            if errors.Is(err, update.ErrBrewNotFound) {
                os.Exit(wt.ExitGeneralError)
            }
            return err
        },
    }
    cmd.Flags().BoolVar(&skipBrewUpdate, "skip-brew-update", false,
        "skip the internal `brew update` tap-metadata refresh (version check and upgrade still run)")
    return cmd
}
```

The constructor changes from a single `return &cobra.Command{...}` to a `var` + `cmd :=` + `cmd.Flags().BoolVar(...)` + `return cmd` shape — the standard cobra flag-registration pattern. Long-form flag name only (no short flag), per constitution principle II. `Args: cobra.NoArgs` is retained.

### 4. Test: flag omits `brew update`, still runs `brew upgrade`

Add a unit test in `src/internal/update/update_test.go` asserting that with `skipBrewUpdate=true`, `Run` does NOT invoke `brew update` but DOES invoke `brew upgrade` (and `brew info`), and that with `skipBrewUpdate=false` it DOES invoke `brew update`.

**Test seam (matches existing convention, not a refactor)**: The repo's established pattern for keeping `exec.Command` calls direct yet testable is an **environment-variable seam** that short-circuits real subprocess behavior — see `src/internal/worktree/apps.go:201` (`WT_TEST_NO_LAUNCH=1` makes `OpenInApp` skip real launches so tests can assert behavior without spawning GUIs). This is NOT the rejected `var execCommand = exec.Command` indirection nor an interface refactor; the `exec.CommandContext(...)` call sites stay exactly as they are.

Concretely: the test puts a fake `brew` script first on `PATH` (via `t.Setenv("PATH", ...)`) that appends its first argument (`update` / `info` / `upgrade`) to a log file, and — critically — emits the JSON `brew info` expects so `brewLatestVersion` parses a version, plus exits 0 for `upgrade`. The test then drives `Run` and asserts the log contains `upgrade` and `info` but not `update` (skip case), and contains `update` (default case).

**The `isBrewInstalled()` gate** short-circuits `Run` before any brew call when the test binary is not under `/Cellar/` (always true for `go test`). To exercise the skip-vs-run logic, the test must bypass that gate. The cleanest convention-matching way is a sibling env-var seam: `isBrewInstalled()` returns true when a dedicated test env var (e.g. `WT_TEST_FORCE_BREW=1`) is set, mirroring the `apps.go` `WT_TEST_NO_LAUNCH` precedent. This keeps the production code path identical (the env var is never set in production) and adds no production indirection. [NEEDS CLARIFICATION resolved as Tentative — see Assumptions #6.]

To force a version mismatch so the upgrade path is reached, `currentVersion` is passed as a value that differs from the fake `brew info` stable version (e.g. binary reports `v0.0.0`, fake info reports `9.9.9`).

### Out of scope

- No change to `brew info`, the up-to-date short-circuit, or `brew upgrade` behavior.
- No change to the `Run` writer parameters' routing or the wrapper messages.
- No refactor of subprocess invocation into a runner/interface — direct `exec.CommandContext` calls stay.
- No short flag, no flag aliases, no config-file binding.

## Affected Memory

- `wt-cli/update-command-contract`: (new) Behavior contract for `wt update` including the `--skip-brew-update` flag — what it skips (only `brew update`), what it preserves (`brew info` version check, up-to-date short-circuit, `brew upgrade`), default behavior, and the brew-not-found detection shift to the `brew info` call. Created during hydrate.

## Impact

- **`src/internal/update/update.go`**: `Run` signature gains `skipBrewUpdate bool` (leading param); the `brew update` block is wrapped in `if !skipBrewUpdate`; doc comment updated; `isBrewInstalled` gains a test-only env-var bypass (production behavior unchanged).
- **`src/cmd/wt/update.go`**: cobra `BoolVar` flag `--skip-brew-update` registered; value threaded into `update.Run`. The single existing caller is updated to the new signature.
- **`src/internal/update/update_test.go`**: new test (fake-`brew`-on-PATH + env-var seam) plus existing tests updated for the new `Run` signature.
- **Dependencies**: none added. Pure stdlib + cobra (already present).
- **External callers**: `update.Run` is internal to this module; the only caller is `cmd/wt/update.go`. No public API break.
- **Cross-toolkit**: flag name/semantics must match the other 5 tools' implementations of the same contract.

## Open Questions

- None blocking. The test-seam mechanism for bypassing `isBrewInstalled()` is the one genuine implementation choice (Assumptions #6); it is reversible and confined to test scaffolding, so it is taken as a Tentative assumption rather than asked.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Flag name is EXACTLY `--skip-brew-update`, cobra `BoolVar`, default `false`, long-form only | Fixed by cross-toolkit contract and constitution principle II (long-form flag names) | S:98 R:90 A:95 D:98 |
| 2 | Certain | When set, skip ONLY the `brew update` block; `brew info` / up-to-date short-circuit / `brew upgrade` unchanged; default preserves current behavior exactly | Explicit, repeated in the contract; one obvious interpretation | S:98 R:75 A:95 D:95 |
| 3 | Certain | Preserve existing output routing: brew update/info capture stdout (stderr→os.Stderr), brew upgrade inherits stdin/stdout/stderr (interactive) | Explicitly required; documented in current `Run` doc comment | S:95 R:80 A:95 D:95 |
| 4 | Confident | `Run` signature becomes `Run(skipBrewUpdate bool, currentVersion string, out, errOut io.Writer)` — new param leads, writers stay adjacent at tail | Contract says "thread skipBrewUpdate through Run"; leading-param keeps writer pairing; one internal caller, trivially updated | S:80 R:80 A:80 D:70 |
| 5 | Confident | brew-not-found stays detected: with the update call skipped, `brew info` (`brewLatestVersion`) surfaces `exec.ErrNotFound`→`ErrBrewNotFound` with identical handling; single-line stderr contract preserved | Reading update.go L82–85 shows identical ErrNotFound handling already on the info call; no new code needed | S:75 R:75 A:85 D:75 |
| 6 | Tentative | Test bypasses `isBrewInstalled()` via a test-only env-var seam (e.g. `WT_TEST_FORCE_BREW=1`), mirroring `apps.go:201` `WT_TEST_NO_LAUNCH`; fake `brew` on PATH logs which subcommands ran | Matches existing env-seam convention (not a refactor); alternative (build-tag stub or `var isBrewInstalled` indirection) rejected as heavier. Reversible, test-only — exact var name finalized at apply | S:55 R:80 A:60 D:55 |
| 7 | Confident | New memory file `wt-cli/update-command-contract` created at hydrate to document the flag contract | Spec-level behavior changes; wt-cli domain exists and houses command contracts | S:70 R:85 A:80 D:75 |

7 assumptions (3 certain, 3 confident, 1 tentative, 0 unresolved).
