# Plan: Show base ref in wt create summary

**Change**: 260717-aeka-create-summary-base-ref
**Intake**: `intake.md`

## Requirements

### wt-cli: Create summary base-ref line

#### R1: The Git-phase summary gains a fourth `From:` line on stderr
The deferred Git-phase summary block in `src/cmd/wt/create.go` SHALL emit a fourth
`key: value` line, `From: <value>`, in the **same** `fmt.Fprintf(os.Stderr, ...)` call that
already prints `Created worktree:` / `Path:` / `Branch:`. The line SHALL be written to
**stderr**; stdout MUST remain byte-identical (the machine-readable path line only).

- **GIVEN** a successful create that reaches the Git-phase summary block
- **WHEN** the summary is emitted
- **THEN** stderr contains a `From: <value>` line immediately after the `Branch:` line
- **AND** stdout still contains only the worktree-path line (no `From:` leak)

#### R2: The `From:` value distinguishes the three creation modes
The `From:` value SHALL reflect which creation path ran:

| Mode | `From:` value |
|------|---------------|
| Positional new branch, `--base` given | the `--base` ref verbatim as typed |
| Positional new branch, no `--base` | resolved HEAD label (`DescribeHead()`) |
| Bare exploratory create | `--base` verbatim if given, else resolved HEAD label |
| `--checkout <branch>` | `existing branch '<branch>' (checked out directly)` |

- **GIVEN** `wt create newfeat --base main`
- **WHEN** the summary is emitted
- **THEN** stderr contains `From: main`

- **GIVEN** `wt create newfeat` invoked from branch `main` with no `--base`
- **WHEN** the summary is emitted
- **THEN** stderr contains `From: main` (the current branch label)

- **GIVEN** `wt create --checkout feature/auth` (feature/auth exists)
- **WHEN** the summary is emitted
- **THEN** stderr contains `From: existing branch 'feature/auth' (checked out directly)`

- **GIVEN** a bare `wt create` (no positional, no `--checkout`) invoked from branch `main`
- **WHEN** the summary is emitted
- **THEN** stderr contains `From: main`

#### R3: `--base` is shown verbatim, not resolved to a SHA
When `--base <ref>` was supplied (positional-new or bare mode), the `From:` value SHALL be the
`--base` token exactly as the user typed it — no `git rev-parse` resolution.

- **GIVEN** `--base` set to a branch name, tag, or SHA
- **WHEN** the `From:` value is computed
- **THEN** it equals the raw `--base` string, with no git query performed to derive it

#### R4: New `DescribeHead()` helper in `internal/worktree`, best-effort
A new exported helper `DescribeHead() string` SHALL live in
`src/internal/worktree/context.go`, next to the existing `CurrentBranch()`. It SHALL return a
display label for the current HEAD: the branch name via `git rev-parse --abbrev-ref HEAD`, or the
short SHA (`git rev-parse --short HEAD`) when HEAD is detached (abbrev-ref returns the literal
`HEAD`). On ANY git error it SHALL return the fallback string `"HEAD"` — the helper MUST NOT
return an error and MUST NOT be able to fail or abort the create.

- **GIVEN** the repo is on a named branch `foo`
- **WHEN** `DescribeHead()` is called
- **THEN** it returns `"foo"`

- **GIVEN** the repo is in detached-HEAD state
- **WHEN** `DescribeHead()` is called
- **THEN** it returns the short commit SHA (not the literal `"HEAD"`)

- **GIVEN** a `git rev-parse` invocation fails
- **WHEN** `DescribeHead()` is called
- **THEN** it returns `"HEAD"` and never panics or errors

#### R5: The `From:` value is resolved BEFORE the create dispatch (outside the reinstall window)
`cmd/create.go` SHALL compute the `From:` value into a local (`createdSummaryFrom`) **before** the
create-dispatch `switch` (before any `git worktree add`), so the one git query `DescribeHead()`
may perform never enters the tight reinstall window between `git worktree add` returning and the
init-phase `signal.Reset`. Resolution logic: `checkout != ""` → the fixed checkout copy (no git
query); `base != ""` → `base` verbatim (no git query); else → `wt.DescribeHead()`.

