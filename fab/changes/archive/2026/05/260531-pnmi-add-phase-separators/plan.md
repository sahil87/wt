# Plan: Phase Separators in Worktree Creation Output

**Change**: 260531-pnmi-add-phase-separators
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- **No terminal-width detection** — the separator rule is a fixed width; it does not query the
  terminal size or adapt to it. Rationale: determinism for tests and the single-binary / no-hidden-state posture.
- **No change to exit codes, rollback behavior, signal handling, or the init resolution contract**
  (`ResolveInitInvocation`) — this change is purely additive output structure plus one stream realignment.
- **No new output on `wt create`'s stdout** — the stdout final-path line (`launcher-contract.md`)
  is untouched. Separators never appear on stdout for `wt create`.
- **No separators around the validation phase or the dirty-state prompt** — separators bracket
  only the Git, Init, and Open phases that follow successful argument validation.

## Output Contract: Phase Separators

### Requirement: PhaseSeparator helper
A single helper `PhaseSeparator(label string) string` SHALL be added to
`src/internal/worktree/errors.go`, alongside the existing output helpers (`WtError`,
`PrintError`, `PrintInitFailureBanner`), and SHALL be the sole producer of phase-separator
strings. The helper SHALL:

- Return a single labeled rule line **without** a trailing newline (callers add newlines via the
  `Fprintln` they already use).
- Use a fixed total width of **40 columns** (label plus rule glyphs), and SHALL NOT query the
  terminal size.
- Render the label using the existing `ColorBold` treatment when color is enabled.
- Produce a **unicode** rule (`──`, U+2500) framing the label when color is enabled
  (e.g. `── Git ──────────────────────────`).
- Produce a **plain-ASCII** rule (`--`) framing the label when `NO_COLOR` is set, reusing the
  package's existing color-blanking mechanism (the `init()` that blanks `ColorBold` etc. when
  `NO_COLOR` is non-empty). When the color variables are blank, the helper SHALL emit the
  ASCII form and SHALL NOT emit any ANSI escape sequences.

#### Scenario: Colored form
- **GIVEN** `NO_COLOR` is unset
- **WHEN** `PhaseSeparator("Git")` is called
- **THEN** the result contains the label `Git`
- **AND** the result contains the unicode rule glyph `─` (U+2500)
- **AND** the result contains the `ColorBold` ANSI escape around the label

#### Scenario: NO_COLOR form
- **GIVEN** `NO_COLOR` is set to a non-empty value
- **WHEN** `PhaseSeparator("Git")` is called
- **THEN** the result contains the label `Git`
- **AND** the result contains the ASCII rule glyph `-`
- **AND** the result contains no ANSI escape sequences (no `\033[`)

#### Scenario: Fixed width, no trailing newline
- **GIVEN** any label shorter than the fixed width
- **WHEN** `PhaseSeparator(label)` is called
- **THEN** the returned string has no trailing newline
- **AND** the total visible width (label + glyphs + spaces, excluding ANSI escapes) equals the fixed 40-column target

### Requirement: wt create emits Git, Init, and Open separators on stderr
`wt create` SHALL emit three phase separators to **stderr** (never stdout), in order, each
immediately preceding the output of its phase:

1. `PhaseSeparator("Git")` immediately before the deferred summary block
   (`Created worktree:` / `Path:` / `Branch:`).
2. The **Init** separator, emitted by the init runner (see *Init Protocol* domain below), not by
   `create.go` directly.
3. `PhaseSeparator("Open")` immediately before the open phase (the `worktree-open` branch).

The existing summary, warning, and prompt strings SHALL be preserved verbatim (in particular the
substrings `Created worktree:` and `Open in:`). The stdout final-path line SHALL remain the only
thing `wt create` writes to stdout and SHALL be byte-identical to today's output.

#### Scenario: All three separators present on stderr
- **GIVEN** a valid git repository and `wt create` with init enabled and an app/open step
- **WHEN** `wt create` runs to completion
- **THEN** stderr contains a separator labeled `Git`, one labeled `Init` (with the resolved command), and one labeled `Open`
- **AND** they appear in that order

#### Scenario: stdout contract preserved
- **GIVEN** a successful `wt create` whose open step is not `open_here`
- **WHEN** the command completes
- **THEN** stdout consists solely of the absolute worktree path line (no separators, no summary, no init output)

#### Scenario: Git separator placement respects the reinstall-window contract
- **GIVEN** the create flow has completed `git worktree add` and is emitting the deferred summary
- **WHEN** the `Git` separator is emitted
- **THEN** it is emitted as part of the existing deferred-summary emission (under the rollback handler, before the init-phase `signal.Reset`)
- **AND** no new I/O or user prompt is introduced inside the tight reinstall window between `git worktree add` returning and `signal.Reset`

