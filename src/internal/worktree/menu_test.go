package worktree

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/term"
)

// =============================================================================
// nextMenuState — state-machine tests (T012)
// =============================================================================

func TestNextMenuState(t *testing.T) {
	// Standard 3-option menu state.
	st3 := func(highlight int) menuState {
		return menuState{highlight: highlight, numOptions: 3, defaultIdx: -1}
	}

	cases := []struct {
		name       string
		prev       menuState
		key        keyEvent
		wantHL     int
		wantSubmit bool
	}{
		// --- Up navigation ---
		{"Up from row 2 → row 1", st3(2), keyEvent{kind: keyUp}, 1, false},
		{"Up from row 1 → Cancel", st3(1), keyEvent{kind: keyUp}, 0, false},
		{"Up from Cancel → last option", st3(0), keyEvent{kind: keyUp}, 3, false},
		{"Up from row 3 → row 2", st3(3), keyEvent{kind: keyUp}, 2, false},

		// --- Down navigation ---
		{"Down from row 1 → row 2", st3(1), keyEvent{kind: keyDown}, 2, false},
		{"Down from row 2 → row 3", st3(2), keyEvent{kind: keyDown}, 3, false},
		{"Down from row 3 → Cancel (wrap)", st3(3), keyEvent{kind: keyDown}, 0, false},
		{"Down from Cancel → row 1 (wrap)", st3(0), keyEvent{kind: keyDown}, 1, false},

		// --- Enter submit ---
		{"Enter on row 1 submits row 1", st3(1), keyEvent{kind: keyEnter}, 1, true},
		{"Enter on row 3 submits row 3", st3(3), keyEvent{kind: keyEnter}, 3, true},
		{"Enter on Cancel submits 0", st3(0), keyEvent{kind: keyEnter}, 0, true},

		// --- Cancel ---
		{"Cancel from row 2 → highlight 0 submitted", st3(2), keyEvent{kind: keyCancel}, 0, true},
		{"Cancel from Cancel → highlight 0 submitted", st3(0), keyEvent{kind: keyCancel}, 0, true},

		// --- Digit submit ---
		{"Digit 0 submits Cancel", st3(2), keyEvent{kind: keyDigit, digit: 0}, 0, true},
		{"Digit 1 submits row 1", st3(2), keyEvent{kind: keyDigit, digit: 1}, 1, true},
		{"Digit 2 submits row 2", st3(1), keyEvent{kind: keyDigit, digit: 2}, 2, true},
		{"Digit 3 submits row 3", st3(1), keyEvent{kind: keyDigit, digit: 3}, 3, true},

		// --- Digit out-of-range ignored ---
		{"Digit 7 on 3-option menu is ignored", st3(1), keyEvent{kind: keyDigit, digit: 7}, 1, false},
		{"Digit 4 on 3-option menu is ignored", st3(2), keyEvent{kind: keyDigit, digit: 4}, 2, false},
		{"Digit 9 on 3-option menu is ignored", st3(3), keyEvent{kind: keyDigit, digit: 9}, 3, false},

		// --- Ignore ---
		{"Ignore leaves highlight unchanged", st3(2), keyEvent{kind: keyIgnore}, 2, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := nextMenuState(tc.prev, tc.key)
			if got.highlight != tc.wantHL || got.submitted != tc.wantSubmit {
				t.Errorf("nextMenuState(%+v, %+v) = {hl:%d, submit:%t}; want {hl:%d, submit:%t}",
					tc.prev, tc.key, got.highlight, got.submitted, tc.wantHL, tc.wantSubmit)
			}
		})
	}
}

// TestInitialHighlight asserts the defaultIdx seeding rule that the
// interactive renderer uses on first paint (Scenario "Default highlight on
// first paint" + Scenario "No default" in spec.md).
func TestInitialHighlight(t *testing.T) {
	cases := []struct {
		name       string
		defaultIdx int
		numOptions int
		want       int
	}{
		{"defaultIdx=-1 with 3 options → row 1", -1, 3, 1},
		{"defaultIdx=0 with 3 options → Cancel", 0, 3, 0},
		{"defaultIdx=1 with 3 options → row 1", 1, 3, 1},
		{"defaultIdx=2 with 3 options → row 2", 2, 3, 2},
		{"defaultIdx=3 with 3 options → row 3", 3, 3, 3},
		{"defaultIdx=5 out-of-range falls through to row 1", 5, 3, 1},
		{"empty options pins to Cancel", 2, 0, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := initialHighlight(tc.defaultIdx, tc.numOptions)
			if got != tc.want {
				t.Errorf("initialHighlight(%d, %d) = %d; want %d",
					tc.defaultIdx, tc.numOptions, got, tc.want)
			}
		})
	}
}

// TestNextMenuState_EmptyOptions covers the edge case where ShowMenu is
// invoked with no options — the state machine MUST NOT panic and SHOULD
// keep the highlight pinned to Cancel on navigation (Acceptance A-031).
func TestNextMenuState_EmptyOptions(t *testing.T) {
	st := menuState{highlight: 0, numOptions: 0, defaultIdx: -1}

	for _, key := range []keyEvent{
		{kind: keyUp}, {kind: keyDown},
	} {
		got := nextMenuState(st, key)
		if got.highlight != 0 || got.submitted {
			t.Errorf("nextMenuState empty options with %v = %+v; want {hl:0, submit:false}", key, got)
		}
	}

	// Enter still submits the (Cancel) row.
	got := nextMenuState(st, keyEvent{kind: keyEnter})
	if got.highlight != 0 || !got.submitted {
		t.Errorf("Enter on empty-options menu = %+v; want {hl:0, submit:true}", got)
	}
}

// =============================================================================
// parseKey — escape-sequence parser tests (T013)
// =============================================================================

// fakeByteReader is a deterministic byteReader for parser tests. It returns
// queued bytes in order; once the queue is drained, subsequent reads time
// out (simulating the "no follow-up byte" case for bare Esc).
type fakeByteReader struct {
	queue []byte
}

