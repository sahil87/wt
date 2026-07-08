# Plan: Fix stdin byte-theft at the menu→line-prompt seam; correct mislabeled dirty-state warning in `wt create`

**Change**: 260708-wryx-menu-prompt-byte-theft
**Intake**: `intake.md`

## Requirements

### Menu Session: Session-aware line prompts

#### R1: `MenuSession.PromptWithDefault` reads through the shared reader
`MenuSession` SHALL expose a method `PromptWithDefault(prompt, defaultValue string) string` that, in interactive mode, reads a full line of input through the session's shared `blockingByteReader` (the same reader used by `Show`) rather than constructing a fresh `bufio.Reader` over `os.Stdin`. This closes the menu→line-prompt byte-theft seam: because the parked pump and the prompt consume the same reader, the pump's pending byte is delivered to the prompt instead of being stolen.

- **GIVEN** an interactive `MenuSession` whose shared reader's pump has one byte parked after a preceding `Show`
- **WHEN** `session.PromptWithDefault("Worktree name", "lively-tamarin")` is called and the user types a name followed by Enter
- **THEN** the typed line is returned intact (not swallowed), and no second reader is created on the stdin fd
- **AND** the prompt text `"%s [%s]: "` is printed to stdout unchanged

#### R2: `MenuSession.ConfirmYesNo` reads through the shared reader
`MenuSession` SHALL expose a method `ConfirmYesNo(prompt string) bool` that, in interactive mode, reads a full line through the session's shared reader and parses the answer as `strings.HasPrefix(strings.ToLower(line), "y")`. The default (empty line) SHALL be `true`.

- **GIVEN** an interactive `MenuSession` with a shared reader
- **WHEN** `session.ConfirmYesNo("Initialize worktree?")` is called and the user presses Enter (empty line)
- **THEN** it returns `true` (default yes)
- **AND** when the user types a line beginning with `y`/`Y` it returns `true`, otherwise `false`
- **AND** the prompt text `"%s [Y/n] "` is printed to stderr unchanged

#### R3: Line-read helper accumulates bytes through the pump until newline
A line-read helper (a method on `blockingByteReader`, e.g. `readLine()`) SHALL accumulate bytes via `readByteBlocking()` until a `'\n'` is seen, then strip a trailing `"\r\n"` or `"\n"`, returning the line plus an ok/error signal. On a read failure/EOF before any newline, it SHALL signal failure and discard partial input. No raw mode is entered for line reads — line prompts run in cooked mode exactly as today.

- **GIVEN** a `blockingByteReader` over a stream delivering `h`, `i`, `\n`
- **WHEN** `readLine()` is called
- **THEN** it returns `"hi"` with an ok signal
- **AND GIVEN** the stream delivers `x`, `y` then EOF (no newline), **THEN** `readLine()` signals failure and the partial `"xy"` is discarded
- **AND GIVEN** the stream delivers `a`, `\r`, `\n`, **THEN** `readLine()` returns `"a"` (CRLF stripped)
- **AND GIVEN** the stream delivers only `\n`, **THEN** `readLine()` returns `""` with an ok signal

#### R4: EOF/error and empty-line semantics match the package-level functions exactly
The session-aware prompts SHALL preserve every caller-observable behavior of the current package-level functions. On read failure/EOF before a newline: `PromptWithDefault` returns `defaultValue`; `ConfirmYesNo` returns `false`. On empty line: `PromptWithDefault` returns `defaultValue`; `ConfirmYesNo` returns `true`.

- **GIVEN** an interactive session whose reader hits EOF before any newline
- **WHEN** `session.PromptWithDefault("Name", "def")` is called
- **THEN** it returns `"def"`
- **AND WHEN** `session.ConfirmYesNo("OK?")` is called under the same EOF, **THEN** it returns `false`

#### R5: Fallback mode delegates to the package-level implementations (byte-for-byte)
When the session is not interactive (`s.interactive == false` — non-TTY or Windows; a raw-mode-entry failure does NOT flip the session to fallback, it degrades only the affected `Show` call), the session-aware methods SHALL delegate to the existing package-level `PromptWithDefault`/`ConfirmYesNo` so behavior under piped stdin is byte-for-byte identical to today (fresh `bufio.NewReader(os.Stdin)`, same prompt strings, same streams).