- **GIVEN** the summary-value resolution
- **WHEN** the code path is inspected
- **THEN** the `From:` value is assigned before the create-dispatch switch, and the only git
  query (`DescribeHead`) sits pre-dispatch — no new subprocess work is introduced between
  `git worktree add` and the init-phase `signal.Reset`

#### R6: `--reuse` and all other paths are unchanged
The `--reuse` collision short-circuit prints `Reusing existing worktree:` and no Git-phase
summary block, so it SHALL gain no `From:` line. The `wt init` path, Open phase, init-failure
paths, and the existing `Created worktree:` / `Path:` / `Branch:` substrings SHALL be unchanged.

- **GIVEN** `wt create --reuse --worktree-name X` that reuses an existing worktree
- **WHEN** it completes
- **THEN** no `From:` line is emitted (there is no summary block on the reuse path)

- **GIVEN** any successful create
- **WHEN** the summary is emitted
- **THEN** the `Created worktree:` / `Path:` / `Branch:` lines are byte-preserved (existing test
  assertions stay green)

### Non-Goals

- Resolving `--base` to a canonical SHA for display — the verbatim token is intentionally kept (R3).
- Showing base information on the `--reuse` path — no branch is created or checked out there (R6).
- Any stdout change — the machine-readable path-line contract is untouched.

### Design Decisions

1. **`From:` value resolved pre-dispatch into a local**: mirrors the existing
   `createdSummaryBranch` pattern — *Why*: keeps the one possible git query (`DescribeHead`) out
   of the reinstall window, and HEAD cannot move between pre-dispatch and summary time since
   `wt create` never moves the invoking worktree's HEAD — *Rejected*: resolving HEAD inside the
   deferred summary block (would add a subprocess call inside the forbidden window).
2. **`DescribeHead()` best-effort with `"HEAD"` fallback**: an informational line must never abort
   a successful create — *Why*: Constitution V keeps git ops in `internal/`; the line is cosmetic
   — *Rejected*: returning `(string, error)` and threading the error into `cmd/` (an informational
   label failing the create is the wrong tradeoff).

## Tasks

### Phase 1: Core Implementation

- [x] T001 Add `DescribeHead() string` to `src/internal/worktree/context.go` next to `CurrentBranch()`: `git rev-parse --abbrev-ref HEAD`, fall back to `git rev-parse --short HEAD` when the output is the literal `HEAD` (detached), return `"HEAD"` on any git error. <!-- R4 -->
- [x] T002 In `src/cmd/wt/create.go`, compute `createdSummaryFrom` before the create-dispatch switch: `checkout != ""` → `fmt.Sprintf("existing branch '%s' (checked out directly)", checkout)`; `base != ""` → `base`; else → `wt.DescribeHead()`. <!-- R2 R3 R5 -->
- [x] T003 In `src/cmd/wt/create.go`, extend the deferred Git-phase summary `fmt.Fprintf(os.Stderr, ...)` with a fourth `From: %s\n` line carrying `createdSummaryFrom`, keeping the existing three lines byte-identical. <!-- R1 R6 -->

### Phase 2: Tests

- [x] T004 [P] Add `TestDescribeHead` to `src/internal/worktree/context_test.go`: on a named branch → branch name; detached HEAD → short SHA (not literal `HEAD`). Reuse the `setupGitRepo(t)` fixture and `os.Chdir` pattern already used in `git_test.go`. <!-- R4 -->
- [x] T005 [P] Add create-summary `From:` assertions to `src/cmd/wt/create_test.go`: (a) `--base <ref>` new branch → stderr contains `From: <ref>`; (b) new branch without `--base` → stderr contains `From: main`; (c) `--checkout <existing>` → stderr contains `existing branch` and `checked out directly`; (d) bare exploratory create → stderr contains `From: main`. <!-- R1 R2 R3 R6 -->

### Phase 3: Polish

- [x] T006 Run `gofmt -l .` from `src/` and `go test ./...`; ensure gofmt is clean (CI fails fast on it) and all tests pass. <!-- R1 R2 R3 R4 R5 R6 -->

## Execution Order

