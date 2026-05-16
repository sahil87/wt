# Plan: Arrow-Key Menu Navigation

**Change**: 260516-dkg7-arrow-key-menu-navigation
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 1: Setup

<!-- Scaffolding, dependencies, configuration. No business logic. -->

- [x] T001 Add `golang.org/x/term` as a direct dependency: run `go get golang.org/x/term@latest` from `src/`, then `go mod tidy` to update `src/go.mod` and `src/go.sum`. Confirm no other entries (charmbracelet/AlecAivazis/manifoldco) appear.
- [x] T002 Verify existing `src/internal/worktree/menu.go` fallback behavior pre-change: run `go test ./internal/worktree/...` from `src/` to establish a green baseline (or note no menu tests yet exist) before introducing new files.

### Phase 2: Core Implementation

<!-- Primary functionality. Order by dependency — earlier tasks are prerequisites for later ones. -->

- [x] T003 In `src/internal/worktree/menu.go`, introduce internal types for the pure state machine: `menuState` (current `highlight` index where `0` denotes the Cancel row and `1..N` denotes options, plus `numOptions` and `defaultIdx`), `keyEvent` (sum-type tagged with one of `keyUp`, `keyDown`, `keyEnter`, `keyCancel`, `keyDigit`, `keyIgnore`; `Digit` carries the numeric value 0–9), and `menuStateTransition` (new `highlight`, `submitted` bool). Keep all types unexported.
- [x] T004 Implement the pure function `nextMenuState(prev menuState, key keyEvent) menuStateTransition` in `src/internal/worktree/menu.go`. Encode: `Up`/`Down` move highlight with wrap-around between Cancel (row `0`) and the first/last option, `Enter` submits the current highlight, `Cancel` returns highlight `0` with `submitted=true`, `Digit(0)` submits the Cancel row, `Digit(1..numOptions)` submits that option, out-of-range digits and `Ignore` keep state unchanged with `submitted=false`. No I/O, no globals.
- [x] T005 [P] Implement the pure escape parser `parseKey(buf []byte) keyEvent` (or an equivalent small reader interface) in `src/internal/worktree/menu.go`. Map `\x1b[A` → `Up`, `\x1b[B` → `Down`, `\r`/`\n` → `Enter`, `\x03` → `Cancel`, `q` → `Cancel`, `j` → `Down`, `k` → `Up`, ASCII `0`–`9` → `Digit(n)`, bare `\x1b` (no follow-up byte available) → `Cancel`, unknown sequences (including `\x1bOP` F1, Tab, Backspace) → `Ignore`. Inject the 50ms timeout via a small interface so tests can fake the clock.
- [x] T006 [P] Add `isInteractiveTTY()` (or equivalent helper) in `src/internal/worktree/menu.go` that calls `term.IsTerminal(int(os.Stdin.Fd()))` AND `term.IsTerminal(int(os.Stdout.Fd()))`, returning `true` only if both are TTYs. Document the rule that detection runs once per `ShowMenu` invocation before any output.
- [x] T007 Implement the raw-mode I/O shell `runInteractiveMenu(prompt string, options []string, defaultIdx int) (int, error)` in `src/internal/worktree/menu.go`. Responsibilities: call `term.MakeRaw(int(os.Stdin.Fd()))`, `defer term.Restore(...)` for guaranteed cooked-mode restore on every exit (submit/Cancel/panic), seed initial highlight per the rule (`defaultIdx >= 1` highlights that row; `defaultIdx == 0` highlights Cancel; `defaultIdx == -1` highlights row 1), perform initial paint, then loop reading bytes from stdin → `parseKey` → `nextMenuState` → in-place redraw via `\x1b[<N>A`, `\x1b[2K`, `\r`. Preserve the existing `(default)` green marker on the default row and apply reverse-video highlight on the current row (with a `›` gutter marker per intake §2). Return `0, nil` for Cancel and `idx, nil` for submit.
- [x] T008 Implement the in-place redraw helper inside the shell from T007: compute total rendered rows (prompt + N options + Cancel row), and on each state transition emit `\x1b[<rows>A` to move cursor up, then `\x1b[2K` per line followed by the row content. On final exit emit `\x1b[<rows>A`, `\x1b[2K` per line, then write a single line: `<prompt> <option-text>` on submit or `<prompt> (cancelled)` on Cancel.

