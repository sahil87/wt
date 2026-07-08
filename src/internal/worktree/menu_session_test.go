package worktree

import (
	"io"
	"testing"
	"time"
)

// =============================================================================
// REGRESSION: sequential-menu byte theft (the `wt open` second-screen bug)
// =============================================================================
//
// `wt open` shows two interactive menus back to back: "Select worktree to
// open:" then "Open in:". The bug was that each ShowMenu built its OWN
// blockingByteReader over the same os.Stdin. Each reader's pump goroutine
// reads ONE BYTE AHEAD (the result channel is buffered with cap 1), so after
// the user submits menu 1 with Enter, menu 1's pump is parked inside a
// blocking ReadByte() on the shared stream — an orphan nobody cancels.
//
// When menu 2 started, its fresh reader competed with that orphan on the same
// stream. The orphan was parked first, so it STOLE menu 2's first keystroke:
// pressing Enter on the second screen did nothing until an arrow key (a
// multi-byte burst) spilled enough bytes for menu 2's reader to react.
//
// The fix (MenuSession) shares ONE reader across both Show() calls, so there
// is never a second reader to race — no keystroke is stolen.

// drainingReader keeps Read calls blocked until a byte is available and never
// returns EOF on a transient empty read, modeling a live terminal stream that
// two readers would contend over. Backed by a channel so the test controls
// exactly when each byte becomes available.
type sharedStream struct {
	bytes chan byte
}

func newSharedStream() *sharedStream { return &sharedStream{bytes: make(chan byte, 8)} }

func (s *sharedStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b, ok := <-s.bytes
	if !ok {
		return 0, io.EOF
	}
	p[0] = b
	return 1, nil
}

func (s *sharedStream) push(b byte) { s.bytes <- b }

// TestUnderlyingReadAhead_DemonstratesTheft documents WHY the bug existed: two
// independent blockingByteReaders over one stream steal each other's bytes
// because the first reads one byte ahead. This is a characterization test of
// the primitive — it is the failure the MenuSession fix routes around by never
// creating a second reader.
func TestUnderlyingReadAhead_DemonstratesTheft(t *testing.T) {
	stream := newSharedStream()

	// Reader 1 (menu 1). Consume its Enter; its pump then reads one byte ahead
	// and parks on the shared stream.
	reader1 := newBlockingByteReader(stream)
	stream.push('\r')
	if bt, ok := reader1.readByteBlocking(); !ok || bt != '\r' {
		t.Fatalf("reader1 = (%q,%v); want ('\\r',true)", bt, ok)
	}
	// Let reader1's pump re-enter its blocking read on the shared stream.
	time.Sleep(20 * time.Millisecond)

	// Reader 2 (menu 2). Push one byte for it — but reader1's orphan grabs it.
	reader2 := newBlockingByteReader(stream)
	stream.push('\r')

	got := make(chan bool, 1)
	go func() { _, ok := reader2.readByteBlocking(); got <- ok }()
	select {
	case <-got:
		t.Fatalf("expected reader2 to be starved by reader1's orphaned read; it received a byte instead")
	case <-time.After(200 * time.Millisecond):
		// Starved as expected — this is the bug the fix avoids by sharing one reader.
	}
}