func (f *fakeByteReader) readByteWithin(_ time.Duration) (byte, bool) {
	if len(f.queue) == 0 {
		return 0, false
	}
	bt := f.queue[0]
	f.queue = f.queue[1:]
	return bt, true
}

func TestParseKey_SingleByte(t *testing.T) {
	cases := []struct {
		name string
		in   byte
		want keyEventKind
		dig  int // valid only when want == keyDigit
	}{
		{"Enter (\\r)", '\r', keyEnter, 0},
		{"Enter (\\n)", '\n', keyEnter, 0},
		{"Ctrl-C (\\x03)", 0x03, keyCancel, 0},
		{"q → Cancel", 'q', keyCancel, 0},
		{"j → Down", 'j', keyDown, 0},
		{"k → Up", 'k', keyUp, 0},
		{"digit 0", '0', keyDigit, 0},
		{"digit 1", '1', keyDigit, 1},
		{"digit 5", '5', keyDigit, 5},
		{"digit 9", '9', keyDigit, 9},
		{"Tab → Ignore", '\t', keyIgnore, 0},
		{"Backspace (DEL) → Ignore", 0x7f, keyIgnore, 0},
		{"letter 'a' → Ignore", 'a', keyIgnore, 0},
		{"letter 'Z' → Ignore", 'Z', keyIgnore, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := parseKey(tc.in, &fakeByteReader{})
			if got.kind != tc.want {
				t.Errorf("parseKey(%q) = %v; want %v", tc.in, got.kind, tc.want)
			}
			if got.kind == keyDigit && got.digit != tc.dig {
				t.Errorf("parseKey(%q).digit = %d; want %d", tc.in, got.digit, tc.dig)
			}
		})
	}
}

func TestParseKey_ArrowSequences(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want keyEventKind
	}{
		{"\\x1b[A → Up", []byte{'[', 'A'}, keyUp},
		{"\\x1b[B → Down", []byte{'[', 'B'}, keyDown},
		{"\\x1b[C (right arrow) → Ignore", []byte{'[', 'C'}, keyIgnore},
		{"\\x1b[D (left arrow) → Ignore", []byte{'[', 'D'}, keyIgnore},
		{"\\x1b[5~ (Page Up) → Ignore (and discards payload)", []byte{'[', '5'}, keyIgnore},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeByteReader{queue: append([]byte{}, tc.buf...)}
			got := parseKey(byteEsc, r)
			if got.kind != tc.want {
				t.Errorf("parseKey(ESC + %v) = %v; want %v", tc.buf, got.kind, tc.want)
			}
		})
	}
}

// TestParseKey_BareEscTimesOutToCancel asserts that a lone ESC byte with no
// follow-up within the escTimeoutMs window resolves to keyCancel (spec
// Scenario "Bare Esc parses to Cancel after timeout"). The fakeByteReader
// returns (0, false) on an empty queue regardless of the timeout parameter,
// which is exactly the behavior the live blockingByteReader exhibits when
// no byte arrives within the deadline — so this test exercises the contract
// without depending on real wall-clock time.
func TestParseKey_BareEscTimesOutToCancel(t *testing.T) {
	got := parseKey(byteEsc, &fakeByteReader{})
	if got.kind != keyCancel {
		t.Errorf("parseKey(bare ESC, empty reader) = %v; want keyCancel", got.kind)
	}
}

// TestParseKey_EscThenF1IsIgnored covers \x1bOP (F1) per spec Scenario
// "Unknown sequence parses to Ignore".
func TestParseKey_EscThenF1IsIgnored(t *testing.T) {
	r := &fakeByteReader{queue: []byte{'O', 'P'}}
	got := parseKey(byteEsc, r)
	if got.kind != keyIgnore {
		t.Errorf("parseKey(\\x1bOP) = %v; want keyIgnore", got.kind)
	}
}

// TestParseKey_EscWithCSIButTimeoutOnThirdByte covers the partial-sequence
// case where \x1b[ arrives but the selector byte never does. The parser
// must NOT treat this as Cancel (the user did press the start of an arrow
// sequence) — Ignore is the correct outcome since we can't tell which arrow
// was intended.
func TestParseKey_EscWithCSIButNoSelector(t *testing.T) {
	r := &fakeByteReader{queue: []byte{'['}}
	got := parseKey(byteEsc, r)
	if got.kind != keyIgnore {
		t.Errorf("parseKey(\\x1b[ with no selector) = %v; want keyIgnore", got.kind)
	}
}

// =============================================================================
// blockingByteReader — timeout behavior (T013 supporting)
// =============================================================================

// TestBlockingByteReader_ReadByteWithin verifies the live reader honors the
// timeout. Pipe stays empty → the call should return (0, false) shortly
// after the timeout. We allow a generous upper bound to keep the test stable
// on busy CI machines.
func TestBlockingByteReader_ReadByteWithin_Timeout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	br := newBlockingByteReader(r)
	start := time.Now()
	_, ok := br.readByteWithin(20 * time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Fatalf("readByteWithin on empty pipe returned ok=true")
	}
	if elapsed < 15*time.Millisecond {
		t.Errorf("readByteWithin returned too early: %v (expected ≥ ~20ms)", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("readByteWithin took unreasonably long: %v", elapsed)
	}
}

// TestBlockingByteReader_ReadByteWithin_ImmediateByte verifies that when a
// byte is available, readByteWithin returns it without waiting for the
// timeout.
func TestBlockingByteReader_ReadByteWithin_ImmediateByte(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	if _, err := w.Write([]byte{'X'}); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	br := newBlockingByteReader(r)
	bt, ok := br.readByteWithin(100 * time.Millisecond)
	if !ok {
		t.Fatalf("readByteWithin returned ok=false with a byte available")
	}
	if bt != 'X' {
		t.Errorf("readByteWithin = %q; want 'X'", bt)
	}
}

// =============================================================================
// Fallback path — byte-identical output tests (T014)
// =============================================================================

