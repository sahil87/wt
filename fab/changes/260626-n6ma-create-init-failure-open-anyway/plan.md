# Plan: wt create — graceful degrade on init-script failure (open-anyway prompt + wt go hint)

**Change**: 260626-n6ma-create-init-failure-open-anyway
**Intake**: `intake.md`

## Requirements

### Init-Failure Banner: `wt go` hint

#### R1: Banner emits a `Go:` navigation hint
`PrintInitFailureBanner` in `src/internal/worktree/errors.go` SHALL emit a `Go:  wt go '<name>'`
hint line on every invocation, single-quoting the name via the existing `shellQuoteSingle` helper,
keyed off a named label constant (`bannerLabelGo`) declared in the SAME `const (...)` block as
`bannerLabelWorktree` / `bannerLabelRetry` / `bannerLabelRemove` and aligned to the existing label
column width. The line SHALL be grouped with the other action hints, placed after the `Worktree:`
line.

- **GIVEN** an init-failure banner is rendered for a kept worktree named `abc`
- **WHEN** `PrintInitFailureBanner(wtPath, "abc", err)` runs
- **THEN** stderr contains a line with `wt go 'abc'` (the name single-quoted)
- **AND** the canonical wording lives only in the `bannerLabelGo` constant (no duplication)
- **AND** the hint appears on every caller path (interactive yes, interactive no, non-interactive),
  because it lives in the shared banner helper.

#### R2: Banner hint single-quotes shell-special names
The `Go:` hint SHALL stay copy-paste-safe when the worktree name contains shell-special characters,
using `shellQuoteSingle` exactly as the sibling `Remove:` hint does.

- **GIVEN** a worktree name containing a single quote (e.g. `my'name`)
- **WHEN** the banner is rendered
- **THEN** the `Go:` hint escapes the embedded quote as `'\''` (matching the `Remove:` hint behavior)

### `wt create` Init-Failure Degradation

#### R3: Non-interactive init failure preserves today's exact behavior
On init-script non-zero exit when NOT interactive (`nonInteractive` set OR stdin is not a TTY),
`wt create` SHALL print the init-failure banner (now including the `Go:` line) and exit
`ExitInitFailed = 7` with NO prompt — byte-for-byte the same flow as today minus the inline exit
location. The interactivity gate is `!nonInteractive` AND `reclaimTTY` (`term.IsTerminal(ttyFd)`,
already computed earlier in the block).

- **GIVEN** `wt create --non-interactive` with an init script that exits non-zero
- **WHEN** init fails
- **THEN** the banner is printed, no prompt is shown, and the process exits 7
- **AND** the worktree directory and branch survive (kept).

#### R4: Interactive init failure offers open-anyway, exits 7 either way
On init-script non-zero exit when interactive (`!nonInteractive` AND stdin is a TTY), `wt create`
SHALL: (1) print the banner; (2) prompt `wt.ConfirmYesNo("Continue and open the worktree anyway?")`;
on **Yes** fall through into the EXISTING Open phase so the user can open the kept worktree; on
**No** skip the Open phase (no app menu). In BOTH cases the function SHALL exit `ExitInitFailed = 7`.
A successful open MUST NEVER downgrade the exit code to 0.

- **GIVEN** an interactive `wt create` with an init script that exits non-zero
- **WHEN** the user answers Yes to the open-anyway prompt
- **THEN** control falls through to the existing Open phase
- **AND** the process exits 7 regardless of whether the open succeeded.
- **GIVEN** the same failure
- **WHEN** the user answers No
- **THEN** the Open phase does not run and the process exits 7.

#### R5: Open-anyway path preserves the kept-worktree + signal/TTY invariants
The restructured block SHALL preserve, on the open-anyway fall-through path, every invariant the
current single-exit path holds: the worktree is KEPT via `rb.Disarm()` (NOT `rb.Execute()`) on
init-script non-zero exit; SIGINT Option B teardown (`signal.Stop(initSigCh)` + `close(initSigCh)`)
runs before the Open phase; the load-bearing terminal-foreground reclaim
(`reclaimTerminalForeground`, gated on `reclaimTTY`) runs before the banner AND before the Open menu.

- **GIVEN** an init failure on a shared-TTY init child
- **WHEN** the user falls through to Open on the Yes path
- **THEN** `rb.Disarm()` has run (worktree kept), the init signal channel is stopped and closed
  before the Open menu, and terminal foreground is reclaimed before both the banner write and the
  Open menu write (no SIGTTOU).

#### R6: Implementation uses an init-failed flag, not inline `os.Exit`
The init-failure block SHALL NOT call `os.Exit(7)` inline at the banner. Instead it SHALL set an
"init failed" flag so the Yes path falls through to the existing Open phase, and the function SHALL
exit 7 at the end on all init-failure paths. The Open phase's normal success exit (which prints the
path line and returns 0) SHALL be overridden to exit 7 when the flag is set.

