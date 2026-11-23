# Plan: `wt create --pr <n>` — a worktree on a PR's head branch

**Change**: 261123-c3ek-create-pr-head-worktree
**Intake**: `intake.md`

## Requirements

### wt-cli: PR-head resolution (`internal/worktree`)

#### R1: `ResolvePRHead` resolves a PR's head via gh
`src/internal/worktree/pr.go` SHALL expose `ResolvePRHead(number string) (PRHead, error)` that
runs `gh pr view <number> --json headRefName,isCrossRepository` as an argv exec (no shell),
bounded by a 15 s timeout, and returns `PRHead{Branch, CrossRepository}` parsed from stdout.

- **GIVEN** a `gh` on PATH that prints `{"headRefName":"feature/x","isCrossRepository":false}`
- **WHEN** `ResolvePRHead("42")` is called
- **THEN** it returns `PRHead{Branch: "feature/x"}` and nil
- **AND** gh received exactly the argv `pr view 42 --json headRefName,isCrossRepository`

#### R2: `ResolvePRHead` classifies failures
- `gh` not on PATH → the sentinel `ErrGhNotFound`.
- `isCrossRepository: true` → the parsed `PRHead` **and** the sentinel `ErrPRCrossRepository`.
- gh exits non-zero → an error whose text is the first non-empty line of gh's stderr (exec error
  text when stderr is empty).
- timeout → an error naming the 15 s bound.
- malformed JSON or empty `headRefName` → an error.

- **GIVEN** a gh stub that exits 1 with stderr `no pull requests found\nmore detail`
- **WHEN** `ResolvePRHead("99")` is called
- **THEN** the error text is `no pull requests found`

- **GIVEN** PATH without `gh`
- **WHEN** `ResolvePRHead("1")` is called
- **THEN** `errors.Is(err, ErrGhNotFound)`

### wt-cli: `wt create --pr <n>` (`cmd/wt/create.go`)

#### R3: The `--pr` flag and its argument validation
`wt create` SHALL accept a long-only string flag `--pr <n>`. A value that is not a positive
decimal integer (`^[1-9][0-9]*$`) — including an explicitly empty `--pr ""` (the check keys on the flag being set, not on its value) — SHALL exit `ExitInvalidArgs` (2) with
`Invalid --pr value: <v>` / `--pr takes a pull request number` / `Example: wt create --pr 42`,
before any git work.

- **GIVEN** `wt create --pr abc` (or `--pr ""`)
- **WHEN** it runs
- **THEN** exit 2, nothing created

#### R4: `--pr` is exclusive with positional, `--base`, `--checkout`, `--reuse`
Each combination SHALL exit `ExitInvalidArgs` (2) before any git work with the intake §2 copy.
The checks run after the existing `--reuse requires --name` check.

- **GIVEN** `wt create --pr 7 --base main`
- **WHEN** it runs
- **THEN** exit 2 with `--base cannot be combined with --pr`, nothing created

#### R5: `--pr` runs the `--checkout <head>` path
After `ValidateGitRepo`/`GetRepoContext` and before the dirty-state menu and any mutation,
`create` SHALL resolve the head via `ResolvePRHead` and then behave exactly as
`--checkout <head>` (validation, suggested name `DeriveWorktreeName(head)`, local as-is /
remote-only fetch-then-checkout, rollback, init-confirm gate, init, Open, stdout path line).

- **GIVEN** a same-repo PR whose head branch exists only on origin
- **WHEN** `wt create --pr 7 --non-interactive` runs with a gh stub reporting it
- **THEN** a worktree is created on that branch and stdout's last line is its path

#### R6: Lookup-failure mapping (no new exit codes)
- `ErrGhNotFound` → `ExitGeneralError` (1), `GitHub CLI (gh) not found` with an install /
  `--checkout` fix.
- `ErrPRCrossRepository` → `ExitInvalidArgs` (2), `PR #<n> is from a fork (head branch '<head>')`,
  fix `Add the fork as a remote and use --checkout`.