// withStdin replaces os.Stdin with a pipe whose contents are `input`, runs
// fn, then restores os.Stdin. Captures os.Stdout during fn and returns the
// captured text.
//
// This is the seam the fallback-path tests use to assert that piped stdin
// always routes through runFallbackMenu (since the pipe is NOT a TTY) and
// to capture the exact bytes emitted to stdout.
func withPipedStdinCapturedStdout(t *testing.T, input string, fn func() (int, error)) (string, int, error) {
	t.Helper()

	// stdin pipe
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe (stdin): %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = stdinR
	defer func() {
		os.Stdin = origStdin
		stdinR.Close()
	}()
	go func() {
		defer stdinW.Close()
		_, _ = stdinW.Write([]byte(input))
	}()

	// stdout pipe
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe (stdout): %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = stdoutW

	var (
		buf  bytes.Buffer
		wg   sync.WaitGroup
		done = make(chan struct{})
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, stdoutR)
		close(done)
	}()

	choice, runErr := fn()

	os.Stdout = origStdout
	stdoutW.Close()
	<-done
	wg.Wait()
	stdoutR.Close()

	return buf.String(), choice, runErr
}

// TestShowMenu_FallbackPath_PipedStdin asserts the spec Scenario "Piped
// stdin (e.g., test harness)" / Acceptance A-013: with stdin piped (non-TTY)
// the output is byte-identical to today's numbered-list rendering and the
// `Choice [N]:` prompt is emitted.
func TestShowMenu_FallbackPath_PipedStdin(t *testing.T) {
	withDisabledColors(t)

	out, choice, err := withPipedStdinCapturedStdout(t, "2\n", func() (int, error) {
		return ShowMenu("Open in:", []string{"cursor", "code", "open_here"}, 1)
	})
	if err != nil {
		t.Fatalf("ShowMenu error: %v", err)
	}
	if choice != 2 {
		t.Errorf("choice = %d; want 2", choice)
	}

	wantPrefix := "Open in:\n" +
		"  1) cursor (default)\n" +
		"  2) code\n" +
		"  3) open_here\n" +
		"  0) Cancel\n" +
		"\n" +
		"Choice [1]: "
	if !strings.HasPrefix(out, wantPrefix) {
		t.Errorf("fallback output prefix mismatch.\n--- got ---\n%s\n--- want prefix ---\n%s", out, wantPrefix)
	}
}

// TestShowMenu_FallbackPath_InvalidInputMessage covers Acceptance A-013 and
// Scenario "Invalid input message preserved": the validation error string
// `Invalid choice. Please enter a number.` MUST appear verbatim when the
// user types non-numeric input.
func TestShowMenu_FallbackPath_InvalidInputMessage(t *testing.T) {
	withDisabledColors(t)

	// First entry is junk → triggers the "Please enter a number." error.
	// Second entry is "1\n" so the loop terminates cleanly.
	out, choice, err := withPipedStdinCapturedStdout(t, "xyz\n1\n", func() (int, error) {
		return ShowMenu("Pick:", []string{"a", "b"}, -1)
	})
	if err != nil {
		t.Fatalf("ShowMenu error: %v", err)
	}
	if choice != 1 {
		t.Errorf("choice = %d; want 1", choice)
	}
	if !strings.Contains(out, "Invalid choice. Please enter a number.") {
		t.Errorf("output missing 'Invalid choice. Please enter a number.':\n%s", out)
	}
}

