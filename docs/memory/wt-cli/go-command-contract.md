---
type: memory
description: "`wt go` worktree-selection contract — the selection verb that owns the which-worktree menu, navigation via `WT_CD_FILE`/stdout, `--open <prompt|default|skip|app>` composition (mirroring `wt create --open`) that launches the selection instead of navigating, exit codes, the all-worktrees-including-main menu (main pinned row 1), the `main`-key resolver, and the deprecated `wt open --select`/`--go` aliases that point at `wt go --open`."
---
# wt-cli: Go Command Contract

> Post-implementation behavior capture for the `wt go` worktree-selection verb,
> its `--open` launch composition, and the deprecated `wt open --select`
> select-then-launch alias.
> Source changes: `260620-3pp5`, `260622-log5`, `260717-59u8`, `260718-daqj`,
> `260722-0is3` (each contract below carries its citation).

This file documents the contract `wt go` honors and how the deprecated
`wt open --select` alias composes it. `wt` splits along two axes — **selection**
(which directory/worktree) × **action** (navigate vs. launch) — and each menu
lives in exactly one verb: `wt go` owns the "which worktree?" menu; `wt open`
owns the "which app?" menu. `wt go` is the **selection** verb. By default it
navigates to the chosen worktree, launching nothing; with `--open` it launches
the selection instead. The composition point is `wt go --open`, mirroring
`wt create --open` (target-producing verbs compose with launching via `--open`).
`wt open` is the pure **launcher** — its no-arg surface, `--app` orthogonality,
and the two-menu ownership model are documented in
`docs/specs/launcher-contract.md` (v2) and `/wt-cli/menu-navigation-contract.md`.
Future changes touching `src/cmd/wt/go.go`, the deprecated `--select`/`--go`
path in `src/cmd/wt/open.go`, the shared `selectWorktree` helper, or
`src/internal/worktree/navigate.go` should preserve these invariants unless an
explicit spec amendment supersedes them.

## Requirements

### `wt go` is a registered, contract-conformant subcommand

- `goCmd() *cobra.Command` is defined in `src/cmd/wt/go.go` and registered on the
  root command in `src/cmd/wt/main.go`'s `root.AddCommand(...)`, alongside the
  other verbs.
- `Use: "go [name]"`, `Args: cobra.MaximumNArgs(1)`, `SilenceUsage: true`,
  `SilenceErrors: true` — domain errors return via `RunE` / `wt.ExitWithError`
  and render through the root handler, never as cobra usage (Constitution II).
