# Plan: Menu Viewport Scrolling

**Change**: 260705-3wr8-menu-viewport-scrolling
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. All changes are confined to the shared renderer layer in
     src/internal/worktree/menu.go; tests in src/internal/worktree/menu_test.go.
     No exported signature changes; the non-TTY fallback path is untouched. -->

### Renderer: Terminal-height-aware windowing

#### R1: Window height derives from terminal height alone
The interactive renderer SHALL cap the number of rows it paints so the total menu region (prompt row + visible option rows + up-to-two overflow-indicator rows + Cancel row) fits the terminal: for terminal heights ≥ 4 the region SHALL occupy at most H − 1 rows (the sound bound — every row ends `\r\n`, so a full-height paint scrolls one line per repaint). Heights 1–3 (below the prompt + Cancel + reserved-row overhead) are the bounded degenerate escape hatch: the region stays ≤ 3 rows (prompt + one option + Cancel, no indicators) and MAY exceed such a terminal's height, but never grows unbounded. Window height SHALL derive from the queried terminal height alone — no additional fixed row-cap constant. <!-- amended post-review c2: height−1 bound + heights-1–3 carve-out (review should-fix) -->

- **GIVEN** an interactive menu with N options rendered in a terminal of height H ≥ 4
- **WHEN** N + 2 (options + prompt + Cancel) exceeds H
- **THEN** the renderer paints at most H − 1 rows total (prompt + windowed options + indicators + Cancel)
- **AND** the returned `rowsRendered` value never exceeds H − 1, so the existing cursor-up in-place redraw stays sound and no repaint scrolls the terminal