### Phase 3: Integration & Edge Cases

<!-- Wire components together. Handle error states, edge cases, validation. -->

- [x] T009 Refactor `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)` in `src/internal/worktree/menu.go` to branch on `isInteractiveTTY()` at entry (before any output). On `true`, delegate to `runInteractiveMenu`. On `false`, run the existing numbered-prompt body verbatim — preserve the exact `Choice [N]: ` / `Choice: ` prompts, the `Invalid choice. Please enter a number.` and `Invalid choice. Please enter a number between 0 and N.` validation messages, and byte-for-byte stdout. Keep the public signature unchanged.
- [x] T010 Decide and record the Windows behavior per spec **Cross-Platform** requirement: either (a) wire raw-mode on Windows ConPTY through `term.MakeRaw` if it works out of the box, or (b) explicitly short-circuit `isInteractiveTTY()` to `false` on `runtime.GOOS == "windows"`. Add a brief code comment in `src/internal/worktree/menu.go` documenting the chosen path so hydrate can capture it in the memory contract.
- [x] T011 Add a signal-restore safety net: if the interactive path is reached and a SIGINT could be delivered between key reads, ensure the `defer term.Restore(...)` in T007 fires. Audit `src/internal/worktree/rollback.go` and `signal_unix.go` for any conflicting global SIGINT handlers; if one exists, document interaction (raw mode swallows `\x03` as a byte, so no SIGINT should escape mid-menu) or coordinate restore. No new public surface — internal comment plus, if needed, a `signal.Notify`/`signal.Stop` guard scoped to the function. Audit: no `signal_unix.go` exists in `src/internal/worktree/`; `rollback.go` has no `signal.Notify` handlers. Raw mode swallows `\x03` as a single byte (parsed by `parseKey` as `keyCancel`), so no SIGINT escapes mid-menu. The deferred `term.Restore` in `runInteractiveMenu` is the sole guarantee for cooked-mode restore on every exit path including panic; no additional `signal.Notify` guard is required. Documented inline in `runInteractiveMenu`.
- [x] T012 Create `src/internal/worktree/menu_test.go` with unit tests for `nextMenuState`: cover every keybinding (Up, Down, Enter, Cancel, Digit, Ignore), wrap-around in both directions (last option → Cancel, Cancel → first option, first option → Cancel via Up), digit submission for in-range digits, digit-zero → Cancel submit, out-of-range digits ignored, and the `defaultIdx` seeding cases (`-1` → row 1, `0` → Cancel, `>=1` → that row). Use table-driven tests per the project's existing patterns in `errors_test.go` / `names_test.go`.
- [x] T013 [P] Add table-driven unit tests for `parseKey` in `src/internal/worktree/menu_test.go`: assert `\x1b[A` → `Up`, `\x1b[B` → `Down`, `\r`/`\n` → `Enter`, `\x03` → `Cancel`, `q` → `Cancel`, `j` → `Down`, `k` → `Up`, ASCII `0`–`9` → `Digit(n)`, `\x1bOP` (F1) → `Ignore`, Tab/Backspace → `Ignore`, and bare `\x1b` with no follow-up byte within the injected fake-clock 50ms window → `Cancel`.
- [x] T014 [P] Add (or retain) fallback-path tests in `src/internal/worktree/menu_test.go` that drive `ShowMenu` with a piped `os.Stdin` (e.g., via `os.Pipe()` swapped onto `os.Stdin`, or by injecting a reader test seam if cleaner) and assert the byte-identical numbered output, the `Choice [N]:` prompt, and both validation error messages. The detection rule must route piped stdin to the fallback path regardless of stdout state.

### Phase 4: Polish

<!-- Documentation, cleanup, performance. Only include if warranted by the change scope. -->

