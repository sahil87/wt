# Intake: `wt create --fetch` and the `--json` receipt

**Change**: 261123-xhgg-create-fetch-json-receipt
**Created**: 2026-11-23

## Origin

> Execute step W1 (--fetch and the --json receipt on wt create) of the plan at /home/sahil/code/sahil87/wt/fab/plans/sahil/26-11-23-worktree-base-receipts.md. Read the whole plan first (Decisions and Rules bind you), then implement only the W1 Brief.

One-shot `/fab-new` invocation by the plan operator. The plan (`fab/plans/sahil/26-11-23-worktree-base-receipts.md`, in the main checkout at `/home/sahil/code/sahil87/wt/`) was read in full; its **Decisions already taken** table and **Rules for the operator** bind this change and are not reopened here. Only the **W1 Brief** is in scope — W2 (recorded base in git config), W3 (`--reuse`+`--base` usage error), W4 (`--pr`) and W5 (`--temp`) are separate later changes. The plan's two W1 open decisions (envelope shape; `--fetch` without `--base`) are resolved below in the Assumptions table.

Plan decisions that bind W1 (verbatim intent):

- **Fresh base**: an explicit `--fetch` flag, never an implicit fetch.
- **Receipt**: `wt create --json` prints **one JSON document on stdout and nothing else**. Without `--json`, stdout stays the path as the last line (`create-output-phases.md`).
- **Exit codes**: no new codes — `--fetch` failing is `ExitGitError` (3).
- **Back-compat**: every change is additive; no existing flag changes meaning.
- **Rules**: flag parsing/output in `src/cmd/wt/create.go`, every git operation in `src/internal/worktree/` (Constitution II/V); unit tests per path + an integration case in `src/cmd/wt/integration_test.go` against a real temp repo (bare origin + clone) (Constitution IV); CLI ⇒ docs in the same change (`docs/specs/cli-surface.md`, `wt skill` bundle via `just sync-skill`, `wt help-dump` fixtures, touched memory contracts, each with a Design Decision citing the plan); flags per `flag-naming-conventions.md` (long names, no new short flags).
- **Review bar (must-fix)**: a worktree or branch left behind on failure, a wrong base, a changed exit code, stdout polluted under `--json`.

## Why

`wt create` already does the worktree half of agent spawning, but callers re-implement the last steps around it, each slightly differently:

1. **Stale base.** `--base origin/eng-v2` uses the remote-tracking ref as the local clone last saw it. hexokit's engine (`internal/autopilot/spawn.go`) runs `git fetch origin` + `git rev-parse` itself to pin a fresh SHA before calling `wt create --base <sha>`; an agent spawning by hand usually forgets the fetch and cuts its branch from a stale commit.
2. **No receipt.** `create` prints the worktree path as its last stdout line; branch, base and resolved SHA must be re-derived by the caller (hexokit `internal/riff` parses the path line; the engine keeps its own copy of the branch, which went stale when `/fab-new` renamed it).
3. **Moving ref.** Today `--base <ref>` is passed to `git worktree add -b` as the ref name; if the ref moves mid-command (a concurrent fetch) the branch start-point is whatever the ref says at that instant, not what `wt` validated.

If not fixed, every caller keeps its own fetch+rev-parse+parse-path glue, and hand-spawned agents keep basing work on stale commits. `wt create` is the one place a worktree gets made, so freshness and the structured receipt belong there (plan Goal). Alternatives rejected by the plan: an implicit fetch on every create (slow, non-deterministic for tests); last-line parsing (fragile, carries only the path).

## What Changes

### 1. `--fetch` flag (bool, default `false`)

New flag on `wt create`: `cmd.Flags().BoolVar(&fetch, "fetch", false, "Fetch the base's remote before resolving --base (default remote: origin)")` — no short flag.

Behavior:

