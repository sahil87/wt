# Plan: wt delete --dry-run and destructive-path audit

**Change**: 260717-p5m9-delete-dry-run
**Intake**: `intake.md`

## Requirements

### wt delete: The `--dry-run` flag

#### R1: `--dry-run` is a long-only boolean on `wt delete`
`wt delete` SHALL register a `--dry-run` boolean flag (no short flag) that previews what a real invocation would do without performing any mutation and without prompting for confirmation.

- **GIVEN** the `wt delete` command
- **WHEN** a user runs `wt delete <target> --dry-run`
- **THEN** the command prints a preview of what would be deleted and exits without removing worktrees, deleting branches, or stashing/discarding changes
- **AND** `--dry-run` has no single-letter short (constitution II; precedent `--non-interactive`)

#### R2: `--dry-run` applies uniformly to all six target-resolution paths
The `--dry-run` flag SHALL be honored across every target-resolution path in the existing resolution order: `--stale`, `--all`, positional names, deprecated `--worktree-name`, current-worktree, and the interactive menu.

- **GIVEN** any of the six target-resolution forms
- **WHEN** `--dry-run` is passed
- **THEN** the resolved target set is computed live and previewed, with no path silently mutating

### wt delete: Live-path-sharing preview

#### R3: All decision logic runs live; only leaf mutations are suppressed
Under `--dry-run`, every selection and decision path SHALL execute live — target resolution (including `--stale` idle computation via `IsIdle`/`RecencyOf`), the `--branch` tri-state auto rule (`branch == wtName`), `BranchExistsRemotely()` remote checks, `HasUncommittedChanges()`/`HasUntrackedFiles()`/`HasUnpushedCommits()` hazard detection, and the orphan `wt/<name>` check. Only the leaf mutations SHALL be suppressed, gated at the mutation seam so live and preview cannot drift. No parallel preview derivation is permitted.

- **GIVEN** a worktree whose branch matches its name and exists on origin, with uncommitted changes
- **WHEN** `wt delete <name> --dry-run` runs
- **THEN** the branch-match decision, remote-existence check, and hazard detection all run against real git state, and each suppressed mutation (worktree removal, local branch delete, remote branch delete, discard) produces exactly one `Would …` preview line

#### R4: The mutation leaves gated under dry-run
The following mutation leaves SHALL be suppressed under `--dry-run` and replaced by a preview line each: worktree removal (`RemoveWorktree`), local branch delete (`DeleteLocalBranch`), remote branch delete (`DeleteRemoteBranch`), orphan `wt/<name>` branch cleanup, and stash-or-discard of uncommitted changes (`StashCreate` / `handleStashInDir`). The `os.Chdir` to the main repo (a prerequisite of live removal) SHALL be skipped under dry-run so the invocation has no side effects.

- **GIVEN** dry-run over any target
- **WHEN** the flow reaches a mutation leaf
- **THEN** the leaf does not execute and a corresponding `Would …` line is printed instead

### wt delete: Consent and prompt interplay

#### R5: Dry-run skips all confirmation prompts
`--dry-run` SHALL skip every confirmation prompt ("Delete this worktree?", "Delete these N worktrees?", "Delete ALL …?").

- **GIVEN** dry-run over a resolved target
- **WHEN** the flow reaches a delete-confirmation prompt
- **THEN** no prompt is shown and the preview proceeds

#### R6: Hazard prompts become preview report lines
Where the live interactive flow prompts on uncommitted changes or unpushed commits, `--dry-run` SHALL instead report what the equivalent consented run would do, using the same live detection functions. With `--stash` the uncommitted-changes report is `Would stash uncommitted changes`; without it the report is `Would discard uncommitted changes (use --stash to preserve them)`. Unpushed commits are reported as `Would lose N unpushed commit(s) on branch <b>`.

- **GIVEN** a dirty worktree (uncommitted changes) under dry-run without `--stash`
- **WHEN** the delete flow runs
- **THEN** `Would discard uncommitted changes (use --stash to preserve them)` is printed and nothing is discarded
- **AND** with `--stash`, `Would stash uncommitted changes` is printed and nothing is stashed
- **GIVEN** a branch with unpushed commits under dry-run
- **WHEN** the delete flow runs
- **THEN** `Would lose N unpushed commit(s) on branch <b>` is printed

