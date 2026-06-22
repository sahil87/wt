# Plan: DX copy polish + `wt go` navigation confirmation

**Change**: 260622-log5-dx-copy-polish
**Intake**: `intake.md`

## Requirements

### CLI Output: `wt go` navigation confirmation

#### R1: `wt go` SHALL emit a stderr navigation confirmation on success
On a successful navigation (a worktree was resolved by name or selected from the
menu), `wt go` SHALL write a compact-arrow confirmation block to **stderr**:
line 1 `→ {RepoName} / {worktree-basename}  ({branch})`, line 2 a two-space-indented
absolute path. The branch SHALL be derived via the existing `getBranchForPath(path)`.

- **GIVEN** a repo with a worktree `frosted-jaguar` on branch `feature-x`
- **WHEN** the user runs `wt go frosted-jaguar`
- **THEN** stderr contains `→ {repo} / frosted-jaguar  (feature-x)` followed by an indented absolute path line
- **AND** the confirmation is NOT emitted on the cancel or no-worktrees paths

#### R2: `wt go` stdout SHALL remain exactly the bare resolved path (NON-NEGOTIABLE)
The confirmation is diagnostic copy and MUST NOT alter the stdout machine contract.
stdout SHALL stay the single bare resolved absolute path as the final (and only)
stdout line, preserving `cd "$(command wt go <name>)"` and the `WT_CD_FILE` write.

- **GIVEN** the user runs `wt go <name>` with `WT_CD_FILE` set
- **WHEN** navigation succeeds
- **THEN** stdout is exactly the resolved path (no confirmation text on stdout)
- **AND** `WT_CD_FILE` contains the resolved path at mode 0600

#### R3: `navigateTo` SHALL receive `*wt.RepoContext` to render the confirmation
`navigateTo(path string)` SHALL become `navigateTo(ctx *wt.RepoContext, path string)`
so it can render `ctx.RepoName`. Both call sites (by-name and menu) SHALL pass `ctx`.

- **GIVEN** the two `navigateTo` call sites in `go.go`
- **WHEN** the change is applied
- **THEN** both pass the resolved `ctx` and the binary compiles

### Diagnostics: unified warning helper + stream discipline

#### R4: A single `wt.Warn` helper SHALL emit color-wrapped warnings to stderr
`internal/worktree/errors.go` SHALL gain `func Warn(format string, args ...any)`
that writes `%sWarning:%s %s\n` (`ColorYellow`/`ColorReset` + formatted message) to
**stderr**, respecting the existing NO_COLOR-blanked package color vars (no fresh
`os.Getenv`).

- **GIVEN** color is enabled
- **WHEN** `Warn("msg %d", 1)` is called
- **THEN** stderr receives `\033[0;33mWarning:\033[0m msg 1\n`
- **AND** under NO_COLOR (blanked vars) stderr receives `Warning: msg 1\n` with no ANSI

#### R5: All `create.go`/`delete.go` warning call sites SHALL route through `wt.Warn`
Every `Warning:`-prefixed diagnostic in `create.go` and `delete.go` SHALL be
converted to `wt.Warn(...)`, preserving each message's existing wording (the helper
only standardizes the prefix/stream/color).

- **GIVEN** the divergent warning call sites
- **WHEN** converted
- **THEN** each emits its original message text via `wt.Warn`, landing on stderr

#### R6: `delete.go`'s two pre-menu warnings SHALL move from stdout to stderr
The uncommitted-changes and unpushed-commits warnings (currently `fmt.Printf` →
stdout) SHALL be routed through `wt.Warn` (→ stderr). Their surrounding blank-line
spacing for menu layout SHALL be preserved.

- **GIVEN** an interactive `wt delete` of a worktree with uncommitted changes / unpushed commits
- **WHEN** the warning prints
- **THEN** it appears on **stderr**, not stdout
- **AND** a leading blank line precedes it and a trailing blank line follows it (menu spacing preserved)

### CLI Consistency: structured errors, capitalization, help text, glyph

#### R7: `wt list --path` not-found SHALL use the structured `ExitWithError` form
`handlePathLookup`'s raw `fmt.Fprintf(os.Stderr, ...) + os.Exit(...)` SHALL be
replaced with `wt.ExitWithError(wt.ExitGeneralError, "Worktree '%s' not found",
"No worktree with that name in this repository", "Use 'wt list' to see available
worktrees")` — byte-parity with `open.go`/`go.go`'s not-found case.

- **GIVEN** `wt list --path no-such`
- **WHEN** the name does not resolve
- **THEN** stderr shows the `Error:`/`Why:`/`Fix:` structure and exit code is 1 (ExitGeneralError)

#### R8: `wt update` `Short` SHALL be capitalized
`update.go`'s `Short: "self-update the wt binary via Homebrew"` SHALL become
`"Self-update the wt binary via Homebrew"`.

- **GIVEN** `wt --help` / `wt update --help`
- **WHEN** rendered
- **THEN** the update Short begins with a capital "S"