- **GIVEN** the init-failure flag is set after a Yes-and-successful-open
- **WHEN** the function reaches its normal return/exit
- **THEN** it exits 7 (the success-path `return nil` / path print does not run, or is overridden).

### Non-Goals

- `--reuse` init path — exempt (warn-but-continue refresh), unchanged.
- `*InitNotFound` (init-script-missing) path — non-fatal, exit 0, unchanged.
- Standalone `wt init` — out of scope (does not call `PrintInitFailureBanner`).
- No new exit code: `ExitInitFailed = 7` is reused on every path.

### Design Decisions

1. **Flag-based fall-through over a new open codepath**: set an `initFailed` bool in the failure
   block and let the Yes path fall through to the existing Open phase — *Why*: reuses the
   load-bearing pre-Open terminal-foreground reclaim that already guards the Open menu against
   SIGTTOU; avoids duplicating the open logic — *Rejected*: a separate inline open call in the
   failure block (would duplicate the Open phase and bypass its reclaim).
2. **`Go:` hint in the banner, not the interactive branch**: *Why*: benefits every path including
   non-interactive/CI at zero behavior cost; centralizes the wording in one helper — *Rejected*:
   printing the hint only on the interactive No branch (loses discoverability for non-interactive).
3. **Exit 7 on all paths, including successful open**: *Why*: `ExitInitFailed = 7` is a documented,
   operator-depended-on contract (Constitution III); a successful open downgrading to 0 would erase
   the init-failure signal — *Rejected*: exit 0 on successful open; a new exit code 8.

## Tasks

### Phase 1: Banner hint (errors.go)

- [x] T001 Add `bannerLabelGo = "Go:      "` to the existing banner-label `const (...)` block in `src/internal/worktree/errors.go`, aligned to the same label column width as `bannerLabelWorktree`/`bannerLabelRetry`/`bannerLabelRemove`. <!-- R1 -->
- [x] T002 In `PrintInitFailureBanner` (`src/internal/worktree/errors.go`), emit the `Go:` hint line after the `Worktree:` line and before the `Retry:` line, single-quoting `name` via `shellQuoteSingle`, mirroring the `Remove:` hint's `Fprintf` shape; update the banner-order doc comment to list the new line. <!-- R1 R2 -->

### Phase 2: Init-failure degradation (create.go)

- [x] T003 Restructure the init-failure block (`src/cmd/wt/create.go` ~339-354): replace the inline `os.Exit(wt.ExitInitFailed)` with setting an `initFailed` flag; keep `signal.Stop(initSigCh)`+`close(initSigCh)`, the pre-banner `reclaimTerminalForeground` (gated on `reclaimTTY`), `rb.Disarm()`, and `wt.PrintInitFailureBanner(...)`; after the banner, when interactive (`!nonInteractive && reclaimTTY`) prompt `wt.ConfirmYesNo("Continue and open the worktree anyway?")` — on No, set `worktreeOpen = "skip"` (or otherwise suppress the Open menu) and fall through; on the non-interactive branch, exit 7 immediately (no prompt). The Yes path falls through to the existing Open phase. <!-- R3 R4 R5 R6 -->
- [x] T004 Override the success exit when `initFailed` is set: at the function's normal end (`src/cmd/wt/create.go`, after the Open phase / `rb.Disarm()`), when `initFailed` is true, exit `wt.ExitInitFailed` instead of printing the path line and returning 0, so a successful open never downgrades to 0. Preserve all kept-worktree and signal/TTY invariants on this path. <!-- R4 R6 -->

### Phase 3: Tests

- [x] T005 [P] Extend the banner test in `src/internal/worktree/errors_test.go` to assert the `wt go '<name>'` / `Go:` substring is present (information-surface assertion, not byte equality), including the single-quote-escaping case for a name with an embedded quote. <!-- R1 R2 -->
- [x] T006 Add a non-interactive init-failure test in `src/cmd/wt/create_test.go` asserting the banner now contains the `wt go` hint and the prompt string is NOT present (still exit 7, no prompt) — reuse `createFailingInitScript` + `--non-interactive --worktree-open skip`. <!-- R1 R3 -->
- [x] T007 Add interactive init-failure tests in `src/cmd/wt/create_test.go` driving stdin: Yes -> prompt shown, falls through to Open, exit 7, worktree kept; No -> prompt shown, no app menu, exit 7, worktree kept. Use a non-side-effecting open target / `runWt`'s env isolation / the `WT_TEST_NO_LAUNCH=1` seam so no real editor/tmux window is created. <!-- R4 R5 -->

## Execution Order

- T001 blocks T002 (constant before use).
- T003 blocks T004 (flag must be set before the override reads it).
- T005 is independent ([P]); T006/T007 depend on Phase 1 + Phase 2 being complete.

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `PrintInitFailureBanner` emits a `Go:  wt go '<name>'` line via a named `bannerLabelGo` constant in the shared label block, on every caller path.
- [ ] A-002 R2: The `Go:` hint single-quotes the name via `shellQuoteSingle`, escaping embedded quotes as `'\''`.
- [ ] A-003 R3: Non-interactive (`--non-interactive` / piped / CI) init failure prints the banner and exits 7 with NO prompt; worktree + branch kept.
- [ ] A-004 R4: Interactive init failure prompts `Continue and open the worktree anyway?`; Yes falls through to Open, No skips Open; both exit 7.
- [ ] A-005 R6: No inline `os.Exit(7)` remains in the failure block; an `initFailed` flag drives fall-through and the end-of-function exits 7 when set.

