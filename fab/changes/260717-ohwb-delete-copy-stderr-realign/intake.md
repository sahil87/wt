# Intake: Realign wt delete Human Copy to Stderr

**Change**: 260717-ohwb-delete-copy-stderr-realign
**Created**: 2026-07-18

## Origin

Backlog item `[ohwb]` (2026-07-18), invoked via `/fab-new ohwb` (one-shot, no prior conversation):

> Realign `wt delete`'s non-warning human copy from stdout to stderr (toolkit principle No.2, stdout=machine-result / stderr=human). The delete handlers print Worktree:/Branch:/Path:, 'Removing worktree...', 'Deleted worktree:', 'Cancelled.', 'Deleted branch: ... (local/remote)', 'Skipped branch deletion: ...', the multi-delete summary block, and 'No worktrees found.' via fmt.Printf/fmt.Println (stdout). Only the two pre-menu warnings were realigned in 260622-log5; the rest still leak to stdout. wt delete has no stdout machine contract today so nothing breaks meanwhile. Deferred from 6end as a command-wide (~20 call-site) output realignment (cross-command output redesign bucket), not a single misrouted line.

## Why

1. **Problem**: `wt delete` violates toolkit principle №2 (*stdout is data, stderr is diagnostics* — a MUST): all of its progress/summary/hint copy prints to stdout via `fmt.Printf`/`fmt.Println`. The toolkit-standards conformance audit (`260709-6end`, recorded in `docs/memory/wt-cli/toolkit-standards-conformance.md`) graded №2 as PASS-with-gap and explicitly deferred this realignment to backlog `[ohwb]`.
2. **Consequence if unfixed**: `wt delete` can never grow a machine-readable stdout contract (e.g. a future `--json` deleted-worktrees result) without breaking stream discipline, and agents/scripts capturing stdout get ~50 lines of human prose. Sibling commands (`create`, `init`, `go`) already conform — the asymmetry invites regressions when copy is added by pattern-matching against `delete.go`.
3. **Why this approach**: mechanical stream flip at each call site, matching the idiom already established in `create.go` (direct `fmt.Fprintf(os.Stderr, ...)`) and the two `wt delete` pre-menu warnings realigned in `260622-log5`. No copy rewording, no new helpers, no behavior change — the smallest change that closes the principle gap.

## What Changes

### `src/cmd/wt/delete.go` — stdout → stderr at every non-error human-copy call site