- `wt go --help` long text describes the two-menu ownership model ("go owns the
  which-worktree menu, open owns the which-app menu; compose the two with
  `wt go --open`"), current-repo worktree selection, the `WT_CD_FILE` / stdout
  navigation contract, and the `--open` launch composition (`260722-0is3`).

### `wt go <name>` resolves by name and navigates (no launch) by default

- `wt go <name>` resolves `<name>` as a worktree of the current repo
  case-insensitively via the **shared** `resolveWorktreeByName` (the same
  resolver `wt open <name>` uses) and, absent `--open`, navigates there. It
  launches **no** application — navigation is the only effect.
- The resolver recognizes a **stable `main` key** (`260718-daqj`): `wt go main`
  (case-insensitively) resolves to the main worktree — the repo root — matching
  the name `wt list` displays and fixing the list/resolve naming inconsistency.
  See § `resolveWorktreeByName`'s `main` key below for the exact-basename
  precedence rule.
- On success it routes through `navigateTo(ctx, path)` (below), which delegates
  to the unified `wt.NavigateTo` helper: writes the resolved absolute path to
  `WT_CD_FILE` when set AND prints it to stdout as the last line, and emits a
  stderr navigation confirmation (see the navigation section below).

### `wt go --open <prompt|default|skip|app>` launches the selection instead of navigating

- `goCmd()` carries a string flag `--open` (`src/cmd/wt/go.go`) reusing
  `wt create --open`'s exact value grammar (`260722-0is3`): `prompt` /
  `default` / `skip` / `<app>`. It has **no short** flag (`-o` is not added —
  the composition flow is the same one `--select` covered short-less; see
  `/wt-cli/flag-naming-conventions.md`) and **no `NoOptDefVal`** — a bare
  `--open code` would parse `code` as the positional `[name]`, the same silent
  footgun `wt create --open` avoids, so a value is always required.
- **A non-`skip` `--open` replaces navigation with launch.** Unset and `skip`
  both mean plain navigation, so bare `wt go` ≡ `wt go --open skip`. The launch
  gate is `launch := openFlag != "" && openFlag != "skip"`. A non-`skip`
  `--open` does NOT additionally cd the parent shell — it is launch-only,
  exactly as the deprecated `wt open --select` behaves.
- Selection source is unchanged by `--open`: no-arg → the shared `selectWorktree`
  menu; `<name>` → `resolveWorktreeByName`. When launching, the no-arg selection
  menu's prompt reads `"Select worktree to open:"` (vs. `"Select worktree to go
  to:"` when navigating) — `selectWorktree` takes the prompt as a parameter.
- **Launch dispatch is `launchSelection(session, openValue, path, repoName,
  wtName)`** (`go.go`): `prompt` renders the "Open in:" app menu
  (`handleAppMenuWithSession` on the shared session when a selection menu already
  ran on that stdin; `handleAppMenu` with a fresh one-shot session on the by-name
  path where no prior menu ran); any other non-`skip` value — including
  `default` and a named app — launches directly via `openInNamedApp`
  (`default` → `wt.ResolveDefaultApp`, `<app>` → `wt.ResolveApp`). `open_here`
  is a named app that routes through the unified `NavigateTo` helper, so
  `wt go <name> --open open_here` is navigation in effect.
- **`wt go --open` gains the launcher exit codes**: `ExitGeneralError` (1,
  unknown app / no default), `ExitByobuTabError` (5), `ExitTmuxWindowError` (6) —
  the same mapping `openInNamedApp` / `handleAppMenuWithSession` apply from the
  substring of the resolved app command. Existing `wt go` navigation exit codes
  (`ExitGitError` 3, `ExitGeneralError` 1) are unchanged; no new exit code is
  introduced (Constitution III).

- **GIVEN** a git repo with worktree `frosty-fox`
- **WHEN** the user runs `wt go frosty-fox --open skip`
- **THEN** the command navigates exactly as bare `wt go frosty-fox` (no launch).

- **GIVEN** a git repo with worktree `bravo` and `WT_CD_FILE` set
- **WHEN** the user runs `wt go bravo --open open_here`
- **THEN** `bravo`'s path is written to `WT_CD_FILE`, printed to stdout as the
  last line, the stderr confirmation is emitted, and the exit code is 0.

- **GIVEN** a git repo with a worktree
- **WHEN** the user runs `wt go <name> --open no-such-app`
- **THEN** the command exits `ExitGeneralError` (1) with an "Unknown app" message.

### `wt go --open prompt` chains the selection menu and the app menu on one session

- No-arg `wt go --open prompt` runs the worktree-selection menu and then the
  "Open in:" app menu on **one shared `wt.MenuSession`** — the single-stdin-reader
  requirement (`/wt-cli/menu-navigation-contract.md`). `goCmd`'s `RunE`
  constructs `session := wt.NewMenuSession()` and passes it to both
  `selectWorktree` and `launchSelection` (which forwards it to
  `handleAppMenuWithSession`). Chaining two menus on separate readers steals the
  second menu's first keystroke — this is the chained-menu flow that the
  deprecated `wt open --select` (via `openGo`) also performs.
- On the **by-name** launch path (`wt go <name> --open prompt`) no worktree menu
  runs, so `launchSelection` is called with a `nil` session and the app menu runs
  on its own fresh one-shot session (`handleAppMenu`).

- **GIVEN** a git repo with a worktree and piped stdin
- **WHEN** the user runs `wt go <name> --open prompt`
- **THEN** the "Open in:" app menu is rendered and no worktree menu is shown.

### `wt go` requires a git repository

- `wt go` (with or without a name, with or without `--open`) gates on
  `wt.ValidateGitRepo()` at the top of `RunE`. From a non-git cwd it exits
  `ExitGitError` (3) with a what/why/fix message ("Not a git repository" / needs
  a git repo / run from inside one) — worktree resolution walks the repo's
  worktree list, unreachable outside a repo.
- This is **stricter than `wt open`**: `wt open` softened its git gate so a bare
  path arg / no-arg-cwd works outside a repo, but `wt go` is a selection verb and
  selection always needs the worktree list, so the hard git gate is correct.

### `wt go <unknown-name>` exits `ExitGeneralError`; a git-list failure exits `ExitGitError`

- The not-found vs. git-failure distinction routes on the `errWorktreeNotFound`
  sentinel returned by `resolveWorktreeByName` (now defined in `src/cmd/wt/go.go`,
  shared with `wt open`):
  - `errors.Is(err, errWorktreeNotFound)` → `ExitGeneralError` (1), message
    "Worktree '<name>' not found" + "Use 'wt list' to see available worktrees".
  - any other error (a genuine `git worktree list` failure) → `ExitGitError` (3),
    "git worktree list failed" + the underlying error + "Check 'git worktree list'
    from this repo".
- This mirrors `wt open`'s exit-code mapping for the same two failure modes
  (`launcher-contract.md` §5) so `wt go` and `wt open <name>` never disagree.

### `resolveWorktreeByName`'s stable `main` key with exact-basename precedence

- `resolveWorktreeByName(name string, ctx *wt.RepoContext)` (relocated to
  `src/cmd/wt/go.go` by `260722-0is3` — see the helper-relocation section)
  resolves in two steps: first the exact-basename loop matches
  `filepath.Base(e.path)` case-insensitively across **all** porcelain entries
  (including main); then, if that finds no match,
  `if strings.EqualFold(name, "main") && len(entries) > 0` returns
  `entries[0].path` — the porcelain-first entry, which is **always the main
  worktree** (the same `mainPath = raw[0]` convention `list.go`'s
  `buildBaseEntry` uses; no extra git call). This one resolver serves
  `wt go main`, `wt open main`, and the deprecated `wt open --select main` at
  once, since all three route through it (`260718-daqj`).
- **Exact-basename match takes precedence.** A worktree directory literally named
  `main` matches in the first loop and resolves to that worktree, not the repo
  root. The `main` key is a fallback consulted only when no directory basename
  matches. The accidental repo-dir-basename resolution also exists
  (`wt go <repo-dir-basename>` → main, e.g. `wt go wt` for a repo at `.../wt`),
  handled by the exact-basename loop.
- This matches the stable-key convention `internal/worktree/worktree.go`
  implements (`List` sets `Name = "main"` for the main entry; `FindByName`
  matches the stable key) — but that internal API has **zero callers in `cmd/`**.
  The `main`-key semantics live in `cmd/`'s resolver rather than the internal API
  (Constitution V keeps selection orchestration in `cmd/`; the internal API lacks
  the `errWorktreeNotFound` sentinel the exit-code mapping needs) — see
  § Design Decisions. The seam's reuse-or-delete reconciliation is tracked in
  `fab/backlog.md`.
- **The `resolveWorktreeByName` `ctx *wt.RepoContext` parameter is unused** —
  the selection/resolution seam's last vestigial `ctx`. Its removal (along with
  the argument at its three call sites) is tracked in `fab/backlog.md`.