#### R9: Flag help strings SHALL be clarified and the `≠` glyph replaced
`create.go` `--worktree-open` and `--non-interactive`, and `delete.go`
`--delete-branch` and `--delete-remote`, SHALL adopt the intake's clarified strings.
`delete.go`'s auto-mode skip message SHALL replace the `≠` glyph with plain ASCII,
keeping its two args `(branch, wtName)`.

- **GIVEN** the relevant `--help` output and the auto-mode skip path
- **WHEN** rendered/triggered
- **THEN** the new strings appear and no `≠` glyph is emitted

### Non-Goals

- Tier 3 items are explicitly excluded: menu prompts, go/open wrapper hints,
  `main.go` root error sink, `init.go` inline failure line, `list.go`
  "Worktrees for:" header.
- No memory or spec hydration in this stage (hydrate stage owns memory; specs
  unchanged because the help-text edits do not alter the documented surface —
  `cli-surface.md` paraphrases behavior, not the verbatim Go help strings).

### Design Decisions

1. **Confirmation on stderr, never stdout**: the launcher/`WT_CD_FILE` contract and
   `cd "$(command wt go)"` depend on a bare-path stdout — *Why*: governing
   stream-discipline rule (stdout=machine, stderr=human) — *Rejected*: printing the
   block to stdout (breaks scripting).
2. **`wt.Warn` in `internal/worktree/errors.go`, not `cmd/`**: collapses ~10 divergent
   call sites and keeps `cmd/` thin (Constitution V) — *Why*: matches how `errors.go`
   already centralizes `WtError`/`RenderWarning` — *Rejected*: a `cmd/`-local helper.
3. **Reuse `getBranchForPath`**: already used by the open/go menus — *Why*: reuse over
   reimplement (Constitution V) — *Rejected*: a fresh `git rev-parse` in `go.go`.

## Tasks

### Phase 1: Helper

- [x] T001 Add `func Warn(format string, args ...any)` to `src/internal/worktree/errors.go` (alongside `WtError`/`PrintError`): writes `%sWarning:%s %s\n` to `os.Stderr` using `ColorYellow`/`ColorReset`. <!-- R4 -->

### Phase 2: Core string/flow edits

- [x] T002 In `src/cmd/wt/go.go`: change `navigateTo(path string)` to `navigateTo(ctx *wt.RepoContext, path string)`, add the stderr compact-arrow confirmation block (`→ {ctx.RepoName} / {filepath.Base(path)}  ({getBranchForPath(path)})` + 2-space-indented absolute path) emitted before the wrapper-hint/`WT_CD_FILE` logic, keep `fmt.Println(path)` as the last stdout write; add `path/filepath` import. <!-- R1 R2 R3 -->
- [x] T003 In `src/cmd/wt/go.go`: update both `navigateTo` call sites (by-name `~:74`, menu `~:102`) to `navigateTo(ctx, path)`. <!-- R3 -->
- [x] T004 In `src/cmd/wt/create.go`: convert all `Warning:` call sites (uncommitted-changes ~:127; reuse-init ~:191; open-menu/default/named-app warnings ~:389/:400/:404/:414/:418) to `wt.Warn(...)`, stripping the literal `Warning: ` prefix and preserving the remaining message text. <!-- R5 -->
- [x] T005 In `src/cmd/wt/delete.go`: convert the two `fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %s\n", ...)` warnings (~:397, ~:477) to `wt.Warn("failed to remove %s: %s", ...)`. <!-- R5 -->
- [x] T006 In `src/cmd/wt/delete.go`: route the two pre-menu warnings (uncommitted-changes ~:668; unpushed-commits ~:703) through `wt.Warn` (stdout→stderr), framing each with a leading `fmt.Fprintln(os.Stderr)` and a trailing `fmt.Fprintln(os.Stderr)` to preserve menu spacing. <!-- R5 R6 -->
- [x] T007 In `src/cmd/wt/list.go` `handlePathLookup`: replace the raw `fmt.Fprintf(os.Stderr, "Worktree '%s' not found. ...") + os.Exit(...) + return nil` with `wt.ExitWithError(wt.ExitGeneralError, fmt.Sprintf("Worktree '%s' not found", name), "No worktree with that name in this repository", "Use 'wt list' to see available worktrees")`; drop the now-unreachable `return nil` if Go flags it (ExitWithError does not return). <!-- R7 -->
- [x] T008 In `src/cmd/wt/update.go`: capitalize `Short` to `"Self-update the wt binary via Homebrew"`. <!-- R8 -->
- [x] T009 In `src/cmd/wt/create.go`: set `--worktree-open` help to `"After creation: prompt (menu), default (auto-detect app), skip, or an app name (e.g. code, cursor)"` and `--non-interactive` help to `"No prompts; use defaults and skip menus"`. <!-- R9 -->
- [x] T010 In `src/cmd/wt/delete.go`: set `--delete-branch` help to `"Delete the associated branch: true (always), false (never), auto (default — only if branch name matches worktree name)"`, `--delete-remote` help to `"Delete the remote-tracking branch when the local branch is deleted (true by default)"`, and replace the `≠` skip message with `"Skipped branch deletion: branch '%s' does not match worktree name '%s'; use --delete-branch=true to force"` (keep args `branch, wtName`). <!-- R9 -->