- T001 blocks T002 (create.go calls `wt.DescribeHead()`).
- T002 blocks T003 (the summary line reads `createdSummaryFrom`).
- T004 and T005 are `[P]` — independent test files, run after their implementation tasks.
- T006 runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: The Git-phase summary emits a fourth `From:` line on stderr; stdout remains the single worktree-path line.
- [x] A-002 R2: The `From:` value differs correctly across the four modes (positional+`--base`, positional no-base, bare, `--checkout`).
- [x] A-003 R4: `DescribeHead()` exists in `internal/worktree/context.go`, returns the branch name / short SHA / `"HEAD"` fallback, and returns no error.
- [x] A-004 R6: The `--reuse` path emits no `From:` line and the existing `Created worktree:`/`Path:`/`Branch:` substrings are byte-preserved.

### Behavioral Correctness

- [x] A-005 R3: `--base` is shown verbatim (no `git rev-parse` resolution of the display value).
- [x] A-006 R5: `createdSummaryFrom` is computed before the create-dispatch switch; no new subprocess work sits between `git worktree add` and the init-phase `signal.Reset`.

### Scenario Coverage

- [x] A-007 R2: `create_test.go` exercises all four modes' `From:` output; `context_test.go` exercises `DescribeHead()` on-branch and detached-HEAD.

### Edge Cases & Error Handling

- [x] A-008 R4: A `git` failure inside `DescribeHead()` yields `"HEAD"` and never aborts the create (best-effort contract).

### Code Quality

- [x] A-009 Pattern consistency: The `From:` local mirrors the existing `createdSummaryBranch` idiom; `DescribeHead()` mirrors `CurrentBranch()`; new code follows surrounding naming/structure.
- [x] A-010 No unnecessary duplication: The new line joins the existing single `Fprintf` emission point; no new output machinery is introduced.
- [x] A-011 Internal package boundary (Constitution V): the git query lives in `internal/worktree` (`DescribeHead`), `cmd/` only routes it into the summary — no git op inlined in `cmd/`.
- [x] A-012 Test what the user sees (Constitution IV): `From:` output is asserted end-to-end via the binary in `create_test.go`; `DescribeHead()` has a unit test.
- [x] A-013 gofmt clean: `gofmt -l .` from `src/` reports no files; `go test ./...` passes.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds new functionality without making existing code redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New `From:` line lives in the deferred Git-phase summary block on stderr; stdout byte-identical | `create-output-phases.md` pins stdout = machine path line only; the summary block is the established stderr emission point | S:85 R:90 A:95 D:95 |
| 2 | Confident | Label is `From:` as a fourth `key: value` summary line | Matches the existing `Created worktree:`/`Path:`/`Branch:` shape; the user asked for "a line showing what ref the new branch was created from" | S:70 R:90 A:60 D:60 |
| 3 | Confident | Checkout-path copy: `From: existing branch '<branch>' (checked out directly)` | Echoes the user's "checked out directly onto <ref>" phrasing while keeping the key: value shape; trivially adjustable | S:60 R:90 A:55 D:45 |
| 4 | Confident | HEAD resolved pre-dispatch (before `git worktree add`), emitted inside the existing Fprintf | Reinstall-window contract forbids new subprocess work between worktree-add and `signal.Reset`; HEAD cannot move in between, so pre-resolution is equivalent | S:55 R:80 A:90 D:80 |
| 5 | Confident | New `DescribeHead()` helper in `internal/worktree`, best-effort (`"HEAD"` fallback on git error) | Constitution V keeps git ops out of `cmd/`; an informational line must not abort a successful create | S:50 R:85 A:90 D:80 |
| 6 | Certain | `--reuse` path unchanged (no summary block, no branch created — nothing to show) | Reuse short-circuits before branch dispatch per `create-branch-semantics.md`; out of scope | S:65 R:90 A:90 D:85 |
| 7 | Confident | `--base` shown verbatim as typed, not resolved to a SHA | The user's own token is the most recognizable label; resolution adds a query for no clarity gain | S:55 R:90 A:70 D:65 |

7 assumptions (2 certain, 5 confident, 0 tentative).
