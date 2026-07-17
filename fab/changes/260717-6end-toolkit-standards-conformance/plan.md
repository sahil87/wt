# Plan: Toolkit Standards Conformance

**Change**: 260717-6end-toolkit-standards-conformance
**Intake**: `intake.md`

> Standards were re-enumerated at apply time via `shll standards` + `shll standards <name>` (shll v0.0.23, matching the intake snapshot). The four standards are `principles`, `help-dump`, `readme-extraction`, `skill`. This plan records the audit verdicts and the proportionate fixes/deferrals. The full per-standard PASS/gap report is written to `conformance-report.md` in this change folder for the ship stage to embed in the PR body.

## Requirements

### Conformance: help-dump (mechanical contract)

#### R1: help-dump PASSES its verification checklist verbatim
The audit SHALL execute the help-dump standard's own "Verifying conformance" checklist against the built binary and confirm PASS with receipts. No fix required (documentation-only verdict).

- **GIVEN** the binary built by `just build` (version stamped from `git describe`)
- **WHEN** `wt help-dump` is run and its stdout/stderr/exit inspected
- **THEN** exit is 0, stderr is empty, stdout is valid JSON, the envelope is exactly `{tool, version, schema_version, root}` with no `captured_at`, `completion`/`help`/`help-dump` (and all hidden nodes) are absent from the tree, and `version` reflects the built binary (not a literal)
- **AND** the pinned test `TestHelpDump_EmitsValidEnvelope` (`src/cmd/wt/help_dump_test.go`) asserts exit 0 + valid JSON + `tool`/`schema_version` + `captured_at` absence + filter rules, protecting the contract surface

### Conformance: readme-extraction (mechanical contract)

