# Intake: Menu Viewport Scrolling

**Change**: 260705-3wr8-menu-viewport-scrolling
**Created**: 2026-07-05

## Origin

Synthesized from a live design conversation with the user (created via promptless dispatch — decisions below were confirmed in that conversation; formerly-open items are recorded as graded assumptions). The user hit the bug on `wt open`'s worktree-selection menu with many worktrees, perceived as:

> "when the list becomes multi-page, arrow keys can't go back to the previous page"

The conversation diagnosed the root cause (the shared interactive menu renderer has no concept of terminal height), confirmed a fix direction (fzf/gum-style scrolling viewport inside the shared renderer), and explicitly rejected four alternatives: adopting a third-party TUI/select library, the alternate-screen buffer, literal page-based navigation, and a fixed max-row cap constant on top of terminal height.

## Why

**The pain point.** The interactive arrow-key menu renderer in `src/internal/worktree/menu.go` has no pagination, no viewport, and no `term.GetSize` call anywhere (verified: the only `term.` calls are `MakeRaw`, `Restore`, and `IsTerminal`). Every menu paints its full region at once — 1 prompt line + N option rows + 1 Cancel row (`paintMenu` returns `len(options) + 2`) — and redraws in place via `\r` + `\x1b[<rowsRendered>A` (cursor-up) + `\x1b[2K` per line (`redrawMenu` → `renderRows` with `linePrefix = ansiClearLine`). When N+2 exceeds the terminal height, three things break:

- (a) the first paint scrolls the top of the menu into scrollback;
- (b) on each highlight change, the ANSI cursor-up clamps at the top of the visible viewport and cannot reach rows in scrollback, so the repaint starts from the wrong row and re-emits all N+2 lines, scrolling the terminal again on every keystroke;
- (c) rows above the viewport can never be repainted, so pressing ↑ toward earlier entries moves the highlight index (the pure state machine `nextMenuState` is correct) but visually nothing happens.

Additionally, actual PageUp/PageDown keys (`\x1b[5~` / `\x1b[6~`) are parsed as `keyIgnore` (the CSI default branch in `parseKey`, `menu.go` ~604–610), so there is no paging escape hatch.

**Consequence of not fixing.** Any user with more worktrees than terminal rows gets a menu that visibly breaks its own in-place-redraw contract (scrollback accumulates intermediate menu states — an explicit invariant violation of `docs/memory/wt-cli/menu-navigation-contract.md`) and cannot reach earlier entries with arrow keys at all. The worktree pickers are the primary interactive surface of `wt open` / `wt go` / `wt delete`; this gets worse as worktree counts grow.

**Why this approach.** A scrolling viewport (windowing) inside the shared renderer is the standard picker behavior (fzf, gum, gh) and repairs the broken assumption at its source: `rowsRendered` never exceeds the terminal height, so the existing cursor-up in-place redraw stays sound with no change to the redraw mechanism itself. Because all interactive menus flow through the single `ShowMenu`/`MenuSession` primitive, fixing the renderer once fixes every menu with zero call-site edits.

## What Changes

All changes land in the shared renderer layer of `src/internal/worktree/menu.go`: `runInteractiveMenuCore`, `renderRows`, `paintMenu`, `redrawMenu`, and `menuState`. No exported signature changes; `ShowMenu(prompt, options, defaultIdx) (int, error)` and `MenuSession.Show` are untouched.

### 1. Terminal-height-aware window

- Query terminal height with `term.GetSize` at paint time. The rendered window is capped at (terminal height − overhead rows), where overhead is the prompt line, the Cancel row, and any indicator rows currently shown.
- **Decision (user-confirmed):** window height derives from `term.GetSize` alone — NO additional fixed row cap constant (e.g., no fzf-style default 15). Rationale: menus are transient widgets, using available height is fine, one fewer knob to document.
- Because height is (re-)queried at paint/redraw time, a terminal resize is naturally picked up on the next keystroke's repaint; no SIGWINCH handling is added (current code has none).
- If `term.GetSize` fails, fall back to assuming 24 rows (the classic terminal default) so output stays bounded — degrading to unwindowed would reproduce the bug on exactly the terminals most at risk.
- The terminal height MUST be injectable/parameterized for tests (dimension injected into the render path, not read ad hoc deep inside it), so the windowing is testable without a PTY.

### 2. Window offset in `menuState`

- Add a `top` (window offset) field to `menuState`. When the highlight moves past the window's edge, shift the window so the highlight stays visible.
- The `top`-offset adjustment is **pure state** (same discipline as `nextMenuState`): computable from (previous top, new highlight, window size, option count) with no I/O — unit-testable without a terminal.
- Wrap-around semantics are preserved exactly per the existing contract (↑ from row 1 → Cancel; ↓ from Cancel → row 1; ↑ from Cancel → last option; ↓ from last option → Cancel). When the highlight wraps across the list, the window jumps to keep it visible.

