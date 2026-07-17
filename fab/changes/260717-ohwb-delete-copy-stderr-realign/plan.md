# Plan: Realign wt delete Human Copy to Stderr

**Change**: 260717-ohwb-delete-copy-stderr-realign
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. This change is a mechanical stream realignment in
     src/cmd/wt/delete.go: all non-error human copy moves stdout → stderr,
     byte-for-byte preserving wording/colors/framing. -->

### CLI Output: `wt delete` Stream Discipline

#### R1: Non-error human copy on stderr
All non-error human copy emitted by `wt delete` (progress lines, confirmations, summaries, hints, and the SIGINT framing newline) SHALL be written to stderr, not stdout. This is required by toolkit principle №2 (*stdout is data, stderr is diagnostics* — a MUST). Concretely, every `fmt.Print` / `fmt.Printf` / `fmt.Println` call in `src/cmd/wt/delete.go` SHALL become the corresponding `fmt.Fprint` / `fmt.Fprintf` / `fmt.Fprintln` with `os.Stderr` as the first argument.

- **GIVEN** a user runs any `wt delete` invocation that succeeds or is cancelled
- **WHEN** the command prints progress/summary/confirmation/hint copy
- **THEN** that copy is written to stderr
- **AND** none of it is written to stdout

#### R2: Byte-for-byte copy preservation
The realignment SHALL change only the output stream. Wording, argument order, color escapes (`wt.ColorBold`/`wt.ColorGreen`/`wt.ColorYellow`/`wt.ColorReset`), and blank-line framing SHALL be preserved byte-for-byte. No copy rewording, no new output helper, no flag or exit-code change.

- **GIVEN** the message text a call site printed before the change
- **WHEN** the same call site is rewritten to target stderr
- **THEN** the format string, arguments, and color escapes are identical
- **AND** the only textual delta on the line is `fmt.Printf(...)` → `fmt.Fprintf(os.Stderr, ...)` (and the `Println`/`Print` analogues)

#### R3: Empty stdout on every path
After the realignment, `wt delete` SHALL produce empty stdout on every non-menu code path — reserving stdout for a future machine-readable contract.

- **GIVEN** a non-interactive `wt delete` invocation that completes (e.g. by-name delete)
- **WHEN** its stdout is captured
- **THEN** stdout is empty (`""`)

#### R4: Menu rendering stays on stdout (out of scope)
The shared interactive menu rendering (`wt.MenuSession.Show` → `showInteractive(os.Stdout, ...)` in `src/internal/worktree/menu.go`) SHALL NOT be modified by this change. Menu content remains on stdout.

- **GIVEN** an interactive `wt delete` that renders the selection menu
- **WHEN** the menu options are drawn
- **THEN** they continue to appear on stdout (via the shared menu session), unaffected by this change

#### R5: Tests conform to the stderr contract
The `delete_test.go` suite SHALL conform to the new stream contract (Constitution IV — tests conform to spec): the two direct-stdout copy assertions on the `No idle worktrees` empty-state message flip to stderr; combined-stream (`r.Stdout + r.Stderr`) assertions stay unchanged; menu-content assertions on `r.Stdout` stay unchanged (menu is out of scope, R4); and one explicit stdout-emptiness guard is added on a representative non-interactive delete path (R3).

- **GIVEN** the realigned `delete.go`
- **WHEN** `go test ./cmd/wt/` runs
- **THEN** all delete tests pass
- **AND** at least one test asserts `r.Stdout == ""` on a completed non-interactive delete

### Non-Goals

- Shared menu rendering in `src/internal/worktree/menu.go` — cross-command infrastructure with its own contract; belongs in a separate backlog item (R4).
- Copy rewording, new output helpers, flag or exit-code changes — stream flip only (R2).
- The already-conformant call sites: the two pre-menu warnings via `wt.Warn` + `fmt.Fprintln(os.Stderr)` framing (delete.go 698–700, 737–739), the two `wt.Warn("failed to remove ...")` calls (425, 505), and all `wt.ExitWithError`/`wt.PrintError` paths (stderr by construction) — left untouched.

### Design Decisions

1. **Mechanism = direct `fmt.Fprintf/Fprintln(os.Stderr, ...)` at each call site**: matches the established idiom in `create.go` (e.g. `fmt.Fprintf(os.Stderr, "Created worktree: ...")`) and the two log5 pre-menu realignments already in `delete.go`. — *Why*: smallest change that closes the principle gap; no indirection to review. — *Rejected*: introducing a shared `stderrPrintf` helper (adds a new abstraction the codebase does not use for this; out of scope per R2).
2. **SIGINT framing newline (delete.go:69) is included**: the bare `fmt.Println()` before rollback is human framing; leaving it on stdout would keep stdout non-empty on the interrupt path, violating R3. — *Why*: completeness of the empty-stdout contract. — *Rejected*: leaving it (breaks R3 on the SIGINT path).
3. **Empty-stdout guard added to `TestDelete_ByName`**: the representative non-interactive by-name delete path. — *Why*: pins the R3 contract the way `create_test.go` pins its one-line-stdout guard, on a stable existing test. — *Rejected*: a brand-new standalone test (unnecessary; the existing by-name test already exercises the fullest success path).