// TestShowMenu_FallbackPath_OutOfRangeMessage covers the second validation
// message `Invalid choice. Please enter a number between 0 and N.` for
// numeric-but-out-of-range input.
func TestShowMenu_FallbackPath_OutOfRangeMessage(t *testing.T) {
	withDisabledColors(t)

	// 7 is out-of-range for a 2-option menu (valid: 0, 1, 2). Follow with 0
	// so the menu cancels cleanly.
	out, choice, err := withPipedStdinCapturedStdout(t, "7\n0\n", func() (int, error) {
		return ShowMenu("Pick:", []string{"a", "b"}, -1)
	})
	if err != nil {
		t.Fatalf("ShowMenu error: %v", err)
	}
	if choice != 0 {
		t.Errorf("choice = %d; want 0 (Cancel)", choice)
	}
	want := "Invalid choice. Please enter a number between 0 and 2."
	if !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

// TestShowMenu_FallbackPath_NoDefaultPrompt verifies the "Choice: " prompt
// (no `[N]` bracket) when defaultIdx == -1.
func TestShowMenu_FallbackPath_NoDefaultPrompt(t *testing.T) {
	withDisabledColors(t)

	out, _, err := withPipedStdinCapturedStdout(t, "1\n", func() (int, error) {
		return ShowMenu("Pick:", []string{"a"}, -1)
	})
	if err != nil {
		t.Fatalf("ShowMenu error: %v", err)
	}
	if !strings.Contains(out, "Choice: ") {
		t.Errorf("output should contain 'Choice: ' (no default prompt):\n%s", out)
	}
	if strings.Contains(out, "Choice [") {
		t.Errorf("output should NOT contain 'Choice [' when defaultIdx is -1:\n%s", out)
	}
}

// TestShowMenu_FallbackPath_EmptyInputUsesDefault asserts that pressing
// Enter at the `Choice [N]:` prompt with a non-negative defaultIdx returns
// the default — preserves the historical fallback behavior.
func TestShowMenu_FallbackPath_EmptyInputUsesDefault(t *testing.T) {
	withDisabledColors(t)

	_, choice, err := withPipedStdinCapturedStdout(t, "\n", func() (int, error) {
		return ShowMenu("Pick:", []string{"a", "b"}, 2)
	})
	if err != nil {
		t.Fatalf("ShowMenu error: %v", err)
	}
	if choice != 2 {
		t.Errorf("choice on empty input with default=2 = %d; want 2", choice)
	}
}

// TestShowMenu_FallbackPath_EOFNoInputActionableError asserts principles №1/№4:
// when stdin is not a TTY and reaches EOF with no choice entered (the piped-
// empty / non-interactive case), the fallback menu refuses with a non-nil,
// actionable error naming the escape (a worktree name or --non-interactive) —
// NOT the bare "reading input: EOF", and never a hang. This is the single choke
// point that covers wt open (main-repo menu), wt go (no name), and wt delete
// (no name) at once.
func TestShowMenu_FallbackPath_EOFNoInputActionableError(t *testing.T) {
	withDisabledColors(t)

	// Empty input, no newline → ReadString returns io.EOF with an empty line.
	_, choice, err := withPipedStdinCapturedStdout(t, "", func() (int, error) {
		return ShowMenu("Pick:", []string{"a", "b"}, 1)
	})
	if err == nil {
		t.Fatalf("expected a non-nil error on EOF with no input; got nil (choice=%d)", choice)
	}
	msg := err.Error()
	// The bare cause must NOT be surfaced verbatim.
	if strings.Contains(msg, "reading input: EOF") {
		t.Errorf("error should be actionable, not the bare %q:\n%s", "reading input: EOF", msg)
	}
	// It must carry the structured what/why/fix shape and name the escape.
	for _, want := range []string{"Error:", "Why:", "Fix:", "end of input", "--non-interactive"} {
		if !strings.Contains(msg, want) {
			t.Errorf("actionable EOF error missing %q:\n%s", want, msg)
		}
	}
}

// TestShowMenu_FallbackPath_PartialLineNoNewlineAtEOF asserts that a valid
// choice typed WITHOUT a trailing newline before EOF is still honored (ReadString
// returns the bytes-so-far alongside io.EOF) — the EOF refusal fires only when
// there is genuinely no pending input.
func TestShowMenu_FallbackPath_PartialLineNoNewlineAtEOF(t *testing.T) {
	withDisabledColors(t)

	_, choice, err := withPipedStdinCapturedStdout(t, "0", func() (int, error) {
		return ShowMenu("Pick:", []string{"a", "b"}, 1)
	})
	if err != nil {
		t.Fatalf("a choice without a trailing newline should be honored, got error: %v", err)
	}
	if choice != 0 {
		t.Errorf("choice = %d; want 0 (Cancel) from partial-line '0' at EOF", choice)
	}
}

// =============================================================================
// runInteractiveMenuCore — panic-restore seam (T017 / A-011 / A-028)
// =============================================================================

// panickingReader is an io.Reader whose first Read call panics. It is used to
// drive runInteractiveMenuCore through a runtime panic so the test can assert
// that the deferred restoreFn fires before the panic unwinds the call stack.
type panickingReader struct{}

func (panickingReader) Read(_ []byte) (int, error) {
	panic("synthetic panic from panickingReader")
}

// TestRunInteractiveMenuCore_PanicRestore asserts that when the byte-read
// step panics mid-loop:
//
//	(a) The panic propagates out of runInteractiveMenuCore (caller-visible).
//	(b) The deferred restoreFn was invoked exactly once before unwind, so
//	    cooked mode would be reinstated in production.
//
// Together these resolve Acceptance A-011 ("Raw-mode restore … verified by a
// test") and A-028 ("Panic in read loop … assert the deferred term.Restore
// fires"). The seam is internal to the worktree package — ShowMenu's public
// signature is unaffected.
func TestRunInteractiveMenuCore_PanicRestore(t *testing.T) {
	var restoreCount int
	restoreFn := func() { restoreCount++ }

	// The initial paint happens before the read loop, so we discard its
	// output rather than asserting on it (paintMenu is covered by the
	// existing fallback / nextMenuState tests).
	var stdout bytes.Buffer

	// Recover the panic in this test body so we can assert on both the
	// propagation AND the restoreFn invocation. Without recover, the test
	// would crash and we'd lose the restoreCount observation.
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		// Use a panic-on-read source wrapped in the shared reader type; its
		// pump goroutine's first ReadByte triggers the panic, which the pump
		// recovers and forwards so the consumer re-raises it on this goroutine.
		_, _ = runInteractiveMenuCore(
			&stdout,
			newBlockingByteReader(panickingReader{}),
			"Pick:",
			[]string{"a", "b"},
			-1,
			restoreFn,
			func() int { return 24 },
		)
	}()

	if recovered == nil {
		t.Fatalf("expected panic to propagate out of runInteractiveMenuCore; got none (restoreCount=%d)", restoreCount)
	}
	if msg, ok := recovered.(string); ok {
		if !strings.Contains(msg, "synthetic panic from panickingReader") {
			t.Errorf("recovered panic value = %q; want the synthetic message", msg)
		}
	}
	if restoreCount != 1 {
		t.Errorf("restoreFn invoked %d time(s); want exactly 1 (deferred restore must fire on panic unwind)", restoreCount)
	}
}

// =============================================================================
// paintMenu / redrawMenu — shared row-rendering core (T018)
// =============================================================================

