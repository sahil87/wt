---
type: memory
description: "The `wt delete --dry-run` contract — a live-path-sharing preview: decisions run live, only leaf mutations are gated at the seam as `Would …` stdout lines under a `Dry run` header; confirmations skipped, hazards reported via the same detection functions, selection menu retained, non-interactive refusal + exit codes unchanged; plus the destructive-path audit (delete is wt's only wt-owned destructive write)."
---
# wt-cli: Delete Dry-Run Contract

**Domain**: wt-cli

> Post-implementation behavior capture for the `wt delete --dry-run` preview
> flag, the live-path-sharing guarantee it rests on, and the destructive-path
> audit that dispositioned every other `wt` command.
> Source change: `260717-p5m9-delete-dry-run`.

## Overview

`wt delete` is the toolkit's most destructive command — it force-removes
worktree directories, deletes local and remote branches, and can discard or
stash uncommitted work. This file documents the `--dry-run` flag that lets a
caller (agent or human) see exactly what a given invocation *would* do without
mutating anything, and the design that keeps that preview from ever drifting
from the live run: every selection and decision path executes live, only the
leaf mutations are gated. It also records the destructive-path audit that
confirmed `wt delete` is the only `wt` command with wt-owned destructive writes,
so no other command needs `--dry-run`.

This is the change that flips toolkit principle №5 (visible mutation boundaries)
to conformant for `wt delete` — see
[toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md). The
flag's long-only, no-short shape is recorded in
[flag-naming-conventions](/wt-cli/flag-naming-conventions.md).

## Requirements

### The `--dry-run` flag: long-only boolean, all six paths

`wt delete` registers a `--dry-run` boolean flag (no short) in `deleteCmd()`
(`src/cmd/wt/delete.go`) that previews what a real invocation would do without
performing any mutation and without prompting for confirmation (R1). It is
honored across **every** target-resolution path in the existing resolution order
— `--stale` → `--all` → positional names → deprecated `--worktree-name` →
current-worktree → interactive menu (R2). No short flag: `--dry-run` is a
preview/automation flag, not common interactive use (constitution II; precedent
`--non-interactive`; `-d` would risk a delete-adjacent misfire) — see
[flag-naming-conventions](/wt-cli/flag-naming-conventions.md) § short-flags rule.

#### Scenario: dry-run previews without mutating
- **GIVEN** the `wt delete` command
- **WHEN** a user runs `wt delete <target> --dry-run`
- **THEN** the command prints a preview of what would be deleted and exits
  without removing worktrees, deleting branches, or stashing/discarding changes

### Live-path-sharing: decision logic live, only leaf mutations gated

Under `--dry-run`, every selection and decision path SHALL execute live —
target resolution (including `--stale` idle computation via `IsIdle`/`RecencyOf`
— see [idle-staleness-contract](/wt-cli/idle-staleness-contract.md)), the
`--branch` tri-state auto rule (`branch == wtName`), `BranchExistsRemotely()`
remote checks, `HasUncommittedChanges()`/`HasUntrackedFiles()`/
`HasUnpushedCommits()`/`GetUnpushedCount()` hazard detection, and the orphan
`wt/<name>` check. Only the leaf mutations SHALL be suppressed, gated at the
mutation seam so live and preview cannot drift (R3). **No parallel preview
derivation is permitted** — this is the standard's core guarantee ("a dry-run
that drifts from the live path is worse than none"), single-sourced exactly as
shll's `brewUninstallArgv` threads into both live and preview.

The gated mutation leaves, each replaced by exactly one `Would …` line (R4):

