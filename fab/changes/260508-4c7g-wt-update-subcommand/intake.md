# Intake: wt update subcommand

**Change**: 260508-4c7g-wt-update-subcommand
**Created**: 2026-05-09
**Status**: Draft

## Origin

> Create a wt update subcommand that does the same thing as hop update — take reference from ~/code/sahil87/hop/ and adapt the logic to wt

One-shot natural-language input. The reference implementation (`~/code/sahil87/hop/src/cmd/hop/update.go` and `~/code/sahil87/hop/src/internal/update/update.go`) is concrete and complete; the task is a structured port. The user explicitly asked for parity with `hop update`, so the design follows that command's behavior modulo project-specific differences (binary name, tap formula, subprocess wrapper).

## Why

1. **Problem**: `wt` ships via Homebrew (`sahil87/tap/wt` per `docs/specs/build-and-release.md`), but users currently have no in-binary upgrade path. They must remember the formula name and run `brew upgrade sahil87/tap/wt` manually — friction that delays uptake of bug-fix and feature releases.
2. **Consequence**: Users keep stale `wt` binaries even when fixes are available; bug reports arrive against versions long since fixed; the tool feels less polished than its sibling `hop` (which already provides `hop update`).
3. **Why this approach**: `hop update` already exists in a sibling repo with the same release/distribution model (Homebrew tap, single binary, `git describe`-derived version). Porting that logic gives `wt` an identical user experience with proven behavior — same prompts, same exit semantics, same edge-case handling — at minimal design risk. Tap formula is the only meaningful surface difference: `sahil87/tap/wt` instead of `sahil87/tap/hop`.

## What Changes

### New subcommand: `wt update`

User-facing behavior matches `hop update` exactly:

```
$ wt update
Current version: v0.1.0
Checking for updates...
==> Updating Homebrew...
Updating sahil87/tap...
Updating sahil87/tap/wt v0.1.0 → v0.1.1...
==> Fetching sahil87/tap/wt
==> Pouring wt-0.1.1...
🍺  /opt/homebrew/Cellar/wt/0.1.1: 3 files, 8.4MB
Updated to v0.1.1.
```

When no update is available:

```
$ wt update
Current version: v0.1.1
Checking for updates...
Already up to date (v0.1.1).
```

When the binary was not installed via Homebrew (e.g., built locally via `just local-install`):

```
$ wt update
wt v0.1.0 was not installed via Homebrew.
Update manually, or reinstall with: brew install sahil87/tap/wt
```

When `brew` is not on PATH:

```
$ wt update
wt update: brew not found on PATH.
```
(exits with `ExitGeneralError`; the cobra wrapper suppresses any further "binary not found" double-print — see "errSilent equivalent" below)

### File layout

Two new files, mirroring hop's split:

1. **`src/cmd/wt/update.go`** — cobra wiring. Calls `wtupdate.Run(version, cmd.OutOrStdout(), cmd.ErrOrStderr())`. Maps `errors.Is(err, exec.ErrNotFound)` (or an equivalent sentinel — see "Subprocess wrapper" below) to a silenced exit so cobra doesn't print a redundant error line.
2. **`src/internal/update/update.go`** — core logic. Mirrors `hop/src/internal/update/update.go` line-for-line with these substitutions:
   - `brewFormula = "sahil87/tap/wt"` (was `sahil87/tap/hop`)
   - User-visible strings: `"hop"` → `"wt"` everywhere (e.g., `"hop update: brew not found on PATH."` → `"wt update: brew not found on PATH."`, `"hop %s was not installed via Homebrew.\n"` → `"wt %s was not installed via Homebrew.\n"`)
   - Package doc comment: replace `hop` references with `wt`

### Subprocess wrapper

Critical project-specific difference: **`wt` does NOT have an `internal/proc` package** like `hop` does. The constitution does not mandate routing through such a wrapper — existing wt code uses `os/exec` directly throughout (`src/internal/worktree/context.go`, `stash.go`, `apps.go`, `cmd/wt/init.go`).

Decision: in `internal/update`, use `os/exec` directly, matching the in-repo convention. Define a small package-local sentinel (`ErrBrewNotFound`) and map `errors.Is(err, exec.ErrNotFound)` from `cmd.Run()` to it — preserving the same control flow as `hop`'s `proc.ErrNotFound` without introducing a new internal package solely for this command. If future commands need shared subprocess plumbing, extracting `internal/proc` becomes a separate refactor.

The two `proc` calls in `hop`'s update become:

