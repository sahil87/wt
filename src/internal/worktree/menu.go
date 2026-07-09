package worktree

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ShowMenu displays a numbered menu with a Cancel option (0) and returns the
// selected index. The semantics are:
//
//   - Return value 0 means the user cancelled.
//   - Return values 1..len(options) correspond to the selected option.
//   - defaultIdx pre-selects a row: a value of -1 means "no default", 0 means
//     "default is Cancel", and 1..len(options) pre-selects that option.
//
// Two rendering paths exist, selected once per invocation before any output:
//
//  1. Interactive arrow-key path — used when BOTH stdin and stdout are TTYs.
//     Supports ↑/↓, j/k, digits 1..9, Enter, Esc, Ctrl-C, q. Cancel returns
//     (0, nil) — no new error type is introduced.
//
//  2. Fallback numbered-prompt path — used when either stream is not a TTY
//     (piped stdin, redirected stdout, integration tests). Byte-for-byte
//     identical to the historical behavior; existing tests pin this output.
//
// Key bindings (interactive path):
//
//	↑ / k        : highlight previous row (wraps from first option to Cancel)
//	↓ / j        : highlight next row (wraps from Cancel back to first option)
//	1..9         : highlight + immediately submit that option (out-of-range ignored)
//	0            : immediately submit Cancel
//	Enter        : submit the highlighted row
//	Esc          : cancel (returns 0, nil); 50ms timeout disambiguates bare Esc
//	               from the start of an escape sequence like \x1b[A
//	Ctrl-C / q   : cancel (returns 0, nil); raw mode swallows Ctrl-C as \x03,
//	               so no SIGINT propagates to the host process
//	other        : silently ignored (no bell, no redraw)
//
// On Windows (runtime.GOOS == "windows") the interactive renderer is currently
// scoped out — TTY detection short-circuits to false and the fallback path is
// taken. ConPTY raw-mode quirks (line-buffering on certain terminals, key-code
// differences) make full Windows arrow-key support a non-trivial side quest
// that would not block Linux/macOS users. See spec Cross-Platform requirement.
//
// See docs/memory/wt-cli/menu-navigation-contract.md (created during hydrate)
// for the full contract.
func ShowMenu(prompt string, options []string, defaultIdx int) (int, error) {
	s := NewMenuSession()
	defer s.Close()
	return s.Show(prompt, options, defaultIdx)
}

// MenuSession owns the single stdin reader shared across one or more menus
// shown back to back — most importantly the multi-menu flows of `wt open`
// (worktree selection → "Open in:") and `wt delete` (selection →
// uncommitted-changes → unpushed-commits → confirm).
//
// Why this exists: each interactive menu reads stdin through a pump goroutine
// that reads one byte ahead. If every menu built its own reader over os.Stdin,
// the first menu's pump would be left parked inside a blocking read on the
// shared fd after it submitted — an orphan that then steals the next menu's
// first keystroke (see TestUnderlyingReadAhead_DemonstratesTheft and the
// TestMenuSession_SharesReaderAcrossMenus regression guard). Sharing one
// reader across Show() calls means there is never more than one reader on the
// stream, so no keystroke is stolen between menus.
//
// Raw mode is entered and restored *per Show() call*, not held across the
// whole session. This is deliberate: the delete flow exits the process
// (os.Exit) between menus on cancel paths, and a session that held raw mode
// across menus would leave the terminal in raw mode on those exits. Restoring
// after each menu means the terminal is always in cooked mode between menus,
// so process exit is safe with no special-casing. The byte-theft fix only
// requires a shared *reader*; it does not require continuously-held raw mode.
//
// Single-menu callers do not need to touch this type: ShowMenu wraps a
// one-shot session for them.
type MenuSession struct {
	interactive bool
	reader      *blockingByteReader // shared stdin reader (nil in fallback mode)
}

// NewMenuSession detects whether the terminal supports the interactive
// arrow-key path and, if so, creates the single shared stdin reader reused by
// every Show() call. If the terminal is not an interactive TTY, the session
// degrades to the fallback numbered-prompt path (each Show call reads a line
// from os.Stdin as before).
//
// Callers SHOULD Close the session when the interaction ends. Close is a no-op
// today (raw mode is managed per Show), but calling it keeps the lifecycle
// explicit and future-proofs the type against holding OS resources.
func NewMenuSession() *MenuSession {
	s := &MenuSession{}
	if !isInteractiveTTY() {
		return s // fallback mode
	}
	s.interactive = true
	s.reader = newBlockingByteReader(os.Stdin)
	return s
}

// Show renders one menu against the session's shared reader and returns the
// selected index (0 = Cancel, 1..len(options) = option). Raw mode is entered
// and restored within this call.
func (s *MenuSession) Show(prompt string, options []string, defaultIdx int) (int, error) {
	if !s.interactive {
		return runFallbackMenu(prompt, options, defaultIdx)
	}

	stdinFd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		// Cannot enter raw mode — degrade to the fallback prompt rather than
		// dying. Same return-value contract either way.
		return runFallbackMenu(prompt, options, defaultIdx)
	}
	// The restore is the safety net for every exit path of the read loop —
	// submit, Cancel, error, even a panic — so cooked mode is reinstated
	// before this call returns. runInteractiveMenuCore defers it.
	restoreFn := func() {
		// Ignore the Restore error: nothing useful can be done at process
		// scope, and the terminal is either restored or the process is dying.
		_ = term.Restore(stdinFd, oldState)
	}
	return s.showInteractive(os.Stdout, prompt, options, defaultIdx, restoreFn)
}

// showInteractive runs one interactive menu against the session's shared
// reader and the given writer. It is the seam below the raw-mode setup in
// Show, isolated so reader-sharing across menus can be tested without a real
// TTY (which term.MakeRaw requires). restoreFn is the raw-mode restore (a
// no-op in tests).
func (s *MenuSession) showInteractive(w io.Writer, prompt string, options []string, defaultIdx int, restoreFn func()) (int, error) {
	return runInteractiveMenuCore(w, s.reader, prompt, options, defaultIdx, restoreFn, terminalHeight)
}

// Close ends the session. It is currently a no-op (raw mode is managed per
// Show), kept so callers can express the session lifecycle with a deferred
// Close and so the contract survives future changes that acquire resources.
func (s *MenuSession) Close() {}