## Tasks

<!-- The realignment is one mechanical edit spanning delete.go's handlers.
     Split by logical region to keep each task reviewable; all implement R1/R2/R3. -->

### Phase 1: Realign delete.go call sites (stdout → stderr)

- [x] T001 Flip the SIGINT-handler framing newline `fmt.Println()` (delete.go:69) to `fmt.Fprintln(os.Stderr)` <!-- R1 -->
- [x] T002 Flip all `handleDeleteCurrent` copy — `Worktree:`/`Branch:`/`Path:` block (209–211), `Cancelled.` (234), `Removing worktree...` (246), `Deleted worktree:` (250), and the post-delete hint block (254–256) — to `fmt.Fprintf/Fprintln(os.Stderr, ...)`, preserving text/colors/framing byte-for-byte <!-- R1 --> <!-- R2 -->
- [x] T003 Flip all `handleDeleteByName` copy — `Worktree:`/`Branch:`/`Path:` (288–290), `Cancelled.` (304), `Removing worktree...` (309), `Deleted worktree:` (313) — to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T004 Flip all `handleDeleteMultiple` copy — summary block `Worktrees to delete (N):` + per-item lines + trailing blank (390–394), `Cancelled.` (406), per-worktree `--- Deleting: X ---` + `Worktree:`/`Branch:`/`Path:` blocks (413–416), `Removing worktree...` (423), `Deleted worktree:` (428) — to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T005 Flip all `handleDeleteAll` copy — `No worktrees found.` (472), `Found N worktree(s):` + per-item lines + trailing blank (476–480), `Cancelled.` (492), per-worktree deletion blocks (498–508) — to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T006 Flip `handleDeleteStale` empty-state `No idle worktrees (threshold: %s).` (555) to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T007 Flip `handleDeleteMenu` non-menu copy — `No worktrees found.` (598), `Cancelled.` (657) — to stderr, byte-preserving; leave the `session.Show(...)` menu rendering untouched (R4) <!-- R1 --> <!-- R2 --> <!-- R4 -->
- [x] T008 Flip `handleUncommittedChanges` copy — `Stashing changes...` (680), `Changes stashed (hash: ...)` (686), `Discarding uncommitted changes...` (692), stash/`Discarding changes...`/`Cancelled.` branch lines (712/718/721/723) — to stderr, byte-preserving; leave the existing `fmt.Fprintln(os.Stderr)` + `wt.Warn` pre-menu framing (698–700) untouched <!-- R1 --> <!-- R2 -->
- [x] T009 Flip `handleUnpushedCommits` copy — `Commits that will be lost:` (741), commit lines (744), `... and N more` (747), trailing blank (749), `Cancelled.` (758) — to stderr, byte-preserving; leave the existing `fmt.Fprintln(os.Stderr)` + `wt.Warn` pre-menu framing (737–739) untouched <!-- R1 --> <!-- R2 -->
- [x] T010 Flip `handleBranchCleanup` copy — `Skipped branch deletion: ...` (783), `Deleted branch: %s (local)` (789, 805), `Deleted branch: %s (remote)` (794, 809), `Note: Could not delete remote branch` (796) — to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T011 Flip `handleStashInDir` copy — `Stashing changes...` (840), `Changes stashed (hash: ...)` (873) — to stderr, byte-preserving <!-- R1 --> <!-- R2 -->
- [x] T012 Verify no `fmt.Print`/`fmt.Printf`/`fmt.Println` (stdout) call sites remain in delete.go (`grep -nE 'fmt\.(Print|Printf|Println)\(' cmd/wt/delete.go` returns nothing); run `gofmt -l cmd/wt/delete.go` (must be clean) <!-- R1 --> <!-- R3 -->

### Phase 2: Tests conform to the stderr contract

- [x] T013 In `src/cmd/wt/delete_test.go`, flip the two direct-stdout empty-state assertions to stderr: `assertContains(t, r.Stdout, "No idle worktrees (threshold: 7d).")` (line 567) and `assertContains(t, r.Stdout, "No idle worktrees")` (line 622) → assert against `r.Stderr` <!-- R5 -->
- [x] T014 In `src/cmd/wt/delete_test.go`, add an explicit stdout-emptiness guard to `TestDelete_ByName` (the representative `--non-interactive --worktree-name` success path): assert `r.Stdout == ""` after the delete, pinning R3 the way `create_test.go` pins its one-line-stdout guard <!-- R5 --> <!-- R3 -->
- [x] T015 Run `go test ./cmd/wt/` (delete + integration) and confirm green; combined-stream and menu-content assertions remain unchanged <!-- R5 -->

