# Plan: Arrow-Key Menu Navigation

**Change**: 260516-dkg7-arrow-key-menu-navigation
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- **`ConfirmYesNo` and `PromptWithDefault` redesign** — those are free-form input prompts with no notion of a selectable option list; arrow keys are meaningless there.
- **External TUI dependencies (`bubbletea`, `survey/v2`, `promptui`)** — explicitly rejected to keep Principle I's single-binary commitment intact.
- **Multi-select menus** — every `ShowMenu` call site today selects exactly one option; multi-select is not a current need.
- **Mouse interaction** — keyboard-only.
- **Configurable keybindings** — fixed bindings (arrows + `j`/`k` + digits + Enter + Esc/Ctrl-C/`q`); no env vars, no flags.
- **Pagination / scrolling for very long lists** — call sites today produce ≤ ~30 options (worktree count). Out of scope to handle 100+-row pickers.

## Menu Subsystem: TTY Detection and Path Selection

### Requirement: ShowMenu detects whether both stdin and stdout are TTYs

`src/internal/worktree/menu.go` `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)` SHALL detect at entry whether **both** `os.Stdin` and `os.Stdout` are connected to a TTY (via `golang.org/x/term`'s `term.IsTerminal(int(fd.Fd()))`). The detection result SHALL drive a single branch:

- **Both TTY** → interactive arrow-key path.
- **Either non-TTY** → fallback numbered-prompt path (current behavior, byte-identical).

The detection MUST occur exactly once per invocation, before any output is emitted, so the rendered prompt format is consistent end-to-end within a single call.

#### Scenario: Interactive terminal session
- **GIVEN** `wt open` invoked from a terminal with both stdin and stdout attached to a TTY
- **WHEN** the worktree-selection menu opens
- **THEN** the interactive arrow-key renderer is used
- **AND** raw mode is enabled on the controlling terminal before the first keystroke is read

#### Scenario: Piped stdin (e.g., test harness)
- **GIVEN** `wt open` invoked with `cmd.Stdin = strings.NewReader("2\n")` from an integration test
- **WHEN** the worktree-selection menu opens
- **THEN** the fallback numbered-prompt path is used
- **AND** the output is byte-identical to today's output (numbered list + `Choice [N]:` prompt)

#### Scenario: Redirected stdout (e.g., logging)
- **GIVEN** `wt open` invoked with stdout redirected to a file or pipe
- **WHEN** the menu opens
- **THEN** the fallback numbered-prompt path is used
- **AND** raw mode is NOT enabled on the terminal

### Requirement: ShowMenu's public signature is preserved

The exported signature `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)` SHALL NOT change. The return value semantics SHALL be preserved: `0` means Cancel, `1..N` means the corresponding option. All ~11 existing call sites in `src/cmd/wt/{create,delete,open}.go` SHALL compile and behave correctly without edits.

#### Scenario: Caller binary-compatibility
- **GIVEN** existing call sites passing `(prompt, options, defaultIdx)`
- **WHEN** the change is merged
- **THEN** no caller source files under `src/cmd/wt/` require edits to consume the new behavior
- **AND** every caller's existing `if choice == 0 { return }` Cancel-handling branch continues to function

## Menu Subsystem: Interactive Path Behavior

### Requirement: Interactive renderer pre-selects the default option

When the interactive path is active and `defaultIdx >= 0`, the renderer SHALL pre-highlight that row on first paint. `defaultIdx == 0` SHALL pre-highlight the `Cancel` row. `defaultIdx == -1` (no default) SHALL pre-highlight the first option (index `1`). The existing `(default)` green marker SHALL still be rendered on the default row, distinct from the moving highlight.

#### Scenario: Default highlight on first paint
- **GIVEN** `ShowMenu("Open in:", []string{"cursor", "code", "open_here"}, 2)`
- **WHEN** the menu first renders
- **THEN** row 2 (`code`) is shown with reverse-video highlight and the `(default)` green marker
- **AND** rows 1 and 3 are rendered plain

#### Scenario: No default
- **GIVEN** `ShowMenu("Select worktree to open:", names, -1)`
- **WHEN** the menu first renders
- **THEN** row 1 is pre-highlighted
- **AND** no row carries the `(default)` marker

### Requirement: Arrow keys, `j`/`k`, and digit keys all navigate / select

In the interactive path, the renderer SHALL accept the following key bindings:

| Key | Action |
|-----|--------|
| `↑` (escape sequence `\x1b[A`), `k` | Move highlight up one row. Wraps from `Cancel` (top → bottom: `Cancel` row to the first option, since `Cancel` is rendered last). Wraps from the first option to `Cancel`. |
| `↓` (escape sequence `\x1b[B`), `j` | Move highlight down one row. Wraps from `Cancel` back to option 1. |
| Digit `1`–`9` | If the digit corresponds to a valid option index (`1..len(options)`), highlight and **immediately submit** that option. If the digit is `> len(options)`, the keystroke is ignored. Digit `0` immediately submits the `Cancel` row. |
| `Enter` (`\r` or `\n`) | Submit the currently highlighted row. |
| `Esc` (`\x1b` not followed by `[`), `Ctrl-C` (`\x03`), `q` | Cancel — return `0`, `nil`. |
| Any other key | Ignored (no redraw, no submit). |

Multi-byte escape sequences SHALL be read using a 50ms post-`\x1b` read window to distinguish a bare `Esc` from the start of `\x1b[A`/`\x1b[B`. A bare `Esc` followed by no follow-up within the window SHALL be treated as Cancel.

#### Scenario: Arrow-down moves highlight
- **GIVEN** the menu is showing options `[a, b, c]` with row 1 highlighted
- **WHEN** the user presses `↓`
- **THEN** row 2 (`b`) becomes highlighted
- **AND** the menu region is redrawn in place (no scrollback growth)

#### Scenario: Wrap-around from last option
- **GIVEN** the menu is showing options `[a, b, c]` with row 3 highlighted
- **WHEN** the user presses `↓`
- **THEN** the `Cancel` row becomes highlighted

#### Scenario: Wrap-around from Cancel
- **GIVEN** the menu is showing options `[a, b, c]` with `Cancel` highlighted
- **WHEN** the user presses `↓`
- **THEN** row 1 (`a`) becomes highlighted

#### Scenario: Digit submits immediately
- **GIVEN** the menu is showing options `[a, b, c, d, e]` with row 1 highlighted
- **WHEN** the user presses `3`
- **THEN** the menu submits choice `3` (option `c`)
- **AND** no `Enter` keypress is required

#### Scenario: Digit out of range is ignored
- **GIVEN** the menu is showing options `[a, b, c]` (3 options) with row 1 highlighted
- **WHEN** the user presses `7`
- **THEN** the highlight does not move
- **AND** no submission occurs
- **AND** no warning is rendered

#### Scenario: Vim-style navigation
- **GIVEN** the menu is showing options `[a, b, c]` with row 1 highlighted
- **WHEN** the user presses `j`
- **THEN** row 2 becomes highlighted (equivalent to `↓`)

#### Scenario: Enter submits highlighted row
- **GIVEN** the menu is showing options `[a, b, c]` with row 2 highlighted
- **WHEN** the user presses `Enter`
- **THEN** the menu submits choice `2` (option `b`)

### Requirement: Cancel returns `0, nil`

Pressing `Esc`, `Ctrl-C`, or `q` in the interactive path SHALL cause `ShowMenu` to return `(0, nil)` — identical to the existing fallback path's "user typed `0`" outcome. The interactive path SHALL NOT introduce a new error type (e.g., `ErrUserAbort`) for Cancel; callers retain their `if choice == 0 { ... }` branching.

#### Scenario: Esc cancels
- **GIVEN** the menu is open
- **WHEN** the user presses `Esc` (and 50ms elapse without a follow-up byte)
- **THEN** `ShowMenu` returns `0, nil`
- **AND** the menu region is cleared

#### Scenario: Ctrl-C cancels
- **GIVEN** the menu is open
- **WHEN** the user presses `Ctrl-C`
- **THEN** `ShowMenu` returns `0, nil`
- **AND** the terminal is restored to cooked mode before the return
- **AND** no SIGINT propagates to the host process (raw mode swallows it as a `\x03` byte)

### Requirement: In-place redraw keeps scrollback clean

The interactive renderer SHALL redraw the menu region (prompt + option rows + `Cancel` row) in place on each highlight change, using `\x1b[<N>A` (cursor up N), `\x1b[2K` (clear line), and `\r` sequences. Scrollback SHALL NOT accumulate intermediate menu states.

On exit (submit or Cancel), the menu region SHALL be replaced with a single line of post-prompt content:

- On submit: `<prompt> <option-text>` (e.g., `Open in: cursor`).
- On Cancel: `<prompt> (cancelled)`.

#### Scenario: Highlight change does not grow scrollback
- **GIVEN** a menu with 5 options is open
- **WHEN** the user presses `↓` three times then `↑` twice
- **THEN** the terminal cursor returns to its post-menu position on submit
- **AND** scrollback contains a single rendered menu region — not 6 stacked copies

#### Scenario: Final line after submit
- **GIVEN** the user submits option `cursor` from prompt `Open in:`
- **WHEN** `ShowMenu` returns
- **THEN** the terminal shows `Open in: cursor` as the last line emitted by the menu
- **AND** no orphaned highlight marker remains

#### Scenario: Final line after cancel
- **GIVEN** the user cancels via `Esc` from prompt `Select worktree to open:`
- **WHEN** `ShowMenu` returns `0, nil`
- **THEN** the terminal shows `Select worktree to open: (cancelled)` as the last line

### Requirement: Raw mode is restored on every exit path

Raw mode SHALL be enabled via `term.MakeRaw(fd)` immediately before the first key read and SHALL be restored via `term.Restore(fd, oldState)` on every return path — including normal submit, Cancel, `Ctrl-C`, panic recovery, and SIGINT delivered between key reads. The restore SHALL be wired through a `defer` so a panic in the read loop does not leave the user's terminal in raw mode.

#### Scenario: Normal submit restores cooked mode
- **GIVEN** the menu is in raw mode
- **WHEN** the user submits a choice
- **THEN** `term.Restore` is called before `ShowMenu` returns
- **AND** the next `fmt.Println` in the caller renders normally (newlines visible, no doubled output)

#### Scenario: Panic in read loop restores cooked mode
- **GIVEN** the menu is in raw mode and a read call panics (simulated via test seam)
- **WHEN** the panic unwinds
- **THEN** the `defer term.Restore(...)` fires before the panic propagates
- **AND** the test process's terminal is left in cooked mode

### Requirement: Bell on unknown keys is suppressed

Unknown keys (including `Tab`, `Backspace`, function keys, `Page Up/Down`, etc.) SHALL be silently ignored — no audible bell (`\x07`), no error message, no redraw.

#### Scenario: Function key ignored
- **GIVEN** the menu is open with row 1 highlighted
- **WHEN** the user presses `F1` (escape sequence `\x1bOP`)
- **THEN** the highlight stays on row 1
- **AND** no bell sound is emitted
- **AND** no terminal output is produced

## Menu Subsystem: Fallback Path Stability

### Requirement: Non-TTY fallback path is byte-identical to today's output

When the fallback (non-TTY) path is taken, the rendered prompt, the numbered option list, the `Cancel` line, the `(default)` markers, the `Choice [N]:` / `Choice:` prompt, and the validation error messages (`Invalid choice. Please enter a number.`, `Invalid choice. Please enter a number between 0 and N.`) SHALL be byte-identical to today's behavior. This invariant is enforced by retaining existing unit tests in `src/internal/worktree/menu_test.go` (or its new equivalent) that drive `ShowMenu` via a piped stdin.

#### Scenario: Test harness expects existing output
- **GIVEN** an existing test that pipes `"1\n"` to `ShowMenu` and asserts on captured stdout
- **WHEN** the code change is merged
- **THEN** the test passes without modification

#### Scenario: Invalid input message preserved
- **GIVEN** the fallback path is active and the user enters `xyz`
- **WHEN** `ShowMenu` reads the input
- **THEN** the rendered error is `Invalid choice. Please enter a number.`
- **AND** the prompt is re-emitted for re-entry

## Menu Subsystem: Testability

### Requirement: Key-handling logic is unit-testable without a real PTY

The arrow-key state machine (current highlight + incoming key → new highlight or submission) SHALL be extracted as a pure function with a signature equivalent to:

```go
func nextMenuState(prev menuState, key keyEvent) menuStateTransition
```

where `keyEvent` is a small sum-type (Up, Down, Digit(n), Enter, Cancel, Ignore) and `menuStateTransition` carries the new highlight index and a "submitted" flag. Pure unit tests SHALL exercise every key mapping, every wrap-around, every digit boundary, and the Cancel transitions without ever opening a terminal.

The raw-mode I/O (PTY read, escape parsing, redraw) SHALL remain a thin shell around this pure function. The shell layer MAY be manually QA'd; the contract layer is testable end-to-end via the pure function.

#### Scenario: Wrap-around tested without PTY
- **GIVEN** `prev = { highlight: lastOption }` and `key = Down`
- **WHEN** `nextMenuState` is invoked
- **THEN** the returned transition's new highlight is the `Cancel` row index
- **AND** `submitted` is `false`

#### Scenario: Digit submit tested without PTY
- **GIVEN** `prev = { highlight: 1, numOptions: 5 }` and `key = Digit(3)`
- **WHEN** `nextMenuState` is invoked
- **THEN** the returned transition's new highlight is row 3
- **AND** `submitted` is `true`

#### Scenario: Out-of-range digit tested without PTY
- **GIVEN** `prev = { highlight: 1, numOptions: 3 }` and `key = Digit(7)`
- **WHEN** `nextMenuState` is invoked
- **THEN** the returned transition's new highlight is row 1 (unchanged)
- **AND** `submitted` is `false`

### Requirement: Escape parser is unit-testable

The escape-sequence parser that converts raw bytes (`\x1b[A`, `\x1b[B`, `\x1b`, `\r`, `\n`, `\x03`, ASCII digits, `j`, `k`, `q`, other) into `keyEvent` SHALL also be a pure function operating on a byte slice (or a small buffered reader interface). Tests SHALL exercise the bare-`Esc`-vs-arrow-prefix ambiguity, the timeout fallback (using a fake clock or a dependency-injected timeout source), and the handling of unknown sequences.

#### Scenario: Arrow sequence parses to Down
- **GIVEN** input bytes `[0x1b, 0x5b, 0x42]` (`\x1b[B`)
- **WHEN** the parser is invoked
- **THEN** the returned event is `Down`

#### Scenario: Bare Esc parses to Cancel after timeout
- **GIVEN** input byte `0x1b` and no follow-up byte within 50ms
- **WHEN** the parser is invoked with a fake clock advancing 60ms
- **THEN** the returned event is `Cancel`

#### Scenario: Unknown sequence parses to Ignore
- **GIVEN** input bytes `[0x1b, 0x4f, 0x50]` (`\x1bOP`, F1 key)
- **WHEN** the parser is invoked
- **THEN** the returned event is `Ignore`

## Build & Dependencies

### Requirement: `golang.org/x/term` is the only new dependency

`src/go.mod` SHALL gain exactly one new direct dependency: `golang.org/x/term`. No transitive bloat beyond what `x/term` itself requires (which is `golang.org/x/sys` — already an `x/` package maintained by the Go team). No new dependency on `bubbletea`, `survey/v2`, `promptui`, or any third-party prompt/TUI library SHALL be introduced.

#### Scenario: go.mod inspection
- **GIVEN** the merged change
- **WHEN** `cat src/go.mod` is run
- **THEN** the `require` block contains `golang.org/x/term v0.X.X`
- **AND** no `charmbracelet`, `AlecAivazis`, or `manifoldco` entries are present

## Cross-Platform

### Requirement: Windows behavior is conservative

On Windows (`GOOS=windows`), `ShowMenu` SHALL detect TTY status via the same `golang.org/x/term` API, but the implementation MAY fall back to the numbered-prompt path unconditionally if raw-mode handling on Windows ConPTY would complicate this change. The choice between (a) full Windows raw-mode support and (b) Windows-falls-back-to-numbered SHALL be made during apply, recorded in the memory contract, and SHALL NOT block the change on Linux/macOS.

#### Scenario: Windows interactive (option a)
- **GIVEN** a Windows ConPTY session with both streams as TTY
- **WHEN** `wt open` opens the worktree menu
- **THEN** the interactive renderer is used (if raw-mode support was wired) — OR — the numbered-prompt fallback is used (if Windows was scoped out)
- **AND** the behavior is consistent (one or the other, not flaky)

## Design Decisions

1. **Hand-rolled raw-mode using `golang.org/x/term`** (chosen over `survey/v2` and `bubbletea`).
   - *Why*: Constitution Principle I commits to a slim single-binary CLI. `x/term` is an `x/` package (effectively stdlib-adjacent); the entire renderer is 150–250 LOC. Full control over fallback semantics, default-marker preservation, in-place redraw, and Cancel→`0` mapping.
   - *Rejected*: `survey/v2` (pulls ~5 indirect deps and removes our control over the rendering surface). `bubbletea` (overkill — pulls in `lipgloss`, `termenv`, `harmonica` for one highlight-row widget). `promptui` (less actively maintained, similar tradeoffs to survey).

2. **Cancel returns `0, nil` (not a typed error)** (chosen over introducing `ErrUserAbort`).
   - *Why*: Preserves the existing caller contract across all ~11 `ShowMenu` call sites. Adding an error type forces every caller to add a branch with zero UX gain (callers already handle `choice == 0`).
   - *Rejected*: A typed `ErrUserAbort` — would force edits to every call site with no observable behavior change.

3. **Digit keys submit immediately** (chosen over digit-highlights-only-then-Enter).
   - *Why*: Matches today's `Choice [N]:` UX where typing `3<Enter>` selects option 3. Removing the `Enter` step for digit input preserves muscle memory and arrow-key navigation is itself the slower-but-more-discoverable path.
   - *Rejected*: Digit-highlights-only — adds a step for users who already know what they want.

4. **In-place redraw via cursor-up + clear-line** (chosen over full-screen alternate buffer).
   - *Why*: Alternate-buffer mode (`\x1b[?1049h`) is jarring for a non-fullscreen widget; cursor-up + `\x1b[2K` redraws only the menu region and leaves prior shell output in scrollback. This is the same approach used by `gum choose` and `gh`'s interactive selectors.
   - *Rejected*: Alternate buffer — wrong scope for a per-prompt widget.

5. **Wrap-around at edges** (chosen over hard stops).
   - *Why*: Standard behavior in `fzf`, `gum`, `gh`, and virtually every modern picker. Users navigating to the bottom of a list expect `↓` from the last row to land on `Cancel` (or wrap to the top). Hard stops feel broken on small lists.
   - *Rejected*: Hard-stop at first/last row.

6. **Pure `nextMenuState` function for testing** (chosen over PTY-based test infrastructure).
   - *Why*: Real-PTY tests are slow, flaky, and OS-specific. Extracting the state machine as a pure function tests every keybinding deterministically in ~µs and works identically on Linux, macOS, and Windows CI.
   - *Rejected*: PTY-based tests (e.g. via `github.com/creack/pty`) — adds a dependency just for tests, and the raw I/O shell layer is too thin to be worth integration-testing.


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