// PromptWithDefault prompts for a line of input with a default value, reading
// through the session's SHARED stdin reader. This is the line-prompt analogue
// of Show: a session that mixes menus and line prompts (e.g. `wt create`'s
// dirty-state menu → "Worktree name" prompt) must route every stdin consumer
// through the one shared reader, or a preceding menu's parked read-ahead pump
// steals the prompt's first typed line (the menu→line-prompt byte-theft seam —
// see the MenuSession doc comment and TestUnderlyingReadAhead_DemonstratesTheft).
//
// In fallback mode (non-TTY / Windows) it delegates to the package-level
// PromptWithDefault so piped-stdin behavior is byte-for-byte identical to the
// standalone function. (A raw-mode-entry failure does NOT flip the session to
// fallback — that degrades only the affected Show call; s.interactive is
// decided once at NewMenuSession from TTY detection.)
//
// No raw mode is entered — line prompts run in cooked mode exactly as the
// package-level function does (the kernel line-buffers and echoes). EOF/empty
// semantics match that function: EOF before a newline or an empty (or
// whitespace-only, after trimming) line returns defaultValue.
func (s *MenuSession) PromptWithDefault(prompt, defaultValue string) string {
	if !s.interactive {
		return PromptWithDefault(prompt, defaultValue)
	}
	fmt.Printf(promptWithDefaultFmt, prompt, defaultValue)
	line, ok := s.reader.readLine()
	if !ok {
		return defaultValue
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

// ConfirmYesNo prompts for a Y/n confirmation (default yes), reading through the
// session's SHARED stdin reader — the confirmation analogue of Show and the
// same byte-theft-avoiding contract as PromptWithDefault above.
//
// In fallback mode (non-TTY / Windows) it delegates to the package-level
// ConfirmYesNo so piped-stdin behavior is byte-for-byte identical. The prompt
// is written to stderr (human copy) and EOF/empty semantics match the
// package-level function: EOF before a newline returns false; an empty (or
// whitespace-only, after trimming) line returns true (the default). Answer
// parsing is shared with that function via parseYesNoLine: a line starting
// with "y"/"Y" is yes.
func (s *MenuSession) ConfirmYesNo(prompt string) bool {
	if !s.interactive {
		return ConfirmYesNo(prompt)
	}
	fmt.Fprintf(os.Stderr, confirmYesNoFmt, prompt)
	line, ok := s.reader.readLine()
	if !ok {
		return false
	}
	return parseYesNoLine(strings.TrimSpace(line))
}

// runFallbackMenu is the historical numbered-prompt body, preserved verbatim
// so the byte-for-byte output contract holds for piped/redirected callers.
func runFallbackMenu(prompt string, options []string, defaultIdx int) (int, error) {
	fmt.Println(prompt)

	for i, opt := range options {
		defaultMarker := ""
		if defaultIdx == i+1 {
			defaultMarker = " " + ColorGreen + "(default)" + ColorReset
		}
		fmt.Printf("  %s%d)%s %s%s\n", ColorBold, i+1, ColorReset, opt, defaultMarker)
	}

	cancelMarker := ""
	if defaultIdx == 0 {
		cancelMarker = " " + ColorGreen + "(default)" + ColorReset
	}
	fmt.Printf("  %s0)%s Cancel%s\n", ColorBold, ColorReset, cancelMarker)
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		if defaultIdx >= 0 {
			fmt.Printf("Choice [%d]: ", defaultIdx)
		} else {
			fmt.Print("Choice: ")
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("reading input: %w", err)
		}
		line = strings.TrimSpace(line)

		// Handle empty input
		if line == "" {
			if defaultIdx >= 0 {
				return defaultIdx, nil
			}
			return 0, nil
		}

		// Validate numeric input
		choice, err := strconv.Atoi(line)
		if err != nil {
			fmt.Println("Invalid choice. Please enter a number.")
			continue
		}

		if choice < 0 || choice > len(options) {
			fmt.Printf("Invalid choice. Please enter a number between 0 and %d.\n", len(options))
			continue
		}

		return choice, nil
	}
}

// Prompt rendering and answer-parsing pieces shared by the package-level
// prompts and their MenuSession variants, so the prompt text and grammar
// cannot drift between the standalone and session-aware paths.
const (
	promptWithDefaultFmt = "%s [%s]: " // stdout — PromptWithDefault
	confirmYesNoFmt      = "%s [Y/n] " // stderr — ConfirmYesNo
)

// parseYesNoLine interprets an already-trimmed ConfirmYesNo answer line:
// empty means yes (the default); otherwise a line starting with "y"/"Y" is
// yes and anything else is no.
func parseYesNoLine(line string) bool {
	if line == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(line), "y")
}

