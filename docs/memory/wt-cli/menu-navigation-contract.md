---
type: memory
description: "Arrow-key navigation contract for the shared `ShowMenu` — TTY gating, keybindings, a terminal-height-aware scrolling viewport, a byte-identical non-TTY fallback, and the single-reader `MenuSession` extended to the menu→line-prompt seam."
---
# wt-cli: Menu Navigation Contract

> Post-implementation behavior capture for the shared interactive menu navigation.
> Source changes: `260516-dkg7`, `260705-3wr8`, `260708-wryx`, `260717-6end`, `260722-0is3`
> (each contract below carries its citation).

This file documents the contract that `ShowMenu` honors. Future changes touching `src/internal/worktree/menu.go` should preserve these invariants unless an explicit spec amendment supersedes them. The contract covers every interactive prompt invoked by `wt` (most prominently the `wt go` worktree picker and the `wt delete` worktree picker) via the shared `ShowMenu` entry point. One `cmd/` call site is deliberately session-threaded: `wt create`'s `RunE` threads one `MenuSession` through its whole interactive flow, sitting on the `MenuSession` single-reader invariant this file carries and extending it from menu→menu to menu→line-prompt (260708-wryx; see § The menu→line-prompt seam and § `wt create` joins the session-threading pattern).

## Requirements

### TTY detection is single-shot and gates the whole call

- `isInteractiveTTY()` in `src/internal/worktree/menu.go` calls `term.IsTerminal(int(os.Stdin.Fd()))` AND `term.IsTerminal(int(os.Stdout.Fd()))`. Both must be TTYs for the interactive path to run.
- The detection runs **exactly once per `ShowMenu` invocation**, at the very top of the function, **before any output is emitted**. This guarantees the rendered prompt format is consistent end-to-end within a single call — there is no per-keystroke re-detection and no mid-prompt path switch.
- Either stream being non-TTY (CI, piped stdin via `cmd.Stdin = strings.NewReader("…")`, redirected stdout) routes the call to the fallback path.

### `ShowMenu`'s public signature is preserved

- The exported entry point remains `ShowMenu(prompt string, options []string, defaultIdx int) (int, error)`. Return semantics are unchanged: `0` = Cancel, `1..N` = the corresponding option, `error` is non-nil only for I/O failures.
- All ~11 call sites in `src/cmd/wt/{create,delete,open,go}.go` continue to compile without edits. Their existing `if choice == 0 { return }` Cancel-handling branches continue to function unchanged.

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

> **Page Up/Down stays `keyIgnore` (re-affirmed by `260705-3wr8`).** The viewport change restores full arrow-key reachability to over-tall menus (the reported bug), so no page-jump keybinding was added — mapping `\x1b[5~` / `\x1b[6~` to window jumps would expand the documented keybinding surface and was deliberately left as a candidate follow-up. `parseKey` still resolves both to `keyIgnore` via the CSI default branch (`TestParseKey_*` coverage unchanged).

### Wrap-around at edges

- `↓` from the last option lands on the `Cancel` row. `↓` from `Cancel` lands on row 1.
- `↑` from row 1 lands on `Cancel`. `↑` from `Cancel` lands on the last option.
- Hard stops at first/last row are explicitly rejected — wrap-around matches `fzf`, `gum`, `gh`, and every modern picker.

### In-place redraw via cursor-up + `\x1b[2K`

- Every highlight change emits `\x1b[<rows>A` (cursor up N) followed by `\x1b[2K` (clear line) per line, then the new row content. Here `<rows>` is `rowsRendered` — the *windowed* row count (see § Terminal-height-aware scrolling viewport), not `len(options)+2` — so the cursor-up target always matches the region actually on screen. Scrollback MUST NOT accumulate intermediate menu states.
- The renderer does NOT use the alternate-screen buffer (`\x1b[?1049h`). The menu is a per-prompt widget, not a fullscreen app; alternate-buffer mode is jarring at this scope. This decision was **re-affirmed** by `260705-3wr8` — the windowing change made scrolling menus work *without* switching to the alternate buffer, keeping the per-prompt scope intact.
- `paintMenu` performs the first paint; `redrawMenu` performs in-place updates. Both delegate to a shared `renderRows(w, prompt, options, st, linePrefix, height)` helper — `paintMenu` calls it with `linePrefix=""`, `redrawMenu` writes the cursor-up prelude then calls it with `linePrefix=ansiClearLine`. The `height` argument (added by `260705-3wr8`) drives the scrolling viewport; both paths pass the same value, so first-paint and redraw row content stay byte-identical, asserted by `TestPaintAndRedrawShareCore`. `paintMenu`/`redrawMenu` return the *windowed* row count (1 prompt + up-to-2 indicators + visible options + 1 Cancel), **not** `len(options)+2`.
- **redraw shrink hardening** (`260705-3wr8`): because height is re-queried each redraw, a mid-menu terminal shrink can make the new windowed row count *smaller* than the previous one. When that happens `redrawMenu` clears the trailing `(old − new)` stale lines below the newly-painted region and moves the cursor back up by that delta, so the returned count stays an accurate cursor-up target and no ghost rows linger until finalize (guarded on `extra > 0`; a no-op for equal-height paint/redraw, so byte-equality is unaffected). Full resize handling remains a non-goal — this is minimal robustness.

