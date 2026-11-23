# Plan: `--reuse` + `--base` is a usage error

**Change**: 261123-udcv-reuse-base-usage-error
**Intake**: `intake.md`

## Requirements

### wt create: argument conflicts

#### R1: `--reuse` combined with `--base` is rejected before any mutation
`wt create` SHALL exit `ExitInvalidArgs` (2) when both `--reuse` and a non-empty `--base` are
given, printing via `wt.ExitWithError`:
`--reuse and --base cannot be combined` /
`--reuse keeps the existing worktree's branch, so a start-point has nothing to apply to` /
`Drop --base, or drop --reuse to create a new worktree`.
The check MUST run in `src/cmd/wt/create.go` alongside the `--checkout` + `--base` conflict —
before `ValidateGitRepo`, rollback setup, branch-name validation, `--base` ref validation, and any
git work — and MUST fire whether or not the `--name` worktree already exists.

- **GIVEN** no worktree named `X`
- **WHEN** the user runs `wt create --non-interactive --reuse --name X --base main`
- **THEN** the command exits 2 with `--reuse and --base cannot be combined` on stderr
- **AND** no `<repo>.worktrees/X` directory and no branch `X` exist afterwards

- **GIVEN** an existing worktree `X`
- **WHEN** the user runs `wt create --non-interactive --reuse --name X --base main`
- **THEN** the command exits 2 with the conflict copy, nothing on stdout, and `X`'s HEAD unchanged

- **GIVEN** an existing worktree `X`
- **WHEN** the user runs `wt create --non-interactive --reuse --name X --base nonexistent-ref`
- **THEN** the command exits 2 with the conflict copy, not `Invalid --base ref`

#### R2: Existing `--reuse` behavior is otherwise unchanged
`--reuse requires --name` SHALL remain the first `--reuse` check (so `--reuse --base R` without a
name reports `--reuse requires --name`, exit 2). `--reuse --name X` without `--base` SHALL keep
reusing an existing worktree (exit 0, path on stdout) or creating a fresh one on no collision.

- **GIVEN** `wt create --non-interactive --reuse --base main` (no `--name`)
- **WHEN** it runs
- **THEN** it exits 2 with `--reuse requires --name`

- **GIVEN** an existing worktree `X`
- **WHEN** the user runs `wt create --non-interactive --reuse --name X`
- **THEN** it exits 0 and prints `X`'s path

#### R3: `--base` ref validation no longer special-cases `--reuse`
The `--base` validation guard SHALL be `if base != ""` (the `!reuse` arm is unreachable after R1),
with its comment rewritten to say both `--reuse` and `--checkout` combinations are rejected earlier.
Observable behavior of `--base` validation for new branches is unchanged.

- **GIVEN** `wt create newbranch --base bad-ref`
- **WHEN** it runs
- **THEN** it still exits 2 with `Invalid --base ref: bad-ref`

#### R4: Help text names the conflict
The `--base` flag usage SHALL state it cannot be combined with `--checkout` or `--reuse`, and the
`create` `Long` text SHALL add that `--reuse` cannot be combined with `--base`. `wt help-dump`
reflects these via the cobra tree (no hand-maintained fixture).

- **GIVEN** the built binary
- **WHEN** the user runs `wt create --help`
- **THEN** the `--base` line and the long description mention the `--reuse` conflict

#### R5: CLI docs match the new surface
`docs/specs/cli-surface.md` § `wt create` SHALL document the conflict on the `--reuse` and `--base`
rows (dropping "whenever set and `--reuse` is not"). The canonical skill bundle
`docs/site/skill.md` SHALL replace "`--reuse` takes precedence over `--base`" with the conflict
gotcha, and `src/cmd/wt/skill.md` SHALL be re-synced (`just sync-skill`) so the drift guard holds. `README.md` and `docs/site/workflows.md` (gotcha line and `--base` behavior table) SHALL likewise describe the conflict.

- **GIVEN** the repo after the change
- **WHEN** `go test ./...` runs `skill_test.go`'s drift guard
- **THEN** it passes and neither doc claims `--reuse` takes precedence over `--base`

### Non-Goals