### Requirement: Open separator omitted when open phase is skipped
When the open phase does not run (`--worktree-open=skip`, or `--non-interactive` defaulting to
`skip`), `wt create` SHALL NOT emit the `Open` separator. More generally, a phase separator SHALL
be emitted only when its phase runs and produces diagnostic output:
- The **Git** separator SHALL be emitted on every successful create, since the deferred summary
  block always prints (it is the Git phase's output).
- The **Init** separator SHALL be emitted only when the init phase actually runs an init command —
  i.e. not when init is disabled and not on the `*InitNotFound` path (see *Init Protocol* below).
- The **Open** separator SHALL be emitted only when the open phase runs (any `--worktree-open`
  value other than `skip`).
<!-- clarified: generalized the "emitted only when its phase produces output" rule per assumption #10, resolving the apparent contradiction with the always-emitted Git separator — Git always emits (summary always prints), Init emits only when a command runs (not disabled, not not-found), Open emits only when not skipped; matches create.go control flow (deferred summary at L264, runner short-circuits to RenderWarning, open block gated on worktreeOpen != "skip") -->
A separator never precedes a phase that emits nothing.

#### Scenario: Skip open
- **GIVEN** `wt create --non-interactive` (open defaults to `skip`)
- **WHEN** the command runs
- **THEN** stderr contains no `Open` separator
- **AND** stderr still contains the `Git` and `Init` separators

## Init Protocol: Init Separator and Stream Alignment

### Requirement: Init separator owned by the runner, labeled with the resolved command
The **Init** phase separator SHALL be emitted by the code that owns the existing
`Running worktree init...` line, so it is produced exactly once regardless of caller:

- `RunWorktreeSetupWithObserver` in `src/internal/worktree/crud.go` (used by `wt create`).
- `runInitScript` in `src/cmd/wt/init.go` (the standalone `wt init` path).

The separator label SHALL be `Init (<cmd>)` where `<cmd>` is the resolved init command as the
user would recognize it — for a command invocation, the full command string (e.g. `fab sync`);
for a file-path invocation, the path. The label SHALL be derived from the same init-script value
that resolution uses (`InitScriptPath()` / the resolved `*exec.Cmd`), never hardcoded.

When init resolution returns a structured not-found outcome (`*InitNotFound`), the existing
`RenderWarning()` path SHALL be used unchanged and **no** Init separator SHALL be emitted (there
is no command to label).

The substrings `Running worktree init` and `Worktree init complete` SHALL remain present in the
output (the separator augments, it does not replace these markers), preserving existing test
assertions.

#### Scenario: Init separator labeled with default command
- **GIVEN** `WORKTREE_INIT_SCRIPT` is unset (default `fab sync`) and `fab` is on PATH
- **WHEN** the init runner runs
- **THEN** stderr contains a separator whose label is `Init (fab sync)`

#### Scenario: Init separator labeled with custom script path
- **GIVEN** `WORKTREE_INIT_SCRIPT=scripts/setup.sh` and the file exists under the repo root
- **WHEN** the init runner runs
- **THEN** stderr contains a separator whose label is `Init (scripts/setup.sh)`

#### Scenario: No separator on not-found
- **GIVEN** `WORKTREE_INIT_SCRIPT=__nonexistent_cmd__` (not on PATH)
- **WHEN** the init runner runs
- **THEN** the canonical not-found warning is printed
- **AND** no Init separator is emitted
- **AND** the command exits 0 (graceful skip, unchanged)

### Requirement: wt init streams init diagnostics to stderr
`wt init` (`runInitScript`) SHALL emit its init diagnostics — the `Running worktree init...` /
`Worktree init complete.` banner, the Init separator, and the init script's own stdout/stderr —
to **stderr**, matching `wt create`'s init runner. Specifically, the wired `cmd.Stdout` and
`cmd.Stderr` for the init child SHALL both be `os.Stderr`, and the banner SHALL be written with
`fmt.Fprintln(os.Stderr, ...)`.

Rationale: `wt init` has no machine-readable result (it is a side-effecting command whose outcome
is its exit code), so all of its output is diagnostic and belongs on stderr per the
stdout=machine-result / stderr=diagnostics convention that `wt create` already follows.

This realignment SHALL NOT change `wt init`'s exit-code behavior, the graceful-skip behavior, or
any output text — only the stream (stdout → stderr).

#### Scenario: Diagnostics on stderr
- **GIVEN** a valid repo and a resolvable init script that prints to stdout and exits 0
- **WHEN** `wt init` runs
- **THEN** the `Running worktree init` banner, the Init separator, and the script's output all appear on stderr
- **AND** stdout is empty

#### Scenario: Existing combined-stream assertions still pass
- **GIVEN** the existing `wt init` tests that assert on `r.Stdout + r.Stderr`
- **WHEN** the realigned `wt init` runs
- **THEN** the combined output still contains `Running worktree init`, the script's output, and `Worktree init complete`

## Design Decisions

1. **Single helper in `errors.go` as the sole separator producer**
   - *Why*: `errors.go` already centralizes user-facing output formatting (`WtError`,
     `PrintInitFailureBanner`) and owns the `NO_COLOR` blanking `init()`. Co-locating
     `PhaseSeparator` reuses that mechanism and keeps the canonical wording in one place,
     mirroring how `RenderWarning` is the single source for not-found warnings.
   - *Rejected*: inlining the rule construction at each call site — would duplicate the
     glyph/width/color logic across `create.go`, `crud.go`, and `init.go` and invite drift.

2. **Init separator owned by the runner, not `create.go`**
   - *Why*: the runner already prints `Running worktree init...` and already holds the resolved
     `*exec.Cmd`, so it can label the separator with the resolved command and guarantee exactly
     one emission for both `wt create` and `wt init`.
   - *Rejected*: emitting it from `create.go` — would either duplicate the resolution logic or
     double-label when `wt init` runs the same runner.

3. **Align `wt init` to stderr (stream realignment in scope)**
   - *Why*: resolves the cross-command inconsistency (`wt create` uses stderr, `wt init` used
     stdout for identical init diagnostics) and makes separator placement uniform. `wt init`
     has no stdout contract — its tests assert on combined streams and no spec pins it to stdout.
   - *Rejected*: keeping each command on its own stream — enshrines the inconsistency and makes
     the separator behavior asymmetric between two commands doing the same thing.

4. **Fixed 40-column width, no terminal-size query**
   - *Why*: deterministic output for unit tests; no dependency on terminal/ioctl; consistent with
     the project's single-binary, no-hidden-state posture.
   - *Rejected*: dynamic width via terminal detection — non-deterministic in tests and adds a
     platform-specific code path for a cosmetic gain.


## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 1: Core helper

- [x] T001 Add `PhaseSeparator(label string) string` to `src/internal/worktree/errors.go`, alongside `WtError`/`PrintError`/`PrintInitFailureBanner`. Fixed 40-column visible width, no trailing newline, no terminal-size query. Colored form (when `ColorReset`/`ColorBold` non-empty): unicode `─` (U+2500) rule framing a `ColorBold` label. NO_COLOR form (when color vars blanked by the existing `init()`): ASCII `-` rule, no ANSI escapes. Detect color via the blanked package-level vars, not `os.Getenv`. <!-- A-001 A-003 -->

### Phase 2: Runner-owned Init separator + stream alignment

- [x] T002 In `RunWorktreeSetupWithObserver` (`src/internal/worktree/crud.go`), emit `PhaseSeparator("Init (" + initScript + ")")` to stderr immediately before the existing `Running worktree init...` line. Keep the `Running worktree init` / `Worktree init complete` substrings. Do NOT emit a separator on the `*InitNotFound` path (RenderWarning unchanged). <!-- A-004 A-005 -->

- [x] T003 In `runInitScript` (`src/cmd/wt/init.go`): realign banner from `fmt.Println` to `fmt.Fprintln(os.Stderr, ...)` for `Running worktree init...` / `Worktree init complete.`; change `cmd.Stdout = os.Stdout` to `cmd.Stdout = os.Stderr` (Stderr/Stdin unchanged); emit `PhaseSeparator("Init (" + initScriptRel + ")")` to stderr (same label derivation), only after a command resolves (not on the `*InitNotFound` path). Preserve exit-code and graceful-skip behavior. <!-- A-006 A-008 -->

### Phase 3: wt create Git + Open separators

- [x] T004 In `src/cmd/wt/create.go`, emit `PhaseSeparator("Git")` to stderr immediately before the deferred summary block (`Created worktree:` / `Path:` / `Branch:`), joining the existing deferred-summary emission under the rollback handler — before the init-phase `signal.Reset`, never inside the reinstall window. <!-- A-002 A-007 -->

- [x] T005 In `src/cmd/wt/create.go`, emit `PhaseSeparator("Open")` to stderr immediately before the open phase, gated on `worktreeOpen != "skip"` so it is omitted for the skip case. Do NOT emit the Init separator from create.go (runner owns it). Leave `fmt.Println(wtPath)` as the only stdout write. <!-- A-002 A-009 -->

### Phase 4: Tests

- [x] T006 Add a unit test for `PhaseSeparator` in `src/internal/worktree/errors_test.go`: label present; colored form contains `─` and an ANSI escape; NO_COLOR form (save/blank/restore `ColorBold`/`ColorReset` per existing test precedent) contains `-`, no `\033[`, and the label; visible width (ANSI-stripped) == 40; no trailing newline. <!-- A-010 -->

- [x] T007 Run the existing affected suites (`./internal/worktree/`, `./cmd/wt/`) and confirm they stay green: `create_test.go` stderr `Created worktree:` + stdout-is-one-line guard; `init_test.go` combined `Running worktree init` / `Worktree init complete`; not-found paths emit no separator. Run `gofmt -l` on changed files and `go vet`. <!-- A-011 A-012 -->

## Execution Order

- T001 blocks T002, T003, T004, T005 (callers depend on the helper).
- T002 and T003 are the two Init-separator call sites (independent of each other once T001 lands).
- T004 and T005 both edit create.go (sequential to avoid edit conflicts).
- T006 depends on T001; T007 depends on all implementation tasks.

## Acceptance

### Functional Completeness

- [x] A-001 PhaseSeparator helper: `PhaseSeparator(label string) string` exists in `errors.go` alongside the other output helpers, returns a single labeled rule line with no trailing newline, fixed 40-column visible width, and never queries terminal size.
- [x] A-002 create emits Git/Open on stderr: `wt create` emits `PhaseSeparator("Git")` before the deferred summary and `PhaseSeparator("Open")` before the open phase, both to stderr, never to stdout.

### Behavioral Correctness

- [x] A-003 Colored vs NO_COLOR form: colored form uses unicode `─` framing a `ColorBold` label with ANSI escapes; NO_COLOR form (color vars blanked) uses ASCII `-` and emits no `\033[` sequence; detection uses the blanked package vars (`colorEnabled := ColorReset != ""`), not a fresh `os.Getenv`.
- [x] A-004 Init separator owned by runner: `RunWorktreeSetupWithObserver` emits the `Init (<cmd>)` separator exactly once (crud.go:157), labeled with the resolved init-script value (`initScript` param == `InitScriptPath()`), and the `Running worktree init` / `Worktree init complete` substrings remain present.
- [x] A-006 wt init stream alignment: `runInitScript` writes its banner and the init child's stdout to stderr (`cmd.Stdout = os.Stderr` at init.go:83), emits the matching `Init (<cmd>)` separator, and preserves exit-code (`ExitInitFailed`) and graceful-skip behavior.

### Scenario Coverage

- [x] A-005 No separator on not-found: when init resolution returns `*InitNotFound`, the RenderWarning path runs unchanged (crud.go:148-151, init.go:69-73) and no Init separator is emitted (verified by the existing `TestInit_WarningTextMatchesResolver_*` byte-identity assertions staying green).
- [x] A-007 Reinstall-window contract: the Git separator (create.go:264) joins the existing deferred-summary emission under the rollback handler, before `signal.Reset` (create.go:282), introducing no new I/O inside the tight reinstall window.
- [x] A-009 Open separator skip: when `worktreeOpen == "skip"` (incl. `--non-interactive` default), no Open separator is emitted (gated at create.go:318); the Git (and Init, when run) separators still appear. Covered by the `--non-interactive` create tests (open defaults to skip) which stay green.
- [x] A-010 PhaseSeparator unit test: `errors_test.go` asserts label presence, colored ANSI + `─`, NO_COLOR ASCII `-` with no `\033[`, 40-column visible width (rune-counted via `stripANSI`), and no trailing newline.

### Edge Cases & Error Handling

- [x] A-008 stdout contract preserved: `wt create`'s stdout remains exactly the single worktree-path line (`fmt.Println(wtPath)` at create.go:376 is the only stdout write); no separator leaks to stdout (the `create_test.go` one-line-stdout guard stays green).
- [x] A-012 Existing suites green: `./internal/worktree/` and `./cmd/wt/` tests pass (uncached), including the `Created worktree:` / combined-stream init assertions.

### Code Quality

- [x] A-011 Pattern consistency: new code follows surrounding naming, error, and structure conventions; reuses the existing color vars and `fmt.Fprintln(os.Stderr, ...)` style; 40-width is the named const `phaseSeparatorWidth`; `gofmt -l` clean and `go vet` clean.
- [x] A-013 No unnecessary duplication: separator construction lives solely in `PhaseSeparator`; call sites do not reinline glyph/width/color logic.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

- None — this change adds new functionality (the `PhaseSeparator` helper and its call sites) plus one stream realignment in `wt init`; it does not make any existing code redundant or unused.