#### R2: The command-reference URL matches the standard's `https://shll.ai/<tool>/commands/` form
`README.md` SHALL link the auto-generated command reference at the absolute URL `https://shll.ai/wt/commands/` (the standard's rule 8 form), not the incorrect `https://shll.ai/tools/wt/commands/`.

- **GIVEN** `README.md` line 85 currently reads `[command reference](https://shll.ai/tools/wt/commands/)`
- **WHEN** the readme-extraction standard's rule 8 (`point at the generated command reference with the absolute URL https://shll.ai/<tool>/commands/`) is applied for `<tool> = wt`
- **THEN** the link target becomes `https://shll.ai/wt/commands/`
- **AND** re-running the readme-extraction grep checklist afterward shows no relative-target, relative-image, reserved-name, mermaid, or `#gh-*` violations remaining (the rest of the README + `docs/site/` tree already PASS)

#### R3: The remainder of the README + docs/site tree PASSES readme-extraction
The audit SHALL confirm (documentation-only, no fix) that README top-order (H1 → toolkit blockquote → badges → prose), the tail rule (no `Contributing`/`Development`/`Building`/`License`/`Acknowledgements` heading — whole README is site-worthy), image absoluteness, `docs/site/` closure (between-page `./…` links only, external links absolute), and reserved-name rules all hold.

- **GIVEN** `README.md`, `docs/site/install.md`, `docs/site/workflows.md`
- **WHEN** the readme-extraction "Verifying conformance" checklist is executed verbatim
- **THEN** every item passes except the R2 URL (dispositioned separately)

### Conformance: principles (foundation, №1–№10)

#### R4: Non-TTY menu invocations fail with an actionable, flag-naming error (№1 / №4)
The shared fallback-menu path SHALL, when stdin reaches EOF with no selection (the non-TTY / piped case), return a structured, actionable error naming the non-interactive escape (a worktree name or `--non-interactive`) — not the bare `reading input: EOF`. This closes the №1 "refuse with an error naming that flag, never a hang" gap and the №4 "what failed / why / what next" gap for the interactive-menu commands (`wt open` main-repo menu, `wt go` no-arg, `wt delete` no-arg) at the single choke point.

- **GIVEN** `wt open` (from the main repo), `wt go` (no name), or `wt delete` (no name) run with stdin not a TTY (e.g. `</dev/null`)
- **WHEN** the fallback numbered-menu prompt reaches EOF before a choice is entered
- **THEN** the command exits non-zero (never hangs) with a structured `Error:`/`Why:`/`Fix:` message stating that a selection menu cannot run without a TTY and naming the non-interactive way to proceed (pass a worktree name, or use `--non-interactive` where the command supports it)
- **AND** the interactive TTY path, the successful piped-input path (a valid numeric choice on stdin), and the existing non-interactive refusals (`wt go --non-interactive`, `wt delete --non-interactive`) are unchanged

#### R5: The remaining principles are dispositioned per-command with honest PASS/gap verdicts
The audit SHALL assess each of the ten principles against `wt`'s actual behavior across all user-facing commands (`create`, `list`, `open`, `go`, `delete`, `init`, `shell-init`, `update`, root) and record a per-principle disposition in `conformance-report.md`. Gaps that are restructuring-sized SHALL be deferred to `fab/backlog.md`, not fixed here.

- **GIVEN** the per-command behavior observed during the audit (TTY handling, stdout/stderr split, `--json`/`--non-interactive` coverage, exit codes + error wording, idempotency/rollback, output volume, graceful degradation, composition)
- **WHEN** each principle №1–№10 is evaluated
- **THEN** each is dispositioned PASS, "fixed in this change" (R2, R4), or "deferred to [<backlog-id>]" in the report, with the two deferrals below recorded as backlog entries

### Conformance: skill (binary+repo) + deferred principle gaps

#### R6: The skill standard is reported "deferred, not yet adopted", and restructuring-sized principle gaps are deferred to the backlog
`wt` has no `skill` subcommand; per the standard's own Adoption section ("No tool ships `skill` today"; phased per-repo, no seven-repo flag-day) and the intake, the report SHALL mark it "deferred, not yet adopted" with a backlog tracking id. The two restructuring-sized principle gaps found in the audit — (a) `wt delete`'s non-error human copy printed to stdout instead of stderr (№2 stream-split; command-wide realignment, no stdout machine contract today) and (b) `wt delete` (and any destructive path) lacking `--dry-run` (№5; a new flag + preview code path sharing the live path) — SHALL each be recorded as a `fab/backlog.md` entry and referenced from the report.

- **GIVEN** `wt skill` returns `unknown command`, `wt delete` writes progress/summary lines to stdout, and no command supports `--dry-run`
- **WHEN** the fix-vs-defer proportionality policy from the intake is applied (fix = flag additions on existing commands, stream reroute of a line, error wording, doc-structure; defer = new subcommands, cross-command output redesign, new preview code paths)
- **THEN** `skill`, the `wt delete` stdout→stderr realignment, and `wt delete --dry-run` are each deferred with a 4-char lowercase alphanumeric backlog id and a dated line matching the existing `fab/backlog.md` entry style
- **AND** each deferral is referenced in `conformance-report.md` as `deferred to [<id>]`

### Conformance: deliverable

#### R7: The conformance report is saved into the change folder for the ship stage
A `conformance-report.md` SHALL be written to `fab/changes/260717-6end-toolkit-standards-conformance/` with one section per standard in `shll standards` list order (`principles`, `help-dump`, `readme-extraction`, `skill`), headed by the shll version from `shll version`'s shll row (v0.0.23), each gap carrying exactly one disposition (`fixed in this change` or `deferred to [<id>]`).

- **GIVEN** the audit verdicts and the fixes/deferrals above
- **WHEN** the report is composed
- **THEN** it contains the four standard sections in list order, the audited shll version, the ten principles individually dispositioned, and the mechanical checklists' receipts, and it lives in the change folder (not the PR body directly — the ship stage embeds it)

### Verification

#### R8: Tests green and command tree unchanged
`gofmt -l` on the Go tree under `src/` SHALL report no files, and `go test ./...` from `src/` SHALL pass. Because no fix in this change adds/removes a command or flag (R4 changes only an error string; R2 changes only README prose), the command tree is unchanged and `wt help-dump` output is byte-stable — but the help-dump checklist SHALL nonetheless be re-run once after the fixes to confirm.

- **GIVEN** the R2 and R4 fixes applied
- **WHEN** `gofmt -l .` and `go test ./...` are run from `src/`
- **THEN** gofmt lists nothing and all tests pass
- **AND** `wt help-dump` (rebuilt) still passes its checklist (exit 0, 8 visible subcommands, no `captured_at`)

### Non-Goals

- Implementing a `wt skill` subcommand (deferred — phased per-repo adoption; no tool ships it yet).
- Realigning `wt delete`'s non-warning human copy from stdout to stderr (deferred — command-wide output realignment; `wt delete` has no stdout machine contract today).
- Adding `--dry-run` to `wt delete` (deferred — new flag + preview code path sharing the live path).
- Adding `--non-interactive` to `wt open`'s main-repo selection menu, or `--quiet`/`--json` to commands that lack them (out of proportionate scope; R4's actionable-error fix addresses the concrete non-TTY gap).

