# wt — fresh bases, receipts, a recorded base, PR and temp worktrees

> Plan doc, written 2026-11-23 from a hexokit `/fab-discuss` session on the operator's spawn path.
> Every gap below was found by reading what callers do *around* `wt create` — hexokit's
> `internal/riff` and `internal/autopilot/spawn.go`, and agents spawning by hand — against
> `wt skill`, `wt create --help`, `docs/specs/cli-surface.md` and the `wt-cli` memory contracts.
> Executed by a hexokit operator (plan-step queue, `merge-auto`); this repo's constitution and
> memory contracts bind every step.

**Problem.** `wt create` already does the worktree half of agent spawning well — any base ref
(`--base`), existing branches (`--checkout`), names, init, typed exit codes. Callers then
re-implement the last few steps around it, each slightly differently:

1. **Stale base.** `--base origin/eng-v2` takes the remote-tracking ref as the local clone last saw
   it. hexokit's engine runs `git fetch origin` + `git rev-parse` itself to pin a fresh SHA before
   calling `wt create --base <sha>`; an agent spawning by hand usually forgets the fetch.
2. **No receipt.** `create` prints the worktree path as its last stdout line; the branch, the base
   and the resolved SHA have to be re-derived by the caller (riff parses the path line, the engine
   keeps its own copy of the branch — which went stale when `/fab-new` renamed it).
