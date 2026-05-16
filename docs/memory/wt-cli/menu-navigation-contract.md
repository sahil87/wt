# wt-cli: Menu Navigation Contract

> Post-implementation behavior capture for the arrow-key menu navigation upgrade.
> Source change: `260516-dkg7-arrow-key-menu-navigation`.

This file documents the contract that `ShowMenu` honors after the arrow-key navigation change. Future changes touching `src/internal/worktree/menu.go` should preserve these invariants unless an explicit spec amendment supersedes them. The change affects every interactive prompt invoked by `wt` (most prominently the `wt open` / `wt delete` worktree pickers) — but only via the shared `ShowMenu` entry point. No call site under `src/cmd/wt/` was edited.

## Requirements

### TTY detection is single-shot and gates the whole call

- `isInteractiveTTY()` in `src/internal/worktree/menu.go` calls `term.IsTerminal(int(os.Stdin.Fd()))` AND `term.IsTerminal(int(os.Stdout.Fd()))`. Both must be TTYs for the interactive path to run.
- The detection runs **exactly once per `ShowMenu` invocation**, at the very top of the function, **before any output is emitted**. This guarantees the rendered prompt format is consistent end-to-end within a single call — there is no per-keystroke re-detection and no mid-prompt path switch.
- Either stream being non-TTY (CI, piped stdin via `cmd.Stdin = strings.NewReader("…")`, redirected stdout) routes the call to the fallback path.

### `ShowMenu`'s public signature is preserved

- The exported entry point remains `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)`. Return semantics are unchanged: `0` = Cancel, `1..N` = the corresponding option, `error` is non-nil only for I/O failures.
- All ~11 call sites in `src/cmd/wt/{create,delete,open}.go` continue to compile without edits. Their existing `if choice == 0 { return }` Cancel-handling branches continue to function unchanged.

### Default highlight seeding rule

- `defaultIdx >= 1` → the interactive renderer pre-highlights that row on first paint.
- `defaultIdx == 0` → the renderer pre-highlights the `Cancel` row.
- `defaultIdx == -1` (no default) → the renderer pre-highlights row 1.
- The existing green `(default)` marker is still rendered on the `defaultIdx` row, visually distinct from the moving reverse-video highlight. A row may carry both at once (default row when highlighted = green marker + reverse video + `›` gutter).
- The seeding rule lives in `initialHighlight(defaultIdx, numOptions)` and is exercised by `nextMenuState` seeding tests — it is verified without opening a real terminal.

### Key bindings in the interactive path

| Key | Action |
|-----|--------|
| `↑` (`\x1b[A`), `k` | Move highlight up one row; wraps between row 1 and Cancel |
| `↓` (`\x1b[B`), `j` | Move highlight down one row; wraps between Cancel and row 1 |
| `1`–`9` | If in range, **immediately submit** that option (no Enter required). Out-of-range digits are silently ignored |
| `0` | Immediately submit Cancel |
| `Enter` (`\r` or `\n`) | Submit the currently highlighted row |
| `Esc` (`\x1b` with no follow-up within 50ms), `Ctrl-C` (`\x03`), `q` | Cancel — return `(0, nil)` |
| Any other key (Tab, Backspace, F-keys, `\x1bOP`, Page Up/Down, etc.) | Silently ignored (no redraw, no bell, no submit) |

### Wrap-around at edges

- `↓` from the last option lands on the `Cancel` row. `↓` from `Cancel` lands on row 1.
- `↑` from row 1 lands on `Cancel`. `↑` from `Cancel` lands on the last option.
- Hard stops at first/last row are explicitly rejected — wrap-around matches `fzf`, `gum`, `gh`, and every modern picker.

### In-place redraw via cursor-up + `\x1b[2K`

- Every highlight change emits `\x1b[<rows>A` (cursor up N) followed by `\x1b[2K` (clear line) per line, then the new row content. Scrollback MUST NOT accumulate intermediate menu states.
- The renderer does NOT use the alternate-screen buffer (`\x1b[?1049h`). The menu is a per-prompt widget, not a fullscreen app; alternate-buffer mode is jarring at this scope.
- `paintMenu` performs the first paint; `redrawMenu` performs in-place updates. After the T018 refactor, both delegate to a shared `renderRows(w, prompt, options, st, linePrefix)` helper — `paintMenu` calls it with `linePrefix=""`, `redrawMenu` writes the cursor-up prelude then calls it with `linePrefix=ansiClearLine`. Row content is byte-identical between first paint and redraw, asserted by `TestPaintAndRedrawShareCore`.

