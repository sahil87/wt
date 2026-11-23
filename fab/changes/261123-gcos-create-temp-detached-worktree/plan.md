# Plan: `wt create --temp <ref>` — a detached throwaway worktree

**Change**: 261123-gcos-create-temp-detached-worktree
**Intake**: `intake.md`

## Requirements

> Source of truth: `intake.md` (and the plan it cites, `/home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md` § W5 / Decisions / Rules). At plan time `origin/main` = `22ae6fe` — W1 (`--fetch`/`--json`), W3, W4 (`--pr`) are NOT merged, so this change adds no `--fetch`, `--json` or `--pr` wiring (intake § 6, Assumption 9). If `origin/main` has moved when apply starts, rebase first and, only if W1/W4 landed, add the intake § 6 integration (fetch-before-resolve + `mode: "temp"` receipt; `--temp`×`--pr` mutex) with tests.

### wt create: `--temp <ref>` mode

#### R1: `--temp <ref>` creates a detached worktree at the resolved commit
`wt create --temp <ref>` SHALL resolve `<ref>` to a commit SHA once (in `internal/worktree`, via `git rev-parse --verify --quiet <ref>^{commit}`) and run `git worktree add --detach <path> <sha>` under `<repo>.worktrees/`. It MUST NOT create a branch and MUST NOT write any git config. The rollback SHALL register only `git worktree remove --force <path>` (no `branch -D`). On success stdout's last line SHALL be the worktree path (existing contract).

- **GIVEN** a repo with commit `C` (not HEAD of the current branch)
- **WHEN** the user runs `wt create --temp <C-sha> --non-interactive`
- **THEN** a worktree exists at `<repo>.worktrees/tmp-<adj>-<noun>`, its `HEAD` == `C`, `git symbolic-ref -q HEAD` fails in it (detached), `git branch --list` is unchanged, and stdout's last line is the path
- **AND** `--temp <branch-name>` resolves to that branch's tip SHA and the worktree stays detached (the branch is not checked out)

#### R2: Temp worktree names always carry the `tmp-` prefix
The worktree name SHALL be `tmp-` + a random unique adjective-noun when `--name` is absent; `--name foo` SHALL yield `tmp-foo`; `--name tmp-foo` SHALL yield `tmp-foo` (no double prefix). The prefix SHALL be a named constant in `internal/worktree` (e.g. `TempNamePrefix = "tmp-"`). Random-name generation SHALL reuse the existing generator through a prefixed variant (e.g. `GenerateUniqueNameWithPrefix(dir, prefix, retries)`, with `GenerateUniqueName` delegating with `""`) — collision checked on the prefixed name. The interactive name prompt SHALL be skipped under `--temp`. An existing directory at the final name → the existing `Worktree '<name>' already exists` error (`ExitGeneralError`); retry exhaustion → `ExitRetryExhausted`.

- **GIVEN** `wt create --temp HEAD --name foo --non-interactive`
- **WHEN** it runs
- **THEN** the worktree is `<repo>.worktrees/tmp-foo`
- **AND** `--name tmp-bar` yields `tmp-bar`, and with no `--name` the basename matches `^tmp-[a-z]+-[a-z]+$`

#### R3: Mode conflicts are `ExitInvalidArgs` before any mutation
In `cmd/create.go`, alongside the existing `--checkout` mutex checks and before any git work, these combinations SHALL exit `ExitInvalidArgs` (2) via `wt.ExitWithError` with the intake § 1.1 Error/Why/Fix copy, creating nothing: `--temp` + positional; `--temp` + `--checkout`; `--temp` + `--reuse`; `--temp` + `--base`; `--init` + `--no-init`. An unresolvable `<ref>` SHALL exit 2 with `Invalid --temp ref: <ref>` / `'<ref>' does not resolve to a commit` / `Provide a valid branch name, tag, or commit SHA`. `git worktree add` failure → `ExitGitError` (3). No new exit codes.

- **GIVEN** each conflicting invocation (e.g. `wt create --temp HEAD feat-x`, `--temp HEAD --checkout main`, `--temp HEAD --reuse --name n`, `--temp HEAD --base main`, `--temp HEAD --init --no-init`) or `--temp no-such-ref`
- **WHEN** it runs
- **THEN** exit code is 2, stderr carries the specific `Error:` line, and no worktree directory or branch is created

