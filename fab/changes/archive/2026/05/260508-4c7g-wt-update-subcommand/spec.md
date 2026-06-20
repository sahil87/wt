# Spec: wt update subcommand

**Change**: 260508-4c7g-wt-update-subcommand
**Created**: 2026-05-09
**Affected memory**: *(none — `docs/memory/index.md` is empty; the spec at `docs/specs/cli-surface.md` is the authoritative CLI reference and SHALL be amended to document `wt update`)*

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Tap formula `sahil87/tap/wt` | Confirmed from intake #1; verified in `docs/specs/build-and-release.md` and `README.md` | S:95 R:95 A:95 D:95 |
| 2 | Certain | New package path `github.com/sahil87/wt/internal/update` | Confirmed from intake #2; module path stable per constitution | S:95 R:95 A:95 D:95 |
| 3 | Certain | Use `os/exec` directly in `internal/update` (no new `internal/proc`) | Confirmed from intake #3; promoted to Design Decision 1 | S:95 R:80 A:90 D:85 |
| 4 | Certain | Reuse existing `main.version` var stamped via `-ldflags` | Confirmed from intake #4; promoted to Design Decision 3 | S:95 R:95 A:95 D:95 |
| 5 | Certain | Brew-not-found: cobra wrapper exits directly via `os.Exit(wt.ExitGeneralError)` after `internal/update` prints the hint | Confirmed from intake #5 (clarified); refined to: cobra wrapper calls `os.Exit(wt.ExitGeneralError)` directly (NOT `wt.ExitWithError`, which would add a second structured stderr line and violate the "exactly one line" contract). Documented in Design Decision 2. | S:95 R:80 A:75 D:80 |
| 6 | Certain | Port test cases line-for-line with hop→wt substitutions | Confirmed from intake #6 (clarified) | S:95 R:85 A:75 D:80 |
| 7 | Certain | Drop hop's "Constitution Principle I" doc-comment citation in `internal/update` | Confirmed from intake #7 (clarified); wt constitution has no analogous rule | S:95 R:85 A:80 D:80 |
| 8 | Certain | Add `wt update` entry to `docs/specs/cli-surface.md` | Confirmed from intake #8 (clarified) | S:95 R:85 A:75 D:80 |
| 9 | Certain | `cmd.Stderr = os.Stderr; cmd.Output()` for `brew update`/`brew info` | Confirmed from intake #9 (clarified); avoids JSON parse corruption | S:95 R:65 A:60 D:50 |
| 10 | Certain | `brew upgrade` inherits all three streams (stdin/stdout/stderr → os.*) | Confirmed from intake #10 (clarified); enables tty-aware progress | S:95 R:70 A:65 D:55 |
| 11 | Certain | No new `go.mod` dependencies | Confirmed from intake #11 (clarified); all imports are stdlib | S:95 R:90 A:90 D:90 |
| 12 | Certain | Sentinel error name: `update.ErrBrewNotFound` (package-local exported) | Mirrors `proc.ErrNotFound` from hop without coupling to hop's package; package-local since it's consumed by exactly one cmd wrapper | S:90 R:85 A:90 D:85 |
| 13 | Certain | Timeouts: 30s for `brew update`, 30s for `brew info`, 120s for `brew upgrade` | Verbatim port from hop; no project-specific reason to deviate | S:90 R:90 A:90 D:90 |
| 14 | Certain | Manual-install hint string: `Update manually, or reinstall with: brew install sahil87/tap/wt` | Verbatim port from hop with formula substitution; user-recognizable from hop equivalent | S:95 R:95 A:95 D:95 |
| 15 | Certain | Brew-not-found stderr line: `wt update: brew not found on PATH.` | Verbatim port from hop with binary-name substitution | S:95 R:95 A:95 D:95 |
| 16 | Certain | Already-up-to-date stdout line: `Already up to date ({currentVersion}).` | Verbatim port from hop — preserves the un-normalized form so users see exactly what they invoked | S:95 R:95 A:95 D:95 |
| 17 | Certain | Updated-success stdout line: `Updated to v{normalizedLatest}.` | Verbatim port from hop — explicit `v` prefix for the latest, distinguishing it from brew's bare form | S:95 R:95 A:95 D:95 |
| 18 | Certain | Test helper is `runWt(t, dir, env, args...)` in `cmd/wt/testutil_test.go` (NOT hop's `runArgs(t, args...)`); update_test.go SHALL use `runWt` with `createTestRepo(t)` for the working directory and `nil` env | Clarified — verified by inspecting `src/cmd/wt/testutil_test.go` and existing call sites (e.g., `init_test.go`, `create_test.go`); intake Open Questions correctly flagged this divergence | S:95 R:90 A:95 D:95 |
| 19 | Confident | Subcommand registered in alphabetical-leaning order in `main.go` (between `shellSetupCmd` and end, or near `initCmd`) | Existing order is loose; appended at end of `AddCommand` block is the safest, least-disruptive placement | S:75 R:95 A:80 D:80 |
| 20 | Confident | `update.go` package doc comment is brief (3-5 lines): purpose, formula constant rationale, and a note that `os/exec` is used directly per wt convention | Mirrors hop's doc-comment density without copying its hop-specific constitution citation | S:80 R:90 A:85 D:80 |

20 assumptions (18 certain, 2 confident, 0 tentative, 0 unresolved).
<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
