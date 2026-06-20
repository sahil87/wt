# Plan: Worktree navigation via `wt go`, launcher via `wt open`

**Change**: 260620-3pp5-open-worktree-from-worktree
**Intake**: `intake.md`

## Requirements

### wt-cli: `wt go` worktree selection (Act 2 — navigation, no launch)

#### R1: `wt go` is a registered, contract-conformant subcommand
A new `wt go [name]` subcommand SHALL exist as a `cobra.Command` defined in
`src/cmd/wt/go.go` and registered on the root command in `src/cmd/wt/main.go`'s
`root.AddCommand(...)`. It SHALL set `SilenceUsage: true` and `SilenceErrors: true`
and return domain errors via `RunE` / `wt.ExitWithError`, per Constitution II.

- **GIVEN** the `wt` binary is built
- **WHEN** the user runs `wt go --help`
- **THEN** a `go` subcommand is listed with long-form help describing current-repo
  worktree selection and the `WT_CD_FILE` / stdout navigation contract
- **AND** the command never prints cobra usage on a domain error (errors render via the root handler)

#### R2: `wt go <name>` resolves a worktree by name and navigates to it
`wt go <name>` SHALL resolve `<name>` as a worktree of the current repo
(case-insensitive, via the existing `resolveWorktreeByName` logic) and navigate
there — it SHALL NOT launch any application.

- **GIVEN** the cwd is inside a git repo with a worktree named `alpha`
- **WHEN** the user runs `wt go alpha`
- **THEN** the resolved absolute path of `alpha` is written to `WT_CD_FILE` (when set)
- **AND** the same absolute path is printed to stdout as the last line
- **AND** no application is launched

#### R3: `wt go` (no arg) shows the current-repo selection menu from anywhere in the repo
`wt go` with no positional arg SHALL show the worktree-selection menu for the
current repo — newest-first recency ordering, branch shown per entry — reachable
from the main repo **or** from inside any worktree. On selection it SHALL navigate
to the chosen worktree (write `WT_CD_FILE` + print stdout path). The menu SHALL be
rendered by the shared selection helper (single source of truth — R8).

- **GIVEN** the cwd is inside worktree `alpha` of a repo that also has `bravo` and `charlie`
- **WHEN** the user runs `wt go` (interactive)
- **THEN** a "Select worktree to go to:" menu lists the repo's worktrees (including the current one — behavior-preserving with the shared `wt open` menu per R8) newest-first
- **AND** selecting an entry writes its path to `WT_CD_FILE` and prints it to stdout
- **AND** cancelling (choice 0) prints `Cancelled.` and exits 0 without navigating

#### R4: `wt go` requires a git repository
`wt go` (with or without a name) SHALL require a git repository. From a non-git
cwd it SHALL exit `ExitGitError` (3) with a what/why/fix message — worktree
resolution walks the repo's worktree list, which is unreachable outside a repo.

- **GIVEN** the cwd is not inside a git repository
- **WHEN** the user runs `wt go` or `wt go some-name`
- **THEN** the command exits `ExitGitError` (3)
- **AND** stderr carries a structured what/why/fix message

#### R5: `wt go <unknown-name>` exits `ExitGeneralError`
When `<name>` does not match any worktree (the worktree list succeeded but no
entry matched), `wt go <name>` SHALL exit `ExitGeneralError` (1) with the message
structure "Worktree '<name>' not found" + "Use 'wt list' to see available worktrees".
A genuine git-list failure (distinct sentinel) SHALL exit `ExitGitError` (3).

- **GIVEN** the cwd is inside a git repo with no worktree named `ghost`
- **WHEN** the user runs `wt go ghost`
- **THEN** the command exits `ExitGeneralError` (1)
- **AND** stderr contains `not found`