### Terminal-height-aware scrolling viewport

The renderer windows its region to the terminal height (fzf/gum-style windowing) inside the shared renderer, covering every interactive menu at once with zero call-site edits (260705-3wr8). Without windowing, an over-tall region (`N + 2` rows exceeding the terminal height) would scroll the top into scrollback on first paint, re-scroll on every keystroke's redraw, and leave rows above the viewport unreachable by the arrow keys — the failure mode the viewport exists to prevent.

- **Window height derives from `term.GetSize` alone.** `terminalHeight()` calls `term.GetSize(int(os.Stdout.Fd()))` (stdout — the stream the menu region is painted to) and is re-queried at paint/redraw time, so a terminal resize is absorbed on the next keystroke's repaint. There is **no SIGWINCH handling** and **no additional fixed row-cap constant** (menus are transient widgets — available height is the only knob). On `term.GetSize` failure (or a non-positive height) it falls back to `defaultTerminalHeight = 24` (the classic terminal default) so output stays bounded rather than degrading to the unbounded full-region paint the change exists to fix.
- **Height is injected, not read ad hoc.** `runInteractiveMenuCore` takes a `heightFn func() int` (mirroring the existing `restoreFn func()` injection); production wires `terminalHeight`, tests wire a constant — so the windowing is fully unit-testable without a PTY. `height` then flows as an `int` parameter through `paintMenu`/`redrawMenu`/`renderRows`.
- **`menuState.top` is the window offset.** `menuState` carries a `top` field (0-based index of the first visible option). It is pure state: the render path recomputes it from `menuLayout` at paint/redraw time (so a resize is naturally absorbed), same discipline as `nextMenuState`.
- **`menuLayout` is the single windowing seam.** `menuLayout(numOptions, highlight, prevTop, height) (top, first, count, moreAbove, moreBelow int)` is a pure function (no I/O, no globals, no clock) computing the whole window — visible slice `[first, first+count)` plus hidden-row counts. Two extracted helpers keep it under the 50-line bar: `optionRowsForTop` (how many option rows fit for a candidate `top`, accounting for indicator rows) and `dropUnaffordableIndicators` (the cramped-viewport chrome drop, below). It is exercised by `TestMenuLayout`, the exhaustive `TestMenuLayout_FootprintSweep`, and `TestMenuLayout_WrapJumpFromTopToBottom`.
- **Footprint bound: `rowsRendered ≤ height − 1` for heights ≥ 4.** The option-region budget is `height − menuOverheadRows − 1` (`menuOverheadRows = 2`: prompt + Cancel). The extra reserved row is load-bearing: **every row ends `\r\n`, so a region filling all `height` rows emits a trailing newline on the bottom screen row and scrolls the terminal one line per repaint**, leaking a stale menu copy per keystroke. Reserving one row keeps the footprint at `rowsRendered ≤ height − 1`, so the cursor-up in-place redraw stays sound. The invariant is verified exhaustively by `TestMenuLayout_FootprintSweep` (heights 1–12 × options 1–25 × all highlights × all prevTops: zero violations at height ≥ 4). Heights 1–3 (raw budget < 1, clamped to 1) are the bounded **degenerate escape hatch** — the region stays ≤ 3 rows (prompt + one option + Cancel, no indicators), ≥ 1 option, and MAY exceed such a terminal's height but never grows unbounded.
- **Overflow indicator rows.** When options are hidden above/below the window, `renderRows` emits an `↑ N more` row at the top and/or a `↓ N more` row at the bottom (`writeIndicatorRow`, using the `indicatorUp` / `indicatorDown` glyphs). They show the true hidden count, are **non-selectable rendering artifacts** (no number, never highlighted), and are **plain-styled** like a non-highlighted, non-default row so they read as chrome, not a choice. Each indicator occupies a row within the window budget, reducing visible options by up to 2 when both are shown.
- **Cramped-viewport indicator drop.** When the budget cannot fit the indicator rows *plus* at least one option (heights 4–5, where indicator deductions would drive the visible count below one), `dropUnaffordableIndicators` **drops indicator chrome** rather than overflowing the budget — ↓ first, then ↑. Hidden options may then exist with no indicator shown (the "N more" hint IS the row's payload, so it cannot be shown without spending the row — dropping it and preferring the visible option is fzf's behavior on a cramped viewport). This is what lets the footprint bound hold down to height 4.
- **Wrap-around jumps the window.** Wrap-around navigation semantics are unchanged (↑ from row 1 → Cancel; ↓ from Cancel → row 1; ↑ from Cancel → last option; ↓ from last option → Cancel — all still in the pure `nextMenuState`). When the highlight wraps far across the list, the window jumps so the highlighted option stays visible. Cancel (highlight 0) never constrains the option window — it is a fixed overhead row rendered after the option region and is always visible.
- **Small menus render byte-identically.** A menu whose options fit the budget (confirmations, 1–4-option pickers in a normal terminal) produces `top = 0`, no indicators, all options visible — byte-identical to the pre-windowing output. Verified by `TestRenderRows_SmallMenuByteIdenticalToUnwindowed`; slicing/indicator behavior by `TestRenderRows_WindowedSlicing` and `TestRenderRows_NoIndicatorsAtEdges`; the 24-row fallback by `TestTerminalHeightFallback`.

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