- **GIVEN** a `MenuSession` in fallback mode (non-TTY)
- **WHEN** `session.PromptWithDefault`/`session.ConfirmYesNo` are called
- **THEN** they produce output and read behavior byte-for-byte identical to the package-level functions
- **AND** `runFallbackMenu` and existing pinned fallback tests remain unmodified and passing

#### R6: Package-level `PromptWithDefault`/`ConfirmYesNo` retain their current implementations
The package-level `ConfirmYesNo`/`PromptWithDefault` functions SHALL keep their current fresh-`bufio.Reader` cooked-mode implementations for standalone use. They MUST NOT be reimplemented as one-shot-`MenuSession` wrappers (a one-shot session would orphan a pump and reintroduce theft in the prompt→next-reader direction). Their doc comments SHALL gain the constraint that flows mixing menus and line prompts on the same stdin MUST use the session-aware variants.

- **GIVEN** the package-level `PromptWithDefault`/`ConfirmYesNo`
- **WHEN** this change is applied
- **THEN** their bodies still construct a fresh `bufio.NewReader(os.Stdin)` and read a line synchronously (no pump, no parked goroutine)
- **AND** their doc comments direct mixed menu+prompt flows to the session-aware methods

### Create Flow: Single session threaded through `wt create`

#### R7: One `MenuSession` is created and threaded through every interactive stdin consumer in `RunE`
`wt create`'s `RunE` SHALL create a single `MenuSession` near the top (`session := wt.NewMenuSession(); defer session.Close()`) and route every interactive stdin consumer through it: the dirty-state menu (`session.Show`), the worktree-name prompt (`session.PromptWithDefault`), the "Initialize worktree?" confirm (`session.ConfirmYesNo`), the "Continue and open the worktree anyway?" confirm (`session.ConfirmYesNo`), and the "Open in:" app menu (`session.Show`). Both former one-shot `wt.ShowMenu` calls SHALL be replaced with `session.Show`. This fixes all seams at once (including the menu→menu :128→:423 theft) and makes `wt create` consistent with the `wt open`/`wt delete`/`wt go` session pattern.

