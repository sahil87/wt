# Intake: Recorded base — `branch.<name>.wt-base[-sha]` on create; `base`/`base_sha` in `wt list --json`

**Change**: 261123-o7jg-create-recorded-base-list-json
**Created**: 2026-11-23

## Origin

> Execute step W2 (Recorded base: branch.<name>.wt-base[-sha] on create; base/base_sha in wt list --json) of the plan at /home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md. Read the whole plan first (Decisions and Rules bind you), then implement only the W2 Brief.

One-shot `/fab-new` invocation by the plan operator. The plan (`fab/plans/sahil/26-11-23-worktree-base-receipts.md`) was read in full; its **Decisions already taken** table and **Rules for the operator** bind this change and are not reopened. Only the **W2 Brief** is in scope. W2 depends on W1 (`261123-xhgg-create-fetch-json-receipt` — `--fetch`, SHA pinning, `--json` receipt), which is already committed on this branch and is this change's base branch; W2 builds directly on W1's pinned `baseSHA` and `baseRefLabel` in `src/cmd/wt/create.go`. W3 (`--reuse`+`--base` error), W4 (`--pr`), W5 (`--temp`) are separate changes.

Plan decisions that bind W2 (verbatim intent):

- **Recorded base**: git config on the branch — `branch.<name>.wt-base <ref>` and `branch.<name>.wt-base-sha <sha>` — written **only when `wt` created the branch** (positional / bare / `--base`); **never** on `--checkout` or `--reuse` (and later `--temp`). Rationale: travels with the repo, no hidden state (Constitution I); derivable by any later tool with `git config`.
- **Back-compat**: every change additive; no exit-code changes (Constitution III).
- **Rules**: git operations and config writes in `src/internal/worktree/`, flag parsing/output in `src/cmd/wt/` (Constitution II/V); unit tests per path + an integration case in `src/cmd/wt/integration_test.go` against a real temp repo (Constitution IV); CLI ⇒ docs in the same change (`cli-surface.md`, skill bundle via `just sync-skill`, help-dump fixtures if affected, touched memory contracts each with a Design Decision citing the plan).
- **Review bar (must-fix)**: a worktree or branch left behind on failure, a wrong base, **a config written for a branch `wt` did not create**, a changed exit code, stdout polluted under `--json`.

W2 Brief (verbatim):

> - When `wt` creates the branch (positional, bare, with or without `--base`), write `git config branch.<branch>.wt-base <ref-as-typed-or-HEAD-label>` and `branch.<branch>.wt-base-sha <sha>` after a successful `worktree add`, inside the rollback scope (a failed config write rolls back like any other create failure). `--checkout`, `--reuse` and `--temp` write nothing.
> - `wt list --json` records gain `base` and `base_sha` (null when unset — every pre-W2 worktree); `wt list --status` is unchanged (no new column in v1).
> - Tests: config present after create, absent after checkout; list reflects it; rollback removes the branch (and so the config).
> - Docs: `cli-surface.md` § `wt list`, `list-status-contract.md`, `worktree-layout.md` (branch ↔ worktree relationship gains the base), skill bundle, help-dump.

## Why