### Non-TTY fallback is byte-identical to the numbered-prompt behavior — except the two EOF cases below

- When `isInteractiveTTY()` returns `false`, `ShowMenu` runs the numbered-prompt fallback body. The byte-for-byte invariant covers:
  - The numbered option list and the `0) Cancel` line.
  - The `(default)` green marker on the default row.
  - The `Choice [N]: ` prompt (when `defaultIdx >= 0`) and the `Choice: ` prompt (when `defaultIdx == -1`).
  - Both validation error messages: `Invalid choice. Please enter a number.` and `Invalid choice. Please enter a number between 0 and N.`
- Existing test harnesses driving `ShowMenu` via piped stdin continue to pass unmodified. Integration tests under `cmd/integration_test.go` invoke the binary with pipes and naturally land on the fallback path.
- The byte-identity invariant excludes exactly two EOF-adjacent cases (260717-6end) — the interactive path, the successful piped-choice path, and every non-EOF validation-retry are byte-identical. See § Non-TTY EOF refusal and § Partial-line-at-EOF is honored below.

### Non-TTY EOF refusal

- When the fallback numbered-menu prompt reaches **EOF with no pending input** (`reader.ReadString('\n')` returns `io.EOF` and the accumulated line is empty after `strings.TrimSpace`), `runFallbackMenu` returns a **structured, actionable `WtError`** — `Error:`/`Why:`/`Fix:` shape — never a bare `reading input: EOF` (260717-6end). This is the non-TTY / piped-empty case: a selection menu with no one to answer it. The message states the menu cannot run without a terminal and names the escape: pass a worktree name (`wt open <name>` / `wt go <name>` / `wt delete <name>`), or add `--non-interactive` where the command supports it. This satisfies toolkit principles №1 (non-interactive by default — refuse naming the flag, never a hang) and №4 (fail fast with actionable errors) at the single shared choke point, covering `wt open` (main-repo menu), `wt go` (no name), and `wt delete` (no name) at once — no `cmd/` call-site edits.
  - **Exit is non-zero, never a hang.** `ShowMenu` returns `(0, error)`; `main.go` maps the error to `ExitGeneralError` (1).
  - **The non-EOF read-error path**: a genuine I/O read failure that is *not* `io.EOF` returns `return 0, fmt.Errorf("reading input: %w", err)`. Only the EOF-with-no-input branch carries the structured refusal.
  - The reworded error uses the existing `WtError(what, why, fix)` helper (`internal/worktree/errors.go`), not an ad-hoc string, and lives entirely in `internal/worktree/menu.go` (Constitution IV error-wording convention; V thin-`cmd/`).

### Partial-line-at-EOF is honored

- `reader.ReadString('\n')` returns any bytes read so far **alongside** the `io.EOF` error, so a valid choice typed **without a trailing newline** before EOF is accepted rather than discarded (260717-6end). The guard is `if err != nil && (!errors.Is(err, io.EOF) || strings.TrimSpace(line) == "")` — the error path fires only when EOF arrives with genuinely no pending input; otherwise the accumulated line falls through to the normal choice-parsing logic.
  - A piped `"0"` (no newline) selects Cancel; a piped `"2"` (no newline) selects option 2 — deliberately matching what a newline-terminated line would do.

### `wt go` (and its deprecated `wt open --select` alias) are the `ShowMenu`/`MenuSession` worktree-selection callers

- `wt go` renders its worktree-selection menu through the **shared**
  `selectWorktree` helper (relocated to `src/cmd/wt/go.go` by `260722-0is3`) —
  which calls `session.Show(...)` → `ShowMenu` (260620-3pp5). The deprecated
  `wt open --select` (alias `--go`) path (`openGo`) is the other caller. There is
  no separate menu primitive; the picker inherits this contract wholesale (arrow
  keys, digit-submit, Cancel→`0`, wrap-around, the `(default)` marker, and the
  in-place redraw) with no `menu.go` specialization. (The helper pins the main
  worktree to row 1; signature `(session, prompt)` (260718-daqj) — that is menu
  *content*, not menu *navigation*: this `ShowMenu`/`MenuSession` contract is
  independent of it. See `/wt-cli/go-command-contract.md`.)
