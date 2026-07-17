# Intake: wt delete --dry-run and destructive-path audit

**Change**: 260717-p5m9-delete-dry-run
**Created**: 2026-07-18

## Origin

One-shot `/fab-new p5m9` from backlog item `[p5m9]` (deferred from change `260717-6end-toolkit-standards-conformance`):

> Add `--dry-run` to `wt delete` (toolkit principle No.5: destructive writes MUST support an accurate --dry-run preview that shares the real code path, requiring no consent). Audit `wt create` init/open and any other destructive path for the same. Deferred from 6end as a new flag plus a preview code path threaded through the live deletion flow (a restructuring-sized change beyond the audit's small-additive fixes).

The 6end conformance report (`fab/changes/260717-6end-toolkit-standards-conformance/conformance-report.md`) dispositioned principle №5 as **gap**: read-vs-write is clear from command names/help and `wt delete` requires explicit consent per №1, but no destructive path supports `--dry-run`. The sibling deferral `[ohwb]` (`wt delete` stdout→stderr realignment) is a **separate** backlog item and stays out of scope here.

## Why

1. **Pain point**: `wt delete` is the toolkit's most destructive command — it removes worktree directories (`git worktree remove --force`), deletes local branches (`git branch -D`), deletes remote branches (`git push origin --delete`), and can discard uncommitted work. An agent (or cautious human) has no way to see what a given invocation *would* do — especially under the selector forms (`--all`, `--stale`, multiple positionals) where the target set is computed, and under the tri-state `--branch` auto logic where branch deletion depends on a name-match rule. Today the only "preview" is running the interactive flow and cancelling at the confirm prompt, which is unusable for automation.
2. **Consequence of not fixing**: `wt` remains non-conformant with toolkit principle №5 (a MUST): "Destructive writes MUST support `--dry-run` (an accurate preview, sharing the real code path — a dry-run that drifts from the live path is worse than none)." Agents either avoid `wt delete` or mutate blindly — the principle's named failure mode.
3. **Why this approach**: the standard itself prescribes the shape via its enforcement receipt — `shll uninstall --dry-run` "previews with the same single-sourced command builder the live run executes (`brewUninstallArgv` threads into both)". The wt analog: run every selection/decision path live (target resolution, idle selection, branch tri-state decision, remote-existence check, hazard detection) and suppress only the leaf mutations, printing what each would have done. A parallel "preview implementation" that re-derives the decisions separately is explicitly worse than nothing per the standard.

## What Changes

### 1. New `--dry-run` flag on `wt delete`

Register on `deleteCmd()` in `src/cmd/wt/delete.go`:

```go
cmd.Flags().BoolVar(&dryRun, "dry-run", false,
    "Preview what would be deleted without making any change (no confirmation prompts)")
```

No short flag (constitution II: short flags only when they aid common interactive use; this is a preview/automation flag — precedent: `--non-interactive` has none). The flag applies uniformly to **all six target-resolution paths** in the existing resolution order (`delete.go` RunE): `--stale` → `--all` → positional names → deprecated `--worktree-name` → current-worktree → interactive menu.

### 2. Preview shares the live code path

The mutation leaves of the delete flow are:

| Mutation | Call site(s) | Underlying commands |
|----------|--------------|---------------------|
| Worktree removal | `wt.RemoveWorktree(path, true)` in `handleDeleteCurrent`/`ByName`/`Multiple`/`All` | `git worktree remove --force <path>` + `git worktree prune` |
| Local branch delete | `handleBranchCleanup` → `wt.DeleteLocalBranch(branch, true)` | `git branch -D <branch>` |
| Remote branch delete | `handleBranchCleanup` → `wt.DeleteRemoteBranch(branch)` | `git push origin --delete <branch>` |
| Orphan `wt/<name>` branch cleanup | `handleBranchCleanup` tail | same two branch commands |
| Stash-and-clear | `handleUncommittedChanges` (`wt.StashCreate`) and `handleStashInDir` | `git add -A`, `stash create/store`, `reset --hard`, `clean -fd` |
| Discard (implicit) | force removal of a dirty worktree | carried by `git worktree remove --force` |

Under `--dry-run`, **all decision logic executes live** — target resolution (including `--stale` idle computation via `IsIdle`/`RecencyOf`), the `--branch` tri-state auto rule (`branch == wtName`), `BranchExistsRemotely()` remote checks, `HasUncommittedChanges()`/`HasUnpushedCommits()` hazard detection, and the orphan `wt/<name>` check. **Only the leaf mutations are suppressed**, each replaced by a preview line naming the exact action and target. The gating lives at the mutation seam so live and preview cannot drift: each leaf call site routes through a single point that either executes or prints (mechanism — threading a `dryRun` flag through the `handle*` signatures vs. bundling the existing five threaded params (`nonInteractive`, `deleteBranch`, `deleteRemote`, `stashMode`, + `dryRun`) into an options struct — is decided at plan time; the contract is: one decision path, gated leaves, no parallel preview derivation).

Constitution V holds: decision logic already lives in `internal/worktree` (idle predicate, branch existence, stash); the preview gating is orchestration and stays in `cmd/`.

### 3. Consent and prompt interplay

Per principle №5 + №1 (and the shll reference: "`--dry-run` requiring no consent at all"):

- `--dry-run` **skips all confirmation prompts** — the "Delete this worktree?" / "Delete these N worktrees?" / "Delete ALL …?" menus never show.
- The **hazard prompts become preview lines**: where the live interactive flow prompts on uncommitted changes or unpushed commits, dry-run instead reports what the equivalent consented run would do — e.g. `Would discard uncommitted changes (use --stash to preserve them)`, `Would stash uncommitted changes`, `Would lose 3 unpushed commit(s) on branch <b>`. Hazard detection uses the same live functions the prompts use today.
- The **target-selection menu is not consent** and still shows when dry-run is invoked with no target on a TTY (`wt delete --dry-run` from the main repo) — selection picks *what* to preview. The row labels/flow are unchanged; the selected target routes into the preview. Non-TTY/`--non-interactive` with no target keeps today's refusal (`ExitInvalidArgs`, "No worktree specified").
- `--dry-run` composes with every existing flag: `--stash` previews the stash action instead of performing it; `--stale`/`--all`/positionals preview the computed target set; `--branch`/`--no-remote` alter the previewed branch actions exactly as they alter the live run.

### 4. Preview output contract

- Preview lines go to **stdout** — the preview is the machine result the caller asked for (principle №2; repo stdout/stderr convention). The existing live-path chatter realignment is `[ohwb]`'s scope and is not touched here.
- Format: human-scannable `Would …` action lines mirroring the live path's completion messages one-for-one, so each suppressed mutation produces exactly one line with its concrete target. Example, single dirty worktree whose branch matches its name and exists on origin:

```
$ wt delete lively-otter --dry-run
Worktree: lively-otter
Branch: lively-otter
Path: /home/u/repo.worktrees/lively-otter

Dry run — no changes will be made.
Would discard uncommitted changes (use --stash to preserve them)
Would remove worktree: lively-otter
Would delete branch: lively-otter (local)
Would delete branch: lively-otter (remote)
```

- Multi-target forms (`--all`, `--stale`, positionals) keep their per-worktree block structure with the same `Would …` lines per block; `--stale` keeps its live empty-state (`No idle worktrees (threshold: Nd).`).
- **Exit codes unchanged**: successful preview exits `ExitSuccess` (0); arg validation (`--stale` mutexes, unknown worktree names, positional×`--worktree-name` mix) keeps its existing `ExitInvalidArgs`/`ExitGeneralError` behavior — dry-run must fail on exactly the inputs the live run fails on.
- Help text (`Long`) gains a `--dry-run` mention; `wt help-dump` picks the flag up automatically from the cobra tree.

### 5. Destructive-path audit (the backlog's second half)

Audit every other command for wt-owned destructive writes and record a per-command disposition. Expected verdicts from intake-time code survey (to be verified at apply):

| Command | Survey verdict | Basis |
|---------|---------------|-------|
| `wt create` | Additive, no `--dry-run` required | `git worktree add -b`, `MkdirAll`; rollback (`rollback.go`) force-removes only its **own partial creation** on failure/SIGINT — never pre-existing user data |
| `wt create` init phase / `wt init` | Delegated, not wt-owned | Runs the user-configured init script (`init-protocol.md`); wt itself writes nothing destructive |
| `wt create` open phase / `wt open` | Side-effecting, not destructive | Spawns windows/apps; `os.RemoveAll(byobuCache)` in `apps.go` clears a wt-owned cache dir, not user data |
| `wt update` | Delegated to Homebrew | Binary upgrade via `brew upgrade`; brew owns the mutation semantics |
| `wt go` / `wt list` / `wt shell-init` | Read-only | `WT_CD_FILE` write is the launcher-contract handshake, caller-owned |

If the apply-time audit confirms these verdicts, **no other command gains `--dry-run`** and the disposition is recorded (memory + a short audit section in the plan/requirements). If the audit finds a wt-owned destructive write, that path gets the same treatment as delete within this change.

## Affected Memory

- `wt-cli/delete-dry-run-contract`: (new) The `wt delete --dry-run` contract — live-path-sharing guarantee, per-mutation `Would …` preview lines, consent/prompt interplay (confirmations skipped, hazards reported, selection retained), stdout placement, exit codes, and the destructive-path audit dispositions for the other commands.
- `wt-cli/toolkit-standards-conformance`: (modify) Flip the principle №5 verdict from "gap — deferred to [p5m9]" to conformant for `wt delete`; record the audit dispositions for the remaining commands.
- `wt-cli/flag-naming-conventions`: (modify) Record `--dry-run` (long-only, no short flag) on the flag surface.

## Impact

- `src/cmd/wt/delete.go` — flag registration, dry-run threading through all six `handle*` paths, preview lines at the mutation seams (the restructuring-sized core).
- `src/cmd/wt/delete_test.go` — unit coverage: preview-per-path, flag composition (`--stash`, `--stale`, `--all`, `--branch` tri-state, `--no-remote`), no-mutation guarantee, prompt suppression.
- `src/cmd/wt/integration_test.go` — end-to-end: dry-run against a real repo leaves worktrees/branches/stash state byte-identical; exit codes.
- `src/internal/worktree/` — only if the plan opts to expose mutation argv/plan builders there (constitution V keeps orchestration in `cmd/`); hazard/decision functions are reused as-is.
- `docs/specs/cli-surface.md` — `wt delete` flag table gains the `--dry-run` row.
- Conformance re-check per constitution's Toolkit Standards article: CLI-surface change → re-verify help-dump checklist; README needs no change (its command table carries no per-flag detail).
- `fab/backlog.md` — mark `[p5m9]` done at archive time (existing convention).

## Open Questions

None — the backlog entry, principle №5's text and reference implementation, and the current code give a complete picture; remaining choices are graded as assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--dry-run` applies uniformly to all six target-resolution paths (`--stale`, `--all`, positionals, deprecated `--worktree-name`, current-worktree, menu) | Principle №5 covers the command's destructive writes wholesale; a path-partial preview would be a trap | S:85 R:75 A:85 D:85 |
| 2 | Confident | Preview gates at the leaf-mutation seam with all decision logic executing live; no parallel preview derivation (exact mechanism — threaded flag vs. options struct — decided at plan time) | The standard's receipt (`brewUninstallArgv` threads into both) prescribes single-sourcing; "a dry-run that drifts from the live path is worse than none" | S:85 R:60 A:85 D:80 |
| 3 | Confident | `--dry-run` skips all confirmation prompts; hazard prompts (uncommitted/unpushed) become preview report lines using the same detection functions | shll reference: "`--dry-run` requiring no consent at all"; reporting hazards is the preview's core value | S:75 R:70 A:80 D:70 |
| 4 | Confident | Target-selection menu still shows under `--dry-run` on a TTY with no target; selection is not consent; non-interactive no-target refusal unchanged | Selection picks what to preview; alternatives (refuse, or preview-all) contradict either №1's reconciliation or least surprise | S:55 R:80 A:65 D:55 |
| 5 | Confident | Preview lines print to stdout as the machine result; `[ohwb]` stream realignment of existing live chatter stays out of scope | Principle №2 + repo stdout/stderr convention (stdout = machine result); ohwb is a separately tracked deferral | S:65 R:70 A:80 D:70 |
| 6 | Confident | Preview format = `Would …` action lines mirroring live completion messages one-for-one, plus a `Dry run — no changes will be made.` header | Mirroring live messages keeps preview↔live drift visible in review; argv-dump and JSON formats rejected as heavier than the standard requires | S:55 R:80 A:70 D:50 |
| 7 | Confident | `--stash` × `--dry-run` previews the stash action (`Would stash uncommitted changes`) without stashing | Stash is itself a mutation; previewing it preserves flag composition semantics | S:70 R:80 A:80 D:75 |
| 8 | Certain | No short flag for `--dry-run` | Constitution II: short flags only for common interactive use; precedent `--non-interactive`; `-d` risks delete-adjacent misfire | S:60 R:90 A:85 D:80 |
| 9 | Certain | Exit codes unchanged: preview exits 0; validation errors keep existing codes; dry-run fails on exactly the inputs the live run fails on | Constitution III typed exit codes; principle №6 retry-safety expects identical validation | S:75 R:90 A:90 D:90 |
| 10 | Confident | Audit disposition: create/init/open/update/go carry no wt-owned destructive writes → no additional `--dry-run` flags expected; verdicts recorded in conformance memory | Intake-time code survey (rollback self-scope, delegated init/brew, cache-only RemoveAll); apply verifies and records | S:70 R:75 A:75 D:65 |
| 11 | Confident | Docs delta = `cli-surface.md` delete flag row + help-dump checklist re-verification; README untouched | Conformance memory's re-audit trigger governs CLI-surface changes; README carries no per-flag tables | S:70 R:85 A:80 D:80 |

11 assumptions (3 certain, 8 confident, 0 tentative, 0 unresolved).
