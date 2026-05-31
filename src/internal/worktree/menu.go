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
	return runInteractiveMenuCore(w, s.reader, prompt, options, defaultIdx, restoreFn)
}

// Close ends the session. It is currently a no-op (raw mode is managed per
// Show), kept so callers can express the session lifecycle with a deferred
// Close and so the contract survives future changes that acquire resources.
func (s *MenuSession) Close() {}

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

// ConfirmYesNo prompts for a Y/n confirmation. Returns true if yes (default).
func ConfirmYesNo(prompt string) bool {
	fmt.Printf("%s [Y/n] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(line), "y")
}

// PromptWithDefault prompts for input with a default value.
func PromptWithDefault(prompt, defaultValue string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultValue)
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
)

// menuState is the pure state passed into nextMenuState.
//
// Highlight indexing convention (also used by the renderer):
//   - 0           → the Cancel row.
//   - 1..numOptions → the corresponding option row.
//
// defaultIdx mirrors ShowMenu's parameter and is carried so seeding tests can
// assert the initial highlight rule without re-deriving it.
type menuState struct {
	highlight  int
	numOptions int
	defaultIdx int
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
//
// SIGINT handling: raw mode delivers Ctrl-C as a raw \x03 byte rather than
// generating a SIGINT, so no signal escapes mid-menu. The deferred restoreFn
// always runs.
func runInteractiveMenuCore(w io.Writer, reader *blockingByteReader, prompt string, options []string, defaultIdx int, restoreFn func()) (int, error) {
	defer restoreFn()

	state := menuState{
		highlight:  initialHighlight(defaultIdx, len(options)),
		numOptions: len(options),
		defaultIdx: defaultIdx,
	}

	// rowsRendered tracks how many lines the menu currently occupies on
	// screen so the next redraw knows how far up to move the cursor.
	// The menu region is: 1 prompt line + len(options) option rows +
	// 1 Cancel row = len(options) + 2 lines.
	rowsRendered := paintMenu(w, prompt, options, state)

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
			rowsRendered = redrawMenu(w, prompt, options, menuState{
				highlight:  next.highlight,
				numOptions: state.numOptions,
				defaultIdx: state.defaultIdx,
			}, rowsRendered)
			state.highlight = next.highlight
		}
		// Ignored key with no highlight change → no output, no bell.
	}
}

// paintMenu writes the full menu region for the first time and returns the
// number of lines written (so redraw / finalize know how far to move up).
func paintMenu(w io.Writer, prompt string, options []string, st menuState) int {
	renderRows(w, prompt, options, st, "")
	// 1 prompt + N options + 1 Cancel
	return len(options) + 2
}

// redrawMenu rewrites the menu region in place by moving the cursor up
// rowsRendered lines, clearing each line, and reprinting the new state.
// Returns the (unchanged) row count for the caller to keep tracking.
func redrawMenu(w io.Writer, prompt string, options []string, st menuState, rowsRendered int) int {
	// Move the cursor to the start of the menu region.
	fmt.Fprint(w, ansiCarriageRet)
	fmt.Fprintf(w, ansiCursorUpFmt, rowsRendered)
	// Repaint each line with a leading \x1b[2K clear to wipe whatever was
	// there before — the row content may have changed length (highlight
	// gutter `›` vs. plain space). The renderRows helper handles the
	// actual row content; the linePrefix arg supplies the per-line clear.
	renderRows(w, prompt, options, st, ansiClearLine)
	return len(options) + 2
}

// renderRows emits the menu's row block — prompt line, every option row, and
// the Cancel row — to w. linePrefix is written before each row; pass "" for
// the first paint and ansiClearLine for in-place redraws. This single helper
// is the source of truth for menu row layout so paintMenu and redrawMenu
// cannot drift apart on formatting (Acceptance A-033: no unnecessary
// duplication; code-quality.md anti-pattern: duplicating existing utilities).
//
// Every row is terminated with `\r\n`. The interactive renderer runs while
// the terminal is in raw mode (ONLCR disabled), so a plain `\n` does not
// imply a carriage return and rows would stair-step across the screen.
func renderRows(w io.Writer, prompt string, options []string, st menuState, linePrefix string) {
	fmt.Fprint(w, linePrefix)
	fmt.Fprint(w, prompt)
	fmt.Fprint(w, "\r\n")
	for i, opt := range options {
		row := i + 1
		fmt.Fprint(w, linePrefix)
		writeOptionRow(w, row, opt, row == st.defaultIdx, row == st.highlight)
	}
	fmt.Fprint(w, linePrefix)
	writeCancelRow(w, st.defaultIdx == 0, st.highlight == 0)
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