- [x] T015 Update the package doc-comment on `ShowMenu` in `src/internal/worktree/menu.go` to describe the new TTY-aware behavior, the key bindings table, and the Cancel-returns-`0` contract. Reference `docs/memory/wt-cli/menu-navigation-contract.md` (to be created during hydrate) for the full contract.
- [x] T016 Manual QA notes: append a short `Manual QA` block at the end of `plan.md` (under `## Notes`) listing the human-verified scenarios the pure tests cannot cover — interactive run of `wt open` (worktree picker), `wt delete` (worktree picker), `Esc` exit leaves cooked mode, `Ctrl-C` exit leaves cooked mode, panic-during-read leaves cooked mode (simulate by deleting the test seam). Used during review. (Manual QA block already present under `## Notes` from plan generation.)
- [x] T017 Add a panic-restore test in `src/internal/worktree/menu_test.go`. Extracted `runInteractiveMenuCore(w, stdin, prompt, options, defaultIdx, restoreFn)` in `menu.go` as the internal seam; the existing `runInteractiveMenu` wires real stdin/stdout plus a `term.Restore` closure. The new test `TestRunInteractiveMenuCore_PanicRestore` injects a `panickingReader` and a counter-based fake `restoreFn`, recovers the panic in the test body, and asserts (a) the panic propagates AND (b) `restoreFn` ran exactly once before unwind. Public signature of `ShowMenu` unchanged. Resolves Acceptance A-011 and A-028.
- [x] T018 Refactored `paintMenu` and `redrawMenu` in `src/internal/worktree/menu.go` to share `renderRows(w, prompt, options, st, linePrefix)`. `paintMenu` calls it with `linePrefix=""`; `redrawMenu` writes the cursor-up prelude then calls it with `linePrefix=ansiClearLine`. New test `TestPaintAndRedrawShareCore` strips redraw's prelude/clears and asserts byte-identical row content. No behavioral change; all existing menu tests pass unchanged.

## Execution Order

<!-- Summary of non-obvious dependencies between tasks. -->

- T001 (dependency) must complete before any task that imports `golang.org/x/term` (T006, T007).
- T003 blocks T004 (state machine needs the types) and T005 (parser emits `keyEvent`).
- T004 and T005 are independent given T003 and can run in parallel.
- T007 depends on T003, T004, T005, T006 (it composes all of them).
- T008 is a helper inside T007 — implement as part of T007 or immediately after.
- T009 depends on T007 (delegation target must exist).
- T010 and T011 are independent polish tasks on top of T007/T009.
- T012, T013, T014 are test tasks that depend on the corresponding implementation tasks but are independent of each other.
- T015, T016 are pure documentation — run last.

## Acceptance

<!-- Declarative acceptance criteria used by the review stage. -->

### Functional Completeness

<!-- Every requirement in spec.md has working implementation. -->