// ConfirmYesNo prompts for a Y/n confirmation. Returns true if yes (default).
// The prompt is written to stderr (human-facing copy), keeping stdout reserved
// for machine-readable results even when the caller's stdout is redirected
// while stdin remains a TTY.
//
// This standalone form reads a line synchronously via a fresh bufio.Reader over
// os.Stdin (cooked mode), so it leaves no parked read-ahead goroutine and is
// safe for one-off use. It MUST NOT be used in a flow that also shows an
// interactive menu on the same stdin — a preceding menu's parked pump would
// steal this prompt's first line. Such mixed flows MUST use
// MenuSession.ConfirmYesNo, which reads through the session's shared reader.
func ConfirmYesNo(prompt string) bool {
	fmt.Fprintf(os.Stderr, confirmYesNoFmt, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return parseYesNoLine(strings.TrimSpace(line))
}

// PromptWithDefault prompts for input with a default value.
//
// This standalone form reads a line synchronously via a fresh bufio.Reader over
// os.Stdin (cooked mode), so it leaves no parked read-ahead goroutine and is
// safe for one-off use. It MUST NOT be used in a flow that also shows an
// interactive menu on the same stdin — a preceding menu's parked pump would
// steal this prompt's first typed line. Such mixed flows MUST use
// MenuSession.PromptWithDefault, which reads through the session's shared reader.
func PromptWithDefault(prompt, defaultValue string) string {
	fmt.Printf(promptWithDefaultFmt, prompt, defaultValue)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

// =============================================================================
// Interactive arrow-key renderer
// =============================================================================
//
// The renderer is split into three layers so the contract can be unit-tested
// without ever opening a real terminal (Constitution Principle IV):
//
//   1. nextMenuState  — pure state machine (current highlight + keyEvent →
//                       new highlight + submitted flag).
//   2. parseKey       — pure escape-sequence parser (raw bytes → keyEvent),
//                       with the 50ms post-Esc read window injected via a
//                       reader+clock interface so tests can drive it
//                       deterministically.
//   3. runInteractiveMenu — thin I/O shell that owns raw-mode setup/teardown,
//                       initial paint, in-place redraw, and exit rendering.
//
// Layers 1 and 2 carry the keybinding contract; layer 3 is intentionally thin
// and is QA'd manually (see plan.md Manual QA notes).

// ANSI / control sequence constants. Centralized so the state machine and
// renderer reference named values rather than scattered string literals.
const (
	// Escape sequence prefixes / payloads recognized by parseKey.
	escCSIArrowUp   = "\x1b[A"
	escCSIArrowDown = "\x1b[B"

	// Single-byte control codes.
	byteEsc   = 0x1b // ESC
	byteEnter = '\r'
	byteLF    = '\n'
	byteETX   = 0x03 // Ctrl-C
	byteCSI   = '['  // CSI introducer following ESC

	// Terminal control sequences used for in-place redraw.
	ansiClearLine   = "\x1b[2K"
	ansiCarriageRet = "\r"
	ansiCursorUpFmt = "\x1b[%dA" // moves cursor up N rows

	// Highlight / marker visuals.
	ansiReverseVideo = "\x1b[7m"
	ansiBoldOff      = "\x1b[22m" // turns off bold/dim without clearing reverse video
	ansiResetSGR     = "\x1b[0m"
	gutterHighlight  = "›"
	gutterPlain      = " "

	// Bare-Esc disambiguation window. A standalone ESC byte followed by no
	// follow-up byte within this window is treated as Cancel; otherwise the
	// follow-up bytes form an escape sequence like \x1b[A.
	escTimeoutMs = 50

	// defaultTerminalHeight is the row count assumed when term.GetSize fails.
	// 24 is the classic terminal default; falling back to it keeps the windowed
	// output bounded rather than degrading to the unbounded full-region paint
	// that the viewport change exists to fix.
	defaultTerminalHeight = 24

	// menuOverheadRows is the number of always-present, non-option rows in the
	// menu region: the prompt line and the Cancel row. The option viewport is
	// the terminal height minus this overhead minus one further reserved row
	// (so the `\r\n`-terminated region's footprint stays <= height-1 and never
	// scrolls the terminal on repaint — see menuLayout's budget comment).
	menuOverheadRows = 2

	// Overflow indicator gutter text. Rendered as non-selectable rows at the
	// window edges when options are hidden above/below (e.g. "↑ 3 more").
	indicatorUp   = "↑"
	indicatorDown = "↓"
)

// menuState is the pure state passed into nextMenuState.
//
// Highlight indexing convention (also used by the renderer):
//   - 0           → the Cancel row.
//   - 1..numOptions → the corresponding option row.
//
// top is the window offset — the 0-based index of the first option row visible
// in the scrolling viewport (see menuLayout). It is 0 for menus that fit the
// terminal. Like highlight, it is pure state: the renderer recomputes it from
// menuLayout at paint/redraw time so a terminal resize is absorbed on the next
// keystroke without any SIGWINCH handling.
//
// defaultIdx mirrors ShowMenu's parameter and is carried so seeding tests can
// assert the initial highlight rule without re-deriving it.
type menuState struct {
	highlight  int
	numOptions int
	defaultIdx int
	top        int
}

// menuStateTransition is the result of feeding a keyEvent to nextMenuState.
// submitted == true means the renderer should exit with the new highlight as
// the final return value (0 for Cancel, 1..numOptions for an option).
type menuStateTransition struct {
	highlight int
	submitted bool
}

// keyEventKind is the tag of the keyEvent sum type.
type keyEventKind int

const (
	keyIgnore keyEventKind = iota
	keyUp
	keyDown
	keyEnter
	keyCancel
	keyDigit
)

// keyEvent is a small sum type emitted by parseKey and consumed by
// nextMenuState. Only Digit carries a payload; the rest are tag-only.
type keyEvent struct {
	kind  keyEventKind
	digit int // valid only when kind == keyDigit; range 0..9
}

// initialHighlight returns the highlight row that ShowMenu's renderer should
// pre-select on first paint, per spec:
//
//   - defaultIdx >= 1               → that option row.
//   - defaultIdx == 0               → the Cancel row (highlight = 0).
//   - defaultIdx == -1 (no default) → the first option (highlight = 1) when
//     options exist; otherwise Cancel.
//
// Any defaultIdx that is out of range (e.g., > numOptions) falls through to
// the "no default" behavior so a misuse from a caller does not crash.
func initialHighlight(defaultIdx, numOptions int) int {
	if numOptions <= 0 {
		// Degenerate menu — only Cancel is reachable.
		return 0
	}
	if defaultIdx >= 1 && defaultIdx <= numOptions {
		return defaultIdx
	}
	if defaultIdx == 0 {
		return 0
	}
	return 1
}

// menuLayout computes the scrolling viewport for the option region. It is pure:
// no I/O, no globals, no clock — the same discipline as nextMenuState — so every
// windowing edge case is unit-testable without a PTY.
//
// Inputs:
//   - numOptions : total option rows (excludes prompt and Cancel).
//   - highlight  : the current highlight (0 = Cancel, 1..numOptions = option).
//   - prevTop    : the previous window offset (0-based first-visible option
//     index), so the window shifts only as far as needed to keep the highlight
//     visible instead of jumping on every keystroke.
//   - height     : the terminal height in rows (already resolved, e.g. via
//     terminalHeight()).
//
// Returns:
//   - top       : the new window offset (0-based index of the first visible option).
//   - first     : alias of top, the 0-based index of the first visible option.
//   - count     : number of option rows visible in the window.
//   - moreAbove : count of options hidden above the window (0 → no ↑ indicator).
//   - moreBelow : count of options hidden below the window (0 → no ↓ indicator).
//
// Row budget: the option viewport is height-menuOverheadRows-1 (prompt + Cancel
// + one reserved row that must stay unrendered so the region's `\r\n`-terminated
// footprint is rowsRendered <= height-1 and never scrolls the terminal on
// repaint — see the budget comment in the body). When indicators are shown they
// consume viewport rows, so the visible option count shrinks by up to 2 — but
// only while the budget can still spare a row for an indicator AND show at least
// one option. When the budget is too small to fit indicator chrome plus an
// option (heights 4–5, where indicator deductions would drive the visible count
// below one), the indicator rows are DROPPED rather than overflowing the budget:
// the "N more" chrome is sacrificed so the option is shown and the option-region
// rows never exceed the budget (prefer showing the option over the chrome, as
// fzf does). This keeps rowsRendered <= height-1 down to height 4 — the honest
// footprint boundary. Only at the truly sub-overhead heights 1–3 (where the raw
// budget is < 1 and is clamped up to 1) does the layout necessarily overshoot
// height-1: the ≥1-option escape hatch keeps output bounded and non-empty rather
// than panicking on a degenerate terminal.
//
// When highlight is Cancel (0) the option window is not re-centered — Cancel is
// a fixed overhead row rendered after the option region and is always visible,
// so it never constrains the window. The window is only shifted to keep a
// highlighted *option* in view; on wrap-around the highlight can jump far (e.g.
// from option 1 up to Cancel to the last option) and the window jumps with it.
func menuLayout(numOptions, highlight, prevTop, height int) (top, first, count, moreAbove, moreBelow int) {
	if numOptions <= 0 {
		return 0, 0, 0, 0, 0
	}

	// Option-region budget: total height minus the prompt + Cancel overhead,
	// minus one further row that MUST stay unrendered. Every row (including the
	// Cancel row) is terminated with `\r\n`, so a region that filled all `height`
	// rows would emit a trailing newline on the bottom screen row and scroll the
	// terminal by one line on every repaint — pushing the prompt into scrollback
	// and leaking a stale menu copy per keystroke. Reserving one row keeps the
	// rendered footprint at `rowsRendered <= height - 1`, so the cursor-up
	// in-place redraw stays sound. Clamped to at least one row so degenerate
	// short terminals still render bounded, non-empty output.
	budget := height - menuOverheadRows - 1
	if budget < 1 {
		budget = 1
	}

	// Everything fits — no windowing, no indicators.
	if numOptions <= budget {
		return 0, 0, numOptions, 0, 0
	}

	// Windowing is required. Start from the previous top and shift it just
	// enough to keep the highlighted option visible. Cancel (highlight 0) does
	// not constrain the option window.
	top = prevTop
	if top < 0 {
		top = 0
	}

	if highlight >= 1 {
		h := highlight - 1 // 0-based option index
		// Shift the window down until the highlight is not below it.
		for {
			v := optionRowsForTop(budget, numOptions, top)
			if h < top+v {
				break
			}
			top++
		}
		// Shift the window up until the highlight is not above it.
		for top > 0 && h < top {
			top--
		}
	}

	// Clamp top so the window never runs past the end of the list, then
	// recompute the final visible count and hidden-row counts.
	maxTop := numOptions - 1
	if top > maxTop {
		top = maxTop
	}
	if top < 0 {
		top = 0
	}
	count = optionRowsForTop(budget, numOptions, top)
	if top+count > numOptions {
		count = numOptions - top
	}
	if count < 1 {
		count = 1
	}

	first = top
	moreAbove = top
	moreBelow = numOptions - (top + count)
	if moreBelow < 0 {
		moreBelow = 0
	}

	// Drop indicator chrome the budget cannot afford, so the option-region rows
	// (indicators + visible options) never exceed the budget.
	moreAbove, moreBelow = dropUnaffordableIndicators(budget, count, moreAbove, moreBelow)
	return top, first, count, moreAbove, moreBelow
}

// optionRowsForTop returns how many option rows fit in the viewport for a
// candidate window offset `top`, accounting for the ↑/↓ indicator rows that
// offset implies. An ↑ indicator is present when top > 0 (options hidden above);
// a ↓ indicator when options remain below the window — and whether that is true
// depends on the count itself, so both are tested against the reduced count and
// the tighter (safe) answer is taken. When the budget is so small that deducting
// the indicator rows would leave zero options, it clamps to one option; the
// indicator chrome is then dropped by dropUnaffordableIndicators so the
// option-region rows still fit the budget.
func optionRowsForTop(budget, numOptions, top int) int {
	v := budget
	if top > 0 {
		v-- // ↑ indicator row
	}
	if top+v < numOptions {
		v-- // ↓ indicator row
	}
	if v < 1 {
		v = 1
	}
	return v
}

// dropUnaffordableIndicators sacrifices indicator chrome the budget cannot hold.
// renderRows keys each indicator row off moreAbove/moreBelow, so an indicator
// occupies a row iff its count is > 0. When the budget is too small to hold the
// indicator rows plus the visible options (heights 4–5, and the clamped
// sub-overhead heights 1–3), the ↓ indicator is dropped first, then the ↑ — so
// the option-region rows (indicators + count) never exceed the budget and the
// rendered footprint stays <= height-1 (down to the honest boundary, height 4).
// Preferring the option over the "N more" hint mirrors fzf's behavior on a
// cramped viewport. Returns the (possibly zeroed) hidden-row counts.
func dropUnaffordableIndicators(budget, count, moreAbove, moreBelow int) (int, int) {
	indicators := func() int {
		c := 0
		if moreAbove > 0 {
			c++
		}
		if moreBelow > 0 {
			c++
		}
		return c
	}
	for indicators()+count > budget && indicators() > 0 {
		if moreBelow > 0 {
			moreBelow = 0
		} else {
			moreAbove = 0
		}
	}
	return moreAbove, moreBelow
}

// nextMenuState applies a keyEvent to the previous state and returns the
// new highlight plus a submitted flag. Pure: no I/O, no globals, no clock.
//
// Wrap-around rules:
//
//	Up   from row 1               → Cancel (highlight = 0)
//	Up   from Cancel              → last option (highlight = numOptions)
//	Down from last option         → Cancel (highlight = 0)
//	Down from Cancel              → first option (highlight = 1)
//
// Digit rules:
//
//	Digit(0)                      → submit Cancel (highlight = 0)
//	Digit(1..numOptions)          → submit that option
//	Digit(>numOptions) or other   → no-op (highlight unchanged, submitted=false)
//
// Cancel / Enter:
//
//	Cancel                        → submit current highlight as 0 (Cancel row)
//	Enter                         → submit current highlight
//
// Ignore (and any unhandled key kind) leaves state unchanged.
func nextMenuState(prev menuState, key keyEvent) menuStateTransition {
	switch key.kind {
	case keyUp:
		// Walking up: option 1 → Cancel; Cancel → last option; else highlight-1.
		switch {
		case prev.numOptions <= 0:
			return menuStateTransition{highlight: 0}
		case prev.highlight == 0:
			return menuStateTransition{highlight: prev.numOptions}
		case prev.highlight == 1:
			return menuStateTransition{highlight: 0}
		default:
			return menuStateTransition{highlight: prev.highlight - 1}
		}
	case keyDown:
		// Walking down: last option → Cancel; Cancel → option 1; else highlight+1.
		switch {
		case prev.numOptions <= 0:
			return menuStateTransition{highlight: 0}
		case prev.highlight == 0:
			return menuStateTransition{highlight: 1}
		case prev.highlight == prev.numOptions:
			return menuStateTransition{highlight: 0}
		default:
			return menuStateTransition{highlight: prev.highlight + 1}
		}
	case keyEnter:
		return menuStateTransition{highlight: prev.highlight, submitted: true}
	case keyCancel:
		return menuStateTransition{highlight: 0, submitted: true}
	case keyDigit:
		if key.digit == 0 {
			return menuStateTransition{highlight: 0, submitted: true}
		}
		if key.digit >= 1 && key.digit <= prev.numOptions {
			return menuStateTransition{highlight: key.digit, submitted: true}
		}
		// Out-of-range digit: silently ignore.
		return menuStateTransition{highlight: prev.highlight}
	default:
		// keyIgnore and any future kinds: no-op.
		return menuStateTransition{highlight: prev.highlight}
	}
}

// =============================================================================
// Escape-sequence parser
// =============================================================================

// byteReader is the minimal seam parseKey uses to pull bytes one at a time.
// The bufio.Reader satisfies it; tests provide a fake that can simulate
// either an immediate follow-up byte or a timeout.
type byteReader interface {
	// readByteWithin reads one byte, waiting at most `timeout` for it to
	// arrive. Returns (byte, true) when a byte is available within the
	// window, (0, false) on timeout or EOF.
	readByteWithin(timeout time.Duration) (byte, bool)
}

// blockingByteReader wraps a bufio.Reader behind a single pump goroutine that
// reads one byte at a time and forwards it through `bytes`. Both
// readByteBlocking and readByteWithin consume from that channel — so the
// underlying reader has exactly one outstanding ReadByte() in flight at any
// moment. On a bare-Esc timeout, the pump goroutine remains blocked on its
// pending read, but the next call (timed or blocking) drains the same channel
// rather than spawning a new reader. This avoids two goroutines racing on the
// same bufio.Reader and prevents stolen / interleaved bytes after a timeout.
type blockingByteReader struct {
	src   *bufio.Reader
	bytes chan readPumpResult
	once  sync.Once
}

type readPumpResult struct {
	bt    byte
	err   error
	panic any // non-nil if the pump goroutine recovered a panic from the reader
}

func newBlockingByteReader(r io.Reader) *blockingByteReader {
	return &blockingByteReader{
		src:   bufio.NewReader(r),
		bytes: make(chan readPumpResult, 1),
	}
}

// startPump launches the single byte-pump goroutine on first use. It reads
// one byte, sends it (or its error) to `bytes`, then loops. On error/EOF the
// goroutine sends the error result and exits — subsequent receives on the
// channel will block, but `readByteWithin` always pairs the receive with a
// timer and `readByteBlocking` will simply return (0, false) as soon as the
// error result has been delivered (the channel is buffered with capacity 1).
//
// If the underlying reader panics (e.g., a test seam injects a panicking
// reader), the pump recovers the panic value and forwards it via the
// `panic` field. The consumer (readByteBlocking / readByteWithin) re-raises
// the panic on the caller's goroutine so deferred cleanup — most importantly
// the raw-mode restore in runInteractiveMenuCore — still runs.
func (b *blockingByteReader) startPump() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				b.bytes <- readPumpResult{panic: r}
			}
		}()
		for {
			bt, err := b.src.ReadByte()
			b.bytes <- readPumpResult{bt: bt, err: err}
			if err != nil {
				return
			}
		}
	}()
}