#### R6: `wt go` navigates via the existing `WT_CD_FILE` shell-cd contract + stdout
`wt go` SHALL reuse the existing shell-cd plumbing with no new env var: write the
resolved absolute path to `WT_CD_FILE` (mode `0600`, truncate-on-write) when set,
AND print the resolved absolute path to stdout as the last line. When `WT_CD_FILE`
is unset and `WT_WRAPPER` is not `1`, it SHALL emit the same shell-wrapper hint
convention `wt open`'s "Open here" uses. It SHALL NEVER `cd` the parent shell
directly (Constitution VII).

- **GIVEN** `WT_CD_FILE` points at a writable file and `WT_WRAPPER=1`
- **WHEN** `wt go alpha` resolves successfully
- **THEN** `WT_CD_FILE` contains exactly the resolved absolute path, mode `0600`
- **AND** stdout's last line is that same path
- **AND** no shell-wrapper hint is printed to stderr

#### R7: `wt go` no-arg under `--non-interactive` / non-TTY is deterministic and non-prompting
`wt go` SHALL accept a `--non-interactive` flag (Constitution VI). With no arg and
`--non-interactive`, it SHALL NOT prompt; it SHALL exit `ExitGeneralError` (1) with
a what/why/fix message stating that a no-arg selection has no non-interactive
default (pass a name). `wt go <name> --non-interactive` resolves directly (nothing
to suppress). When stdout/stdin is not a TTY, the no-arg menu degrades through the
existing `MenuSession` fallback path.

- **GIVEN** the cwd is inside a git repo
- **WHEN** the user runs `wt go --non-interactive` (no name)
- **THEN** the command exits `ExitGeneralError` (1) without prompting
- **AND** stderr advises passing a worktree name

### wt-cli: `wt open --go` composition (select-then-launch)

#### R8: shared worktree-selection helper (single source of truth)
The worktree-selection logic currently inside `selectAndOpen` (`src/cmd/wt/open.go`)
SHALL be extracted into a reusable helper in `src/cmd/wt/` that both `wt go` and
`wt open --go` call. The helper SHALL own the menu UX: filter out the main repo,
newest-first `SortByRecency` ordering, per-entry branch display, and rendering via
a caller-supplied `MenuSession` (so `open --go` can chain the "Open in:" menu on the
same stdin reader). `selectAndOpen`'s existing behavior SHALL be preserved by
re-expressing it on top of the helper. No new business rules move into
`internal/worktree/` — the helper composes existing exported helpers (Constitution V).

- **GIVEN** the refactor is complete
- **WHEN** `wt open` (no-arg, main repo), `wt go` (no-arg), and `wt open --go` (no-arg) each show a selection menu
- **THEN** all three render the identical menu (same recency ordering, same branch display, same default highlight)
- **AND** the menu construction lives in exactly one helper function

#### R9: `wt open --go` composes selection then launch
A boolean `--go` flag SHALL be added to `wt open`. When set, `wt open` SHALL first
perform `wt go`'s selection (menu when no positional arg; resolve-by-name when
`<name>` given) to obtain a worktree path, then run its own launcher (app menu, or
`--app` directly) against that path — an internal composition via shared functions,
no subprocess. `--go` + `--app` SHALL compose (select, then open directly in the
named app). `wt open`'s existing surface (no-arg in worktree opens current; no-arg
in main repo shows menu+launch; `wt open <name>` resolves-AND-launches) SHALL be
preserved unchanged.

- **GIVEN** the cwd is inside a git repo with worktrees `alpha` and `bravo`
- **WHEN** the user runs `wt open --go bravo --app open_here`
- **THEN** `bravo` is resolved and opened in `open_here` (path written to `WT_CD_FILE`)
- **AND** `wt open --go` (no arg) shows the selection menu, then the app menu on the same session
- **AND** plain `wt open <name>` continues to resolve AND launch (launcher-contract surface unchanged)

#### R10: `wt open --go` requires a git repository
Because `--go` performs worktree selection, `wt open --go` from a non-git cwd SHALL
exit `ExitGitError` (3) — the same precondition as `wt go`.

- **GIVEN** the cwd is not inside a git repository
- **WHEN** the user runs `wt open --go` or `wt open --go some-name`
- **THEN** the command exits `ExitGitError` (3)