### Cancel returns `(0, nil)` — no new error type

- The interactive path's Cancel outcome is byte-compatible with the fallback path's "user typed `0`" outcome. No `ErrUserAbort` or similar typed error is introduced.
- Every existing call site's `if choice == 0 { ... }` branch continues to function unchanged. This was the load-bearing reason for not introducing a typed error — adding one would force edits across ~11 call sites with zero observable UX gain.

### Raw mode is restored on every exit path including panic

- The interactive shell calls `term.MakeRaw(int(os.Stdin.Fd()))` immediately before the first key read.
- `defer term.Restore(fd, oldState)` is wired at the top of `runInteractiveMenu` and fires on every return path: normal submit, Cancel, `Ctrl-C` (which is parsed as a `\x03` byte inside raw mode and never escapes as a signal), I/O error, and panic recovery during the read loop.
- The actual loop body lives in `runInteractiveMenuCore(w, stdin, prompt, options, defaultIdx, restoreFn)` — extracted as a **test seam** during T017. The outer `runInteractiveMenu` wires real stdin/stdout plus a `term.Restore` closure; tests inject a `panickingReader` plus a counter-based fake `restoreFn` to assert the deferred restore runs exactly once before a panic unwinds. See `TestRunInteractiveMenuCore_PanicRestore` in `menu_test.go`.
- No `signal.Notify`/`signal.Stop` is required: raw mode swallows `\x03` as a single byte (parsed by `parseKey` as `keyCancel`), so no SIGINT escapes mid-menu. The deferred `term.Restore` is the sole guarantee for cooked-mode restore.

### Bell on unknown keys is suppressed

- Unknown keystrokes produce no `\x07`, no error message, no redraw. The state machine returns the previous state unchanged with `submitted=false`. Consistent with `fzf` / `gum` — bell on every typo would be obnoxious in a picker.

### Non-TTY fallback is byte-identical to historical behavior

- When `isInteractiveTTY()` returns `false`, `ShowMenu` runs the original numbered-prompt body verbatim. The byte-for-byte invariant covers:
  - The numbered option list and the `0) Cancel` line.
  - The `(default)` green marker on the default row.
  - The `Choice [N]: ` prompt (when `defaultIdx >= 0`) and the `Choice: ` prompt (when `defaultIdx == -1`).
  - Both validation error messages: `Invalid choice. Please enter a number.` and `Invalid choice. Please enter a number between 0 and N.`
- Existing test harnesses driving `ShowMenu` via piped stdin continue to pass unmodified. Integration tests under `cmd/integration_test.go` invoke the binary with pipes and naturally land on the fallback path.

### Pure `nextMenuState` and `parseKey` are testable without a PTY

- `nextMenuState(prev menuState, key keyEvent) menuStateTransition` is a **pure function** with no I/O, no globals, no clock. It encodes every key-mapping, wrap-around, digit boundary, and Cancel transition.
- `parseKey(first byte, rest byteReader) keyEvent` is a **pure escape-sequence parser**. It maps raw bytes to a small sum-type (`keyUp`, `keyDown`, `keyEnter`, `keyCancel`, `keyDigit{n}`, `keyIgnore`). The 50ms bare-Esc disambiguation window is injected through the `byteReader` interface (in tests, a fake reader returns `io.EOF` instead of blocking).
- Unit tests in `src/internal/worktree/menu_test.go` exercise every keybinding, every wrap-around direction, in-range and out-of-range digit submission, both `defaultIdx` seeding cases (`-1`, `0`, `>=1`), the bare-Esc-vs-arrow ambiguity (with the fake-clock 50ms window), and the unknown-sequence ignore path — all without opening a terminal.

### Final-line semantics

- On submit, the menu region is replaced with a single line: `<prompt> <option-text>` (e.g., `Open in: cursor`).
- On Cancel, the menu region is replaced with a single line: `<prompt> (cancelled)`.
- No orphaned highlight marker, `›` gutter, or reverse-video residue remains after the final line is emitted. This keeps the post-prompt scrollback clean.

### 50ms bare-Esc disambiguation window

