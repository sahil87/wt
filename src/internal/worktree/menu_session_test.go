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