// TestPaintAndRedrawShareCore asserts that paintMenu and redrawMenu emit the
// same row content for the same menuState. redrawMenu adds a leading
// cursor-up prelude and a per-line \x1b[2K clear; once those are stripped, the
// resulting row bytes MUST match paintMenu's output byte-for-byte. This
// guards the renderRows refactor (T018) against future drift.
//
// A deliberately small height (6) forces the scrolling viewport on: the option
// budget is height-menuOverheadRows-1 = 3, and with 5 options windowed at top 1
// the layout emits both ↑/↓ indicators plus one visible option row — so the
// byte-equality property is asserted on the windowed render path, not just the
// fits-on-screen path.
func TestPaintAndRedrawShareCore(t *testing.T) {
	withDisabledColors(t)

	const height = 6 // budget height-menuOverheadRows-1 = 3 < 5 options → windowed
	st := menuState{highlight: 3, numOptions: 5, defaultIdx: 1, top: 1}
	prompt := "Open in:"
	options := []string{"cursor", "code", "open_here", "split", "tab"}

	var paintBuf, redrawBuf bytes.Buffer
	paintRows := paintMenu(&paintBuf, prompt, options, st, height)
	redrawRows := redrawMenu(&redrawBuf, prompt, options, st, paintRows, height)

	if paintRows != redrawRows {
		t.Errorf("row count diverged: paintMenu=%d redrawMenu=%d", paintRows, redrawRows)
	}
	// The rendered region reserves one row (every row ends \r\n), so the sound
	// footprint is rowsRendered <= height-1; a full-height paint would scroll.
	if paintRows > height-1 {
		t.Errorf("windowed paint rendered %d rows, exceeds sound bound height-1 = %d", paintRows, height-1)
	}

	// Strip redraw's cursor-up prelude (\r + \x1b[<N>A) and every per-line
	// \x1b[2K clear — what's left must equal paintMenu's output exactly.
	redrawOut := redrawBuf.String()
	cursorUp := ansiCarriageRet + fmt.Sprintf(ansiCursorUpFmt, paintRows)
	if !strings.HasPrefix(redrawOut, cursorUp) {
		t.Fatalf("redrawMenu output missing cursor-up prelude.\n--- got ---\n%q\n--- want prefix ---\n%q", redrawOut, cursorUp)
	}
	redrawOut = strings.TrimPrefix(redrawOut, cursorUp)
	redrawOut = strings.ReplaceAll(redrawOut, ansiClearLine, "")

	if redrawOut != paintBuf.String() {
		t.Errorf("paintMenu and redrawMenu emit different row content after stripping redraw's prelude/clears.\n--- paint ---\n%q\n--- redraw (stripped) ---\n%q",
			paintBuf.String(), redrawOut)
	}
}

// TestRedrawMenu_ClearsExtraLinesOnShrink asserts redrawMenu's shrink hardening:
// when the previous region was taller than the freshly-painted one (e.g. the
// terminal was made smaller mid-menu, so height re-queried at redraw yields a
// smaller window), redrawMenu clears the trailing (old-new) stale lines instead
// of leaving ghosts until finalize. Resize is a declared non-goal; this guards
// the minimal robustness fix. PTY-free — it inspects the emitted byte stream.
func TestRedrawMenu_ClearsExtraLinesOnShrink(t *testing.T) {
	withDisabledColors(t)

	options := make([]string, 20)
	for i := range options {
		options[i] = fmt.Sprintf("opt%d", i+1)
	}
	st := menuState{highlight: 1, numOptions: len(options), defaultIdx: -1, top: 0}

	// Redraw with a small height (fewer visible rows now) while telling redraw
	// the region previously occupied many more rows (the pre-shrink count).
	const smallHeight = 6
	newRows := renderRows(io.Discard, "Pick:", options, st, "", smallHeight)
	const prevRows = 20 // taller region that was on screen before the shrink

	var buf bytes.Buffer
	got := redrawMenu(&buf, "Pick:", options, st, prevRows, smallHeight)
	if got != newRows {
		t.Fatalf("redrawMenu returned %d rows; want the windowed count %d", got, newRows)
	}

	out := buf.String()
	extra := prevRows - newRows
	if extra <= 0 {
		t.Fatalf("test setup expected a shrink (prevRows %d > newRows %d)", prevRows, newRows)
	}

	// There must be exactly newRows clears for the repaint plus `extra` clears
	// for the stale trailing lines — one \x1b[2K per cleared line.
	wantClears := newRows + extra
	gotClears := strings.Count(out, ansiClearLine)
	if gotClears != wantClears {
		t.Errorf("redrawMenu emitted %d line clears; want %d (newRows %d + extra %d)",
			gotClears, wantClears, newRows, extra)
	}

	// After clearing the extra lines, the cursor is moved back up by `extra`
	// so the returned newRows stays an accurate cursor-up target for the next
	// redraw. Assert that final cursor-up is present.
	wantCursorBack := fmt.Sprintf(ansiCursorUpFmt, extra)
	if !strings.Contains(out, wantCursorBack) {
		t.Errorf("redrawMenu did not move the cursor back up by the extra %d lines (want %q):\n%q",
			extra, wantCursorBack, out)
	}
}

// =============================================================================
// menuLayout — scrolling-viewport state machine (windowing)
// =============================================================================

