# Intake: `--reuse` + `--base` is a usage error

**Change**: 261123-udcv-reuse-base-usage-error
**Created**: 2026-11-23

## Origin

> Execute step W3 (--reuse + --base is a usage error) of the plan at /home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md. Read the whole plan first (Decisions and Rules bind you), then implement only the W3 Brief.

One-shot invocation by a hexokit operator executing a plan-step queue (`merge-auto`, no user gates).
The plan (written 2026-11-23, "wt — fresh bases, receipts, a recorded base, PR and temp worktrees")
has a **Decisions already taken (do not reopen at intake)** table; the row that binds this step:

| Decision | Choice | Why |
|---|---|---|
| `--reuse` + `--base` | A usage error (exit 2), like `--checkout` + `--base` today | Silent precedence is a documented gotcha; an explicit spec amendment to `create-branch-semantics.md` |
| Back-compat | Every change is additive; no existing flag changes meaning except the `--reuse`+`--base` conflict (now an error) | Downstream `riff` and agents keep working unchanged until they opt in |
| Exit codes | No new codes | Constitution III |

The W3 Brief, verbatim:

> - `wt create --reuse --base <ref>` exits `ExitInvalidArgs` (2) before any mutation:
>   `Error: --reuse and --base cannot be combined` / `Why: --reuse keeps the existing worktree's
>   branch, so a start-point has nothing to apply to` / `Fix: drop --base, or drop --reuse to create
>   a new worktree`. Same for `--reuse` + `--fetch`.
> - Tests: the conflict exits 2 with nothing created; `--reuse` alone unchanged.
> - Docs: `create-branch-semantics.md` ("`--reuse` takes precedence over `--base`" becomes the
>   conflict row, with a Design Decision), `cli-surface.md`, skill bundle gotcha, help-dump.

Plan rules that bind this step: flag parsing/validation in `src/cmd/wt/create.go`, git ops in
`src/internal/worktree/` (Constitution II/V — this step needs no git op); unit tests per path plus
an integration case in `src/cmd/wt/integration_test.go` against a real temp repo (Constitution IV);
CLI ⇒ docs in the same change (`docs/specs/cli-surface.md`, the `wt skill` bundle via
`just sync-skill`, help-dump, the touched memory contract with a Design Decision citing the plan);
no new short flags. Review bar must-fix items relevant here: a worktree or branch left behind on
failure, a changed exit code.

## Why