### Design Decisions

1. **Fix the non-TTY menu error at the shared `runFallbackMenu` choke point, not per-command**: One typed error in `internal/worktree/menu.go`'s fallback reader covers `wt open`, `wt go`, and `wt delete` menu paths simultaneously — *Why*: keeps `cmd/` thin (Constitution V), single source of truth, no per-command duplication — *Rejected*: adding `--non-interactive` to `wt open` + editing three call sites (larger surface, changes the command tree, over-scoped for the concrete gap).
2. **Defer the `wt delete` stdout→stderr realignment rather than fix it here**: — *Why*: it is a ~20-call-site command-wide realignment (the intake's "cross-command output redesign" defer bucket), and the *warnings* were already realigned as their own change (260622-log5), so the non-warning realignment is a coherent standalone follow-up; `wt delete` has no stdout machine contract, so nothing programmatic breaks meanwhile — *Rejected*: fixing all call sites here (exceeds the small-additive proportionality boundary and risks churn in a conformance-audit change).
3. **Record deferrals as `fab/backlog.md` entries (4-char id), not GitHub issues or draft changes**: — *Why*: backlog.md is this repo's live deferral convention (existing dated `[id]` entries) — *Rejected*: draft fab changes (heavier; these gaps are not yet fully specified) / GitHub issues (not this repo's convention).

## Tasks

### Phase 1: Mechanical-contract fix (readme-extraction)

- [x] T001 Fix the command-reference URL in `README.md` line 85: change `https://shll.ai/tools/wt/commands/` to `https://shll.ai/wt/commands/` (readme-extraction rule 8). No other README/docs-site edits — the rest of the tree already passes. <!-- R2 -->
- [x] T002 Re-run the readme-extraction grep checklist (relative targets, relative images, `#gh-*`, mermaid, reserved names, top-order) against `README.md` + `docs/site/**` and confirm the URL is the only change and no new violation was introduced. <!-- R2 R3 -->

### Phase 2: Principle fix (non-TTY actionable error)

- [x] T003 In `src/internal/worktree/menu.go`, change `runFallbackMenu`'s EOF/read-error return (currently `return 0, fmt.Errorf("reading input: %w", err)`) to a typed, actionable error whose message follows the `Error:`/`Why:`/`Fix:` shape (reuse `WtError`) — stating a selection menu cannot run without a TTY and naming the escape (pass a worktree name, or use `--non-interactive` where supported). Keep the non-EOF invalid-input loop behavior unchanged; keep the exit non-zero (never a hang). <!-- R4 -->
- [x] T004 Add/extend a unit test in `src/internal/worktree/menu_test.go` asserting that `runFallbackMenu` on EOF stdin returns a non-nil error carrying the actionable wording (menu-cannot-run-without-TTY + the flag/name hint), and that a valid numeric choice on stdin still returns the chosen index with no error. <!-- R4 -->
- [x] T005 Add an integration assertion in `src/cmd/wt/integration_test.go` (or the nearest existing menu integration test) that `wt open` from the main repo, `wt go` (no name), and `wt delete` (no name) with stdin `</dev/null` exit non-zero and print the actionable menu-refusal wording to stderr (not the bare `reading input: EOF`). <!-- R4 -->

