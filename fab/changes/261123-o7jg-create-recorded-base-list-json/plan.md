# Plan: Recorded base — `branch.<name>.wt-base[-sha]` on create; `base`/`base_sha` in `wt list --json`

**Change**: 261123-o7jg-create-recorded-base-list-json
**Intake**: `intake.md`

## Requirements

### Create: Recorded base

#### R1: Record the base for wt-created branches
When `wt create` creates a new branch (positional or bare exploratory, with or without `--base`), it SHALL write `git config branch.<branch>.wt-base <label>` and `git config branch.<branch>.wt-base-sha <sha>` in the repository's own config (never `--global`), where `<label>` is the `--base` ref exactly as typed, or — with no `--base` — the `wt.DescribeHead()` HEAD label (the same value as the receipt's `base_ref` and the `From:` line's ref), and `<sha>` is the full 40-hex pinned start-point (the receipt's `base_sha`). The writes SHALL happen after a successful `git worktree add -b` (and after the upstream step), inside the create functions in `src/internal/worktree/`.

- **GIVEN** a repo with a remote-tracking ref `origin/main`
- **WHEN** `wt create --name w2 --base origin/main --non-interactive --no-init` runs
- **THEN** `git config branch.w2.wt-base` prints `origin/main`
- **AND** `git config branch.w2.wt-base-sha` prints the full SHA `origin/main` resolved to, equal to the `--json` receipt's `base_sha`

- **GIVEN** the invoking worktree is on branch `main`
- **WHEN** a bare `wt create --non-interactive --no-init` runs
- **THEN** `branch.<new>.wt-base` is `main` and `wt-base-sha` is HEAD's full SHA

#### R2: Never record for branches wt did not create
`--checkout` (local or remote-only) and `--reuse` SHALL NOT write any `wt-base*` key. The internal create functions SHALL write nothing when called with an empty start-point.

- **GIVEN** an existing branch `feat`
- **WHEN** `wt create --checkout feat` runs
- **THEN** `git config --get branch.feat.wt-base` exits 1 (unset), as does `wt-base-sha`

- **GIVEN** an existing worktree named `w`
- **WHEN** `wt create --name w --reuse` runs
- **THEN** no `wt-base*` key is added for its branch

#### R3: A failed record rolls back the create
A failed config write SHALL roll back the just-created worktree and branch (via the existing rollback: `worktree remove --force` then `branch -D`), and `wt create` SHALL exit `ExitGitError` (3) with `Error: Failed to record base for branch '<branch>'`, `Why:` the first line of git's output, `Fix: Check that .git/config is writable (no stale config.lock), then retry`. Under `--json` the failure document is `{"ok":false,"error":{"code":3,"message":"Failed to record base for branch '<branch>'"}}`. Because `git branch -D` deletes the branch's config section, no `wt-base*` key survives a rollback. No new exit code.

- **GIVEN** a repo with a stale `.git/config.lock` and a local (non-remote-tracking) base, so no upstream is set
- **WHEN** `wt create newb --non-interactive --no-init` runs
- **THEN** exit code is 3, stderr names `Failed to record base for branch 'newb'`
- **AND** no worktree directory, no branch `newb`, and no `branch.newb.*` keys remain (after removing the lock)

### List: base in JSON records

#### R4: `wt list --json` records carry `base` and `base_sha`
Every `wt list --json` record (with or without `--status`) SHALL carry `base` and `base_sha` keys, always present, `null` when the branch has no recorded base (pre-W2 worktrees, the main worktree on an unrecorded branch, `--checkout` worktrees, detached worktrees). Values come from `branch.<branch>.wt-base` / `wt-base-sha`.

- **GIVEN** a worktree created by `wt create --name w2 --base origin/main`
- **WHEN** `wt list --json` runs
- **THEN** the `w2` record has `"base":"origin/main"` and `"base_sha":"<sha>"`
- **AND** the main record has `"base":null,"base_sha":null` (keys present)

#### R5: Constant-cost read; human output unchanged
Recorded bases SHALL be read with a single `git config --get-regexp` call for all worktrees, only on the `--json` path, parsed by stripping the `branch.` prefix and the `.wt-base`/`.wt-base-sha` suffix (branch names may contain dots and slashes). Any read failure (including exit 1 = no matches) SHALL degrade to all-null, never an error. Human `wt list`, `wt list --status`, and `--path` SHALL NOT read bases and SHALL be byte-identical to before (no new column).