1. **Problem.** Today `wt create --reuse --name X --base <ref>` silently lets `--reuse` win: when
   worktree `X` already exists it is reused on whatever branch/base it had, and `--base` is ignored
   without a word (it is not even validated — `wt create --reuse --name X --base <bad-ref>` succeeds).
   A caller (human, agent, or hexokit's engine) that passes both believes it got a worktree cut from
   `<ref>` and may not have. `create-branch-semantics.md` documents this as accepted precedence,
   i.e. a known gotcha.
2. **Consequence of not fixing.** Wrong-base worktrees surface much later (a PR diff full of
   unrelated commits, a rebase onto the wrong line). The upcoming W1 receipt (`base_ref`/`base_sha`
   null for `reuse`) and W2 recorded base make the ambiguity worse: a caller that asked for a base
   gets a receipt saying there was none.
3. **Why this approach.** It mirrors the existing `--checkout` + `--base` conflict (exit 2, same
   `Error/Why/Fix` shape, same placement), so the rule is uniform: `--base` only ever pairs with
   creating a new branch. Rejected alternatives (per the plan's Decisions table): keep silent
   precedence (the gotcha); warn-and-ignore (still lets scripts proceed on a wrong assumption).

## What Changes

### 1. New argument conflict in `src/cmd/wt/create.go`

Add a pure flag-parsing check in `RunE`, in the existing conflicting-selector block (right after the
`--checkout` + `--base` check, i.e. **before** `wt.ValidateGitRepo()`, rollback setup, branch-name
validation, `--base` ref validation, and any git work), using the same `wt.ExitWithError` helper as
its neighbours:

```go
if reuse && base != "" {
	wt.ExitWithError(wt.ExitInvalidArgs,
		"--reuse and --base cannot be combined",
		"--reuse keeps the existing worktree's branch, so a start-point has nothing to apply to",
		"Drop --base, or drop --reuse to create a new worktree")
}
```

Copy is the Brief's verbatim (`Error:` / `Why:` / `Fix:` prefixes are added by `ExitWithError`).
The Fix line's first word is capitalised to match the sibling conflict messages
(`Drop --base, or create a new branch: …`).

Semantics:

- The check is **unconditional on collision**: it fires whether or not worktree `--name` exists.
  The plan says "before any mutation" and the check runs before the repo context is even read.
  This deliberately changes one previously-working invocation: a non-colliding
  `wt create --reuse --name X --base R` used to create `X` off `R`; it now exits 2. This is the
  single back-compat break the plan's Decisions table names.
- **Ordering vs. `--reuse requires --name`**: the existing `--reuse requires --name` check stays
  first (it is the more fundamental misuse). `wt create --reuse --base R` (no `--name`) therefore
  reports `--reuse requires --name`; `wt create --reuse --name X --base R` reports the new conflict.
  Both exit 2.
- Flag-presence test is `base != ""`, consistent with the existing `checkout != "" && base != ""`
  check (an explicitly empty `--base ""` is treated as unset, as today).

### 2. Drop the now-dead `--reuse` short-circuit on `--base` validation

The current guard:

```go
// Validate --base ref whenever it is set and --reuse is not.
// ... only --reuse short-circuits before --base is used
// (so `wt create --reuse --base <bad>` must not fail on the ref).
if base != "" && !reuse {
```

becomes `if base != "" {` with the comment rewritten: `--base` + `--reuse` and `--base` +
`--checkout` are both rejected above, so whenever `--base` reaches this point it applies to a new
branch. Behaviour is identical (the `!reuse` arm is unreachable after change 1).

### 3. `--reuse` + `--fetch` (conditional on W1)

The Brief says "Same for `--reuse` + `--fetch`", but `--fetch` does not exist on `main` yet — it is
introduced by plan step W1, which this step does not depend on. This change therefore:

- **If `--fetch` is absent at apply time** (expected — this branch is cut from `main` at `22ae6fe`):
  implements only the `--base` conflict and adds nothing about `--fetch` to code, help, or docs (no
  documentation of a flag that does not exist). The `--reuse` + `--fetch` conflict is left to W1
  (or whichever of W1/W3 merges second), and this intake's Assumption #4 records that hand-off so a
  reviewer can trace it.
- **If `--fetch` already exists at apply time** (W1 merged and this branch rebased): add the
  sibling check with the same shape —
  `--reuse and --fetch cannot be combined` / `--reuse keeps the existing worktree as it is, so there
  is no base to fetch` / `Drop --fetch, or drop --reuse to create a new worktree` — plus its test and
  doc rows alongside the `--base` ones.

### 4. Help text

- `--base` flag usage string: append that it cannot be combined with `--checkout` or `--reuse`
  (e.g. `"Git ref (branch, tag, SHA) to use as start-point for new branch (not with --checkout or --reuse)"`)
  — keep it short; the `Long` text is the fuller reference.
- `create` `Long` text: the `--checkout` paragraph already says what `--checkout` cannot be combined
  with; add one sentence: `--reuse cannot be combined with --base: a reused worktree keeps its
  existing branch.`
- `wt help-dump` is generated from the cobra tree, so it picks these up automatically; update any
  help-dump test/fixture that asserts on these strings (check `src/cmd/wt/help_dump_test.go` and
  `src/internal/worktree/helpdump_test.go`).

### 5. Tests (Constitution IV — unit + integration)

- **Conflict, worktree does not exist**: `wt create --non-interactive --reuse --name X --base main`
  in a real temp repo → exit 2, stderr contains `--reuse and --base cannot be combined`, no
  `<repo>.worktrees/X` directory, no branch `X` (`git rev-parse --verify X` fails).
- **Conflict, worktree already exists**: create `X` first, then the same `--reuse --base` call →
  exit 2, and `X`'s branch/HEAD is unchanged (no reuse happened; init did not run).
- **Conflict with a bad ref**: `--reuse --name X --base does-not-exist` → exit 2 with the conflict
  copy, not `Invalid --base ref` (proves the check runs before ref validation).
- **`--reuse` alone unchanged**: `--reuse --name X` on an existing `X` still reuses (exit 0, path
  on stdout), and `--reuse` without `--name` still reports `--reuse requires --name`.
- Integration cases go in `src/cmd/wt/integration_test.go` (binary against `t.TempDir()` repo, using
  the existing `runWt` helper with its env isolation and `--open skip` / `--non-interactive` — the
  code-review rule about no host side-effects applies). If the existing suite has an in-process unit
  harness for `create` conflict checks (look at `create_test.go` for how the `--checkout` + `--base`
  conflict is tested), add the parallel unit case there too.

### 6. Docs (CLI ⇒ docs, same change)

- **`docs/specs/cli-surface.md`** § `wt create`: `--reuse` row gains "Cannot be combined with
  `--base`."; `--base` row drops "whenever set and `--reuse` is not" and reads "Cannot be combined
  with `--checkout` or `--reuse`."; any exit-code / conflict list in that section gains the new
  combination.
- **`docs/site/skill.md`** (canonical skill bundle) → `just sync-skill` copies it to
  `src/cmd/wt/skill.md` (the drift-guard test in `skill_test.go` enforces the copy). Add the
  gotcha: `--reuse` cannot be combined with `--base` (exit 2) — to cut from a ref, create a new
  worktree. Check lines ~25 and ~99 of the bundle where `--reuse` is already described.
- **`docs/memory/wt-cli/create-branch-semantics.md`** (hydrate stage owns the memory edit, listed
  here for scope): the `--base` section's "`--reuse` short-circuits before `--base` is validated"
  paragraph becomes the conflict; the "Conflicting selectors" section gains the third combination
  with its what/why/fix; the exit-code/mode matrix gains `wt create --reuse --name X --base Y |
  conflict | ExitInvalidArgs (2)` and the reuse row notes `--base` is rejected; the
  "`--reuse` is unchanged" section is retitled/adjusted; a new Design Decision cites the plan
  (`fab/plans/sahil/26-11-23-worktree-base-receipts.md`, step W3) — silent precedence rejected
  in favour of a hard conflict mirroring `--checkout` + `--base`.

## Affected Memory

- `wt-cli/create-branch-semantics`: (modify) `--reuse` + `--base` becomes a conflict row
  (`ExitInvalidArgs`, copy above) instead of "`--reuse` short-circuits `--base`"; matrix row; `--base`
  validation guard simplified to `base != ""`; new Design Decision citing plan step W3.

## Impact

- **Code**: `src/cmd/wt/create.go` only (one new conflict check, one simplified guard + comment,
  help strings). No `src/internal/worktree/` change — the check is pure argument parsing, which
  the existing "Flag-mutex validation in `cmd/`, existence rules in `internal/`" design decision
  places in `cmd/`.
- **Tests**: `src/cmd/wt/integration_test.go` (+ `create_test.go` if a unit harness exists for
  conflict checks); help-dump tests if they assert on changed strings; `skill_test.go` drift guard
  (satisfied by `just sync-skill`).
- **Docs**: `docs/specs/cli-surface.md`, `docs/site/skill.md` + `src/cmd/wt/skill.md`.
- **Exit codes**: no new codes; reuses `ExitInvalidArgs` (2).
- **Downstream**: hexokit `internal/riff` calls `wt create --name <id> --base <sha>` (no `--reuse`)
  and `--checkout` for respawn — unaffected. Only a caller combining `--reuse` with `--base` is
  affected, and it now fails loudly instead of silently getting the wrong base.

## Open Questions

- None blocking. The `--fetch` half depends on W1's merge order and is handled conditionally (What
  Changes § 3).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--reuse` + `--base` exits `ExitInvalidArgs` (2) with the Brief's Error/Why/Fix copy verbatim | Plan Decisions table + W3 Brief fix both; constitution III keeps the code set | S:95 R:85 A:95 D:95 |
| 2 | Certain | Check lives in `cmd/create.go` beside the `--checkout` + `--base` conflict, before `ValidateGitRepo` and any git work | Brief says "before any mutation"; memory DD "Flag-mutex validation in `cmd/`" and Constitution V | S:85 R:90 A:95 D:90 |
| 3 | Certain | Conflict fires regardless of whether the named worktree exists — a non-colliding `--reuse --name X --base R` now errors too | Brief says unconditional "before any mutation"; plan Back-compat row names this as the one accepted meaning change | S:80 R:80 A:85 D:85 |
| 4 | Confident | `--reuse` + `--fetch` conflict is implemented here only if `--fetch` exists at apply time; otherwise it is left to W1 and nothing about `--fetch` is documented | `--fetch` is not on `main` (W1 adds it; W3 has no dependency on W1); documenting a nonexistent flag would be wrong | S:65 R:85 A:70 D:65 |
| 5 | Confident | `--reuse requires --name` stays the first check; the new conflict runs after it | It is the more fundamental misuse; both exit 2, so the order only selects the message | S:50 R:95 A:80 D:75 |
| 6 | Certain | Simplify the `--base` validation guard to `base != ""` and rewrite its comment | `!reuse` arm becomes unreachable; leaving it would document stale precedence | S:60 R:95 A:90 D:85 |
| 7 | Confident | Fix copy capitalised (`Drop --base, …`) to match sibling conflict messages | Brief's lowercase `drop` reads as prose inside a quote; existing `ExitWithError` fix strings start uppercase | S:55 R:95 A:80 D:70 |
| 8 | Certain | Help strings (`--base` usage, `create` Long text) mention the new conflict; help-dump updates via cobra | Plan Rule 3 lists help-dump; `--checkout` usage already names its conflicts | S:70 R:95 A:80 D:75 |

8 assumptions (5 certain, 3 confident, 0 tentative, 0 unresolved).