| Mutation leaf | Live call | Dry-run preview line |
|---------------|-----------|----------------------|
| Worktree removal | `wt.RemoveWorktree(path, true)` | `Would remove worktree: <name>` |
| Local branch delete | `wt.DeleteLocalBranch(branch, true)` | `Would delete branch: <b> (local)` |
| Remote branch delete | `wt.DeleteRemoteBranch(branch)` | `Would delete branch: <b> (remote)` |
| Orphan `wt/<name>` cleanup | same two branch commands | same two lines for `wt/<name>` |
| Stash | `wt.StashCreate(...)` / `handleStashInDir` | `Would stash uncommitted changes` |
| Discard (implicit) | carried by force removal | `Would discard uncommitted changes (use --stash to preserve them)` |

The `Would …` wording mirrors the live path's completion messages one-for-one
(`Deleted worktree` → `Would remove worktree`, `Deleted branch: X (local)` →
`Would delete branch: X (local)`), so preview↔live drift stays visible in
review — this is a single-sourced intent, not ad-hoc duplicated copy.

Constitution V holds: the decision/hazard functions (`IsIdle`, `RecencyOf`,
`BranchExistsRemotely`, `BranchExistsLocally`, `HasUncommittedChanges`,
`GetUnpushedCount`) all live in `internal/worktree` and are reused as-is; the
dry-run gating is orchestration and stays in `cmd/`.

#### Scenario: each suppressed mutation prints exactly one preview line
- **GIVEN** a worktree whose branch matches its name and exists on origin, with
  uncommitted changes
- **WHEN** `wt delete <name> --dry-run` runs
- **THEN** the branch-match decision, remote-existence check, and hazard
  detection all run against real git state, and each suppressed mutation
  (worktree removal, local branch delete, remote branch delete, discard)
  produces exactly one `Would …` line — nothing is mutated

### The `os.Chdir` skip

The `os.Chdir` to the main repo — a prerequisite of live `RemoveWorktree` (you
cannot remove the worktree you are standing in) — SHALL be skipped under
`--dry-run` on every path that performs it (`handleDeleteCurrent`,
`handleDeleteMultiple`, `handleDeleteAll`, all gated on `IsWorktree() &&
!dryRun`), so the invocation changes neither filesystem nor process state
(constitution I / principle №5). The "You are no longer in a valid directory."
trailer in `handleDeleteCurrent` is likewise suppressed under dry-run — it is
false under a preview (nothing was removed, the caller is still in a valid dir).

### Orphan `wt/<name>` handling: local gated on local existence, remote independent

The orphan `wt/<name>` cleanup in `handleBranchCleanup` always runs regardless
of the `--branch` decision. Under `--dry-run`, its local and remote preview
lines are gated **separately** to mirror the live structure exactly (both
`BranchExistsLocally` and `BranchExistsRemotely` are read-only):

- the local `Would delete branch: wt/<name> (local)` line prints only when the
  orphan exists **locally** (`BranchExistsLocally(wtOriginBranch)`);
- the remote `Would delete branch: wt/<name> (remote)` line prints whenever
  `deleteRemote == "true" && BranchExistsRemotely(wtOriginBranch)` — **independent
  of local existence**.

This matches the live path, which deletes the local orphan (`DeleteLocalBranch`
no-ops when absent) and, independently, the remote orphan when it exists on
origin. The independence is load-bearing: for a remote-only `refs/heads/wt/<name>`
(no local counterpart), the live run deletes the remote orphan while a
local-nested preview would print nothing — a real preview↔live drift. Gating the
two lines separately closes it.

#### Scenario: remote-only orphan previews the remote line
- **GIVEN** a `wt/<name>` orphan branch that exists only on origin (no local
  branch), under `--dry-run` with remote deletion enabled
- **WHEN** the delete flow reaches orphan cleanup
- **THEN** `Would delete branch: wt/<name> (remote)` is printed and no local line
  is printed — matching what the live run would delete
  (`TestDelete_DryRunRemoteOnlyOrphanPreviewsRemote`)

### Consent and prompt interplay