#### R7: The target-selection menu still shows; selection is not consent
The interactive target-selection menu SHALL still show under `--dry-run` when invoked with no target on a TTY; the selected target routes into the preview. Non-TTY / `--non-interactive` with no target SHALL keep today's `ExitInvalidArgs` refusal ("No worktree specified").

- **GIVEN** `wt delete --dry-run` from the main repo on a TTY with no target
- **WHEN** the command runs
- **THEN** the selection menu shows unchanged; the picked worktree is previewed
- **GIVEN** `wt delete --dry-run --non-interactive` with no target
- **WHEN** the command runs
- **THEN** it exits `ExitInvalidArgs` with "No worktree specified" (unchanged)

### wt delete: Preview output contract

#### R8: Preview lines go to stdout with a `Dry run` header
Preview `Would …` lines SHALL print to stdout (the machine result the caller asked for — principle №2 / repo stdout=machine convention). A `Dry run — no changes will be made.` header SHALL precede the per-mutation lines. Multi-target forms keep their per-worktree block structure with the same `Would …` lines per block. The `[ohwb]` realignment of existing live-path chatter is out of scope.

- **GIVEN** a dry-run over a single dirty worktree whose branch matches and exists on origin
- **WHEN** the command runs
- **THEN** stdout contains `Dry run — no changes will be made.` followed by `Would discard uncommitted changes …`, `Would remove worktree: <name>`, `Would delete branch: <name> (local)`, `Would delete branch: <name> (remote)`
- **GIVEN** `--all`/`--stale`/positional multi-target dry-run
- **WHEN** the command runs
- **THEN** each worktree gets its own preview block with its `Would …` lines
- **AND** `--stale` with zero matches keeps its live `No idle worktrees (threshold: Nd).` empty-state

#### R9: Exit codes unchanged; dry-run fails on exactly the inputs the live run fails on
A successful preview SHALL exit `ExitSuccess` (0). Argument validation (`--stale` mutexes, unknown worktree names, positional×`--worktree-name` mix, invalid `--stale` threshold, outside-git-repo) SHALL keep its existing `ExitInvalidArgs`/`ExitGitError`/`ExitGeneralError` behavior — dry-run fails on exactly the inputs the live run fails on.

- **GIVEN** `wt delete --stale <name> --dry-run`
- **WHEN** the command runs
- **THEN** it exits `ExitInvalidArgs` ("mutually exclusive") exactly as the non-dry-run form does
- **GIVEN** `wt delete <unknown> --dry-run`
- **WHEN** the command runs
- **THEN** it exits `ExitGeneralError` ("not found") exactly as the non-dry-run form does

#### R10: Help text mentions `--dry-run`
The `wt delete` `Long` help SHALL mention `--dry-run`; `wt help-dump` picks the flag up automatically from the cobra tree.

- **GIVEN** `wt delete --help`
- **WHEN** rendered
- **THEN** `--dry-run` appears in the flag list and the deprecated aliases stay hidden

### Destructive-path audit

#### R11: Audit the other commands and record dispositions
The apply-time audit SHALL verify the intake's per-command survey verdicts and record the dispositions. No other command gains `--dry-run` unless the audit finds a wt-owned destructive write.

- **GIVEN** the wt command surface (`create` incl. init/open phases, `update`, `go`, `list`, `shell-init`)
- **WHEN** each is audited for wt-owned destructive writes
- **THEN** `create` rollback force-removes only its own partial creation (`rollback.go`), init/open are delegated/side-effecting-not-destructive (`apps.go` `os.RemoveAll` targets the wt-owned byobu cache dir), `update` is delegated to brew, and `go`/`list`/`shell-init` are read-only — so none require `--dry-run`, and the disposition is recorded in conformance memory (hydrate)

### Docs & conformance

