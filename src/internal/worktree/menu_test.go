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
func TestPaintAndRedrawShareCore(t *testing.T) {
	withDisabledColors(t)

	st := menuState{highlight: 2, numOptions: 3, defaultIdx: 1}
	prompt := "Open in:"
	options := []string{"cursor", "code", "open_here"}

	var paintBuf, redrawBuf bytes.Buffer
	paintRows := paintMenu(&paintBuf, prompt, options, st)
	redrawRows := redrawMenu(&redrawBuf, prompt, options, st, paintRows)

	if paintRows != redrawRows {
		t.Errorf("row count diverged: paintMenu=%d redrawMenu=%d", paintRows, redrawRows)
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
