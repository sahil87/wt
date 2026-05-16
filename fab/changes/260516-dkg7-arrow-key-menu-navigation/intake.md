# Intake: Arrow-Key Menu Navigation

**Change**: 260516-dkg7-arrow-key-menu-navigation
**Created**: 2026-05-17
**Status**: Draft

## Origin

One-shot `/fab-new` invocation. Raw user input:

> Instead of just having the option of entering numbers after most / all wt commands, allow using the arrow keys to navigate between the options.

No prior conversation context. Several decisions (library, replace-vs-additive, scope) were not pre-specified by the user and are surfaced as Open Questions / Unresolved assumptions below.

## Why

Today every interactive prompt routes through `wt.ShowMenu` (`src/internal/worktree/menu.go:15`), which prints a numbered list and reads a line from `os.Stdin`. The user has to:

1. Read the printed options.
2. Identify the number they want.
3. Type the number and press Enter.

This is fine for short lists (Yes/No-style menus with one option) but feels heavy for the longer worktree-selection menus produced by `wt open` and `wt delete`, where the user is choosing among 5–20 worktree names. Arrow-key navigation is the standard pattern across modern CLIs (gh, fzf, gum, brew's interactive prompts) — typing a number from a 15-row list breaks flow because the user's eyes have to scan back to the number column.

What happens if we don't fix it:

- The CLI continues to feel dated relative to its peers.
- Worktree selection (the most-used menu in practice) remains the slowest path through `wt open` / `wt delete`.
- Future menus (`wt update --select-version`, hypothetical app pickers with descriptions) will inherit the same friction.

Why this approach over alternatives:

- **Arrow-key navigation in-place** is the lowest-friction upgrade — no new commands, no flag changes, no shell-integration changes. The user just sees a highlighted row and presses ↑/↓/Enter.
- **Fuzzy-finder integration (fzf, gum)** would require an external dependency, violating Principle I (single self-contained binary).
- **Web UI / TUI dashboard** is far beyond the scope of a worktree helper.

## What Changes

### 1. `ShowMenu` gets a TTY-aware interactive path

`internal/worktree/menu.go:ShowMenu` learns to detect whether `os.Stdin` and `os.Stdout` are both connected to a TTY. When yes, it renders an interactive selector that supports:

| Key | Action |
|-----|--------|
| `↑` / `k` | Move highlight up one row |
| `↓` / `j` | Move highlight down one row |
| `Enter` | Confirm the highlighted row |
| `Esc` / `Ctrl-C` / `q` | Cancel (equivalent to selecting `0) Cancel` today) |
| `0`–`9` | Direct numeric selection still works (typing `3` submits option 3, matching today's behavior) |

When either stream is not a TTY (CI, piped input, `--non-interactive`), behavior is **identical to today** — the numbered list is printed and a line is read from stdin. This preserves:

- Existing test harnesses that feed input via `cmd.Stdin = strings.NewReader("1\n")`.
- Scripts and operators that pipe choices in.
- The `--non-interactive` contract per constitution Principle VI.

### 2. Rendering

Interactive rendering uses ANSI escape sequences for cursor movement and reverse-video highlight on the selected row. The screen region used by the menu is redrawn in place (cursor up by N lines, clear, redraw) so that scrollback isn't polluted with intermediate states. On exit (Enter or Cancel), the menu region is replaced with a single line showing the final choice (e.g. `Open in: cursor`) so that the post-prompt output stays clean.

The existing `(default)` green marker on the default row is preserved. The currently-highlighted row gets a distinct visual treatment (reverse video plus a `›` gutter marker), which is orthogonal to the `(default)` marker.

### 3. Scope

Only `ShowMenu` (the numbered multi-choice menu) is changed. `ConfirmYesNo` and `PromptWithDefault` stay as-is because they have no notion of a navigable option list — they read free-form input. Affected `ShowMenu` call sites (verified via grep):

```
src/cmd/wt/open.go:216    "Open in:" (app menu)
src/cmd/wt/open.go:297    "Select worktree to open:" (worktree picker)
src/cmd/wt/delete.go:154  "Delete this worktree?" (single-option confirm)
src/cmd/wt/delete.go:224  "Delete this worktree?" (single-option confirm)
src/cmd/wt/delete.go:323  (post-error continuation prompt)
src/cmd/wt/delete.go:409  (post-error continuation prompt)
src/cmd/wt/delete.go:498  "Select worktree to delete:" (worktree picker)
src/cmd/wt/delete.go:538  "What would you like to do?" (rollback action menu)
src/cmd/wt/delete.go:583  "Continue anyway?" (warning continuation)
src/cmd/wt/create.go:128  "How to proceed?" (uncommitted-changes branch)
src/cmd/wt/create.go:317  "Open in:" (app menu after create)
```

The biggest UX wins are the two **worktree pickers** (`open.go:297`, `delete.go:498`) — those produce the longest lists.

### 4. Library choice: hand-rolled `golang.org/x/term`

**Decision**: implement with `golang.org/x/term` directly — `term.MakeRaw` + raw-mode escape parsing, roughly 150–250 lines of code. The only new dependency is `golang.org/x/term` itself (an `x/` package maintained by the Go team).

Rejected alternatives:

- **`github.com/AlecAivazis/survey/v2`** — purpose-built `Select` widget that fits exactly, but adds ~5 indirect deps and removes our control over rendering details (cancel handling, default markers, in-place redraw of just the menu region).
- **`github.com/charmbracelet/bubbletea`** — full Charm TUI framework (`lipgloss`, `termenv`, `harmonica`). Overkill for one highlight-row widget; pulls binary size up considerably.
- **`github.com/manifoldco/promptui`** — narrower scope than survey, similar fit, less actively maintained.

Rationale: Constitution Principle I commits to a slim single-binary CLI. Hand-rolled keeps the dep graph minimal and gives full control over fallback behavior, cancel semantics, and in-place rendering.

### 5. Tests

- Unit tests for the non-TTY path remain unchanged — they already drive `ShowMenu` via a piped stdin and assert on captured stdout.
- A new test helper exercises the interactive path by writing raw escape sequences (`\x1b[A`, `\x1b[B`, `\r`) to a `*os.File` pair created via `pty` — gated by a `//go:build linux` tag if needed. Alternative: extract the key→action mapping into a pure function and unit-test that, leaving the raw I/O as a thin shell that's manually QA'd.
- Integration tests under `cmd/integration_test.go` need no changes — they invoke the binary with pipes, which lands on the non-TTY path.

## Affected Memory

- `wt-cli/menu-navigation-contract`: (new) Behavior contract for interactive arrow-key navigation: TTY detection, key bindings, fallback to numbered input, default-row preservation, terminal-restoration on signal interruption.

The existing `wt-cli/list-status-contract.md` and `wt-cli/init-failure-contract.md` are unrelated and unaffected.

## Impact

**Code areas:**

- `src/internal/worktree/menu.go` — primary surface area. `ShowMenu` gains a TTY-aware branch.
- `src/internal/worktree/menu_test.go` (new) — unit tests for the new mapping logic.
- `src/go.mod`, `src/go.sum` — new direct dependency (`golang.org/x/term`) pending Q2 outcome.
- All call sites listed in §3 — no API signature change, so call sites are not edited.

**APIs:**

- Public-API impact: zero. `ShowMenu(prompt, options, defaultIdx)` keeps its signature and return type. The new behavior is internal to the function.

**Dependencies:**

- Adds `golang.org/x/term` (pending Q2). This is an `x/` package maintained by the Go team — about as close to stdlib as a non-stdlib dependency gets.

**Systems:**

- Shell-init eval flow (`wt shell-init` → `WT_CD_FILE`): no impact. Arrow-key navigation happens before the worktree-path emission step.
- Operator / CI flows that pipe input: no impact (non-TTY fallback path).
- Signal handling: SIGINT during a raw-mode terminal session must restore cooked mode before propagating the exit — `internal/worktree/rollback.go` and `signal_unix.go` are the relevant locations.

## Open Questions

- **Q3**: When the user presses `Esc` or `Ctrl-C` mid-selection, should the return value be `0` (the existing "Cancel" sentinel) or a distinct error (`ErrUserAbort`) that callers can branch on? Today, callers branch on `choice == 0` — keeping that contract is the lowest-risk path. (Deferred — answered by Assumption #9: keep `0`.)

### Resolved during intake

- **Q1** (was: number keys in interactive mode) — Resolved: number keys remain active. Typing `1`–`9` jumps directly to that option (and submits, matching today's behavior of submit-on-Enter after a one-digit entry).
- **Q2** (was: library choice) — Resolved: hand-rolled using `golang.org/x/term`. Aligns with Principle I (slim single binary).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Change scope is limited to `ShowMenu`; `ConfirmYesNo` and `PromptWithDefault` are out of scope | Those two functions have no notion of "navigable options" — they read free-form text. Arrow keys would be meaningless. | S:90 R:85 A:95 D:90 |
| 2 | Certain | Non-TTY input/output paths fall back to the existing numbered-prompt behavior | Constitution Principle VI mandates graceful degradation when stdout is not a TTY. Required for CI, piped input, `--non-interactive`. | S:85 R:75 A:95 D:95 |
| 3 | Confident | `ShowMenu`'s public signature is preserved (no caller edits) | All ~11 call sites pass `(prompt, options, defaultIdx)` and use the integer return — changing the signature explodes the blast radius for zero UX gain. | S:80 R:60 A:90 D:85 |
| 4 | Confident | The `(default)` row highlight (green marker) is preserved and pre-selects that row on first render | Natural mapping of `defaultIdx` semantics. Matches what every other arrow-key picker does (gh, gum, brew). | S:75 R:80 A:85 D:80 |
| 5 | Confident | Interactive rendering uses in-place redraw (cursor-up + clear + redraw) so scrollback stays clean | Standard pattern for arrow-key pickers. Pollution of scrollback is the #1 complaint about naive implementations. | S:70 R:75 A:80 D:75 |
| 6 | Confident | Signal handling (SIGINT) must restore cooked terminal mode before exit | Without this, a Ctrl-C mid-menu leaves the user's terminal in raw mode (broken). Already required by Go raw-terminal idioms. | S:80 R:50 A:90 D:90 |
| 7 | Certain | Number-key shortcuts (`1`–`9`) remain active alongside arrow keys in interactive mode | Clarified — user confirmed (additive UX, preserves muscle memory from today's numbered prompt). | S:95 R:60 A:90 D:90 |
| 8 | Certain | Library choice: hand-rolled using `golang.org/x/term` (single new dep, no transitive bloat) | Clarified — user chose hand-rolled over survey/v2 and bubbletea. Aligns with Constitution Principle I (slim single binary). | S:95 R:55 A:90 D:90 |
| 9 | Confident | Cancel semantics: Esc / Ctrl-C / q in interactive mode all return choice `0` (the existing Cancel sentinel) | Preserves the existing caller contract (`if choice == 0 { return }`). A distinct error type would force every call site to change. | S:65 R:70 A:90 D:80 |

9 assumptions (4 certain, 5 confident, 0 tentative, 0 unresolved).