// TestMenuLayout exercises the pure windowing function across the edge cases
// called out in the intake: window at top (no ↑), window at bottom (no ↓),
// both indicators mid-list, highlight-driven shifts up/down, wrap-around jumps,
// a terminal shorter than the overhead, and a 0-option menu. All PTY-free.
func TestMenuLayout(t *testing.T) {
	cases := []struct {
		name                                   string
		numOptions, highlight, prevTop, height int
		wantTop, wantFirst, wantCount          int
		wantMoreAbove, wantMoreBelow           int
	}{
		// Option budget is height-menuOverheadRows-1 (prompt + Cancel + one
		// reserved row that keeps rowsRendered <= height-1 so the region never
		// scrolls the terminal on repaint).

		// --- Fits entirely: no windowing, no indicators ---
		{"fits: 3 options in tall terminal", 3, 1, 0, 24, 0, 0, 3, 0, 0},
		{"fits exactly: options == budget", 5, 3, 0, 8, 0, 0, 5, 0, 0},
		{"single option fits", 1, 1, 0, 24, 0, 0, 1, 0, 0},

		// --- Window at top: only ↓ indicator ---
		// height 10 → budget 7; 20 options, highlight near top, top 0.
		{"window at top → only ↓ more", 20, 1, 0, 10, 0, 0, 6, 0, 14},

		// --- Window at bottom: only ↑ indicator ---
		// highlight on last option → window jumps to end, no ↓.
		{"window at bottom → only ↑ more", 20, 20, 0, 10, 14, 14, 6, 14, 0},

		// --- Both indicators mid-list ---
		// highlight in the middle, window already scrolled → both edges hidden.
		{"both indicators mid-list", 20, 10, 8, 10, 8, 8, 5, 8, 7},

		// --- Honest windowed cases at heights 4 and 5 (the rework-cycle-2 boundary) ---
		// budget = height-menuOverheadRows-1: height 5 → 2, height 4 → 1. A mid-list
		// window would want BOTH indicators, but the budget cannot fit indicator
		// chrome plus an option, so the chrome is dropped (moreAbove/moreBelow report
		// 0 even though options are hidden) and count == budget. These cases would
		// FAIL the pre-rework code (which fabricated a row via the v<1→v=1 clamp,
		// rendering 5 rows at height 5 = full-height scroll, and 5 rows > height at
		// height 4). The ≤ height-1 invariant guard below now asserts them.
		{"height 5 mid-list drops chrome to fit budget", 20, 10, 8, 5, 9, 9, 1, 9, 0},
		{"height 4 mid-list drops both indicators", 20, 10, 8, 4, 9, 9, 1, 0, 0},
		{"height 5 window at top keeps ↓, drops nothing extra", 20, 1, 0, 5, 0, 0, 1, 0, 19},
		{"height 5 window at bottom keeps ↑", 20, 20, 0, 5, 19, 19, 1, 19, 0},
		{"height 4 window at top drops chrome", 20, 1, 0, 4, 0, 0, 1, 0, 0},

		// --- Highlight-driven shift down ---
		// highlight past the bottom of the current window forces top down until
		// the highlight is the last visible row.
		{"shift down to reveal highlight", 20, 8, 0, 10, 3, 3, 5, 3, 12},

		// --- Highlight-driven shift up ---
		// highlight above the current window forces top down.
		{"shift up to reveal highlight", 20, 3, 8, 10, 2, 2, 5, 2, 13},

		// --- Wrap-around jump: highlight at last option, window was at top ---
		{"wrap jump to last option", 30, 30, 0, 12, 22, 22, 8, 22, 0},

		// --- Cancel highlight does not constrain the option window ---
		{"cancel highlight keeps prevTop window", 20, 0, 5, 10, 5, 5, 5, 5, 10},

		// --- Degenerate: terminal shorter than overhead ---
		// budget is clamped up to 1, which cannot spare a row for chrome, so the
		// indicators are dropped (moreBelow reports 0 though options are hidden).
		// The ≥1-option escape hatch keeps output non-empty; these heights are
		// exempt from the ≤ height-1 invariant guard below (see its height gate).
		{"terminal shorter than overhead clamps to 1 option", 20, 1, 0, 1, 0, 0, 1, 0, 0},
		{"terminal exactly overhead clamps to 1 option", 20, 1, 0, 2, 0, 0, 1, 0, 0},

		// --- Degenerate: 0-option menu ---
		{"zero options", 0, 0, 0, 24, 0, 0, 0, 0, 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			top, first, count, moreAbove, moreBelow := menuLayout(tc.numOptions, tc.highlight, tc.prevTop, tc.height)
			if top != tc.wantTop || first != tc.wantFirst || count != tc.wantCount ||
				moreAbove != tc.wantMoreAbove || moreBelow != tc.wantMoreBelow {
				t.Errorf("menuLayout(n=%d, hl=%d, prevTop=%d, h=%d) = (top=%d first=%d count=%d above=%d below=%d); want (top=%d first=%d count=%d above=%d below=%d)",
					tc.numOptions, tc.highlight, tc.prevTop, tc.height,
					top, first, count, moreAbove, moreBelow,
					tc.wantTop, tc.wantFirst, tc.wantCount, tc.wantMoreAbove, tc.wantMoreBelow)
			}

			// Invariant: the highlighted OPTION (1..numOptions) is always inside
			// the returned window. (Cancel — highlight 0 — is not an option row.)
			if tc.highlight >= 1 && count > 0 {
				h := tc.highlight - 1
				if h < first || h >= first+count {
					t.Errorf("highlighted option index %d not visible in window [%d,%d)", h, first, first+count)
				}
			}

			// Invariant: total rendered rows never exceed height-1 when windowing
			// is active (i.e. not everything fits). Every row ends \r\n, so the
			// region must leave one row unrendered or the trailing newline on the
			// bottom screen row scrolls the terminal on every repaint (the bug
			// this reserved row fixes). The rendered region is prompt + indicators
			// + visible options + Cancel.
			if tc.numOptions > 0 {
				indicators := 0
				if moreAbove > 0 {
					indicators++
				}
				if moreBelow > 0 {
					indicators++
				}
				rendered := 1 + indicators + count + 1 // prompt + indicators + options + Cancel
				// The honest footprint boundary is height >= 4 (== menuOverheadRows+2,
				// the smallest height whose raw budget height-menuOverheadRows-1 is
				// >= 1). At and above it the drop-chrome logic (menuLayout sacrifices
				// indicator rows the budget cannot afford) guarantees
				// indicators + count <= budget, so rendered <= height-1 — verified by
				// the honest windowed height-4/5 cases above (which would fail the
				// pre-rework code). Below height 4 the raw budget is < 1 and is
				// clamped to 1; the ≥1-option escape hatch keeps output bounded and
				// non-empty but necessarily taller than height-1 on such a
				// pathological sub-overhead terminal, so those heights are exempt.
				if tc.height >= menuOverheadRows+2 && rendered > tc.height-1 {
					t.Errorf("rendered %d rows exceeds sound bound height-1 = %d (top=%d count=%d above=%d below=%d)",
						rendered, tc.height-1, top, count, moreAbove, moreBelow)
				}
			}
		})
	}
}