- **GIVEN** a user runs `wt create` from a dirty worktree and selects "Continue anyway" on the dirty-state menu
- **WHEN** the `"Worktree name [<suggested>]:"` prompt appears and the user types a name + Enter
- **THEN** the typed name is captured by the prompt (not stolen by the dirty-state menu's orphaned pump)
- **AND** every subsequent interactive consumer in the flow reads through the same session's shared reader

#### R8: `cmd/` stays orchestration-only
The new line-reading logic SHALL live in `src/internal/worktree/menu.go`; `create.go` changes SHALL be limited to constructing the session, swapping call sites to session methods, and the warning-string edit — no new business logic in `cmd/` (Constitution Principle V).

- **GIVEN** the create.go edits
- **WHEN** reviewed against Constitution Principle V
- **THEN** all non-trivial line-reading logic resides in `internal/worktree`, and `cmd/` only parses flags, prompts, and orchestrates

### Create Flow: Dirty-state warning copy

#### R9: The dirty-state warning describes the current worktree, not "main repo"
The `wt create` dirty-state warning SHALL read exactly `"current worktree has uncommitted changes"` (via `wt.Warn`, stderr). The `HasUncommittedChanges()`/`HasUntrackedFiles()` checks run in the process CWD (any worktree, linked or main), so the copy must describe the current worktree/checkout. No behavior change to the checks themselves.

- **GIVEN** `wt create` is run interactively from a checkout with uncommitted or untracked changes
- **WHEN** the dirty-state warning is emitted
- **THEN** the warning text on stderr is `"current worktree has uncommitted changes"` (not `"main repo has uncommitted changes"`)
- **AND** the warning is written to stderr (satisfying the stdout=machine / stderr=human convention)

### Non-Goals

- `wt create` base-ref semantics — new worktree still based on current HEAD when `--base` absent (crud.go); that behavior is the *reason* the dirty warning exists, not part of this fix.
- Raw-mode-per-`Show()` lifecycle — unchanged; line prompts do NOT enter raw mode.
- Cooked-mode type-ahead buffering between consecutive line prompts (no parked reader involved) — out of scope.
- `wt delete` / `wt open` / `wt go` — untouched; the intake audit confirms they already thread a shared `MenuSession` and contain no line prompts.
- The stdout/stderr-convention cleanup of `PromptWithDefault`'s prompt stream (it prints to stdout today) is NOT taken — byte-compat mandate outweighs it for a fix-type change.
- No public CLI surface change (no flags, exit codes, or output-contract change other than the corrected warning string).

### Design Decisions

1. **Extend `MenuSession` with session-aware line prompts sharing the single stdin reader**: the pump is not cancelled/killed. — *Why*: reuses the codebase's existing, tested, documented mechanism for exactly this byte-theft bug class; a blocking `ReadByte` on a TTY fd is not interruptible without closing stdin, so a single-shared-reader design is the only correct shape. — *Rejected*: killing the pump after each menu (impossible to cleanly cancel), a global singleton reader (hidden state).
2. **Methods on `MenuSession` (`session.PromptWithDefault`, `session.ConfirmYesNo`)** rather than free functions taking a session parameter. — *Why*: mirrors the existing `session.Show` method; conventional Go shape for shared-resource access. — *Rejected*: free functions (`PromptWithDefaultSession(s, …)`) — noisier, inconsistent with `Show`.
3. **Package-level prompts keep fresh-`bufio` cooked-mode implementations, not one-shot-session wrappers.** — *Why*: a cooked-mode `ReadString('\n')` completes synchronously and leaves no parked goroutine, so standalone use is safe; a one-shot-session wrapper would orphan a pump and reintroduce theft in the prompt→next-reader direction. — *Rejected*: wrapping them in one-shot sessions.
4. **Line-read helper lives on `blockingByteReader` (`readLine`)** accumulating via `readByteBlocking()`. — *Why*: the pump already delivers one byte at a time through the single-reader channel; accumulating there keeps the single-reader invariant and is directly unit-testable via the `sharedStream` seam without a PTY.

## Tasks

### Phase 1: Core Implementation (internal/worktree/menu.go)

- [x] T001 Add a `readLine()` line-read helper on `blockingByteReader` in `src/internal/worktree/menu.go` that accumulates bytes via `readByteBlocking()` until `'\n'`, strips a trailing `"\r\n"`/`"\n"`, and returns `(line string, ok bool)` — `ok == false` on read failure/EOF before a newline (partial input discarded). <!-- R3 -->
- [x] T002 Add method `func (s *MenuSession) PromptWithDefault(prompt, defaultValue string) string` in `src/internal/worktree/menu.go`: fallback mode (`!s.interactive`) delegates to the package-level `PromptWithDefault`; interactive mode prints `"%s [%s]: "` to stdout, reads via `s.reader.readLine()`, returns `defaultValue` on `!ok` or empty line, else the trimmed line. <!-- R1 -->
- [x] T003 Add method `func (s *MenuSession) ConfirmYesNo(prompt string) bool` in `src/internal/worktree/menu.go`: fallback mode delegates to the package-level `ConfirmYesNo`; interactive mode prints `"%s [Y/n] "` to stderr, reads via `s.reader.readLine()`, returns `false` on `!ok`, `true` on empty line, else `strings.HasPrefix(strings.ToLower(line), "y")`. <!-- R2 R4 R5 -->
- [x] T004 Update the doc comments on the package-level `ConfirmYesNo`/`PromptWithDefault` in `src/internal/worktree/menu.go` to state they keep fresh-`bufio` cooked-mode reads for standalone use and that flows mixing menus and line prompts on the same stdin MUST use the `MenuSession` variants; do NOT change their bodies. <!-- R6 -->

### Phase 2: Integration (cmd/wt/create.go)

- [x] T005 In `src/cmd/wt/create.go` `RunE`, create one `session := wt.NewMenuSession()` with `defer session.Close()` near the top of the function body (before the dirty-state check), and thread it through the flow. <!-- R7 R8 -->
- [x] T006 In `src/cmd/wt/create.go`, replace the dirty-state one-shot `wt.ShowMenu("How to proceed?", …)` (create.go:128) with `session.Show(...)`, the worktree-name `wt.PromptWithDefault(...)` (create.go:176) with `session.PromptWithDefault(...)`, the `"Initialize worktree?"` `wt.ConfirmYesNo(...)` (create.go:257) with `session.ConfirmYesNo(...)`, the `"Continue and open the worktree anyway?"` `wt.ConfirmYesNo(...)` (create.go:389) with `session.ConfirmYesNo(...)`, and the `"Open in:"` one-shot `wt.ShowMenu(...)` (create.go:423) with `session.Show(...)`. <!-- R7 R8 -->
- [x] T007 In `src/cmd/wt/create.go`, change the dirty-state warning `wt.Warn("main repo has uncommitted changes")` (create.go:127) to `wt.Warn("current worktree has uncommitted changes")`. <!-- R9 -->

### Phase 3: Tests

- [x] T008 [P] In `src/internal/worktree/menu_session_test.go`: add a characterization test demonstrating menu→line-prompt theft with two independent readers (analogous to `TestUnderlyingReadAhead_DemonstratesTheft`) and a regression test asserting a `session.Show` followed by a session-aware line prompt on the same `sharedStream` delivers the typed line intact (analogous to `TestMenuSession_SharesReaderAcrossMenus`). <!-- R1 R7 -->
- [x] T009 [P] In `src/internal/worktree/menu_session_test.go` (or `menu_test.go`): add direct unit coverage for `readLine()` — multi-byte line, empty line (`\n`), `"\r\n"` stripping, and EOF-before-newline → `ok=false`. <!-- R3 -->
- [x] T010 [P] In `src/internal/worktree/menu_session_test.go`: add session-aware `PromptWithDefault`/`ConfirmYesNo` unit tests via the injected-reader seam (no PTY): default-on-empty, y/n parsing, EOF → `defaultValue`/`false`. <!-- R2 R4 -->
- [x] T011 In `src/cmd/wt/create_test.go`: add an integration test asserting the corrected dirty-state warning string `"current worktree has uncommitted changes"` appears on stderr (dirty repo, interactive/piped stdin choosing Abort so no worktree is created — host-isolation preserved). <!-- R9 -->

## Execution Order

- Phase 1 (T001–T004) before Phase 2 (T005–T007): create.go calls the new session methods.
- T001 blocks T002 and T003 (they call `readLine()`).
- Phase 3 tests depend on the implementation in Phases 1–2.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `MenuSession.PromptWithDefault` exists and, in interactive mode, reads through the session's shared reader (no fresh `bufio.Reader` on `os.Stdin`); prompt `"%s [%s]: "` still printed to stdout.
- [x] A-002 R2: `MenuSession.ConfirmYesNo` exists and, in interactive mode, reads through the shared reader; prompt `"%s [Y/n] "` still printed to stderr; answer parsed via `HasPrefix(ToLower(line),"y")`.
- [x] A-003 R3: a `readLine()` helper on `blockingByteReader` accumulates via `readByteBlocking()` to `'\n'`, strips trailing `"\r\n"`/`"\n"`, and reports failure on pre-newline EOF.
- [x] A-004 R7: `wt create` `RunE` constructs one `MenuSession` and routes all five interactive stdin consumers (2 menus + 3 line prompts) through it; both one-shot `wt.ShowMenu` calls are gone.
- [x] A-005 R9: the dirty-state warning reads exactly `"current worktree has uncommitted changes"` via `wt.Warn` (stderr).

### Behavioral Correctness

- [x] A-006 R4: session-aware prompt EOF/empty semantics match today — `PromptWithDefault`→`defaultValue` on EOF/empty; `ConfirmYesNo`→`false` on EOF, `true` on empty.
- [x] A-007 R7: the menu→line-prompt seam no longer steals the user's typed name — a `session.Show` followed by a session-aware line prompt on one shared stream delivers the line intact (regression test passes).
- [x] A-008 R6: package-level `PromptWithDefault`/`ConfirmYesNo` bodies are unchanged (fresh `bufio.Reader`, synchronous read); only doc comments gained the session-variant guidance.

### Scenario Coverage

- [x] A-009 R3: `readLine()` unit tests cover multi-byte line, empty line, CRLF stripping, and EOF-before-newline.
- [x] A-010 R1 R2: session-aware `PromptWithDefault`/`ConfirmYesNo` unit tests cover default-on-empty, y/n parsing, and EOF paths without a PTY.
- [x] A-011 R9: an integration test asserts the corrected warning string on stderr from a dirty repo without leaking a worktree (aborts).

### Edge Cases & Error Handling

- [x] A-012 R5: fallback-mode session methods delegate to the package-level functions; `runFallbackMenu` and all existing pinned byte-for-byte fallback/prompt tests pass unmodified.
- [x] A-013 R3: `readLine()` discards partial pre-EOF input (returns not-ok), matching the `err != nil` short-circuit in the current prompt functions.

### Code Quality

- [x] A-014 Pattern consistency: new `MenuSession` methods mirror the existing `Show` method shape and the `showInteractive`/`readByteBlocking` conventions in `menu.go`; create.go session threading matches the `wt open`/`wt delete`/`wt go` pattern.
- [x] A-015 No unnecessary duplication: session-aware prompts reuse `readByteBlocking()` (via `readLine()`) and delegate to the package-level functions in fallback mode rather than duplicating the fresh-`bufio` line-read; `cmd/` adds no line-reading logic (Constitution Principle V).
- [x] A-016 Host isolation: new tests satisfy `code-review.md` § Project-Specific Review Rules — pure-seam tests (no PTY) for the primitive; the create integration test aborts before creating a worktree and relies on `runWt` env isolation, leaking no side effects.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

- `ShowMenu` (src/internal/worktree/menu.go:56-60) — after create.go switched both call sites to `session.Show`, the exported one-shot wrapper has zero production call sites (only `menu_test.go` fallback tests drive it); kept deliberately as the documented single-menu convenience API pinned by `docs/memory/wt-cli/menu-navigation-contract.md`, so not recommended for removal — flagged because the change made it production-unused.
- Package-level `PromptWithDefault`/`ConfirmYesNo` (src/internal/worktree/menu.go:272, :294) — their only remaining callers are the session methods' fallback delegation (menu.go:168, :189); retention is mandated by plan R6 (standalone cooked-mode use is safe and one-shot-session wrappers would reintroduce theft), so these are not deletable — noted because this change removed their last direct `cmd/` call sites.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Line-read helper is `readLine() (string, bool)` on `blockingByteReader`, accumulating via `readByteBlocking()` until `'\n'` then stripping `"\r\n"`/`"\n"` | Intake §1 names this exact mechanism ("a line-read helper (e.g. `readLine()` on `blockingByteReader`)"); keeps the single-reader invariant | S:85 R:80 A:85 D:80 |
| 2 | Confident | `readLine` returns `(line, ok bool)` where `ok==false` on pre-newline read failure/EOF (partial discarded) | Intake §1/§4 pins EOF-before-newline as an error path; a bool ok mirrors `readByteBlocking`'s `(byte, bool)` signature already in the file | S:70 R:80 A:80 D:70 |
| 3 | Confident | Session-aware prompts `strings.TrimSpace` the line after `readLine`'s CRLF strip, so interactive and fallback modes behave identically on padded input (`"  "` → default, `" y"` → yes) | Review-corrected (should-fix): both modes must be behaviorally identical; the package-level funcs trim, so the interactive path trims too | S:70 R:85 A:85 D:75 |
| 4 | Certain | Session created before the dirty-state check and `defer session.Close()` immediately after, matching `wt open`/`wt delete` placement | Intake §2 says "near the top of `RunE`"; the dirty-state menu is the first interactive consumer so the session must precede it | S:85 R:85 A:85 D:85 |
| 5 | Confident | Warning-copy integration test drives a dirty repo with piped stdin (non-TTY → fallback menu) and feeds `3\n` (Abort) so no worktree is created | Only viable host-isolated route to reach the interactive dirty-state block (all existing create tests use `--non-interactive`, which skips it); Abort returns before any filesystem mutation | S:65 R:85 A:80 D:65 |
| 6 | Confident | Package-level `PromptWithDefault` prompt stays on stdout (not migrated to stderr) | Intake assumption #8: byte-compat mandate outweighs the stdout/stderr-convention cleanup for a fix-type change; scope discipline | S:60 R:85 A:80 D:70 |

6 assumptions (2 certain, 4 confident, 0 tentative).