## Execution Order

- T001–T011 are independent stdout→stderr edits within the same file; execute sequentially to keep one clean diff (do not parallelize edits to the same file).
- T012 runs after T001–T011 (verifies no stdout call sites remain).
- T013–T014 (test edits) depend on the source realignment being complete.
- T015 (full test run) is last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: Every non-error human-copy call site in `delete.go` writes to stderr; `grep -nE 'fmt\.(Print|Printf|Println)\(' cmd/wt/delete.go` returns no matches.
- [x] A-002 R4: `src/internal/worktree/menu.go` is unmodified by this change; menu rendering still targets stdout.
- [x] A-003 R5: `delete_test.go` conforms to the new contract — the two `No idle worktrees` assertions target `r.Stderr`, and a stdout-emptiness guard exists.

### Behavioral Correctness

- [x] A-004 R2: For each realigned line, only the stream changed — format string, arguments, and color escapes are byte-identical to the pre-change text (diff shows only `fmt.Print*(` → `fmt.Fprint*(os.Stderr, `).
- [x] A-005 R3: A completed non-interactive `wt delete` produces empty stdout — `TestDelete_ByName`'s new `r.Stdout == ""` guard passes.

### Scenario Coverage

- [x] A-006 R3: The empty-stdout contract is exercised by an automated test on a representative non-interactive delete path.
- [x] A-007 R5: `go test ./cmd/wt/` (unit + integration) is green; combined-stream assertions (`r.Stdout + r.Stderr`) and menu-content stdout assertions still pass unchanged.

### Edge Cases & Error Handling

- [x] A-008 R1: The SIGINT-handler framing newline (delete.go:69) is on stderr, so stdout stays empty on the interrupt path.
- [x] A-009 R2: The already-conformant sites (pre-menu `wt.Warn` framing at 698–700 / 737–739, `wt.Warn` remove-failure at 425/505, and all `ExitWithError`/`PrintError` paths) are untouched — no double-realignment, no duplicate stderr newlines.

### Code Quality

- [x] A-010 Pattern consistency: The stderr edits follow the established `create.go` / log5 idiom (`fmt.Fprintf(os.Stderr, ...)`), matching surrounding code.
- [x] A-011 No unnecessary duplication: No new output helper is introduced; existing `fmt.Fprintf/Fprintln(os.Stderr, ...)` idiom is reused (code-quality "duplicating existing utilities" / "composition over inheritance").
- [x] A-012 Readability: No god functions introduced (mechanical in-place edits, no new functions); no magic strings/numbers added (copy is preserved verbatim, not re-templated).
- [x] A-013 gofmt: `gofmt -l cmd/wt/delete.go cmd/wt/delete_test.go` reports no files (CI enforces `gofmt -l`).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change is a mechanical stdout→stderr stream realignment; it adds no new functionality and makes no existing code, function, branch, or config redundant or unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | All non-error human copy in `delete.go` moves to stderr; stdout becomes empty on every non-menu path | Backlog + intake enumerate the strings explicitly; principle №2 is a MUST; spec pins no stdout contract for delete | S:90 R:85 A:95 D:90 |
| 2 | Certain | Mechanism is direct `fmt.Fprintf/Fprintln(os.Stderr, ...)` at each call site — no new output helper | Matches `create.go`'s established idiom and the log5 pre-menu realignment in the same file | S:70 R:90 A:85 D:75 |
| 3 | Certain | Wording, colors, and blank-line framing preserved byte-for-byte — only the stream changes | Mirrors log5's recorded rule ("standardizes only the prefix, stream, and color, never the message text") | S:75 R:85 A:90 D:85 |
| 4 | Confident | Shared menu rendering (`menu.go` `showInteractive` → stdout) stays out of scope | Backlog/intake enumerate delete.go copy only; menu is cross-command infrastructure with its own contract; flipping it would churn create/open/go tests | S:65 R:75 A:70 D:65 |
| 5 | Confident | The SIGINT-handler bare `fmt.Println()` (line 69) is included in the realignment despite not being enumerated in the backlog | It is human framing output; leaving it would keep stdout non-empty on the interrupt path (violates R3) | S:55 R:90 A:80 D:75 |
| 6 | Certain | Tests: flip the two `r.Stdout` "No idle worktrees" assertions to `r.Stderr`; combined-stream and menu-content assertions untouched; add one stdout-empty guard to `TestDelete_ByName` | Test inventory verified by inspection; guard mirrors `create_test.go`'s one-line-stdout pin | S:80 R:90 A:95 D:85 |

6 assumptions (4 certain, 2 confident, 0 tentative).