### `wt go` navigates via the unified `wt.NavigateTo` helper — `navigateTo` in `cmd/` delegates

- `navigateTo(ctx *wt.RepoContext, path string)` in `src/cmd/wt/go.go` is a thin
  `cmd/`-layer wrapper: it calls
  `wt.NavigateTo(path, ctx.RepoName, wt.BranchForPath(path))` and maps a returned
  error to `ExitGeneralError` with the existing "Cannot write navigation target"
  copy (`260722-0is3`). The observable `wt go` navigation contract (WT_CD_FILE +
  bare-path stdout + stderr confirmation + hint gating + exit codes) is
  byte-preserved.
- The **single shell-cd implementation** now lives in `internal/worktree`:
  `wt.NavigateTo(path, repoName, branch string) error`
  (`src/internal/worktree/navigate.go`). It is shared verbatim by `wt go`'s
  navigation and the launcher's `open_here` action, so the two output contracts
  can no longer drift. Its behavior:
  1. Writes `path` to `WT_CD_FILE` (mode `0600`, truncate-on-write) when the env
     var is set — the write runs **first**, and a write failure returns an error
     with no output emitted (so a failed write never leaks a misleading success
     confirmation).
  2. When `WT_CD_FILE` is unset **and** `WT_WRAPPER != "1"`, prints the two-line
     "shell wrapper required / `eval "$(wt shell-init zsh)"` (or bash)" hint to
     stderr (the `WT_WRAPPER`-gated hint convention, `launcher-contract.md` §4).
  3. Emits a two-line compact-arrow confirmation to **stderr** (see the
     confirmation section below), degrading to `→ {basename}` outside a git
     context (`repoName == ""`).
  4. **Always** prints the bare resolved path to stdout as the **last line**, so
     the no-wrapper scripting form `cd "$(command wt go some-name)"` works.
- `wt go` NEVER `cd`s the parent shell directly — it cooperates via `WT_CD_FILE`
  / stdout and the shell wrapper evaluates the result (Constitution VII).