### 3. Row slicing in `renderRows` + overflow indicators

- `renderRows` renders only the rows inside the window (driven by the injected dimensions), rendering `↑ more` / `↓ more` indicator rows at the window edges when rows are hidden above/below. Indicator rows show hidden-row counts (e.g., `↑ 3 more`) and are non-selectable rendering artifacts, not menu options.
- Indicator rows occupy rows within the window budget (reducing visible options by up to 2 when both are shown) — layout arithmetic is implementation detail.
- `rowsRendered` (the value `paintMenu`/`redrawMenu` return and the cursor-up prelude consumes) becomes the *windowed* row count and never exceeds the terminal height — this is what keeps the existing cursor-up in-place redraw sound.
- The byte-equality-between-paint-and-redraw property is preserved: `paintMenu` and `redrawMenu` keep delegating to the shared `renderRows`; `TestPaintAndRedrawShareCore` (`src/internal/worktree/menu_test.go`) will need to accommodate the window slicing, but the property itself stays asserted.
- Small fixed menus (confirmations, 1–4 options) never exceed the window, so their rendered output is unchanged in practice.

### 4. PageUp/PageDown — out of scope for this change

PageUp/PageDown (`\x1b[5~` / `\x1b[6~`) remain `keyIgnore`; no window-jump mapping is added in this change. The viewport restores full arrow-key reachability, which is the reported bug; adding new keybindings expands the documented navigation contract and was deliberately left unsettled in the design conversation. Candidate follow-up via `/fab-clarify` or a later change.
<!-- assumed: PageUp/PageDown left ignored — conversation explicitly left the mapping open; status-quo default preserves contract surface, trivially added later -->

### 5. Explicitly rejected alternatives (user-confirmed this session)

- **Third-party TUI/select library** (charmbracelet/bubbletea + bubbles, charmbracelet/huh, AlecAivazis/survey [archived], manifoldco/promptui [unmaintained], ktr0731/go-fuzzyfinder): violates Constitution Principle I (single-binary, slim dep graph), which already motivated the hand-rolled `x/term` implementation. The expensive parts — raw-mode lifecycle with panic-safe restore, 50ms Esc disambiguation, shared-stdin `MenuSession` byte-theft fix across chained menus, byte-identical non-TTY fallback, Cancel→`(0, nil)` across ~11 call sites — are already built and test-pinned; a library swap would re-litigate all of it. The calculus only flips if richer interaction (fuzzy filtering, multi-select) is wanted later — out of scope here.
- **Alternate-screen buffer** (`\x1b[?1049h`): wrong scope for a per-prompt widget — existing documented decision in `docs/memory/wt-cli/menu-navigation-contract.md`, deliberately reaffirmed.
- **Literal page-based navigation**: viewport scrolling is the standard picker behavior (fzf/gum/gh).
- **Fixed max-row cap constant on top of terminal height**: unnecessary knob.

### 6. What does NOT change

- **Zero call-site edits.** All 11 interactive menus across `src/cmd/wt/{create,delete,open,go}.go` go through the single `ShowMenu`/`MenuSession` primitive and inherit the fix. The two menus that actually exhibit the bug are the worktree pickers: `selectWorktree` (`src/cmd/wt/open.go:360`, invoked at open.go:206 and open.go:420 — shared by `wt open`, `wt go`, and `wt open --go`) and the delete picker (`src/cmd/wt/delete.go:624`, `session.Show("Select worktree to delete:", ...)`).
- **Non-TTY numbered-prompt fallback is UNTOUCHED** — its byte-identical historical contract is pinned by integration tests and must remain so.
- All other `menu-navigation-contract.md` invariants are preserved: in-place redraw (no scrollback accumulation of intermediate states), Cancel returns `(0, nil)`, raw-mode restore on every exit path, pure `nextMenuState`/`parseKey` testable without a PTY, wrap-around at edges, final-line semantics (menu region replaced by a single summary line on submit/cancel). The only invariant deliberately amended is the implicit "whole menu fits on screen" assumption.
- **No new dependencies** (Constitution Principle I) — `golang.org/x/term` is already present and already provides `GetSize`.

### 7. Tests (Constitution Principle IV)

Unit tests for the windowing state machine plus the render slicing — happy path and edge cases:

- window at top (no `↑ more`), window at bottom (no `↓ more`), both indicators mid-list
- highlight-driven window shifts in both directions
- wrap-around jumps (highlight wraps across the list → window jumps to keep it visible)
- terminal shorter than overhead (degenerate: fewer rows than prompt + Cancel + indicators)
- degenerate 0-option menu
- `term.GetSize` failure fallback path
- `TestPaintAndRedrawShareCore` updated to accommodate slicing while still asserting paint/redraw byte-equality