- **When**: after flag-mutex validation and `ValidateGitRepo`/`GetRepoContext`, and **before** the `--base` validation/resolution (so a ref that only exists after the fetch resolves). No worktree/branch mutation has happened yet, so a fetch failure leaves nothing behind.
- **Which remote**: if `--base` is of the form `<remote>/<rest>` and `<remote>` is a configured remote (membership in `git remote` output — not a naive string split, because local branches like `feature/x` contain slashes), fetch `<remote>`; otherwise fetch `origin`. Examples: `--base origin/eng-v2` → `git fetch origin`; `--base upstream/main` (with an `upstream` remote) → `git fetch upstream`; `--base feature/x` (no `feature` remote) → `git fetch origin`; `--base f68be281` → `git fetch origin`; no `--base` → `git fetch origin`, base stays `HEAD`.
- **Command**: `git fetch <remote>` (the whole remote, argv exec, no shell). Output captured (not streamed to stdout — `--json` stdout must stay clean).
- **Failure**: `ExitGitError` (3) with the structured `Error/Why/Fix`, naming the remote, e.g.
  - `Error: Failed to fetch remote 'origin'`
  - `Why: <first line / trimmed git output>`
  - `Fix: Check the remote URL and your network/credentials (git remote -v), or drop --fetch to use the local ref`
  Nothing is created.