| hop call | wt equivalent |
|----------|---------------|
| `proc.Run(ctx, "brew", "update", "--quiet")` | `exec.CommandContext(ctx, "brew", "update", "--quiet").Output()` (capture stdout, stderr → parent stderr) |
| `proc.Run(ctx, "brew", "info", "--json=v2", brewFormula)` | same pattern — `Output()` captures stdout for JSON parsing |
| `proc.RunForeground(upCtx, "", "brew", "upgrade", brewFormula)` | `exec.CommandContext(ctx, "brew", "upgrade", brewFormula)` with `Stdin/Stdout/Stderr` wired to `os.Stdin/os.Stdout/os.Stderr` so brew's tty-aware progress output renders inline |

`exec.CommandContext` honors timeout cancellation natively, matching `hop`'s `context.WithTimeout` pattern. Timeout values port verbatim: `brewUpdateTimeout = 30s`, `brewInfoTimeout = 30s`, `brewUpgradeTimeout = 120s`.

### Cobra wiring details

In `src/cmd/wt/update.go`:

```go
func updateCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "update",
        Short: "self-update the wt binary via Homebrew",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            return update.Run(version, cmd.OutOrStdout(), cmd.ErrOrStderr())
        },
    }
}
```

Register in `src/cmd/wt/main.go`:

```go
root.AddCommand(
    createCmd(),
    listCmd(),
    openCmd(),
    deleteCmd(),
    initCmd(),
    shellSetupCmd(),
    updateCmd(), // new
)
```

The `version` variable is already defined in `main.go` as `var version = "dev"` and is overridden via `-ldflags "-X main.version=..."` at build time — the `update.Run()` signature receives this verbatim. No build-system changes needed.

**`errSilent` equivalent**: `wt`'s `main.go` uses a simpler error path (`fmt.Fprintf(os.Stderr, "%s\n", err); os.Exit(wt.ExitGeneralError)`) with no `errSilent` sentinel like `hop` has. To avoid a redundant error line when brew is missing, the cobra wrapper in `update.go` SHOULD print its own hint (already done in `internal/update`) and return `nil` after a brew-not-found, rather than propagating the error. The `internal/update` package writes the user-facing hint to `errOut` before returning the sentinel; the cmd wrapper checks `errors.Is(err, update.ErrBrewNotFound)` and returns `nil` so `main.go` doesn't double-print. This preserves the user experience from `hop` without introducing `errSilent` machinery.

Alternatively, return a non-nil error and accept one extra "brew not found on PATH" line from `main.go` — that's still acceptable but slightly noisier. The intake assumes the cleaner suppress-via-nil approach; the spec stage may revisit.

### Tests

Mirror `hop`'s test files line-for-line with the same substitutions:

1. **`src/cmd/wt/update_test.go`** (new):
   - `TestUpdateCobraWiring` — runs `wt update`; binary is not in `/Cellar/`, so the function short-circuits to "was not installed via Homebrew" branch. Exercises cobra plumbing without hitting brew.
   - `TestUpdateRejectsArgs` — `wt update extra` returns an error (cobra.NoArgs).
   - `TestUpdateAppearsInHelp` — `wt --help` includes `wt update`.

2. **`src/internal/update/update_test.go`** (new):
   - `TestNormalizeVersion` — verbatim port of hop's table cases.
   - `TestRunNonBrewInstall` — when `isBrewInstalled()` returns false, asserts the manual-update hint is printed and `wt v0.0.3 was not installed via Homebrew` / `brew install sahil87/tap/wt` strings appear.
   - `TestIsBrewInstalledReturnsBool` — smoke test that the function doesn't panic.

The test helper `runArgs` already exists in `src/cmd/wt/testutil_test.go` (used by other cobra wiring tests in the package). If signature doesn't match `hop`'s, adapt the test calls accordingly — the helper API is the wt repo's convention.

## Affected Memory

No memory files exist yet (`docs/memory/index.md` is empty). The change is implementation + spec only; no behavioral surface that warrants memory hydrate beyond the spec update.

- `cli/update`: (new) — only IF the project decides to populate `docs/memory/cli/` to mirror `docs/specs/cli-surface.md`. Otherwise omit; the spec at `docs/specs/cli-surface.md` is the authoritative reference.

## Impact

