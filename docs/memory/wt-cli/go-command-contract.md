---
type: memory
description: "`wt go` worktree-selection contract — selection-only navigation via `WT_CD_FILE`/stdout (no launch), exit codes, the current-worktree-included menu, and the `wt open --go` composition."
---
# wt-cli: Go Command Contract

> Post-implementation behavior capture for the `wt go` worktree-selection verb
> and the `wt open --go` select-then-launch composition.
> Source change: `260620-3pp5-open-worktree-from-worktree`
> (stderr navigation confirmation added by `260622-log5-dx-copy-polish`).

This file documents the contract `wt go` honors and how `wt open --go` composes
it. `wt go` is the **selector** half of the selector/launcher split: it picks a
worktree of the current repo and navigates there, launching nothing. `wt open`
remains the **launcher** (`go` selects, `open` launches) — the launcher surface
is unchanged and is documented in `docs/specs/launcher-contract.md`. Future
changes touching `src/cmd/wt/go.go`, the `--go` path in `src/cmd/wt/open.go`, or
the shared `selectWorktree` helper should preserve these invariants unless an
explicit spec amendment supersedes them.

## Requirements

### `wt go` is a registered, contract-conformant subcommand

- `goCmd() *cobra.Command` is defined in `src/cmd/wt/go.go` and registered on the
  root command in `src/cmd/wt/main.go`'s `root.AddCommand(...)`, alongside the
  other verbs.
- `Use: "go [name]"`, `Args: cobra.MaximumNArgs(1)`, `SilenceUsage: true`,
  `SilenceErrors: true` — domain errors return via `RunE` / `wt.ExitWithError`
  and render through the root handler, never as cobra usage (Constitution II).
- `wt go --help` long text describes current-repo worktree selection, the
  open=launcher / go=selector framing, and the `WT_CD_FILE` / stdout navigation
  contract.

### `wt go <name>` resolves by name and navigates (no launch)

- `wt go <name>` resolves `<name>` as a worktree of the current repo
  case-insensitively via the **shared** `resolveWorktreeByName` (the same
  resolver `wt open <name>` uses, in `src/cmd/wt/open.go`) and navigates there.
  It launches **no** application — navigation is the only effect.
- On success it routes through `navigateTo(ctx, path)` (below): writes the resolved
  absolute path to `WT_CD_FILE` when set AND prints it to stdout as the last line,
  and emits a stderr navigation confirmation (see the navigation section below).

### `wt go` (no arg) shows the current-repo selection menu from anywhere in the repo

- No-arg `wt go` renders the worktree-selection menu via the shared
  `selectWorktree` helper (`src/cmd/wt/open.go`) with the prompt
  **`"Select worktree to go to:"`**, on a fresh one-shot `wt.MenuSession`.
- The menu is **reachable from anywhere in the repository** — the main repo *or*
  inside another worktree. This is the capability the pre-change menu lacked:
  `wt open`'s no-arg menu was gated to the main repo (in-worktree no-arg `open`
  opens the current folder). `wt go` has no such gate.
- The menu lists non-main worktrees newest-first (see
  `/wt-cli/recency-ordering-contract.md` for the shared `SortByRecency`
  ordering), branch shown per entry, newest pre-selected as the default
  (`defaultIdx = 1`).
- **The current worktree IS included in the menu.** `selectWorktree` filters out
  only the main repo (`e.path == ctx.RepoRoot`); it does NOT filter out the cwd's
  own worktree. So when `wt go` is run from inside worktree `alpha`, `alpha` still
  appears as a selectable row. This is **behavior-preserving** with `wt open`'s
  shared menu (which has always listed every non-main worktree), not a `wt go`
  special case — the single shared helper guarantees both verbs show the identical
  set. Navigating to the worktree you are already in is a harmless no-op `cd`.
- On selection, `wt go` navigates to the chosen worktree via `navigateTo`.
- On Cancel (menu choice `0`), `wt go` prints `Cancelled.` and exits `0` without
  navigating. (When there are no non-main worktrees, `selectWorktree` prints
  `No worktrees found.` itself and the `Cancelled.` line is suppressed — see the
  cancel/no-worktrees split below.)

### `wt go` requires a git repository

- `wt go` (with or without a name) gates on `wt.ValidateGitRepo()` at the top of
  `RunE`. From a non-git cwd it exits `ExitGitError` (3) with a what/why/fix
  message ("Not a git repository" / needs a git repo / run from inside one) —
  worktree resolution walks the repo's worktree list, unreachable outside a repo.