### Phase 3: Tests

- [x] T011 In `src/cmd/wt/go_test.go`: add a test that `wt go <name>` success emits the confirmation block on **stderr** (`→`, repo name, worktree name, branch, indented path) AND that stdout is still exactly the bare path (regression guard for R2). <!-- R1 R2 -->
- [x] T012 In `src/cmd/wt/go_test.go`: add a test that the confirmation does NOT appear on the no-worktrees menu path (no `→` on stderr). <!-- R1 -->
- [x] T013 In `src/internal/worktree/errors_test.go`: unit-test `Warn` — color-wrapped to stderr when color enabled; plain `Warning: ` with no ANSI when color vars are blanked (NO_COLOR). <!-- R4 -->
- [x] T014 In `src/cmd/wt/delete_test.go`: add a test asserting the uncommitted-changes / unpushed-commits warning appears on **stderr** and NOT on stdout. <!-- R6 -->
- [x] T015 In `src/cmd/wt/list_test.go`: add a test that `wt list --path <unknown>` emits the structured what/why/fix (`Error:`, `Why:`, `Fix:`) on stderr and exits 1. <!-- R7 -->

## Execution Order

- T001 blocks T004, T005, T006 (they call `wt.Warn`).
- T002 blocks T003 (signature before call-site updates).
- Phase 3 tests follow their corresponding implementation tasks.

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `wt go <name>` (and the menu path) emits `→ {repo} / {worktree}  ({branch})` + an indented absolute path on stderr on success
- [ ] A-002 R3: `navigateTo` takes `(ctx *wt.RepoContext, path string)` and both call sites pass `ctx`; binary compiles
- [ ] A-003 R4: `wt.Warn(format, args...)` exists in `errors.go`, writes color-wrapped `Warning:` + message to stderr
- [ ] A-004 R5: every former `Warning:` call site in `create.go`/`delete.go` now calls `wt.Warn` with its original wording preserved
- [ ] A-005 R7: `wt list --path <unknown>` uses `wt.ExitWithError(ExitGeneralError, ...)` (structured what/why/fix)
- [ ] A-006 R8: `wt update` Short reads "Self-update the wt binary via Homebrew"
- [ ] A-007 R9: the four flag help strings and the auto-mode skip message match the intake's strings; no `≠` glyph remains

### Behavioral Correctness

- [ ] A-008 R2: `wt go` stdout is exactly the bare resolved path (no confirmation leaks to stdout); `WT_CD_FILE` still written at mode 0600 (regression guard test passes)
- [ ] A-009 R6: the two `delete` pre-menu warnings now land on stderr, not stdout, with leading/trailing blank-line spacing preserved

### Scenario Coverage

- [ ] A-010 R1: a test asserts the confirmation does NOT appear on the cancel/no-worktrees path
- [ ] A-011 R4: a test asserts `Warn` emits ANSI when colored and plain text under blanked color vars

### Edge Cases & Error Handling

- [ ] A-012 R7: `wt list --path <unknown>` exits with code 1 (ExitGeneralError)

### Code Quality

- [ ] A-013 Pattern consistency: new code follows the cobra/stderr/`wt.`-helper patterns of surrounding code (Constitution II/V); the confirmation respects NO_COLOR-blanked color vars without fresh `os.Getenv`
- [ ] A-014 No unnecessary duplication: branch derivation reuses `getBranchForPath`; warnings reuse the single `wt.Warn` helper
- [ ] A-015 Tooling: `gofmt -l .`, `go vet ./...`, `go test ./...` are all clean/green from `src/`

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `wt go` stdout stays the bare path; confirmation goes to stderr only | Governing stream rule + launcher/WT_CD_FILE contract; user-confirmed in intake | S:95 R:85 A:100 D:95 |
| 2 | Certain | Compact-arrow confirmation `→ {repo} / {worktree}  ({branch})` + indented path | User chose this exact format over alternatives (intake) | S:95 R:90 A:95 D:95 |
| 3 | Confident | `wt.Warn` helper in errors.go is the right consolidation | Matches errors.go's WtError/RenderWarning centralization; DRY (Constitution V) | S:80 R:75 A:90 D:80 |
| 4 | Confident | Strip literal `Warning: ` from each converted format string (helper re-adds it) | Helper standardizes the prefix; preserving it in the format would double it | S:85 R:85 A:90 D:90 |
| 5 | Confident | Frame the two delete warnings with `Fprintln(os.Stderr)` before+after to reproduce the old `\n…\n\n` menu spacing | Old form had leading `\n` + trailing `\n\n`; Warn appends one `\n`, so one blank line each side reproduces it | S:80 R:80 A:85 D:80 |
| 6 | Confident | Reuse `getBranchForPath` (open.go) for the branch in the confirmation | Already exists and used by open/go menus; reuse over reimplement | S:75 R:85 A:90 D:80 |
| 7 | Confident | docs/specs/cli-surface.md left unchanged | It paraphrases behavior (unchanged), not the verbatim Go help strings | S:75 R:80 A:85 D:80 |

7 assumptions (2 certain, 5 confident, 0 tentative).