- **GIVEN** branches `feat/v1.2` and `a.b` both with recorded bases and an unrelated `branch.x.remote` key
- **WHEN** the reader parses the config
- **THEN** it returns entries for `feat/v1.2` and `a.b` only, with both ref and sha

### Non-Goals

- A `BASE` column in `wt list --status` — JSON only in v1 (plan W2 open decision).
- Changes to the `wt create --json` receipt, the `From:` line, or any success copy.
- `--temp` (W5) — not present yet; it will simply not call the new-branch create functions.
- Memory files under `docs/memory/` — updated at hydrate.

### Design Decisions

#### Recorded base lives in branch git config
**Decision**: `branch.<b>.wt-base` / `branch.<b>.wt-base-sha`, written by `internal/worktree` inside the create functions' rollback scope.
**Why**: Constitution I (no hidden state); travels with the branch through `git branch -m`, removed by `git branch -D`; readable by any tool via `git config`.
**Rejected**: a wt state file (Constitution I violation; plan Non-goal); git notes/refs (not keyed by branch).
*Introduced by*: 261123-o7jg-create-recorded-base-list-json

#### One extra constant subprocess on `wt list --json`
**Decision**: `wt list --json` runs `git worktree list --porcelain` plus one `git config --get-regexp`; human `wt list` stays at one subprocess.
**Why**: the one-subprocess rule exists to forbid per-worktree O(N) git calls; a single constant read preserves that guarantee while serving the base keys.
**Rejected**: per-worktree `git config --get` (O(N)); parsing `.git/config` directly (misses includes, worktree config, and git's own quoting rules).
*Introduced by*: 261123-o7jg-create-recorded-base-list-json

## Tasks

### Phase 1: Core Implementation

- [x] T001 Add `src/internal/worktree/base.go`: key constants (`wt-base`, `wt-base-sha` suffixes), `RecordBaseError{Branch, Output}` (with `Error()`), `RecordBase(branch, ref, sha string) error` (two argv `git config branch.<b>.<key> <val>` calls), and `RecordedBases() map[string]RecordedBase` (`RecordedBase{Ref, SHA string}`) reading one `git config -z --get-regexp '^branch\..*\.wt-base(-sha)?$'`, failures → empty map <!-- R1 --> <!-- R5 -->
- [x] T002 In `src/internal/worktree/crud.go`, extend `CreateNewBranchWorktree` and `CreateExploratoryWorktree` with the base ref label and add a `recordBase(branch, ref, sha, rb)` step after `trackUpstream` that skips when `sha == ""` and on failure runs `rb.Execute(); rb.Disarm()` and returns the `*RecordBaseError`; update all call sites (create.go, internal tests) <!-- R1 --> <!-- R2 --> <!-- R3 -->
- [x] T003 In `src/cmd/wt/create.go`, pass `baseRefLabel` into both new-branch create calls and add a `failIfRecordBase` mapping (exit 3, Error/Why/Fix copy per R3) next to `failIfSetUpstream` in both new-branch arms <!-- R1 --> <!-- R3 -->
- [x] T004 In `src/cmd/wt/list.go`, add `Base *string \`json:"base"\`` and `BaseSHA *string \`json:"base_sha"\`` to `listEntry`; on the `--json` path only, call `wt.RecordedBases()` once and join by `Branch` (skip `(detached)`); human/`--path` paths untouched <!-- R4 --> <!-- R5 -->

### Phase 2: Tests

- [x] T005 [P] Unit tests in `src/internal/worktree/base_test.go` and `crud_test.go`: new-branch and exploratory creates record both keys; checkout (local) records nothing; empty start-point records nothing; config.lock-forced record failure rolls back worktree, branch and keys and returns `*RecordBaseError`; `RecordedBases` parses dotted/slashed branch names, ignores unrelated keys, returns empty map when none <!-- R1 --> <!-- R2 --> <!-- R3 --> <!-- R5 -->
- [x] T006 [P] cmd tests in `src/cmd/wt/create_test.go` and `list_test.go`: positional+`--base` records ref-as-typed and pinned SHA; bare create records HEAD label; `--checkout` and `--reuse` record nothing; config.lock with local base → exit 3, `--json` failure document code 3, nothing left behind; `wt list --json` carries `base`/`base_sha` for a created worktree and present-null for main; human `wt list` has no base output <!-- R1 --> <!-- R2 --> <!-- R3 --> <!-- R4 --> <!-- R5 -->
- [x] T007 Integration case in `src/cmd/wt/integration_test.go` (bare origin + clone): `wt create --name w2 --base origin/main --json` → config `wt-base`=`origin/main`, `wt-base-sha` = receipt `base_sha`; `wt list --json` reflects it; `--checkout` of an existing branch leaves no keys <!-- R1 --> <!-- R2 --> <!-- R4 -->

### Phase 3: Docs

- [x] T008 Update `docs/specs/cli-surface.md` (§ `wt create`: recorded base, never for checkout/reuse, exit 3 on write failure; § `wt list`: `--json` `base`/`base_sha`), `docs/specs/worktree-layout.md` § Branch ↔ worktree relationship (wt-created branches carry the recorded base), and `docs/site/skill.md` Machine-surface bullet, then run `just sync-skill`; confirm `help_dump_test.go` passes unchanged <!-- R1 --> <!-- R4 -->

## Execution Order

- T001 blocks T002; T002 blocks T003; T001 blocks T004
- T005–T007 after T001–T004; T008 independent of tests

## Acceptance

### Functional Completeness

- [ ] A-001 R1: Positional/bare/`--base` creates write `branch.<b>.wt-base` (ref as typed or HEAD label) and `wt-base-sha` (full pinned SHA) to repo config
- [ ] A-002 R2: `--checkout` and `--reuse` write no `wt-base*` keys; empty start-point writes nothing
- [ ] A-003 R3: A failed config write rolls back worktree and branch, exits 3 with the specified copy and the `--json` failure document
- [ ] A-004 R4: Every `wt list --json` record has `base` and `base_sha` keys (null when unset), with and without `--status`
- [ ] A-005 R5: Bases are read by exactly one `git config --get-regexp` call on the `--json` path only

### Behavioral Correctness

- [ ] A-006 R5: Human `wt list` / `wt list --status` / `--path` output is unchanged (no base column, no extra subprocess)
- [ ] A-007 R1: Recorded `wt-base`/`wt-base-sha` equal the `--json` receipt's `base_ref`/`base_sha` for the same create

### Scenario Coverage

- [ ] A-008 R1: Tests assert config present after positional, bare and `--base` creates (unit, cmd, integration)
- [ ] A-009 R2: Tests assert config absent after `--checkout` and `--reuse`
- [ ] A-010 R4: Tests assert `wt list --json` reflects the recorded base and is null for main

### Edge Cases & Error Handling

- [ ] A-011 R3: Test with a stale `config.lock` and a local base asserts exit 3, no worktree dir, no branch, no `branch.<b>.*` keys
- [ ] A-012 R5: Reader handles branch names with dots/slashes, ignores unrelated `branch.*` keys, and returns empty on no matches/error

### Code Quality

- [ ] A-013 Pattern consistency: `recordBase`/`RecordBaseError`/`failIfRecordBase` mirror the existing `trackUpstream`/`SetUpstreamError`/`failIfSetUpstream` shape
- [ ] A-014 No unnecessary duplication: rollback reuses the existing `Rollback`; no hand-rolled `config --unset` cleanup
- [ ] A-015 Readability: no god functions; key names are named constants, not magic strings
- [ ] A-016 Constitution V: all git operations (writes and the read) live in `src/internal/worktree/`; `cmd/` only wires and maps errors
- [ ] A-017 Docs: `cli-surface.md`, `worktree-layout.md`, `docs/site/skill.md` and synced `src/cmd/wt/skill.md` describe the recorded base and list keys; help-dump tests pass

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | New code goes in a new `src/internal/worktree/base.go` rather than growing `git.go` | Keeps the recorded-base concept (writer, reader, error, constants) in one file; `git.go` is already large | S:60 R:95 A:80 D:70 |
| 2 | Confident | The reader uses `git config -z --get-regexp` with NUL-delimited records | Robust against unusual values; one call for all keys | S:60 R:95 A:80 D:75 |
| 3 | Confident | Failure injection for record-base tests reuses the stale `.git/config.lock` technique from W1's set-upstream rollback test, with a local base so no upstream is set first | `git worktree add -b` from a SHA does not write config, so the record write is the first config write to fail | S:70 R:95 A:80 D:75 |
| 4 | Confident | `--reuse` test asserts no keys rather than adding new reuse plumbing | `--reuse` never reaches a create function; asserting absence is sufficient | S:70 R:95 A:85 D:80 |

4 assumptions (0 certain, 4 confident, 0 tentative).