// readByteBlocking reads one byte, blocking until one is available or EOF.
// Used for the first byte of every key (no timeout there — we're waiting for
// the user to type something).
func (b *blockingByteReader) readByteBlocking() (byte, bool) {
	b.once.Do(b.startPump)
	r := <-b.bytes
	if r.panic != nil {
		panic(r.panic)
	}
	if r.err != nil {
		return 0, false
	}
	return r.bt, true
}

// readByteWithin reads one byte with a deadline. Because all reads flow
// through the single pump goroutine (see blockingByteReader doc comment),
// timing out here leaves at most one pending read in flight on the underlying
// bufio.Reader — that read's result is delivered to the next caller of
// readByteBlocking / readByteWithin, never lost and never racing a second
// reader on the same stream.
func (b *blockingByteReader) readByteWithin(timeout time.Duration) (byte, bool) {
	b.once.Do(b.startPump)
	select {
	case r := <-b.bytes:
		if r.panic != nil {
			panic(r.panic)
		}
		if r.err != nil {
			return 0, false
		}
		return r.bt, true
	case <-time.After(timeout):
		return 0, false
	}
}

// readLine reads a full line through the same single pump goroutine that Show's
// key reads use, accumulating one byte at a time via readByteBlocking until a
// '\n' arrives, then stripping a trailing "\r\n" or "\n". It returns
// (line, true) on a complete line and ("", false) on a read failure/EOF before
// any newline (partial input before the failure is discarded — matching the
// err != nil short-circuit in the package-level PromptWithDefault/ConfirmYesNo).
//
// Reading through the shared reader is the whole point: it is what lets a menu
// (Show) and a line prompt (MenuSession.PromptWithDefault/ConfirmYesNo) run back
// to back on one stdin without the menu's parked pump stealing the prompt's
// first line. No raw mode is involved — line prompts run in cooked mode, so the
// kernel line-buffers and echoes; this helper only reassembles the bytes the
// pump forwards.
func (b *blockingByteReader) readLine() (string, bool) {
	var sb strings.Builder
	for {
		bt, ok := b.readByteBlocking()
		if !ok {
			// EOF/error before a newline: discard partial input, signal failure.
			return "", false
		}
		if bt == byteLF {
			line := sb.String()
			// Strip a trailing "\r" so "\r\n" and "\n" line endings both yield
			// the bare line (the '\n' itself is already excluded above).
			return strings.TrimSuffix(line, "\r"), true
		}
		sb.WriteByte(bt)
	}
}

