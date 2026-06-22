# Intake: DX copy polish + `wt go` navigation confirmation

**Change**: 260622-log5-dx-copy-polish
**Created**: 2026-06-22

## Origin

Initiated conversationally via `/fab-new`, immediately following a **DX/copy audit** of the entire
`wt` CLI user-facing surface (the audit itself was the user's request: "do a run of all user-facing
copy / DX and see whether all we can be more / less verbose", plus a concrete ask: "In the output of
`wt go`, also specify the name of the repo, the current location etc.").

The audit fanned out three read-only auditors (one per command cluster: create+delete, list+open+go,
init+update+shell-init+main) and the findings were **verified against source** (line numbers
confirmed; one auditor's proposed code that wouldn't compile was corrected). Full report:
`scratchpad/dx-copy-audit.md` (session-local). Findings were triaged into three tiers; the user chose
to implement **Tier 1 + Tier 2** as one coherent change and leave Tier 3 as-is.

Key decisions made this session (all by the user):
- **`wt go` confirmation format**: compact-arrow (`→ {repo} / {worktree}  ({branch})` + indented path),
  chosen over a create-style labeled block and over a single line.
- **Scope**: Tier 1 (4 items) + Tier 2 (help text + glyph) via the full `/fab-fff` pipeline. Tier 3 OUT.
- **Governing rule** (pre-existing, reaffirmed): stdout = machine-readable result; stderr = all human
  copy. See `docs/memory/wt-cli/create-output-phases.md` and the user-memory note
  `wt-stdout-stderr-convention`.

This is a **copy + consistency** change. The ONLY behavior changes are (a) the new `wt go` stderr
confirmation and (b) moving two `delete` warnings from stdout to stderr. Everything else is string
edits and one refactor (a shared warning helper).

## Why

**Problem.** The `wt` CLI copy is well-calibrated overall (not chatty, not cryptic), but the audit
surfaced real **inconsistencies** and one **feature gap**:
1. `wt go` gives no human confirmation of *where* it's navigating — just a bare path. The user can't
   see which repo/worktree/branch they're about to land in. (This was the original request.)
2. Warning output is inconsistent: some warnings color-wrap `%sWarning:%s`, most use bare inline
   `Warning:`; and two `delete` warnings print to **stdout** (a stream-discipline violation) while
   sibling warnings in the same file correctly use stderr.
3. `wt list`'s `--path` not-found error bypasses the `ExitWithError(what,why,fix)` structure that the
   byte-identical not-found case in `open`/`go` uses — ad-hoc `fmt.Fprintf` + `os.Exit` instead.
4. `wt update`'s cobra `Short` is lowercase (`"self-update…"`) — every other command capitalizes.
5. A few flag help strings are terse or use jargon ("porcelain"); one message uses a `≠` glyph that
   renders poorly in some terminals.

**Consequence if unfixed.** `wt go` stays less reassuring than `wt create` (which already prints a
`Created worktree: / Path: / Branch:` summary). The warning-stream bug means scripts that capture
`wt delete` stdout could ingest a warning line. The error-structure and capitalization drift make the
CLI read as less cohesive when commands are compared side-by-side.

**Why this approach.** Surgical, consistency-first. No wholesale rewrites — the audit found nothing
that needs wholesale trimming/expansion. A single shared `wt.Warn` helper collapses ~10 divergent
call sites to one idiom (DRY, matches how `errors.go` already centralizes `WtError`/`RenderWarning`).
The `wt go` confirmation reuses the existing `create`-summary template and the existing
`getBranchForPath` helper, and goes to **stderr** so the stdout machine contract
(`cd "$(command wt go)"`, `WT_CD_FILE`) is untouched.

## What Changes

### 1. `wt go` — stderr navigation confirmation (compact-arrow)

`navigateTo` in `src/cmd/wt/go.go` currently prints only the bare path to stdout (+ the wrapper hint
to stderr when no wrapper). Add a **stderr** confirmation block showing repo + worktree + branch.

- **stdout stays byte-identical**: the bare resolved path remains the last (and only) stdout line.
  This preserves `cd "$(command wt go some-name)"` and the `WT_CD_FILE` write contract
  (launcher-contract.md §3). NON-NEGOTIABLE.
- **stderr gains** (compact-arrow form):
  ```
  → idea / frosted-jaguar  (feature-x)
    /home/sahil/code/sahil87/idea.worktrees/frosted-jaguar
  ```
  Line 1: `→ {RepoName} / {worktree-basename}  ({branch})`. Line 2: two-space-indented absolute path.
- **Wiring**: thread `ctx *wt.RepoContext` into `navigateTo` (it currently takes only `path string`)
  so it has `ctx.RepoName`. Update both call sites (the by-name path ~go.go:74 and the menu path
  ~go.go:102) to pass `ctx`. Derive the branch via the existing `getBranchForPath(path)` in `open.go`
  (one `git rev-parse --abbrev-ref HEAD` in the target worktree — negligible cost; reuse, don't
  reimplement).
- **Ordering**: emit the confirmation block to stderr, then the existing wrapper-hint logic (or vice
  versa — keep the hint and confirmation both on stderr; the bare path is the final stdout write).
  The confirmation should print on the SUCCESS path only (after a worktree is resolved/selected),
  not on cancel/no-worktrees.
- **Color**: if using any color (e.g. dimmed path), respect the existing `NO_COLOR`-blanked package
  color vars (`wt.ColorReset != ""` test) — do NOT call `os.Getenv` afresh. Plain text is acceptable
  too; match the create-summary's level of color.

### 2. Unified warning helper + stream fix

Add a single warning emitter and route all warning call sites through it.

- **New helper** in `src/internal/worktree/errors.go`, alongside `WtError`/`RenderWarning`:
  ```go
  // Warn writes a color-wrapped "Warning:" diagnostic to stderr. Respects the
  // NO_COLOR-blanked package color vars (no fresh os.Getenv).
  func Warn(format string, args ...any) {
      fmt.Fprintf(os.Stderr, "%sWarning:%s %s\n", ColorYellow, ColorReset, fmt.Sprintf(format, args...))
  }
  ```
  (Confirm the exact color var names against the existing file — `ColorYellow`/`ColorReset` per the
  `%sWarning:%s Worktree has uncommitted changes` lines already in `delete.go`.)
- **Convert all warning call sites** to `wt.Warn(...)`:
  - `create.go`: lines ~127 (already color-wrapped, to stderr — convert for uniformity), ~191, ~389,
    ~400, ~404, ~414, ~418 (bare inline `Warning:` → helper).
  - `delete.go`: lines ~397, ~477 (bare inline, stderr → helper), and **~668, ~703 — the STREAM FIX**:
    these use `fmt.Printf` (**stdout**) for a `%sWarning:%s` warning before an interactive menu. Route
    through `wt.Warn` so they land on **stderr**. (`wt delete` has no stdout machine contract, but
    warnings are diagnostics and the rest of the file already uses stderr — this removes the
    inconsistency.)
  - Preserve each warning's wording (the helper only standardizes the `Warning:` prefix + stream +
    color); keep the existing message text. Note line ~668/~703 print surrounding blank lines
    (`\n…\n\n`) for menu spacing — preserve that spacing (the helper appends one `\n`; add the extra
    blank-line framing at the call site if the menu layout needs it).

