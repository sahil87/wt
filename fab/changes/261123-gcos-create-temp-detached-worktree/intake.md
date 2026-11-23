# Intake: `wt create --temp <ref>` — a detached throwaway worktree

**Change**: 261123-gcos-create-temp-detached-worktree
**Created**: 2026-11-23

## Origin

> Execute step W5 (--temp <ref>: a detached throwaway worktree) of the plan at /home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md. Read the whole plan first (Decisions and Rules bind you), then implement only the W5 Brief.

One-shot `/fab-new`, dispatched by the hexokit operator executing the plan's step queue (`merge-auto`, no user gates). Step W5 of plan `26-11-23-worktree-base-receipts.md` (absolute path above — the apply agent MUST read its **Decisions already taken**, **Rules for the operator** and **W5 Brief** sections; they bind this change). W5 has no dependencies in the plan's DAG; W1 (`--fetch` + `--json` receipt), W3 (`--reuse`+`--base` conflict) and W4 (`--pr <n>`) are being executed **in parallel** in sibling worktrees (`wtr-w1`, `wtr-w3`, `wtr-w4`) and touch the same files (`src/cmd/wt/create.go`, `cli-surface.md`, `create-branch-semantics.md`, skill bundle). None of them is merged on `main` at intake time (`origin/main` = `22ae6fe`).

The W5 Brief, verbatim:

> - `wt create --temp <ref>`: `git worktree add --detach <path> <sha>` at the resolved `<ref>` (`--fetch` honoured), random name prefixed `tmp-` (`--name` overrides, prefix still applied), no branch, no config. `wt list --json` marks it `temp: true` (`branch` null); `wt list --status` shows `(detached)` in the branch column. `wt delete <name>` removes it without branch prompts. Exclusive with the positional, `--checkout`, `--reuse`, `--pr`.
> - Init: per the W5 intake decision — skipped by default with `--init` to opt in, or run as usual; whichever is chosen goes into `init-protocol.md`.
> - Tests: detached HEAD at the SHA; list marking; delete; conflicts exit 2.
> - Docs: `cli-surface.md`, `worktree-layout.md` (temp worktrees), `create-branch-semantics.md` (mode row), `init-protocol.md`, skill bundle, help-dump.

The plan's binding decision row: *"`--temp <ref>` — A detached worktree at `<ref>` (`git worktree add --detach`), random name with a `tmp-` prefix, no branch created, listed by `wt list` with `temp: true`; removable with `wt delete`. Why: PR surgery wants a worktree that cannot pollute a branch and is obviously disposable."* The open decision *"Init on `--temp`: skip by default vs run"* is resolved at this intake (see § Init below — **skip by default, `--init` opts in**).

## Why

