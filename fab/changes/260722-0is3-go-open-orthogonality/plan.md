# Plan: Make `wt go` and `wt open` Orthogonal

**Change**: 260722-0is3-go-open-orthogonality
**Intake**: `intake.md`

## Requirements

### wt go: `--open` composition flag

#### R1: `wt go` accepts `--open <prompt|default|skip|app>` with create's exact grammar
`goCmd()` (`src/cmd/wt/go.go`) SHALL gain a string flag `--open` reusing `wt create --open`'s
value grammar (`prompt` / `default` / `skip` / `<app>`) and its explicit-value rule: the flag
MUST NOT carry a `NoOptDefVal` (a bare form would swallow the optional positional `[name]`).
No short flag is added (see Assumptions #1). Unset ≡ `skip`.

- **GIVEN** a git repo with worktree `frosty-fox`
- **WHEN** the user runs `wt go frosty-fox --open skip`
- **THEN** the command navigates exactly as bare `wt go frosty-fox` does (no launch)

- **GIVEN** the flag definition
- **WHEN** `wt go --help` is rendered
- **THEN** `--open` appears with the prompt/default/skip/app value description and no `-o` short

#### R2: A non-`skip` `--open` replaces navigation with launch
When `--open` is set to a non-`skip` value, `wt go` SHALL resolve/select the worktree exactly as
today (no-arg → shared `selectWorktree` menu; `<name>` → `resolveWorktreeByName` with the `main`
key and exact-basename precedence) and then **launch** the selection instead of navigating —
`prompt` renders the "Open in:" app menu, `default` resolves the auto-detected default app,
`<app>` launches directly via `openInNamedApp`. It does not additionally cd the parent shell
(matching today's `wt open --select`). `wt go --open prompt` SHALL run the selection menu and the
"Open in:" menu on ONE shared `wt.MenuSession` (single-stdin-reader requirement). `go --open`
thereby gains the launcher exit codes: `ExitGeneralError` (1, unknown app / no default),
`ExitByobuTabError` (5), `ExitTmuxWindowError` (6). Existing `wt go` exit codes are unchanged.
`--open open_here` routes through the unified shell-cd helper (R8), so it is navigation in effect.

- **GIVEN** a git repo with worktrees `alpha` and `bravo` and `WT_CD_FILE` set
- **WHEN** the user runs `wt go bravo --open open_here`
- **THEN** the resolved path of `bravo` is written to `WT_CD_FILE`, printed to stdout as the last
  line, and the stderr confirmation is emitted — and exit code is 0

- **GIVEN** a git repo with a worktree
- **WHEN** the user runs `wt go <name> --open no-such-app`
- **THEN** the command exits `ExitGeneralError` (1) with an "Unknown app" message

- **GIVEN** a git repo with a worktree and piped stdin selecting nothing
- **WHEN** the user runs `wt go <name> --open prompt`
- **THEN** the "Open in:" app menu is rendered (chained on the same session) and no worktree menu
  is shown

#### R3: `wt go --open` × `--non-interactive` mirrors `wt create --open`'s rules
`--non-interactive` SHALL keep its existing `wt go` semantics: no-name + `--non-interactive`
refuses deterministically with `ExitGeneralError` (1) BEFORE any launch logic. Mirroring create
(where `--non-interactive` only changes `--open`'s *default* and an explicitly passed value is
honored as-is), an explicit `--open prompt` under `--non-interactive` with a name is honored —
the app menu runs (degrading through the non-TTY fallback / EOF refusal when stdin is piped).

- **GIVEN** a git repo with worktrees
- **WHEN** the user runs `wt go --non-interactive --open code`
- **THEN** the command exits 1 with "No worktree specified" and renders no menu

### wt open: pure launcher

#### R4: `wt open` no-arg uniformly opens the current context; the main-repo worktree menu is removed
`wt open` with no positional SHALL resolve one target by a single rule: inside a worktree → the
worktree root (unchanged); in a non-worktree git cwd (the main repo) → the **repo root**
(`ctx.RepoRoot`, tab-named `main` for repo/name parity with `wt open main`); outside git → the
cwd (unchanged). The main-repo selection-menu branch and `selectAndOpen` SHALL be removed.
`--app` becomes orthogonal to every selection mode: `wt open --app <app>` from the main repo
opens the repo root in that app; the `ExitInvalidArgs` "--app with the main-repo selection menu"
case is retired. Name/path resolution for a positional is otherwise byte-identical (path-first
stat precedence, worktree-name fallback requiring a git repo, `main` key, soft git gating). The
selection helpers (`selectWorktree`, `resolveWorktreeByName`, `errWorktreeNotFound`) relocate to
`src/cmd/wt/go.go` (they are now primarily `go`'s machinery); `getBranchForPath` is replaced by
the internal `wt.BranchForPath` (R8).

- **GIVEN** the main repo (non-worktree git cwd) with worktrees on disk and `WT_CD_FILE` set
- **WHEN** the user runs `wt open --app open_here`
- **THEN** the repo root is written to `WT_CD_FILE` and the command exits 0 (previously
  `ExitInvalidArgs`)

- **GIVEN** the main repo with worktrees on disk
- **WHEN** the user runs `wt open` (no args, piped stdin)
- **THEN** the "Open in:" app menu for the repo root is rendered and NO worktree-selection menu
  appears

#### R5: `--select` and `--go` become functional hidden deprecated aliases pointing at `wt go --open`
`--select` SHALL be marked deprecated via `cmd.Flags().MarkDeprecated("select", "use \"wt go
--open\" instead")` — still functional (the `openGo` path is retained internally), auto-hidden
from `--help`, stderr warning on use. `--go` (already deprecated toward `--select`) SHALL warn
toward `wt go --open` directly. The `--list` mutual exclusion with `--select`/`--go` keeps its
`ExitInvalidArgs` behavior.

- **GIVEN** a git repo with worktrees `alpha`/`bravo` and `WT_CD_FILE` set
- **WHEN** the user runs `wt open --select bravo --app open_here`
- **THEN** the behavior is unchanged (select-then-launch writes `bravo`'s path to `WT_CD_FILE`)
  AND a stderr deprecation warning naming `wt go --open` is printed

- **GIVEN** any cwd
- **WHEN** `wt open --help` is rendered
- **THEN** neither `--select` nor `--go` appears

#### R6: One-release transitional tip on the changed main-repo no-arg invocation
A no-positional `wt open` from the main repo (the one invocation whose behavior visibly changes)
SHALL print a one-line stderr tip: `tip: to pick a worktree, use wt go (or wt go --open)`.
stdout is unaffected.

- **GIVEN** the main repo
- **WHEN** the user runs `wt open` with no positional
- **THEN** the tip line appears on stderr and never on stdout

#### R7: The `wt open --list [--json]` query surface keeps working unchanged
The `--list`/`--json` registry surface (commit `18eed9d`) SHALL be untouched: same records, same
ordering, same `[]`-not-`null`, same `ExitInvalidArgs` flag-misuse cases (list×target, list×app,
list×select/go, json-without-list), all validated at flag-check time.

- **GIVEN** a non-git cwd
- **WHEN** the user runs `wt open --list --json`
- **THEN** the JSON registry is emitted exactly as before, exit 0

### Unified shell-cd contract (launcher-contract v2)

#### R8: One shell-cd implementation — `wt.NavigateTo` in `internal/worktree`
A single helper `NavigateTo(path, repoName, branch string) error` SHALL live in
`src/internal/worktree/navigate.go` and implement the unified contract:
1. write the resolved absolute path to `WT_CD_FILE` when set (mode `0600`, truncate-on-write) —
   a write failure returns an error before any success output;
2. when `WT_CD_FILE` is unset AND `WT_WRAPPER != "1"`, print the two-line "shell wrapper" hint
   to stderr;
3. emit the two-line stderr confirmation — `→ {repoName} / {Base(path)}  ({branch})` plus the
   two-space-indented absolute path; outside a git context (`repoName == ""`) the first line
   degrades to `→ {Base(path)}`;
4. **always** print the bare resolved path to stdout as the last line.
`OpenInApp`'s `"open_here"` case (`src/internal/worktree/apps.go`) SHALL route through
`NavigateTo` — the `cd -- '<path>'` stdout fallback is retired (and `shellQuoteSingle` deleted if
it loses its last caller). A best-effort `BranchForPath(path string) string` helper (returns
`"unknown"` on error) SHALL live in `internal/worktree` (Constitution V — git op out of `cmd/`)
and replace `cmd/`'s `getBranchForPath`.

- **GIVEN** any directory and `WT_CD_FILE` set
- **WHEN** `wt open <path> --app open_here` runs
- **THEN** the path is written to `WT_CD_FILE` AND printed to stdout as the last line AND the
  stderr confirmation appears AND no `cd -- ` line is printed

- **GIVEN** a non-git target directory and `WT_CD_FILE` unset, `WT_WRAPPER` unset
- **WHEN** `wt open <path> --app open_here` runs
- **THEN** stderr carries the wrapper hint and the `→ {basename}` confirmation, and stdout is
  the bare path (no `cd -- ` line)

#### R9: `wt go` navigation routes through the same helper, behavior-preserved
`cmd/wt/go.go`'s `navigateTo(ctx, path)` SHALL delegate to
`wt.NavigateTo(path, ctx.RepoName, wt.BranchForPath(path))`, mapping a returned error to
`ExitGeneralError` with the existing "Cannot write navigation target" copy. The observable
`wt go` contract (WT_CD_FILE + bare-path stdout + stderr confirmation + hint gating + exit
codes) is unchanged.

- **GIVEN** a git repo with worktree `swift-fox` and `WT_CD_FILE` set
- **WHEN** `wt go swift-fox` runs
- **THEN** stdout is exactly the bare resolved path, `WT_CD_FILE` holds it (mode 0600), and the
  `→ {repo} / swift-fox  ({branch})` confirmation is on stderr — byte-compatible with today

#### R10: `wt create` drops the open_here stdout-suppression special case
`create.go` SHALL remove the `suppressPath` mechanism: the final worktree-path
`fmt.Println(wtPath)` runs whenever create succeeds (the `initFailed` exit-7 guard is unchanged).
When the chosen open app is `open_here`, the unified helper's own path print precedes it —
stdout's contract simplifies to "the worktree path is always the last line".

- **GIVEN** a repo
- **WHEN** `wt create --non-interactive --name X --no-init --open open_here` runs
- **THEN** stdout contains no `cd -- ` line and its last line is the worktree path, exit 0

### Docs and help text

#### R11: `docs/specs/cli-surface.md` matches the new surface
The `## wt go [name]` section SHALL document `--open` (grammar, composition semantics, launcher
exit codes 5/6); the `## wt open [name|path]` section SHALL document the current-context no-arg
rule, the `--select`/`--go` deprecations, the retired `ExitInvalidArgs` case, and the updated
exit-code list; the `wt create` section's stdout sentence drops the suppression parenthetical.
The `## Selection × action model` section keeps the matrix and its "Target model / in flight"
annotation is REMOVED (the per-command sections now match it).

- **GIVEN** the revised spec
- **WHEN** the `Selection × action model` section is read
- **THEN** no "in flight"/"Remove this note" annotation remains and the per-command sections
  agree with the matrix

#### R12: `docs/specs/launcher-contract.md` is revised to v2
§2's invocation table SHALL reflect the new no-arg behavior and `--app` orthogonality; §3 SHALL
describe the single unified mechanism (WT_CD_FILE write AND always-bare-path stdout AND stderr
confirmation; `cd --` fallback retired) shared by `open_here` and `wt go`; §5 SHALL drop the
`--app`+menu `ExitInvalidArgs` case (the `--list`/`--json` misuse cases remain); §6 SHALL
re-affirm the stability list for the new semantics with a changelog note recording the
authorized amendment (user decision, 2026-07-22 discussion).

- **GIVEN** the revised contract
- **WHEN** §5 is read
- **THEN** `ExitInvalidArgs` no longer lists the main-repo-menu case and §6 carries the
  amendment note

#### R13: Help text and toolkit docs reflect the two-menu ownership model
Cobra `Long` help for `wt go` and `wt open` SHALL be rewritten around the ownership model ("go
owns which-worktree, open owns which-app; compose with `wt go --open`"). README.md,
`docs/site/workflows.md`, and `docs/site/skill.md` (synced to `src/cmd/wt/skill.md` via
`scripts/sync-skill.sh`, staying within the ≤150-line budget) SHALL be updated where they
describe the old main-repo menu / `--select` composition, per the constitution's Toolkit
Standards article (`readme-extraction`, `skill`, principles №3/№10 — checked against
`shll standards`).

- **GIVEN** the updated docs
- **WHEN** README's / workflows' `wt open` matrix row "In the main repo" is read
- **THEN** it says the repo root is opened and points worktree-picking at `wt go`

### Non-Goals

- Removing `--select`/`--go` or the internal `openGo` path — deferred to a later change.
- Any hop repo change — verified unnecessary at intake (hop passes explicit paths, inherits
  stdio, reads `WT_CD_FILE` via its shell shim). run-kit unaffected.
- Removing `resolveWorktreeByName`'s unused `ctx` parameter — separately tracked in
  `fab/backlog.md`.
- New exit codes — `go --open` reuses the existing launcher codes only (Constitution III).

### Design Decisions

#### Unified helper lives in `internal/worktree`, cmd delegates
**Decision**: `NavigateTo` + `BranchForPath` live in `internal/worktree` (new `navigate.go`;
`context.go`); `cmd/wt/go.go`'s `navigateTo` and `OpenInApp`'s `open_here` case both delegate.
**Why**: `OpenInApp` (internal) must call the shared implementation, and Constitution V wants
git ops (`BranchForPath`) out of `cmd/`; a single emitter makes output-contract drift
impossible.
**Rejected**: keeping `navigateTo` in `cmd/` and duplicating the contract in `apps.go` (the
exact two-implementation drift this change exists to kill); passing branch from callers into
`OpenInApp` (widens a public signature for one case).
*Introduced by*: 260722-0is3-go-open-orthogonality

#### create+open_here prints the path twice; last-line contract is the guarantee
**Decision**: with suppression dropped, `wt create --open open_here` stdout carries the unified
helper's path line followed by create's own final path line — both identical; the documented
machine contract is "path is always the last line".
**Why**: the intake explicitly drops the suppression special case; each emitter honors its own
contract, and re-suppressing either side would reintroduce the coupling.
**Rejected**: create skipping its final print when open_here launched (that IS the suppression
special case, inverted); `NavigateTo` growing a no-stdout mode (forks the unified contract).
*Introduced by*: 260722-0is3-go-open-orthogonality

### Deprecated Requirements

#### `wt open` main-repo no-arg worktree-selection menu
**Reason**: `open` becomes a pure launcher; each menu lives in exactly one verb.
**Migration**: `wt go` (navigate) or `wt go --open` (select-then-launch).

#### `ExitInvalidArgs` for `--app` with the main-repo selection menu
**Reason**: with no menu on that path, `--app` is orthogonal to every selection mode.
**Migration**: `wt open --app <app>` from the main repo opens the repo root.

#### `cd -- '<path>'` stdout fallback in `open_here`
**Reason**: shell-cd unifies on `navigateTo`'s contract — bare path always on stdout.
**Migration**: consumers eval'ing stdout switch to `cd "$(command wt …)"`.

#### `wt create` open_here stdout suppression
**Reason**: stdout is uniformly "resolved path as last line" under the unified contract.
**Migration**: N/A (contract simplifies; last line is still the path).

## Tasks

### Phase 1: Unified shell-cd core (internal)

- [x] T001 Add best-effort `BranchForPath(path string) string` to `src/internal/worktree/context.go` (single `git rev-parse --abbrev-ref HEAD` in dir, `"unknown"` fallback) with unit tests in `context_test.go` <!-- R8 -->
- [x] T002 Create `src/internal/worktree/navigate.go` with `NavigateTo(path, repoName, branch string) error` implementing the unified contract (WT_CD_FILE-first write, WT_WRAPPER-gated hint, stderr confirmation with non-git degrade, always-bare-path stdout) plus `navigate_test.go` unit tests covering all four steps and the degrade form <!-- R8 -->
- [x] T003 Route `OpenInApp`'s `"open_here"` case in `src/internal/worktree/apps.go` through `NavigateTo` (branch via `BranchForPath`), retire the `cd -- '<path>'` fallback, delete `shellQuoteSingle` if orphaned; update `apps_test.go` <!-- R8 -->

### Phase 2: Command wiring (cmd/wt)

- [x] T004 In `src/cmd/wt/go.go`, delegate `navigateTo(ctx, path)` to `wt.NavigateTo` (error → `ExitGeneralError`, existing copy); relocate `selectWorktree`, `resolveWorktreeByName`, `errWorktreeNotFound` from `open.go` to `go.go`; replace `getBranchForPath` call sites with `wt.BranchForPath` and delete the cmd copy <!-- R9 R4 -->
- [x] T005 Add `--open` to `goCmd()` (`src/cmd/wt/go.go`): string flag, no `NoOptDefVal`, no short; non-`skip` values launch the selection via `openInNamedApp`/`handleAppMenuWithSession` on one shared `MenuSession` (prompt wording "Select worktree to open:" for launch modes); keep the no-name `--non-interactive` refusal ahead of launch logic; rewrite the `Long` help around two-menu ownership <!-- R1 R2 R3 R13 -->
- [x] T006 Purify `src/cmd/wt/open.go`: main-repo no-arg branch opens `ctx.RepoRoot` (wtName `main`) instead of `selectAndOpen`; delete `selectAndOpen` and the `--app`+menu `ExitInvalidArgs` case; emit the stderr tip on the main-repo no-positional path; `MarkDeprecated("select", …)` and re-point `--go`'s deprecation at `wt go --open`; update `openGo`'s error copy; rewrite the `Long` help; keep `--list`/`--json` validation untouched <!-- R4 R5 R6 R7 R13 -->
- [x] T007 In `src/cmd/wt/create.go`, remove the `suppressPath` variable and its three `open_here` assignments so the final path print always runs (initFailed guard unchanged) <!-- R10 -->

### Phase 3: Tests

- [x] T008 `src/cmd/wt/go_test.go`: add coverage for `--open skip` ≡ bare navigate, `--open open_here <name>` unified navigation (WT_CD_FILE + stdout last line + stderr `→`), `--open <unknown-app>` → exit 1, `<name> --open prompt` renders the chained "Open in:" menu with no worktree menu, no-name `--non-interactive --open code` still refuses (exit 1), and a menu-ordering test through `wt go --open prompt` (adopting the retired `TestOpen_MenuOrdersNewestFirst` coverage) <!-- R1 R2 R3 -->
- [x] T009 `src/cmd/wt/open_test.go`: replace `TestOpen_ErrorFromMainRepoWithoutTarget` and `TestOpen_MenuOrdersNewestFirst` with main-repo orthogonality tests (`wt open --app open_here` from main repo → repo root in WT_CD_FILE; no-arg piped → "Open in:" menu, no worktree menu, stderr tip); update `TestOpen_HelpHidesGoShowsSelect` → help hides both `--select` and `--go`; add `--select` deprecation-warning test; keep all `--list`/`--json` tests green unchanged <!-- R4 R5 R6 R7 -->
- [x] T010 `src/cmd/wt/create_test.go`: rewrite `TestCreate_OpenHereSuppressesPath` as the new contract (no `cd -- ` on stdout; last line = worktree path; WT_CD_FILE written when set) <!-- R10 -->
- [x] T011 `src/cmd/wt/integration_test.go`: extend `TestIntegration_LauncherContract_NonGitTempDir` for the unified stdout/stderr shape; flip `TestIntegration_OpenSelect_NameArg_ResolvesAndLaunches` to expect the deprecation warning; verify `TestIntegration_OpenGo_Deprecated` still holds; add `TestIntegration_GoOpen_NameArg_ResolvesAndLaunches` (`wt go bravo --open open_here` from a sibling worktree); update `TestIntegration_NonTTYMenuActionableRefusal`'s `wt open` entry point for the app-menu path <!-- R2 R4 R5 R8 -->
- [x] T012 Run the full suite (`go test ./...` from `src/`) and fix regressions <!-- R2 R4 R8 R10 -->

### Phase 4: Docs

- [x] T013 [P] Rewrite `docs/specs/cli-surface.md`: `wt go` section (+`--open`, launcher exit codes), `wt open` section (current-context rule, deprecations, retired case, exit codes), create stdout sentence, and remove the "Target model / in flight" annotation from `## Selection × action model` <!-- R11 -->
- [x] T014 [P] Revise `docs/specs/launcher-contract.md` to v2 (§2 invocation table, §3 unified mechanism, §5 exit-code table, §6 amendment changelog note) <!-- R12 -->
- [x] T015 Update `README.md`, `docs/site/workflows.md`, and `docs/site/skill.md` for the new model (launcher matrix rows, `--select` mention, `wt go --open` composition); run `scripts/sync-skill.sh` and keep the skill bundle ≤150 lines; check the touched surfaces against `shll standards` (principles, readme-extraction, skill, help-dump) <!-- R13 -->

## Execution Order

- T001–T003 (internal core) block T004–T007 (cmd wiring); T004 blocks T005/T006 (helper relocation first).
- T008–T011 follow their implementation tasks; T012 runs after all code tasks.
- T013/T014 are parallelizable; T015 after code lands (help text must be final for skill sync).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt go --open` exists with the prompt/default/skip/app grammar, requires an explicit value, and `--open skip` behaves as bare `wt go`
- [x] A-002 R2: A non-`skip` `--open` launches the selection (menu chaining on one session for `prompt`; direct app for named values) and carries the launcher exit codes 1/5/6
- [x] A-003 R4: `wt open` no-arg opens worktree root / repo root / cwd by context; `selectAndOpen` and the main-repo menu are gone
- [x] A-004 R8: `wt.NavigateTo` is the single shell-cd implementation and `OpenInApp("open_here")` routes through it
- [x] A-005 R11: `cli-surface.md`'s go/open sections match the implemented surface and the "in flight" annotation is removed
- [x] A-006 R12: `launcher-contract.md` v2 documents the unified §3 mechanism and the §6 amendment note

### Behavioral Correctness

- [x] A-007 R4: `wt open --app <app>` from the main repo opens the repo root (exit 0) instead of exiting `ExitInvalidArgs`
- [x] A-008 R5: `--select`/`--go` still work end-to-end and warn on stderr toward `wt go --open`; both are hidden from `--help`
- [x] A-009 R9: `wt go <name>` output is byte-compatible with the pre-change contract (bare-path stdout, WT_CD_FILE 0600, stderr confirmation)
- [x] A-010 R10: `wt create --open open_here` prints no `cd -- ` line and its stdout's last line is the worktree path
- [x] A-011 R3: `wt go --non-interactive` with no name refuses (exit 1) regardless of `--open`

### Removal Verification

- [x] A-012 R4: No code path reaches a worktree-selection menu from `wt open` without `--select`/`--go`; the `--app`+menu `ExitInvalidArgs` message is gone
- [x] A-013 R8: No `cd -- '` emission remains in the codebase (`shellQuoteSingle` is retained — still used by `PrintInitFailureBanner` in `errors.go`, so it is NOT orphaned; the "delete if orphaned" condition correctly resolves to keep)

### Scenario Coverage

- [x] A-014 R2: Tests cover `go --open open_here` (unified navigation), `go --open <unknown>` (exit 1), and `go <name> --open prompt` (chained app menu)
- [x] A-015 R6: A test pins the stderr tip on main-repo no-arg `wt open`
- [x] A-016 R7: The `--list`/`--json` tests (registry shape, ordering, flag exclusivity `ExitInvalidArgs` cases) pass unchanged
- [x] A-017 R8: An integration test asserts the unified stdout/stderr shape for `wt open <path> -a open_here` from a non-git cwd

### Edge Cases & Error Handling

- [x] A-018 R8: `WT_CD_FILE` write failure surfaces as an error before any success output (no confirmation, no stdout path)
- [x] A-019 R8: Outside a git context the confirmation degrades to `→ {basename}` and branch lookup failure renders without crashing
- [ ] A-020 R2: Cancelling either menu in `wt go --open prompt` prints `Cancelled.` and exits 0 without launching — **behavior verified by code inspection** (`go.go` selection-menu cancel prints `Cancelled.`+exit 0 before launch; `handleAppMenuWithSession` returns nil without launching on app-menu cancel), but **no dedicated test pins it** — no `Cancelled` assertion exists in any go/open test. Should-fix test-coverage gap.

### Code Quality

- [x] A-021 Pattern consistency: New code follows naming and structural patterns of surrounding code (helpers relocated per ownership, `cmd/` stays orchestration-only per Constitution V)
- [x] A-022 No unnecessary duplication: one shell-cd implementation, `openInNamedApp`/`handleAppMenuWithSession` reused by `go --open`
- [x] A-023 No god functions: `goCmd` RunE and the open-phase logic stay decomposed (<50 lines per unit where reasonable)
- [x] A-024 No magic strings: `--open` values compared against the same literals create uses; exit codes via named constants only
- [x] A-025 Test side-effect discipline: no unit/integration test launches real tmux/byobu windows (open_here / failing-resolution targets / runWt env isolation only)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `resolveWorktreeByName`'s `ctx *wt.RepoContext` parameter (`src/cmd/wt/go.go:185`) — unused at all three call sites; the selection/resolution seam's last vestigial `ctx`. Already tracked in `fab/backlog.md` and declared a Non-Goal here; surfaced again as a follow-up cleanup once the deprecated paths are removed.
- Stale doc-comment reference to the deleted `selectAndOpen` in `handleAppMenuWithSession` (`src/cmd/wt/open.go:396`) — not code, but a documentation-drift cleanup: the comment names a function this change removed; its current callers are `openGo` and `wt go --open` (via `launchSelection`).
- `openGo` / `--select` / `--go` deprecated composition path (`src/cmd/wt/open.go`) — made redundant by `wt go --open` but **deliberately retained** as a functional deprecated alias per the explicit Non-Goal ("deferred to a later change"); listed for visibility, not for action now.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | `wt go --open` gets NO short flag (`-o`) | flag-naming-conventions memory: shorts only for common interactive use; the composition flow is the same one `--select` covered short-less; easily added later | S:55 R:90 A:80 D:60 |
| 2 | Confident | Selection-menu prompt reads "Select worktree to open:" when `--open` launches, "Select worktree to go to:" when navigating | preserves both existing wordings by action; `selectWorktree` already parameterizes the prompt per verb | S:45 R:95 A:80 D:65 |
| 3 | Confident | `wt create --open open_here` stdout carries the path twice (helper print + create's final print); documented contract is "path is always the last line" | intake explicitly drops the suppression special case; re-suppressing either emitter would reintroduce the coupling | S:60 R:85 A:75 D:55 |
| 4 | Certain | Unified helper (`NavigateTo`) + `BranchForPath` live in `internal/worktree`; `cmd/` delegates | `OpenInApp` (internal) must call it; Constitution V forbids git ops in `cmd/`; single emitter kills the drift | S:70 R:85 A:90 D:80 |
| 5 | Confident | Main-repo no-arg `wt open` uses wtName `main` (tab name `{repo}-main`) | parity with `wt open main`, which passes the target `main` as wtName; matches `selectWorktree`'s stable main-row name | S:50 R:90 A:85 D:70 |
| 6 | Confident | The transitional tip fires on the whole main-repo no-positional branch (with or without `--app`) | both invocations formerly hit the menu/`ExitInvalidArgs` there; a stderr tip is harmless and the branch is one seam | S:45 R:95 A:80 D:60 |
| 7 | Confident | The WT_WRAPPER-gated hint copy is unified to one neutral wording shared by open_here and `wt go` | launcher-contract §6 explicitly allows cosmetic wording changes; two verb-specific copies cannot survive a single implementation | S:50 R:90 A:85 D:70 |
| 8 | Confident | `openGo` internals and error copy stay, with messages re-pointed at `wt go --open` | intake keeps the deprecated path functional "until a later removal change"; copy updates are cosmetic | S:60 R:90 A:85 D:75 |
| 9 | Confident | README.md / docs/site/workflows.md / docs/site/skill.md updates are in scope | constitution Toolkit Standards binds readme-extraction/skill standards; leaving the old menu documented is exactly the drift principle №3/№10 prohibit | S:55 R:90 A:85 D:70 |
| 10 | Confident | Worktree-menu-ordering test coverage moves to the `wt go --open prompt` flow | the selection menu is only reachable via `go` now; the shared `selectWorktree` helper is what the ordering test pins | S:55 R:90 A:85 D:75 |

10 assumptions (1 certain, 9 confident, 0 tentative).