3. **The base is forgotten.** Nothing records what a worktree was cut from, so a PR base has to be
   told twice downstream (`fab status set-base-branch`, or a prompt's "open a draft PR against
   `<base>`"), and a later tool cannot derive it.
4. **`--reuse` silently wins over `--base`** (a documented gotcha in `create-branch-semantics.md`):
   a caller that passes both gets the existing worktree on whatever base it had.
5. **PR-surgery worktrees are hand-rolled.** Resolving a conflict or restacking a PR wants a
   throwaway detached worktree at a PR's head; today that is raw `git worktree add --detach` plus
   manual cleanup, in two different codebases.

---

## Goal

`wt create` is the one way a worktree gets made, by a human, an agent or an engine, and it
returns everything the caller needs to know about what it made — without the caller fetching,
resolving or recording anything itself.

```sh
wt create --name mp-f1 --base origin/eng-v2 --fetch --json
# {"ok":true,"result":{"path":"…/repo.worktrees/mp-f1","name":"mp-f1","branch":"mp-f1",
#   "base_ref":"origin/eng-v2","base_sha":"f68be281ad…","init":"ok"}}
wt list --json      # … "base":"origin/eng-v2","base_sha":"f68be281ad…" …
wt create --pr 5686 # worktree on PR 5686's head branch
wt create --temp f68be281  # detached throwaway for a rebase
```

---

## Decisions already taken (do not reopen at intake)

| Decision | Choice | Why |
|---|---|---|
| Scope of `wt` | Creating the right worktree on the right commit, and saying what it made. **Not**: fetching dependency code, cherry-picks, PR bases in other tools, tabs, prompts | "A directory per branch is the whole point" (`wt skill`). Policy stays with the caller; `wt` is mechanism |
| Fresh base | An explicit `--fetch` flag, never an implicit fetch | A surprise network call on every create is slower and non-deterministic for tests; the caller knows when freshness matters |
| Receipt | `wt create --json` prints **one JSON document on stdout and nothing else** — the toolkit `--json` contract (`shll standards`; confirm the envelope shape at W1 intake against `wt list --json`'s existing array form). Without `--json`, stdout stays the path as the last line (`create-output-phases.md`) | stdout is data, stderr is human copy; a structured receipt replaces last-line parsing in riff |
| Recorded base | Git config on the branch: `branch.<name>.wt-base <ref>` and `branch.<name>.wt-base-sha <sha>`, written only when `wt` created the branch (positional / bare / `--base`); never on `--checkout` or `--reuse` | Travels with the repo, no hidden state (Constitution I); derivable by any later tool with `git config` |
| `--reuse` + `--base` | A usage error (exit 2), like `--checkout` + `--base` today | Silent precedence is a documented gotcha; an explicit spec amendment to `create-branch-semantics.md` |
| `--pr <n>` | Resolves the head branch with `gh pr view <n> --json headRefName,headRepositoryOwner,isCrossRepository` and runs the `--checkout <head>` path; a cross-repository (fork) head is exit 2 in v1; `gh` missing or unauthenticated is exit 1 with the hint | Reuses the existing remote-only checkout path; forks need a remote `wt` does not manage |
| `--temp <ref>` | A detached worktree at `<ref>` (`git worktree add --detach`), random name with a `tmp-` prefix, no branch created, listed by `wt list` with `temp: true`; removable with `wt delete` | PR surgery wants a worktree that cannot pollute a branch and is obviously disposable |
| Init on `--temp` | Decide at W5 intake: skip by default (surgery never wants `fab sync`) vs run for consistency | Both defensible; the init protocol spec is the place to record it |
| Exit codes | No new codes: `--fetch` failing is `ExitGitError` (3); `--pr` lookup failures map to 1 (gh) / 2 (fork, args) | Constitution III: the documented set stays stable |
| Back-compat | Every change is additive; no existing flag changes meaning except the `--reuse`+`--base` conflict (now an error) | Downstream `riff` and agents keep working unchanged until they opt in |
| Queue mode for this plan | `merge-auto`; steps in table order; no user gates | Small, independent steps; CI + review are the gate |

---

## Current state (verified 2026-11-23)

- `wt` v0.1.7. `src/cmd/wt/create.go` (flags at ~580: `--no-init`, `--reuse`, `--base`,
  `--checkout`, `--name`, `--open`, `--non-interactive`), `src/internal/worktree/crud.go`
  (`CreateNewBranchWorktree` / `CheckoutBranchWorktree`), `src/internal/worktree/worktree.go`
  (the `list --json` record), `src/internal/worktree/errors.go` (exit codes 0–7, 130),
  `src/cmd/wt/list.go` (`--json`, `--status`, `--path`).
- Specs: `docs/specs/cli-surface.md` (per-subcommand reference), `worktree-layout.md`,
  `init-protocol.md`, `launcher-contract.md`, `build-and-release.md` (`just release <bump>` tags;
  CI cross-compiles and updates the Homebrew tap).
- Memory (`docs/memory/wt-cli/`): `create-branch-semantics.md` (the `--base`/`--checkout`/`--reuse`
  matrix; "`--reuse` takes precedence over `--base`"), `create-output-phases.md` (stdout = machine
  result, the `From:` summary line already names the `--base` ref verbatim), `flag-naming-conventions.md`
  (no command-noun prefixes, `--no-*` negation, deprecate-never-remove).
- Skill bundle: `wt skill` is static Markdown synced by `just sync-skill`; `wt help-dump` is the
  machine command tree. Both are toolkit standards and change with every CLI surface change.
- Downstream today: hexokit `internal/riff` calls `wt create --name <id> --base <sha>` (and
  `--checkout` for respawn) and parses the path line; `internal/autopilot/spawn.go` fetches and
  `rev-parse`s before it; `rk riff -- <flags>` passes through to `wt create`.

---

## Rules for the operator executing this plan

1. One fab change per step, started with `/fab-new` in a fresh worktree off `main`, the step's
   **Brief** as the description plus this file's absolute path.
2. Constitution II/V: flag parsing and output in `src/cmd/wt/create.go`, every git operation and
   the config writes in `src/internal/worktree/`. Constitution IV: unit tests per path plus an
   integration case in `src/cmd/wt/integration_test.go` against a real temp repo (a bare origin
   plus a clone where a remote ref is needed).
3. CLI ⇒ docs, in the same change: `docs/specs/cli-surface.md`, the `wt skill` bundle
   (`just sync-skill`), `wt help-dump` fixtures, and the memory contract the step touches
   (`create-branch-semantics.md`, `create-output-phases.md`, `list-status-contract.md`,
   `flag-naming-conventions.md`), each with a Design Decision citing this plan.
4. Flags follow `flag-naming-conventions.md`: long names, no `create-`/`worktree-` prefixes, no new
   short flags.
5. Review bar: must-fix = a worktree or branch left behind on failure, a wrong base, a config
   written for a branch `wt` did not create, a changed exit code, stdout polluted under `--json`.
   Should-fix = everything else.
6. Nothing in this plan touches hexokit; the downstream adoption is listed at the end for the
   hexokit operator and is not a step here.

---

## Steps and dependencies

| ID | Step | Depends on | Size |
|---|---|---|---|
| **W1** | `--fetch` and the `--json` receipt on `wt create` | — | M |
| **W2** | Recorded base: `branch.<name>.wt-base[-sha]` on create; `base`/`base_sha` in `wt list --json` | W1 | S |
| **W3** | `--reuse` + `--base` is a usage error | — | S |
| **W4** | `--pr <n>`: a worktree on a PR's head branch | — | S |
| **W5** | `--temp <ref>`: a detached throwaway worktree | — | M |
| **R1** | Release (`just release patch`), by the operator; notify | W1–W5 | S |

```
W1 ── W2 ──┐
W3 ────────┼── R1
W4 ────────┤
W5 ────────┘
```

---

## Briefs

### W1 — `--fetch` and the `--json` receipt

- `--fetch` (bool): before resolving `--base`, run `git fetch <remote>` where `<remote>` is the
  remote named by a `remote/branch` base (`origin/eng-v2` → `origin`), else `origin`; with
  `--checkout` of a remote-only branch the existing fetch already happens (no second fetch). A
  failed fetch is `ExitGitError` (3) with `Error/Why/Fix` naming the remote; nothing is created.
  Without `--fetch`, behaviour is byte-identical to today.
- Resolve the base to a SHA once (`git rev-parse --verify <base>^{commit}`) and use that SHA as
  the `git worktree add -b` start point, so the branch is pinned even if the ref moves during the
  command. The `From:` summary line keeps the ref verbatim and appends ` @ <short-sha>` when the
  base was resolved (`create-output-phases.md` amendment).
- `--json` (bool): stdout carries exactly one JSON document, the toolkit envelope around
  `{path, name, branch, base_ref, base_sha, mode, init}` — `mode` ∈ `new | checkout | reuse | temp`,
  `base_ref`/`base_sha` null for `checkout`/`reuse`, `init` ∈ `ok | failed | skipped | not-found`.
  All phase separators and summaries stay on stderr. `--json` implies `--non-interactive` and
  `--open skip` (a menu cannot share stdout with a document). Exit codes unchanged; on failure
  the document is `{"ok":false,"error":{"code","message"}}` and the exit code is the usual one.
- Tests: fetch happens (a commit pushed to the bare origin after the clone's last fetch is the
  base); `--base` pinned to the SHA; `--json` stdout is one parseable document and nothing else;
  failure document + exit code; `--fetch` absent ⇒ no fetch.
- Docs: `cli-surface.md` § `wt create`, `create-output-phases.md` (From line, `--json` stream
  rule), skill bundle, help-dump.

### W2 — Recorded base

- When `wt` creates the branch (positional, bare, with or without `--base`), write
  `git config branch.<branch>.wt-base <ref-as-typed-or-HEAD-label>` and
  `branch.<branch>.wt-base-sha <sha>` after a successful `worktree add`, inside the rollback
  scope (a failed config write rolls back like any other create failure). `--checkout`, `--reuse`
  and `--temp` write nothing.
- `wt list --json` records gain `base` and `base_sha` (null when unset — every pre-W2 worktree);
  `wt list --status` is unchanged (no new column in v1).
- Tests: config present after create, absent after checkout; list reflects it; rollback removes
  the branch (and so the config).
- Docs: `cli-surface.md` § `wt list`, `list-status-contract.md`, `worktree-layout.md` (branch ↔
  worktree relationship gains the base), skill bundle, help-dump.

### W3 — `--reuse` + `--base` is a usage error

- `wt create --reuse --base <ref>` exits `ExitInvalidArgs` (2) before any mutation:
  `Error: --reuse and --base cannot be combined` / `Why: --reuse keeps the existing worktree's
  branch, so a start-point has nothing to apply to` / `Fix: drop --base, or drop --reuse to create
  a new worktree`. Same for `--reuse` + `--fetch`.
- Tests: the conflict exits 2 with nothing created; `--reuse` alone unchanged.
- Docs: `create-branch-semantics.md` ("`--reuse` takes precedence over `--base`" becomes the
  conflict row, with a Design Decision), `cli-surface.md`, skill bundle gotcha, help-dump.

### W4 — `--pr <n>`

- `wt create --pr <n>` resolves the PR's head with
  `gh pr view <n> --json headRefName,isCrossRepository` (argv exec, 15 s timeout) and runs the
  `--checkout <headRefName>` path (local as-is, remote-only fetched then checked out). Exclusive
  with the positional, `--base`, `--checkout`, `--reuse`, `--temp`. `isCrossRepository: true` is
  exit 2 (`Fix: add the fork as a remote and use --checkout`); `gh` missing or failing is exit 1
  with the `gh` error's first line. The `From:` line reads `PR #<n> (existing branch '<head>')`.
- Tests: scripted `gh` fake on PATH (the repo's integration tests build the binary; a shell stub
  for `gh` in a temp PATH dir); fork case; missing gh.
- Docs: `cli-surface.md`, `create-branch-semantics.md` (a fourth mode row), skill bundle,
  help-dump.

### W5 — `--temp <ref>`

- `wt create --temp <ref>`: `git worktree add --detach <path> <sha>` at the resolved `<ref>`
  (`--fetch` honoured), random name prefixed `tmp-` (`--name` overrides, prefix still applied),
  no branch, no config. `wt list --json` marks it `temp: true` (`branch` null); `wt list --status`
  shows `(detached)` in the branch column. `wt delete <name>` removes it without branch prompts.
  Exclusive with the positional, `--checkout`, `--reuse`, `--pr`.
- Init: per the W5 intake decision — skipped by default with `--init` to opt in, or run as
  usual; whichever is chosen goes into `init-protocol.md`.
- Tests: detached HEAD at the SHA; list marking; delete; conflicts exit 2.
- Docs: `cli-surface.md`, `worktree-layout.md` (temp worktrees), `create-branch-semantics.md`
  (mode row), `init-protocol.md`, skill bundle, help-dump.

### R1 — Release (operator)

After W1–W5 are merged: from a clean `main` checkout, `just release patch`; watch the release
workflow; confirm the tap updated (`build-and-release.md`); `rk notify` with the tag. No daemon or
user action — `wt` is a binary consumers pick up on their next `brew upgrade` / `wt update`.

---

## Downstream (hexokit — not steps of this plan)

Once R1 has shipped and the box has the new `wt`:

- `internal/riff`: pass `--fetch --json` and read the receipt instead of the path line; drop the
  `--base` SHA pre-resolution callers do today.
- `internal/autopilot/spawn.go`: delete the engine's own `git fetch` + `rev-parse`; the receipt's
  `base_sha`/`branch` replace the engine's stored copies; the queue entry's `base` field (plan
  `26-11-20-operator-triggers.md` T8) maps straight onto `--base`.
- `fab status set-base-branch` becomes derivable from `branch.<name>.wt-base` for a fab change;
  a plain agent's "open a draft PR against `<base>`" likewise.
- The operator run's Merging rules use `wt create --temp <head-sha>` / `--pr <n>` for restacks and
  conflict resolution instead of raw `git worktree add --detach`.

## Open decisions (resolve at each step's intake)

| Step | Question |
|---|---|
| W1 | Exact `--json` envelope: the toolkit standard's `{"ok","result"}` vs `wt list --json`'s bare array — one shape for the whole binary, decided once |
| W1 | Whether `--fetch` without `--base` fetches `origin` and re-resolves `HEAD`'s upstream, or is a usage error (prefer: fetch `origin`, base stays HEAD) |
| W2 | Whether `wt list --status` grows a `BASE` column (prefer: JSON only in v1) |
| W5 | Init on `--temp`: skip by default vs run |

## Non-goals

- Dependency code (cherry-picks, stacked bases from a dependency), PR creation or merging, tabs,
  prompts — the callers' policy.
- A persistent `wt` state file of any kind (Constitution I).
- Changing `wt go` / `wt open` / `wt delete` beyond what `--temp` listing needs.