Every `fmt.Printf` / `fmt.Println` in `delete.go` becomes `fmt.Fprintf(os.Stderr, ...)` / `fmt.Fprintln(os.Stderr, ...)`. Wording, ordering, colors, and blank-line framing are preserved byte-for-byte — only the stream changes. Inventory by handler (~50 call sites; the backlog's "~20" undercounts):

- **SIGINT handler** (`deleteCmd` RunE, line 69): the bare `fmt.Println()` framing newline before rollback.
- **`handleDeleteCurrent`**: `Worktree:`/`Branch:`/`Path:` block (209–211), `Cancelled.` (234), `Removing worktree...` (246), `Deleted worktree:` (250), and the post-delete hint block `You are no longer in a valid directory.` / `Run: cd <repo-root>` (254–256).
- **`handleDeleteByName`**: `Worktree:`/`Branch:`/`Path:` (288–290), `Cancelled.` (304), `Removing worktree...` (309), `Deleted worktree:` (313).
- **`handleDeleteMultiple`**: the summary block `Worktrees to delete (N):` + per-item lines (390–394), `Cancelled.` (406), per-worktree `--- Deleting: X ---` + `Worktree:`/`Branch:`/`Path:` blocks (413–416), `Removing worktree...` (423), `Deleted worktree:` (428).
- **`handleDeleteAll`**: `No worktrees found.` (472), `Found N worktree(s):` + per-item lines (476–480), `Cancelled.` (492), the per-worktree deletion blocks (498–508).
- **`handleDeleteStale`**: `No idle worktrees (threshold: Nd).` (555).
- **`handleDeleteMenu`**: `No worktrees found.` (598), `Cancelled.` (657).
- **`handleUncommittedChanges`**: `Stashing changes...` / `Changes stashed (hash: ...)` / `Discarding uncommitted changes...` / `Discarding changes...` / `Cancelled.` (680–723).
- **`handleUnpushedCommits`**: `Commits that will be lost:` + commit lines + `... and N more` + framing (741–749).
- **`handleBranchCleanup`**: `Skipped branch deletion: ...` (783), `Deleted branch: %s (local)` (789, 805), `Deleted branch: %s (remote)` (794, 809), `Note: Could not delete remote branch` (796).
- **`handleStashInDir`**: `Stashing changes...` (840), `Changes stashed (hash: ...)` (873).

Already conformant, untouched: the two pre-menu warnings via `wt.Warn` + `fmt.Fprintln(os.Stderr)` framing (698–700, 737–739, realigned by `260622-log5`), the two `wt.Warn("failed to remove ...")` calls (425, 505), and all `wt.ExitWithError`/`wt.PrintError` paths (stderr by construction).

After this change `wt delete`'s stdout is **empty on every path** — reserving it for a future machine contract.

### Out of scope

- **Shared interactive menu rendering** (`wt.MenuSession.Show` → `showInteractive(os.Stdout, ...)` in `src/internal/worktree/menu.go:134`, and its non-TTY fallback): shared infrastructure used by `create`/`open`/`go`/`delete` alike; flipping it is a cross-command change with its own contract (`menu-navigation-contract.md`) and belongs in its own backlog item, not this delete-scoped realignment.
- No copy rewording, no new output helpers, no flag or exit-code changes.

### Tests — `src/cmd/wt/delete_test.go` (and integration guard)

- Most delete tests assert on `combined := r.Stdout + r.Stderr` — they stay green unchanged.
- Two direct stdout assertions flip to stderr: `assertContains(t, r.Stdout, "No idle worktrees (threshold: 7d).")` (line 567) and `assertContains(t, r.Stdout, "No idle worktrees")` (line 622).
- Menu-content assertions on `r.Stdout` (lines 438–520, 720–723) are unaffected — the menu stays on stdout (out of scope above).
- Add an explicit stdout-emptiness guard on at least one representative non-interactive delete path (e.g. `--non-interactive` by-name delete asserting `r.Stdout == ""`), pinning the new contract the way `create_test.go` pins its one-line-stdout guard.

## Affected Memory

- `wt-cli/create-output-phases`: (modify) The canonical stdout/stderr stream-discipline file — extend its `260622-log5` note (two pre-menu warnings realigned) to record the full `wt delete` realignment: all non-error human copy on stderr, stdout empty pending a future machine contract.
- `wt-cli/toolkit-standards-conformance`: (modify) Principle №2 verdict currently reads "PASS with one gap — deferred to `[ohwb]`" (line 34); update to reflect the gap is closed.
- `wt-cli/idle-staleness-contract`: (modify) Annotate that the `No idle worktrees (threshold: Nd).` empty-state message (line 104) is emitted on stderr.

## Impact

- **Code**: `src/cmd/wt/delete.go` only (~50 print call sites across 9 handlers + the SIGINT closure). No `internal/worktree` changes.
- **Tests**: `src/cmd/wt/delete_test.go` (two assertion flips + one new stdout-empty guard); `src/cmd/wt/integration_test.go` delete invocations use `runWtSuccess` without stdout assertions — expected green.
- **Behavior**: user-visible terminal output is unchanged (both streams render to the same TTY); scripts capturing `wt delete` stdout see it go empty — acceptable per the backlog's "no stdout machine contract today".
- **Docs/spec**: `docs/specs/cli-surface.md` pins no output streams for delete (flags/resolution/exit codes only) — no spec change required.
- **Backlog**: `[ohwb]` gets checked off at archive time. Sibling deferrals `[p5m9]` (`--dry-run`) and `[2af2]`/`[6lkr]` are untouched.

## Open Questions

*(none — the backlog item, principle №2, the spec, and the log5 precedent resolve all decision points)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | All non-error human copy in `delete.go` moves to stderr; stdout becomes empty on every path | Backlog enumerates the strings explicitly; principle №2 is a MUST; spec pins no stdout contract for delete | S:90 R:85 A:95 D:90 |
| 2 | Certain | Mechanism is direct `fmt.Fprintf/Fprintln(os.Stderr, ...)` at each call site — no new output helper | Matches `create.go`'s established idiom and the log5 pre-menu realignment in the same file | S:70 R:90 A:85 D:75 |
| 3 | Certain | Wording, colors, and blank-line framing preserved byte-for-byte — only the stream changes | Mirrors log5's recorded rule ("standardizes only the prefix, stream, and color, never the message text") | S:75 R:85 A:90 D:85 |
| 4 | Confident | Shared menu rendering (`menu.go` `showInteractive` → stdout) stays out of scope | Backlog enumerates delete.go copy only; menu is cross-command infrastructure with its own contract; flipping it would churn create/open/go tests | S:65 R:75 A:70 D:65 |
| 5 | Confident | The SIGINT-handler bare `fmt.Println()` (line 69) is included in the realignment despite not being enumerated in the backlog | It is human framing output; leaving it would keep stdout non-empty on the interrupt path | S:55 R:90 A:80 D:75 |
| 6 | Certain | Tests: flip the two `r.Stdout` "No idle worktrees" assertions to `r.Stderr`; combined-stream assertions untouched; add one stdout-empty guard | Test inventory verified by inspection; guard mirrors `create_test.go`'s one-line-stdout pin | S:80 R:90 A:95 D:85 |

6 assumptions (4 certain, 2 confident, 0 tentative, 0 unresolved).