### Phase 3: Deferrals (backlog)

- [x] T006 Append three `fab/backlog.md` entries (4-char lowercase alphanumeric ids, dated `2026-07-18` lines, matching the existing `- [ ] [id] YYYY-MM-DD: …` style): (a) `wt skill` subcommand adoption (skill standard — deferred, not yet adopted); (b) realign `wt delete`'s non-warning human copy (Worktree/Branch/Path/Removing/Deleted/Cancelled/branch-cleanup lines) from stdout to stderr per the stdout=machine / stderr=human convention (№2); (c) add `--dry-run` to `wt delete` (and audit other destructive paths) with a preview sharing the live code path (№5). Capture the generated ids for the report. <!-- R6 -->

### Phase 4: Deliverable + verification

- [x] T007 Write `fab/changes/260717-6end-toolkit-standards-conformance/conformance-report.md`: header `## Conformance report — audited against shll v0.0.23`; one section per standard in `shll standards` list order (`principles` — all ten individually dispositioned; `help-dump` — PASS with checklist receipts; `readme-extraction` — the R2 URL fixed-in-this-change + rest PASS; `skill` — deferred, not yet adopted with the backlog id); every gap carries exactly one disposition (`fixed in this change` / `deferred to [<id>]`) using the T006 ids. <!-- R7 R5 R6 R1 R3 -->
- [x] T008 From `src/`: run `gofmt -l .` (expect no output) and `go test ./...` (expect pass); fix any failures. Rebuild via `just build` and re-run the help-dump checklist (`wt help-dump`: exit 0, empty stderr, valid JSON, no `captured_at`, 8 visible subcommands) to confirm the command tree is unchanged. <!-- R8 R1 -->

## Execution Order

- T001 → T002 (verify after edit).
- T003 → T004 → T005 (implement, then unit test, then integration test).
- T006 must complete before T007 (the report references the generated backlog ids).
- T007 after the audit verdicts are settled (T001–T006).
- T008 last (tests + help-dump re-verify gate the whole change).

## Acceptance

### Functional Completeness

- [x] A-001 R1: The help-dump verification checklist is executed against the built binary and PASSES (exit 0, empty stderr, valid JSON, `{tool, version, schema_version, root}` with no `captured_at`, filter rules hold, version = built-binary version), with receipts recorded in the report.
- [x] A-002 R2: `README.md` links the command reference as `https://shll.ai/wt/commands/` (no `tools/` segment), matching readme-extraction rule 8.
- [x] A-003 R3: The readme-extraction "Verifying conformance" checklist passes for `README.md` + `docs/site/**` with the R2 URL as the only fixed item.
- [x] A-004 R4: The shared fallback-menu EOF path returns a structured, actionable, flag-naming error (not `reading input: EOF`).
- [x] A-005 R5: `conformance-report.md` dispositions all ten principles individually (PASS / fixed in this change / deferred to [<id>]).
- [x] A-006 R6: `fab/backlog.md` gains three entries (skill adoption, `wt delete` stdout→stderr realignment, `wt delete --dry-run`) with 4-char ids, dated lines, and the existing entry style; each is referenced in the report.
- [x] A-007 R7: `conformance-report.md` exists in the change folder with one section per standard in list order, headed by shll v0.0.23, and one disposition per gap.

### Behavioral Correctness

- [x] A-008 R4: `wt open` (main repo), `wt go` (no name), and `wt delete` (no name) with stdin `</dev/null` exit non-zero (no hang) and emit the actionable menu-refusal wording to stderr.
- [x] A-009 R4: The interactive TTY path, a valid numeric choice piped on stdin, and the existing `--non-interactive` refusals (`wt go`, `wt delete`) are behaviorally unchanged.

