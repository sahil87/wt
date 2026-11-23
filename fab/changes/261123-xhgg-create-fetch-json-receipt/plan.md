# Plan: `wt create --fetch` and the `--json` receipt

**Change**: 261123-xhgg-create-fetch-json-receipt
**Intake**: `intake.md`

## Requirements

### wt create: `--fetch`

#### R1: `--fetch` fetches the base's remote before the base is resolved
`wt create` SHALL accept a long-only bool flag `--fetch` (default `false`, no short flag). When set, and the create is a new-branch create (positional or bare exploratory), `wt create` MUST run `git fetch <remote>` after flag-mutex validation and the git-repo check and BEFORE the `--base` ref is resolved, where `<remote>` is the first `/`-separated segment of `--base` if and only if that segment is a configured remote (`git remote` output), otherwise `origin`. With no `--base`, `<remote>` is `origin` and the base stays `HEAD`. The fetch command MUST be argv-exec'd (no shell) and its output MUST NOT reach stdout.

- **GIVEN** a clone whose `origin/main` is one commit behind the bare origin
- **WHEN** `wt create --base origin/main --fetch` runs
- **THEN** `git fetch origin` runs first and the new branch's tip is the bare origin's newest commit
- **AND** `--base feature/x` (no `feature` remote) fetches `origin`; `--base upstream/main` with an `upstream` remote fetches `upstream`

#### R2: A failed fetch is `ExitGitError` with nothing created
When the fetch exits non-zero, `wt create` MUST exit `ExitGitError` (3) with a structured `Error/Why/Fix` naming the remote (`Error: Failed to fetch remote '<remote>'`; `Why:` the trimmed git output; `Fix:` names `git remote -v` and dropping `--fetch`) and MUST NOT create any worktree directory or branch.

- **GIVEN** `origin` points at a nonexistent path
- **WHEN** `wt create --non-interactive --fetch --name x newbr` runs
- **THEN** the exit code is 3, stderr names `origin`, and neither `<repo>.worktrees/x` nor branch `newbr` exists

#### R3: `--fetch` is a no-op for `--checkout` and for a taken `--reuse` path; absent ⇒ no fetch
`--fetch` combined with `--checkout` MUST be accepted and MUST NOT run a separate fetch (the remote-only checkout path already fetches via `FetchRemoteBranch`; a local branch is checked out as-is). When `--reuse` takes the reuse path, no fetch runs. Without `--fetch`, `wt create` MUST NOT run any `git fetch` it did not run before this change.

- **GIVEN** a clone one commit behind origin
- **WHEN** `wt create --base origin/main` runs WITHOUT `--fetch`
- **THEN** the new branch tip is the stale (pre-push) `origin/main` commit

### wt create: base pinning

#### R4: The new-branch start-point is resolved once to a full SHA and pinned
For the positional and bare exploratory modes, `wt create` MUST resolve the start-point once — `git rev-parse --verify <base>^{commit}` when `--base` is given, else `git rev-parse --verify HEAD^{commit}` — BEFORE the create dispatch (outside the reinstall window), and MUST pass that full SHA as the `git worktree add -b` start-point. A `--base` that does not resolve to a commit MUST stay `ExitInvalidArgs` (2) with the existing `Invalid --base ref: <ref>` Error/Why/Fix copy, before any mutation. The existing `--reuse`-skips-`--base`-validation behavior is preserved.

- **GIVEN** `--base feature-A`
- **WHEN** `wt create` runs
- **THEN** the worktree HEAD equals `git rev-parse feature-A` taken before the command, and `git worktree add` received the 40-hex SHA, not the ref name

<!-- rework: cycle 1 — SHA pinning dropped git's branch.autoSetupMerge upstream for a remote-tracking --base, silently disabling wt delete's unpushed-commits guard and wt list --status ↑N -->
Pinning MUST NOT change the new branch's upstream relative to what `git worktree add -b <branch> <path> <base-ref>` would have produced: when `--base` names a remote-tracking branch (it resolves under `refs/remotes/`) and `branch.autoSetupMerge` is not `false`, `wt create` MUST set the new branch's upstream to that remote-tracking branch (e.g. `git branch --set-upstream-to=<base> <branch>`) after the successful add, inside the rollback scope (a failure rolls back like any other create failure, exit 3). When `branch.autoSetupMerge` is `always`, a local-branch `--base` MUST also get its upstream set, mirroring git. No upstream is set for a SHA, tag, `HEAD`, or no `--base`. The git operations live in `internal/worktree`.