// parseKey reads bytes from `first` (the byte already in hand) plus `rest`
// (the follow-up reader, used only for multi-byte escape sequences) and
// returns the corresponding keyEvent.
//
// Mapping:
//
//	\x1b[A      → Up
//	\x1b[B      → Down
//	\x1b<other> → Ignore (e.g., \x1bOP is F1)
//	bare \x1b   → Cancel (after escTimeoutMs without a follow-up byte)
//	\r, \n      → Enter
//	\x03        → Cancel (Ctrl-C as a raw-mode byte)
//	q           → Cancel
//	j           → Down
//	k           → Up
//	'0'..'9'    → Digit(n)
//	other       → Ignore
func parseKey(first byte, rest byteReader) keyEvent {
	switch first {
	case byteEnter, byteLF:
		return keyEvent{kind: keyEnter}
	case byteETX:
		return keyEvent{kind: keyCancel}
	case 'q':
		return keyEvent{kind: keyCancel}
	case 'j':
		return keyEvent{kind: keyDown}
	case 'k':
		return keyEvent{kind: keyUp}
	case byteEsc:
		return parseEscapeSequence(rest)
	}
	if first >= '0' && first <= '9' {
		return keyEvent{kind: keyDigit, digit: int(first - '0')}
	}
	return keyEvent{kind: keyIgnore}
}

