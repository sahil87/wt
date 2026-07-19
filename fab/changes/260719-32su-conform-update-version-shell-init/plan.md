# Plan: Conform update, version, and shell-init to the shll toolkit standards

**Change**: 260719-32su-conform-update-version-shell-init
**Intake**: `intake.md`

> Apply-time re-audit (2026-07-20, `shll standards` at shll v0.1.7): all four intake
> verdicts CONFIRMED against the runtime standards text and current code —
> update help lacks the literal `--skip-brew-update` (MarkDeprecated hides it);
> `brew upgrade` runs under a 120s `exec.CommandContext` SIGKILL bound (`brew update`/
> `brew info` 30s, same kill mode); `wt shell-init` with a missing/unsupported shell
> emits the wrapper on stdout and exits 0; no test anywhere covers `--version`.
> The "already fully conformant → skip /git-pr" branch does NOT trigger.

## Requirements

### update: flag surface (`src/cmd/wt/update.go`)

#### R1: `--skip-brew-update` is a visible, non-deprecated flag
`wt update --help` MUST contain the exact literal substring `--skip-brew-update` (the update standard's frozen textual contract, probed by `strings.Contains`). The flag MUST NOT be deprecated or hidden — the `MarkDeprecated("skip-brew-update", ...)` call is removed. `--no-brew-update` SHALL remain registered and visible, bound to the same `skipBrewUpdate` bool (behavior identical whichever is passed). Passing either flag MUST print no deprecation warning. Help strings per the intake: `--skip-brew-update` carries the full description (`skip the internal `+"`brew update`"+` tap-metadata refresh (toolkit contract flag; version check and upgrade still run)`), `--no-brew-update` reads `alias for --skip-brew-update`.

- **GIVEN** a built `wt` binary
- **WHEN** `wt update --help` runs
- **THEN** stdout contains the literal substrings `--skip-brew-update` AND `--no-brew-update`
- **AND** passing `--skip-brew-update` prints no deprecation warning on stderr and still skips the internal `brew update`

### update: brew-handling safety (`src/internal/update/update.go`)

#### R2: `brew upgrade` runs with no timeout
The `brew upgrade` invocation MUST use plain `exec.Command` (no `context`, no bound). The `brewUpgradeTimeout` constant is removed. brew can legitimately block for minutes on the network; the call is interactive and stream-inheriting, so Ctrl-C (SIGINT, which brew traps and unwinds) remains the user's escape.

- **GIVEN** a `brew upgrade` that blocks longer than the former 120s bound
- **WHEN** `wt update` runs the upgrade
- **THEN** no timer fires and no signal is sent — brew completes at its own pace

#### R3: bounded brew calls terminate gracefully — no code path can SIGKILL brew
`brew update` SHALL keep a bound raised to **5 minutes**; `brew info` raised to **60 seconds**. Both MUST replace the default SIGKILL cancel with a graceful shape: `cmd.Cancel` sends **SIGTERM** and `cmd.WaitDelay` grants a **10-second** grace before the runtime's forced kill. The shape SHALL live in one shared helper so no bounded brew call site can regress to the SIGKILL default.

- **GIVEN** a bounded brew subprocess whose context expires
- **WHEN** the cancel fires
- **THEN** the process receives SIGTERM (trappable — brew can unwind), never an immediate SIGKILL
- **AND** a process that exits promptly after SIGTERM is never force-killed

### shell-init: strict argument contract (`src/cmd/wt/shell_init.go`)

#### R4: explicit `zsh`/`bash` argument emits only the eval-safe wrapper
`wt shell-init zsh` and `wt shell-init bash` MUST write exactly the wrapper function (`ShellWrapperFunc`, valid source for both shells — unchanged content) to stdout, exit 0, with stderr empty.

- **GIVEN** a built `wt` binary
- **WHEN** `wt shell-init zsh` (or `bash`) runs
- **THEN** stdout equals the wrapper bytes exactly, stderr is empty, exit code is 0
- **AND** `eval`ing the stdout in the named shell exits cleanly

#### R5: missing shell argument is a usage error — exit 2, usage on stderr, empty stdout
`wt shell-init` with no argument MUST be a usage error: message on **stderr** (naming the fix, e.g. `wt shell-init zsh|bash`), **empty stdout**, exit **`wt.ExitInvalidArgs` (2)**. The `$SHELL`-inference path (including the `filepath.Base("")` → `"."` case) is removed. Because `main.go` maps `RunE` errors to exit 1, the exit-2 path MUST use the established direct-exit pattern (`wt.ExitWithError(wt.ExitInvalidArgs, what, why, fix)` inside `RunE`, as `update.go` does for `ErrBrewNotFound`).

- **GIVEN** any environment (including `SHELL=/bin/zsh` set)
- **WHEN** `wt shell-init` runs with no argument
- **THEN** stdout is empty, stderr carries a usage message naming `wt shell-init zsh|bash`, and the exit code is 2

#### R6: unsupported shell argument is the same usage error
`wt shell-init fish` (any non-`zsh`/`bash` argument) MUST produce the same usage error: stderr message, empty stdout, exit 2. No more warn-and-emit-anyway.

- **GIVEN** a built `wt` binary
- **WHEN** `wt shell-init fish` runs
- **THEN** stdout is empty, stderr carries the usage message, and the exit code is 2

### version: pin the contract (SHOULD-level gap)

#### R7: a test pins `--version` exit 0 + first-line shape
A test SHALL pin the version contract: `wt --version` exits 0, writes to stdout (stderr empty), and the **first non-empty line** matches the standard's `<word> version <rest>` prefix shape (`wt version <token>` — cobra's canonical default). The regexes mirroring shll's `versionTokenRE`/`versionPrefixRE` document the contract in the test. No version *behavior* changes — the built binary already conforms.