- **GIVEN** `wt create --non-interactive --name w nb --base origin/main` in a clone
- **WHEN** it succeeds
- **THEN** `git rev-parse --abbrev-ref nb@{upstream}` prints `origin/main` (identical to pre-change behavior) and the branch tip is the pinned SHA
- **AND** with `--base main` (local) and default config, `nb` has no upstream (as before)

#### R5: `From:` line appends ` @ <short-sha>` when `--base` was given
When `--base` was given, the Git-phase summary `From:` line MUST be `From: <ref-as-typed> @ <short-sha>` (7-character abbreviation of the resolved SHA). The no-`--base` (HEAD label) and `--checkout` `From:` values MUST be unchanged; `--reuse` still prints no summary block.

- **GIVEN** `--base main`
- **WHEN** `wt create --non-interactive --name w nb --base main` runs
- **THEN** stderr contains `From: main @ ` followed by the first 7 hex chars of `main`'s SHA

### wt create: `--json` receipt

#### R6: `--json` stdout is exactly one JSON document
`wt create` SHALL accept a long-only bool flag `--json`. Under `--json`, stdout MUST carry exactly one JSON document terminated by a single newline and nothing else, on every exit path `RunE` controls (success, every failure, init failure, SIGINT/SIGTERM rollback). All phase separators, the summary block, warnings, the `Reusing existing worktree:` line, init output and structured errors MUST stay on stderr. The final path line is replaced by the document (not printed in addition). The SIGINT handler's framing newline MUST NOT be written to stdout under `--json`.

- **GIVEN** any successful `wt create --json …`
- **WHEN** stdout is parsed
- **THEN** `json.Unmarshal` of the whole stdout succeeds into one object and stdout contains exactly one line

#### R7: `--json` implies `--non-interactive` and `--open skip`; an explicit non-skip `--open` is a usage error
Under `--json`, all prompts/menus MUST be skipped exactly as with `--non-interactive`, and the open phase MUST default to `skip`. If `--open` (or deprecated `--worktree-open`) is explicitly set to any value other than `skip` together with `--json`, `wt create` MUST exit `ExitInvalidArgs` (2) before any mutation with `Error: --json cannot be combined with --open <value>` / `Why: --json reserves stdout for one JSON document; opening an app can write to stdout or prompt` / `Fix: Drop --open (it defaults to skip under --json), or drop --json`, and emit the failure document.

- **GIVEN** `wt create --json --open code --name w nb`
- **WHEN** it runs
- **THEN** exit 2, stdout is `{"ok":false,"error":{"code":2,…}}`, and no worktree `w` exists
- **AND** `wt create --json --open skip …` succeeds