// TestMenuLayout_FootprintSweep is the brute-force guard the reviewers used to
// find the rework-cycle-2 bug: it sweeps every (height, numOptions, highlight,
// prevTop) combination in a small grid and asserts the two load-bearing
// invariants on menuLayout's output —
//
//  1. Footprint: the rendered region (prompt + indicators + visible options +
//     Cancel) stays <= height-1 for every height >= 4 (the honest boundary).
//     Every row ends \r\n, so a region of exactly `height` rows scrolls the
//     terminal one line per repaint — the original bug. Heights 1-3 are exempt:
//     the raw budget is < 1, clamped up to 1, so the ≥1-option escape hatch
//     keeps output bounded but necessarily taller than height-1.
//  2. Highlight visibility: the highlighted option (1..numOptions) is always
//     inside the returned window, even after chrome is dropped.
//
// It is exhaustive over the grid rather than table-driven so a regression at ANY
// height/size/highlight/prevTop is caught, not just the hand-picked rows in
// TestMenuLayout. PTY-free and cheap (pure arithmetic).
func TestMenuLayout_FootprintSweep(t *testing.T) {
	for height := 1; height <= 12; height++ {
		for n := 1; n <= 25; n++ {
			for highlight := 0; highlight <= n; highlight++ {
				for prevTop := 0; prevTop < n; prevTop++ {
					top, first, count, above, below := menuLayout(n, highlight, prevTop, height)

					// Invariant 1: footprint <= height-1 for height >= 4.
					indicators := 0
					if above > 0 {
						indicators++
					}
					if below > 0 {
						indicators++
					}
					rendered := 1 + indicators + count + 1
					if height >= menuOverheadRows+2 && rendered > height-1 {
						t.Fatalf("footprint overshoot: menuLayout(n=%d, hl=%d, prevTop=%d, h=%d) => top=%d count=%d above=%d below=%d rendered=%d > height-1=%d",
							n, highlight, prevTop, height, top, count, above, below, rendered, height-1)
					}

					// Invariant 2: highlighted option always visible.
					if highlight >= 1 && count > 0 {
						h := highlight - 1
						if h < first || h >= first+count {
							t.Fatalf("highlight invisible: menuLayout(n=%d, hl=%d, prevTop=%d, h=%d) => window [%d,%d) excludes option index %d",
								n, highlight, prevTop, height, first, first+count, h)
						}
					}

					// Sanity: never report a hidden-count > 0 without leaving room,
					// never overrun the list, always show >=1 option where options exist.
					if count < 1 {
						t.Fatalf("count < 1 with options present: menuLayout(n=%d, hl=%d, prevTop=%d, h=%d) => count=%d", n, highlight, prevTop, height, count)
					}
					if first+count > n {
						t.Fatalf("window overruns list: menuLayout(n=%d, hl=%d, prevTop=%d, h=%d) => first=%d count=%d", n, highlight, prevTop, height, first, count)
					}
				}
			}
		}
	}
}

// TestMenuLayout_WrapJumpFromTopToBottom simulates the reported wrap-around bug
// scenario: a long list windowed at the top, highlight on option 1, user presses
// ↑ (wraps to Cancel) then ↑ again (to the last option). The window must jump to
// the bottom so the last option is visible — the arrow-key reachability the
// viewport change restores.
func TestMenuLayout_WrapJumpFromTopToBottom(t *testing.T) {
	const numOptions, height = 20, 10
	// Start: highlight on option 1, window at top.
	top := 0
	top, _, _, _, _ = menuLayout(numOptions, 1, top, height)
	if top != 0 {
		t.Fatalf("initial window should be at top, got top=%d", top)
	}
	// ↑ wraps to Cancel (highlight 0) — window unchanged (Cancel is chrome).
	top, _, _, _, _ = menuLayout(numOptions, 0, top, height)
	if top != 0 {
		t.Fatalf("Cancel highlight should not move the window, got top=%d", top)
	}
	// ↑ again wraps to the last option (20) — window must jump to the bottom.
	top, first, count, above, below := menuLayout(numOptions, numOptions, top, height)
	if below != 0 {
		t.Errorf("with last option highlighted, ↓ indicator should be gone; moreBelow=%d", below)
	}
	if above == 0 {
		t.Errorf("with the window jumped to the bottom, ↑ indicator should show; moreAbove=%d", above)
	}
	lastIdx := numOptions - 1
	if lastIdx < first || lastIdx >= first+count {
		t.Errorf("last option index %d not visible after wrap jump: window [%d,%d)", lastIdx, first, first+count)
	}
	if top != lastIdx-count+1 {
		t.Errorf("window not anchored to bottom: top=%d, expected %d", top, lastIdx-count+1)
	}
}

// =============================================================================
// terminalHeight — GetSize-failure fallback (24-row default)
// =============================================================================

// TestTerminalHeightFallback asserts the intake's 24-row fallback contract:
// when the height source fails, windowing must use defaultTerminalHeight rather
// than degrading to an unbounded (unwindowed) render. The production
// terminalHeight() calls term.GetSize on stdout, which errors here because the
// test's stdout is not a TTY — exercising the real fallback branch.
func TestTerminalHeightFallback(t *testing.T) {
	// Under `go test`, stdout is a pipe (not a TTY), so term.GetSize errors and
	// terminalHeight() must return the classic 24-row default. Only assert the
	// fallback when GetSize actually failed — guards the rare case of running
	// the test attached to a real terminal, where GetSize would succeed.
	_, _, err := term.GetSize(int(os.Stdout.Fd()))
	got := terminalHeight()
	if err != nil {
		if got != defaultTerminalHeight {
			t.Errorf("terminalHeight() on GetSize failure = %d; want %d", got, defaultTerminalHeight)
		}
	} else if got < 1 {
		t.Errorf("terminalHeight() on GetSize success = %d; want a positive height", got)
	}

	// And the layout consuming a 24-row height stays bounded for a long list.
	_, _, count, above, below := menuLayout(100, 1, 0, defaultTerminalHeight)
	rendered := 1 + count + 1 // prompt + visible options + Cancel
	if above > 0 {
		rendered++
	}
	if below > 0 {
		rendered++
	}
	if rendered > defaultTerminalHeight {
		t.Errorf("layout at 24-row fallback rendered %d rows, exceeds %d", rendered, defaultTerminalHeight)
	}
}

// =============================================================================
// renderRows / paintMenu — windowed row slicing + overflow indicators
// =============================================================================