Nothing records what a worktree's branch was cut from. Downstream, the PR base has to be told twice — `fab status set-base-branch`, or a prompt's "open a draft PR against `<base>`" — and no later tool (hexokit's engine, a respawned agent, `fab`) can derive it from the repo. W1's `--json` receipt hands the base to the *immediate* caller, but it is ephemeral: once the caller's process is gone, so is the base.

If not fixed, every caller keeps its own copy of the base (hexokit's engine already keeps stale copies of branch names), and a tool picking up an existing worktree has to guess the PR base.

Why git config on the branch (plan decision, not reopened): it lives in the repository (Constitution I — no hidden state file), travels with the branch name (renames via `git branch -m` move the `branch.<name>` section; `git branch -D` removes it), and is readable by any tool with plain `git config branch.<b>.wt-base`. Alternatives rejected by the plan: a `wt` state file (Constitution I violation, plan Non-goal); a git note or ref (not keyed by branch, harder to read).

## What Changes

### 1. Record the base on branch creation (`src/internal/worktree/`)

Config keys (named constants in `internal/worktree`, e.g. in `git.go` or a new `base.go`):

```
branch.<branch>.wt-base      = <ref as typed to --base, or the HEAD label when no --base>
branch.<branch>.wt-base-sha  = <full 40-hex SHA of the pinned start-point>
```

Example after `wt create --name mp-f1 --base origin/eng-v2 --fetch`:

```sh
$ git config branch.mp-f1.wt-base
origin/eng-v2
$ git config branch.mp-f1.wt-base-sha
f68be281ad0c…(40 hex)
```

Example after a bare `wt create` while on `main`: `wt-base = main`, `wt-base-sha = <HEAD sha>`. When the invoking worktree's HEAD is detached, `wt-base` is the short SHA `wt.DescribeHead()` returns (the same label the `From:` line and the W1 receipt's `base_ref` use) and `wt-base-sha` is the full SHA.

- **Values**: exactly W1's receipt values — `wt-base` = `baseRefLabel` (the `--base` ref verbatim, or `wt.DescribeHead()`), `wt-base-sha` = `baseSHA` (the pinned start-point W1 resolves pre-dispatch). The recorded base, the receipt's `base_ref`/`base_sha`, and the `From:` line therefore always agree.
- **When**: inside `CreateNewBranchWorktree` and `CreateExploratoryWorktree`, **after** the successful `git worktree add -b` and after W1's `trackUpstream` step — i.e. inside the existing rollback scope (`registerNewBranchRollback` has already registered `worktree remove --force` + `branch -D`).
- **How**: two `git config branch.<branch>.wt-base <ref>` / `git config branch.<branch>.wt-base-sha <sha>` invocations (argv exec, no shell; repo-level config — branch config is shared across all worktrees of the repo, so it is written from the repo's cwd like `SetUpstream`). Never `--global`.
- **Plumbing**: the two create functions gain the base ref label (the start-point SHA is already their `startPoint` parameter). Shape is the implementer's choice — e.g. a `baseRef string` parameter alongside `startPoint`/`upstream`, or a small `NewBranchBase{Ref, SHA, Upstream}` struct replacing the three loose strings — following the existing `trackUpstream` pattern: a helper (e.g. `recordBase(branch, ref, sha string, rb *Rollback) error`) that writes both keys and, on failure, calls `rb.Execute(); rb.Disarm()` and returns a typed error. When `startPoint` is empty (a non-`wt create` caller / test that passes no start-point), nothing is recorded — `wt create` always passes a pinned SHA for new branches since W1.
- **Failure**: a failed config write (e.g. a stale `.git/config.lock`) rolls back the worktree and branch exactly like a set-upstream failure, and `create.go` maps it to `ExitGitError` (3) with structured copy, mirroring `failIfSetUpstream`:
  - `Error: Failed to record base for branch '<branch>'`
  - `Why: <first line of git config output>`
  - `Fix: Check that .git/config is writable (no stale config.lock), then retry`
  Under `--json` this is the usual failure document `{"ok":false,"error":{"code":3,"message":"Failed to record base for branch '<branch>'"}}`. Use a typed error (e.g. `*RecordBaseError{Branch, Output}`) detected via `errors.As` in create.go — no new exit code.
- **Rollback removes the config**: `git branch -D <branch>` deletes the branch's whole `branch.<branch>` config section, so a rollback after a partial or full config write leaves no `wt-base*` keys behind. A test asserts this (see §4).
- **Never written** for: `--checkout` (existing branch — `CheckoutBranchWorktree` is untouched), `--reuse` (no create function runs), and future `--temp` (W5; no branch). This is the review bar's "config written for a branch `wt` did not create" guard.
- **Signal/reinstall window**: the writes are git subprocesses inside the create functions, exactly like W1's `SetUpstream` — they run before the init-phase `signal.Reset` under the rollback handler; no new stdout/stderr output is emitted, so the deferred-summary / reinstall-window contract (`create-output-phases.md`) is unaffected.
- **No change** to the receipt (`wt create --json`), the `From:` line, or any stdout/stderr copy on success.

### 2. `wt list --json` gains `base` and `base_sha` (`src/cmd/wt/list.go` + an `internal/worktree` reader)

`listEntry` gains two always-present nullable keys:

```go
Base    *string `json:"base"`     // branch.<branch>.wt-base, null when unset
BaseSHA *string `json:"base_sha"` // branch.<branch>.wt-base-sha, null when unset
```

No `omitempty` — the keys are present in every record (null for every pre-W2 worktree, the main worktree on an un-recorded branch, a `--checkout` worktree, and a detached worktree). Example:

```json
[
  {"name":"main","branch":"main","path":"/r/repo","is_main":true,"is_current":true,"base":null,"base_sha":null},
  {"name":"mp-f1","branch":"mp-f1","path":"/r/repo.worktrees/mp-f1","is_main":false,"is_current":false,"base":"origin/eng-v2","base_sha":"f68be281ad0c…"}
]
```

- **Reading**: one constant-cost read for all worktrees — a single `git config --get-regexp '^branch\..*\.wt-base(-sha)?$'` (e.g. with `-z` / `--null` for robust parsing), parsed into a `map[branch]{ref, sha}` by stripping the `branch.` prefix and the `.wt-base` / `.wt-base-sha` suffix (branch names may contain dots and slashes; the suffix strip from the right handles that). Exit 1 (no matches) and any other failure ⇒ empty map (best-effort, non-actionable for a list command — every record gets nulls). The reader lives in `internal/worktree` (Constitution V), e.g. `RecordedBases() map[string]RecordedBase`; list.go joins it onto entries by `Branch` (skip `(detached)`).
- **When read**: only on the `--json` path (with or without `--status`). Human output (`wt list`, `wt list --status`) and `--path` do not read it and are unchanged — no new column in v1 (plan's preferred answer to the W2 open decision).
- **Subprocess contract amendment** (`list-status-contract.md`): today default `wt list` "(no flags, or with `--json` only) SHALL invoke exactly **one** `git` subprocess". New rule: default human `wt list` stays at exactly one; `wt list --json` invokes exactly **two** — `git worktree list --porcelain` plus the single `git config --get-regexp` — constant regardless of worktree count, still never any per-worktree subprocess. The contract's purpose (no O(N) per-worktree git calls on the default path) is preserved.
- Sort, `is_current`, `--status` pointer fields, `--path`/`--json` mutual exclusion: all unchanged.

### 3. `create.go` wiring (`src/cmd/wt/create.go`)

- Pass `baseRefLabel` (already computed pre-dispatch by W1) into `CreateNewBranchWorktree` / `CreateExploratoryWorktree` alongside `baseSHA`/`baseUpstream`.
- Add the record-base failure mapping next to `failIfSetUpstream` (exit 3, copy in §1) for both new-branch dispatch arms.
- `--checkout` and `--reuse` arms unchanged.

### 4. Tests (Constitution IV)

- **Unit** (`src/internal/worktree/crud_test.go` / `git_test.go` or a new `base_test.go`): new-branch create writes both keys with the given ref/SHA; exploratory create writes both keys; `CheckoutBranchWorktree` writes nothing (local and remote-only); empty start-point writes nothing; a forced config-write failure (e.g. a pre-created `.git/config.lock`) rolls back — no worktree dir, no branch, no `wt-base*` keys — and returns the typed error; the reader parses multiple branches including a branch name with dots and slashes (`feat/v1.2`), ignores unrelated `branch.*` keys, and returns an empty map when no keys exist.
- **cmd** (`src/cmd/wt/create_test.go`, `list_test.go`): `wt create` positional + `--base` → config present with the ref as typed and the pinned SHA; bare create → `wt-base` = HEAD label; `--checkout` → config absent; `--reuse` → nothing written; `wt list --json` records carry `base`/`base_sha` for a created worktree and `null` (key present) for main/checkout/pre-existing worktrees; human `wt list` / `--status` output unchanged.
- **Integration** (`src/cmd/wt/integration_test.go`, real temp repo — bare origin + clone): `wt create --name w2 --base origin/main --json` then `git config branch.w2.wt-base` = `origin/main`, `wt-base-sha` = receipt `base_sha`; `wt list --json` shows the same `base`/`base_sha`; `wt create --checkout <existing>` leaves no `wt-base` keys; a rollback (e.g. the config-write failure, or the existing failure-injection used by W1's set-upstream rollback test) leaves neither branch nor config.

### 5. Docs (CLI ⇒ docs, same change)

- `docs/specs/cli-surface.md`: § `wt create` — new-branch modes record `branch.<b>.wt-base`/`wt-base-sha` (never for `--checkout`/`--reuse`), record failure = exit 3 with rollback; § `wt list` — `--json` records include `base`/`base_sha` (null when unset).
- `docs/specs/worktree-layout.md` § Branch ↔ worktree relationship: a `wt`-created branch carries its recorded base in git config; existing-branch (`--checkout`) worktrees do not; rename/delete semantics follow git's `branch.<name>` section handling.
- `docs/memory/wt-cli/list-status-contract.md`: `base`/`base_sha` always-present nullable keys on the JSON path; the subprocess-count amendment (human: 1; `--json`: 2, constant); Design Decision citing the plan (W2) — JSON-only in v1, no `--status` column.
- `docs/memory/wt-cli/create-branch-semantics.md`: recorded-base row/section (written only for wt-created branches, values = W1 receipt values, rollback scope, exit 3 on write failure); Design Decision citing the plan (W2).
- `wt skill` bundle: edit `docs/site/skill.md` (canonical) — the Machine-surface bullet notes `list --json` `base`/`base_sha` and the `branch.<b>.wt-base` config — then `just sync-skill` to refresh `src/cmd/wt/skill.md`.
- `wt help-dump`: no flag is added or renamed, so no help-dump fixture is expected to change; verify `help_dump_test.go` still passes. If the `wt list --json` flag description is touched (it currently says "Output worktree data as a JSON array" — leave as-is unless a test pins the record shape there), update accordingly.

## Affected Memory

- `wt-cli/create-branch-semantics`: (modify) recorded base — `branch.<b>.wt-base`/`wt-base-sha` written for wt-created branches only, inside rollback scope, exit 3 on write failure; never for `--checkout`/`--reuse`
- `wt-cli/list-status-contract`: (modify) `base`/`base_sha` always-present nullable keys in `--json` records; subprocess-count rule amended (human default 1, `--json` 2 constant); no `--status` column in v1
- `wt-cli/create-output-phases`: (modify) only if it enumerates the git subprocesses inside the reinstall window/rollback scope — note the config writes run alongside set-upstream before `signal.Reset`

## Impact

- **Code**: `src/internal/worktree/crud.go` (record-base step in both new-branch create functions + signature change), `git.go` or new `base.go` (key constants, `recordBase` writer, typed error, `RecordedBases` reader); `src/cmd/wt/create.go` (pass `baseRefLabel`, map the new typed error to exit 3); `src/cmd/wt/list.go` (`listEntry` fields, join on the JSON path).
- **Callers of the changed create functions**: every call site of `CreateNewBranchWorktree` / `CreateExploratoryWorktree` (create.go and tests) updates for the new parameter.
- **Tests**: `crud_test.go`, `git_test.go` (or `base_test.go`), `create_test.go`, `list_test.go`, `integration_test.go`.
- **Docs**: `docs/specs/cli-surface.md`, `docs/specs/worktree-layout.md`, `docs/site/skill.md` + synced `src/cmd/wt/skill.md`, memory files above.
- **Downstream**: additive. `wt list --json` consumers (e.g. `hop ls --trees`) get two extra keys; JSON consumers ignoring unknown keys are unaffected. hexokit / fab can later derive a PR base from `git config branch.<b>.wt-base` (plan Downstream, not a step here).
- **No new exit codes, no new flags, no persistent state outside the repo** (git config is the repository's own; never `--global`).

## Open Questions

- None blocking. The plan's W2 open decision (`wt list --status` BASE column) is resolved in Assumption 2.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Keys `branch.<b>.wt-base` / `branch.<b>.wt-base-sha` written only for wt-created branches (positional, bare, with/without `--base`); never `--checkout`/`--reuse`/`--temp` | Plan Decisions table + W2 Brief, verbatim | S:95 R:70 A:95 D:95 |
| 2 | Certain | No `BASE` column in `wt list --status` (JSON only in v1) | Plan's stated preference for the W2 open decision; Brief says `--status` unchanged | S:90 R:90 A:90 D:90 |
| 3 | Certain | Recorded values = W1's `baseRefLabel` (ref as typed, or `wt.DescribeHead()` label) and pinned `baseSHA` | Brief "ref-as-typed-or-HEAD-label"; W1 Assumption 10 chose the same label so receipt, `From:` and config agree | S:90 R:80 A:90 D:85 |
| 4 | Confident | Config write failure rolls back worktree+branch and exits `ExitGitError` (3) with `Failed to record base for branch '<b>'` copy, via a typed error mirroring `SetUpstreamError` | Brief: "rolls back like any other create failure"; exit 3 matches W1's post-add set-upstream failure; plan forbids new codes | S:75 R:80 A:85 D:75 |
| 5 | Confident | `base`/`base_sha` are always-present nullable keys (no `omitempty`) on every `wt list --json` record, with or without `--status` | Brief "null when unset — every pre-W2 worktree"; null-not-absent mirrors the `--status` keys-present contract | S:80 R:75 A:85 D:75 |
| 6 | Confident | `wt list --json` reads all recorded bases with one `git config --get-regexp` call (2 subprocesses total, constant); human `wt list` stays at exactly 1; contract amended in `list-status-contract.md` | The one-subprocess rule exists to forbid per-worktree O(N) calls; a single constant read preserves that and git config has no batched read alongside `worktree list` | S:60 R:85 A:75 D:65 |
| 7 | Confident | Reader failures (no matches, git error) degrade to nulls, never an error exit | List is a read-only dashboard; existing list contract treats per-worktree failures as non-actionable | S:70 R:90 A:80 D:75 |
| 8 | Confident | Rollback relies on `git branch -D` removing the `branch.<b>` config section (no explicit `config --unset` registered); a test asserts it | git deletes the branch's config section on branch deletion; Brief: "rollback removes the branch (and so the config)" | S:80 R:85 A:80 D:80 |
| 9 | Confident | Nothing is recorded when the create functions receive an empty start-point | Since W1 `wt create` always pins a SHA for new branches; guards non-create callers/tests from writing a half record | S:60 R:90 A:75 D:70 |
| 10 | Confident | No help-dump fixture change expected (no flag added/renamed); skill bundle Machine-surface bullet updated via `just sync-skill` | Brief lists help-dump, but help-dump pins the flag tree which W2 does not touch | S:65 R:90 A:80 D:75 |
| 11 | Certain | Receipt (`wt create --json`), `From:` line and all success stdout/stderr copy unchanged | Brief adds config + list fields only; plan Back-compat | S:85 R:85 A:90 D:90 |

11 assumptions (4 certain, 7 confident, 0 tentative, 0 unresolved).