- This is **stricter than `wt open`**: `wt open` softened its git gate so a bare
  path arg / no-arg-cwd works outside a repo, but `wt go` is selection-only and
  selection always needs the worktree list, so the hard git gate is correct.

### `wt go <unknown-name>` exits `ExitGeneralError`; a git-list failure exits `ExitGitError`

- The not-found vs. git-failure distinction routes on the `errWorktreeNotFound`
  sentinel returned by `resolveWorktreeByName` (defined in `src/cmd/wt/open.go`,
  shared with `wt open`):
  - `errors.Is(err, errWorktreeNotFound)` → `ExitGeneralError` (1), message
    "Worktree '<name>' not found" + "Use 'wt list' to see available worktrees"
    (same structure as `wt open`'s not-found path).
  - any other error (a genuine `git worktree list` failure) → `ExitGitError` (3),
    "git worktree list failed" + the underlying error + "Check 'git worktree list'
    from this repo".
- This mirrors `wt open`'s exit-code mapping for the same two failure modes
  (`launcher-contract.md` §5) so `wt go` and `wt open <name>` never disagree.

### `wt go` navigates via `WT_CD_FILE` + stdout — `navigateTo`, never `OpenInApp`

- `navigateTo(ctx *wt.RepoContext, path string)` in `src/cmd/wt/go.go` is the
  dedicated navigation helper (it takes `ctx` so it can render `ctx.RepoName` in the
  stderr confirmation below — `260622-log5` widened the signature from the original
  `navigateTo(path string)`; both call sites, the by-name path and the menu path,
  pass the resolved `ctx`). It:
  1. Emits a **stderr navigation confirmation** (see the dedicated requirement
     below) — the first thing it writes, on the success path only.
  2. Writes `path` to `WT_CD_FILE` (mode `0600`, truncate-on-write) when the env
     var is set — the identical semantics `launcher-contract.md` §3 fixes for
     "Open here". A write failure exits `ExitGeneralError`.
  3. **Always** prints the resolved absolute path to stdout as the **last line**,
     so the no-wrapper scripting form `cd "$(command wt go some-name)"` works.
  4. When `WT_CD_FILE` is unset **and** `WT_WRAPPER != "1"`, prints the same
     two-line "shell wrapper required / `eval "$(wt shell-init)"`" hint to stderr
     that the launcher's "Open here" emits (the `WT_WRAPPER`-gated hint
     convention, `launcher-contract.md` §4).
- `wt go` NEVER `cd`s the parent shell directly — it cooperates via
  `WT_CD_FILE` / stdout and the shell wrapper evaluates the result
  (Constitution VII).
- **It does NOT route through `OpenInApp("open_here", ...)`.** `OpenInApp`'s
  "open_here" path writes `WT_CD_FILE` **OR** prints a `cd -- '<path>'` line
  (mutually exclusive, and the bare path is not always emitted), and lives in the
  launcher/app subsystem. `wt go` must do **both** (write the file when set AND
  always print the bare path) and has no app concept, so a small dedicated helper
  is correct — reusing `OpenInApp` would emit the wrong output contract.
- **No new env var** is introduced. `wt go` reuses `WT_CD_FILE` / `WT_WRAPPER`
  verbatim, so the launcher-contract stability guarantees (§6) are unchanged and
  no constitution amendment is required.

### `wt go` emits a stderr navigation confirmation on success — stdout stays the bare path (`260622-log5`)

- On the **success path only** (a worktree was resolved by name OR selected from
  the menu, i.e. inside `navigateTo`), `wt go` writes a two-line compact-arrow
  confirmation to **stderr** so the user can see *where* they are landing:
  ```
  → idea / frosted-jaguar  (feature-x)
    /home/sahil/code/sahil87/idea.worktrees/frosted-jaguar
  ```
  - Line 1: `→ {ctx.RepoName} / {filepath.Base(path)}  ({branch})` — two spaces
    before the parenthesized branch.
  - Line 2: the absolute resolved `path`, **two-space-indented**.
- The branch is derived via the existing `getBranchForPath(path)` (in
  `src/cmd/wt/open.go`) — the **same** single `git rev-parse --abbrev-ref HEAD` the
  open/go menus already run per entry; `wt go` reuses it rather than issuing a
  fresh git call (Constitution V).
- **The confirmation is success-path-only.** It is emitted from inside
  `navigateTo`, so it never fires on the cancel path (`Cancelled.`) or the
  no-worktrees path (`No worktrees found.`) — those return before `navigateTo` is
  reached.