- **GIVEN** the built test binary (a plain `go build`, version `dev`)
- **WHEN** `wt --version` runs
- **THEN** exit code is 0, stderr is empty, and the first non-empty stdout line matches `^wt version \S+$`

### shell-init: eval-safety test (SHOULD-level gap)

#### R8: a test evals shell-init output in a subshell
A test SHALL pipe `wt shell-init bash` stdout into a real `bash` subshell `eval` and assert a clean exit 0; the same for `zsh` when present on `PATH` (skip otherwise) — the standard's recommended cheapest guard against a poisoned blob.

- **GIVEN** the wrapper emitted by `wt shell-init bash`
- **WHEN** `bash -c 'eval "$WRAPPER"'` runs with the wrapper in `$WRAPPER`
- **THEN** the subshell exits 0

### docs: every no-arg `eval "$(wt shell-init)"` becomes shell-named

#### R9: docs, help text, and runtime hints name the shell
Every occurrence of the no-arg form `eval "$(wt shell-init)"` MUST become `eval "$(wt shell-init zsh)"` (mentioning bash where instructional), and prose describing the no-arg/`$SHELL`-inference behavior MUST be rewritten to the strict contract. Surfaces: `README.md` (quick-start eval, command table row, gotcha), `docs/site/install.md`, `docs/site/workflows.md` (command table, `wt shell-init` section, gotcha), `docs/site/skill.md` **and** `src/cmd/wt/skill.md` (byte-identical pair via `scripts/sync-skill.sh`; keep the drift-guard test and ≤150-line budget green), `docs/specs/cli-surface.md` § `wt shell-init` (spec amended in the same change per constitution § Test Integrity), the root command `Long` in `src/cmd/wt/main.go`, `shell_init.go`'s own `Use`/`Long`, and the runtime stderr hints in `src/internal/worktree/apps.go` and `src/cmd/wt/go.go` (plus `go.go`'s `Long`).

- **GIVEN** the repo after this change
- **WHEN** grepping for `shell-init)` across docs, README, and Go source
- **THEN** no runnable `eval "$(wt shell-init)"` (no-arg) form remains — every eval names a shell

#### R10: update-command docs reflect the new flag surface
`docs/specs/cli-surface.md` § `wt update` and `docs/site/workflows.md`'s update flag table MUST list both `--skip-brew-update` (the toolkit contract flag) and `--no-brew-update` (alias) as visible flags, with the "deprecated alias / hidden from --help" wording removed.

- **GIVEN** the amended docs
- **WHEN** reading either update flag table
- **THEN** both flags appear as visible, non-deprecated, behaviorally identical

### Non-Goals

- No change to `--version` output, timing, or wiring (already conformant; only a pinning test is added).
- No change to the wrapper function content (`ShellWrapperFunc`) — it is already valid zsh and bash source.
- No re-audit fixes for the other four standards (`principles`, `help-dump`, `readme-extraction`, `skill`) beyond the ripple surfaces this change touches.
- No `HOMEBREW_NO_GITHUB_API=1` injection — the standard offers it as an alternative to a bound on `brew upgrade`; dropping the bound entirely is the chosen conformance.