// parseEscapeSequence is invoked once the leading ESC has been consumed. It
// peeks the next byte within escTimeoutMs to disambiguate a bare Esc (Cancel)
// from the start of a CSI sequence (\x1b[A, \x1b[B, etc.).
//
// Recognized:
//
//	\x1b[A      → Up
//	\x1b[B      → Down
//	\x1b[<other>→ Ignore (consumes one trailing byte to clear the buffer)
//	\x1b<other> → Ignore (e.g., \x1bOP)
//	timeout     → Cancel
func parseEscapeSequence(rest byteReader) keyEvent {
	timeout := time.Duration(escTimeoutMs) * time.Millisecond
	second, ok := rest.readByteWithin(timeout)
	if !ok {
		return keyEvent{kind: keyCancel}
	}
	if second != byteCSI {
		// Esc followed by some non-CSI byte (e.g., 'O' from F1 \x1bOP).
		// Drain one more byte so the next read does not see a stray
		// payload byte, but otherwise treat the sequence as unknown.
		_, _ = rest.readByteWithin(timeout)
		return keyEvent{kind: keyIgnore}
	}
	// We have \x1b[ — the third byte selects the arrow.
	third, ok := rest.readByteWithin(timeout)
	if !ok {
		return keyEvent{kind: keyIgnore}
	}
	switch third {
	case 'A':
		return keyEvent{kind: keyUp}
	case 'B':
		return keyEvent{kind: keyDown}
	default:
		// Other CSI sequences (Page Up/Down, F-keys via \x1b[<digit>~, etc.)
		// — silently ignored. We don't aggressively drain because the
		// remaining bytes (if any) will be parsed as their own keys; in
		// practice raw-mode terminals deliver these in a single read burst
		// well within the 50ms window, so this is robust enough.
		return keyEvent{kind: keyIgnore}
	}
}

// =============================================================================
// TTY detection
// =============================================================================

// isInteractiveTTY returns true only when BOTH os.Stdin and os.Stdout are
// connected to a TTY. The check runs exactly once per ShowMenu invocation,
// before any output is emitted, so the chosen path (interactive vs.
// fallback) governs the entire rendered output.
//
// On Windows the function unconditionally returns false: ConPTY raw-mode
// quirks (line-buffering on certain terminals, key-code differences) make a
// full Windows arrow-key implementation a non-trivial side quest that would
// not block Linux/macOS users. The Windows fallback path is byte-identical
// to the historical behavior. See spec Cross-Platform requirement.
func isInteractiveTTY() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// terminalHeight returns the current terminal height (row count) of stdout —
// the stream the menu region is painted to. On any term.GetSize failure it
// returns defaultTerminalHeight (24) so the scrolling viewport stays bounded
// rather than degrading to the unbounded full-region paint the viewport change
// fixes. It is the production heightFn wired into runInteractiveMenuCore; tests
// inject a constant instead so windowing is exercised without a PTY.
func terminalHeight() int {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h < 1 {
		return defaultTerminalHeight
	}
	return h
}

// =============================================================================
// Raw-mode I/O shell
// =============================================================================