- A lone `\x1b` byte is ambiguous: it could be the start of an arrow sequence (`\x1b[A` / `\x1b[B`) or a bare `Esc` keystroke.
- `parseKey` resolves the ambiguity by attempting to read one more byte from the `byteReader` with a 50ms timeout. If a follow-up byte arrives within the window, the parser continues consuming the sequence. If no follow-up arrives within 50ms (or the reader returns `io.EOF`), the bare `\x1b` is treated as Cancel.
- 50ms is below human perception threshold but well above typical arrow-key burst timing (sub-ms between the `\x1b` and the `[`). Standard approach also used by `readline`.

### Windows: short-circuit to fallback (chosen path)

- `isInteractiveTTY()` returns `false` unconditionally on `runtime.GOOS == "windows"` (chosen path b per spec). Windows users get the historical numbered-prompt UX; Linux/macOS users get arrow-key navigation immediately.
- Rationale: Windows ConPTY raw-mode handling has enough quirks (line-buffering on certain terminals, key-code differences, conhost vs. Windows Terminal) that fully wiring `term.MakeRaw` on Windows was deferred. Linux/macOS users gain the UX immediately; Windows users are no worse off than before this change.
- This is **deliberate and deterministic** — not a bug, not flaky. Future change MAY revisit if ConPTY raw-mode is stabilized. The comment in `menu.go` next to `isInteractiveTTY` documents the choice for traceability.

### Sole new dependency: `golang.org/x/term`

- `src/go.mod` gains exactly one new direct dependency: `golang.org/x/term`. Transitive: `golang.org/x/sys` (already an `x/` package maintained by the Go team).
- No `github.com/charmbracelet/...`, `github.com/AlecAivazis/...`, or `github.com/manifoldco/...` entries are introduced. Constitution Principle I (single-binary CLI, no hidden state, slim dep graph) explicitly motivated rejecting `bubbletea`, `survey/v2`, and `promptui` in favor of a hand-rolled implementation against `x/term`.

## Design Decisions

### Hand-rolled `golang.org/x/term` over survey/v2 / bubbletea / promptui