#### R8: Success document shape
On success the document MUST be `{"ok":true,"result":{"path","name","branch","base_ref","base_sha","mode","init"}}` with every key present:
`path` (absolute worktree path), `name` (worktree dir name), `branch` (new branch name / `--checkout` target / for `--reuse` the existing worktree's current branch, `null` when unreadable or detached), `base_ref` (`--base` verbatim; with no `--base` the `wt.DescribeHead()` label; `null` for `checkout`/`reuse`), `base_sha` (full 40-hex start-point; `null` for `checkout`/`reuse`), `mode` (`new` | `checkout` | `reuse`; `temp` reserved, never emitted), `init` (`ok` | `failed` | `skipped` | `not-found`).

- **GIVEN** `wt create --json --name mp-f1 --base main`
- **WHEN** it succeeds
- **THEN** `result` = `{path:<abs>, name:"mp-f1", branch:"mp-f1", base_ref:"main", base_sha:<rev-parse main>, mode:"new", init:<outcome>}`
- **AND** a `--checkout` create has `mode:"checkout"`, `base_ref:null`, `base_sha:null`; a `--reuse` hit has `mode:"reuse"`, `base_ref:null`, `base_sha:null`

#### R9: Init outcome mapping
`init` MUST be: `skipped` when init is disabled (`--no-init` / `--worktree-init false`) or the built-in `fab sync` default-not-applicable exit-3 skip fired; `not-found` when the structured init not-found skip fired; `ok` when the init script ran and exited 0; `failed` when it ran and exited non-zero. The init runner MUST expose which outcome occurred without changing the existing `RunWorktreeSetup` / `RunWorktreeSetupWithObserver` signatures or behavior for other callers (`wt init`).

- **GIVEN** the test env `WORKTREE_INIT_SCRIPT=__wt_test_noinit__ noop`
- **WHEN** `wt create --json` runs
- **THEN** `init` is `not-found`; with `--no-init` it is `skipped`; with a script exiting 0 it is `ok`

#### R10: Failure document shape; exit codes unchanged
On any failure `RunE` raises under `--json`, the document MUST be `{"ok":false,"error":{"code":<int exit code>,"message":<the Error: line>}}` and the process MUST exit with the same code it exits with today (or the new R2/R7 codes). The SIGINT/SIGTERM rollback path MUST emit `{"ok":false,"error":{"code":130,"message":"interrupted"}}`. On an init failure (exit 7, worktree kept) the document MUST additionally carry `result` (the success receipt with `init:"failed"`): `{"ok":false,"error":{"code":7,"message":…},"result":{…}}`. A `--reuse` hit whose init fails stays non-fatal: `ok:true`, `init:"failed"`, exit 0. `ok` MUST equal `exit code == 0` in every document. Cobra parse errors raised before `RunE` (unknown flag, >1 positional) are out of scope and keep today's behavior.

- **GIVEN** `wt create --json --base nonexistent-ref`
- **WHEN** it runs
- **THEN** exit 2 and stdout is `{"ok":false,"error":{"code":2,"message":"Invalid --base ref: nonexistent-ref"}}`
- **AND** with a failing init script, exit 7 and `result.init == "failed"`, `result.path` exists on disk

### Docs

#### R11: CLI ⇒ docs in the same change
`docs/specs/cli-surface.md` § `wt create` MUST document `--fetch`, `--json` (document shapes, implications, `--open` conflict), SHA pinning and the `From:` suffix; `docs/site/skill.md` MUST mention `--fetch` / `--json` and the receipt as the machine surface, and `src/cmd/wt/skill.md` MUST be re-synced via `just sync-skill` (drift-guard test stays green). `wt help-dump` is generated from the cobra tree, so the new flags appear there automatically; its tests MUST stay green.

- **GIVEN** the change is applied
- **WHEN** `go test ./...` runs
- **THEN** the skill drift-guard and help-dump tests pass and `wt create -h` lists `--fetch` and `--json`

### Non-Goals

- Recording the base in git config (`branch.<name>.wt-base`) — W2.
- `--reuse` + `--base` / `--fetch` conflict — W3; W1 keeps `--reuse` precedence.
- `--pr`, `--temp` — W4/W5; `mode:"temp"` is reserved only.
- Migrating `wt list --json` to the envelope — would break `hop ls --trees` (plan back-compat).
- A document for cobra-level parse errors raised before `RunE`.

### Design Decisions

#### Envelope for new `--json` surfaces; `wt list --json` keeps its bare array
**Decision**: `wt create --json` emits `{"ok":true,"result":…}` / `{"ok":false,"error":{"code","message"}[,"result"]}`; `wt list --json` is unchanged.
**Why**: a create can fail, so the document needs an `ok` discriminator and an error slot; `shll standards` defines no envelope; changing `list --json` breaks `hop ls --trees` (plan Back-compat decision).
**Rejected**: bare result object (no failure channel on stdout); migrating `list --json` (breaking).
*Introduced by*: 261123-xhgg-create-fetch-json-receipt

#### `error.code` is the integer exit code; `ok == (exit == 0)`
**Decision**: the failure document's `code` mirrors the process exit code; an init failure (exit 7) is `ok:false` with the kept worktree's receipt in an optional `result`.
**Why**: Constitution III makes the typed exit codes the stable contract; one invariant (`ok` ⇔ exit 0) is easier for callers than per-case rules, and the `result` keeps the kept worktree discoverable.
**Rejected**: string error codes (a second code namespace to keep stable); `ok:true` on exit 7 (contradicts the exit code).
*Introduced by*: 261123-xhgg-create-fetch-json-receipt

#### Pin every new-branch create to a resolved SHA
**Decision**: the start-point is always `rev-parse --verify <base|HEAD>^{commit}`, resolved pre-dispatch and passed as a SHA to `git worktree add -b`.
**Why**: the branch cannot move with the ref mid-command, and the receipt's `base_sha` is exactly what the branch was cut from.
**Rejected**: passing the ref name (racy, and `base_sha` could disagree with the branch). Passing a SHA loses git's `branch.autoSetupMerge` upstream for a remote-tracking `--base`, so `wt` restores it explicitly after the add (review cycle 1: the lost upstream silently disabled `wt delete`'s unpushed-commits guard and `wt list --status` ↑N); dropping the upstream was rejected as a data-safety regression.
*Introduced by*: 261123-xhgg-create-fetch-json-receipt