### docs: spec + help conformance

#### R11: docs reflect the new surface
`docs/specs/cli-surface.md` SHALL gain a `## wt go [name]` section and document
the `wt open --go` flag and the open=launcher / go=selector framing.
`docs/specs/launcher-contract.md` SHALL note that `wt go` reuses `WT_CD_FILE` (§3)
for navigation with no new env var — confirming the env-var contract is unchanged
(no stability-guarantee amendment needed). `--help` long text for `wt go` and the
`--go` flag SHALL be accurate.

- **GIVEN** the change is implemented
- **WHEN** a reader consults `docs/specs/cli-surface.md` and `docs/specs/launcher-contract.md`
- **THEN** `wt go` and `wt open --go` are documented with exit codes and the navigation contract
- **AND** the launcher-contract notes `wt go` reuses `WT_CD_FILE` without adding any env var

### Non-Goals

- Cross-repo navigation — `wt go` is scoped to the current repo's worktrees only; cross-repo is `hop`'s job.
- Changing `wt open`'s no-arg behavior (in-worktree "open current", main-repo menu+launch) — preserved as-is.
- Narrowing `wt open <name>` to selection-only — it keeps resolve-AND-launch (the surface `hop` depends on).
- Delegating the main-repo no-arg `wt open` menu to `wt go` internally — deferred (intake Open Questions).
- `docs/memory/` files — hydration is a later stage.

### Design Decisions

1. **`wt go` writes `WT_CD_FILE` AND prints stdout directly, rather than routing through `OpenInApp("open_here", …)`**: `wt go` is a navigation verb with no app concept — *Why*: `OpenInApp`'s "open_here" path writes `WT_CD_FILE` OR prints `cd -- '<path>'` (mutually exclusive) and lives in the launcher subsystem; `wt go` must do BOTH (write file when set AND always print the bare path for `cd "$(command wt go)"`). A small dedicated navigation helper in `cmd/wt` keeps `go` decoupled from the app catalog — *Rejected*: reusing `OpenInApp("open_here")` (wrong output contract: it prints a `cd --` line, not a bare path, and only when `WT_CD_FILE` is unset).
2. **Shared selection helper returns `(path, name, cancelled, err)` and takes a `*MenuSession`**: *Why*: lets `open --go` chain the "Open in:" menu on the same stdin reader (the documented byte-theft fix in `MenuSession`), and lets `wt go` own its own one-shot session — *Rejected*: returning only a path (loses the cancel signal and the name needed for tab naming).
3. **`--go` no-arg under non-interactive inherits `wt go`'s deterministic error** rather than auto-picking newest: *Why*: a no-arg "pick a worktree" has no obviously-correct silent default; erroring is safer (matches intake Assumption 8).

## Tasks

### Phase 1: Refactor — shared selection helper

- [x] T001 Extract the worktree-selection logic from `selectAndOpen` in `src/cmd/wt/open.go` into a new reusable helper `selectWorktree(ctx *wt.RepoContext, session *wt.MenuSession) (path, name string, cancelled bool, err error)` — filter out main repo, `SortByRecency` newest-first, build "name (branch)" menu rows, render via the passed session with prompt "Select worktree to go to:", default highlight index 1. Re-express `selectAndOpen` to call it (preserving its existing launch behavior on the same session). <!-- R8 -->

### Phase 2: Core Implementation — `wt go`