### 3. `wt list` `--path` not-found → `ExitWithError`

`src/cmd/wt/list.go` ~line 311 (the `--path <name>` lookup, in `handlePathLookup`) does:
```go
fmt.Fprintf(os.Stderr, "Worktree '%s' not found. Use 'wt list' to see available worktrees.\n", name)
os.Exit(wt.ExitGeneralError)
```
Replace with the structured form, byte-parity with `open.go`/`go.go`'s not-found case:
```go
wt.ExitWithError(wt.ExitGeneralError,
    fmt.Sprintf("Worktree '%s' not found", name),
    "No worktree with that name in this repository",
    "Use 'wt list' to see available worktrees")
```

### 4. `wt update` `Short` capitalization

`src/cmd/wt/update.go:17`: `Short: "self-update the wt binary via Homebrew"` →
`Short: "Self-update the wt binary via Homebrew"`.

### 5. Help-text clarity (flag descriptions) + glyph

- `create.go`:
  - `--worktree-open` help → list the option space: `"After creation: prompt (menu), default (auto-detect app), skip, or an app name (e.g. code, cursor)"`.
  - `--non-interactive` help → drop "porcelain" jargon: `"No prompts; use defaults and skip menus"`.
  - (Verify exact current strings/lines before editing — the audit cited ~line 438 area.)
- `delete.go`:
  - `--delete-branch` help → tri-state: `"Delete the associated branch: true (always), false (never), auto (default — only if branch name matches worktree name)"`.
  - `--delete-remote` help → dependency note: `"Delete the remote-tracking branch when the local branch is deleted (true by default)"`.
  - Line ~747: replace the `≠` glyph — `"Skipped branch deletion: %s ≠ worktree name (%s). Use --delete-branch true to force."` →
    `"Skipped branch deletion: branch '%s' does not match worktree name '%s'; use --delete-branch=true to force"`.

> **Verification mandate for apply**: the audit's line numbers were verified at audit time but the
> apply agent MUST re-grep each target string before editing (the file may have shifted). Edit by
> matching the string, not by trusting the line number.

### Out of scope (Tier 3 — deliberately NOT changed)