- **`wt open` no longer renders a worktree-selection menu.** `260722-0is3`
  purified `wt open` into a pure launcher: `selectAndOpen` (the former main-repo
  no-arg worktree picker) is **removed** — `wt open` no-arg now opens the current
  context (worktree root / repo root / cwd) directly, so the only menu `wt open`
  itself renders is the "Open in:" app menu (`handleAppMenu` /
  `handleAppMenuWithSession`). The which-worktree menu lives in exactly one verb,
  `wt go` (the two-menu ownership model — see `/wt-cli/go-command-contract.md`).
- **Non-TTY fallback**: `wt go`'s no-arg menu degrades through the same numbered-prompt fallback as every other caller — `isInteractiveTTY()` gates it, and piped/CI invocations land on the numbered prompt automatically. `wt go` adds a separate, earlier guard for the **`--non-interactive` no-arg** case: it refuses with `ExitGeneralError` *before* reaching `selectWorktree` at all (a no-arg selection has no non-interactive default — see `/wt-cli/go-command-contract.md`), so that path never renders even the fallback prompt. `wt go <name>` and `wt go` interactive both reach the menu only when a menu is actually wanted.
- **Shared `MenuSession` for the launch-chaining callers**: the chained-menu flow
  is now **`wt go --open prompt`** (`260722-0is3`) — its `RunE` passes ONE
  `MenuSession` to both `selectWorktree` (the "Select worktree to open:" menu) and,
  via `launchSelection` → `handleAppMenuWithSession`, the "Open in:" menu, so the
  two consecutive menus share a single stdin reader — the documented byte-theft
  fix. The deprecated `wt open --select`'s `openGo` does the same chaining. Plain
  `wt go` (navigating) and `wt go <name> --open prompt` (by-name, app menu on a
  fresh one-shot session) chain no two worktree→app menus on the same reader. The
  single-reader requirement and why it matters are the `MenuSession` contract this
  file's navigation behavior sits on.

### The menu→line-prompt seam: session-aware line prompts

The `MenuSession` single-reader guarantee covers line prompts (`PromptWithDefault`, `ConfirmYesNo`) as well as menus (260708-wryx). The mechanism it defends against: a menu's read-ahead pump (`blockingByteReader`) parks in a blocking `ReadByte` on the fd after the menu submits; in cooked mode the kernel delivers the whole typed line to ONE reader, so a fresh `bufio.NewReader(os.Stdin)` alongside the parked pump loses the race — the orphaned pump (queued first) slurps the line into its buffer, the real prompt hangs and loses the user's first typed line (e.g. a dirty-state menu followed by the `Worktree name [<suggested>]:` prompt swallowing the typed name). Line prompts in a mixed flow must therefore read through the session's shared reader.