- [x] T002 Add `src/cmd/wt/go.go`: `goCmd() *cobra.Command` with `Use: "go [name]"`, `SilenceUsage`/`SilenceErrors` true, `Args: cobra.MaximumNArgs(1)`, a `--non-interactive` bool flag, and long help describing selection + `WT_CD_FILE`/stdout navigation. Implement `RunE`: require git repo (else `ExitGitError`); with a name → `resolveWorktreeByName` (not-found → `ExitGeneralError`, list-fail → `ExitGitError`) then navigate; no arg + `--non-interactive` → `ExitGeneralError` deterministic message; no arg interactive → shared `selectWorktree` via a fresh `MenuSession`, cancel prints `Cancelled.` exit 0, else navigate. <!-- R1 R2 R3 R4 R5 R7 -->
- [x] T003 Add a navigation helper `navigateTo(path string)` in `src/cmd/wt/go.go`: write `path` to `WT_CD_FILE` (mode `0600`) when set; print the shell-wrapper hint to stderr when `WT_CD_FILE` is unset and `WT_WRAPPER != "1"` (mirroring `OpenInApp`'s "open_here" hint wording); always print the resolved absolute path to stdout as the last line. <!-- R6 -->
- [x] T004 Register `goCmd()` in `src/cmd/wt/main.go`'s `root.AddCommand(...)`. <!-- R1 -->

### Phase 3: Core Implementation — `wt open --go`

- [x] T005 Add a `--go` bool flag to `openCmd()` in `src/cmd/wt/open.go`. When set: require a git repo (else `ExitGitError`); obtain a worktree path via selection (resolve `<name>` when a positional arg is given; otherwise `selectWorktree` on a shared session — cancel exits 0), then launch it via the existing launcher path (`--app` direct, or `handleAppMenuWithSession` on the same session). Compose with `--app`. Leave the non-`--go` code paths untouched. <!-- R9 R10 -->

### Phase 4: Tests

- [x] T006 [P] Add `src/cmd/wt/go_test.go`: unit-level binary tests — `wt go <name>` happy path writes `WT_CD_FILE` + stdout path; `wt go ghost` (unknown) exits 1 with "not found"; `wt go` from non-git cwd exits 3; `wt go --non-interactive` (no arg) exits 1 without prompting. <!-- R2 R4 R5 R7 -->
- [x] T007 [P] Add integration coverage in `src/cmd/wt/integration_test.go`: end-to-end `wt go <name>` from inside a sibling worktree writes the correct sibling path to `WT_CD_FILE` and stdout; no-arg `wt go` menu lists siblings newest-first (mirror `TestOpen_MenuOrdersNewestFirst`); `wt open --go <name> --app open_here` resolves and writes `WT_CD_FILE`. <!-- R3 R6 R9 -->

### Phase 5: Docs

- [x] T008 [P] Update `docs/specs/cli-surface.md`: add `## wt go [name]` section (behavior matrix, exit codes, `--non-interactive`, `WT_CD_FILE`/stdout navigation); update `## wt open` to document `--go` and the launcher/selector framing. <!-- R11 -->
- [x] T009 [P] Update `docs/specs/launcher-contract.md`: note `wt go` reuses `WT_CD_FILE` (§3) for navigation, no new env var, no stability amendment. <!-- R11 -->

## Execution Order

- T001 blocks T002, T005 (both call `selectWorktree`)
- T002, T003, T004 are the `wt go` command (T003's `navigateTo` is used by T002)
- T006, T007, T008, T009 follow implementation; T006–T009 are mutually `[P]`

## Acceptance

### Functional Completeness

- [x] A-001 R1: `wt go` is registered in `main.go`, defined in `go.go`, sets `SilenceUsage`/`SilenceErrors`, returns errors via `RunE`/`ExitWithError`, and `wt go --help` shows accurate long text.
- [x] A-002 R2: `wt go <name>` resolves a worktree case-insensitively and navigates (no app launched).
- [x] A-003 R3: `wt go` (no arg) shows the selection menu from inside a worktree as well as the main repo, newest-first with branch display.
- [x] A-004 R4: `wt go` and `wt go <name>` from a non-git cwd exit `ExitGitError` (3) with a what/why/fix message.
- [x] A-005 R5: `wt go <unknown>` exits `ExitGeneralError` (1) with "not found"; a git-list failure exits `ExitGitError` (3).
- [x] A-006 R6: `wt go` writes the resolved abs path to `WT_CD_FILE` (mode 0600) when set AND prints it to stdout as the last line; never cd's the parent shell directly.
- [x] A-007 R7: `wt go --non-interactive` (no arg) exits `ExitGeneralError` (1) without prompting; `wt go <name> --non-interactive` resolves directly.
- [x] A-008 R8: a single shared `selectWorktree` helper renders the menu for `wt open` (main-repo no-arg), `wt go`, and `wt open --go` — identical ordering/branch display/default.
- [x] A-009 R9: `wt open --go [name]` composes selection + launch internally; `--go`+`--app` compose; `wt open <name>` still resolves AND launches (launcher contract unchanged).
- [x] A-010 R10: `wt open --go` from a non-git cwd exits `ExitGitError` (3).
- [x] A-011 R11: `docs/specs/cli-surface.md` documents `wt go [name]` and `wt open --go`; `docs/specs/launcher-contract.md` notes `wt go` reuses `WT_CD_FILE` with no new env var.

### Behavioral Correctness

- [x] A-012 R8: `selectAndOpen`'s pre-change behavior (main-repo no-arg menu → launch) is preserved after the refactor — `TestOpen_MenuOrdersNewestFirst` and the other `open` tests still pass.
- [x] A-013 R9: `hop`'s launcher-contract surface (`wt open <name>` / `<path>` / `--app` / exit codes / `WT_CD_FILE`) is unchanged by the `--go` addition.

### Scenario Coverage

- [x] A-014 R2 R6: an integration test exercises `wt go <name>` from inside a sibling worktree end-to-end (path → `WT_CD_FILE` + stdout).
- [x] A-015 R3: an integration test asserts the no-arg `wt go` menu lists siblings newest-first.
- [x] A-016 R9: an integration test exercises `wt open --go <name> --app open_here`.

### Edge Cases & Error Handling

- [x] A-017 R5 R7: unit tests cover unknown-name (exit 1), non-git (exit 3), and no-arg non-interactive (exit 1) paths for `wt go`.

### Code Quality

- [x] A-018 Pattern consistency: new code follows the `wt.ExitWithError` what/why/fix style, cobra `RunE` conventions, and test conventions (`runWt`, `createTestRepo`, `createWorktreeViaWt`) of surrounding code.
- [x] A-019 No unnecessary duplication: `wt go` and `wt open --go` reuse `selectWorktree`, `resolveWorktreeByName`, `SortByRecency`, and `MenuSession` rather than reimplementing selection/recency/menu logic.
- [x] A-020 gofmt/vet clean: `gofmt -l` (from `src/`) emits no output and `go vet ./...` passes.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Menu prompt for `wt go` is "Select worktree to go to:" (vs. open's "Select worktree to open:") | Intake calls the menu "the same … menu `selectAndOpen` builds"; `go` is navigation not launch, so a navigation-flavored prompt reads correctly. Cosmetic, trivially reversible | S:60 R:90 A:70 D:65 |
| 2 | Confident | `wt go` no-arg non-interactive exits `ExitGeneralError` (1), not a silent no-op | Intake Assumption 8 (Tentative there) leaned this way; erroring surfaces the misuse deterministically and is scriptable. Reversible if a "newest default" is later wanted | S:55 R:80 A:70 D:55 |
| 3 | Certain | `wt go` navigation writes `WT_CD_FILE` + prints stdout via a dedicated `cmd/wt` helper, not `OpenInApp` | Constitution VII + launcher-contract §3 fix the mechanism; `OpenInApp`'s output contract differs (cd-line vs bare path). No new env var, no `internal/` business rule added | S:85 R:80 A:95 D:90 |
| 4 | Confident | Shared selection helper signature `selectWorktree(ctx, *MenuSession) (path, name string, cancelled bool, err error)` | The `MenuSession` param is required for `open --go` to chain the app menu on one reader (documented byte-theft fix); returning name+cancel covers all callers' needs | S:65 R:75 A:85 D:70 |

4 assumptions (1 certain, 3 confident, 0 tentative).