## Affected Memory

- `wt-cli/menu-navigation-contract`: (modify) Amend the in-place-redraw section with the viewport windowing contract: window height derived from `term.GetSize` alone (no fixed cap), the `top` offset in `menuState`, `↑ N more` / `↓ N more` indicator rows within the window budget, the 24-row `GetSize`-failure fallback, wrap-around window jumps, and `rowsRendered` ≤ terminal height as the invariant that keeps cursor-up redraw sound. Record that the alternate-screen buffer was re-rejected for a per-prompt widget and that PageUp/PageDown remain ignored.

## Impact

- **Code**: `src/internal/worktree/menu.go` (renderer core: `menuState`, `runInteractiveMenuCore`, `paintMenu`, `redrawMenu`, `renderRows`, plus height acquisition/injection). Zero edits under `src/cmd/wt/`.
- **Tests**: `src/internal/worktree/menu_test.go` (new windowing/slicing tests; `TestPaintAndRedrawShareCore` accommodation). Integration tests in `src/cmd/wt/integration_test.go` are unaffected (they drive the non-TTY fallback, which is untouched).
- **Dependencies**: none added.
- **Docs**: `docs/memory/wt-cli/menu-navigation-contract.md` updated at hydrate.
- **User-visible**: menus taller than the terminal become windowed with overflow indicators; menus that fit render byte-identically to today.

## Open Questions

None blocking. Two soft preferences were left open in the design conversation and are recorded with defaults in `## Assumptions` (rows 10–12) for optional `/fab-clarify` review: whether to map PageUp/PageDown to window jumps (default: no, out of scope), and the exact indicator-row text/styling (default: `↑ N more` / `↓ N more` with counts).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Fix is a scrolling viewport (fzf/gum-style windowing) inside the shared renderer — not literal pages, not alternate-screen buffer | Discussed — user confirmed this direction explicitly; alternatives rejected in-session | S:95 R:70 A:90 D:90 |
| 2 | Certain | Window height derives from `term.GetSize` alone — no additional fixed row cap constant | Discussed — user confirmed; "menus are transient widgets, one fewer knob to document" | S:90 R:90 A:85 D:85 |
| 3 | Certain | No third-party TUI/select library — extend the hand-rolled `x/term` renderer | Constitution Principle I + user confirmed; existing raw-mode/fallback/session machinery is built and test-pinned | S:95 R:60 A:95 D:95 |
| 4 | Certain | Change confined to the shared renderer layer; zero call-site edits across the 11 menus | Discussed — all menus flow through `ShowMenu`/`MenuSession`; verified in codebase | S:90 R:80 A:90 D:90 |
| 5 | Certain | Non-TTY numbered-prompt fallback untouched (byte-identical historical contract) | Pinned by integration tests + memory contract; user-stated constraint | S:95 R:70 A:95 D:95 |
| 6 | Certain | Wrap-around semantics preserved; on wrap the window jumps to keep the highlight visible | Discussed — user confirmed; existing contract invariant | S:90 R:85 A:90 D:85 |
| 7 | Certain | Windowing stays PTY-free testable: `top` adjustment as pure state, terminal height injected into the render path | User-stated constraint; matches existing `nextMenuState`/`parseKey` seam pattern | S:85 R:75 A:90 D:85 |
| 8 | Confident | Indicator rows occupy rows within the window budget (visible options reduced by up to 2) | Conversation leaned "presumably yes"; layout arithmetic is implementation detail | S:60 R:90 A:75 D:70 |
| 9 | Confident | Terminal resize mid-menu is out of scope — no SIGWINCH; height re-queried at paint time adapts on next keystroke | Conversation leaned "likely out of scope"; current code has no resize handling | S:55 R:85 A:75 D:70 |
| 10 | Tentative | PageUp/PageDown (`\x1b[5~`/`\x1b[6~`) remain `keyIgnore` — no window-jump mapping in this change | Conversation explicitly left this open; status-quo default avoids expanding the documented keybinding contract, trivially added later | S:30 R:85 A:40 D:30 |
| 11 | Confident | Indicator text shows hidden-row counts — `↑ N more` / `↓ N more`, non-selectable, styled like non-highlighted rows | Conversation floated counts as an option; strictly more informative, purely cosmetic, trivially changed | S:40 R:95 A:60 D:50 |
| 12 | Confident | On `term.GetSize` failure, assume 24 rows (classic terminal default) rather than degrading to unwindowed | Conversation floated both options; 24-row fallback keeps output bounded — unwindowed would reproduce the bug on short terminals; failure is rare since menus only run when stdin+stdout are TTYs | S:40 R:90 A:65 D:45 |

12 assumptions (7 certain, 4 confident, 1 tentative, 0 unresolved).