### Behavioral Correctness

- [ ] A-006 R4: A successful open on the Yes path still exits 7 — the success path's path-print/return-0 is overridden when `initFailed` is set.
- [ ] A-007 R5: On the open-anyway path the worktree is kept (`rb.Disarm()`, not `rb.Execute()`), `signal.Stop(initSigCh)`+`close(initSigCh)` run before Open, and `reclaimTerminalForeground` (gated on `reclaimTTY`) runs before both the banner and the Open menu.

### Scenario Coverage

- [ ] A-008 R3: A test asserts non-interactive init failure -> banner (with `wt go`) + exit 7, no prompt string.
- [ ] A-009 R4: Tests assert interactive Yes (falls through to Open, exit 7) and No (no app menu, exit 7), both side-effect-free.
- [ ] A-010 R1: The banner unit test asserts the `wt go` substring on the information surface.

### Edge Cases & Error Handling

- [ ] A-011 R5: Open-anyway tests confirm the worktree directory and branch survive on both Yes and No.
- [ ] A-012 R4: No test creates a real editor/tmux/byobu window (uses a non-side-effecting open target or `WT_TEST_NO_LAUNCH` / env isolation).

### Code Quality

- [ ] A-013 Pattern consistency: New code follows naming and structural patterns of surrounding code (label-constant pattern, `Fprintf` banner shape, the create.go signal/TTY comment density).
- [ ] A-014 No unnecessary duplication: The `Go:` wording lives only in `bannerLabelGo`; the Yes path reuses the existing Open phase rather than a new open codepath.
- [ ] A-015 No magic strings: The prompt string and banner label are the only new literals; the label is a named constant (banner) consistent with siblings.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- gofmt is CI-enforced from `src/` — run `gofmt -l .` after edits.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Process exits `ExitInitFailed = 7` on every init-failure path, including a successful open-anyway open (no exit 0, no new code 8). | Explicitly decided with the user; exit 7 is a documented, depended-on contract (Constitution III; init-failure-contract). A successful-open downgrade to 0 would erase the init-failure signal. | S:98 R:88 A:95 D:95 |
| 2 | Certain | The `wt go '<name>'` hint goes in `PrintInitFailureBanner` itself (not only the interactive branch), single-quoted via `shellQuoteSingle`, with a named `bannerLabelGo` constant alongside the existing banner labels. | Benefits every path incl. non-interactive; mirrors the existing Retry/Remove single-quoting + label-constant pattern; centralizes wording (no duplication). | S:95 R:90 A:95 D:90 |
| 3 | Certain | The open-anyway prompt is interactive-only, gated by `!nonInteractive` AND the existing TTY check (`reclaimTTY`/`term.IsTerminal`); non-interactive/piped/CI keeps today's banner + exit 7 with NO prompt. | Constitution VI; the intake pins the gate to the same TTY/--non-interactive discipline the rest of create uses; `reclaimTTY` is already computed in this block. | S:95 R:85 A:95 D:90 |
| 4 | Certain | On Yes, control falls through to the EXISTING Open phase rather than a new open codepath; SIGINT-teardown and the pre-Open terminal-foreground reclaim run before the Open menu on this path too. | Reuses the load-bearing reclaim guarding the Open menu against SIGTTOU; the intake names these invariants as MUST-preserve. | S:92 R:80 A:90 D:90 |
| 5 | Certain | The worktree is KEPT (`rb.Disarm()`, not `rb.Execute()`) on init-script non-zero exit; `--reuse` and the not-found path remain exempt/unchanged. | Existing contract restated verbatim in the intake; no change requested to rollback, reuse, or not-found semantics. | S:95 R:88 A:95 D:95 |
| 6 | Confident | Implementation uses an `initFailed` flag (set in the failure block) rather than inline `os.Exit`; the Yes path falls through to Open and the function exits 7 at the end; the success path's exit-0/return is overridden to 7 when the flag is set. The No branch suppresses the Open menu by setting `worktreeOpen = "skip"`. | The intake gives this as the implementation note; flag + worktree-open=skip is the minimal, low-blast-radius structuring that reuses the existing Open phase without a new codepath. | S:80 R:75 A:82 D:72 |
| 7 | Confident | Exact prompt wording is `"Continue and open the worktree anyway?"`; the No branch shows no message beyond the banner's `Go:` line. | The intake suggests this exact string and says "show how to reach the worktree" — the banner's `Go:` line satisfies that. Reversible copy decision; a clear front-runner default exists. | S:60 R:80 A:65 D:45 |

7 assumptions (5 certain, 2 confident, 0 tentative).