### Design Decisions

#### Contract flag un-hidden: `--skip-brew-update` visible alongside `--no-brew-update`
**Decision**: `--skip-brew-update` becomes a visible, non-deprecated flag carrying the full help text; `--no-brew-update` stays registered and visible as `alias for --skip-brew-update`; both bind one bool; the `MarkDeprecated` call is removed.
**Why**: the update standard freezes the literal substring `--skip-brew-update` in `--help` as a MUST (substring probe), and shll passes the flag on every toolkit-wide run — a deprecation warning on the contract flag every run is wrong. Nothing is removed, so no caller breaks.
**Rejected**: keeping `MarkDeprecated` but un-hiding via `flag.Hidden = false` (pflag still prints the deprecation warning on every use); dropping `--no-brew-update` (breaks the repo's own documented surface).
*Introduced by*: 260719-32su-conform-update-version-shell-init

#### Graceful-bound helper for brew metadata calls; upgrade unbounded
**Decision**: one shared constructor builds every bounded brew command with `cmd.Cancel` = SIGTERM and `cmd.WaitDelay` = 10s; `brew update` bound 5 min, `brew info` 60s; `brew upgrade` gets no bound at all (plain `exec.Command`).
**Why**: the standard's brew-handling MUSTs (no SIGKILL mid-transaction, no short hard timeout on upgrade); a single helper makes the no-SIGKILL property structural rather than per-call-site.
**Rejected**: a generous bound on `brew upgrade` (also conformant but adds a failure mode for zero benefit on an interactive call the user can Ctrl-C); per-call-site Cancel wiring (regression-prone).
*Introduced by*: 260719-32su-conform-update-version-shell-init

#### shell-init usage errors use the direct-exit pattern
**Decision**: missing/unsupported shell exits via `wt.ExitWithError(wt.ExitInvalidArgs, ...)` inside `RunE`, keeping `cobra.MaximumNArgs(1)` so the missing-arg branch is ours, not cobra's.
**Why**: toolkit convention is exit 2 for usage errors; `main.go` maps all `RunE` errors to exit 1, and a cobra `ExactArgs(1)` failure would take that path — the direct-exit pattern (precedent: `update.go`'s `ErrBrewNotFound`) is the established in-repo bypass.
**Rejected**: returning an error from `RunE` (exit 1, violates the standard's exit-2 convention); custom `Args` validator (same exit-1 mapping).
*Introduced by*: 260719-32su-conform-update-version-shell-init

### Deprecated Requirements

#### `wt shell-init` infers the shell from `$SHELL`
**Reason**: the shell-init standard states verbatim that a missing shell argument is a usage error (exit 2, usage on stderr, empty stdout); silent inference is the non-conformance.
**Migration**: users add the shell name — `eval "$(wt shell-init zsh)"` / `eval "$(wt shell-init bash)"`. Existing no-arg profiles degrade safely: stdout stays empty (nothing poisonous to eval), a usage line appears on stderr until migrated.

## Tasks

### Phase 1: Core Implementation

- [x] T001 Un-deprecate `--skip-brew-update` in `src/cmd/wt/update.go`: remove `MarkDeprecated`, register `--skip-brew-update` with the full contract help text and `--no-brew-update` as `alias for --skip-brew-update` (same bool) <!-- R1 -->
- [x] T002 Rework brew-handling in `src/internal/update/update.go`: delete `brewUpgradeTimeout` and run `brew upgrade` via plain `exec.Command`; add a shared graceful-bound helper (`cmd.Cancel`→SIGTERM, `cmd.WaitDelay` 10s); raise `brewUpdateTimeout` to 5 min and `brewInfoTimeout` to 60s and route both calls through the helper <!-- R2 R3 -->
- [x] T003 Rewrite `src/cmd/wt/shell_init.go` to the strict contract: `zsh`/`bash` arg → wrapper + exit 0; missing arg or unsupported arg → `wt.ExitWithError(wt.ExitInvalidArgs, ...)` with usage naming `wt shell-init zsh|bash`, empty stdout; remove `$SHELL` inference; update `Use` to `shell-init <shell>` and rewrite `Long` <!-- R4 R5 R6 -->
- [x] T004 Update in-binary copy: root `Long` in `src/cmd/wt/main.go`, `wt go` `Long` + wrapper hint in `src/cmd/wt/go.go`, and the "Open here" wrapper hint in `src/internal/worktree/apps.go` — every eval names a shell (`zsh`, mentioning bash where instructional) <!-- R9 -->

### Phase 2: Tests

- [x] T005 Update `src/cmd/wt/update_test.go`: rework `TestUpdate_NoBrewUpdateFlagInHelp` to assert BOTH flags visible; replace `TestUpdate_SkipBrewUpdateDeprecated` with a no-deprecation-warning assertion; keep/extend the literal-substring help probe mirroring shll's `strings.Contains` check <!-- R1 -->
- [x] T006 Extend `src/internal/update/update_test.go`: pin the new bounds/constants and add a graceful-cancel test — a fake slow `brew` on `PATH` that traps SIGTERM (writes a marker, exits) under a short test-injected context, proving SIGTERM (not SIGKILL) delivery via the shared helper <!-- R2 R3 -->
- [x] T007 Rewrite `src/cmd/wt/shell_init_test.go` to the new contract: zsh/bash happy paths (byte-exact stdout, exit 0, empty stderr); missing-arg and unsupported-arg → exit 2 + empty stdout + stderr usage (incl. `SHELL` env set/unset variants proving inference is gone); add the eval-in-subshell tests (`bash` always, `zsh` skipped when absent from PATH) <!-- R4 R5 R6 R8 -->
- [x] T008 Add `src/cmd/wt/version_test.go`: `wt --version` exits 0, stderr empty, first non-empty stdout line matches the canonical `wt version <token>` shape; document shll's `versionTokenRE`/`versionPrefixRE` in the test <!-- R7 -->
- [x] T009 Update `src/internal/worktree/apps_test.go` hint assertion to the shell-named eval form <!-- R9 -->

### Phase 3: Docs

- [x] T010 [P] Update `README.md`: quick-start eval line, command table `wt shell-init` row, troubleshooting/gotcha eval line <!-- R9 -->
- [x] T011 [P] Update `docs/site/install.md` (wrapper eval + prose) and `docs/site/workflows.md` (command table row, `wt shell-init` section contract text, gotcha eval, and the `wt update` flag table listing both visible flags) <!-- R9 R10 -->
- [x] T012 Update `docs/site/skill.md` (shell-named evals) and run `scripts/sync-skill.sh` so `src/cmd/wt/skill.md` stays byte-identical; keep ≤150 lines <!-- R9 -->
- [x] T013 [P] Amend `docs/specs/cli-surface.md`: § `wt shell-init` rewritten to the strict contract (args, exit 2 usage errors, empty stdout on error); § `wt update` flag table lists both visible flags, deprecated-alias wording removed <!-- R9 R10 -->

### Phase 4: Validation

- [x] T014 Run scoped tests (`cd src && go test ./cmd/wt/ ./internal/update/`), then the full suite (`go test ./...`), then `gofmt -l .` from `src/` — all green/clean <!-- R1 R2 R3 R4 R5 R6 R7 R8 -->

## Execution Order

- T001–T004 before their test tasks (T005–T009); docs tasks T010–T013 are independent of code but T012 depends on nothing else; T014 last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt update --help` contains the literal substrings `--skip-brew-update` and `--no-brew-update`; no `MarkDeprecated` remains on either; a test pins the substring presence
- [x] A-002 R2: `brew upgrade` is invoked via plain `exec.Command` with no context/bound; the `brewUpgradeTimeout` constant no longer exists
- [x] A-003 R3: `brew update` (5 min) and `brew info` (60s) both route through the shared graceful-bound helper (SIGTERM cancel + 10s `WaitDelay`); no brew call site retains the default SIGKILL cancel
- [x] A-004 R4: `wt shell-init zsh` and `wt shell-init bash` emit exactly the wrapper on stdout, exit 0, empty stderr
- [x] A-005 R5: `wt shell-init` (no arg) exits 2 with empty stdout and a stderr usage message naming `wt shell-init zsh|bash`; `$SHELL` inference code is gone
- [x] A-006 R6: `wt shell-init fish` exits 2 with empty stdout and the stderr usage message
- [x] A-007 R7: a version test pins `--version` → exit 0, stdout-only, first non-empty line `wt version <token>`
- [x] A-008 R8: an eval-in-subshell test evals the emitted wrapper in `bash` (and `zsh` when present) asserting exit 0

### Behavioral Correctness

- [x] A-009 R1: passing `--skip-brew-update` prints NO deprecation warning and still skips the internal `brew update` (existing skip semantics unchanged)
- [x] A-010 R5: with `SHELL=/bin/zsh` exported, `wt shell-init` (no arg) still exits 2 — the env var no longer influences behavior

### Removal Verification

- [x] A-011 R5: no `os.Getenv("SHELL")`/`filepath.Base` inference remains in `shell_init.go`; the warn-and-emit-anyway path is gone
- [x] A-012 R9: no runnable no-arg `eval "$(wt shell-init)"` remains anywhere in docs, README, skill bundle, spec, or Go string literals

### Scenario Coverage

- [x] A-013 R3: a test proves a bounded brew call's context expiry delivers a trappable SIGTERM (marker written by the fake brew's trap handler)
- [x] A-014 R4: happy-path shell-init tests assert byte-exact wrapper stdout for both shells

### Edge Cases & Error Handling

- [x] A-015 R5: usage-error stdout emptiness is asserted (eval-safety on the error path); exit codes asserted via the built-binary harness (`runWt`), since `os.Exit` paths cannot be asserted in-process

### Code Quality

- [x] A-016 Pattern consistency: new code follows the direct-exit precedent (`update.go`), the `WT_TEST_FORCE_BREW`/fake-brew-on-PATH test seam, and existing cobra registration shapes
- [x] A-017 No unnecessary duplication: the SIGTERM+grace shape lives in one shared helper; no magic numbers — bounds are named constants
- [x] A-018 Docs integrity: `docs/site/skill.md` and `src/cmd/wt/skill.md` are byte-identical (drift-guard test green) and ≤150 lines; help-dump tests remain green with the changed update-node flag shape

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
- Pipeline conditional from intake: re-audit CONFIRMED all four gaps (see header note) — the "skip /git-pr" branch is dead for this change.

## Deletion Candidates

None — this change already deleted every symbol it rendered redundant (the
`brewUpgradeTimeout` constant, the `MarkDeprecated("skip-brew-update", …)`
call, and the `$SHELL`/`filepath.Base` inference block in `shell_init.go`).
Verified at review: no orphaned constants, unused imports, or dead branches
remain (a clean `go build` + `go vet` confirms — Go would fail on unused
imports/symbols). No further deletion opportunities discovered.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | `--skip-brew-update` listed first with the full contract help text; `--no-brew-update` reads `alias for --skip-brew-update` (intake's suggested strings, near-verbatim) | Intake supplies the strings; ordering/wording is presentational and trivially reversible | S:80 R:90 A:85 D:75 |
| 2 | Confident | SIGTERM+grace pinned by a short-timeout fake-brew trap test through a shared helper (test injects a small context), not a 5-minute e2e | Intake explicitly defers the exact seam to apply and names the fake-brew-on-PATH convention; helper + injected context is the cheapest real behavioral pin | S:70 R:85 A:80 D:70 |
| 3 | Certain | Missing-arg handling stays inside `RunE` with `cobra.MaximumNArgs(1)` (not `ExactArgs(1)`) so the exit-2 direct-exit path is ours | `ExactArgs` failures route through `main.go`'s exit-1 mapper — mechanically incompatible with the standard's exit-2 convention; intake assumption 5 prescribes the direct-exit pattern | S:85 R:90 A:95 D:90 |
| 4 | Confident | Version test asserts the `<word> version <rest>` prefix shape (`^wt version \S+$`) on the dev-built test binary; the semver token regex is documented, not asserted (plain `go build` stamps `dev`) | The test harness builds without ldflags, so `wt version dev` is the observable output; the prefix shape is the contract clause a dev build can pin | S:75 R:90 A:85 D:80 |
| 5 | Confident | Runtime stderr hints (`apps.go`, `go.go`) and `wt go`'s `Long` are in-scope for the shell-named eval rewrite, plus their test assertion (`apps_test.go`) | The hints are copy users paste; leaving the no-arg form would instruct users into the new usage error — intake's "every occurrence" doc rule extends to Go string literals | S:75 R:90 A:85 D:80 |
| 6 | Certain | `docs/site/workflows.md`'s update flag table gains both visible flags (it currently lists only `--skip-brew-update`) | Pure doc ripple of R1; both flags are now visible so both tables list both | S:85 R:95 A:90 D:90 |

6 assumptions (2 certain, 4 confident, 0 tentative).