#### R12: cli-surface spec gains the `--dry-run` row; conformance re-check
`docs/specs/cli-surface.md`'s `wt delete` flag table SHALL gain a `--dry-run` row. The CLI-surface change SHALL be re-checked against the toolkit standards per the constitution's Toolkit Standards article (help-dump remains byte-stable modulo the new flag; README needs no change — it carries no per-flag detail).

- **GIVEN** the CLI-surface change
- **WHEN** the spec and help-dump are re-verified
- **THEN** the `--dry-run` row is present in the spec table and `help-dump` still emits valid JSON with the flag visible on the delete node

### Design Decisions

1. **Thread a `dryRun bool` through the `handle*` signatures**: add `dryRun bool` as an additional parameter to `handleDeleteCurrent`, `handleDeleteByName`, `handleDeleteMultiple`, `handleDeleteAll`, `handleDeleteStale`, `handleDeleteMenu`, `handleBranchCleanup`, `handleUncommittedChanges`, and `handleStashInDir`. — *Why*: the `handle*` functions already thread five explicit params (`nonInteractive, deleteBranch, deleteRemote, stashMode`, `session`/`rb`); adding a sixth `dryRun` bool follows the established pattern with the least churn and keeps the gating at the mutation seam. — *Rejected*: an options struct bundling the params (intake assumption #2 named this as the alternative) — a larger refactor that touches every call site and diverges from the current explicit-param style with no offsetting readability gain at six params.

2. **Gate at the leaf, print a `Would …` line mirroring the live completion message**: each suppressed mutation prints exactly one stdout line whose wording mirrors the live path's success message (`Deleted worktree` → `Would remove worktree`, `Deleted branch: X (local)` → `Would delete branch: X (local)`). — *Why*: one-for-one mirroring keeps preview↔live drift visible in review and satisfies the standard's single-source guarantee. — *Rejected*: argv-dump / JSON preview formats (heavier than the standard requires — intake assumption #6).

3. **Skip the `os.Chdir` to main repo under dry-run**: the chdir is a prerequisite of live `RemoveWorktree` (you cannot remove the worktree you are standing in); under dry-run nothing is removed, so the chdir is skipped to keep the invocation side-effect-free. The "You are no longer in a valid directory" trailer in `handleDeleteCurrent` is likewise suppressed under dry-run (it is false under a preview). — *Why*: constitution I / principle №5 — a dry-run must not mutate process or filesystem state.

### Non-Goals

- The `[ohwb]` stdout→stderr realignment of the existing live-path human chatter — separately tracked backlog item, explicitly out of scope.
- Adding `--dry-run` to any other command — the audit (R11) confirms none is required.
- Changing any exit code, prompt wording, or menu structure of the live (non-dry-run) path.

## Tasks

### Phase 1: Core flag + threading

- [x] T001 Register `--dry-run` (long-only bool) on `deleteCmd()` in `src/cmd/wt/delete.go`, add the local `dryRun` var, and thread it into the six `handleDelete*` dispatch calls in `RunE`. Add the `--dry-run` mention to the `Long` help text. <!-- R1 R2 R10 -->
- [x] T002 Thread `dryRun bool` through the signatures of `handleDeleteCurrent`, `handleDeleteByName`, `handleDeleteMultiple`, `handleDeleteAll`, `handleDeleteStale`, `handleDeleteMenu`, `handleBranchCleanup`, `handleUncommittedChanges`, and `handleStashInDir` in `src/cmd/wt/delete.go`, updating all internal call sites (menu→multiple/all/byName routing, stale→multiple). <!-- R2 R3 -->

### Phase 2: Gate the mutation leaves

- [x] T003 Gate worktree removal in `handleDeleteCurrent`, `handleDeleteByName`, `handleDeleteMultiple`, `handleDeleteAll`: under `dryRun`, skip the `os.Chdir` to main repo and the `RemoveWorktree` call, printing `Would remove worktree: <name>` to stdout instead of `Removing worktree...`/`Deleted worktree`. Suppress the "You are no longer in a valid directory" trailer in `handleDeleteCurrent`. <!-- R3 R4 -->
- [x] T004 Gate branch cleanup in `handleBranchCleanup`: under `dryRun`, run the tri-state decision, `BranchExistsRemotely`, and orphan `wt/<name>` checks live, but replace `DeleteLocalBranch`/`DeleteRemoteBranch` with `Would delete branch: <b> (local)` / `Would delete branch: <b> (remote)` stdout lines. The auto-mode name-mismatch skip message still prints. <!-- R3 R4 --> <!-- rework: review cycle 1 must-fix — preview↔live drift at delete.go:898-905: the orphan wt/<name> remote-existence check + "Would delete branch: wt/<name> (remote)" line are nested inside `if wt.BranchExistsLocally(wtOriginBranch)`, but the live path (delete.go:906-915) deletes the remote orphan independently of local existence; with a remote-only refs/heads/wt/<name> the live run deletes it while dry-run prints nothing. Un-nest to mirror the live structure: gate only the local "Would …" line on BranchExistsLocally; print the remote line whenever deleteRemote == "true" && BranchExistsRemotely(wtOriginBranch). Add a regression test for the remote-only wt/<name> case. -->
- [x] T005 Gate hazard handling: in `handleUncommittedChanges`, under `dryRun` skip the prompt and `StashCreate`, printing `Would stash uncommitted changes` (when `stashMode == "stash"`) or `Would discard uncommitted changes (use --stash to preserve them)`. Add a dry-run unpushed report (`Would lose N unpushed commit(s) on branch <b>`) at the `handleUnpushedCommits` call sites, computed via the live `GetUnpushedCount`. In `handleStashInDir` (used by ByName/Multiple), under `dryRun` run the has-changes detection live but print `Would stash uncommitted changes` instead of stashing. <!-- R3 R4 R6 -->

### Phase 3: Prompt suppression, header, and edge cases

- [x] T006 Suppress all delete-confirmation prompts under `dryRun` in `handleDeleteCurrent`, `handleDeleteByName`, `handleDeleteMultiple`, `handleDeleteAll`, and print the `Dry run — no changes will be made.` header once per invocation before the per-mutation lines (single-target and per the multi-target block structure). Keep the target-selection menu showing under dry-run (R7) and the non-interactive no-target refusal unchanged. <!-- R5 R7 R8 -->
- [x] T007 Verify exit-code and empty-state parity under dry-run: `--stale` mutexes, unknown-name, positional×`--worktree-name`, invalid `--stale` threshold, and outside-git-repo all keep their existing exit codes; `--stale` zero-match keeps `No idle worktrees (threshold: Nd).`. No code change expected beyond confirming the mutex/validation checks precede any dry-run branching. <!-- R9 R8 -->

### Phase 4: Tests, audit, docs

- [x] T008 [P] Add unit tests to `src/cmd/wt/delete_test.go`: preview-per-path (current, byName, multiple, all, stale), `--stash`×`--dry-run` (stash preview, no stash), `--branch` tri-state preview, `--no-remote` preview, uncommitted/unpushed hazard report lines, confirmation-prompt suppression, and the no-mutation guarantee (worktrees/branches/stash unchanged after dry-run). Exit-code parity: stale mutex, unknown name, invalid threshold under `--dry-run`. Follow the `runWt`/`runWtSuccess` + `assert*` patterns and the code-review no-side-effect rule. <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 --> <!-- rework: review cycle 1 should-fix — TestDelete_DryRunByNamePreviewsNoMutation and TestDelete_DryRunStashByNameNoStash pass the name positionally, routing through handleDeleteMultiple, so handleDeleteByName's dry-run branches (delete.go:324, 343-347) have no coverage. Add a `--worktree-name <name> --dry-run` variant (and optionally a menu-route test via the runWtStdin fallback-menu pattern, see create_test.go:860) so the byName/menu paths are genuinely covered. -->
- [x] T009 [P] Add an integration test to `src/cmd/wt/integration_test.go`: dry-run over a real repo (dirty worktree, matching branch, pushed to origin) leaves worktree dir, local branch, remote branch, and stash state byte-identical, exits 0, and stdout carries the `Would …` lines; plus a multi-target (`--all`/`--stale`) dry-run leaving all state intact. <!-- R3 R4 R8 -->
- [x] T010 [P] Add the `--dry-run` row to the `wt delete` flag table in `docs/specs/cli-surface.md` and confirm `TestHelpDump_EmitsValidEnvelope` still passes (flag visible, envelope valid). <!-- R10 R12 -->
- [x] T011 Verify the destructive-path audit verdicts against source (`rollback.go` self-scoped removal, `apps.go` byobu-cache `RemoveAll`, `update.go` brew delegation, `go`/`list`/`shell-init` read-only) and record the audit findings in the plan Notes for hydrate to fold into conformance memory. No other command gains `--dry-run`. <!-- R11 -->

## Execution Order

- T001 → T002 (threading depends on the flag + var existing) → T003, T004, T005 (leaf gating depends on the threaded param) → T006, T007 (prompt/header/edge on top of gated leaves).
- T008–T011 are `[P]` (independent files: delete_test.go, integration_test.go, cli-surface.md, audit) but run after Phase 3 so the behavior they assert exists.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt delete --dry-run` is registered as a long-only bool, previews without mutating, and takes no short flag.
- [x] A-002 R2: `--dry-run` is honored across all six target-resolution paths (stale, all, positionals, `--worktree-name`, current, menu).
- [x] A-003 R3: All decision logic runs live under dry-run — target resolution (incl. `--stale` idle computation), the `--branch` tri-state auto rule, remote-existence checks, hazard detection, and the orphan `wt/<name>` checks. The prior cycle's drift is fixed: the orphan remote-existence check now runs independently of local existence (`delete.go:903-909` gates the local `Would …` line on `BranchExistsLocally` and the remote line only on `deleteRemote == "true" && BranchExistsRemotely`), mirroring the live structure — verified by `TestDelete_DryRunRemoteOnlyOrphanPreviewsRemote`.
- [x] A-004 R4: Every mutation leaf is suppressed and replaced by exactly one `Would …` line — worktree removal, local/remote branch delete, orphan `wt/<name>` cleanup (including the remote-only-orphan case), and stash-or-discard (`handleUncommittedChanges` / `handleStashInDir`). The `os.Chdir` to main repo is skipped under dry-run on all paths (current/multiple/all).
- [x] A-005 R10: `wt delete --help` lists `--dry-run`; deprecated aliases stay hidden.
- [x] A-006 R11: The destructive-path audit verdicts are verified against source and recorded; no other command gains `--dry-run`.
- [x] A-007 R12: `docs/specs/cli-surface.md` `wt delete` table has the `--dry-run` row.

### Behavioral Correctness

- [x] A-008 R5: Under dry-run, every delete-confirmation prompt is skipped.
- [x] A-009 R6: Hazard prompts become report lines — `Would stash …` (with `--stash`) / `Would discard uncommitted changes (use --stash to preserve them)` (without), and `Would lose N unpushed commit(s) on branch <b>` — using the live detection functions.
- [x] A-010 R7: The target-selection menu still shows under dry-run on a TTY with no target; non-interactive no-target keeps the `ExitInvalidArgs` refusal.
- [x] A-011 R8: Preview `Would …` lines print to stdout under a `Dry run — no changes will be made.` header; multi-target forms keep per-worktree blocks; `--stale` zero-match keeps its empty-state message.

### Scenario Coverage

- [x] A-012 R3: A unit test exercises a single dirty worktree with matching+pushed branch and asserts one `Would …` line per suppressed mutation with no state change. (Covered by `TestIntegration_DeleteDryRun_LeavesStateByteIdentical` end-to-end plus the per-line `delete_test.go` tests.)
- [x] A-013 R4: An integration test asserts dry-run leaves worktree dir, local branch, remote branch, and stash state byte-identical and exits 0.
- [x] A-014 R8: A test asserts the multi-target (`--all`/`--stale`) dry-run block structure and no-mutation guarantee.

### Edge Cases & Error Handling

- [x] A-015 R9: Exit-code parity — `--stale` mutexes, unknown-name, positional×`--worktree-name`, invalid `--stale` threshold, and outside-git-repo keep their existing exit codes under `--dry-run`.
- [x] A-016 R12: `help-dump` still emits a valid envelope with `--dry-run` visible on the delete node (`TestHelpDump_EmitsValidEnvelope` green).

### Code Quality

- [x] A-017 Pattern consistency: New code follows the existing `delete.go` threaded-param convention, `wt.Color*` output style, and stdout/stderr discipline (preview = machine result on stdout).
- [x] A-018 No unnecessary duplication: Hazard/decision detection reuses the existing `internal/worktree` functions (`IsIdle`, `RecencyOf`, `BranchExistsRemotely`, `HasUncommittedChanges`, `GetUnpushedCount`) — no parallel preview derivation.
- [x] A-019 No magic strings without constants: `Would …` line wording mirrors the live completion messages one-for-one (single-sourced intent), not ad-hoc duplicated copy.
- [x] A-020 Business rules stay in `internal/worktree`; the dry-run gating is orchestration and lives in `cmd/` (constitution V).
- [x] A-021 No test side-effect leaks: dry-run tests assert state is unchanged and rely on `runWt` env isolation (no tmux/byobu/app launches), per code-review.md.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

### Destructive-path audit findings (for hydrate → conformance memory)

Apply-time audit of every wt command for wt-owned destructive writes (principle №5). Verdicts confirm the intake survey — only `wt delete` has wt-owned destructive writes, and it now supports `--dry-run`. No other command requires `--dry-run`.

| Command | Verdict | Basis (verified in source) |
|---------|---------|----------------------------|
| `wt delete` | Destructive — now conformant | Gains `--dry-run` in this change (live-path-sharing preview, all six paths). |
| `wt create` | Additive, no `--dry-run` | `git worktree add -b` + `MkdirAll`; `rollback.go` `Register`s only its own partial creation (`git worktree remove --force <wtPath>`, `git branch -D <branch>`) and fires on failure/SIGINT — never touches pre-existing user data. |
| `wt create` init / `wt init` | Delegated, not wt-owned | Runs the user-configured init script; wt writes nothing destructive of its own. |
| `wt create` open / `wt open` | Side-effecting, not destructive | Spawns windows/apps; `apps.go:269` `os.RemoveAll` targets the wt-owned byobu cache dir (`~/.cache/byobu/.last.tmux`), not user data. |
| `wt update` | Delegated to Homebrew | `internal/update` shells to `brew upgrade`/`brew update`; brew owns the mutation semantics. |
| `wt go` | Read-only + launcher handshake | Only write is `os.WriteFile(WT_CD_FILE, ...)` (`go.go:132`) — the caller-owned launcher-contract handshake, not user-data mutation. |
| `wt list` / `wt shell-init` | Read-only | No filesystem/git mutation. |

## Deletion Candidates

None — this change adds new functionality without making existing code redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Thread a `dryRun bool` through the `handle*` signatures rather than bundle params into an options struct | The `handle*` functions already thread five explicit params; a sixth bool follows the pattern with least churn (intake assumption #2 deferred the mechanism to plan time) | S:80 R:65 A:85 D:75 |
| 2 | Confident | Skip the `os.Chdir` to main repo and suppress the "no longer in a valid directory" trailer under dry-run | The chdir is a prerequisite of live removal only; a dry-run must be side-effect-free (constitution I / principle №5) | S:75 R:75 A:85 D:70 |
| 3 | Confident | Unpushed-commit report wording is `Would lose N unpushed commit(s) on branch <b>`, computed via live `GetUnpushedCount` | Intake §3 names this exact line; it mirrors the live `handleUnpushedCommits` warning intent one-for-one | S:75 R:80 A:80 D:75 |
| 4 | Confident | The `Dry run — no changes will be made.` header prints once per invocation, before the per-mutation lines (per multi-target block) | Intake §4 example shows the header; once-per-invocation matches the single-target example and reads cleanly for multi-target blocks | S:70 R:80 A:75 D:70 |

4 assumptions (0 certain, 4 confident, 0 tentative).