## Tasks

### Phase 1: Internal building blocks (`src/internal/worktree/`)

- [x] T001 [P] In `src/internal/worktree/git.go` add `RemoteForRef(ref string) string` (first `/` segment iff it is in `git remote` output, else `"origin"`), `FetchRemote(remote string) error` (`git fetch <remote>`, CombinedOutput, error wraps trimmed output), `ResolveCommit(ref string) (string, error)` (`git rev-parse --verify <ref>^{commit}`, trimmed full SHA) and `ShortSHA(sha string) string` (7-char prefix via a named const). Unit tests in `src/internal/worktree/git_test.go` (follow its existing temp-repo pattern): configured-remote prefix, slash branch without matching remote, bare SHA, empty ref; ResolveCommit on branch / bad ref; FetchRemote error on a nonexistent remote. <!-- R1 R2 R4 -->
- [x] T002 [P] New `src/internal/worktree/receipt.go`: `CreateMode` consts (`ModeNew="new"`, `ModeCheckout="checkout"`, `ModeReuse="reuse"`), `InitOutcome` string type with consts (`InitOK="ok"`, `InitFailed="failed"`, `InitSkipped="skipped"`, `InitNotFoundOutcome="not-found"` or similar non-clashing name — `InitNotFound` is already a type), `CreateReceipt` struct (`Path`, `Name string`; `Branch`, `BaseRef`, `BaseSHA *string` without `omitempty` so they marshal as `null`; `Mode`, `Init string`), and formatters `FormatCreateSuccessJSON(r CreateReceipt) (string, error)` / `FormatCreateFailureJSON(code int, message string, r *CreateReceipt) (string, error)` producing compact single-line JSON (no trailing newline; caller prints with Println). Unit tests in `src/internal/worktree/receipt_test.go`: all keys present, nulls for checkout/reuse, failure with and without `result`. <!-- R8 R10 -->
- [x] T003 [P] In `src/internal/worktree/crud.go` add `RunWorktreeSetupOutcome(wtPath, initScript string, isDefault bool, repoRoot string, observer func(*exec.Cmd)) (InitOutcome, error)` returning `not-found` / `skipped` (default-not-applicable) / `ok` / `failed`+err; make `RunWorktreeSetupWithObserver` a thin wrapper discarding the outcome (behavior and stderr output byte-identical). Unit test (e.g. in `src/internal/worktree/init_unix_test.go` or a new `crud_test.go`) covering not-found, ok, and failed outcomes. <!-- R9 -->

### Phase 2: `wt create` wiring (`src/cmd/wt/create.go`)