- **`MenuSession.PromptWithDefault(prompt, defaultValue string) string` and `MenuSession.ConfirmYesNo(prompt string) bool`** are the line-prompt analogues of `Show` (`src/internal/worktree/menu.go`). In **interactive mode** they read a full line through the session's SHARED `blockingByteReader` (the same reader `Show` uses) instead of a fresh `bufio.Reader` — so a menu's parked pump and the following prompt consume the *same* reader, and the pump's pending byte is delivered to the prompt instead of being stolen. Prompt rendering is unchanged: `PromptWithDefault` prints `"%s [%s]: "` to **stdout**; `ConfirmYesNo` prints `"%s [Y/n] "` to **stderr**. Only the *reader* seam changed.
- **No raw mode for line prompts.** The session methods do NOT call `term.MakeRaw` — line prompts run in cooked mode exactly as the package-level functions do (the kernel line-buffers and echoes). Raw mode remains entered/restored **per `Show()` call only**, per the deliberate `MenuSession` doc-comment contract (§ Raw mode is restored on every exit path). This is why `readLine` reassembles bytes from the same pump the key reads use rather than reading keys in raw mode.
- **`readLine()` on `blockingByteReader`** (`menu.go`) is the line-read mechanism: it accumulates bytes via `readByteBlocking()` (the same single-pump channel path the key reader drains) until a `'\n'`, then strips a trailing `"\r"` via `strings.TrimSuffix(line, "\r")` so `"\r\n"` and `"\n"` both yield the bare line. It returns `(line, true)` on a complete line and `("", false)` on read failure/EOF before any newline — **partial pre-EOF input is discarded**, matching the `err != nil` short-circuit in the package-level prompt functions. Keeping the accumulation on `blockingByteReader` preserves the single-reader invariant (exactly one outstanding `ReadByte` on the fd at any moment) and is unit-testable via the `sharedStream` seam without a PTY.
- **Interactive-mode `TrimSpace` parity with the fallback path.** Both session methods `strings.TrimSpace` the line *after* `readLine`'s CRLF strip, so interactive and fallback modes behave identically on padded input (`"  "` → default; `" y"` → yes). The package-level functions also `TrimSpace`; the session methods trim too so the two paths never diverge.
- **EOF/empty semantics match the package-level functions exactly.** On read failure/EOF before a newline: `PromptWithDefault` → `defaultValue`, `ConfirmYesNo` → `false`. On empty (or, after trimming, whitespace-only) line: `PromptWithDefault` → `defaultValue`, `ConfirmYesNo` → `true` (the default). `ConfirmYesNo`'s answer parse is `strings.HasPrefix(strings.ToLower(line), "y")`.
- **Shared prompt-format constants + `parseYesNoLine`.** The prompt-text formats live in package-level constants `promptWithDefaultFmt = "%s [%s]: "` (stdout) and `confirmYesNoFmt = "%s [Y/n] "` (stderr), and the Y/n grammar lives in `parseYesNoLine(line string) bool` (empty → `true`; else prefix-`y`). Both the package-level functions and the `MenuSession` methods reference these, so the prompt text and answer grammar cannot drift between the standalone and session-aware paths.
- **Fallback mode delegates to the package-level functions (byte-for-byte).** When the session is not interactive (`!s.interactive` — non-TTY / Windows), `MenuSession.PromptWithDefault`/`ConfirmYesNo` delegate to the package-level `PromptWithDefault`/`ConfirmYesNo` so piped-stdin behavior is byte-for-byte identical to the standalone functions (fresh `bufio.NewReader(os.Stdin)`, same prompt strings, same streams). `s.interactive` is decided once at `NewMenuSession` from TTY detection; a **raw-mode-entry failure does NOT flip the session to fallback** — it degrades only the affected `Show` call. (The fallback *menu* reader, `runFallbackMenu`, carries the structured EOF refusal and partial-line handling — 260717-6end, see § Non-TTY EOF refusal and § Partial-line-at-EOF is honored; the *line-prompt* delegation described in this bullet is byte-for-byte.)
- **Package-level `PromptWithDefault`/`ConfirmYesNo` keep their fresh-`bufio` cooked-mode bodies — standalone-only.** They are NOT reimplemented as one-shot-`MenuSession` wrappers: a cooked-mode `ReadString('\n')` completes synchronously and leaves no parked goroutine, so standalone use is safe, whereas a one-shot session would orphan a pump and reintroduce theft in the prompt→next-reader direction. Their doc comments carry the constraint: **flows that mix menus and line prompts on the same stdin MUST use the `MenuSession` variants.** (After this change their only remaining callers are the session methods' fallback delegation; they are retained deliberately, not deletable.)

### `wt create` joins the session-threading pattern

- `wt create`'s `RunE` (`src/cmd/wt/create.go`) creates ONE `session := wt.NewMenuSession()` with `defer session.Close()` near the top (before the dirty-state check — the first interactive consumer), and routes **all five** interactive stdin consumers through it: the dirty-state menu (`session.Show`), the "Worktree name" prompt (`session.PromptWithDefault`), the "Initialize worktree?" confirm (`session.ConfirmYesNo`), the "Continue and open the worktree anyway?" confirm (`session.ConfirmYesNo`), and the "Open in:" app menu (`session.Show`) (260708-wryx). There are no one-shot `wt.ShowMenu` calls in `create.go`.
- This guards **two** seams at once: the menu→line-prompt theft (dirty-state menu → name prompt) AND the menu→menu theft (the dirty-state menu's orphaned pump stealing from the later "Open in:" menu — the exact menu→menu class `MenuSession` guards for the `wt go --open prompt` selection→app-menu chain). `wt create` matches the `wt go` / `wt delete` session-threading pattern.
- **`cmd/` stays orchestration-only** (Constitution Principle V): the line-reading logic lives entirely in `internal/worktree/menu.go`; `create.go`'s edits are limited to constructing the session, swapping call sites to session methods, and the warning-string edit — no new business logic in `cmd/`.
- **Dirty-state warning copy** is `wt.Warn("current worktree has uncommitted changes")`. The `HasUncommittedChanges()` / `HasUntrackedFiles()` checks run in the process CWD — whatever worktree (linked or main) the user is standing in — so the copy must describe the *current worktree/checkout*, never "main repo". The warning exists because uncommitted work doesn't carry over to the new worktree, and the "Stash changes first" option remains valid (the git stash is repo-global). The stream is unchanged (`wt.Warn` → stderr, per the stdout=machine / stderr=human convention — see `/wt-cli/create-output-phases.md`, the canonical stream-discipline file that owns the `wt.Warn` emitter).

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

An `ErrUserAbort` typed error would force every one of the ~11 `ShowMenu` call sites in `src/cmd/wt/{create,delete,open,go}.go` to add a new branch — for zero observable UX gain over the existing `if choice == 0 { return }` pattern. Preserving the integer-return contract was the lowest-risk path with the largest backward-compatibility win. (Source: intake Q3 + spec Design Decisions #2.)

### Digit keys submit immediately (no Enter)

Today's fallback path treats `3<Enter>` as "select option 3"; in the interactive path, typing `3` submits option 3 immediately. This preserves the muscle memory of every user who learned the numbered prompt before this change. Arrow-key navigation is the slower-but-more-discoverable path. Digit-highlights-only-then-Enter was rejected as adding a step for users who already know what they want. (Source: spec Design Decisions #3.)

### In-place redraw over alternate-screen buffer

Alternate-buffer mode (`\x1b[?1049h`) is the right call for a fullscreen TUI but wrong scope for a per-prompt widget. Cursor-up + `\x1b[2K` redraws only the menu region and leaves prior shell output in scrollback. Same approach used by `gum choose` and `gh`'s interactive selectors. (Source: spec Design Decisions #4.) **Re-affirmed by `260705-3wr8`**: the scrolling-viewport fix for over-tall menus was built *inside* the cursor-up in-place redraw (windowing keeps `rowsRendered ≤ height − 1`), so switching to the alternate buffer was reconsidered and rejected again — the per-prompt-widget scope is preserved and prior shell output still stays in scrollback.

### Wrap-around at edges

`↓` from the last row landing on `Cancel` (and wrapping past `Cancel` to the first option) is the industry-standard picker behavior. Hard stops feel broken on short lists like the typical worktree picker. (Source: spec Design Decisions #5.)

### Pure `nextMenuState` state machine for testability

Real-PTY tests are slow, flaky, and OS-specific. Extracting the state machine as a pure function tests every keybinding deterministically in microseconds and works identically on Linux, macOS, and Windows CI. The raw I/O shell is intentionally a thin composition layer around `parseKey` and `nextMenuState`. (Source: spec Design Decisions #6 + constitution Principle IV "Test what the user sees".)

### Windows scoped out of interactive raw-mode (path b)

Of the two options the spec allowed (path a: wire raw-mode on Windows ConPTY; path b: fall back to numbered prompt on Windows), path b was chosen. ConPTY raw-mode quirks (line-buffering on certain terminals, key-code differences) make full Windows arrow-key support a non-trivial side quest. The Linux/macOS UX win ships now without blocking on Windows-specific debugging. (Source: spec Cross-Platform requirement + T010.)

### `runInteractiveMenuCore` extracted as a test seam (T017)

This is a **pattern worth carrying to future changes**: when a function combines raw-mode I/O (which is hostile to testing) with a pure-ish loop body (which is easy to test), extract the loop body into a `*Core` helper that accepts `io.Reader` / `io.Writer` for streams and `func()` for the restore action. The outer function wires real OS handles + a `term.Restore` closure; tests inject a panicking reader + a counter-based fake restore to verify the deferred-restore guarantee. The seam is internal-only (lowercase, unexported); `ShowMenu`'s public signature did not change. This is the canonical pattern for "I need to test that `defer` fires on panic, but I can't open a real terminal in CI." Future changes that introduce similar raw-mode/signal-handling code in `wt` should reach for this seam shape first. (Source: T017 + Acceptance A-011/A-028.)

### `paintMenu` and `redrawMenu` share `renderRows` (T018 refactor)

Originally `paintMenu` and `redrawMenu` had two parallel rendering paths — the redraw path additionally emitted `\x1b[2K` per line. The T018 refactor extracted `renderRows(w, prompt, options, st, linePrefix, height)` as the shared core: `paintMenu` calls it with `linePrefix=""`, `redrawMenu` writes the cursor-up prelude then calls it with `linePrefix=ansiClearLine`. (The `height` argument was added later by `260705-3wr8` for the scrolling viewport; both callers pass the same value.) The row content is byte-identical between first paint and redraw; `TestPaintAndRedrawShareCore` strips the prelude/clears and asserts byte-equality. No behavioral change from the refactor itself, and it eliminates the drift risk where the two paths could diverge on a future highlight-marker tweak — a property `260705-3wr8` leaned on directly, since the viewport slicing lives entirely inside the shared `renderRows`.

### Pure `menuLayout` as the windowing seam

The viewport arithmetic lives in a pure `menuLayout(numOptions, highlight, prevTop, height)` function rather than inside `renderRows` reading `term.GetSize` directly. — *Why*: matches the existing `nextMenuState` / `parseKey` / `initialHighlight` pure-function discipline (Constitution Principle IV) — every windowing edge case (window at top/bottom, both indicators, highlight-driven shifts, wrap jumps, degenerate short terminals, 0-option menus) is unit-testable without a PTY, and the footprint invariant can be swept exhaustively (`TestMenuLayout_FootprintSweep`). — *Rejected*: reading `term.GetSize` inside `renderRows` (untestable, hidden I/O), and a struct return (heavier than named returns for a package-internal helper). The height source is injected as `heightFn func() int` on `runInteractiveMenuCore`, mirroring the existing `restoreFn` injection — *rejected*: a global var or package-level hook (hidden state).

### Indicators consume the budget; drop chrome before overflowing it

Overflow indicators (`↑ N more` / `↓ N more`) spend one option-region row each, so `rowsRendered ≤ height − 1` holds and the cursor-up redraw stays sound. On a cramped viewport (heights 4–5) where indicators plus one option would exceed the budget, the chrome is **dropped** (↓ first, then ↑) so the option wins over the hint (260705-3wr8). — *Why*: the footprint bound is load-bearing (a full-height `\r\n`-terminated region scrolls the terminal on every repaint — the exact failure the windowing prevents); showing the option over the "N more" hint is fzf's cramped-viewport behavior, and the hint text *is* the indicator row's payload so it cannot be surfaced without spending the row. — *Rejected*: an in-layout `v<1→v=1` clamp (fabricates a row the budget never allocated and overshoots at heights 4–5); reporting the true hidden count without rendering the row (impossible — the count is the row).

### Extend `MenuSession` to line prompts rather than kill the pump or add a singleton

The pump is **not** cancelled or killed after each menu — a blocking `ReadByte` on a TTY fd is not interruptible without closing stdin — so the single-shared-reader design is the only correct shape, extended to *all* stdin consumers within one interactive flow (line prompts, not just menus) (260708-wryx). — *Why*: reuses the mechanism the codebase already built, tested, and documented for the menu→menu byte-theft class; a shared reader guarantees at most one reader on the fd, so no keystroke is stolen at any seam. — *Rejected*: killing the pump after each menu (can't cleanly cancel a blocking TTY read), a global singleton reader (hidden state, against Constitution I).

### Methods on `MenuSession`, and package-level prompts kept standalone-only

The line prompts are **methods** on `MenuSession` (`session.PromptWithDefault`, `session.ConfirmYesNo`) mirroring `session.Show`, not free functions taking a session parameter — the conventional Go shape for shared-resource access. The package-level `PromptWithDefault`/`ConfirmYesNo` are **kept with their fresh-`bufio` cooked-mode bodies** for standalone use rather than rewritten as one-shot-session wrappers. — *Why*: a one-shot session would spin up a pump whose orphan reintroduces theft in the prompt→next-reader direction, so the standalone functions must stay pump-free; a cooked-mode synchronous `ReadString` is safe standalone. Their doc comments now direct mixed menu+prompt flows to the session variants. — *Rejected*: free functions (`PromptWithDefaultSession(s, …)` — noisier, inconsistent with `Show`); one-shot-session wrappers (reintroduce the bug).

### `readLine` on `blockingByteReader`; shared prompt constants + `parseYesNoLine`

The line-read helper lives on `blockingByteReader` (`readLine`), accumulating via `readByteBlocking()` (260708-wryx). — *Why*: the pump already delivers one byte at a time through the single-reader channel, so accumulating there keeps the single-reader invariant and is directly unit-testable via the `sharedStream` seam without a PTY (the same seam the arrow-key state machine uses). The prompt-text formats and the Y/n grammar live in package-level constants (`promptWithDefaultFmt`, `confirmYesNoFmt`) and a `parseYesNoLine` helper shared by both the package-level functions and the `MenuSession` methods — *why*: the interactive and standalone paths must be behaviorally identical, and sharing the constants/parser makes prompt text and answer grammar impossible to drift between them. — *Rejected*: duplicating the prompt strings / parse logic across the two paths (drift risk).

## Cross-references

- Spec doc: none — interactive prompt UX is per-prompt-widget behavior, not part of `docs/specs/cli-surface.md`'s per-subcommand flag surface. This contract lives in memory only.
- Source: `src/internal/worktree/menu.go` — `ShowMenu`, `isInteractiveTTY`, `runInteractiveMenu`, `runInteractiveMenuCore` (now takes `heightFn func() int`), `paintMenu`, `redrawMenu`, `renderRows` (now take `height int`), `nextMenuState`, `parseKey`, `initialHighlight`, `menuState` (now carries `top`), `keyEvent`, `menuStateTransition`, named ANSI / key constants. Windowing (`260705-3wr8`): `menuLayout`, `optionRowsForTop`, `dropUnaffordableIndicators`, `terminalHeight`, `writeIndicatorRow`, and the constants `defaultTerminalHeight`, `menuOverheadRows`, `indicatorUp`, `indicatorDown`. Line-prompt seam (`260708-wryx`): `MenuSession.PromptWithDefault`, `MenuSession.ConfirmYesNo`, `blockingByteReader.readLine`, the package-level `PromptWithDefault`/`ConfirmYesNo` (kept standalone-only), the shared `parseYesNoLine` helper, and the `promptWithDefaultFmt` / `confirmYesNoFmt` prompt-format constants. Non-TTY EOF refusal (`260717-6end`): `runFallbackMenu`'s `ReadString` error branch — the `errors.Is(err, io.EOF)` guard, the `WtError`-wrapped refusal, and the partial-line-at-EOF fall-through (the `errors` package import was added).
- Tests: `src/internal/worktree/menu_test.go` — `nextMenuState` table-driven coverage (every keybinding, wrap-around, digit boundaries, seeding), `parseKey` table-driven coverage (arrow sequences, bare-Esc-vs-arrow timeout via fake clock, unknown sequences), fallback-path byte-equality tests, `TestRunInteractiveMenuCore_PanicRestore` (defer-restore guarantee), `TestPaintAndRedrawShareCore` (first-paint / redraw byte-equality). Windowing (`260705-3wr8`): `TestMenuLayout`, `TestMenuLayout_FootprintSweep` (exhaustive `rowsRendered ≤ height−1` sweep, heights 1–12 × options 1–25), `TestMenuLayout_WrapJumpFromTopToBottom`, `TestTerminalHeightFallback` (24-row `GetSize`-failure fallback), `TestRenderRows_WindowedSlicing`, `TestRenderRows_NoIndicatorsAtEdges`, `TestRenderRows_SmallMenuByteIdenticalToUnwindowed`, `TestRedrawMenu_ClearsExtraLinesOnShrink`. Line-prompt seam (`260708-wryx`) in `src/internal/worktree/menu_session_test.go`: `TestUnderlyingReadAhead_MenuToLinePromptTheft` (characterizes the two-reader theft), `TestMenuSession_LinePromptAfterMenuNoTheft` (regression guard — `session.Show` then `session.PromptWithDefault` on one `sharedStream` delivers the typed line intact), `TestBlockingByteReader_ReadLine` (multi-byte / empty / CRLF-strip / EOF-before-newline), `TestMenuSession_PromptWithDefault_Semantics` and `TestMenuSession_ConfirmYesNo_Semantics` (default-on-empty, y/n parsing, EOF paths — via the injected-`sharedStream` seam, no PTY). The corrected warning string is asserted in `src/cmd/wt/create_test.go` `TestCreate_DirtyStateWarningCopy` (dirty repo, piped-stdin fallback menu, `3\n` Abort so no worktree leaks). Non-TTY EOF refusal (`260717-6end`): `TestShowMenu_FallbackPath_EOFNoInputActionableError` (EOF-no-input → non-nil `Error:`/`Why:`/`Fix:` refusal naming the escape, not `reading input: EOF`) and `TestShowMenu_FallbackPath_PartialLineNoNewlineAtEOF` (a choice typed without a trailing newline before EOF is still honored) in `menu_test.go`; the integration guard `TestIntegration_NonTTYMenuActionableRefusal` in `src/cmd/wt/integration_test.go` pins `wt open` / `wt go` / `wt delete` with empty stdin to exit 1 with the refusal on stderr and no `reading input: EOF`.
- Constitution: Principle I (Single-Binary CLI — motivated rejecting third-party TUI deps; `260708-wryx` rejected a global singleton reader on the same no-hidden-state grounds), Principle IV (Test What the User Sees — motivated the pure state-machine seam and the PTY-free `sharedStream`/`readLine` tests), Principle V (Internal Package Boundary — `260708-wryx` kept all line-reading logic in `internal/worktree`, `create.go` stays orchestration-only), Principle VI (Interactive by Default, Scriptable on Demand — motivated the byte-identical non-TTY fallback, preserved by the fallback-delegation of the session methods).
- Sibling memory: `wt-cli/init-failure-contract.md` (different `wt` subcommand, same post-change invariant-capture pattern), `wt-cli/list-status-contract.md` (different subcommand, same pattern).
- Sibling memory: `wt-cli/go-command-contract.md` — the `wt go` selector (which owns the which-worktree menu) and its `--open` launch composition; the chained selection→app-menu flow is `wt go --open prompt` (with the deprecated `wt open --select`/`--go` alias behaving identically), both rendering the worktree-selection menu via the shared `selectWorktree` → `ShowMenu`/`MenuSession`. `selectAndOpen` (the former `wt open` main-repo worktree picker) was removed by `260722-0is3`, so `wt open` itself renders only the "Open in:" app menu.
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md) — the conformance baseline (shll v0.0.23); the non-TTY EOF refusal above is the principle №1/№4 fix that change landed at this choke point.
- Sibling memory: `wt-cli/create-output-phases.md` — the canonical stdout/stderr stream-discipline file that owns the single `wt.Warn` emitter; the `260708-wryx` dirty-state warning-copy correction (`"current worktree has uncommitted changes"`) flows through that `wt.Warn` → stderr path (this file documents the *copy* change on the menu-scoped `wt create` flow; the stream discipline itself lives there).
- Call sites (informational): `src/cmd/wt/go.go` (the shared `selectWorktree` → `session.Show` worktree-selection menu — relocated here from `open.go` by `260722-0is3`; the `wt go --open prompt` chained selection→app-menu flow via `launchSelection`), `src/cmd/wt/open.go` (the "Open in:" app menu `handleAppMenu` / `handleAppMenuWithSession`, plus the deprecated `wt open --select`'s `openGo` chained flow; the former `selectAndOpen` worktree picker was removed by `260722-0is3`), `src/cmd/wt/delete.go` (7 calls), `src/cmd/wt/create.go` (`260708-wryx`: one `MenuSession` threaded through `RunE` across all five interactive consumers — 2 `session.Show` menus + 3 session line prompts; both former one-shot `wt.ShowMenu` calls removed).