- **New code**: `src/cmd/wt/update.go`, `src/cmd/wt/update_test.go`, `src/internal/update/update.go`, `src/internal/update/update_test.go`.
- **Modified**: `src/cmd/wt/main.go` (one-line addition: `updateCmd()` in the `AddCommand` block).
- **Modified spec**: `docs/specs/cli-surface.md` — add `wt update` entry with flags (none), exit codes (general error on brew failure, 0 on success or no-op), and behavior notes mirroring this intake's "What Changes" section.
- **Module dependencies**: No new external imports. `os/exec`, `encoding/json`, `errors`, `os`, `path/filepath`, `strings`, `time`, `context`, `io`, `fmt` are all stdlib and already used elsewhere in wt.
- **Build flow**: No changes — `main.version` is already stamped via `-ldflags` per `docs/specs/build-and-release.md`.
- **CI/release**: No changes. The Homebrew tap formula is updated by the existing release workflow.

## Open Questions

- Does the `runArgs` test helper in `src/cmd/wt/testutil_test.go` accept a `t *testing.T` and varargs of strings like hop's helper does? If the signatures diverge, the cobra wiring tests need adapted call sites — minor adaptation, not a design question.
- Should `wt update` accept a `--check` flag (print latest version, exit non-zero if behind, no upgrade)? Not in `hop update`; out of scope for parity port. Defer to a follow-up if requested.

## Clarifications

### Session 2026-05-09 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 7 | Confirmed | — |
| 8 | Confirmed | — |
| 11 | Confirmed | — |

### Session 2026-05-09 (tentative resolution)

| # | Question | Answer |
|---|----------|--------|
| 9 | Stream wiring for `brew update`/`brew info` (non-interactive) | stdout captured, stderr → parent (matches hop's `proc.Run`) |
| 10 | Stream wiring for `brew upgrade` (interactive) | Inherit all three streams (matches hop's `proc.RunForeground`) |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Tap formula `sahil87/tap/wt` (not `sahil87/tap/hop`) | `docs/specs/build-and-release.md` and `README.md` both reference this formula; release workflow publishes to it | S:95 R:95 A:95 D:95 |
| 2 | Certain | New package path `github.com/sahil87/wt/internal/update` | Module path is `github.com/sahil87/wt` per `go.mod` and constitution Module Path Stability rule; `internal/update` mirrors `internal/worktree` layout | S:95 R:95 A:95 D:95 |
| 3 | Certain | Use `os/exec` directly in `internal/update`, not a new `internal/proc` wrapper | wt convention: `os/exec` used directly throughout (context.go, stash.go, apps.go, cmd/init.go); constitution does not mandate a proc wrapper; introducing one for a single command is over-scoped | S:90 R:80 A:90 D:85 |
| 4 | Certain | Reuse existing `version` package var in `main.go`; pass to `update.Run` | `version` is already declared and `-ldflags`-stamped per build-and-release spec; no new build wiring needed | S:95 R:95 A:95 D:95 |
| 5 | Certain | Brew-not-found returns nil from cmd wrapper to avoid double-print | Clarified — user confirmed | S:95 R:80 A:75 D:65 |
| 6 | Certain | Port test cases line-for-line with `hop` → `wt` substitutions | Clarified — user confirmed | S:95 R:85 A:75 D:80 |
| 7 | Certain | Package doc comment in `internal/update` references wt instead of hop, drops the constitution Principle I citation (wt's constitution doesn't mandate proc routing) | Clarified — user confirmed | S:95 R:85 A:80 D:80 |
| 8 | Certain | Update `docs/specs/cli-surface.md` to add the `wt update` entry | Clarified — user confirmed | S:95 R:85 A:75 D:80 |
| 9 | Certain | Use `cmd.Stderr = os.Stderr; out, _ := cmd.Output()` for `brew update --quiet` and `brew info` (capturing stdout, passing stderr through) | Clarified — user confirmed (recommended option: avoids JSON parse corruption from stderr noise, matches hop's `proc.Run`) | S:95 R:65 A:60 D:50 |
| 10 | Certain | For `brew upgrade`, wire `cmd.Stdin/Stdout/Stderr` to `os.Stdin/Stdout/Stderr` to mirror hop's `RunForeground` (interactive subprocess, exits with subprocess code) | Clarified — user confirmed (recommended option: inherits all three streams so brew's tty-aware progress renders inline) | S:95 R:70 A:65 D:55 |
| 11 | Certain | No new dependencies in `go.mod` | Clarified — user confirmed | S:95 R:90 A:90 D:90 |

11 assumptions (11 certain, 0 confident, 0 tentative, 0 unresolved).