- [x] A-001 TTY detection: `ShowMenu` calls `term.IsTerminal` on both `os.Stdin` and `os.Stdout` exactly once, before any output, and branches to the interactive or fallback path accordingly.
- [x] A-002 Public signature preserved: `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)` is unchanged; all ~11 call sites in `src/cmd/wt/{create,delete,open}.go` compile without edits.
- [x] A-003 Default pre-highlight: the interactive renderer pre-highlights row `defaultIdx` when `>= 1`, the Cancel row when `defaultIdx == 0`, and row 1 when `defaultIdx == -1`. The existing green `(default)` marker is still rendered on the default row and is visually distinct from the moving highlight.
- [x] A-004 Arrow + Vim navigation: `↑`/`k` and `↓`/`j` move the highlight one row with wrap-around between the last option and the Cancel row in both directions.
- [x] A-005 Digit submit: digits `1`–`9` corresponding to a valid option index submit that option immediately (no Enter required); digit `0` submits Cancel; out-of-range digits are silently ignored.
- [x] A-006 Enter submit: pressing `Enter` (`\r` or `\n`) submits the currently highlighted row.
- [x] A-007 Cancel returns `(0, nil)`: pressing `Esc`, `Ctrl-C`, or `q` causes `ShowMenu` to return `(0, nil)` with no new error type introduced.
- [x] A-008 In-place redraw: each highlight change redraws the menu region in place using `\x1b[<N>A`, `\x1b[2K`, and `\r`; scrollback does not accumulate intermediate menu states.
- [x] A-009 Final-line on submit: on submit, the menu region is replaced with a single line `<prompt> <option-text>` (e.g., `Open in: cursor`).
- [x] A-010 Final-line on cancel: on Cancel, the menu region is replaced with a single line `<prompt> (cancelled)`.
- [x] A-011 Raw-mode restore: `defer term.Restore` is wired in `runInteractiveMenu`, and the deferred `restoreFn` in `runInteractiveMenuCore` is now verified by `TestRunInteractiveMenuCore_PanicRestore` — the test panics inside the read loop and asserts the restore callback runs exactly once before the panic unwinds. Resolved by T017 panic-restore test seam.
- [x] A-012 Bell suppression: unknown keys (Tab, Backspace, function keys, Page Up/Down, `\x1bOP`) produce no `\x07`, no error message, and no redraw.
- [x] A-013 Fallback byte-identical: when either stdin or stdout is non-TTY, `ShowMenu` produces byte-for-byte the existing numbered list, `Choice [N]:` / `Choice:` prompts, and both validation error messages (`Invalid choice. Please enter a number.` and `Invalid choice. Please enter a number between 0 and N.`).
- [x] A-014 Pure state machine extracted: a pure function `nextMenuState(prev, key) → transition` exists in `src/internal/worktree/menu.go` and is exercised by unit tests in `menu_test.go` without opening a real terminal.
- [x] A-015 Pure escape parser extracted: the escape-sequence parser is a pure function on a byte slice (or buffered reader interface) and is exercised by unit tests covering arrow sequences, bare-Esc timeout (via injected fake clock), and unknown sequences.
- [x] A-016 Single new dependency: `src/go.mod` adds exactly one new direct dependency, `golang.org/x/term`, and no entries from `charmbracelet`, `AlecAivazis`, or `manifoldco` are present. `go mod tidy` leaves the file clean.
- [x] A-017 Windows behavior is conservative and consistent: either the interactive renderer is wired on Windows OR `isInteractiveTTY()` returns `false` on `runtime.GOOS == "windows"` — the choice is recorded in a code comment for hydrate to capture in the memory contract; behavior is deterministic (not flaky).

### Scenario Coverage

<!-- Key scenarios from spec.md have been exercised. -->

- [x] A-018 **N/A**: Manual QA item per spec — covered by `## Notes > Manual QA` checklist (item 1); not subject to automated review.
- [x] A-019 Scenario "Piped stdin": `cmd.Stdin = strings.NewReader("2\n")` routes to the fallback path with byte-identical output — verified by a unit test in `menu_test.go`.
- [x] A-020 Scenario "Redirected stdout": stdout to a file or pipe routes to the fallback path and raw mode is NOT enabled — verified by a unit test in `menu_test.go`.
- [x] A-021 Scenario "Default highlight on first paint": `ShowMenu("Open in:", []string{"cursor","code","open_here"}, 2)` pre-highlights row 2 with both reverse video and the `(default)` marker on first paint — covered by a `nextMenuState` seeding test.
- [x] A-022 Scenario "Wrap-around from last option": `↓` from row N moves highlight to Cancel — covered by a `nextMenuState` unit test.
- [x] A-023 Scenario "Wrap-around from Cancel": `↓` from Cancel moves highlight to row 1 — covered by a `nextMenuState` unit test.
- [x] A-024 Scenario "Digit out of range is ignored": pressing `7` on a 3-option menu leaves highlight unchanged and produces no submission — covered by a `nextMenuState` unit test.
- [x] A-025 Scenario "Bare Esc parses to Cancel after timeout": injected fake clock advancing 60ms produces `Cancel` from a lone `\x1b` byte — covered by a `parseKey` unit test.
- [x] A-026 Scenario "Unknown sequence parses to Ignore": `\x1bOP` (F1) produces `Ignore` — covered by a `parseKey` unit test.
- [x] A-027 Scenario "Invalid input message preserved": fallback path on input `xyz` emits the exact message `Invalid choice. Please enter a number.` — covered by a fallback-path unit test.

