# Plan: Phase Separators in Worktree Creation Output

**Change**: 260531-pnmi-add-phase-separators
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
