# Spec: Arrow-Key Menu Navigation

**Change**: 260516-dkg7-arrow-key-menu-navigation
**Created**: 2026-05-17
**Affected memory**: `docs/memory/wt-cli/menu-navigation-contract.md`

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope limited to `ShowMenu`; `ConfirmYesNo` and `PromptWithDefault` unchanged | Confirmed from intake #1. Free-form prompts have no option list to navigate. | S:95 R:85 A:95 D:95 |
| 2 | Certain | Non-TTY paths fall back to existing numbered prompt with byte-identical output | Confirmed from intake #2. Constitution Principle VI mandates graceful TTY-less degradation; existing tests pin the fallback output. | S:90 R:80 A:95 D:95 |
| 3 | Certain | `ShowMenu` public signature preserved; no call-site edits required | Confirmed from intake #3. All 11 call sites pass `(prompt, options, defaultIdx)` and use the integer return. | S:90 R:75 A:90 D:90 |
| 4 | Certain | `defaultIdx` row pre-highlighted on first paint; `(default)` marker preserved | Confirmed from intake #4. Natural mapping of existing default semantics into the arrow-key UX. | S:90 R:80 A:90 D:90 |
| 5 | Certain | Number-key shortcuts (`1`–`9`) submit immediately in interactive mode | Confirmed from intake #7 (clarified — user confirmed). Preserves muscle memory; matches current `Choice [N]:` submit-on-Enter semantics for digit input. | S:95 R:60 A:90 D:90 |
| 6 | Certain | Library: hand-rolled using `golang.org/x/term`; no third-party TUI deps | Confirmed from intake #8 (clarified — user chose hand-rolled over survey/v2 and bubbletea). Aligns with Principle I. | S:95 R:55 A:90 D:90 |
| 7 | Certain | Cancel (Esc/Ctrl-C/`q`) returns `(0, nil)`; no `ErrUserAbort` type introduced | Upgraded from intake #9 Confident → Certain. Preserves all 11 call sites' `if choice == 0` Cancel handling without edit. | S:90 R:75 A:95 D:95 |
| 8 | Certain | In-place redraw via cursor-up + `\x1b[2K`; no alternate screen buffer | Confirmed from intake #5. Standard pattern used by `gum choose`, `gh`, and virtually every modern arrow-key picker for a per-prompt widget. | S:85 R:70 A:85 D:85 |
| 9 | Certain | `defer term.Restore(...)` guards every exit path (submit, Cancel, panic) | Confirmed from intake #6. Required by Go raw-terminal idioms; leaving the terminal in raw mode is the canonical "this CLI is broken" footgun. | S:90 R:55 A:95 D:95 |
| 10 | Certain | Pure `nextMenuState` function extracted for unit tests; raw I/O is a thin shell | Discovered during spec generation. Mirrors constitution Principle IV ("Test what the user sees") combined with avoiding PTY-test flakiness. | S:85 R:75 A:90 D:90 |
| 11 | Confident | Wrap-around at list edges (last row `↓` → Cancel; Cancel `↓` → first row) | Standard behavior in `fzf`, `gum`, `gh`, and modern pickers. Hard stops feel broken on short lists like worktree pickers. | S:75 R:80 A:85 D:80 |
| 12 | Confident | Vim-style `j`/`k` aliases for `↓`/`↑` | Costs ~4 lines of code, broadly expected by terminal-power-user audience. Low risk; easy to remove if disliked. | S:65 R:90 A:80 D:80 |
| 13 | Confident | Bare Esc disambiguated from arrow prefix via 50ms read window | Standard approach for raw-mode escape parsing (also used by readline). 50ms is below human perception threshold but well above typical arrow-key burst timing (sub-ms). | S:75 R:70 A:80 D:75 |
| 14 | Confident | Bell on unknown keys suppressed | Bell on every typo would be obnoxious in a picker; consistent with `fzf`, `gum`, etc. | S:75 R:90 A:85 D:90 |
| 15 | Confident | Post-prompt final line shows `<prompt> <option-text>` or `<prompt> (cancelled)` | Keeps output stream consistent for scrollback readers and `wt`'s convention that prompts leave a trace of the decision. | S:70 R:80 A:80 D:75 |
| 16 | Confident | Windows interactive renderer is best-effort; fallback path is conservative | ConPTY raw-mode quirks (line-buffering on certain terminals, key-code differences) make full Windows arrow-key support a non-trivial side quest. Linux/macOS users gain the UX immediately; Windows users keep today's UX and are no worse off. Apply phase decides which path Windows takes. | S:55 R:60 A:75 D:60 |
16 assumptions (10 certain, 6 confident, 0 tentative, 0 unresolved).
<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