- **With `--checkout`**: no separate fetch. A remote-only `--checkout` branch is already fetched by the existing `FetchRemoteBranch` path (no second fetch); a local `--checkout` branch is checked out as-is (wt never updates local branches). `--fetch` + `--checkout` is accepted (not an error) and is a no-op.
- **With `--reuse`**: when the reuse path is taken, no fetch runs — mirroring how `--base` validation is skipped under `--reuse` today. (W3 later makes `--reuse` + `--fetch` a usage error; W1 does not.)
- **Without `--fetch`**: no fetch is run — the git commands executed and stdout are identical to today.
- **Placement**: the fetch is a git operation, so it lives in `src/internal/worktree/` (e.g. `git.go`): something like `FetchRemote(remote string) error` and `RemoteForRef(ref string) string` (returns the remote whose name is the ref's first path segment, else `"origin"`). `create.go` only calls them and maps the error to exit 3.

### 2. Resolve the base to a SHA once; pin the new branch to it

- For the two new-branch modes (positional and bare exploratory), resolve the start-point once with `git rev-parse --verify <base>^{commit}` (`--base` given) or `git rev-parse --verify HEAD^{commit}` (no `--base`), **before** the create-dispatch switch (outside the reinstall window — same rule that resolves `createdSummaryFrom` pre-dispatch, `create-output-phases.md`).
- This replaces the current `git rev-parse --verify <base>` validation: a `--base` that does not resolve to a commit stays `ExitInvalidArgs` (2) with the existing `Invalid --base ref` Error/Why/Fix copy (a tag pointing at a non-commit now also fails here, which is correct — it could never be a branch start-point).
- The resolved full SHA is passed as the `startPoint` to `CreateNewBranchWorktree` / `CreateExploratoryWorktree` → `git worktree add -b <branch> <path> <sha>`, so the branch is pinned even if the ref moves during the command.
- Resolution helper lives in `internal/worktree` (e.g. `ResolveCommit(ref string) (string, error)`).
- **`From:` summary line amendment** (`create-output-phases.md`): when `--base` was given, the line keeps the ref verbatim and appends ` @ <short-sha>` (7-char abbreviation of the resolved SHA, e.g. `From: origin/eng-v2 @ f68be28`). The no-`--base` (HEAD label) and `--checkout` `From:` values are unchanged. `--reuse` still emits no summary block.

### 3. `--json` flag (bool, default `false`) — the receipt

New flag: `cmd.Flags().BoolVar(&jsonOut, "json", false, "Print one JSON result document on stdout (implies --non-interactive and --open skip)")` — no short flag.

**Stream rule**: under `--json`, stdout carries **exactly one JSON document** (single trailing newline) and nothing else. All phase separators, the Git-phase summary block, warnings, the `Reusing existing worktree:` line, init output and error copy stay on stderr exactly as today. The final `fmt.Println(wtPath)` is replaced by the document (not printed in addition).

**Implications**: `--json` implies `--non-interactive` (no dirty-state menu, no name prompt, no init confirm, no open-anyway prompt) and `--open skip` (a menu or an `open_here` path emission cannot share stdout with a document). An **explicitly set** `--open` (or deprecated `--worktree-open`) with any value other than `skip` combined with `--json` is a usage error, `ExitInvalidArgs` (2), before any mutation:
- `Error: --json cannot be combined with --open <value>`
- `Why: --json reserves stdout for one JSON document; opening an app can write to stdout or prompt`
- `Fix: Drop --open (it defaults to skip under --json), or drop --json`

**Success document** (envelope `{"ok": true, "result": {...}}`):

```json
{"ok":true,"result":{"path":"/home/u/code/repo.worktrees/mp-f1","name":"mp-f1","branch":"mp-f1","base_ref":"origin/eng-v2","base_sha":"f68be281ad0c…40-hex…","mode":"new","init":"ok"}}
```

Fields (all keys always present):

| Key | Type | Value |
|-----|------|-------|
| `path` | string | absolute worktree path (what the path line prints today) |
| `name` | string | worktree directory name |
| `branch` | string | branch checked out in the worktree (new branch name; `--checkout` target; for `--reuse`, the existing worktree's current branch, best-effort via `git -C <path> rev-parse --abbrev-ref HEAD`, `null` if it cannot be read or is detached) |
| `base_ref` | string\|null | `--base` verbatim as typed; with no `--base`, the HEAD label the `From:` line shows (`wt.DescribeHead()`: branch name, or short SHA when detached); `null` for `checkout`/`reuse` |
| `base_sha` | string\|null | full 40-hex resolved start-point SHA; `null` for `checkout`/`reuse` |
| `mode` | string | `new` (positional or bare exploratory) \| `checkout` \| `reuse`. (`temp` is reserved for W5 and never emitted by W1.) |
| `init` | string | `ok` (init ran and exited 0) \| `failed` (init ran and exited non-zero) \| `skipped` (`--no-init`, or the built-in `fab sync` default-not-applicable exit-3 skip) \| `not-found` (init command/file not found — the graceful structured not-found skip) |

**Failure document** (envelope `{"ok": false, "error": {...}}`), exit code unchanged:

```json
{"ok":false,"error":{"code":3,"message":"Failed to fetch remote 'origin'"}}
```

- `error.code` is the integer exit code the process exits with (wt's typed codes, `errors.go` — Constitution III is the stable contract, so the code mirrors it rather than inventing string codes).
- `error.message` is the `Error:` (what) line of the structured error; the full `Error/Why/Fix` still prints to stderr.
- Emitted for every failure `wt create`'s `RunE` raises (all current `wt.ExitWithError` / `PrintError`+`os.Exit` sites in create, the new fetch/resolve/`--open` conflicts, and the SIGINT/SIGTERM rollback path → `{"ok":false,"error":{"code":130,"message":"interrupted"}}`).
- **Init failure (exit 7)**: the worktree is kept, so the document is `{"ok":false,"error":{"code":7,"message":"init script exited with status N"},"result":{…same receipt…,"init":"failed"}}` — `ok` always equals `exit == 0`, and the kept worktree's receipt rides along as an optional `result` (additive to the plan's failure shape).
- **Reuse with a failing init**: today non-fatal (warning, exit 0) — stays so: `ok:true`, `init:"failed"`, exit 0.
- Out of scope: cobra-level parse errors raised before `RunE` (unknown flag, >1 positional) keep today's behavior (stderr message, exit 1, empty stdout) — the document contract covers what `RunE` detects.

**Implementation shape** (Constitution V): the receipt struct + JSON marshalling and an init-outcome type can live in `internal/worktree` (mirroring `worktree.go`'s `list --json` record); `create.go` assembles and prints. Failure emission is wired in `create.go` via a create-local fail helper that prints the stderr `Error/Why/Fix` (via `wt.PrintError`) and, under `--json`, the failure document on stdout, then `os.Exit(code)` — replacing `wt.ExitWithError` call sites inside create's `RunE`. The init runner (`RunWorktreeSetupWithObserver`) needs to report *which* nil-error outcome happened (ran ok / not-found / default skip) — add an outcome-returning variant and keep the existing signatures as thin wrappers so `wt init` and other callers are untouched.

### 4. Tests (Constitution IV)

- **Unit** (`src/internal/worktree/*_test.go`): `RemoteForRef` (configured remote prefix, slash branch with no matching remote, bare SHA, empty); `ResolveCommit` (branch, tag, bad ref); `FetchRemote` failure on a nonexistent remote; receipt JSON marshalling (null `base_ref`/`base_sha` for checkout/reuse; all keys present).
- **cmd** (`src/cmd/wt/create_test.go`): `--json` stdout is exactly one parseable document and nothing else (per mode: bare, positional, `--base`, `--checkout`, `--reuse`); failure document + exit code (e.g. invalid `--base` → code 2, branch-exists → code 2); `--json --open code` → exit 2, nothing created; `--base` pinned to SHA (new branch tip == `rev-parse <base>` at create time); `From:` line carries ` @ <short-sha>` with `--base`.
- **Integration** (`src/cmd/wt/integration_test.go`, real temp repos: a bare origin + a clone): **fetch happens** — push a commit to the bare origin from a second clone after the first clone's last fetch, then `wt create --base origin/main --fetch --json` → `base_sha` equals the new commit and the worktree HEAD is at it; **`--fetch` absent ⇒ no fetch** — same setup without `--fetch` → `base_sha` is the stale commit; **fetch failure** — `--fetch` with a broken/nonexistent origin → exit 3, failure document, no worktree dir and no branch created; init outcome values (`skipped` with `--no-init`, `ok`/`failed` with a `WORKTREE_INIT_SCRIPT` exiting 0/1 → exit 7 with `result.init:"failed"`).

### 5. Docs (CLI ⇒ docs, same change)

- `docs/specs/cli-surface.md` § `wt create`: `--fetch` and `--json` flag rows; the `--json` stdout rule (one document) next to the "path is the last line" paragraph; SHA pinning; the `--json`+`--open` conflict in the exit-code paragraph.
- `docs/memory/wt-cli/create-output-phases.md`: `From:` ` @ <short-sha>` amendment; the `--json` stream rule (stdout = one document; all phase output stays stderr; path line replaced); Design Decision citing the plan.
- `docs/memory/wt-cli/create-branch-semantics.md`: `--base` now resolved once to a SHA and passed as start-point; `--fetch` remote selection; `--fetch` no-op with `--checkout`/`--reuse`; Design Decision citing the plan.
- `docs/memory/wt-cli/flag-naming-conventions.md` and `toolkit-standards-conformance.md`: note the two new flags / the `--json` envelope decision (`{ok,result}` / `{ok,error}` for new `--json` surfaces; `wt list --json` keeps its bare array) if those files track per-flag / per-principle conformance.
- `wt skill` bundle: edit `docs/site/skill.md` (canonical) and run `just sync-skill` to refresh `src/cmd/wt/skill.md`.
- `wt help-dump` fixtures/tests: update whatever golden/fixture the help-dump tests pin so the new flags appear.

## Affected Memory

- `wt-cli/create-output-phases`: (modify) `From:` line ` @ <short-sha>` suffix when `--base` resolved; `--json` stdout-is-one-document stream rule; failure/init-failure document shapes
- `wt-cli/create-branch-semantics`: (modify) `--base` resolved once to a SHA and used as the pinned start-point; `--fetch` (remote selection, timing, failure → exit 3, no-op with `--checkout`/`--reuse`); `--json` implies non-interactive/open-skip and conflicts with an explicit non-skip `--open`
- `wt-cli/flag-naming-conventions`: (modify) record `--fetch` / `--json` on `wt create` as conforming long-only flags
- `wt-cli/toolkit-standards-conformance`: (modify) principle №2 — `wt create --json` receipt and the envelope decision (`wt list --json` bare array preserved)
- `wt-cli/help-dump-contract`: (modify) only if the contract file enumerates per-command flags/fixtures

## Impact

- **Code**: `src/cmd/wt/create.go` (two flags, `--json` gating/conflict, fail helper, receipt assembly, From-line suffix, pre-dispatch SHA resolution); `src/internal/worktree/git.go` (fetch, remote selection, commit resolution), `crud.go` (outcome-reporting init runner variant), a receipt type (new file or `worktree.go`).
- **Tests**: `create_test.go`, `integration_test.go`, internal `*_test.go`, help-dump tests.
- **Docs**: `docs/specs/cli-surface.md`, `docs/site/skill.md` + `src/cmd/wt/skill.md` (synced), memory files above.
- **Downstream**: none broken — both flags are opt-in; without them stdout is identical. One subtle side effect of SHA pinning (see Assumption 9): with a remote-tracking `--base`, git's `branch.autoSetupMerge` no longer sets the new branch's upstream to that remote branch.
- **No new exit codes**; no new dependencies; no persistent state (Constitution I).

## Open Questions

- None blocking. The two plan-level W1 open decisions are resolved in Assumptions 1 and 2.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Envelope for `wt create --json` is `{"ok":true,"result":{…}}` / `{"ok":false,"error":{"code","message"}}`; `wt list --json` keeps its bare array (not migrated) | Plan Goal example + Brief failure shape fix the create envelope; `shll standards` defines no envelope; changing `list --json` would break `hop ls --trees` and violate the plan's Back-compat decision. "One shape" = the envelope for every new `--json` surface | S:75 R:60 A:75 D:60 |
| 2 | Certain | `--fetch` without `--base` fetches `origin`; base stays `HEAD` (no usage error) | Plan's stated preference for this open decision | S:85 R:85 A:90 D:85 |
| 3 | Confident | Remote for `--fetch` = the ref's first path segment only if it is a configured remote (`git remote`), else `origin` | Brief says `remote/branch` → remote else origin; local branches with slashes (`feature/x`) must not be mistaken for remotes | S:75 R:85 A:80 D:70 |
| 4 | Confident | `--fetch` + `--checkout` is accepted and a no-op; `--fetch` under a taken `--reuse` path is skipped (no error in W1) | Brief: "no second fetch" for checkout; `--base` validation is already skipped under reuse; W3 owns the reuse conflict | S:65 R:85 A:75 D:65 |
| 5 | Confident | `error.code` is the integer exit code; `error.message` is the structured error's `Error:` line | Constitution III makes typed exit codes the stable contract; Brief lists `{"code","message"}` without a type | S:60 R:70 A:75 D:60 |
| 6 | Confident | Init failure (exit 7) under `--json` emits `ok:false` + `error{code:7}` + an optional `result` receipt with `init:"failed"` | Brief lists `init:"failed"` (so a receipt must exist on init failure) and says exit codes unchanged; keeping `ok == (exit==0)` and adding `result` is additive | S:60 R:80 A:60 D:50 |
| 7 | Confident | Explicit non-`skip` `--open` with `--json` is exit 2 before any mutation; an unset `--open` silently defaults to skip | Plan's own lesson (#4, `--reuse` silently winning) argues against silent precedence; `open_here` writes to stdout so cannot coexist with the document | S:60 R:80 A:70 D:60 |
| 8 | Confident | `From:` ` @ <short-sha>` suffix only when `--base` was given; HEAD-label and `--checkout` lines unchanged | Brief scopes resolution to "the base"; a detached-HEAD label is already a short SHA (would render `abc1234 @ abc1234`); minimizes stderr churn | S:55 R:90 A:70 D:55 |
| 9 | Confident | SHA pinning applies to every new-branch create (with or without `--base`, with or without `--fetch`); accepted side effect: no auto-upstream to a remote-tracking `--base` | Brief mandates pinning unconditionally ("byte-identical" read as stdout/fetch behavior); auto-tracking `eng-v2` from an agent branch was an accidental git default, but it is a behavior change worth surfacing | S:55 R:65 A:45 D:40 |
| 10 | Confident | `base_ref` with no `--base` is the `wt.DescribeHead()` label (what `From:` shows); `base_sha` is the resolved HEAD SHA | Plan W2 records "ref-as-typed-or-HEAD-label" — same label keeps W1 receipt and W2 config consistent | S:70 R:80 A:80 D:65 |
| 11 | Confident | Init outcome mapping: `--no-init` and `fab sync` default-not-applicable → `skipped`; structured not-found → `not-found`; exit 0 → `ok`; non-zero → `failed` | Brief enum `ok / failed / skipped / not-found`; default-not-applicable is a documented skip, not a missing script | S:65 R:85 A:75 D:65 |
| 12 | Confident | Cobra parse errors before `RunE` (unknown flag, extra positional) stay stderr-only, exit 1, no document | Raised by cobra before create can read `--json`; empty stdout + non-zero exit is still unambiguous for callers; keeps main.go's root handler unchanged | S:50 R:80 A:70 D:60 |
| 13 | Certain | No short flags; long `--fetch` / `--json` only; no new exit codes | Plan Rules 4 and Exit-codes decision; `flag-naming-conventions.md` | S:95 R:90 A:95 D:95 |

13 assumptions (2 certain, 11 confident, 0 tentative, 0 unresolved).