#### R4: Git-phase summary for temp worktrees
The Git-phase summary (stderr) SHALL read `Branch: (detached)` and `From: <ref as typed> @ <7-char short sha>`, the `From:` value resolved pre-dispatch (outside the reinstall window — `create-output-phases.md`). stdout carries only the path line(s) as today.

- **GIVEN** `wt create --temp HEAD~1 --non-interactive`
- **WHEN** it runs
- **THEN** stderr contains `Branch: (detached)` and `From: HEAD~1 @ <short-sha-of-HEAD~1>`, and stdout's last line is the path

### wt create: init on `--temp`

#### R5: Init is skipped for `--temp` unless `--init`
Under `--temp`, the init script SHALL NOT run by default (no Init separator, no `Running worktree init...`, no skip warning). A new long-only bool `--init` SHALL opt in: init then runs through the existing runner and signal/foreground/init-failure machinery (`ExitInitFailed` (7) with the worktree kept), with **no** `Initialize worktree?` confirm. `--init` without `--temp` SHALL be accepted as a no-op. `--init` + `--no-init` → R3.

- **GIVEN** `WORKTREE_INIT_SCRIPT` pointing at a script that creates a marker file
- **WHEN** the user runs `wt create --temp HEAD --non-interactive`
- **THEN** the marker does not exist in the temp worktree and stderr has no `Init (` separator
- **AND** with `--init` added, the marker exists and the init separator is printed
- **AND** `wt create --init --non-interactive` (no `--temp`) behaves exactly like `wt create --non-interactive`

### wt list: temp marking

#### R6: `wt list` marks temp worktrees without extra subprocesses
`listEntry` SHALL gain an always-present `temp` JSON bool. A worktree SHALL be temp iff its porcelain entry is `detached` AND it is not the main worktree AND its basename starts with `TempNamePrefix` — computed by a pure helper in `internal/worktree` (e.g. `IsTempWorktree(name string, detached bool) bool`) with zero additional git subprocesses (default mode stays exactly one `git worktree list --porcelain`). Temp records SHALL emit `"branch": null`; all other records (including non-temp detached ones, which keep `"(detached)"`) keep their branch string. Human output (all layouts incl. `--status`) SHALL show `(detached)` in the Branch column for temp worktrees; `--status` SHALL still skip the unpushed probe for detached worktrees.

- **GIVEN** a repo with one temp worktree (`tmp-x`, detached), one normal worktree, and a raw `git worktree add --detach` worktree named `raw-det`
- **WHEN** the user runs `wt list --json`
- **THEN** `tmp-x` has `"temp": true, "branch": null`; the normal worktree and main have `"temp": false` with their branch; `raw-det` has `"temp": false, "branch": "(detached)"`
- **AND** `wt list` / `wt list --status` show `(detached)` for `tmp-x`

### wt delete: detached worktrees have no branch

#### R7: `wt delete` performs no branch handling for detached worktrees
For a detached worktree — porcelain sentinel `(detached)` on the by-name/multi/all/stale/menu paths, or `HEAD` from `git rev-parse --abbrev-ref HEAD` on the delete-current path — `wt delete` SHALL remove the worktree and SHALL NOT: run branch cleanup (no `Skipped branch deletion`/`Deleted branch` lines, `--branch true` is a no-op, no `wt/<name>` orphan cleanup), run the unpushed-commits check, or print `Would delete branch` lines under `--dry-run`. The delete header's `Branch:` line SHALL show `(detached)` (not `HEAD`). All other delete behavior (stash, dirty-discard hazard, confirmations, exit codes) is unchanged.

- **GIVEN** a temp worktree `tmp-x`
- **WHEN** the user runs `wt delete tmp-x --non-interactive` (also with `--branch true`)
- **THEN** the worktree is removed, output contains no `branch deletion`/`Deleted branch` copy, and every pre-existing branch still exists
- **AND** `wt delete tmp-x --dry-run` prints `Would remove worktree: tmp-x` and no `Would delete branch` line

### Docs: CLI ⇒ docs in the same change