Menu prompts (`"Select worktree to go to:"` / `"Open in:"` / `"Cancelled."` / `"No worktrees found."`),
the `go`/`open` wrapper hints, `main.go:43` root error sink (intentional raw catch-all), `init.go`'s
inline failure line, and `list.go`'s `"Worktrees for:"` header — all judged well-calibrated. Do not
touch them.

## Affected Memory

- `wt-cli/go-command-contract`: (modify) — document the new `wt go` stderr navigation-confirmation
  block (compact-arrow `→ {repo} / {worktree} ({branch})` + indented path), and reaffirm that stdout
  stays the bare path only. The existing contract already covers navigateTo's WT_CD_FILE + stdout
  behavior; this adds the stderr confirmation.
- `wt-cli/create-output-phases`: (modify, optional) — this is the canonical stdout/stderr stream
  contract. Optionally note the new `wt.Warn` helper as the single warning emitter (stderr, color-
  wrapped) and that `wt delete`'s two formerly-stdout warnings were realigned to stderr. If a
  dedicated warning-helper contract fits better as its own file, the hydrate stage may create
  `wt-cli/warning-output-contract` (new) instead — apply/hydrate's judgment.

## Impact

**Code:**
- `src/cmd/wt/go.go` — `navigateTo` signature (`+ctx`), confirmation block, call-site updates.
- `src/internal/worktree/errors.go` — new `Warn` helper.
- `src/cmd/wt/create.go` — warning call sites → `wt.Warn`; `--worktree-open`/`--non-interactive` help.
- `src/cmd/wt/delete.go` — warning call sites → `wt.Warn` (incl. the two stdout→stderr fixes);
  `--delete-branch`/`--delete-remote` help; `≠` glyph.
- `src/cmd/wt/list.go` — `--path` not-found → `ExitWithError`.
- `src/cmd/wt/update.go` — `Short` capitalization.

**Tests** (Constitution IV — test what the user sees):
- `go_test.go` / `integration_test.go` — assert the new stderr confirmation appears on `wt go`
  success AND that **stdout is still exactly the bare path** (the critical regression guard). Assert
  it does NOT appear on cancel/no-worktrees.
- `delete_test.go` — assert the two realigned warnings now appear on **stderr**, not stdout.
- `errors_test.go` — unit-test `Warn` (color-wrapped to stderr; NO_COLOR → plain, no ANSI).
- `list_test.go` — the `--path` not-found path now emits the what/why/fix structure (assert on the
  combined/stderr output and exit code).
- Update any existing test asserting the old `update` `Short`, the old `≠` message, or the old help
  strings (Test Integrity: tests conform to the new spec).

**Exit codes / contracts:** No new exit codes. No env-var changes. The `wt open` launcher-contract
surface `hop` depends on is untouched. `wt go` stdout contract is preserved (the whole point).

**CI:** gofmt-enforced (module root `src/`) — run `gofmt -l`, `go vet`, `go test ./...` from `src/`.

## Open Questions

- None blocking. The one judgment call left to apply: whether the warning realignment + helper
  warrants a new `wt-cli/warning-output-contract` memory file vs. a note appended to
  `create-output-phases` — deferred to hydrate.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `wt go` stdout stays the bare path; confirmation goes to stderr only | Governing stream rule + the launcher/WT_CD_FILE contract depend on it; user-confirmed | S:95 R:85 A:100 D:95 |
| 2 | Certain | `wt go` confirmation uses compact-arrow `→ {repo} / {worktree} ({branch})` + indented path | User explicitly chose this format over the two alternatives this session | S:95 R:90 A:95 D:95 |
| 3 | Certain | Scope = Tier 1 + Tier 2; Tier 3 explicitly excluded | User chose this scope this session; Tier 3 items listed as out-of-scope | S:95 R:80 A:95 D:90 |
| 4 | Confident | Single `wt.Warn` helper in errors.go is the right consolidation | Matches how errors.go already centralizes WtError/RenderWarning; collapses ~10 sites; DRY | S:80 R:75 A:90 D:80 |
| 5 | Confident | Realign delete.go:668/703 warnings stdout→stderr (not a breaking change) | wt delete has no stdout machine contract; warnings are diagnostics; sibling warnings already use stderr | S:80 R:70 A:90 D:80 |
| 6 | Confident | Reuse `getBranchForPath` (open.go) for the branch in the confirmation | Already exists and is used by open's menu; reuse over reimplement (Constitution V) | S:75 R:85 A:90 D:80 |
| 7 | Confident | Preserve each warning's existing message text; helper only standardizes prefix/stream/color | Minimizes blast radius; the audit flagged format not wording | S:75 R:80 A:85 D:80 |
| 8 | Tentative | Whether warning realignment gets its own memory file or a note on create-output-phases | Either is defensible; deferred to hydrate's judgment | S:50 R:75 A:60 D:45 |

8 assumptions (3 certain, 4 confident, 1 tentative, 0 unresolved).