#### R2: Terminal height is queried at paint time and injectable for tests
The renderer SHALL query terminal height with `term.GetSize` at paint/redraw time (so a terminal resize is picked up on the next keystroke's repaint, with no SIGWINCH handling). The height source SHALL be injectable into the render path — not read ad hoc deep inside it — so the windowing logic is unit-testable without a PTY.

- **GIVEN** the interactive core with an injected height provider
- **WHEN** a test supplies a fixed height
- **THEN** the windowing behavior is fully exercised without opening a real terminal
- **AND** in production the injected provider calls `term.GetSize(int(os.Stdout.Fd()))`

#### R3: `term.GetSize` failure falls back to 24 rows
On `term.GetSize` failure the renderer SHALL assume a height of 24 rows (the classic terminal default) rather than degrading to unwindowed rendering.

- **GIVEN** a height provider whose underlying `term.GetSize` returns an error
- **WHEN** the renderer acquires the height
- **THEN** it uses 24 as the terminal height
- **AND** output stays bounded (windowing still applies), never reproducing the unbounded-paint bug

### Renderer: Window offset state

#### R4: `menuState` carries a pure `top` window offset
`menuState` SHALL gain a `top` field (the index of the first option visible in the window). The adjustment of `top` SHALL be pure — computable from (previous top, new highlight, window size, option count) with no I/O — mirroring the `nextMenuState` discipline, so it is unit-testable without a terminal.

- **GIVEN** a previous window offset, a new highlight, a window size, and an option count
- **WHEN** the layout is recomputed
- **THEN** the new `top` is produced by a pure function with no I/O
- **AND** when the highlight moves past the window's edge, `top` shifts so the highlight stays visible

#### R5: Wrap-around jumps the window to keep the highlight visible
Wrap-around navigation semantics SHALL be preserved exactly (↑ from row 1 → Cancel; ↓ from Cancel → row 1; ↑ from Cancel → last option; ↓ from last option → Cancel). When the highlight wraps across the list, the window SHALL jump so the (now possibly far-away) highlighted option is visible.

- **GIVEN** a long list windowed near the top, highlight on option 1
- **WHEN** the user presses ↑ (wrapping to Cancel) then ↑ again (to the last option)
- **THEN** the window jumps to the bottom so the last option is visible
- **AND** the pure wrap-around transitions in `nextMenuState` are unchanged (highlight is still Cancel=0 / option index)

### Renderer: Row slicing and overflow indicators

#### R6: `renderRows` renders only the windowed rows with overflow indicators
`renderRows` SHALL render only the option rows inside the current window (driven by injected dimensions), emitting an `↑ N more` indicator row at the top when options are hidden above and a `↓ N more` indicator row at the bottom when options are hidden below. Indicator rows SHALL show the hidden-row count, be non-selectable rendering artifacts (never menu options), and be styled like non-highlighted rows. EXCEPTION (cramped viewports): when the option-region budget cannot fit the indicator rows plus at least one option (heights 4–5), indicator chrome is dropped (↓ first, then ↑) so the visible option wins over the hint — hidden options MAY then exist with no indicator shown (fzf precedent; the footprint bound of R1 takes priority over the hint). <!-- amended post-review c2: drop-chrome exception (review should-fix) -->

- **GIVEN** a windowed list with options hidden both above and below
- **WHEN** the menu is painted
- **THEN** an `↑ N more` row appears at the top of the option region and a `↓ M more` row at the bottom, with N and M the true hidden counts (when the budget affords the indicator rows — see the cramped-viewport exception above)
- **AND** when the window is at the top no `↑ more` row is shown; when at the bottom no `↓ more` row is shown

#### R7: Indicator rows occupy the window budget
Overflow indicator rows SHALL occupy rows within the window budget (reducing the number of visible options by up to 2 when both indicators are shown). The exact layout arithmetic is implementation detail, but `rowsRendered` (returned by `paintMenu`/`redrawMenu` and consumed by the cursor-up prelude) SHALL be the windowed total and SHALL NOT exceed the terminal height.

- **GIVEN** a window budget of B option-region rows and both indicators shown
- **WHEN** the layout is computed
- **THEN** at most B − 2 options are visible
- **AND** `rowsRendered` == 1 (prompt) + (indicators + visible options) + 1 (Cancel) ≤ terminal height

#### R8: paint/redraw byte-equality is preserved
`paintMenu` and `redrawMenu` SHALL keep delegating to the shared `renderRows` so first-paint and in-place-redraw row content stay byte-identical (the redraw path adds only the cursor-up prelude and per-line `\x1b[2K` clear). `TestPaintAndRedrawShareCore` SHALL be updated to drive the windowed render path while still asserting the byte-equality property.

- **GIVEN** the same menuState and injected dimensions
- **WHEN** `paintMenu` and `redrawMenu` render
- **THEN** stripping the redraw prelude and per-line clears yields output byte-identical to the paint output

### Renderer: Degenerate and preserved behaviors

#### R9: Degenerate terminals and empty menus stay bounded and panic-free
The windowing SHALL handle degenerate inputs without panic: a terminal shorter than the fixed overhead (prompt + Cancel + indicators), and a 0-option menu. Small fixed menus (1–4 options) that fit within the window SHALL render byte-identically to today's behavior (no indicators, all options visible).

- **GIVEN** a terminal height smaller than the overhead, or a menu with 0 options
- **WHEN** the layout is computed and rendered
- **THEN** no panic occurs, at least one option row's worth of space is preserved where options exist, and output stays bounded
- **AND** GIVEN a 3-option menu in a tall terminal, WHEN painted, THEN no indicators appear and all 3 options plus prompt and Cancel render exactly as before

#### R10: Non-TTY fallback and all other contract invariants are preserved
The non-TTY numbered-prompt fallback (`runFallbackMenu`) SHALL remain byte-identical (pinned by integration tests). All other `menu-navigation-contract.md` invariants SHALL be preserved: in-place redraw without scrollback accumulation, Cancel → `(0, nil)`, raw-mode restore on every exit path, pure `nextMenuState`/`parseKey`, wrap-around, and final-line semantics. PageUp/PageDown SHALL remain `keyIgnore` (no window-jump mapping added in this change).

- **GIVEN** either stdin or stdout is not a TTY
- **WHEN** `ShowMenu` runs
- **THEN** the historical numbered-prompt output is produced byte-for-byte (no windowing code runs)
- **AND** PageUp (`\x1b[5~`) / PageDown (`\x1b[6~`) still parse to `keyIgnore`

### Design Decisions

1. **Pure `menuLayout` function is the windowing seam**: `menuLayout(numOptions, highlight, prevTop, height) (top int, first int, count int, moreAbove int, moreBelow int)` computes the whole window from previous top + highlight + option count + terminal height, with no I/O. — *Why*: matches the existing `nextMenuState`/`parseKey`/`initialHighlight` pure-function discipline; makes every windowing edge case unit-testable without a PTY (intake §2, §7; assumption 7). — *Rejected*: reading `term.GetSize` inside `renderRows` (untestable, violates the injectable-height constraint).
2. **Height injected via a `func() int` field on the interactive core**: `runInteractiveMenuCore` takes a `heightFn func() int`; production wires `terminalHeight` (which calls `term.GetSize` on stdout and returns 24 on error), tests wire a constant. — *Why*: minimal seam, mirrors the existing `restoreFn func()` injection pattern already in the core signature (intake §1, R2, R3). — *Rejected*: a global var or a package-level hook (hidden state, harder to reason about).
3. **`top` stored on `menuState`, recomputed each transition from `menuLayout`**: the render path recomputes layout from `(highlight, top, height)` at paint/redraw time so a resize is naturally absorbed on the next keystroke. — *Why*: intake §1/§2 (resize picked up at paint time, no SIGWINCH). — *Rejected*: caching a fixed window computed once at first paint (would not adapt to resize).
4. **Indicators reduce the visible-option budget**: when hidden rows exist above/below, one option-region row is spent on each indicator. — *Why*: keeps `rowsRendered` ≤ height so cursor-up redraw stays sound (intake §3; assumption 8).

### Non-Goals

- PageUp/PageDown window-jump keybindings (assumption 10 — left ignored, out of scope).
- SIGWINCH / mid-menu resize handling beyond next-keystroke repaint (assumption 9).
- Any change to `runFallbackMenu`, `ShowMenu`/`MenuSession.Show` signatures, or call sites under `src/cmd/wt/`.

## Tasks

### Phase 1: Windowing state machine (pure, PTY-free)

- [x] T001 Add a `top` field to `menuState` in `src/internal/worktree/menu.go` (the window offset — index of the first option visible), documented alongside the existing highlight-indexing convention comment. <!-- R4 -->
- [x] T002 Implement a pure `menuLayout(numOptions, highlight, prevTop, height int) (top, first, count, moreAbove, moreBelow int)` function in `src/internal/worktree/menu.go`: compute the option-region budget from `height` minus prompt+Cancel overhead, clamp for degenerate short terminals, shift `top` to keep `highlight` visible (highlight 0 = Cancel does not constrain the option window), account for indicator rows consuming budget when rows are hidden above/below, and return the visible slice `[first, first+count)` plus hidden-row counts. Handle 0-option and tiny-height inputs without panic. <!-- R1 R4 R5 R7 R9 --> <!-- rework: budget off-by-one — region fills exactly `height` rows but every row ends \r\n so the screen footprint is height+1; each repaint scrolls one line and the prompt is lost to scrollback (PTY-reproduced). Reserve one more row so rowsRendered ≤ height−1 (e.g. budget := height − menuOverheadRows − 1) --> <!-- rework cycle 2: the height−1 bound still fails at heights 4–5 — when indicator deductions drive visible options below 1, visibleFor's v<1→v=1 clamp fabricates a row the budget never allocated (height 5 both-indicators → 5 rows = full height, scrolls again; height 4 → 5 rows > height; brute-force confirmed, all violations at heights 4–5, overshoot exactly 1). Fix: drop indicator rows before clamping — when budget − indicators < 1, sacrifice indicator chrome so count == budget and the bound holds down to height 4 (prefer showing the option over chrome, as fzf does). Also correct Assumptions row 7's boundary claim (menuOverheadRows+2 ignores indicators — arithmetically wrong) -->

### Phase 2: Height acquisition + injection

- [x] T003 Add a `terminalHeight() int` helper in `src/internal/worktree/menu.go` that calls `term.GetSize(int(os.Stdout.Fd()))` and returns 24 on error (classic default); add a named constant `defaultTerminalHeight = 24`. <!-- R2 R3 -->
- [x] T004 Thread an injectable `heightFn func() int` into `runInteractiveMenuCore` (new parameter) and its caller `MenuSession.showInteractive`; production (`Show`) wires `terminalHeight`, so the render path receives height via injection rather than reading it ad hoc. Update the `runInteractiveMenuCore` doc comment to describe the new parameter. <!-- R2 -->

### Phase 3: Windowed rendering

- [x] T005 Update `renderRows(w, prompt, options, st, linePrefix, height int)` in `src/internal/worktree/menu.go` to call `menuLayout`, render only the windowed option slice, emit `↑ N more` / `↓ N more` indicator rows (styled like non-highlighted rows, non-selectable) when rows are hidden above/below, and keep the prompt + Cancel rows. Add a `writeIndicatorRow` helper for the indicator rows. <!-- R6 R7 -->
- [x] T006 Update `paintMenu` and `redrawMenu` in `src/internal/worktree/menu.go` to accept/propagate `height`, delegate to the windowed `renderRows`, and return the *windowed* `rowsRendered` (1 prompt + indicators + visible options + 1 Cancel), which never exceeds the terminal height. Keep both delegating to the shared `renderRows` so byte-equality holds. <!-- R1 R7 R8 -->
- [x] T007 Update the `runInteractiveMenuCore` paint/redraw call sites and `finalizeMenu` to acquire height via `heightFn` at paint/redraw time, store/propagate `top` on `menuState`, and pass the windowed `rowsRendered` through the cursor-up prelude so redraw and finalize stay aligned with the windowed row count. <!-- R1 R5 R8 -->

### Phase 4: Tests

- [x] T008 [P] Add `TestMenuLayout` table-driven tests in `src/internal/worktree/menu_test.go`: window at top (no `↑ more`), window at bottom (no `↓ more`), both indicators mid-list, highlight-driven shifts up and down, wrap-around jump (highlight wraps → window jumps), terminal shorter than overhead (degenerate), and 0-option menu. Assert returned `top`/`first`/`count`/`moreAbove`/`moreBelow`. <!-- R1 R4 R5 R7 R9 --> <!-- rework: the ≤-height invariant (menu_test.go:679–691) pins the wrong bound — must assert rowsRendered ≤ height−1 so a full-height paint (which scrolls) fails the test --> <!-- rework cycle 2: the invariant guard `tc.height >= menuOverheadRows+2` claims the bound from height 4 but the table has NO windowed height-4/5 case (cases jump from 1–2 to 8+), so the violation passes silently. Add honest windowed cases at heights 4 and 5 (both-indicator states) and recalibrate the guard to the boundary the fixed code actually satisfies -->
- [x] T009 [P] Add `TestTerminalHeightFallback`-style coverage in `src/internal/worktree/menu_test.go` for the `term.GetSize`-failure path: verify the 24-row default is used when the injected height source fails (test the fallback via the pure layout path or a height-fn wrapper — no PTY). <!-- R3 -->
- [x] T010 [P] Add render-slicing tests in `src/internal/worktree/menu_test.go` that call `renderRows`/`paintMenu` with a fixed small height and a long option list, asserting: only the windowed options appear, indicator rows carry the correct hidden counts, indicators are absent at the respective edges, and small menus in a tall terminal render byte-identically to the pre-change output (no indicators). <!-- R6 R7 R9 -->
- [x] T011 Update `TestPaintAndRedrawShareCore` in `src/internal/worktree/menu_test.go` to pass the new `height` argument (a value forcing windowing) and still assert paint/redraw byte-equality after stripping the redraw prelude/clears. <!-- R8 --> <!-- rework: update expectations for the height−1 budget; also fix the stale doc comment (menu_test.go:557–560) describing height 4 / 3 options when the test uses height 6 / 5 options (review should-fix) -->

### Phase 5: Verify preserved behavior

- [x] T012 Run `cd src && go test ./internal/worktree/` and `go test ./cmd/` to confirm the existing `nextMenuState`/`parseKey`/panic-restore/fallback + integration tests still pass unchanged (non-TTY fallback byte-identical, PageUp/PageDown still ignored), then `gofmt -l internal/worktree/menu.go internal/worktree/menu_test.go` from the `src/` module root and fix any output. <!-- R8 R10 -->

## Execution Order

- T001, T002 (Phase 1) are prerequisites for T005–T007 (rendering) and T008 (layout tests).
- T003, T004 (Phase 2) are prerequisites for T004's caller wiring and T007's paint-time height acquisition.
- T005 depends on T002 (`menuLayout`); T006 depends on T005; T007 depends on T004 + T006.
- Phase 4 tests (T008–T011) depend on the code they test but T008/T009/T010 are mutually independent ([P]); T011 edits an existing test.
- T012 runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: Rendered menu region never exceeds terminal height when options + overhead would overflow; `rowsRendered` ≤ terminal height. — Met (rework cycle 2): `menuLayout` now drops indicator chrome the budget cannot afford, so the footprint holds at `rowsRendered ≤ height−1` down to the honest boundary height 4. Brute-force sweep (heights 1–12 × options 1–25 × all highlights × all prevTops, `TestMenuLayout_FootprintSweep`) reports zero footprint violations at height ≥ 4; the sole overshoot zone is heights 1–3 (raw budget < 1, ≥1-option escape hatch — bounded, non-empty, exempt).
- [x] A-002 R2: Terminal height is queried at paint time via an injectable seam (`heightFn`), exercised by tests without a PTY.
- [x] A-003 R3: `term.GetSize` failure falls back to a 24-row height; output stays bounded.
- [x] A-004 R4: `menuState.top` exists and its adjustment is a pure function (`menuLayout`) with no I/O.
- [x] A-005 R5: Wrap-around navigation preserves exact highlight semantics and jumps the window to keep the highlight visible.
- [x] A-006 R6: `renderRows` renders only windowed options with `↑ N more` / `↓ N more` indicators showing correct hidden counts.
- [x] A-007 R7: Indicator rows consume window budget; visible options reduced by up to 2 when both shown.
- [x] A-008 R8: `paintMenu`/`redrawMenu` byte-equality preserved and asserted by the updated `TestPaintAndRedrawShareCore`.

### Behavioral Correctness

- [x] A-009 R1: A menu with N+2 > H rows scrolls the highlight into view instead of painting all rows and scrolling the terminal on every keystroke. — Met (rework cycle 2): the both-indicator overshoot at heights 4–5 is fixed by dropping chrome before the budget is exceeded (height 5 mid-list now renders 4 rows, not 5; height 4 renders 3, not 5). `TestMenuLayout` now carries honest windowed height-4/5 cases (which fail the pre-fix code — verified: pre-fix rendered 5 rows at height 5, the original scroll bug), and `TestMenuLayout_FootprintSweep` guards the invariant exhaustively. `rowsRendered ≤ height−1` holds for every height ≥ 4.
- [x] A-010 R9: Degenerate short terminal and 0-option menu render without panic and stay bounded.
- [x] A-011 R9: A small (1–4 option) menu in a tall terminal renders byte-identically to the pre-change output (no indicators).

### Scenario Coverage

- [x] A-012 R1 R6: `TestMenuLayout` covers window-at-top, window-at-bottom, both-indicators, shift-up, shift-down, wrap-jump, degenerate, 0-option cases.
- [x] A-013 R3: A test verifies the 24-row `GetSize`-failure fallback path.
- [x] A-014 R6 R7: Render-slicing tests assert windowed options and indicator counts at a fixed small height.

### Edge Cases & Error Handling

- [x] A-015 R9: Terminal shorter than prompt+Cancel+indicators overhead is handled without panic (clamped).
- [x] A-016 R10: Non-TTY fallback output is byte-identical (existing fallback + integration tests pass unmodified).
- [x] A-017 R10: PageUp (`\x1b[5~`) / PageDown (`\x1b[6~`) still parse to `keyIgnore`.

### Code Quality

- [x] A-018 Pattern consistency: New code follows the existing pure-function seam pattern (`nextMenuState`/`parseKey`/`initialHighlight`) and naming/structural conventions of `menu.go`.
- [x] A-019 No unnecessary duplication: `paintMenu`/`redrawMenu` continue to share `renderRows`; the indicator-row writer reuses the existing row-writing style; no reimplementation of existing helpers.
- [x] A-020 Readability over cleverness: `menuLayout` is a focused function with named constants instead of magic numbers (e.g., `defaultTerminalHeight`, overhead counts). (rework c2: the drop-chrome fix pushed the body past the 50-line threshold, so the window-fitting arithmetic was extracted to `optionRowsForTop` and the chrome-dropping to `dropUnaffordableIndicators` — exactly the "split if it grows" the c1 note anticipated. `menuLayout` is now ~51 code lines with the two edge-case computations named and documented.)
- [x] A-021 Composition over inheritance: windowing is composed from pure layout + thin render wiring, not by expanding the render functions with embedded I/O.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
- Tests MUST NOT leak host side-effects (code-review.md project rule) — all new tests are pure/in-process (no binary invocation, no PTY, no tmux).
- CI enforces `gofmt -l` before vet/test (memory) — run gofmt from the `src/` module root on touched files.

## Deletion Candidates

None — verified against the working-tree diff (re-review, cycle 2): the change adds windowing on top of the existing renderer without orphaning any code. All pre-existing helpers (`renderRows`, `writeOptionRow`, `writeCancelRow`, `finalizeMenu`, `runFallbackMenu`, `initialHighlight`, `nextMenuState`, `parseKey`) remain load-bearing and are called by the new paths. Code the change made redundant was removed inside this diff rather than left behind: the fixed `len(options)+2` row-count arithmetic in `paintMenu`/`redrawMenu`, the pre-window doc comments, and the cycle-1 `visibleFor` closure plus its fabricating `v<1→v=1` in-layout clamp (superseded by the cycle-2 extraction into `optionRowsForTop` + `dropUnaffordableIndicators` — `grep visibleFor` returns zero hits). Every newly added symbol has call sites (`menuLayout` ×3 in menu.go + tests, `optionRowsForTop` ×2, `dropUnaffordableIndicators` ×1, `terminalHeight`, `defaultTerminalHeight`, `menuOverheadRows`, `indicatorUp`/`indicatorDown`, `writeIndicatorRow` ×2 — verified by grep across `src/`).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Height is queried on stdout's fd (`os.Stdout.Fd()`), not stdin | Menu output is written to stdout (`w` is `os.Stdout` in production); the rendered region lives on the stdout terminal, so its height governs windowing. Both are the same TTY in the interactive path (both must be TTYs per `isInteractiveTTY`). | S:80 R:85 A:90 D:85 |
| 2 | Confident | Indicator rows render as a plain (non-highlighted, non-default) styled row with text `↑ N more` / `↓ N more`, using the same gutter/formatting family as option rows via a dedicated `writeIndicatorRow` | Intake assumption 11 (Confident) fixes the text and non-selectable/plain-styling; a dedicated writer keeps `renderRows` readable and avoids duplicating the `\r\n` raw-mode discipline | S:55 R:90 A:70 D:65 |
| 3 | Confident | `menuLayout` signature returns `(top, first, count, moreAbove, moreBelow)` and is the single windowing seam threaded into `renderRows`; height flows in as an `int` param to `renderRows`/`paintMenu`/`redrawMenu` | Matches the existing pure-seam pattern and the injectable-height constraint (intake §1/§2/§7); alternative of a struct return is heavier for a package-internal helper | S:65 R:80 A:80 D:70 |
| 4 | Confident | The window is computed to keep the highlighted *option* visible; when highlight is Cancel (0) the option window is left as-is (Cancel is always rendered outside the option region) | Cancel is a fixed overhead row rendered after the option region, so it is always visible regardless of `top`; only option highlights drive window shifts | S:60 R:85 A:80 D:70 |
| 5 | Confident | When both indicators would be shown but the terminal is extremely short, the layout clamps to render at least one option row (where options exist) rather than zero, trading strict ≤-height for non-empty output on pathological sizes | Degenerate-terminal case (intake §7) is unspecified on exact clamp; keeping at least one option visible is the least-surprising behavior and still far more bounded than the bug; purely cosmetic on realistic terminals | S:35 R:85 A:60 D:45 |
| 6 | Certain | (rework c1) Reserve one row in `menuLayout`'s budget (`height - menuOverheadRows - 1`, clamped ≥1) so `rowsRendered ≤ height − 1` and the `\r\n`-terminated region never scrolls the terminal on repaint | Directly implements the review must-fix / A-009 fix (PTY-reproduced scrollback leak); the sound cursor-up in-place-redraw bound is height−1 because the bottom screen row's trailing newline scrolls; degenerate clamp preserved | S:95 R:75 A:90 D:90 |
| 7 | Confident | (rework c2, supersedes c1) The honest footprint boundary is `height ≥ 4` (== `menuOverheadRows+2`, the smallest height whose raw budget `height−menuOverheadRows−1` is ≥ 1). At/above it the drop-chrome logic guarantees `indicators + count ≤ budget` ⇒ `rowsRendered ≤ height−1`; the `TestMenuLayout` ≤-height-1 invariant is asserted only there. Heights 1–3 (raw budget < 1, clamped to 1) are exempt — the ≥1-option escape hatch keeps output bounded/non-empty but necessarily taller than height−1. The c1 rationale ("prompt + Cancel + reserved + 1 option") was arithmetically wrong: it ignored indicator rows, which is exactly why the c1 code overshot at heights 4–5. The boundary value (4) is unchanged; only its cause is corrected — the bound is now honored by dropping chrome, not by the overhead-plus-one-option count. | Verified by `TestMenuLayout_FootprintSweep` (heights 1–12 × options 1–25 × all highlights × all prevTops): zero footprint violations at height ≥ 4, all overshoots confined to heights 1–3. | S:90 R:80 A:90 D:88 |
| 8 | Confident | (rework c1) redrawMenu shrink hardening clears the trailing (old − new) stale lines then moves the cursor back up by that delta, keeping the returned windowed count an accurate cursor-up target; guarded on `extra > 0` | Review should-fix; resize is a declared non-goal so this is minimal robustness (no full resize handling); PTY-free unit test asserts the clear count and cursor-back sequence; byte-equality test unaffected since extra==0 for equal-height paint/redraw | S:55 R:85 A:80 D:70 |
| 9 | Confident | (rework c2) When the budget cannot fit indicator rows plus ≥1 option, drop indicator chrome (↓ first, then ↑) via a post-hoc loop after `moreBelow` is computed — reporting `moreAbove/moreBelow == 0` so `renderRows` omits the row — rather than overflowing the budget. Dropped-chrome cases thus report a hidden count of 0 even though options are hidden (the "N more" hint is sacrificed on a cramped viewport, as fzf does). The shift loops are unchanged; the window-fitting arithmetic lives in `optionRowsForTop` (the extracted successor of the c1 `visibleFor` closure); the drop loop is the sole budget enforcer, keeping the indicator-budget logic in one place. | Directly implements the c2 must-fix (both reviewers converged on drop-chrome). The alternative — reporting the true hidden count without rendering the row — is impossible (the count IS the row's payload). Verified byte-identical to the c1 code for all heights ≥ 6 (only heights 1–5 change), so no passing test regresses; sweep confirms the footprint + highlight-visibility invariants hold. | S:88 R:80 A:88 D:85 |

9 assumptions (2 certain, 7 confident, 0 tentative).