`golang.org/x/term` + raw-mode escape parsing produces a ~150–250 LOC renderer with full control over cancel semantics, default-marker preservation, in-place redraw, and the Cancel→`0` mapping. Constitution Principle I (slim single-binary CLI) was the load-bearing motivator. `survey/v2` would have added ~5 indirect deps and surrendered control over the rendering surface; `bubbletea` was overkill (pulls in `lipgloss`, `termenv`, `harmonica`) for a one-widget upgrade; `promptui` is less actively maintained with similar tradeoffs to survey. (Source: intake §4 + spec Design Decisions #1.)

### Cancel returns `(0, nil)` over `ErrUserAbort`

An `ErrUserAbort` typed error would force every one of the ~11 `ShowMenu` call sites in `src/cmd/wt/{create,delete,open}.go` to add a new branch — for zero observable UX gain over the existing `if choice == 0 { return }` pattern. Preserving the integer-return contract was the lowest-risk path with the largest backward-compatibility win. (Source: intake Q3 + spec Design Decisions #2.)

### Digit keys submit immediately (no Enter)

Today's fallback path treats `3<Enter>` as "select option 3"; in the interactive path, typing `3` submits option 3 immediately. This preserves the muscle memory of every user who learned the numbered prompt before this change. Arrow-key navigation is the slower-but-more-discoverable path. Digit-highlights-only-then-Enter was rejected as adding a step for users who already know what they want. (Source: spec Design Decisions #3.)

### In-place redraw over alternate-screen buffer

Alternate-buffer mode (`\x1b[?1049h`) is the right call for a fullscreen TUI but wrong scope for a per-prompt widget. Cursor-up + `\x1b[2K` redraws only the menu region and leaves prior shell output in scrollback. Same approach used by `gum choose` and `gh`'s interactive selectors. (Source: spec Design Decisions #4.)

### Wrap-around at edges

`↓` from the last row landing on `Cancel` (and wrapping past `Cancel` to the first option) is the industry-standard picker behavior. Hard stops feel broken on short lists like the typical worktree picker. (Source: spec Design Decisions #5.)

### Pure `nextMenuState` state machine for testability

Real-PTY tests are slow, flaky, and OS-specific. Extracting the state machine as a pure function tests every keybinding deterministically in microseconds and works identically on Linux, macOS, and Windows CI. The raw I/O shell is intentionally a thin composition layer around `parseKey` and `nextMenuState`. (Source: spec Design Decisions #6 + constitution Principle IV "Test what the user sees".)

### Windows scoped out of interactive raw-mode (path b)

Of the two options the spec allowed (path a: wire raw-mode on Windows ConPTY; path b: fall back to numbered prompt on Windows), path b was chosen. ConPTY raw-mode quirks (line-buffering on certain terminals, key-code differences) make full Windows arrow-key support a non-trivial side quest. The Linux/macOS UX win ships now without blocking on Windows-specific debugging. (Source: spec Cross-Platform requirement + T010.)

### `runInteractiveMenuCore` extracted as a test seam (T017)

This is a **pattern worth carrying to future changes**: when a function combines raw-mode I/O (which is hostile to testing) with a pure-ish loop body (which is easy to test), extract the loop body into a `*Core` helper that accepts `io.Reader` / `io.Writer` for streams and `func()` for the restore action. The outer function wires real OS handles + a `term.Restore` closure; tests inject a panicking reader + a counter-based fake restore to verify the deferred-restore guarantee. The seam is internal-only (lowercase, unexported); `ShowMenu`'s public signature did not change. This is the canonical pattern for "I need to test that `defer` fires on panic, but I can't open a real terminal in CI." Future changes that introduce similar raw-mode/signal-handling code in `wt` should reach for this seam shape first. (Source: T017 + Acceptance A-011/A-028.)

### `paintMenu` and `redrawMenu` share `renderRows` (T018 refactor)

Originally `paintMenu` and `redrawMenu` had two parallel rendering paths — the redraw path additionally emitted `\x1b[2K` per line. The T018 refactor extracted `renderRows(w, prompt, options, st, linePrefix)` as the shared core: `paintMenu` calls it with `linePrefix=""`, `redrawMenu` writes the cursor-up prelude then calls it with `linePrefix=ansiClearLine`. The row content is byte-identical between first paint and redraw; `TestPaintAndRedrawShareCore` strips the prelude/clears and asserts byte-equality. No behavioral change, but the refactor eliminates the drift risk where the two paths could diverge on a future highlight-marker tweak.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260516-dkg7-arrow-key-menu-navigation` | 2026-05-17 | Added arrow-key menu navigation to `ShowMenu`: TTY-aware branch on entry (single-shot detection of stdin+stdout); interactive path renders an in-place reverse-video highlight with `↑`/`↓`/`j`/`k` navigation, wrap-around at edges, digit-submit (`1`–`9`, `0`→Cancel), Enter-submit, Esc/Ctrl-C/`q`→Cancel→`(0, nil)`; `defer term.Restore` on every exit including panic (via the `runInteractiveMenuCore` test seam); non-TTY fallback path byte-identical to historical numbered-prompt behavior; pure `nextMenuState` + `parseKey` testable without PTY; Windows short-circuits to fallback; sole new dependency `golang.org/x/term`. |

## Cross-references

- Spec doc: none — interactive prompt UX is per-prompt-widget behavior, not part of `docs/specs/cli-surface.md`'s per-subcommand flag surface. This contract lives in memory only.
- Source: `src/internal/worktree/menu.go` — `ShowMenu`, `isInteractiveTTY`, `runInteractiveMenu`, `runInteractiveMenuCore`, `paintMenu`, `redrawMenu`, `renderRows`, `nextMenuState`, `parseKey`, `initialHighlight`, `menuState`, `keyEvent`, `menuStateTransition`, named ANSI / key constants.
- Tests: `src/internal/worktree/menu_test.go` — `nextMenuState` table-driven coverage (every keybinding, wrap-around, digit boundaries, seeding), `parseKey` table-driven coverage (arrow sequences, bare-Esc-vs-arrow timeout via fake clock, unknown sequences), fallback-path byte-equality tests, `TestRunInteractiveMenuCore_PanicRestore` (defer-restore guarantee), `TestPaintAndRedrawShareCore` (first-paint / redraw byte-equality).
- Constitution: Principle I (Single-Binary CLI — motivated rejecting third-party TUI deps), Principle IV (Test What the User Sees — motivated the pure state-machine seam), Principle VI (Interactive by Default, Scriptable on Demand — motivated the byte-identical non-TTY fallback).
- Sibling memory: `wt-cli/init-failure-contract.md` (different `wt` subcommand, same post-change invariant-capture pattern), `wt-cli/list-status-contract.md` (different subcommand, same pattern).
- Call sites (informational, not edited by this change): `src/cmd/wt/open.go` (2 calls), `src/cmd/wt/delete.go` (7 calls), `src/cmd/wt/create.go` (2 calls).