// runInteractiveMenuCore is the testable core of the interactive renderer. It
// owns the paint/read/redraw/finalize loop. The caller provides:
//
//   - w         : output sink (os.Stdout in production).
//   - reader    : the SHARED stdin reader for this terminal session. Reusing
//     one reader across consecutive menus is what prevents an
//     orphaned read-ahead pump from stealing the next menu's first
//     keystroke (see MenuSession). In tests this is a reader over a
//     pipe or a panicking source.
//   - restoreFn : raw-mode restore safety net, deferred so cooked mode is
//     reinstated if the read loop panics mid-menu. In production
//     the owning MenuSession also restores on Close; restoring
//     twice is harmless. In tests it is a counter-incrementing fake.
//     MUST be invoked exactly once before this function returns,
//     even on panic — that contract guarantees the user's terminal
//     is never left in raw mode.
//   - heightFn  : returns the terminal height, queried at paint/redraw time so
//     the scrolling viewport (see menuLayout) adapts to the current
//     terminal size and a resize is absorbed on the next keystroke's
//     repaint (no SIGWINCH handling). Production wires terminalHeight;
//     tests inject a constant so windowing is exercised without a PTY.
//
// SIGINT handling: raw mode delivers Ctrl-C as a raw \x03 byte rather than
// generating a SIGINT, so no signal escapes mid-menu. The deferred restoreFn
// always runs.
func runInteractiveMenuCore(w io.Writer, reader *blockingByteReader, prompt string, options []string, defaultIdx int, restoreFn func(), heightFn func() int) (int, error) {
	defer restoreFn()

	state := menuState{
		highlight:  initialHighlight(defaultIdx, len(options)),
		numOptions: len(options),
		defaultIdx: defaultIdx,
	}
	// Query height once for both the initial window settle and the first paint,
	// so they observe the same terminal size (heightFn is a syscall in
	// production — no reason to call it twice back to back).
	initialHeight := heightFn()
	// Settle the initial window so the seeded highlight is visible on first
	// paint (e.g. a defaultIdx deep in an over-tall list starts scrolled).
	state.top, _, _, _, _ = menuLayout(state.numOptions, state.highlight, state.top, initialHeight)

	// rowsRendered tracks how many lines the menu currently occupies on
	// screen so the next redraw knows how far up to move the cursor. It is the
	// *windowed* row count (1 prompt + indicators + visible options + 1 Cancel)
	// and stays within menuLayout's budget — rowsRendered <= height-1 (one
	// reserved row keeps the `\r\n`-terminated footprint from scrolling the
	// terminal), the only exception being degenerate heights 1–3 where the
	// ≥1-option escape hatch necessarily overshoots. This is what keeps the
	// cursor-up in-place redraw sound for lists taller than the terminal.
	rowsRendered := paintMenu(w, prompt, options, state, initialHeight)

	for {
		first, ok := reader.readByteBlocking()
		if !ok {
			// stdin closed mid-menu — treat as Cancel so the caller's
			// "choice == 0" branch fires cleanly.
			finalizeMenu(w, prompt, options, 0, rowsRendered)
			return 0, nil
		}
		ev := parseKey(first, reader)
		next := nextMenuState(state, ev)

		if next.submitted {
			finalizeMenu(w, prompt, options, next.highlight, rowsRendered)
			return next.highlight, nil
		}

		if next.highlight != state.highlight {
			// Shift the window (pure) so the new highlight stays visible, then
			// redraw. Height is re-queried here so a resize is picked up.
			height := heightFn()
			newTop, _, _, _, _ := menuLayout(state.numOptions, next.highlight, state.top, height)
			rowsRendered = redrawMenu(w, prompt, options, menuState{
				highlight:  next.highlight,
				numOptions: state.numOptions,
				defaultIdx: state.defaultIdx,
				top:        newTop,
			}, rowsRendered, height)
			state.highlight = next.highlight
			state.top = newTop
		}
		// Ignored key with no highlight change → no output, no bell.
	}
}

// paintMenu writes the (windowed) menu region for the first time and returns
// the number of lines written (so redraw / finalize know how far to move up).
// height drives the scrolling viewport; the returned count is the windowed
// total (prompt + indicators + visible options + Cancel) and stays within
// menuLayout's budget of height-1 (one reserved row), except at degenerate
// heights 1–3 where the ≥1-option escape hatch overshoots. This keeps the
// cursor-up in-place redraw sound for over-tall lists.
func paintMenu(w io.Writer, prompt string, options []string, st menuState, height int) int {
	return renderRows(w, prompt, options, st, "", height)
}

// redrawMenu rewrites the menu region in place by moving the cursor up
// rowsRendered lines, clearing each line, and reprinting the new (windowed)
// state. Returns the new windowed row count for the caller to keep tracking.
func redrawMenu(w io.Writer, prompt string, options []string, st menuState, rowsRendered, height int) int {
	// Move the cursor to the start of the menu region.
	fmt.Fprint(w, ansiCarriageRet)
	fmt.Fprintf(w, ansiCursorUpFmt, rowsRendered)
	// Repaint each line with a leading \x1b[2K clear to wipe whatever was
	// there before — the row content may have changed length (highlight
	// gutter `›` vs. plain space, or an indicator row appearing/disappearing).
	// The renderRows helper handles the actual row content; the linePrefix arg
	// supplies the per-line clear.
	newRows := renderRows(w, prompt, options, st, ansiClearLine, height)

	// Shrink robustness: if the new windowed region is shorter than what was
	// on screen (e.g. the terminal was made smaller mid-menu — height is
	// re-queried each redraw, so the window count can drop), the trailing
	// old-minus-new lines below the newly-painted region still hold stale
	// content that renderRows did not touch. Clear those extra lines, then move
	// the cursor back up so it rests just below the new region — keeping the
	// returned newRows an accurate cursor-up target for the next redraw. Resize
	// is a declared non-goal; this is minimal hardening so a shrink does not
	// leave visible ghosts until finalize.
	if extra := rowsRendered - newRows; extra > 0 {
		for i := 0; i < extra; i++ {
			fmt.Fprint(w, ansiClearLine)
			fmt.Fprint(w, "\r\n")
		}
		fmt.Fprint(w, ansiCarriageRet)
		fmt.Fprintf(w, ansiCursorUpFmt, extra)
	}
	return newRows
}