### Scenario Coverage

- [x] A-010 R4: A unit test pins the `runFallbackMenu` EOF-error wording and the valid-choice happy path; an integration test pins the three commands' non-TTY refusal on stderr.

### Edge Cases & Error Handling

- [x] A-011 R4: The non-EOF invalid-input retry loop of `runFallbackMenu` (non-numeric / out-of-range on a still-open stdin) is unchanged — only the terminal EOF/read-error return is reworded.

### Code Quality

- [x] A-012 Pattern consistency: The new fallback-menu error uses the existing `WtError`/`ExitWithError` `what/why/fix` helper idiom (Constitution IV; error-wording convention), not an ad-hoc string; the fix lives in `internal/worktree` keeping `cmd/` thin (Constitution V).
- [x] A-013 No unnecessary duplication: The fix is made once at the shared `runFallbackMenu` choke point rather than duplicated across `open`/`go`/`delete` call sites (reuses the shared menu path).
- [x] A-014 Magic strings: The reworded error is a single-sourced message (no scattered literals); tests assert on shape/substring, not brittle byte-equality where avoidable.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. (The reworded EOF return replaces the bare `reading input: EOF` string for the EOF case only; the remaining `return 0, fmt.Errorf("reading input: %w", err)` in `src/internal/worktree/menu.go` still serves non-EOF read errors and is not redundant. The audit's other outputs are report/backlog/doc edits with no code superseded.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Audit set = the four standards `shll standards` enumerated at apply time (principles, help-dump, readme-extraction, skill @ shll v0.0.23); runtime list matched the intake snapshot | Re-ran `shll standards` + `shll standards <name>` and `shll version` at apply; exit 0, no `shll update` needed | S:95 R:90 A:95 D:95 |
| 2 | Certain | help-dump PASSES verbatim (documentation-only verdict, no code fix) | Ran the standard's checklist against the `just build` binary: exit 0, empty stderr, valid JSON, `{tool,version,schema_version,root}`, no `captured_at`, completion/help/help-dump absent, version = built binary; pinned test present | S:95 R:95 A:95 D:95 |
| 3 | Certain | readme-extraction gap = only the command-reference URL (`tools/wt` → `wt`); rest PASSES | Grep checklist run verbatim against README + docs/site: top-order, tail rule, image absoluteness, docs/site closure, reserved names all pass; only rule-8 URL diverges | S:95 R:90 A:95 D:90 |
| 4 | Certain | skill = "deferred, not yet adopted" with a backlog id, not implemented | Intake note + the standard's Adoption section ("No tool ships skill today"; phased per-repo); `wt skill` → unknown command confirmed | S:95 R:90 A:100 D:95 |
| 5 | Confident | The one small-additive principle fix in scope is the non-TTY fallback-menu actionable error (№1/№4); everything else PASSES or defers | Empirically probed non-TTY menu paths — they don't hang but emit unactionable `reading input: EOF`; policy lists "unhelpful/unactionable error" as fixable and error-wording is a single-choke-point change | S:80 R:80 A:80 D:70 |
| 6 | Confident | `wt delete` stdout→stderr realignment (№2) and `wt delete --dry-run` (№5) are deferred, not fixed here | Both exceed the small-additive boundary (command-wide realignment / new preview code path); `wt delete` has no stdout machine contract so nothing breaks meanwhile; the warnings were already realigned separately (260622-log5) | S:75 R:75 A:75 D:65 |
| 7 | Confident | Principle audit is per-command against actual behavior (8 user-facing commands + root), each of the ten principles individually dispositioned in the report | Intake lists the assessment dimensions; sources for create/list/open/go/delete/init/shell-init/update + errors.go/menu.go/apps.go/update.go were read and non-TTY behavior probed empirically | S:80 R:80 A:85 D:75 |

7 assumptions (4 certain, 3 confident, 0 tentative).