- **No new env var** is introduced. `wt.NavigateTo` reuses `WT_CD_FILE` /
  `WT_WRAPPER` verbatim, so the launcher-contract stability guarantees (§6) hold
  for the unified v2 semantics (see `launcher-contract.md` §6's amendment note).

- **GIVEN** a git repo with worktree `swift-fox` and `WT_CD_FILE` set
- **WHEN** `wt go swift-fox` runs
- **THEN** stdout is exactly the bare resolved path, `WT_CD_FILE` holds it
  (mode 0600), and the `→ {repo} / swift-fox  ({branch})` confirmation is on
  stderr — byte-compatible with the pre-change contract.

### `wt go` emits a stderr navigation confirmation on success — stdout stays the bare path

- On the **success path only** (a worktree was resolved by name OR selected from
  the menu, i.e. inside `wt.NavigateTo`), `wt go` writes a two-line compact-arrow
  confirmation to **stderr** so the user can see *where* they are landing
  (`260622-log5`; the emitter is now the shared `NavigateTo` per `260722-0is3`):
  ```
  → idea / frosted-jaguar  (feature-x)
    /home/sahil/code/sahil87/idea.worktrees/frosted-jaguar
  ```
  - Line 1: `→ {repoName} / {filepath.Base(path)}  ({branch})` — two spaces
    before the parenthesized branch. Outside a git context (`repoName == ""`)
    it degrades to `→ {filepath.Base(path)}`.
  - Line 2: the absolute resolved `path`, **two-space-indented**.
- The branch is derived via `wt.BranchForPath(path)` — a best-effort single
  `git rev-parse --abbrev-ref HEAD` in `internal/worktree` returning `"unknown"`
  on error (Constitution V — the git op lives out of `cmd/`; it replaced the
  former `cmd/`-local `getBranchForPath`, `260722-0is3`).
- **The confirmation is success-path-only.** It is emitted from inside
  `NavigateTo` (after the WT_CD_FILE write succeeds), so it never fires on the
  cancel path (`Cancelled.`), which returns before `navigateTo` is reached.
- **stdout is unchanged — NON-NEGOTIABLE.** The confirmation is diagnostic copy
  on stderr; stdout stays **exactly** the single bare resolved absolute path as
  the final (and only) stdout line. This preserves `cd "$(command wt go <name>)"`
  and the `WT_CD_FILE` write. The confirmation writes precede the bare-path
  `fmt.Println(path)`, which is the last write, so no confirmation text leaks to
  stdout.
- **No color.** The confirmation is plain text (no `ColorYellow`/etc.), so it is
  NO_COLOR-safe by construction — it never touches the package color vars.

### `wt go` no-arg under `--non-interactive` / non-TTY is deterministic and non-prompting

- `wt go` accepts a `--non-interactive` bool flag (Constitution VI).
- **No arg + `--non-interactive`**: it does NOT prompt. It exits
  `ExitGeneralError` (1) with a what/why/fix message ("No worktree specified" /
  a no-arg menu has no non-interactive default / "Pass a worktree name:
  wt go <name>"). **This refusal runs BEFORE any launch logic** — it is
  selection's precondition, independent of `--open` (`260722-0is3`): mirroring
  `wt create --open`'s rule that `--non-interactive` only changes `--open`'s
  default and never bypasses a precondition, `wt go --non-interactive --open code`
  still refuses with exit 1 and renders no menu. An explicit `--open prompt` under
  `--non-interactive` with a name is honored — the app menu runs, degrading
  through the non-TTY fallback / EOF refusal when stdin is piped.
- **`wt go <name> --non-interactive`** resolves directly — there is no menu to
  suppress, so the flag is a no-op on the navigation path.
- **Non-TTY (no flag)**: the no-arg menu degrades through the existing
  `ShowMenu`/`MenuSession` non-TTY fallback (numbered-prompt path), the same
  fallback every `wt` menu uses — see `/wt-cli/menu-navigation-contract.md`.

- **GIVEN** a git repo with worktrees
- **WHEN** the user runs `wt go --non-interactive --open code`
- **THEN** the command exits 1 with "No worktree specified" and renders no menu.

### Selection helpers live in `go.go`; `wt go` is the primary owner

- `260722-0is3` relocated the shared selection/resolution machinery from
  `open.go` into `src/cmd/wt/go.go` — `wt go` is now the primary owner of
  worktree selection, with the deprecated `wt open --select` path and
  `wt open <name>` as consumers:
  - `selectWorktree(session *wt.MenuSession, prompt string) (path, name string,
    cancelled bool, err error)` — the single source of truth for worktree
    selection.
  - `resolveWorktreeByName(name string, ctx *wt.RepoContext) (string, error)`
    and the `errWorktreeNotFound` sentinel — the shared name resolver.
- The former `getBranchForPath` `cmd/`-local helper is **deleted**; its call
  sites use `wt.BranchForPath` (`internal/worktree/context.go`) instead
  (Constitution V — git op out of `cmd/`).

### Shared `selectWorktree` helper — single source of truth (go / open --select / open <name> consumers)

- `selectWorktree(session, prompt) (path, name, cancelled, err)` is the single
  worktree-selection helper, consumed by:
  - `wt go` no-arg (prompt `"Select worktree to go to:"` when navigating,
    `"Select worktree to open:"` when `--open` launches).
  - `openGo` — the deprecated `wt open --select` no-arg path (prompt
    `"Select worktree to open:"`).
- **`selectAndOpen` is removed** (`260722-0is3`): `wt open`'s no-arg main-repo
  path no longer runs a worktree-selection menu (see
  `/wt-cli/menu-navigation-contract.md` and `launcher-contract.md`). The two
  remaining callers are both selection-then-something flows owned by / delegated
  from `wt go`.
- The helper owns the menu UX (`260718-daqj`): **partition** the porcelain-first
  entry (`entries[0]`, always the main worktree) out from the rest; order the
  non-main entries (`entries[1:]`) newest-first via `wt.SortByRecency`; then
  **prepend the pinned main row** `wtOption{path: entries[0].path, name: "main"}`
  so main is always row 1, OUTSIDE the recency ordering (mirroring `wt list`'s
  `sortEntries` pin-first convention). Rows render `"name (branch)"` via
  `wt.BranchForPath`; `defaultIdx = 2` when ≥1 non-main worktree exists (main
  row 1, newest worktree row 2), else `1` (main is the only row). Rendering is
  via the **caller-supplied** `MenuSession`.
- **Fail-fast on an empty list**: in a validated git repo
  `git worktree list --porcelain` always yields ≥1 entry (main), so an empty
  options slice is unreachable in normal use — but the helper still returns a
  `WtError` on `len(entries) == 0` rather than building a zero-option menu whose
  empty-input default would panic at `options[choice-1]`.
- **Caller-supplied session** is load-bearing for the launch-chaining flows:
  `wt go --open prompt` and `openGo` pass the SAME `MenuSession` to
  `selectWorktree` and then to `handleAppMenuWithSession`, so the "Open in:"
  menu runs on the same stdin reader. Chaining two menus on separate readers
  steals the second menu's first keystroke — see
  `/wt-cli/menu-navigation-contract.md` and `wt.MenuSession`. Plain `wt go`
  (navigating) uses its session for the one selection menu only.

### The current worktree and main are both included in the menu

- `selectWorktree` filters nothing out — main is pinned to row 1, not hidden, and
  the cwd's own worktree appears as a selectable row (`260718-daqj`). When run
  from inside worktree `alpha`, both `alpha` and `main` appear. Navigating to the
  worktree you are already in is a harmless no-op `cd`. The single shared helper
  guarantees both callers show the identical set.

### Cancel is the only helper-signalled early exit

- `selectWorktree` returns `cancelled=true` **only** when the user picks Cancel
  (choice `0`). Each caller (`wt go`, `openGo`) prints its own `Cancelled.` line
  and exits `0`. A nil error with `cancelled=false` guarantees `path` and `name`
  are populated.
- **There is no `noWorktrees` return flag and no `No worktrees found.` message**
  on the selection path (`260718-daqj`). With main always pinned as row 1, a repo
  with only main shows the one-row menu, and the empty-options branch is
  unreachable in a validated git repo (guarded by the fail-fast `WtError` above).

### `wt open --select` / `--go` compose selection then launch (deprecated aliases pointing at `wt go --open`)

- `--select` and `--go` are both **functional hidden deprecated aliases** on
  `wt open` (`openCmd()` in `src/cmd/wt/open.go`), bound to the same `goFlag`
  bool (`260722-0is3`). When either is set, `RunE` delegates to
  `openGo(target, appFlag)` before any of the non-select resolution branches.
- **Deprecation**: both are marked via
  `cmd.Flags().MarkDeprecated("select", "use \"wt go --open\" instead")` and
  `MarkDeprecated("go", "use \"wt go --open\" instead")` — auto-hidden from
  `wt open --help`, with a stderr deprecation warning on use (never stdout). Both
  point at the replacement `wt go --open` (`260722-0is3`). See
  `/wt-cli/flag-naming-conventions.md` for the rename mechanism.
- **The `openGo` path is retained internally** as the alias implementation until
  a later removal change (an explicit Non-Goal of `260722-0is3`); its error copy
  points at `wt go --open` (e.g. the non-git message adds "or use wt go --open").
  It requires a git repo (else `ExitGitError` (3)), resolves a worktree path by
  name (`resolveWorktreeByName`) or via `selectWorktree` on a shared session
  (cancel → `Cancelled.` + exit 0), then launches via `openInNamedApp` (`--app`)
  or `handleAppMenuWithSession` (the "Open in:" menu on the same session).
- The `--list` mutual exclusion with `--select`/`--go` keeps its `ExitInvalidArgs`
  behavior, validated at flag-check time (see `/wt-cli/open-list-contract.md`).

- **GIVEN** a git repo with worktrees `alpha`/`bravo` and `WT_CD_FILE` set
- **WHEN** the user runs `wt open --select bravo --app open_here`
- **THEN** the behavior is unchanged (select-then-launch writes `bravo`'s path to
  `WT_CD_FILE`) AND a stderr deprecation warning naming `wt go --open` is printed.

- **GIVEN** any cwd
- **WHEN** `wt open --help` is rendered
- **THEN** neither `--select` nor `--go` appears.

## Design Decisions

### `wt go --open` composes launch via `wt create --open`'s grammar; a non-`skip` value replaces navigation with launch

**Decision**: `wt go` gains a string `--open <prompt|default|skip|app>` flag with
no short and no `NoOptDefVal`; unset/`skip` navigate, any other value launches the
selection (`prompt` → app menu on a shared session, `default`/`<app>` → direct via
`openInNamedApp`). The composition point is `wt go --open`, mirroring
`wt create --open`.
**Why**: making `wt go` the sole selection verb with a uniform `--open`
composition flag matches the established grammar `wt create --open` set
(target-producing verbs compose with launching via `--open`) and collapses the
four blur points between `go` and `open` (the `open` embedded selector, the two
shell-cd implementations, the `--select <name>` path-precedence overlap, the
`--app`/menu asymmetry). Reusing `openInNamedApp` / `handleAppMenuWithSession`
means `go --open` inherits the launcher exit codes for free and no new exit code
is added (Constitution III). The explicit-value rule copies create's
positional-footgun fix verbatim.
**Rejected**: one mega-verb with `go` as sugar for `--app open_here`
(navigation-as-an-app muddies the selector/launcher model already established in
docs); `wt open --select` as the composition point (keeps the selector embedded
in the launcher — the exact blur this change removes); a bare `--open` form
(`NoOptDefVal` would swallow the positional `[name]`); a `-o` short (the flow is
the same one `--select` covered short-less; easily added later).
*Introduced by*: `260722-0is3-go-open-orthogonality`.

### Unified shell-cd helper lives in `internal/worktree`; `cmd/` delegates

**Decision**: `NavigateTo(path, repoName, branch string) error` +
`BranchForPath(path string) string` live in `internal/worktree`
(`navigate.go` / `context.go`); `cmd/wt/go.go`'s `navigateTo` and
`OpenInApp`'s `open_here` case both delegate to `NavigateTo`. The former
`cmd/`-local `getBranchForPath` is deleted in favor of `wt.BranchForPath`.
**Why**: `OpenInApp` (internal) must call the shared implementation, and
Constitution V wants git ops (`BranchForPath`) out of `cmd/`; a single emitter
makes output-contract drift impossible — the two-implementation drift (`navigateTo`
wrote WT_CD_FILE AND a bare path; `OpenInApp`'s open_here wrote WT_CD_FILE OR a
`cd --` line) is exactly what this change exists to kill.
**Rejected**: keeping `navigateTo` in `cmd/` and duplicating the contract in
`apps.go` (reinstates the drift); passing branch from callers into `OpenInApp`
(widens a public signature for one case).
*Introduced by*: `260722-0is3-go-open-orthogonality`.

### Selection helpers relocate from `open.go` to `go.go`

**Decision**: `selectWorktree`, `resolveWorktreeByName`, and `errWorktreeNotFound`
move from `src/cmd/wt/open.go` into `src/cmd/wt/go.go`.
**Why**: with `open` purified into a pure launcher and the selection menu owned by
`go`, this machinery is now primarily `go`'s — `wt open <name>` and the deprecated
`wt open --select` are consumers of a resolver/selector whose home is the
selection verb. Co-locating ownership with the primary owner keeps the seam
legible.
**Rejected**: leaving the helpers in `open.go` (ownership no longer matches — the
menu they render is `go`'s, not `open`'s).
*Introduced by*: `260722-0is3-go-open-orthogonality`.

### Navigation confirmation on stderr, never stdout

**Decision**: the compact-arrow `→ {repo} / {worktree}  ({branch})` + indented-path
confirmation goes to **stderr**, emitted from inside the shared `NavigateTo` on the
success path only; stdout stays the bare resolved path as the final line.
**Why**: the governing stream-discipline rule — stdout = machine result, stderr =
human copy (see `/wt-cli/create-output-phases.md`) — and the
`cd "$(command wt go)"` / `WT_CD_FILE` scripting contract both require a bare-path
stdout. Putting the confirmation on stderr closes the `wt go` reassurance gap
without touching the machine contract. Plain text (no color) keeps it NO_COLOR-safe
by construction.
**Rejected**: printing the block to stdout (breaks every scripting/launcher
consumer); a create-style multi-line labeled block or a single dense line (the
compact-arrow form was the user's explicit choice); a fresh `git rev-parse` for the
branch (reuse `BranchForPath` per Constitution V).
*Introduced by*: `260622-log5-dx-copy-polish`.

### Shared `selectWorktree(session, prompt)` returning `(path, name, cancelled, err)`

**Decision**: one helper that takes a `*MenuSession` and a `prompt`, and returns
the chosen `path`+`name` plus a `cancelled` flag.
**Why**: the `MenuSession` parameter lets the chained-menu flows (`wt go --open
prompt`, `wt open --select`) chain the "Open in:" menu on one stdin reader (the
documented byte-theft fix in `MenuSession`); the `name` return covers tab-naming
for the launch flows; the `cancelled` flag lets each caller own its own
`Cancelled.` line. One helper means `recency-ordering-contract` and
`menu-navigation-contract` hold across all callers.
**Rejected**: returning only a path (loses the cancel signal and the name); baking
the prompt into the helper (the two actions want different prompt wording:
"…to go to:" vs "…to open:").
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.
The helper's shape is `(session, prompt) → (path, name, cancelled, err)`
(`260718-daqj`): no `ctx` parameter (there is no `RepoRoot` filter — main is a
pinned row, not a hidden entry) and no `noWorktrees` return (with main always
pinned, the empty-options branch is unreachable in a validated git repo).

### `wt go` no-arg under non-interactive errors rather than auto-picking newest

**Decision**: `wt go --non-interactive` with no name exits `ExitGeneralError` (1)
with a "pass a name" message, instead of silently selecting the newest worktree.
The refusal runs before any `--open` launch logic — it is selection's precondition.
**Why**: a no-arg "pick a worktree" menu has no obviously-correct silent default;
erroring surfaces the misuse deterministically and is scriptable (Constitution VI).
Placing it ahead of launch logic mirrors `wt create --open`'s rule that
`--non-interactive` changes `--open`'s default but never bypasses a precondition.
**Rejected**: auto-picking the newest worktree (guesses intent); a silent no-op
(swallows the misuse); running launch logic before the refusal (would let
`--open code` reach the launcher on a no-name invocation).
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

### The current worktree is included in the `wt go` menu (behavior-preserving, not a special case)

**Decision**: `wt go`'s menu lists every worktree, including the one the user is
currently inside; no row is suppressed.
**Why**: the menu is rendered by the SHARED `selectWorktree` helper. Keeping
`wt go` on the identical row set means the selection menu never diverges per
caller — a single source of truth. Navigating to the worktree you are already in
is a harmless no-op `cd`, so suppressing the current row would be a special case
with no real benefit and a divergence cost.
**Rejected**: filtering out the cwd's own worktree (forks the shared menu; the
helper would need a "current path" param it otherwise does not want).
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

### Main is included in the menu, pinned to row 1 via a partition (not a sort key)

**Decision**: the shared `selectWorktree` helper includes the main worktree,
pinned to row 1 rendered `main (<branch>)`, by partitioning the porcelain-first
entry (`entries[0]`, always main) out, sorting the non-main slice (`entries[1:]`)
newest-first, then **prepending** the pinned main row — mirroring `list.go`'s
`sortEntries`. The pre-selected default stays the newest *non-main* worktree
(`defaultIdx = 2` with ≥1 non-main, `1` when main-only).
**Why**: `wt go` is a navigation verb reachable from anywhere in the repo, and the
single most common navigation from a worktree is *back to main* — yet the menu
could not offer it. Pinning main outside the recency ordering copies `wt list`'s
convention. Because the helper is shared, the deprecated `wt open --select`'s menu
gains the main row too — intentional consistency.
**Rejected**: giving main an artificial max-recency so the sort floats it first
(couples the pin to the comparator, fragile); defaulting the highlight to main
(breaks the newest-worktree enter-key habit); forking the filter per-caller
(violates the single-source-of-truth invariant).
*Introduced by*: `260718-daqj-go-include-main-worktree`.

### `main` resolver key resolves to `entries[0].path`, exact-basename precedence

**Decision**: `resolveWorktreeByName` carries a stable `main` key — after the
exact-basename loop finds no match, `main` (case-insensitive) resolves to
`entries[0].path` (the porcelain-first entry, always the main worktree). Exact
basename matches keep precedence.
**Why**: `entries[0]` is always main (`list.go` sets `mainPath = raw[0].path`), so
the key costs no extra git call and matches `buildBaseEntry`'s convention and the
name `wt list` displays. Fixing it at the one shared resolver keeps `wt go main`,
`wt open main`, and `wt open --select main` consistent. The parallel stable-key
API in `internal/worktree/worktree.go` is NOT wired in: Constitution V keeps
selection orchestration in `cmd/`, and that API lacks the `errWorktreeNotFound`
sentinel the exit-code mapping needs (tracked in `fab/backlog.md`).
**Rejected**: a dedicated "find main" git call (redundant); wiring in the internal
`List`/`FindByName` API (fails the sentinel/Constitution-V constraints).
*Introduced by*: `260718-daqj-go-include-main-worktree`.

## Cross-references

- Sibling memory: `/wt-cli/recency-ordering-contract.md` — the shared
  `RecencyOf`/`RecencyLess`/`SortByRecency` newest-first ordering that
  `selectWorktree` consumes for the non-main rows, alongside
  `wt list`/`wt open`/`wt delete`; also the pin-first convention (`260718-daqj`)
  that keeps main pinned to row 1 *outside* that ordering and the
  `defaultIdx = 2/1` arithmetic it produces.
- Sibling memory: `/wt-cli/open-list-contract.md` — the `wt open --list [--json]`
  query surface; `--list` is mutually exclusive with the deprecated
  `--select`/`--go` aliases, rejecting the combination at flag-check time with
  `ExitInvalidArgs`.
- Sibling memory: `/wt-cli/menu-navigation-contract.md` — the shared
  `ShowMenu`/`MenuSession` arrow-key navigation, TTY gating, and the non-TTY
  numbered-prompt fallback that `wt go`'s no-arg menu degrades through; also the
  single-stdin-reader (`MenuSession`) requirement behind the shared session in
  `wt go --open prompt` / `openGo`, and the caller inventory (`selectAndOpen`
  removed; `wt go --open prompt` is the chained-menu flow — `260722-0is3`).
- Sibling memory: `/wt-cli/flag-naming-conventions.md` — the deprecated
  `--select`/`--go` aliases (both now pointing at `wt go --open` via
  `MarkDeprecated`), the `--open` explicit-value / no-`NoOptDefVal` rule, and the
  no-short decision.
- Sibling memory: `/wt-cli/create-output-phases.md` — the canonical stdout =
  machine-result / stderr = human-diagnostic stream-discipline contract this
  navigation confirmation honors, and the `wt create --open open_here` stdout
  contract that simplified to "path always the last line" under the same unified
  `NavigateTo` helper (`260722-0is3`).
- Spec doc: `docs/specs/cli-surface.md` — the `## wt go [name]` section
  (`--open` grammar, composition semantics, launcher exit codes) and the
  `## wt open [name|path]` section (current-context no-arg rule, `--select`/`--go`
  deprecations); the `## Selection × action model` behavior matrix.
- Spec doc: `docs/specs/launcher-contract.md` (v2) — §2 (invocation table, new
  no-arg behavior + `--app` orthogonality), §3 (the unified `WT_CD_FILE` + always-
  bare-path stdout + stderr confirmation mechanism shared by `open_here` and
  `wt go`; `cd --` fallback retired), §5 (exit-code contract `wt go --open`
  reuses), §6 (stability guarantees re-affirmed for the v2 semantics with the
  authorized-amendment changelog note).
- Source: `src/cmd/wt/go.go` — `goCmd` (`--open` flag), `launchSelection`,
  `navigateTo` (delegates to `wt.NavigateTo`), and the relocated `selectWorktree`
  / `resolveWorktreeByName` / `errWorktreeNotFound`.
- Source: `src/cmd/wt/open.go` — `openCmd` (`--select`/`--go` both deprecated via
  `MarkDeprecated` toward `wt go --open`, sharing `goFlag`; `--app`/`-a`), `openGo`
  (retained deprecated composition; error copy points at `wt go --open`),
  `openInNamedApp`, `handleAppMenu`, `handleAppMenuWithSession` (its doc-comment's
  stale `selectAndOpen` reference is a tracked documentation-cleanup candidate).
- Source: `src/internal/worktree/navigate.go` — `NavigateTo`, the single unified
  shell-cd implementation; `src/internal/worktree/context.go` — `BranchForPath`
  (best-effort, `"unknown"` fallback); `src/internal/worktree/apps.go` —
  `OpenInApp`'s `open_here` case routes through `NavigateTo`.
- Source: `src/cmd/wt/main.go` — `goCmd()` registered in `root.AddCommand(...)`.
- Tests: `src/cmd/wt/go_test.go` (unit: name happy path → `WT_CD_FILE`+stdout,
  unknown name → exit 1, non-git → exit 3, no-arg `--non-interactive` → exit 1;
  `260722-0is3`: `--open skip` ≡ bare navigate, `--open open_here <name>` unified
  navigation, `--open <unknown-app>` → exit 1, `<name> --open prompt` chained
  "Open in:" menu with no worktree menu, no-name `--non-interactive --open code`
  refusal, and the menu-ordering test through `wt go --open prompt`),
  `src/cmd/wt/open_test.go` (`260722-0is3`: main-repo orthogonality tests
  replacing the retired `TestOpen_ErrorFromMainRepoWithoutTarget` /
  `TestOpen_MenuOrdersNewestFirst`; help hides both `--select` and `--go`;
  `--select` deprecation-warning test),
  `src/cmd/wt/integration_test.go` (`260722-0is3`:
  `TestIntegration_GoOpen_NameArg_ResolvesAndLaunches`,
  `TestIntegration_OpenSelect_NameArg_ResolvesAndLaunches` flipped to expect the
  deprecation warning, the unified launcher-contract stdout/stderr shape).
- Constitution: Principle II (Cobra command surface — `SilenceUsage`/
  `SilenceErrors`, `RunE`), III (Typed exit codes — reuses `ExitGitError` /
  `ExitGeneralError` / launcher codes 5/6, no new code), V (selection
  orchestration and the `cmd/` delegation stay in `cmd/`; the shell-cd
  implementation and `BranchForPath` git op live in `internal/`), VI
  (`--non-interactive` deterministic; scriptable stdout path), VII (shell-cd via
  `WT_CD_FILE`, never a direct parent-shell `cd`).