### Edge Cases & Error Handling

<!-- Error states, boundary conditions, failure modes. -->

- [x] A-028 Panic in read loop: `TestRunInteractiveMenuCore_PanicRestore` in `menu_test.go` injects a `panickingReader` whose `Read` panics on first call, recovers the panic, and asserts both that it propagates out of `runInteractiveMenuCore` AND that the deferred `restoreFn` was invoked exactly once before unwind. Resolved by T017 panic-restore test seam.
- [x] A-029 **N/A**: Manual QA item per spec — covered by `## Notes > Manual QA` checklist (item 4); not subject to automated review.
- [x] A-030 Bare-Esc-vs-arrow disambiguation: a lone `\x1b` followed by a valid arrow suffix within 50ms parses to the arrow event; without a follow-up within 50ms it parses to Cancel — covered by `parseKey` tests.
- [x] A-031 Empty options slice: `ShowMenu` does not panic when called with an empty options slice in either path; behavior on the interactive path is graceful (highlight pinned to Cancel) — covered by a unit test on `nextMenuState`.

### Code Quality

<!-- Per `fab/project/code-quality.md` principles, anti-patterns, and baseline items. -->

- [x] A-032 Pattern consistency: new code in `menu.go` and `menu_test.go` follows naming and structural patterns of surrounding files (`errors.go`, `names.go`, `apps.go`) — internal types unexported, table-driven tests, lowercase error strings.
- [x] A-033 No unnecessary duplication: the fallback-path body reuses the existing numbered-prompt logic verbatim rather than re-implementing input parsing or validation messages.
- [x] A-034 Readability over cleverness: the state machine and parser are small, named, and individually testable; no inline lambdas or nested switches longer than the surrounding code's typical function size.
- [x] A-035 No god functions: `runInteractiveMenu` stays under ~50 lines or is decomposed into helpers (`initialPaint`, `redraw`, `finalize`); `nextMenuState` and `parseKey` are each well under 50 lines.
- [x] A-036 No magic constants: ANSI sequences (`\x1b[A`, `\x1b[B`, `\x1b[2K`, `\x1b[<N>A`), the 50ms Esc-timeout, and key bytes are extracted as named constants (e.g., `escUp = "\x1b[A"`, `escTimeoutMs = 50`) rather than scattered string literals.
- [x] A-037 Constitution Principle I: only `golang.org/x/term` is added; no third-party TUI dependency (`bubbletea`, `survey/v2`, `promptui`, etc.) appears in `src/go.mod` or `src/go.sum`.
- [x] A-038 Constitution Principle IV: every new exported/internal function has a corresponding unit test; PTY-based testing is avoided in favor of the pure-function seam.
- [x] A-039 Constitution Principle VI: behavior degrades gracefully without `--non-interactive` when stdout is not a TTY (verified by the redirected-stdout scenario test).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

### Manual QA (T016)

Run after apply, before review marks the corresponding Manual QA acceptance items:

1. `wt open` in an interactive terminal — pick a worktree via arrow keys, confirm in-place redraw and clean post-prompt line.
2. `wt delete` in an interactive terminal — pick a worktree via `j`/`k`, confirm wrap-around.
3. Press `Esc` mid-menu — confirm the terminal returns to cooked mode (run `stty -a` after, or simply `echo hi`).
4. Press `Ctrl-C` mid-menu — confirm `ShowMenu` returns `0, nil` and the host process keeps running (raw mode swallows the SIGINT byte).
5. (Optional) Simulate a panic via a test seam and confirm `term.Restore` fires before unwind.

## Deletion Candidates

- None — this change adds new functionality (interactive arrow-key path) on top of the existing fallback path, which is preserved verbatim. No existing functions, branches, or files are made redundant. `runFallbackMenu` extracts the historical body in place without obsoleting any caller-facing surface.