- [x] T004 Add `--fetch` and `--json` bool flags (long-only, help strings per intake). Under `--json`: force `nonInteractive = true`; if `cmd.Flags().Changed("open") || Changed("worktree-open")` with a value other than `skip` → R7 usage error; else default open to `skip`. Introduce a create-local `fail(code int, what, why, fix string)` helper that calls `wt.PrintError` (stderr) and, under `--json`, prints `FormatCreateFailureJSON(code, what, nil)` to stdout, then `os.Exit(code)`; route every `wt.ExitWithError` / `PrintError`+`os.Exit` site inside `RunE` through it (incl. `--reuse requires --name`). Make the SIGINT/SIGTERM rollback goroutine write the 130 failure document instead of the bare `fmt.Println()` when `--json`. <!-- R6 R7 R10 -->
- [x] T005 After repo validation and before the `--base` check: when `--fetch` and not `--checkout`, and not about to take the reuse path (reuse && the `--name` already collides — compute collision early or skip fetch when `reuse` is set and `CheckNameCollision` is true), call `wt.FetchRemote(wt.RemoteForRef(base))`; failure → `fail(ExitGitError, "Failed to fetch remote '<remote>'", <err>, "Check the remote URL and your network/credentials (git remote -v), or drop --fetch to use the local ref")`. Replace the `git rev-parse --verify <base>` exec in cmd/ with `wt.ResolveCommit` (keeping the `Invalid --base ref` copy and exit 2, skipped when `reuse`); for new-branch modes resolve `baseSHA` pre-dispatch (`ResolveCommit(base)` or `ResolveCommit("HEAD")`) and pass `baseSHA` as `startPoint` to `CreateNewBranchWorktree` / `CreateExploratoryWorktree`. When `--base` is set, `createdSummaryFrom = base + " @ " + wt.ShortSHA(baseSHA)`. Keep all resolution before the dispatch switch (reinstall-window rule). <!-- R1 R2 R3 R4 R5 -->
- [x] T006 Assemble the receipt: `mode` from the dispatch branch; `branch`; `base_ref` (= `base` or `createdSummaryFrom`'s HEAD label when no `--base`, null for checkout); `base_sha`; `init` from `RunWorktreeSetupOutcome` (or `skipped` when `runInit` is false). Replace the final `fmt.Println(wtPath)` with the success document under `--json`. Init-failure branch under `--json` (non-interactive): print `FormatCreateFailureJSON(ExitInitFailed, "init script exited with status N" / "init script did not complete", &receipt{init:"failed"})` before `os.Exit(7)`. Reuse path under `--json`: run init via the outcome variant (failure stays a warning), branch via `wt.BranchForPath` mapping `"unknown"`/`"HEAD"` → null, print `ok:true` document instead of the path. <!-- R6 R8 R9 R10 -->

### Phase 3: Tests

- [x] T007 In `src/cmd/wt/create_test.go` add: `--json` one-document-only for bare, positional+`--base`, `--checkout`, `--reuse` (parse with `encoding/json`, assert single stdout line and field values/nulls/mode); `init` values `not-found` (default test env), `skipped` (`--no-init`); failure documents for invalid `--base` (code 2) and positional-existing-branch (code 2); `--json --open code` → exit 2 + nothing created; `--json --open skip` succeeds; init-failure under `--json` → exit 7, `ok:false`, `result.init=="failed"`, path exists; `From: main @ <short>` with `--base`; `--base` pin (worktree HEAD == pre-command `rev-parse`). Update any existing assertions that break only because of the `From:` suffix. <!-- R4 R5 R6 R7 R8 R9 R10 -->
- [x] T008 In `src/cmd/wt/integration_test.go` add a bare-origin + clone fixture (clone `remote.git` into a second dir, push a new commit from another clone after the first clone's fetch): `--fetch --json --base origin/main` → `base_sha` == the new commit and worktree HEAD at it; same without `--fetch` → stale commit; `--fetch` with `origin` URL set to a nonexistent path → exit 3, failure document, no worktree dir, no branch; `--fetch` with `--checkout` of a local branch succeeds (no-op). Use `--no-init`/test env so no host side effects. <!-- R1 R2 R3 R6 R10 -->

### Phase 4: Docs

- [x] T009 [P] Update `docs/specs/cli-surface.md` § `wt create`: flag rows for `--fetch` and `--json`; the `--json` stdout rule and both document shapes next to the "path is the last line" paragraph; SHA pinning + `From: <ref> @ <short-sha>`; exit-code paragraph gains the `--fetch` failure (3) and `--json`+`--open` conflict (2); cite plan `fab/plans/sahil/26-11-23-worktree-base-receipts.md` W1. <!-- R11 -->
- [x] T010 [P] Update `docs/site/skill.md`: `create` bullet mentions `--fetch` (fresh base) and `--json` (receipt `{ok,result:{path,name,branch,base_ref,base_sha,mode,init}}`); the Machine-surface bullet names `wt create --json` alongside the path line. Run `just sync-skill` (or `./scripts/sync-skill.sh`) and run `cd src && go test ./...` to confirm skill drift-guard, help-dump and all tests pass; run `gofmt -l` clean. <!-- R11 -->

### Phase 5: Review cycle 1 rework

- [x] T011 Restore the upstream lost by SHA pinning (R4 amendment): in `src/internal/worktree/` (e.g. `git.go` + `crud.go`) add a helper that, given the typed `--base` and the new branch, decides per `branch.autoSetupMerge` (unset/`true` → only when the base resolves under `refs/remotes/`; `always` → also a local branch under `refs/heads/`; `false` → never; SHA/tag/HEAD/empty → never) and runs `git branch --set-upstream-to=<base> <branch>`. Call it from `CreateNewBranchWorktree` / `CreateExploratoryWorktree` after the successful add and after the rollback registrations (so a failure rolls back the worktree + branch) — thread the typed base ref alongside the SHA start-point. `create.go` maps a failure to exit 3 like other create failures. Unit tests for the decision helper; cmd tests in `src/cmd/wt/create_test.go`: `--base origin/main` (after pushing main to the fixture's bare origin) → `nb@{upstream}` == `origin/main`; `--base main` → no upstream; `wt list --status`/`HasUnpushedCommits` sees an unpushed commit on such a branch. <!-- R4 --> <!-- rework: cycle 2 — rollback is LIFO, so `git branch -D` (registered after `worktree remove`) runs first and fails on a checked-out branch: a set-upstream failure (and the pre-existing SIGINT rollback) leaves the new branch behind. Register `git branch -D <branch>` BEFORE `git worktree remove --force <path>` in CreateNewBranchWorktree and CreateExploratoryWorktree; add a cmd test forcing SetUpstream to fail (create `.git/config.lock` after pushing main) asserting exit 3, failure document, no worktree dir and no branch; and an internal/worktree test (or rollback_test) proving the registration order removes the worktree before the branch. Also: compute the UpstreamForBase decision pre-dispatch (pure lookup) and keep only the set-upstream write post-add; add a `branch.autoSetupMerge=inherit` arm mirroring git (copy the base branch's own upstream when it has one, else none); call `signal.Stop(sigCh)` after the final rb.Disarm() in create.go so a late SIGINT on a kept worktree cannot emit the 130 document --> <!-- rework: cycle 3 — UpstreamForBase returns any refs/remotes/ base, but git only auto-tracks a remote-tracking ref that a configured remote's fetch refspec maps to (validate_remote_tracking_branch/find_tracked_branch); a stray `refs/remotes/ghost/main` (no `ghost` remote / no covering refspec) now fails set-upstream → exit 3 where git succeeded untracked with exit 0. Fix: return a remote-tracking base only when some remote in `git remote` has a `remote.<r>.fetch` refspec whose destination pattern matches the full ref (glob `*` match on the dst side; handle `+` prefix and multiple fetch values). Tests: git_test.go `update-ref refs/remotes/ghost/main` → ""; create_test.go `--base ghost/main` → exit 0, no upstream. Also (should-fix): give a set-upstream failure its own error from trackUpstream (sentinel/wrapped) mapped in create.go to exit 3 with copy `Error: Failed to set upstream '<upstream>' for branch '<branch>'` / Why: git's first line / `Fix: Check that .git/config is writable (no stale config.lock), then retry`; and add the `simple`/`inherit` arms to the base-pinning paragraph in docs/specs/cli-surface.md. Optional if trivial: a unit test of the --json exactly-once emitter (jsonDocGuard). -->
- [x] T012 Make `--json` document emission exactly-once: route `fail`, the init-failure emitter, the success print and the SIGINT/SIGTERM handler's 130 document in `src/cmd/wt/create.go` through one `sync.Once`-guarded (or mutex + flag) emitter, so a signal racing a failure/success path can never print two documents. <!-- R6 -->
- [x] T013 Cleanups in `src/cmd/wt/create.go` / `src/internal/worktree/`: add a cmd test pinning the Assumption-6 behavior (`--reuse --name <non-colliding> --base badref nb` → exit 2 `Invalid --base ref`, nothing created); fix the stale comments naming `RunWorktreeSetupWithObserver` as the run site (`create.go` ~450, `init.go` ~46); `git.go` `fmt.Errorf("%s", msg)` → `errors.New(msg)`; drop the redundant `receipt.Init = wt.InitFailed` assignment; update `docs/specs/cli-surface.md` (the SHA-pinning paragraph that says the upstream is lost → upstream preserved per `branch.autoSetupMerge`). Re-run `gofmt -l src` and `cd src && env -u WT_WRAPPER go test ./...`. <!-- R4 R6 -->

## Execution Order

- T001–T003 precede T004–T006 (cmd/ uses the new internal APIs); T004 → T005 → T006 (same file, sequential).
- T007–T008 after T006. T009/T010 independent of code but T010's final `go test ./...` runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `--fetch` exists (long-only, bool) and fetches the remote chosen by `RemoteForRef` before base resolution
- [x] A-002 R2: fetch failure exits 3 with an Error/Why/Fix naming the remote; no worktree dir or branch left behind
- [x] A-003 R3: `--fetch` with `--checkout` / taken `--reuse` runs no extra fetch; without `--fetch` no fetch runs
- [x] A-004 R4: new-branch creates pass a resolved 40-hex SHA as the `git worktree add -b` start-point, resolved before dispatch
- [x] A-005 R5: `From:` shows `<ref> @ <7-char sha>` with `--base`; other `From:` values unchanged
- [x] A-006 R6: under `--json` stdout is exactly one JSON document on every `RunE` exit path, including SIGINT rollback
- [x] A-007 R7: `--json` implies non-interactive + open skip; explicit non-skip `--open` is exit 2 before mutation
- [x] A-008 R8: success document has all seven keys with correct values/nulls per mode
- [x] A-009 R9: `init` reports `ok`/`failed`/`skipped`/`not-found` correctly; `wt init` and existing runner callers unchanged — met; note no other production callers of the wrapped runners remain (see Deletion Candidates)
- [x] A-010 R10: failure documents carry the integer exit code and the `Error:` line; exit 7 carries `result`; `ok` ⇔ exit 0
- [x] A-011 R11: `cli-surface.md` and `docs/site/skill.md` updated; `src/cmd/wt/skill.md` synced; help-dump shows the flags

### Behavioral Correctness

- [x] A-012 R4: invalid `--base` still exits 2 with `Invalid --base ref: <ref>`; `--reuse` with an invalid `--base` still reuses (precedence preserved)
- [x] A-013 R6: without `--json`, stdout is byte-identical to before (the path as last line)

### Scenario Coverage

- [x] A-014 R1: integration test proves a commit pushed to the bare origin after the clone's last fetch becomes `base_sha` with `--fetch`
- [x] A-015 R3: integration test proves the stale commit is used without `--fetch`
- [x] A-016 R8: cmd tests parse the document for bare, positional+base, checkout and reuse modes

### Edge Cases & Error Handling

- [x] A-017 R2: fetch-failure test asserts exit 3, failure document, and absence of both worktree dir and branch
- [x] A-018 R10: init-failure under `--json` exits 7 with `ok:false` and `result.init == "failed"`, worktree kept
- [x] A-019 R1: `RemoteForRef` treats a slash branch with no matching remote as `origin`
- [x] A-026 R4: a new branch cut from a remote-tracking `--base` gets that ref as its upstream (per `branch.autoSetupMerge`), identical to pre-change git behavior; local/SHA/HEAD bases get none (default config); a set-upstream failure rolls back — met (re-review, cycle 3): `origin/main`/`origin/HEAD` → `origin/main` (matches git); stray `refs/remotes/ghost/main` → exit 0, untracked (matches git); `.git/config.lock` → exit 3, one document, no worktree dir, no branch (reproduced by hand + `TestCreate_SetUpstreamFailure_RollsBackCompletely`)
- [x] A-027 R6: document emission is guarded exactly-once — a signal racing fail/success cannot print two documents
- [x] A-028 R4: non-colliding `--reuse` with an invalid `--base` exits 2 before mutation, pinned by a test

### Code Quality

- [x] A-020 Pattern consistency: new code follows surrounding naming/structure (constructor-per-command, `wt.` helpers, Error/Why/Fix copy, test helpers `runWt`/`gitRun`)
- [x] A-021 No unnecessary duplication: existing utilities reused (`BranchForPath`, `DescribeHead`, `PrintError`, `CheckNameCollision`, `createTestRepo`)
- [x] A-022 Readability over cleverness: the `--json` gating is a small number of explicit branches, not scattered conditionals
- [x] A-023 Constitution V: every git operation (fetch, rev-parse, remote list) lives in `internal/worktree`; `create.go` only orchestrates
- [x] A-024 No god functions / magic values: short-SHA length, mode and init values are named constants; no new >50-line helper without reason
- [x] A-025 Test hygiene: new tests leak no host side effects (open stays `skip`; `runWt` env isolation)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `src/internal/worktree/crud.go:210` `RunWorktreeSetup` — zero production callers remain (create's reuse path now calls `RunWorktreeSetupOutcome`; `wt init` never used it); kept only because R9 froze its signature.
- `src/internal/worktree/crud.go:219` `RunWorktreeSetupWithObserver` — zero production callers remain (create's init phase now calls `RunWorktreeSetupOutcome`); referenced only by `RunWorktreeSetup` and `crud_test.go`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | JSON is compact single-line (`json.Marshal`), not indented | "One document" is easiest to assert and consume as one line; `list --json`'s indent is not a contract for create | S:60 R:85 A:75 D:65 |
| 2 | Confident | Fetch skipped when `--reuse` is set and the name collides (checked before fetching) | Mirrors `--base` validation skipped under reuse; avoids a network call for a path that ignores the base | S:65 R:85 A:75 D:65 |
| 3 | Confident | Init-failure `error.message` reuses the banner's status phrasing (`init script exited with status N` / `did not complete`) | Same wording the stderr banner already uses; single source of truth for the copy | S:60 R:90 A:75 D:70 |
| 4 | Certain | `help-dump` has no golden fixtures; new flags flow through the cobra walk automatically | Verified: no testdata/fixtures in help_dump tests; dump is generated from the command tree | S:85 R:90 A:90 D:90 |
| 5 | Confident | Reuse `branch` via `wt.BranchForPath`, mapping `unknown`/`HEAD` to `null` | Reuses the existing best-effort helper instead of a new git call | S:65 R:90 A:80 D:70 |
| 6 | Confident | `--reuse` with a non-colliding name and an invalid `--base` now fails `ExitInvalidArgs` (2) at base resolution instead of a later `git worktree add` failure (3); a colliding `--reuse` still ignores `--base` | The reuse hit is computed up front (`--reuse` requires `--name`, so the final name is known); a new-branch create needs the pinned SHA before dispatch | S:60 R:80 A:75 D:65 |
| 7 | Confident | Unresolvable `HEAD` (no commits) without `--base` is `ExitGitError` (3) `Failed to resolve HEAD`, before any mutation | Previously `git worktree add -b` failed with exit 3 on an unborn HEAD; same code, earlier and clearer | S:60 R:85 A:75 D:70 |
| 8 | Confident | Not-found init outcome constant is `InitNotFoundSkip` (value `"not-found"`) | `InitNotFound` is already the structured-skip error type in `init.go` | S:70 R:90 A:80 D:75 |
| 9 | Confident | Failure `error.message` is the `Error:` line text (the `what` argument), and the init-failure message reuses `InitFailureMessage` shared with the banner | Matches R10 and keeps one source for the init-failure phrasing | S:65 R:90 A:80 D:70 |
| 10 | Confident | With `branch.autoSetupMerge=inherit`, a local `--base` branch that has an upstream passes that upstream on to the new branch; a remote-tracking or upstream-less base gets none | Mirrors git's documented "copy the tracking configuration of the starting point branch"; remote-tracking refs carry no branch config to copy | S:60 R:85 A:70 D:65 |

10 assumptions (1 certain, 9 confident, 0 tentative).