- Any other lookup error → `ExitGeneralError` (1), `Could not resolve PR #<n>`, why = the error
  text (gh's first stderr line).
- `ErrBranchNotFound` from the checkout under `--pr` → `ExitInvalidArgs` (2),
  `PR #<n> head branch '<head>' not found`.
In every case no worktree or branch is left behind.

- **GIVEN** a gh stub reporting `isCrossRepository: true`
- **WHEN** `wt create --pr 7 --non-interactive` runs
- **THEN** exit 2, the fork copy is on stderr, no worktree directory exists

#### R7: The `From:` summary line under `--pr`
The Git-phase summary's `From:` line SHALL read `PR #<n> (existing branch '<head>')`.

- **GIVEN** a successful `wt create --pr 7`
- **THEN** stderr contains `From: PR #7 (existing branch '<head>')`

### wt-cli: Docs

#### R8: CLI surface docs updated in the same change
`docs/specs/cli-surface.md` § `wt create` SHALL document `--pr` (flag row, prose, exit codes);
`docs/site/skill.md` SHALL mention `--pr` and its fork/gh gotcha, synced to
`src/cmd/wt/skill.md` via `just sync-skill` within the 150-line budget; the `Long` help text
SHALL describe `--pr` (flows into `wt help-dump`).

- **GIVEN** the change is applied
- **WHEN** `go test ./...` runs
- **THEN** the skill drift-guard and line-budget tests pass and `wt help-dump` lists `--pr`

### Non-Goals

- `--temp` exclusion — owned by W5 (`--temp` is not on main).
- `--fetch` / `--json` interplay — W1.
- Fork support, PR URLs or `#n` forms.

### Design Decisions

#### `--pr` is a resolver in front of `--checkout`
**Decision**: `--pr` only turns `<n>` into a branch name (via `gh`) and then sets `checkout`, so
every downstream phase is the existing `--checkout` path.
**Why**: the plan's `--pr` decision — reuse the tested remote-only checkout path; one mode's
behaviour, one code path.
**Rejected**: fetching `refs/pull/<n>/head` (detached commit, not the PR's branch — W5's job);
calling the GitHub API directly (auth + dependency; `gh` is the toolkit's GitHub seam).
*Introduced by*: 261123-c3ek-create-pr-head-worktree

#### Forks are refused, not guessed
**Decision**: `isCrossRepository: true` exits 2 pointing at "add the fork as a remote and use
--checkout".
**Why**: a fork's head is not on `origin`; checking out a same-named `origin` branch would be the
wrong code. `wt` does not manage remotes (plan scope).
**Rejected**: auto-adding the fork as a remote (hidden state, Constitution I).
*Introduced by*: 261123-c3ek-create-pr-head-worktree

## Tasks

### Phase 2: Core Implementation

- [x] T001 Create `src/internal/worktree/pr.go` (`PRHead`, `ErrGhNotFound`, `ErrPRCrossRepository`, `prLookupTimeout = 15 * time.Second`, `ResolvePRHead`) and `src/internal/worktree/pr_test.go` (stub-gh on PATH: success + argv check, cross-repo, gh failure first-line, malformed JSON / empty head, gh missing) <!-- R1 R2 -->
- [x] T002 Wire `--pr` in `src/cmd/wt/create.go`: flag registration, digits validation, the four mutex checks, `ResolvePRHead` call + error mapping before the dirty-state menu, `checkout = head`, `From:` label, `ErrBranchNotFound` PR copy, `Long` help paragraph <!-- R3 R4 R5 R6 R7 --> <!-- rework: gate --pr validation on Flags().Changed so an explicit empty value exits 2 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Tests in `src/cmd/wt/create_test.go` (bad value, four conflicts, fork, gh missing, gh failure, head not found, From line) and `src/cmd/wt/integration_test.go` (remote-only head + local head via stub gh; stdout path line), with a `writeGhStub` helper in `src/cmd/wt/testutil_test.go` <!-- R3 R4 R5 R6 R7 --> <!-- rework: cover --pr= (empty) in TestCreate_PRInvalidValue -->

### Phase 4: Polish

- [x] T004 Docs: `docs/specs/cli-surface.md` § `wt create`, `docs/site/skill.md` → `just sync-skill`; run `go test ./...` and `go vet ./...` from `src/` <!-- R8 --> <!-- rework: document that gh's resolved repo must be origin (fork-clone limitation) -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `ResolvePRHead` execs `gh pr view <n> --json headRefName,isCrossRepository` via argv with a 15 s bound and returns the parsed head
- [x] A-002 R3: `--pr` is a long-only flag; non-numeric / zero / explicitly empty values exit 2 before git work (validation keyed on `Flags().Changed("pr")`; `--pr ""` verified exit 2 with nothing created)
- [x] A-003 R5: `wt create --pr <n>` lands on the head branch for both local and remote-only heads, with the `--checkout` path's naming, rollback, init and stdout contract

### Behavioral Correctness

- [x] A-004 R7: the `From:` line reads `PR #<n> (existing branch '<head>')`
- [x] A-005 R4: each of positional / `--base` / `--checkout` / `--reuse` combined with `--pr` exits 2 with nothing created; `--reuse` alone and `--checkout` alone are unchanged

### Scenario Coverage

- [x] A-006 R2: unit tests cover success+argv, cross-repo, gh failure first line, malformed output, gh missing
- [x] A-007 R5: an integration test drives the built binary with a stub gh against a repo with a bare origin

### Edge Cases & Error Handling

- [x] A-008 R6: fork → 2, gh missing → 1, gh failing → 1 (gh's first stderr line shown), head not found → 2; no worktree/branch left behind in any case; no new exit codes
- [x] A-009 R8: cli-surface, skill bundle (synced, within budget) and `Long` help document `--pr`; `wt help-dump` lists it

### Code Quality

- [x] A-010 Pattern consistency: new code follows the surrounding sentinel-error / `ExitWithError` / bounded-exec idioms
- [x] A-011 No unnecessary duplication: `--pr` reuses the `--checkout` path and existing helpers rather than a parallel implementation
- [x] A-012 Internal boundary: the `gh` exec and JSON parsing live in `internal/worktree`; `cmd/` only validates flags and maps errors (Constitution V)
- [x] A-013 Test isolation: tests use a stub gh on a temp PATH and never call a real `gh` or the network

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without making existing code redundant

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Timeout via `exec.CommandContext` + `context.WithTimeout`; default kill on expiry (gh is a read-only lookup, no transaction to unwind) | Brief fixes 15 s; update.go's SIGTERM-grace idiom exists for brew's transactions, not needed here | S:60 R:90 A:75 D:70 |
| 2 | Confident | The timeout path is not unit-tested (a sleeping stub would cost ≥15 s) | Keeps the suite fast; the bound is a one-line constant | S:55 R:90 A:70 D:70 |
| 3 | Certain | `ErrBranchNotFound` under `--pr` keeps exit 2 with PR-specific copy | Intake assumption 9 | S:80 R:85 A:85 D:80 |

| 4 | Confident | The gh-default-repo vs origin mismatch in fork clones is documented (cli-surface + skill gotcha) rather than detected; the argv stays the Brief's `headRefName,isCrossRepository` | Review should-fix offered either; detection needs origin-URL parsing, and the Brief fixes the gh argv | S:55 R:85 A:65 D:55 |
| 5 | Confident | `cmd.WaitDelay` (2 s) added so a gh descendant holding the pipes cannot stretch the 15 s bound | Review nice-to-have; mirrors internal/update | S:60 R:90 A:80 D:75 |

5 assumptions (1 certain, 4 confident, 0 tentative).