#### R8: Specs, skill bundle and help-dump reflect `--temp` / `--init`
`docs/specs/cli-surface.md` (§ `wt create`: `--temp`, `--init` rows + temp paragraph + exit codes; § `wt list`: `temp` key and `branch: null`; § `wt delete`: detached = no branch cleanup), `docs/specs/worktree-layout.md` (temp naming + branch↔worktree case 3), `docs/specs/init-protocol.md` (temp init default + `--init`) SHALL be updated. `docs/site/skill.md` SHALL gain a short `--temp` mention and `just sync-skill` SHALL refresh `src/cmd/wt/skill.md` (drift-guard + ≤150-line budget tests pass). `wt help-dump` reflects the new flags automatically (cobra tree); any test enumerating `create` flags is updated. The change SHALL be checked against `shll standards` for help/CLI-surface conventions when `shll` is available. (Memory files are hydrate's job.)

- **GIVEN** the built binary
- **WHEN** `wt create --help` and `wt help-dump` run
- **THEN** both list `--temp` and `--init`; `go test ./...` (incl. skill drift-guard) passes

### Non-Goals

- `--fetch`, `--json` receipt, `--pr` — W1/W4 scope; integrate only if already merged at apply time (intake § 6)
- A `temp` field on `internal/worktree.Info` / changes to `wt go` / `wt open` — plan non-goal
- Recording a base in git config for temp worktrees — W2 explicitly excludes `--temp`
- Changing `"(detached)"` → null for non-temp detached worktrees — back-compat

### Design Decisions

#### Init skipped by default on `--temp`, `--init` opts in
**Decision**: `--temp` skips the init script unless the new `--init` bool is passed; `--init` alone is a no-op; `--init`+`--no-init` is exit 2.
**Why**: Surgery worktrees are short-lived; `fab sync` would write files into the very tree being rebased/resolved, and init is the slowest create phase. The plan lists this option first and its rationale ("surgery never wants `fab sync`") favors it.
**Rejected**: Run init as usual (consistency) — makes every throwaway pay for and be dirtied by init; callers would need `--no-init` on every surgery call.
*Introduced by*: 261123-gcos-create-temp-detached-worktree

#### Temp predicate = detached + non-main + `tmp-` basename
**Decision**: `wt list` derives `temp` from the porcelain `detached` line and the basename prefix only.
**Why**: Zero extra subprocesses (preserves list-status-contract's single-subprocess default) and no state file/config (Constitution I); the prefix is guaranteed by `--temp` naming.
**Rejected**: A git-config or marker-file flag per worktree — hidden state and an extra read per worktree; "any detached worktree is temp" — mislabels hand-made detached worktrees.
*Introduced by*: 261123-gcos-create-temp-detached-worktree

#### `--temp` + `--base` is a usage error
**Decision**: Reject the combination with exit 2 even though the brief's exclusive list omits it.
**Why**: `--temp`'s value already is the commit; silent precedence is the gotcha the plan's W3 removes for `--reuse`+`--base`.
**Rejected**: Ignore `--base` or let it win — silent precedence.
*Introduced by*: 261123-gcos-create-temp-detached-worktree

## Tasks

### Phase 1: Setup

- [x] T001 Rebase onto latest `origin/main` — verified by the orchestrator at apply entry: branch base == `origin/main` (`22ae6fe`), W1/W3/W4 NOT merged, so no `--fetch`/`--json`/`--pr` integration in this change (intake § 6) <!-- R1 -->
- [x] T002 [P] Add `TempNamePrefix` constant, `TempName(name string) string` (prefix unless already prefixed), `GenerateUniqueNameWithPrefix(worktreesDir, prefix string, maxRetries int)` (with `GenerateUniqueName` delegating to it with `""`) and `IsTempWorktree(name string, detached bool) bool` in `src/internal/worktree/names.go`; unit tests in `src/internal/worktree/names_test.go` <!-- R2 -->

### Phase 2: Core Implementation

- [x] T003 [P] Add `ResolveCommit(ref string) (string, error)` (`git rev-parse --verify --quiet <ref>^{commit}`) to `src/internal/worktree/git.go` and `CreateTempWorktree(sha, name string, ctx *RepoContext, rb *Rollback) (string, error)` (EnsureWorktreesDir + `git worktree add --detach <path> <sha>`, rollback registers only worktree removal, no config) to `src/internal/worktree/crud.go`; unit tests (detached HEAD at sha, no branch created, bad ref error) in the package's test files <!-- R1 -->
- [x] T004 Add `--temp <ref>` and `--init` flags to `src/cmd/wt/create.go`; add the R3 mutex checks (`--temp`×positional/`--checkout`/`--reuse`/`--base`, `--init`×`--no-init`) next to the existing `--checkout` checks with intake § 1.1 copy; resolve the ref via `wt.ResolveCommit` (bad ref → exit 2); compute the temp name (skip the name prompt); dispatch to `wt.CreateTempWorktree` (failure → `ExitGitError`); set summary `Branch: (detached)` and pre-dispatch `From: <ref> @ <short-sha>`; update the command's `Long` help to describe `--temp` <!-- R1 R2 R3 R4 -->
- [x] T005 Implement the R5 init gate in `src/cmd/wt/create.go`: under `--temp`, `runInit` is true only when `--init` is set, and the `Initialize worktree?` confirm never fires for `--temp`; `--init` without `--temp` leaves today's flow unchanged <!-- R5 -->
- [x] T006 [P] In `src/cmd/wt/list.go`: add always-present `Temp bool \`json:"temp"\``; make `branch` serialize as `null` for temp records only (e.g. `Branch *string` without omitempty, or a custom `MarshalJSON` — least invasive; keep human rendering and sorting by branch working, temp rows render `(detached)`); set `Temp` in `buildBaseEntry` via `wt.IsTempWorktree`; keep the `--status` unpushed skip for detached worktrees; no new subprocess in default mode <!-- R6 -->
- [x] T007 [P] In `src/cmd/wt/delete.go`: treat a detached worktree (`(detached)` from the porcelain, `HEAD` from `CurrentBranch` on the delete-current path) as branchless — normalize at the read sites or via a small helper so `handleBranchCleanup` returns early (no orphan `wt/` cleanup, `--branch true` no-op, no dry-run branch lines), skip `HasUnpushedCommits` when detached, and print `Branch: (detached)` in the header <!-- R7 -->

### Phase 3: Integration & Edge Cases

- [x] T008 Unit tests in `src/cmd/wt/create_test.go`: temp happy path (detached at sha, no branch, path last stdout line, summary lines), `--temp <branch>` stays detached, naming (`--name foo` → `tmp-foo`, `--name tmp-foo`, random `^tmp-`), each R3 conflict exits 2 with nothing created, bad ref exits 2, init skipped by default / runs with `--init` / `--init` alone no-op (use `--open skip` or `--non-interactive`; no host side-effects per code-review.md) <!-- R1 R2 R3 R4 R5 -->
- [x] T009 [P] Unit tests in `src/cmd/wt/list_test.go` (temp JSON `temp:true, branch:null`; others `temp:false`; raw detached non-`tmp-` keeps `"(detached)"`, `temp:false`; human + `--status` show `(detached)`) and `src/cmd/wt/delete_test.go` (delete temp by name, `--branch true`, `--dry-run`, delete-current from inside a temp worktree: no branch copy, branches untouched) <!-- R6 R7 -->
- [x] T010 Integration test in `src/cmd/wt/integration_test.go`: end-to-end temp lifecycle against a real temp repo — `create --temp <sha>` → `list --json` marks it → `delete <name>` removes it, no branch created or deleted, conflicts exit 2 <!-- R1 R3 R6 R7 -->

### Phase 4: Polish

- [x] T011 Update `docs/specs/cli-surface.md`, `docs/specs/worktree-layout.md`, `docs/specs/init-protocol.md` per R8; add a one/two-line `--temp` mention to `docs/site/skill.md` and run `just sync-skill`; check `shll standards` (if available) for help-output conventions; run `cd src && go vet ./... && go test ./...` and `gofmt -l` clean <!-- R8 -->

## Execution Order

- T002 and T003 precede T004/T005 (cmd uses the internal helpers); T002 precedes T006 (`IsTempWorktree`)
- T008–T010 follow the code they test; T011 last

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt create --temp <ref>` creates a worktree whose HEAD is the resolved SHA, detached, with no new branch and no new `branch.*` git config
- [x] A-002 R2: Temp names are `tmp-`-prefixed (random, `--name foo` → `tmp-foo`, `--name tmp-foo` not double-prefixed), prefix is a named constant, random generation reuses the existing generator
- [x] A-003 R3: All five conflicts and an invalid `--temp` ref exit 2 before any mutation with Error/Why/Fix copy
- [x] A-004 R4: Summary shows `Branch: (detached)` and `From: <ref> @ <short-sha>`; stdout last line is the path
- [x] A-005 R5: Init skipped by default under `--temp`, runs with `--init` (no confirm), `--init` alone is a no-op
- [x] A-006 R6: `wt list --json` emits `temp` on every record; temp records have `branch: null`; non-temp detached keep `"(detached)"`
- [x] A-007 R7: `wt delete` on a detached worktree removes it with no branch handling on every target-resolution path, including `--branch true` and `--dry-run`
- [x] A-008 R8: cli-surface, worktree-layout, init-protocol specs and the skill bundle (synced) document `--temp` / `--init`

### Behavioral Correctness

- [x] A-009 R7: The bogus `Skipped branch deletion: branch '(detached)'…` line and the `git branch -D HEAD` attempt no longer occur for detached worktrees
- [x] A-010 R1: Existing create modes (bare, positional, `--checkout`, `--reuse`, `--base`) are byte-for-byte unchanged when `--temp` is absent

### Scenario Coverage

- [x] A-011 R1: Unit + integration tests assert detached HEAD at the SHA and no branch for `--temp <sha>` and `--temp <branch>`
- [x] A-012 R6: Tests cover temp, normal, main and raw-detached records in `wt list --json` and human output
- [x] A-013 R5: Tests cover init skipped / `--init` runs / `--init` alone

### Edge Cases & Error Handling

- [x] A-014 R3: Failed `git worktree add` under `--temp` exits 3 and leaves no directory behind (rollback)
- [x] A-015 R2: Name collision at `tmp-<name>` exits 1 with the existing "already exists" copy; nothing created
- [x] A-016 R6: Default `wt list` still spawns exactly one git subprocess (temp predicate is pure) — verified by inspection: `IsTempWorktree` is pure and reads the porcelain `detached` flag already parsed; no subprocess-count test exists

### Code Quality

- [x] A-017 Pattern consistency: New code follows create.go/crud.go/list.go/delete.go naming and Error/Why/Fix patterns; git ops only in `internal/worktree` (Constitution V)
- [x] A-018 No unnecessary duplication: Prefixed name generation reuses `GenerateRandomName`/`GenerateUniqueName`; detached normalization lives in one place
- [x] A-019: No magic strings — `tmp-` and the detached sentinel are named constants
- [x] A-020: No god functions — the create RunE grows by small, readable blocks (extract helpers if the temp path adds >~30 lines inline)
- [x] A-021: Readability over cleverness; tests use non-side-effecting `--open skip` / `--non-interactive` per code-review.md

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `src/internal/worktree/context.go:181-196` (`DescribeHead`'s literal `"HEAD"` comparisons) — the new `detachedHeadRef` constant / `IsDetachedBranch` in `worktree.go` now names the same "abbrev-ref says detached" sentinel; `DescribeHead` could reuse it instead of the bare literal
- `src/cmd/wt/list.go` `rawEntry.branch = DetachedBranchLabel` on `detached` lines — with the new `rawEntry.detached` bool carrying the fact, the sentinel string in `branch` is kept only for human rendering/back-compat; no deletion needed now, noted so a future cleanup does not reintroduce string comparisons on it

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | No `--fetch`/`--json`/`--pr` wiring since W1/W4 are unmerged at plan time; T001 re-checks after rebase | intake Assumption 9; `origin/main` = `22ae6fe` verified at apply entry | S:70 R:70 A:75 D:65 |
| 2 | Confident | `Branch` serialization for temp-only null is the apply agent's least-invasive choice (pointer or custom marshal) | Implementation detail with identical observable JSON | S:60 R:85 A:70 D:60 |
| 3 | Confident | `--init` without `--temp` is a silent no-op, not an error | Keeps scripts that always pass `--init` working; intake § 2 | S:65 R:85 A:70 D:65 |
| 4 | Confident | `ResolveCommit` lives in `git.go`; W1 may add an equivalent — whichever merges second dedupes | Constitution V; parallel steps | S:60 R:85 A:70 D:60 |

4 assumptions (0 certain, 4 confident, 0 tentative).