// renderRows emits the menu's row block — prompt line, the windowed option
// rows (with ↑/↓ overflow indicators when rows are hidden), and the Cancel row
// — to w. linePrefix is written before each row; pass "" for the first paint
// and ansiClearLine for in-place redraws. This single helper is the source of
// truth for menu row layout so paintMenu and redrawMenu cannot drift apart on
// formatting (Acceptance A-033: no unnecessary duplication; code-quality.md
// anti-pattern: duplicating existing utilities).
//
// height is the terminal height driving the scrolling viewport (see
// menuLayout). It returns the number of rows written so the caller can track
// how far up the cursor must move on the next redraw; that count is the
// *windowed* total and never exceeds height, which is what keeps the cursor-up
// in-place redraw sound for lists taller than the terminal.
//
// Every row is terminated with `\r\n`. The interactive renderer runs while
// the terminal is in raw mode (ONLCR disabled), so a plain `\n` does not
// imply a carriage return and rows would stair-step across the screen.
func renderRows(w io.Writer, prompt string, options []string, st menuState, linePrefix string, height int) int {
	_, first, count, moreAbove, moreBelow := menuLayout(len(options), st.highlight, st.top, height)

	fmt.Fprint(w, linePrefix)
	fmt.Fprint(w, prompt)
	fmt.Fprint(w, "\r\n")

	rows := 1 // prompt

	if moreAbove > 0 {
		fmt.Fprint(w, linePrefix)
		writeIndicatorRow(w, indicatorUp, moreAbove)
		rows++
	}
	for i := first; i < first+count; i++ {
		row := i + 1
		fmt.Fprint(w, linePrefix)
		writeOptionRow(w, row, options[i], row == st.defaultIdx, row == st.highlight)
		rows++
	}
	if moreBelow > 0 {
		fmt.Fprint(w, linePrefix)
		writeIndicatorRow(w, indicatorDown, moreBelow)
		rows++
	}

	fmt.Fprint(w, linePrefix)
	writeCancelRow(w, st.defaultIdx == 0, st.highlight == 0)
	rows++ // Cancel

	return rows
}

// finalizeMenu wipes the menu region and writes a single summary line:
//   - On submit (choice >= 1): "<prompt> <option-text>"
//   - On Cancel (choice == 0): "<prompt> (cancelled)"
//
// All line terminators are `\r\n` because this runs while the terminal is
// still in raw mode (ONLCR disabled). A plain `\n` would leave the cursor at
// the current column when advancing to the next line and the post-menu shell
// prompt could appear stair-stepped.
func finalizeMenu(w io.Writer, prompt string, options []string, choice, rowsRendered int) {
	// Reposition to the top of the menu region.
	fmt.Fprint(w, ansiCarriageRet)
	fmt.Fprintf(w, ansiCursorUpFmt, rowsRendered)
	// Clear every rendered line.
	for i := 0; i < rowsRendered; i++ {
		fmt.Fprint(w, ansiClearLine)
		if i < rowsRendered-1 {
			fmt.Fprint(w, "\r\n")
		}
	}
	// After the loop the cursor is on the last cleared line; move back to
	// the top and emit the single summary line.
	fmt.Fprint(w, ansiCarriageRet)
	fmt.Fprintf(w, ansiCursorUpFmt, rowsRendered-1)
	fmt.Fprint(w, ansiClearLine)
	if choice == 0 {
		fmt.Fprintf(w, "%s (cancelled)\r\n", prompt)
		return
	}
	// choice in 1..len(options)
	fmt.Fprintf(w, "%s %s\r\n", prompt, options[choice-1])
}

// writeOptionRow renders one numbered option line. The gutter shows `›` on
// the currently highlighted row (per intake §2); the entire row (number AND
// label) is rendered in reverse video on the highlighted row so the selection
// is visible even when the gutter character is not perceptible (e.g.,
// terminals rendering combining marks oddly). The `(default)` green marker is
// preserved on the default row regardless of highlight state.
//
// Highlighted row composition:
//
//	gutter + ' ' + REV + BOLD + 'N)' + BOLDOFF + ' ' + label + RESET + defaultMarker
//
// `\x1b[22m` (bold off) is used instead of `\x1b[0m` after the number so
// reverse video stays active across the label. The full SGR reset comes once
// at the end of the row, after the label.
//
// Lines are terminated with `\r\n` because the interactive renderer runs while
// the terminal is in raw mode (ONLCR is disabled), so a plain `\n` does not
// implicitly return the cursor to column 0 and rows would stair-step on
// redraw / final output.
func writeOptionRow(w io.Writer, num int, label string, isDefault, isHighlighted bool) {
	gutter := gutterPlain
	if isHighlighted {
		gutter = gutterHighlight
	}
	defaultMarker := ""
	if isDefault {
		defaultMarker = " " + ColorGreen + "(default)" + ColorReset
	}
	if isHighlighted {
		fmt.Fprintf(w, "%s %s%s%d)%s %s%s%s\r\n",
			gutter,
			ansiReverseVideo, ColorBold, num, ansiBoldOff,
			label, ansiResetSGR,
			defaultMarker)
		return
	}
	fmt.Fprintf(w, "%s %s%d)%s %s%s\r\n",
		gutter, ColorBold, num, ColorReset, label, defaultMarker)
}

// writeIndicatorRow renders a non-selectable overflow indicator row shown at a
// window edge when option rows are hidden — e.g. "↑ 3 more" at the top or
// "↓ 5 more" at the bottom. It is a rendering artifact, never a menu option:
// it carries no number, is never highlighted, and occupies a row within the
// viewport budget (see menuLayout). Styled like a plain (non-highlighted,
// non-default) row so it reads as chrome, not a choice.
//
// The `\r\n` terminator matches every other row: the interactive renderer runs
// in raw mode (ONLCR disabled), so a plain `\n` would stair-step the output.
func writeIndicatorRow(w io.Writer, arrow string, hidden int) {
	// Leading two spaces align the indicator text under the option labels
	// (the option gutter+space is also two columns wide before the "N)").
	fmt.Fprintf(w, "  %s %d more\r\n", arrow, hidden)
}

// writeCancelRow renders the Cancel row. Mirrors writeOptionRow's structure
// so highlight/default visuals stay consistent (including the reverse-video
// across the `Cancel` label on the highlighted row and the raw-mode-safe
// `\r\n` line terminator).
func writeCancelRow(w io.Writer, isDefault, isHighlighted bool) {
	gutter := gutterPlain
	if isHighlighted {
		gutter = gutterHighlight
	}
	defaultMarker := ""
	if isDefault {
		defaultMarker = " " + ColorGreen + "(default)" + ColorReset
	}
	if isHighlighted {
		fmt.Fprintf(w, "%s %s%s0)%s Cancel%s%s\r\n",
			gutter,
			ansiReverseVideo, ColorBold, ansiBoldOff,
			ansiResetSGR,
			defaultMarker)
		return
	}
	fmt.Fprintf(w, "%s %s0)%s Cancel%s\r\n",
		gutter, ColorBold, ColorReset, defaultMarker)
}