// TestRenderRows_WindowedSlicing verifies that a long option list rendered in a
// short terminal shows only the windowed options plus ↑/↓ indicator rows with
// the correct hidden counts, and that the row count stays within the height.
func TestRenderRows_WindowedSlicing(t *testing.T) {
	withDisabledColors(t)

	options := []string{"one", "two", "three", "four", "five", "six", "seven", "eight"}
	const height = 6 // budget height-menuOverheadRows-1 = 3; 8 options → windowed

	// Highlight in the middle so both indicators can appear. top=2 keeps the
	// window scrolled off both edges.
	st := menuState{highlight: 4, numOptions: len(options), defaultIdx: -1, top: 2}

	var buf bytes.Buffer
	rows := renderRows(&buf, "Pick:", options, st, "", height)
	out := buf.String()

	if rows > height {
		t.Errorf("renderRows returned %d rows, exceeds height %d", rows, height)
	}

	// Confirm the computed layout so the string assertions below are grounded.
	_, first, count, above, below := menuLayout(len(options), st.highlight, st.top, height)
	if above == 0 || below == 0 {
		t.Fatalf("test setup expected both indicators; got above=%d below=%d", above, below)
	}

	// Indicator rows present with correct counts.
	wantUp := fmt.Sprintf("%s %d more", indicatorUp, above)
	wantDown := fmt.Sprintf("%s %d more", indicatorDown, below)
	if !strings.Contains(out, wantUp) {
		t.Errorf("output missing up indicator %q:\n%s", wantUp, out)
	}
	if !strings.Contains(out, wantDown) {
		t.Errorf("output missing down indicator %q:\n%s", wantDown, out)
	}

	// Visible options appear by label; hidden ones do not. Labels are unique
	// and never overlap the indicator text, so a plain substring check is a
	// reliable visibility probe even for the highlighted row (whose number is
	// wrapped in reverse-video SGR codes that withDisabledColors does not
	// strip — only the color constants are zeroed, not the highlight visuals).
	for i := 0; i < len(options); i++ {
		visible := i >= first && i < first+count
		if visible && !strings.Contains(out, options[i]) {
			t.Errorf("expected visible option %q in output:\n%s", options[i], out)
		}
		if !visible && strings.Contains(out, options[i]) {
			t.Errorf("hidden option %q should not appear in output:\n%s", options[i], out)
		}
	}
}

// TestRenderRows_NoIndicatorsAtEdges verifies the window-at-top case shows no ↑
// indicator and the window-at-bottom case shows no ↓ indicator.
func TestRenderRows_NoIndicatorsAtEdges(t *testing.T) {
	withDisabledColors(t)

	options := make([]string, 12)
	for i := range options {
		options[i] = fmt.Sprintf("opt%d", i+1)
	}
	const height = 7 // budget height-menuOverheadRows-1 = 4

	// Window at top: highlight on option 1.
	var top bytes.Buffer
	renderRows(&top, "Pick:", options, menuState{highlight: 1, numOptions: len(options), defaultIdx: -1, top: 0}, "", height)
	if strings.Contains(top.String(), indicatorUp+" ") {
		t.Errorf("window-at-top output should NOT contain an ↑ indicator:\n%s", top.String())
	}
	if !strings.Contains(top.String(), indicatorDown+" ") {
		t.Errorf("window-at-top output SHOULD contain a ↓ indicator:\n%s", top.String())
	}

	// Window at bottom: highlight on the last option.
	var bottom bytes.Buffer
	renderRows(&bottom, "Pick:", options, menuState{highlight: len(options), numOptions: len(options), defaultIdx: -1, top: 0}, "", height)
	if !strings.Contains(bottom.String(), indicatorUp+" ") {
		t.Errorf("window-at-bottom output SHOULD contain an ↑ indicator:\n%s", bottom.String())
	}
	if strings.Contains(bottom.String(), indicatorDown+" ") {
		t.Errorf("window-at-bottom output should NOT contain a ↓ indicator:\n%s", bottom.String())
	}
}

// TestRenderRows_SmallMenuByteIdenticalToUnwindowed asserts the intake's
// "menus that fit render byte-identically to today" promise: a small menu in a
// tall terminal renders exactly the same bytes whether or not windowing exists
// (no indicators, all options present). We compare the windowed renderer's
// output at a generous height against a hand-built expected block matching the
// pre-change layout.
func TestRenderRows_SmallMenuByteIdenticalToUnwindowed(t *testing.T) {
	withDisabledColors(t)

	options := []string{"cursor", "code", "open_here"}
	st := menuState{highlight: 2, numOptions: 3, defaultIdx: 1, top: 0}

	var buf bytes.Buffer
	rows := renderRows(&buf, "Open in:", options, st, "", 24) // tall → fits

	// Expected block: exactly the pre-change layout — prompt, 3 option rows
	// (row 2 highlighted, row 1 the default), Cancel. No indicator rows.
	var want bytes.Buffer
	fmt.Fprint(&want, "Open in:\r\n")
	writeOptionRow(&want, 1, "cursor", true, false)
	writeOptionRow(&want, 2, "code", false, true)
	writeOptionRow(&want, 3, "open_here", false, false)
	writeCancelRow(&want, false, false)

	if buf.String() != want.String() {
		t.Errorf("small menu not byte-identical to unwindowed layout.\n--- got ---\n%q\n--- want ---\n%q", buf.String(), want.String())
	}
	if rows != len(options)+2 {
		t.Errorf("small menu rendered %d rows; want %d (prompt + %d options + Cancel)", rows, len(options)+2, len(options))
	}
}

// withDisabledColors zeroes the ANSI color constants for the test's
// duration so byte-comparison assertions are not perturbed by NO_COLOR
// state in the host environment. Restored via t.Cleanup.
func withDisabledColors(t *testing.T) {
	t.Helper()
	origRed, origYellow, origGreen, origBold, origReset := ColorRed, ColorYellow, ColorGreen, ColorBold, ColorReset
	ColorRed, ColorYellow, ColorGreen, ColorBold, ColorReset = "", "", "", "", ""
	t.Cleanup(func() {
		ColorRed, ColorYellow, ColorGreen, ColorBold, ColorReset = origRed, origYellow, origGreen, origBold, origReset
	})
}