// TestMenuSession_SharesReaderAcrossMenus is the regression guard for the fix.
// It drives two interactive menus through a single MenuSession over one shared
// stream — exactly the wt-open shape — and asserts the SECOND menu receives its
// Enter keystroke. Before the fix (a fresh reader per menu) the second menu's
// Enter was stolen and this would hang/starve.
func TestMenuSession_SharesReaderAcrossMenus(t *testing.T) {
	withDisabledColors(t)

	stream := newSharedStream()

	// Build a session that shares ONE reader over the stream, mirroring what
	// NewMenuSession does in interactive mode. We drive showInteractive (the
	// seam below Show's raw-mode setup) directly because the test stream is not
	// a real TTY, so term.MakeRaw would fail and Show would fall back to the
	// numbered prompt. This exercises the reader-sharing contract — the part
	// the byte-theft bug lived in. restoreFn is a no-op (no raw mode in tests).
	session := &MenuSession{
		interactive: true,
		reader:      newBlockingByteReader(stream),
	}
	noopRestore := func() {}

	type res struct {
		choice int
		err    error
	}

	// Menu 1: press Enter on the default row (defaultIdx 2 → submit 2).
	m1 := make(chan res, 1)
	go func() {
		c, e := session.showInteractive(io.Discard, "Select worktree to open:", []string{"a", "b", "c"}, 2, noopRestore)
		m1 <- res{c, e}
	}()
	stream.push('\r')
	select {
	case r := <-m1:
		if r.err != nil || r.choice != 2 {
			t.Fatalf("menu 1 = (%d,%v); want (2,nil)", r.choice, r.err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("menu 1 did not submit on Enter")
	}

	// Menu 2: the screen from the bug report. Press Enter on its default row
	// (defaultIdx 1 → submit 1). With the shared reader, this keystroke is NOT
	// stolen.
	m2 := make(chan res, 1)
	go func() {
		c, e := session.showInteractive(io.Discard, "Open in:", []string{"Open here", "tmux window", "tmux session"}, 1, noopRestore)
		m2 <- res{c, e}
	}()
	stream.push('\r')
	select {
	case r := <-m2:
		if r.err != nil || r.choice != 1 {
			t.Fatalf("menu 2 = (%d,%v); want (1,nil)", r.choice, r.err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("BUG: menu 2's Enter was stolen — the second screen did not respond to Enter")
	}
}

// TestMenuSession_ThreeMenusNoTheft covers the wt-delete worst case:
// handleDeleteCurrent can show three menus in a row (uncommitted-changes →
// unpushed-commits → final confirm). This drives three Enter keystrokes
// through one session over a single shared stream and asserts each menu
// receives its keystroke — the byte-theft bug would starve menus 2 and 3.
func TestMenuSession_ThreeMenusNoTheft(t *testing.T) {
	stream := newSharedStream()
	session := &MenuSession{
		interactive: true,
		reader:      newBlockingByteReader(stream),
	}
	noop := func() {}

	for i := 1; i <= 3; i++ {
		got := make(chan int, 1)
		go func() {
			// defaultIdx 1 → Enter submits row 1.
			c, _ := session.showInteractive(io.Discard, "menu", []string{"only"}, 1, noop)
			got <- c
		}()
		stream.push('\r')
		select {
		case c := <-got:
			if c != 1 {
				t.Fatalf("menu %d submitted %d; want 1", i, c)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("BUG: menu %d's Enter was stolen by a prior menu's orphaned reader", i)
		}
	}
}

// =============================================================================
// REGRESSION: menu → line-prompt byte theft (the `wt create` name-prompt bug)
// =============================================================================
//
// `wt create` shows the dirty-state menu and then immediately prompts
// "Worktree name [<suggested>]:". Before the fix the menu built its own
// blockingByteReader over os.Stdin and the prompt built a FRESH bufio.Reader
// over the same fd. The menu's pump parked one byte ahead on the shared stream
// after submit — an orphan — and in cooked mode the kernel delivers a typed
// line to ONE reader; the orphan (queued first) won and slurped the line into
// its buffer, so the real prompt hung and the user's name was lost.
//
// The fix routes the line prompt through the session's SHARED reader
// (MenuSession.PromptWithDefault → blockingByteReader.readLine), so there is
// never a second reader to race — the pump's pending byte is delivered to the
// prompt instead of being stolen.

// pushString feeds each byte of s onto the shared stream. It blocks once more
// than the stream's channel buffer is queued, so callers with a feed longer
// than the buffer must run it concurrently with a consuming reader — use
// feedAsync for that.
func pushString(s *sharedStream, str string) {
	for i := 0; i < len(str); i++ {
		s.push(str[i])
	}
}

// feedAsync pushes str onto the stream from a separate goroutine (so a feed
// longer than the channel buffer does not block the test goroutine before a
// reader starts draining), optionally closing the channel afterward to
// simulate EOF.
func feedAsync(s *sharedStream, str string, closeEOF bool) {
	go func() {
		pushString(s, str)
		if closeEOF {
			close(s.bytes)
		}
	}()
}

// TestUnderlyingReadAhead_MenuToLinePromptTheft characterizes WHY the menu →
// line-prompt bug existed: a blockingByteReader (the menu's) parked on a shared
// stream reads one byte ahead, so an independent second reader (a fresh
// prompt's) does NOT receive the user's typed line intact — the menu's orphan
// steals whatever byte(s) it had already pulled off the stream. This is the
// exact corruption the shared-reader fix routes around. Analogue of
// TestUnderlyingReadAhead_DemonstratesTheft for the menu→line-prompt seam.
func TestUnderlyingReadAhead_MenuToLinePromptTheft(t *testing.T) {
	stream := newSharedStream()

	// The "menu" reader consumes its Enter and its pump parks one byte ahead.
	menuReader := newBlockingByteReader(stream)
	stream.push('\r')
	if bt, ok := menuReader.readByteBlocking(); !ok || bt != '\r' {
		t.Fatalf("menuReader = (%q,%v); want ('\\r',true)", bt, ok)
	}
	time.Sleep(20 * time.Millisecond) // let the pump re-enter its blocking read

	// A fresh, independent reader for the "prompt" — the pre-fix shape (a fresh
	// bufio.Reader over the same fd). Feed the full typed line concurrently; the
	// menu's orphan is parked first and steals the leading byte(s), so the
	// prompt reader never assembles the intact "my-name".
	promptReader := newBlockingByteReader(stream)
	feedAsync(stream, "my-name\n", false)

	got := make(chan string, 1)
	go func() {
		line, ok := promptReader.readLine()
		if !ok {
			got <- "<eof>"
			return
		}
		got <- line
	}()
	select {
	case line := <-got:
		if line == "my-name" {
			t.Fatalf("expected the menu's orphan to steal bytes so the prompt reader cannot read %q intact; it read the full line anyway", "my-name")
		}
		// Corrupted/partial (e.g. "-name") — the theft the shared-reader fix avoids.
	case <-time.After(500 * time.Millisecond):
		// Starved — also a valid manifestation of the theft.
	}
}

// TestMenuSession_LinePromptAfterMenuNoTheft is the regression guard for the
// fix. It drives a session.Show (the dirty-state menu) followed by a
// session.PromptWithDefault (the "Worktree name" prompt) over ONE shared
// stream — exactly the wt-create shape — and asserts the prompt receives the
// full typed line. Before the fix (a fresh reader per prompt) the line was
// stolen by the menu's orphaned pump and this would hang/starve.
func TestMenuSession_LinePromptAfterMenuNoTheft(t *testing.T) {
	withDisabledColors(t)

	stream := newSharedStream()
	session := &MenuSession{
		interactive: true,
		reader:      newBlockingByteReader(stream),
	}
	noopRestore := func() {}

	// Menu: submit the default row via Enter (defaultIdx 1 → submit 1).
	m := make(chan int, 1)
	go func() {
		c, _ := session.showInteractive(io.Discard, "How to proceed?", []string{"Continue anyway", "Stash", "Abort"}, 1, noopRestore)
		m <- c
	}()
	stream.push('\r')
	select {
	case c := <-m:
		if c != 1 {
			t.Fatalf("menu submitted %d; want 1", c)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("menu did not submit on Enter")
	}

	// Line prompt on the SAME session reader: the user types a name + Enter.
	// With the shared reader this line is NOT stolen by the menu's parked pump.
	//
	// PromptWithDefault writes its prompt to os.Stdout; the test only cares
	// about the returned value, so we let that write go to the real stdout
	// (harmless) and assert on the return.
	p := make(chan string, 1)
	go func() { p <- session.PromptWithDefault("Worktree name", "lively-tamarin") }()
	pushString(stream, "chosen-name\n")
	select {
	case name := <-p:
		if name != "chosen-name" {
			t.Fatalf("BUG: prompt returned %q; want %q — the menu's orphan stole the line", name, "chosen-name")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("BUG: the line prompt after the menu hung — its input was stolen by the menu's orphaned reader")
	}
}

// =============================================================================
// blockingByteReader.readLine — unit coverage
// =============================================================================

func TestBlockingByteReader_ReadLine(t *testing.T) {
	tests := []struct {
		name     string
		feed     string // bytes to push
		closeEOF bool   // close the stream after feeding (simulates EOF)
		want     string
		wantOK   bool
	}{
		{name: "multi-byte line", feed: "hello\n", want: "hello", wantOK: true},
		{name: "empty line", feed: "\n", want: "", wantOK: true},
		{name: "crlf stripped", feed: "a\r\n", want: "a", wantOK: true},
		{name: "multi-byte crlf", feed: "swift-fox\r\n", want: "swift-fox", wantOK: true},
		{name: "eof before newline discards partial", feed: "xy", closeEOF: true, want: "", wantOK: false},
		{name: "eof immediately", feed: "", closeEOF: true, want: "", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := newSharedStream()
			r := newBlockingByteReader(stream)
			feedAsync(stream, tc.feed, tc.closeEOF)
			got := make(chan struct {
				line string
				ok   bool
			}, 1)
			go func() {
				line, ok := r.readLine()
				got <- struct {
					line string
					ok   bool
				}{line, ok}
			}()
			select {
			case res := <-got:
				if res.line != tc.want || res.ok != tc.wantOK {
					t.Fatalf("readLine() = (%q, %v); want (%q, %v)", res.line, res.ok, tc.want, tc.wantOK)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("readLine() did not return within timeout")
			}
		})
	}
}

// =============================================================================
// Session-aware PromptWithDefault / ConfirmYesNo — semantics (no PTY)
// =============================================================================
//
// These drive the interactive-mode methods through the injected shared-reader
// seam (a MenuSession with interactive=true over a sharedStream), so the prompt
// semantics are exercised without a real terminal. The prompt text itself goes
// to the process's real stdout/stderr (harmless); the tests assert on the
// returned value.

func newInteractiveSessionOver(stream *sharedStream) *MenuSession {
	return &MenuSession{interactive: true, reader: newBlockingByteReader(stream)}
}

func TestMenuSession_PromptWithDefault_Semantics(t *testing.T) {
	tests := []struct {
		name     string
		feed     string
		closeEOF bool
		want     string
	}{
		{name: "typed value", feed: "my-name\n", want: "my-name"},
		{name: "empty line uses default", feed: "\n", want: "lively-tamarin"},
		{name: "eof before newline uses default", feed: "part", closeEOF: true, want: "lively-tamarin"},
		// Trimming matches the package-level function (fallback mode), so both
		// modes behave identically on padded input.
		{name: "whitespace-only line uses default", feed: "   \n", want: "lively-tamarin"},
		{name: "typed value is trimmed", feed: "  my-name  \n", want: "my-name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := newSharedStream()
			session := newInteractiveSessionOver(stream)
			feedAsync(stream, tc.feed, tc.closeEOF)
			got := make(chan string, 1)
			go func() { got <- session.PromptWithDefault("Worktree name", "lively-tamarin") }()
			select {
			case v := <-got:
				if v != tc.want {
					t.Fatalf("PromptWithDefault = %q; want %q", v, tc.want)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("PromptWithDefault did not return within timeout")
			}
		})
	}
}

func TestMenuSession_ConfirmYesNo_Semantics(t *testing.T) {
	tests := []struct {
		name     string
		feed     string
		closeEOF bool
		want     bool
	}{
		{name: "empty line is default yes", feed: "\n", want: true},
		{name: "y is yes", feed: "y\n", want: true},
		{name: "Y is yes", feed: "Y\n", want: true},
		{name: "yes word is yes", feed: "yes\n", want: true},
		{name: "n is no", feed: "n\n", want: false},
		{name: "no word is no", feed: "no\n", want: false},
		{name: "eof is no", feed: "", closeEOF: true, want: false},
		// Trimming matches the package-level function (fallback mode), so both
		// modes behave identically on padded input.
		{name: "whitespace-only line is default yes", feed: "   \n", want: true},
		{name: "padded y is yes", feed: " y \n", want: true},
		{name: "padded n is no", feed: " n \n", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := newSharedStream()
			session := newInteractiveSessionOver(stream)
			feedAsync(stream, tc.feed, tc.closeEOF)
			got := make(chan bool, 1)
			go func() { got <- session.ConfirmYesNo("Initialize worktree?") }()
			select {
			case v := <-got:
				if v != tc.want {
					t.Fatalf("ConfirmYesNo = %v; want %v", v, tc.want)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("ConfirmYesNo did not return within timeout")
			}
		})
	}
}