**Problem.** PR surgery — resolving a merge conflict, restacking a PR, inspecting a commit — wants a throwaway worktree at an exact commit that cannot pollute any branch. Today `wt` has no way to make one: every `wt create` mode either creates a branch (bare / positional) or checks out an existing one (`--checkout`). Callers therefore hand-roll `git worktree add --detach <path> <sha>` plus manual cleanup — in two different codebases (the hexokit operator's merging rules and agents spawning by hand), each slightly differently, each outside `wt`'s layout (`<repo>.worktrees/`), naming, rollback and listing.

**Consequence of not fixing.** Detached worktrees keep being created outside `wt`'s conventions: they land in arbitrary paths, are not obviously disposable, leak when a script fails half-way (no rollback), and `wt delete` mishandles them today — `handleBranchCleanup` receives the sentinel `(detached)` (or `HEAD` on the current-worktree path, from `git rev-parse --abbrev-ref HEAD`) as if it were a branch name and prints a bogus `Skipped branch deletion: branch '(detached)' does not match worktree name …` (or, with `--branch true`, attempts `git branch -D HEAD`).

**Why this approach.** The plan's Goal is that `wt create` is *the one way a worktree gets made*. A `--temp <ref>` mode reuses everything `wt create` already has — the worktrees dir, random naming, the rollback, the phase-separated output, the path-as-last-stdout-line contract, typed exit codes — and adds only the detached add. The `tmp-` prefix makes disposability visible in every listing and lets `wt list` mark temp worktrees with **zero extra git subprocesses** (the porcelain already reports `detached`; the name is the basename), preserving `list-status-contract.md`'s single-subprocess default. No state file, no git config (Constitution I).

## What Changes

### 1. `wt create --temp <ref>` (new flag, `src/cmd/wt/create.go` + `src/internal/worktree/crud.go`)

New string flag, long-form only (no short flag — `flag-naming-conventions.md`):

```go
cmd.Flags().StringVar(&temp, "temp", "", "Create a detached throwaway worktree at <ref> (no branch; name prefixed tmp-; init skipped unless --init)")
```

Behavior when `--temp <ref>` is set:

1. **Conflicts — `ExitInvalidArgs` (2), before any git work**, validated in `cmd/` alongside the existing `--checkout` mutex checks, using `wt.ExitWithError` with Error/Why/Fix copy:
   - `--temp` + positional branch arg:
     - what: `--temp cannot be combined with a positional branch argument`
     - why: `--temp creates a detached worktree with no branch; the positional names a new branch`
     - fix: `Use one of: wt create <new-branch> | wt create --temp <ref>`
   - `--temp` + `--checkout`:
     - what: `--temp cannot be combined with --checkout`
     - why: `--temp creates a detached worktree; --checkout puts a worktree on an existing branch`
     - fix: `Use one of: wt create --checkout <branch> | wt create --temp <ref>`
   - `--temp` + `--reuse`:
     - what: `--temp cannot be combined with --reuse`
     - why: `A temp worktree is always freshly created; --reuse returns an existing one`
     - fix: `Drop --reuse, or reuse by name without --temp: wt create --reuse --name <name>`
   - `--temp` + `--base`:
     - what: `--temp cannot be combined with --base`
     - why: `--temp's <ref> is already the commit to detach at; --base is the start-point for a NEW branch`
     - fix: `Pass the commit to --temp directly: wt create --temp <ref>`
   - `--temp` + `--pr` (only if W4's `--pr` flag exists on `main` at apply time — see § 6): what `--temp cannot be combined with --pr`, why `--pr checks out the PR's head branch; --temp detaches at a commit`, fix `For a detached worktree at a PR head, pass its SHA: wt create --temp <sha>`.
   - `--init` + `--no-init` (both set): what `--init cannot be combined with --no-init`, why `--init opts in to the init script for --temp; --no-init skips it`, fix `Pass one of --init or --no-init`.
2. **Ref resolution** — resolve `<ref>` to a commit SHA once, in `internal/worktree` (Constitution V), via `git rev-parse --verify --quiet <ref>^{commit}`. Unresolvable → `ExitInvalidArgs` (2), mirroring the existing invalid-`--base` copy: what `Invalid --temp ref: <ref>`, why `'<ref>' does not resolve to a commit`, fix `Provide a valid branch name, tag, or commit SHA`. If W1's `--fetch` is present (§ 6), the fetch runs **before** resolution with W1's remote rule (`origin/x` → `origin`, else `origin`); a failed fetch is `ExitGitError` (3), nothing created.
3. **Name** — `tmp-` prefix always applied:
   - no `--name`: `tmp-` + a random adjective-noun, unique under `<repo>.worktrees/` (e.g. `tmp-swift-fox`). Implement as a prefixed variant of `GenerateUniqueName` (e.g. `GenerateUniqueNameWithPrefix(worktreesDir, "tmp-", 10)`, with `GenerateUniqueName` delegating to it with `""`) — reuse, don't duplicate. Exhaustion → `ExitRetryExhausted` (4), existing copy.
   - `--name foo` → `tmp-foo`; `--name tmp-foo` → `tmp-foo` (no double prefix). A named prefix constant (e.g. `TempNamePrefix = "tmp-"`) lives in `internal/worktree` — no magic strings.
   - Interactive name prompt is **skipped** under `--temp` (the name is either explicit or random; a throwaway needs no naming ceremony). An existing directory at the final name → existing `Worktree '<name>' already exists` error, `ExitGeneralError` (1) — unchanged collision behavior.
4. **Create** — new mode-explicit function in `crud.go`, alongside `CreateNewBranchWorktree` / `CheckoutBranchWorktree`:

   ```go
   // CreateTempWorktree creates a detached worktree at sha (no branch, no config).
   func CreateTempWorktree(sha, name string, ctx *RepoContext, rb *Rollback) (string, error)
   ```

   It runs `EnsureWorktreesDir`, then `git worktree add --detach <path> <sha>`, and registers **only** `git worktree remove --force <path>` on the rollback (there is no branch to `-D`). It writes no git config (the plan's W2 recorded-base explicitly excludes `--temp`). Failure → `ExitGitError` (3) with `Failed to create worktree` + git's message.
5. **Prompts / confirm** — the dirty-state prompt keeps its existing behavior (it is mode-independent). The init-confirm gate does not fire under `--temp` (an explicit `--init` is the consent). The Open phase is unchanged (`--open` default `prompt`, `skip` under `--non-interactive`).
6. **Output** — the Git-phase summary block (stderr) reads:

   ```
   Created worktree: tmp-swift-fox
   Path: /…/repo.worktrees/tmp-swift-fox
   Branch: (detached)
   From: <ref as typed> @ <short-sha>
   ```

   `From:` keeps the ref verbatim and appends ` @ <short-sha>` (7-char, the resolved commit) — the same suffix shape W1 introduces for resolved `--base`s, so the two converge. Resolved pre-dispatch (outside the reinstall window, per `create-output-phases.md`). stdout: the worktree path as the last line, unchanged contract.
7. **Init** — see § 2.

### 2. Init on `--temp`: skipped by default, `--init` opts in (decision)

**Decision (resolves the plan's W5 open question): `--temp` skips the init script by default; a new `--init` bool flag opts in.** Rationale: surgery worktrees are short-lived and exist to rebase/resolve/inspect — they never want `fab sync` (which writes skill/config files into the tree, i.e. would dirty the very tree being operated on) and init is the slowest phase of create. The plan's brief lists this option first, and the decision's own rationale ("surgery never wants `fab sync`") argues for it.

```go
cmd.Flags().BoolVar(&initFlag, "init", false, "Run the worktree init script for --temp (skipped by default for temp worktrees)")
```

- `--temp` alone → init skipped; no Init separator, no `Running worktree init...`. (No "skipped" warning line either — skipping is the documented default, not a degradation.)
- `--temp --init` → init runs exactly as for any other create (same runner, Init separator, SIGINT-during-init handling, `ExitInitFailed` (7) on non-zero with the worktree kept, open-anyway prompt when interactive), with **no** "Initialize worktree?" confirm.
- `--init` without `--temp` → accepted, no-op (init already runs by default). Not an error — keeps scripts that always pass `--init` working.
- `--init` + `--no-init` → `ExitInvalidArgs` (2) (§ 1.1).
- Naming follows `flag-naming-conventions.md`: `--no-init` remains the negation of the default-ON behavior; `--init` is the positive opt-in for the one mode whose default is OFF. Recorded as a Design Decision in `flag-naming-conventions.md`.
- The decision is written into `docs/specs/init-protocol.md` (new short section, e.g. "Temp worktrees: init skipped by default").

### 3. `wt list` marks temp worktrees (`src/cmd/wt/list.go`)

- `listEntry` gains `Temp bool \`json:"temp"\`` — **always emitted** (`true`/`false`), on every record, in default and `--status` modes (an always-present bool is additive and keeps the record schema stable for consumers).
- **Temp predicate** (pure, no subprocess — keeps `list-status-contract.md`'s "default mode = exactly one `git worktree list --porcelain`" invariant): a worktree is temp iff its porcelain entry is `detached` **and** it is not the main worktree **and** its basename starts with `tmp-`. Implement as a named helper in `internal/worktree` (e.g. `IsTempWorktree(name string, detached bool) bool`) used by `buildBaseEntry`. A detached worktree without the prefix (e.g. made by raw `git worktree add --detach`) is not temp; a `tmp-*`-named worktree on a branch is not temp.
- **`branch` is `null` for temp records** in JSON (`Branch` becomes `*string` with no `omitempty`, or a custom marshal — the apply agent picks the least invasive shape). Non-temp detached worktrees keep today's `"branch": "(detached)"` string (back-compat: the plan's "every change is additive" decision — only the new temp records get the new null).
- Human output (all three layouts, including `--status`) shows `(detached)` in the Branch column for temp worktrees — already true today via the porcelain parser; add a test that pins it. `--status` already skips unpushed for detached (`r.branch != "(detached)"`) — keep that working when the branch representation changes.
- `wt list --path <name>` unchanged.
- The duplicated porcelain parser in `internal/worktree/worktree.go` (`parseWorktreeList`, `Info`) is only touched if needed; `Info` (used by `wt go`/`open`) is **not** given a temp field (non-goal: "Changing `wt go` / `wt open` / `wt delete` beyond what `--temp` listing needs").

### 4. `wt delete` handles detached worktrees without branch prompts (`src/cmd/wt/delete.go`)

`wt delete <tmp-name>` (and `--all`, multi-name, `--stale`, the menu, and delete-current from inside the temp worktree) removes the worktree and performs **no branch handling**:

- `handleBranchCleanup` returns immediately for a detached worktree — the porcelain sentinel `(detached)` and the current-path `HEAD` (from `git rev-parse --abbrev-ref HEAD`) both mean "no branch". Normalize once (e.g. a small `isDetachedBranch(branch string) bool` helper, or map both to `""` at the read sites — `handleBranchCleanup` already early-returns on `""`). This also skips the `wt/<name>` orphan cleanup for detached worktrees (a temp worktree never had one) and makes `--branch true` a no-op instead of `git branch -D HEAD`.
- The unpushed-commits check on the delete-current path (`wt.HasUnpushedCommits(branch)`) is skipped when detached.
- `--dry-run` prints `Would remove worktree: <name>` and no `Would delete branch` lines for a temp worktree.
- The human `Branch:` line in delete's stderr header shows `(detached)` (not `HEAD`) for consistency with `wt list`.
- This applies to **any** detached worktree, not only `tmp-*` ones — a detached worktree has no branch to clean up, by definition. No other delete behavior changes (stash/dirty discard hazard, confirmations, exit codes unchanged).

### 5. Docs, skill bundle, help-dump (same change — plan Rule 3)

- `docs/specs/cli-surface.md` § `wt create`: `--temp <ref>` and `--init` flag rows; a "Temp worktrees" paragraph (detached at the resolved SHA, `tmp-` name, no branch/config, init skipped unless `--init`, conflicts); exit codes list gains the `--temp` conflicts / invalid `--temp` ref under `ExitInvalidArgs`. § `wt list`: the `temp` key and `branch: null` for temp records. § `wt delete`: detached worktrees get no branch cleanup.
- `docs/specs/worktree-layout.md`: a "Temp worktrees" case in § Naming convention (`tmp-` prefix, `--name` prefixed) and § Branch ↔ worktree relationship (case 3: detached, no branch; `wt delete --branch` has nothing to act on).
- `docs/specs/init-protocol.md`: the temp init default + `--init` opt-in.
- Memory (hydrate updates; each with a Design Decision citing plan `26-11-23-worktree-base-receipts.md` W5): `create-branch-semantics.md` (a `--temp` mode row in the exit-code/mode matrix + conflict rows + the `From:` table row + the `CreateTempWorktree` seam), `create-output-phases.md` (`From:` for temp, no Init separator by default), `list-status-contract.md` (`temp` key, temp predicate, `branch: null`), `flag-naming-conventions.md` (`--init` positive opt-in beside `--no-init`), `delete-dry-run-contract.md` if the dry-run preview for detached worktrees is described there.
- Skill bundle: edit `docs/site/skill.md`, run `just sync-skill` to refresh `src/cmd/wt/skill.md` (drift-guard test + ≤150-line budget — keep the addition to a line or two: `wt create --temp <sha>` for throwaway detached worktrees, `wt delete <tmp-name>` to drop it).
- `wt help-dump`: generated from the cobra tree at runtime; update any fixture/assertion in `src/cmd/wt/help_dump_test.go` / `src/internal/worktree/helpdump_test.go` that enumerates `create` flags.
- Toolkit standards: the constitution requires checking CLI-surface changes against `shll standards` (help output conventions) — run it if available.

### 6. Coordination with the parallel steps W1 / W3 / W4

Apply MUST start by rebasing the branch onto the latest `origin/main`. Then:

- **W1 merged** (`--fetch` / `--json` exist in `create.go`): `--temp` honours `--fetch` (fetch before ref resolution, W1's remote rule, `ExitGitError` on failure) and the `--json` receipt reports `mode: "temp"`, `branch: null`, `base_ref: <ref as typed>`, `base_sha: <resolved sha>` — W1's brief already reserves `temp` in the `mode` enum. Add tests for both.
- **W1 not merged**: W5 does **not** introduce `--fetch` or `--json` itself (they are W1's scope). Structure the temp path so the resolved SHA and ref are held in locals the W1 receipt can read; whichever of W1/W5 merges second wires `--fetch`/`mode: "temp"` during its conflict resolution (the plan's `merge-auto` queue).
- **W4 merged** (`--pr` exists): add the `--temp` + `--pr` conflict (§ 1.1). W4's own brief lists `--temp` as exclusive with `--pr`, so if W4 lands second it adds the mutex.
- **W3** (`--reuse` + `--base` conflict): independent; just keep the mutex block readable when both land.

## Affected Memory

- `wt-cli/create-branch-semantics`: (modify) `--temp <ref>` mode row, conflict rows (`--temp` × positional / `--checkout` / `--reuse` / `--base` / `--pr`, `--init` × `--no-init`), `From:` table row, `CreateTempWorktree` seam
- `wt-cli/create-output-phases`: (modify) temp `From: <ref> @ <short-sha>` and `Branch: (detached)` summary lines; no Init phase by default under `--temp`
- `wt-cli/list-status-contract`: (modify) always-present `temp` JSON key, the subprocess-free temp predicate, `branch: null` for temp records
- `wt-cli/flag-naming-conventions`: (modify) `--init` as the positive opt-in for the default-OFF temp mode beside `--no-init`
- `wt-cli/delete-dry-run-contract`: (modify) detached worktrees: no branch-cleanup preview lines

## Impact

- **Code**: `src/cmd/wt/create.go` (flags `--temp`, `--init`; mutex checks; temp dispatch branch; summary label; init gate), `src/internal/worktree/crud.go` (`CreateTempWorktree`), `src/internal/worktree/names.go` (prefixed unique-name generation + `TempNamePrefix`), a ref→SHA resolver in `internal/worktree` (e.g. `ResolveCommit(ref string) (string, error)` in `git.go` — reuse W1's if it exists after rebase), `src/cmd/wt/list.go` (`temp` field, `branch` null for temp, temp predicate), `src/cmd/wt/delete.go` (detached → no branch handling).
- **Tests** (Constitution IV; plan Rule 2): unit tests per path in `create_test.go`, `list_test.go`, `delete_test.go`, `crud`/`names` tests; integration cases in `src/cmd/wt/integration_test.go` against a real temp repo:
  - `wt create --temp <sha> --non-interactive` → worktree exists at `<repo>.worktrees/tmp-*`, `git -C <path> rev-parse HEAD` == the SHA, `git -C <path> symbolic-ref -q HEAD` fails (detached), no new branch in `git branch --list`, no `branch.*` config written; stdout last line is the path.
  - `--temp <branch-name>` resolves to the branch tip SHA and stays detached (branch not checked out).
  - `--name foo` → `tmp-foo`; `--name tmp-foo` → `tmp-foo`.
  - init skipped by default (an init script that would create a marker file does not run); `--temp --init` runs it.
  - `wt list --json` → the temp record has `"temp": true, "branch": null`; other records `"temp": false`; `wt list --status` shows `(detached)`.
  - `wt delete <tmp-name> --non-interactive` → worktree removed, no `Skipped branch deletion` / `Deleted branch` output, other branches untouched; `--dry-run` shows no branch lines.
  - conflicts each exit 2 with nothing created: `--temp X <pos>`, `--temp X --checkout Y`, `--temp X --reuse --name N`, `--temp X --base Y`, `--init --no-init`; invalid ref exits 2.
- **No exit-code changes** (Constitution III): conflicts and bad refs → 2, `git worktree add` failure → 3, name exhaustion → 4, init failure → 7.
- **Back-compat**: additive — two new flags, one new always-present JSON key, `branch: null` only on new temp records; delete's detached handling replaces bogus output, not a contract.
- **Downstream**: hexokit operator Merging rules can replace raw `git worktree add --detach` with `wt create --temp <head-sha>` (not part of this change — plan § Downstream).

## Open Questions

- None blocking. The W1/W4 integration points (§ 6) are resolved at apply time by what is on `main` after rebase.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--temp <ref>` = `git worktree add --detach <path> <sha>`, `tmp-` prefixed name (`--name` still prefixed), no branch, no config, removable by `wt delete`, exclusive with positional/`--checkout`/`--reuse`/`--pr` | Plan's binding Decisions row + W5 Brief verbatim | S:95 R:70 A:95 D:95 |
| 2 | Confident | Init on `--temp` skipped by default; new `--init` bool opts in (no confirm); `--init` without `--temp` is a no-op; `--init`+`--no-init` exits 2 | Plan open question delegated to this intake; brief lists this first and the decision's own rationale ("surgery never wants `fab sync`") favors it; `fab sync` would dirty the tree being operated on; plan's merge-auto queue has no user gates | S:70 R:70 A:65 D:55 |
| 3 | Confident | `--temp` + `--base` is also a usage error (exit 2) | Not in the brief's exclusive list, but `--temp`'s value already is the commit; silent precedence is exactly the gotcha the plan's W3 removes for `--reuse`+`--base` | S:55 R:85 A:75 D:70 |
| 4 | Confident | Temp predicate = porcelain `detached` AND non-main AND basename has `tmp-` prefix; computed with zero extra subprocesses | Preserves list-status-contract's single-subprocess default; no state file/config (Constitution I); prefix is guaranteed by `--temp` naming | S:60 R:75 A:75 D:60 |
| 5 | Confident | `temp` JSON key always present (bool); `branch` is `null` only for temp records, non-temp detached keep `"(detached)"` | Brief says `temp: true` / `branch` null; always-present mirrors default-JSON's always-emitted base keys; limiting null to temp keeps the change additive per plan's back-compat decision | S:55 R:60 A:55 D:40 |
| 6 | Confident | Name `--name tmp-foo` is not double-prefixed; interactive name prompt skipped under `--temp` | Throwaway naming needs no ceremony; double prefix is never intended | S:50 R:85 A:70 D:65 |
| 7 | Confident | `wt delete` treats any detached worktree (`(detached)` / `HEAD`) as branchless: no cleanup, no orphan `wt/` cleanup, no unpushed check, `--branch true` a no-op | Brief: "removes it without branch prompts"; current code passes the sentinel as a branch name (bogus copy, `git branch -D HEAD`) — a detached worktree has no branch by definition | S:70 R:80 A:85 D:75 |
| 8 | Confident | `From:` line for temp = `<ref as typed> @ <short-sha>`, `Branch: (detached)` | Mirrors W1's ` @ <short-sha>` suffix for resolved bases so the two converge; ref verbatim per create-output-phases | S:60 R:90 A:75 D:65 |
| 9 | Confident | W5 does not add `--fetch`/`--json`; apply rebases on latest `origin/main` and wires `--fetch` + `mode: "temp"` (and the `--pr` mutex) only if W1/W4 are already merged — else the later-merging step integrates | W1/W4 run in parallel sibling worktrees, unmerged at intake; their scope is theirs; plan's merge-auto queue resolves conflicts at merge | S:65 R:70 A:70 D:60 |
| 10 | Certain | Ref resolved to SHA via `git rev-parse --verify <ref>^{commit}` in `internal/worktree`; bad ref exits 2, worktree-add failure 3; rollback registers only worktree removal | Constitution II/III/V, plan "Exit codes: no new codes", existing invalid-`--base` precedent | S:80 R:85 A:90 D:85 |

10 assumptions (2 certain, 8 confident, 0 tentative, 0 unresolved).