- `--reuse` + `--fetch` conflict — `--fetch` does not exist on this branch (verified: no `fetch`
  flag in `src/cmd/wt/create.go`); plan step W1 introduces it, so whichever of W1/W3 merges second
  wires that conflict (intake Assumption #4).
- Any `src/internal/worktree/` change — the check is pure argument parsing (Constitution V; memory
  DD "Flag-mutex validation in `cmd/`").
- Memory edits — `docs/memory/wt-cli/create-branch-semantics.md` is updated at hydrate.

### Design Decisions

#### `--reuse` + `--base` is a hard conflict, not silent precedence
**Decision**: Reject the combination with `ExitInvalidArgs` (2) before any git work, unconditionally
(not only on a name collision), mirroring the `--checkout` + `--base` conflict.
**Why**: Silent precedence let a caller believe it got a worktree cut from `<ref>` when it got an
existing worktree on whatever base it had; `--base` now only ever pairs with creating a new branch.
Plan `fab/plans/sahil/26-11-23-worktree-base-receipts.md` step W3.
**Rejected**: Keep `--reuse` precedence (the documented gotcha); warn-and-ignore (scripts proceed on
a wrong assumption); reject only on collision (the outcome would depend on filesystem state the
caller cannot see from its own argv).
*Introduced by*: 261123-udcv-reuse-base-usage-error

## Tasks

### Phase 1: Core Implementation

- [x] T001 In `src/cmd/wt/create.go`: add the `reuse && base != ""` conflict check right after the `--checkout` + `--base` check (after `--reuse requires --name`), simplify the `--base` validation guard to `base != ""` with a rewritten comment, update the `--base` flag usage and the `create` `Long` text <!-- R1 R2 R3 R4 -->

### Phase 2: Tests

- [x] T002 In `src/cmd/wt/create_test.go`: replace `TestCreate_BaseInvalidRefWithReuse` and `TestCreate_BaseWithReuse` (which assert the old precedence) with conflict tests — existing worktree (exit 2, no stdout, HEAD unchanged), no worktree (exit 2, no dir, no branch), invalid ref (conflict copy, not `Invalid --base ref`), and `--reuse --base` without `--name` (`--reuse requires --name`); keep the existing `--reuse`-alone tests; run `go test ./cmd/...` from `src/` <!-- R1 R2 R3 -->
- [x] T003 In `src/cmd/wt/integration_test.go`: add `TestIntegration_ReuseWithBaseConflict` — create `X`, then `--reuse --name X --base main` exits 2 with nothing created/changed, and `--reuse --name X` alone still reuses (exit 0, same path); run it <!-- R1 R2 -->

### Phase 3: Docs

- [x] T004 [P] In `docs/specs/cli-surface.md` § `wt create`: `--reuse` row gains "Cannot be combined with `--base`."; `--base` row reads "Validated via `git rev-parse --verify`. Cannot be combined with `--checkout` or `--reuse`." <!-- R5 -->
- [x] T005 [P] In `docs/site/skill.md`: replace "`--reuse` takes precedence over `--base`" with the conflict gotcha (exit 2 — to cut from a ref, create a new worktree); run `just sync-skill`; also fix the same stale precedence claim (gotcha line + `--base` behavior table row) in `README.md` and `docs/site/workflows.md`; run the full `go test ./...` from `src/` (drift guard, help-dump) <!-- R5 R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt create --reuse --name X --base <ref>` exits 2 with the exact Error/Why/Fix copy, before any git work
- [x] A-002 R2: `--reuse requires --name` still fires first; `--reuse --name X` alone still reuses
- [x] A-003 R3: the `--base` validation guard is `base != ""` and its comment no longer describes `--reuse` precedence
- [x] A-004 R4: `--base` usage and `create` Long text name the `--reuse` conflict
- [x] A-005 R5: `cli-surface.md`, `docs/site/skill.md`, and `src/cmd/wt/skill.md` describe the conflict; no doc claims `--reuse` takes precedence over `--base`

### Behavioral Correctness

- [x] A-006 R1: a non-colliding `--reuse --name X --base R` now exits 2 instead of creating `X` off `R`, and the old precedence tests are replaced, not deleted without coverage

### Scenario Coverage

- [x] A-007 R1: tests cover existing worktree, no worktree, and invalid-ref conflict cases, each asserting exit 2 and no state change
- [x] A-008 R1: an integration case in `src/cmd/wt/integration_test.go` exercises the conflict against a real temp repo

### Edge Cases & Error Handling

- [x] A-009 R1: on conflict, nothing is written to stdout and no worktree directory or branch is left behind
- [x] A-010 R2: `--reuse --base R` without `--name` exits 2 with `--reuse requires --name`

### Code Quality

- [x] A-011 Pattern consistency: the new check uses `wt.ExitWithError(wt.ExitInvalidArgs, …)` and sits with the sibling conflict checks, matching their copy shape
- [x] A-012 No unnecessary duplication: tests reuse `runWt` / `assertExitCode` / `assertWorktreeNotExists` / `assertBranchNotExists` helpers
- [x] A-013 Readability: no magic strings beyond the user-facing copy; comments updated where the old precedence was described
- [x] A-014: No new exit codes; no `src/internal/worktree/` change

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without making existing code redundant (the one thing it made dead, the `!reuse` arm of the `--base` validation guard at `src/cmd/wt/create.go:160`, was already removed in the diff). Stale prose remains only in `docs/memory/wt-cli/create-branch-semantics.md:88` and `:163`, which hydrate owns.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--fetch` half is a Non-Goal on this branch | Verified no `--fetch` flag exists in `create.go`; intake Assumption #4 | S:80 R:90 A:95 D:85 |
| 2 | Certain | Replace the two old precedence tests rather than keep them | They assert behavior the plan's Decisions table removes (Test Integrity: tests follow spec) | S:85 R:95 A:95 D:90 |
| 3 | Confident | Test the integration case in `integration_test.go` in addition to `create_test.go` | Plan Rule 2 names `integration_test.go` explicitly even though `create_test.go` is also binary-driven | S:75 R:95 A:80 D:70 |
| 4 | Certain | Extend R5 to `README.md` and `docs/site/workflows.md` | Found during apply: both repeated "`--reuse` takes precedence over `--base`"; leaving them would contradict the binary | S:80 R:95 A:95 D:90 |

4 assumptions (3 certain, 1 confident, 0 tentative).
