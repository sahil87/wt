# Intake: Fix stdin byte-theft at the menu→line-prompt seam; correct mislabeled dirty-state warning in `wt create`

**Change**: 260708-wryx-menu-prompt-byte-theft
**Created**: 2026-07-08

## Origin

Dispatched promptless by `/fab-proceed` from a live debugging conversation (synthesized description treated as source of truth). The user hit an interactive hang in `wt create`: after selecting "Continue anyway" on the dirty-state menu, the `Worktree name [lively-tamarin]:` prompt appeared hung — the first Enter/typed line was swallowed; a second Enter typically got through but any typed name was lost (observed via user screenshot). The conversation diagnosed the root cause (an orphaned stdin read-ahead pump goroutine stealing the next line), agreed on the fix shape (extend the existing `MenuSession` shared-reader mechanism to line prompts), and additionally identified a mislabeled warning: `wt create` warns "main repo has uncommitted changes" when the dirty state actually belongs to whatever worktree the process is standing in.

> Fix stdin byte-theft at the menu→line-prompt seam (orphaned read-ahead pump steals the user's next typed line in `wt create`); extend the shared-reader `MenuSession` mechanism to session-aware `PromptWithDefault`/`ConfirmYesNo` variants and thread one session through `wt create`'s interactive flow; correct the mislabeled dirty-state warning copy in `wt create`.

## Why

**Problem 1 (primary — interactive hang).** Interactive menus run a read-ahead pump goroutine over stdin (`blockingByteReader` in `src/internal/worktree/menu.go:645-728`): the pump loops `src.ReadByte()` → channel send, so after a menu submits, the pump goroutine is left parked in a blocking read on the stdin fd. In `wt create` (`src/cmd/wt/create.go`), the dirty-state menu (`wt.ShowMenu`, create.go:126-153 — a one-shot `MenuSession`) is immediately followed by `wt.PromptWithDefault("Worktree name", …)` (create.go:176), which constructs a fresh `bufio.NewReader(os.Stdin)` (menu.go:228-240). Two readers now race on the same fd. In cooked mode the kernel delivers the whole typed line to ONE reader; the orphaned pump (queued first) wins, and its `bufio.Reader` slurps the line into its 4KB buffer — the line is lost to the real prompt. The user experiences a hung prompt that swallows their input.

The codebase already knows this failure mode: `MenuSession` (menu.go:62-148) was built to fix byte-theft between *consecutive menus* (`wt open`, `wt delete` flows — regression tests `TestUnderlyingReadAhead_DemonstratesTheft`, `TestMenuSession_SharesReaderAcrossMenus`, `TestMenuSession_ThreeMenusNoTheft` in `src/internal/worktree/menu_session_test.go`; contract documented in `docs/memory/wt-cli/menu-navigation-contract.md`). But the **menu → line-prompt** seam is uncovered: `PromptWithDefault` (menu.go:228-240) and `ConfirmYesNo` (menu.go:213-225) each build their own fresh `bufio.Reader` and are not session-aware.

If unfixed, every `wt create` invocation from a dirty worktree produces a broken name prompt — the flagship interactive flow of the tool loses user keystrokes nondeterministically.

**Why this approach**: extending `MenuSession` (rather than, e.g., killing the pump after each menu, or a global singleton reader) reuses the mechanism the codebase already built, tested, and documented for exactly this class of bug. The pump cannot be cleanly cancelled (a blocking `ReadByte` on a TTY fd is not interruptible without closing stdin), so the single-shared-reader design is the only correct shape — the fix is to finish extending it to all stdin consumers within one interactive flow.

**Problem 2 (secondary — mislabeled warning).** create.go:127 warns `"main repo has uncommitted changes"`, but the checks `HasUncommittedChanges()`/`HasUntrackedFiles()` (`src/internal/worktree/git.go:11-28`) run `git diff --quiet` / `git diff --cached --quiet` / `git ls-files --others --exclude-standard` in the process CWD — which is whatever worktree the user is standing in, not necessarily the main repo. The user hit this warning from a dirty *linked worktree* while the main worktree was clean. The copy misleads users into inspecting the wrong checkout.

## What Changes

### 1. Session-aware line prompts in `src/internal/worktree/menu.go`

Add two methods on `MenuSession`, mirroring the existing `Show` method pattern:

```go
// PromptWithDefault prompts for a line of input with a default value,
// reading through the session's shared stdin reader.
func (s *MenuSession) PromptWithDefault(prompt, defaultValue string) string

// ConfirmYesNo prompts for a Y/n confirmation (default yes), reading
// through the session's shared stdin reader.
func (s *MenuSession) ConfirmYesNo(prompt string) bool
```

**Interactive mode** (`s.interactive == true`): read a full line through the shared `blockingByteReader` — the pump delivers one byte at a time, so a line-read helper (e.g. `readLine()` on `blockingByteReader` or a session-internal helper) accumulates bytes via `readByteBlocking()` until `'\n'`, then strips the trailing `"\r\n"`/`"\n"`. No raw mode is entered — line prompts run in cooked mode exactly as today (the kernel line-buffers and echoes; raw mode remains entered/restored per `Show()` call only, per the deliberate `MenuSession` doc-comment contract). Because the parked pump and the prompt now consume the *same* reader, the pump's pending byte is delivered to the prompt instead of being stolen.

**EOF/error semantics match current behavior exactly**: on read failure/EOF before a newline, `PromptWithDefault` returns `defaultValue` and `ConfirmYesNo` returns `false` (partial input before EOF is discarded, matching the current `err != nil` short-circuit in both functions). Empty line → default (`defaultValue` / `true`). `ConfirmYesNo` answer parsing unchanged: `strings.HasPrefix(strings.ToLower(line), "y")`.

**Fallback mode** (`s.interactive == false`, i.e. non-TTY / Windows / raw-mode-entry failure): delegate to the existing package-level implementations so behavior under piped stdin is byte-for-byte identical to today (fresh `bufio.NewReader(os.Stdin)`, same prompt strings, same streams).

**Prompt rendering and output streams are unchanged**: `PromptWithDefault` keeps printing `"%s [%s]: "` to stdout; `ConfirmYesNo` keeps printing `"%s [Y/n] "` to stderr. Only the *reader* seam changes.

The package-level `ConfirmYesNo`/`PromptWithDefault` functions remain with their current fresh-`bufio.Reader` cooked-mode implementations for standalone use (a plain cooked-mode `ReadString('\n')` completes synchronously and leaves no parked goroutine, so standalone use is safe). Their doc comments gain the constraint: flows that mix menus and line prompts on the same stdin MUST use the session-aware variants. They must NOT be reimplemented as one-shot-session wrappers — that would create a pump whose orphan reintroduces the theft in the other direction (prompt → next reader).

### 2. Thread one `MenuSession` through `wt create`'s interactive flow (`src/cmd/wt/create.go`)

Create a single session near the top of `RunE` (`session := wt.NewMenuSession(); defer session.Close()`) and route every interactive stdin consumer in the flow through it:

| create.go line (today) | Today | After |
|---|---|---|
| :128 dirty-state menu `"How to proceed?"` | one-shot `wt.ShowMenu` | `session.Show` |
| :176 `"Worktree name"` prompt | package-level `wt.PromptWithDefault` | `session.PromptWithDefault` |
| :257 `"Initialize worktree?"` confirm | package-level `wt.ConfirmYesNo` | `session.ConfirmYesNo` |
| :389 `"Continue and open the worktree anyway?"` confirm | package-level `wt.ConfirmYesNo` | `session.ConfirmYesNo` |
| :423 `"Open in:"` app menu | one-shot `wt.ShowMenu` | `session.Show` |

The one-shot `ShowMenu` at :128 currently orphans a pump that steals from *every* later stdin consumer in the flow — including the second one-shot menu at :423 (menu→menu theft within create, the exact bug class `MenuSession` fixed for `wt open`). Threading one session fixes all seams at once and makes `wt create` consistent with the `wt open`/`wt delete`/`wt go` session pattern.

**Call-site audit result (performed at intake)**: `grep` over `src/cmd/wt/` and `src/internal/worktree/` shows the ONLY production call sites of `PromptWithDefault`/`ConfirmYesNo` are create.go:176/:257/:389, and the only one-shot `ShowMenu` production call sites are create.go:128/:423. `wt delete` (delete.go:97), `wt open` (open.go:202/:292/:417), and `wt go` (go.go:87) already thread a shared `MenuSession` and contain no line prompts — no changes needed there.

### 3. Correct the dirty-state warning copy (create.go:127)

```go
// before
wt.Warn("main repo has uncommitted changes")
// after
wt.Warn("current worktree has uncommitted changes")
```

The checks run in the process CWD (any worktree, linked or main), so the copy must describe the *current worktree/checkout*. `wt.Warn` (`src/internal/worktree/errors.go:119`) already targets stderr, satisfying the project stdout/stderr convention (stdout = machine result, stderr = human copy). No behavior change to the checks themselves — `HasUncommittedChanges()`/`HasUntrackedFiles()` are correct as CWD checks; the *warning exists* because uncommitted work doesn't carry over to the new worktree, and the "Stash changes first" option remains valid since the git stash is repo-global.

### 4. Tests

- **Primitive characterization** (extend `src/internal/worktree/menu_session_test.go` pattern): a test demonstrating menu→line-prompt theft with two independent readers (analogous to `TestUnderlyingReadAhead_DemonstratesTheft`), and a regression test asserting a `session.Show` followed by a session-aware line prompt on the same `sharedStream` delivers the typed line to the prompt intact (analogous to `TestMenuSession_SharesReaderAcrossMenus`). The line-read helper gets direct unit coverage: multi-byte line, empty line, `"\r\n"` stripping, EOF-before-newline → error path.
- **Prompt semantics**: session-aware `PromptWithDefault`/`ConfirmYesNo` unit tests for default-on-empty, y/n parsing, EOF → default/false — testable via the existing seam discipline (injected reader/writer; no PTY needed, per Constitution Principle IV and the `runInteractiveMenuCore` seam pattern documented in `menu-navigation-contract.md`).
- **Non-TTY contracts preserved**: existing pinned byte-for-byte tests for `runFallbackMenu` and prompts under piped stdin must keep passing unmodified; `cmd/` integration tests (piped stdin → fallback path) keep passing.
- **Copy fix**: update/add the `wt create` test asserting the new warning string on stderr.
- All tests conform to the code-review.md host-isolation rule (no side-effect leakage; use non-side-effecting `--worktree-open`/`--app` targets or `runWt` env isolation).

## Affected Memory

- `wt-cli/menu-navigation-contract`: (modify) Extend the single-reader `MenuSession` contract to cover the menu→line-prompt seam — session-aware `PromptWithDefault`/`ConfirmYesNo`, the line-read-through-pump mechanism, the cooked-mode/no-raw-mode rule for line prompts, and `wt create` joining the session-threading pattern.

## Impact

- `src/internal/worktree/menu.go` — two new `MenuSession` methods + a line-read helper on `blockingByteReader`; doc-comment updates on the package-level prompts and `MenuSession`.
- `src/cmd/wt/create.go` — one session threaded through `RunE`; five call-site edits; one warning-string edit. `cmd/` stays orchestration-only (Constitution Principle V: the new logic lives in `internal/worktree`).
- `src/internal/worktree/menu_session_test.go`, `menu_test.go`, `src/cmd/wt/create_test.go` — new regression/unit coverage; existing pinned fallback-output tests unchanged.
- No public CLI surface change (no flags, no exit codes, no output-contract change other than the corrected warning string on stderr). No new dependencies.
- Non-goals: `wt create` base-ref semantics unchanged (new worktree still based on current HEAD when `--base` absent — crud.go:35-40; that behavior is the *reason* the dirty warning exists, not part of this fix); raw-mode-per-`Show()` lifecycle unchanged; cooked-mode type-ahead buffering between consecutive line prompts (no parked reader involved) out of scope; `wt delete`/`wt open`/`wt go` untouched.

## Open Questions

None — the fix shape, constraints, and scope were resolved in the originating conversation; remaining implementation choices are graded below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Fix mechanism: extend `MenuSession` with session-aware line prompts sharing the single stdin reader; the pump is not cancelled/killed | Agreed fix shape in the originating conversation; reuses the codebase's existing, tested, documented mechanism for exactly this bug class | S:90 R:70 A:90 D:85 |
| 2 | Certain | `wt create` threads ONE session across the whole interactive flow (both menus + all three line prompts), replacing both one-shot `ShowMenu` calls | Discussed; intake audit shows the :128 orphan pump also steals from the :423 menu; matches the established `wt open`/`wt delete`/`wt go` pattern | S:80 R:80 A:85 D:80 |
| 3 | Confident | API shape: methods on `MenuSession` (`session.PromptWithDefault`, `session.ConfirmYesNo`) rather than free functions taking a session parameter | Mirrors the existing `session.Show` method; conventional Go shape for shared-resource access | S:60 R:85 A:85 D:75 |
| 4 | Confident | Package-level `ConfirmYesNo`/`PromptWithDefault` remain with current fresh-`bufio` cooked-mode implementations (NOT one-shot-session wrappers) for standalone use; doc comments direct mixed flows to session variants | A cooked-mode `ReadString` leaves no parked goroutine so standalone use is safe; a one-shot-session wrapper would orphan a pump and reintroduce theft in the prompt→next-reader direction | S:55 R:80 A:75 D:65 |
| 5 | Certain | EOF/error semantics of session-aware prompts match current behavior: `PromptWithDefault` → `defaultValue`, `ConfirmYesNo` → `false`; partial pre-EOF input discarded; empty line → default | Explicit constraint in the synthesized description; preserves every caller's observable behavior | S:85 R:85 A:85 D:85 |
| 6 | Certain | Non-TTY fallback paths preserved byte-for-byte: fallback-mode session prompts delegate to the existing package-level line-read code; `runFallbackMenu` untouched | Explicit constraint; Constitution Principle VI; pinned by existing byte-for-byte tests | S:85 R:80 A:90 D:90 |
| 7 | Confident | Warning copy becomes exactly `"current worktree has uncommitted changes"` (via `wt.Warn`, stderr) | Requirement was "accurately describe what is dirty (the current worktree/checkout)"; exact wording is agent-choosable, trivially reversible; "current worktree" is accurate from both main and linked worktrees | S:65 R:90 A:80 D:60 |
| 8 | Confident | Prompt rendering/streams unchanged: `PromptWithDefault` prompt text stays on stdout, `ConfirmYesNo` on stderr — only the reader seam changes; the stdout/stderr-convention cleanup of `PromptWithDefault`'s prompt stream is NOT taken in this change | Byte-compat mandate ("preserve pinned output contracts") outweighs the convention cleanup; scope discipline for a fix-type change | S:55 R:85 A:70 D:55 |
| 9 | Confident | Test approach: pure-seam unit tests without a PTY (extend `menu_session_test.go` `sharedStream` pattern + injected reader/writer for prompt semantics); host-isolation per code-review.md | Constitution Principle IV + the documented `runInteractiveMenuCore` seam pattern make this the established discipline; exact test names/fixtures are implementation detail | S:70 R:85 A:85 D:70 |
| 10 | Certain | Scope exclusions: base-ref behavior, raw-mode-per-`Show` lifecycle, cooked-mode type-ahead between consecutive line prompts, and `wt delete`/`wt open`/`wt go` are all untouched | Explicitly scoped out in the synthesized description ("Context, not in scope"; "the fix must not change that"); audit confirms the other flows have no menu→line-prompt seam | S:85 R:80 A:85 D:85 |

10 assumptions (5 certain, 5 confident, 0 tentative, 0 unresolved).