Per principle №5 + №1 (and the shll reference "`--dry-run` requiring no consent
at all"):

- **Confirmations are skipped.** `--dry-run` skips every delete-confirmation
  prompt ("Delete this worktree?", "Delete these N worktrees?", "Delete ALL …?")
  — gated on `!nonInteractive && !dryRun` at each prompt site (R5).
- **Hazards become report lines, not prompts.** Where the live interactive flow
  prompts on uncommitted changes or unpushed commits, `--dry-run` instead reports
  what the equivalent consented run would do, using the **same live detection
  functions** (R6):
  - uncommitted changes with `--stash` → `Would stash uncommitted changes`;
  - uncommitted changes without `--stash` → `Would discard uncommitted changes
    (use --stash to preserve them)`;
  - unpushed commits → `Would lose N unpushed commit(s) on branch <b>`, with `N`
    from the live `GetUnpushedCount`. The unpushed report fires **regardless of
    `--non-interactive`** — a preview's value is the information it surfaces.
- **The selection menu is not consent and still shows.** The interactive
  target-selection menu still shows under `--dry-run` when invoked with no target
  on a TTY (`wt delete --dry-run` from the main repo); selection picks *what* to
  preview, and the picked target routes into the preview. The row labels/flow are
  unchanged (R7).
- **Non-interactive no-target refusal is unchanged.** `wt delete --dry-run
  --non-interactive` (or a non-TTY) with no target keeps today's `ExitInvalidArgs`
  refusal ("No worktree specified") (R7).
- **`--dry-run` composes with every existing flag.** `--stash` previews the stash
  action instead of performing it; `--stale`/`--all`/positionals preview the
  computed target set; `--branch`/`--no-remote` alter the previewed branch actions
  exactly as they alter the live run (the tri-state auto rule and remote-existence
  check run live either way).

#### Scenario: hazard prompt becomes a report line
- **GIVEN** a dirty worktree under `--dry-run` without `--stash`
- **WHEN** the delete flow runs
- **THEN** `Would discard uncommitted changes (use --stash to preserve them)` is
  printed and nothing is discarded; with `--stash`, `Would stash uncommitted
  changes` is printed and nothing is stashed

### Output contract: stdout, `Would …` lines, `Dry run` header

Preview `Would …` lines print to **stdout** — the preview is the machine result
the caller asked for (principle №2 / repo stdout=machine convention). A `Dry run
— no changes will be made.` header (via `printDryRunHeader`) precedes the
per-mutation lines and prints **once per invocation** (R8). Multi-target forms
(`--all`, `--stale`, positionals) keep their per-worktree block structure with
the same `Would …` lines per block; `--stale` with zero matches keeps its live
`No idle worktrees (threshold: Nd).` empty-state (R8). The `[ohwb]` realignment
of the *existing* live-path human chatter to stderr is a separately tracked
deferral and is **out of scope** here — this change adds only the new preview
copy on stdout.

Example (single dirty worktree whose branch matches its name and exists on
origin):

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

### Exit-code parity: dry-run fails on exactly the inputs the live run fails on

A successful preview exits `ExitSuccess` (0). Argument validation keeps its
existing behavior — the `--stale`↔positional and `--stale`↔`--all` mutexes
(`ExitInvalidArgs`, "mutually exclusive"), unknown worktree names
(`ExitGeneralError`), positional×`--worktree-name` mix, an invalid `--stale`
threshold, and an outside-git-repo invocation all keep their existing
`ExitInvalidArgs`/`ExitGitError`/`ExitGeneralError` codes (R9, constitution III).
Dry-run **fails on exactly the inputs the live run fails on** — the mutex and
validation checks run in `RunE` before any dry-run branching, so no dry-run path
can silently accept an input the live run rejects (principle №6 retry-safety).

#### Scenario: validation errors are identical under dry-run
- **GIVEN** `wt delete --stale <name> --dry-run`
- **WHEN** the command runs
- **THEN** it exits `ExitInvalidArgs` ("mutually exclusive") exactly as the
  non-dry-run form does
- **GIVEN** `wt delete <unknown> --dry-run`
- **THEN** it exits `ExitGeneralError` ("not found") exactly as the non-dry-run
  form does

### Help text

The `wt delete` `Long` help mentions `--dry-run` (R10); `wt help-dump` picks the
flag up automatically from the cobra tree, keeping the JSON envelope valid with
the flag visible on the delete node
(`TestHelpDump_EmitsValidEnvelope`) — see
[help-dump-contract](/wt-cli/help-dump-contract.md).

### Destructive-path audit: only `wt delete` has wt-owned destructive writes

The change's second half audited **every** `wt` command for wt-owned destructive
writes (principle №5) and recorded a per-command disposition (R11). The verdicts
were verified against source: `wt delete` is the only command with wt-owned
destructive writes, and it now supports `--dry-run`. **No other command requires
`--dry-run`.**

| Command | Verdict | Basis (verified in source) |
|---------|---------|----------------------------|
| `wt delete` | Destructive — now conformant | Gains `--dry-run` in this change (live-path-sharing preview, all six paths). |
| `wt create` | Additive, no `--dry-run` | `git worktree add -b` + `MkdirAll`; `rollback.go` `Register`s only its **own** partial creation (`git worktree remove --force <wtPath>`, `git branch -D <branch>`) and fires on failure/SIGINT — never touches pre-existing user data. |
| `wt create` init / `wt init` | Delegated, not wt-owned | Runs the user-configured init script; wt itself writes nothing destructive. |
| `wt create` open / `wt open` | Side-effecting, not destructive | Spawns windows/apps; `apps.go`'s `os.RemoveAll` targets the wt-owned byobu cache dir (`~/.cache/byobu/.last.tmux`), not user data. |
| `wt update` | Delegated to Homebrew | `internal/update` shells to `brew upgrade`/`brew update`; brew owns the mutation semantics. |
| `wt go` | Read-only + launcher handshake | Only write is `os.WriteFile(WT_CD_FILE, …)` — the caller-owned launcher-contract handshake (see [go-command-contract](/wt-cli/go-command-contract.md)), not user-data mutation. |
| `wt list` / `wt shell-init` | Read-only | No filesystem/git mutation. |

## Design Decisions

### Thread a `dryRun bool` through the `handle*` signatures
**Decision**: add `dryRun bool` as an additional parameter to
`handleDeleteCurrent`, `handleDeleteByName`, `handleDeleteMultiple`,
`handleDeleteAll`, `handleDeleteStale`, `handleDeleteMenu`, `handleBranchCleanup`,
`handleUncommittedChanges`, and `handleStashInDir`, gating each mutation leaf on
it in place.
**Why**: the `handle*` functions already thread five explicit params
(`nonInteractive`, `deleteBranch`, `deleteRemote`, `stashMode`, plus `session`/
`rb`); a sixth `dryRun` bool follows the established pattern with the least churn
and keeps the gating at the mutation seam.
**Rejected**: an options struct bundling the params (intake assumption #2 named
this as the alternative) — a larger refactor touching every call site, diverging
from the current explicit-param style with no offsetting readability gain at six
params.
*Introduced by*: `260717-p5m9-delete-dry-run`

### Gate at the leaf, print a `Would …` line mirroring the live completion message
**Decision**: each suppressed mutation prints exactly one stdout line whose
wording mirrors the live path's success message.
**Why**: one-for-one mirroring keeps preview↔live drift visible in review and
satisfies the standard's single-source guarantee.
**Rejected**: argv-dump / JSON preview formats (heavier than the standard
requires — intake assumption #6).
*Introduced by*: `260717-p5m9-delete-dry-run`

### Skip the `os.Chdir` to main repo under dry-run
**Decision**: the chdir to the main repo (and the "no longer in a valid
directory" trailer in `handleDeleteCurrent`) is skipped under `--dry-run`.
**Why**: the chdir is a prerequisite of live `RemoveWorktree` only; under dry-run
nothing is removed, so the invocation must leave process and filesystem state
untouched (constitution I / principle №5). The trailer is false under a preview.
**Rejected**: chdir-ing anyway (a real side effect on a preview).
*Introduced by*: `260717-p5m9-delete-dry-run`

### Orphan `wt/<name>` preview lines gated separately (local vs. remote)
**Decision**: under dry-run the orphan-cleanup local line is gated on
`BranchExistsLocally` and the remote line on `deleteRemote == "true" &&
BranchExistsRemotely`, independently — mirroring the live path's independent
local/remote deletion rather than nesting the remote check inside local
existence.
**Why**: a remote-only `wt/<name>` orphan is deleted by the live run but would be
missed by a local-nested preview — a real preview↔live drift (caught in review
cycle 1). Separate gating closes it.
**Rejected**: nesting the remote preview inside the local-existence check (the
drift bug the review found).
*Introduced by*: `260717-p5m9-delete-dry-run`

## Cross-references

- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md)
  — this change flips principle №5 to conformant for `wt delete` and records the
  audit dispositions above.
- Sibling memory: [flag-naming-conventions](/wt-cli/flag-naming-conventions.md)
  — `--dry-run` as long-only / no-short on the `wt delete` flag surface.
- Sibling memory: [idle-staleness-contract](/wt-cli/idle-staleness-contract.md)
  — the `--stale` idle computation (`IsIdle`/`RecencyOf`) that runs live under
  dry-run, and the `handleDeleteMultiple` convergence the preview reuses.
- Sibling memory: [help-dump-contract](/wt-cli/help-dump-contract.md) — the
  `help-dump` envelope that picks up `--dry-run` on the delete node.
- Spec doc: [`docs/specs/cli-surface.md`](../../specs/cli-surface.md) — the
  `wt delete` flag table's `--dry-run` row.
- Source: `src/cmd/wt/delete.go` — flag registration (`--dry-run` on
  `deleteCmd()`), the `dryRun` threading through all nine `handle*` functions,
  `printDryRunHeader`, the leaf gating at each mutation seam, the `os.Chdir` skip,
  and the separately-gated orphan `wt/<name>` previews (`handleBranchCleanup`).
- Source: `src/internal/worktree/` — reused decision/hazard functions
  (`RemoveWorktree`, `DeleteLocalBranch`, `DeleteRemoteBranch`,
  `BranchExistsLocally`, `BranchExistsRemotely`, `HasUncommittedChanges`,
  `HasUntrackedFiles`, `HasUnpushedCommits`, `GetUnpushedCount`, `IsIdle`,
  `RecencyOf`, `StashCreate`) — no parallel preview derivation.
- Tests: `src/cmd/wt/delete_test.go` (preview-per-path, `--stash`×`--dry-run`,
  `--branch` tri-state / `--no-remote` preview, hazard report lines, prompt
  suppression, no-mutation guarantee, exit-code parity,
  `TestDelete_DryRunRemoteOnlyOrphanPreviewsRemote`),
  `src/cmd/wt/integration_test.go` (`TestIntegration_DeleteDryRun_LeavesStateByteIdentical`
  and the multi-target byte-identical / exit-0 assertions).
- Constitution: Principle I (no side-effecting state under a preview — the
  `os.Chdir` skip), III (typed exit codes — parity with the live run), V (decision
  logic in `internal/worktree`, dry-run gating is orchestration in `cmd/`), VI
  (`--non-interactive` no-target refusal unchanged; preview scriptable on demand),
  and the **Toolkit Standards** article (principle №5 the flag satisfies).
- External: `shll standards principles` (№5, visible mutation boundaries — the
  MUST this flag satisfies); shll's `uninstall --dry-run` / `brewUninstallArgv`
  single-source receipt (the prescribed shape).