- **stdout is unchanged — NON-NEGOTIABLE.** The confirmation is diagnostic copy on
  stderr; stdout stays **exactly** the single bare resolved absolute path as the
  final (and only) stdout line. This preserves `cd "$(command wt go <name>)"` and
  the `WT_CD_FILE` write (mode `0600`, truncate-on-write). The confirmation is the
  *first* thing `navigateTo` writes and the bare-path `fmt.Println(path)` stays the
  *last* write, so no confirmation text can leak onto stdout.
- **No color.** The confirmation is emitted as plain text (no `ColorYellow`/etc.),
  so it is NO_COLOR-safe **by construction** — it never touches the package color
  vars and never calls `os.Getenv` afresh. (This is a deliberately simpler posture
  than `PhaseSeparator`'s color-detection-via-blanked-vars; `wt go`'s confirmation
  has no colored variant.)

### `wt go` no-arg under `--non-interactive` / non-TTY is deterministic and non-prompting

- `wt go` accepts a `--non-interactive` bool flag (Constitution VI).
- **No arg + `--non-interactive`**: it does NOT prompt. It exits
  `ExitGeneralError` (1) with a what/why/fix message ("No worktree specified" /
  a no-arg menu has no non-interactive default / "Pass a worktree name:
  wt go <name>"). Erroring deterministically is preferred over silently picking a
  default — a no-arg "pick a worktree" has no obviously-correct silent choice.
- **`wt go <name> --non-interactive`** resolves directly — there is no menu to
  suppress, so the flag is a no-op on that path.
- **Non-TTY (no flag)**: the no-arg menu degrades through the existing
  `ShowMenu`/`MenuSession` non-TTY fallback (numbered-prompt path), the same
  fallback every `wt` menu uses — see `/wt-cli/menu-navigation-contract.md`.

### Shared `selectWorktree` helper — single source of truth (open / go / open --go)

- The worktree-selection logic was extracted out of `selectAndOpen` into
  `selectWorktree(ctx *wt.RepoContext, session *wt.MenuSession, prompt string)
  (path, name string, cancelled, noWorktrees bool, err error)` in
  `src/cmd/wt/open.go`. It is the single source of truth for worktree selection,
  consumed by all three menu callers:
  - `selectAndOpen` — `wt open` no-arg in the main repo (prompt
    `"Select worktree to open:"`), re-expressed on top of the helper
    (behavior-preserving — `TestOpen_MenuOrdersNewestFirst` and the other `open`
    tests still pass).
  - `wt go` no-arg (prompt `"Select worktree to go to:"`).
  - `openGo` — `wt open --go` no-arg (prompt `"Select worktree to open:"`).
- The helper owns the menu UX: filter out the main repo (`ctx.RepoRoot`),
  newest-first `wt.SortByRecency` ordering, per-entry `"name (branch)"` rows via
  `getBranchForPath`, `defaultIdx = 1`, and rendering via the **caller-supplied**
  `MenuSession`. No new business rule moves into `internal/worktree` — the helper
  composes existing exported helpers (Constitution V).
- The `prompt` is a parameter (so the wording can differ per verb); everything
  else (filter, ordering, branch display, default) is identical across callers.
- **Caller-supplied session** is load-bearing for the launch-chaining flows:
  `selectAndOpen` and `openGo` pass the SAME `MenuSession` to `selectWorktree`
  and then to `handleAppMenuWithSession`, so the "Open in:" menu runs on the same
  stdin reader. Chaining two menus on separate readers steals the second menu's
  first keystroke — see `/wt-cli/menu-navigation-contract.md` and `wt.MenuSession`.
  `wt go` owns its own one-shot session (no second menu to chain).

### Cancel vs. no-worktrees split — the `noWorktrees` return flag

- `selectWorktree` returns `cancelled=true` both when the user picks Cancel
  (choice `0`) and when there are no non-main worktrees to select. The two cases
  are disambiguated by the `noWorktrees` return:
  - `cancelled=true, noWorktrees=true` — `selectWorktree` has already printed
    `No worktrees found.` itself (shared by all callers). The caller prints
    **nothing more**.
  - `cancelled=true, noWorktrees=false` — explicit user Cancel. The caller prints
    its own `Cancelled.` line and exits `0`.
- This split lives in every caller identically (`wt go`, `selectAndOpen`,
  `openGo`): the "No worktrees found." message is the helper's to print once; the
  "Cancelled." line is the caller's. A nil error with `cancelled=false`
  guarantees `path` and `name` are populated.

### `wt open --go` composes selection then launch

- A boolean `--go` flag on `wt open` (`openCmd()` in `src/cmd/wt/open.go`). When
  set, `RunE` delegates to `openGo(target, appFlag)` **before** any of the
  non-`--go` resolution branches — the non-`--go` code paths are left untouched.
- `openGo` requires a git repo (else `ExitGitError` (3), same precondition as
  `wt go`), then obtains a worktree path by **selection**:
  - `wt open --go <name>` — resolve `<name>` via `resolveWorktreeByName`
    (not-found → `ExitGeneralError`, list-fail → `ExitGitError`, same mapping as
    `wt go`).
  - `wt open --go` (no name) — `selectWorktree` on a shared session (cancel →
    `Cancelled.` + exit `0`; no-worktrees → `No worktrees found.`).
- It then **launches** the selected worktree via the existing launcher path:
  `--app <app>` opens directly through `openInNamedApp`; otherwise
  `handleAppMenuWithSession` renders the "Open in:" menu on the **same** session
  as the selection menu. `--go` + `--app` compose (select, then open directly in
  the named app).
- `wt open`'s existing surface is **unchanged**: no-arg in a worktree opens the
  current folder; no-arg in the main repo shows menu+launch (`selectAndOpen`);
  `wt open <name>` / `<path>` / `--app` resolve-AND-launch. The `hop`
  launcher-contract surface (`wt open <name>` / `<path>` / `--app` / exit codes /
  `WT_CD_FILE`) is not altered by the `--go` addition.

## Design Decisions

### `wt go` writes `WT_CD_FILE` + prints stdout via a dedicated `navigateTo`, not `OpenInApp`

**Decision**: a small `navigateTo(ctx, path)` helper in `cmd/wt/go.go` does both
the `WT_CD_FILE` write (when set) and the always-print-bare-path stdout emission.
**Why**: `wt go` is a navigation verb with no app concept, and it must do BOTH
sides of the cd contract (write the file when set AND always print the bare path
for `cd "$(command wt go)"`). Constitution VII + `launcher-contract.md` §3 fix
the mechanism; no new env var, no `internal/` business rule.
**Rejected**: reusing `OpenInApp("open_here", ...)` — its output contract is the
wrong shape (it writes `WT_CD_FILE` OR prints a `cd --` line, mutually exclusive,
and never emits a bare path), and it couples `go` to the launcher app catalog.
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

### Navigation confirmation on stderr, never stdout (`260622-log5`)

**Decision**: the compact-arrow `→ {repo} / {worktree}  ({branch})` + indented-path
confirmation goes to **stderr**, emitted from inside `navigateTo` (success path
only); stdout stays the bare resolved path as the final line. `navigateTo` was
widened to `navigateTo(ctx, path)` so it has `ctx.RepoName`, and the branch reuses
`getBranchForPath`.
**Why**: the governing stream-discipline rule — stdout = machine result, stderr =
human copy (see `/wt-cli/create-output-phases.md`) — and the `cd "$(command wt go)"`
/ `WT_CD_FILE` scripting contract both require a bare-path stdout. Putting the
confirmation on stderr closes the `wt go` reassurance gap (it previously printed
only a bare path, less informative than `wt create`'s summary) without touching the
machine contract. Plain text (no color) keeps it NO_COLOR-safe by construction and
matches the create-summary's modest styling.
**Rejected**: printing the block to stdout (breaks every scripting/launcher
consumer); a create-style multi-line labeled block or a single dense line (the
compact-arrow form was the user's explicit choice over both); a fresh
`git rev-parse` for the branch (reuse `getBranchForPath` per Constitution V).
*Introduced by*: `260622-log5-dx-copy-polish`.

### Shared `selectWorktree(ctx, session, prompt)` returning `(path, name, cancelled, noWorktrees, err)`

**Decision**: extract the menu logic from `selectAndOpen` into one helper that
takes a `*MenuSession` and a `prompt`, and returns the chosen `path`+`name`, a
`cancelled` flag, and a `noWorktrees` flag.
**Why**: the `MenuSession` parameter lets `wt open --go` chain the "Open in:"
menu on one stdin reader (the documented byte-theft fix in `MenuSession`); the
`name` return covers tab-naming for the launch flows; the `cancelled` +
`noWorktrees` pair lets the helper own the single `No worktrees found.` message
while each caller owns its own `Cancelled.` line. One helper means
`recency-ordering-contract` and `menu-navigation-contract` hold across all three
callers.
**Rejected**: returning only a path (loses the cancel signal, the name, and the
no-worktrees-vs-cancel distinction); baking the prompt into the helper (the two
verbs want different prompt wording).
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

### `wt go` no-arg under non-interactive errors rather than auto-picking newest

**Decision**: `wt go --non-interactive` with no name exits `ExitGeneralError` (1)
with a "pass a name" message, instead of silently selecting the newest worktree.
**Why**: a no-arg "pick a worktree" menu has no obviously-correct silent default;
erroring surfaces the misuse deterministically and is scriptable (Constitution
VI). Reversible if a "newest default" is later wanted.
**Rejected**: auto-picking the newest worktree (guesses intent); a silent no-op
(swallows the misuse).
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

### The current worktree is included in the `wt go` menu (behavior-preserving, not a special case)

**Decision**: `wt go`'s menu lists every non-main worktree, including the one the
user is currently inside; only the main repo is filtered.
**Why**: the menu is rendered by the SHARED `selectWorktree` helper, which has
always (as `selectAndOpen`) listed all non-main worktrees. Keeping `wt go` on the
identical filter means `wt open`'s menu and `wt go`'s menu never diverge — a
single source of truth. Navigating to the worktree you are already in is a
harmless no-op `cd`, so suppressing the current row would be a `wt go`-only
special case with no real benefit and a divergence cost.
**Rejected**: filtering out the cwd's own worktree in `wt go` only (forks the
shared menu into two behaviors; the helper would need a "current path" param it
otherwise does not want).
*Introduced by*: `260620-3pp5-open-worktree-from-worktree`.

## Cross-references

- Sibling memory: `/wt-cli/recency-ordering-contract.md` — the shared
  `RecencyOf`/`RecencyLess`/`SortByRecency` newest-first ordering that
  `selectWorktree` (and thus `wt go`'s no-arg menu) consumes, alongside
  `wt list`/`wt open`/`wt delete`.
- Sibling memory: `/wt-cli/menu-navigation-contract.md` — the shared
  `ShowMenu`/`MenuSession` arrow-key navigation, TTY gating, and the non-TTY
  numbered-prompt fallback that `wt go`'s no-arg menu degrades through; also the
  single-stdin-reader (`MenuSession`) requirement behind the shared session in
  `selectAndOpen`/`openGo`.
- Spec doc: `docs/specs/cli-surface.md` — the `## wt go [name]` section (behavior
  matrix, exit codes, `--non-interactive`, `WT_CD_FILE`/stdout navigation) and
  the `## wt open [name|path]` `--go` flag / launcher-vs-selector framing.
- Spec doc: `docs/specs/launcher-contract.md` — §3 (`WT_CD_FILE`, "Reused by
  `wt go`" note), §4 (`WT_WRAPPER` hint), §5 (exit-code contract `wt go` mirrors),
  §6 (stability guarantees, unchanged — no new env var).
- Source: `src/cmd/wt/go.go` — `goCmd`, `navigateTo`.
- Source: `src/cmd/wt/open.go` — `openCmd` (`--go` flag), `openGo`,
  `selectWorktree` (shared helper), `selectAndOpen` (re-expressed on the helper),
  `resolveWorktreeByName` / `errWorktreeNotFound` (shared resolver/sentinel),
  `handleAppMenuWithSession`, `openInNamedApp`, `getBranchForPath`.
- Source: `src/cmd/wt/main.go` — `goCmd()` registered in `root.AddCommand(...)`.
- Tests: `src/cmd/wt/go_test.go` (unit: name happy path → `WT_CD_FILE`+stdout,
  unknown name → exit 1, non-git → exit 3, no-arg `--non-interactive` → exit 1
  without prompting; `260622-log5`: success emits the `→`/repo/worktree/branch +
  indented-path confirmation on stderr while stdout stays exactly the bare path,
  and the confirmation is absent on the no-worktrees menu path),
  `src/cmd/wt/integration_test.go` (end-to-end `wt go <name>` from a sibling
  worktree, no-arg menu newest-first ordering, `wt open --go <name> --app open_here`).
- Sibling memory: `/wt-cli/create-output-phases.md` — the canonical stdout =
  machine-result / stderr = human-diagnostic stream-discipline contract this
  confirmation honors; it also documents the `wt.Warn` shared one-line warning
  helper used by create/delete (the verbose init not-found renderer,
  `InitNotFound.RenderWarning`, is the documented exception) (`260622-log5`).
- Constitution: Principle II (Cobra command surface — `SilenceUsage`/
  `SilenceErrors`, `RunE`), III (Typed exit codes — `ExitGitError` /
  `ExitGeneralError`, no new code), V (selection orchestration lives in `cmd/`;
  no new `internal/` business rule), VI (`--non-interactive` deterministic;
  scriptable stdout path), VII (shell-cd via `WT_CD_FILE`, never a direct
  parent-shell `cd`).
