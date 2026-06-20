# Intake: Worktree navigation via `wt go`, launcher via `wt open`

**Change**: 260620-3pp5-open-worktree-from-worktree
**Created**: 2026-06-20

## Origin

Initiated conversationally via `/fab-new`. Raw user input:

> There's no easy way to open a worktree from another worktree. `wt open` seems like the command that should do this, but right now it opens the current folder. Thinking holistically, I am also fine transferring the responsibility of 'opening folders' to the hop command, and make `wt open` open a worktree and delegate to `hop open`.

This was a **conversational, multi-turn** intake. The raw prompt fused two acts that the
final design deliberately separates, and several framings were explored and rejected before
landing on the design below. Key decision trail (all decided *with the user*, this session):

1. **Code trace corrected the premise.** `wt open <name>` *already* opens a different worktree
   by name from inside any worktree (`src/cmd/wt/open.go:76-95`). The genuine gap is that the
   worktree-**selection menu** is reachable only from the main repo (`open.go:104` short-circuits
   no-arg-in-a-worktree to "open the current worktree"). So the problem is **discovery from inside
   a worktree**, not name-resolution.

2. **"Delegate to `hop`" was rejected** — and the user explicitly confirmed keeping `wt`
   canonical. `docs/specs/launcher-contract.md` establishes `wt open` as the canonical launcher
   and `hop` (external repo, `github.com/sahil87/hop`) as the *consumer* that delegates **to** `wt`.
   Inverting that (`wt` shelling out to `hop`) would (a) create a circular/inverted dependency
   against the launcher contract and (b) violate Constitution Principle I (single self-contained
   binary, no runtime deps beyond `git`). Decision: **keep `wt` canonical; `hop` keeps consuming
   `wt open`.**

3. **`wt go` vs `wt switch` naming** — settled on **`go`**. `switch` collides with `git switch`
   (in-place *branch* change), which is the wrong mental model: this act *navigates to a different
   directory*, it never changes the current worktree's branch. `go` reads as navigation.

4. **`hop` already owns cross-repo/worktree navigation** (`hop <repo>/<wt>`, `hop ls --trees`
   which fans out `wt list --json`, `hop <name> open` which "delegates to wt's menu"). So `wt go`
   is deliberately scoped to **the current repo's** worktrees only — `wt` has no cross-repo
   registry and must not grow one (Constitution I). This is the one piece `hop` cannot do without
   re-asking the user to type the repo name.

5. **Final shape (user's own proposal, this session):** split the overloaded `open` into two
   orthogonal verbs plus a composition flag —
   - `wt go [name]` — Act 2: current-repo worktree **selection** (navigation).
   - `wt open [path]` — Act 1: the directory **launcher** (opens the current folder by default).
   - `wt open --go` — compose: select via `go`, then launch via `open`.

6. **cd mechanism already exists** — the user pointed to `wt shell-init`. The `wt()` shell
   wrapper (`src/cmd/wt/shell_init.go`) already allocates a fresh `WT_CD_FILE` on *every* `wt`
   invocation and `cd`'s the parent shell to whatever path the binary writes there. `wt go` reuses
   this verbatim — no new env var, no new mechanism, fully consistent with Constitution VII.

A second, related pain point the user raised — `hop`'s worktree picker requires too much typing
(`hop <repo>/<TAB>`; there is no one-keystroke fuzzy picker, and `hop ls --json` does not even
expose worktrees) — is **out of scope for this `wt` change** (it needs `hop.yaml`'s cross-repo
registry, which lives in the `hop` repo). Recorded as a follow-up in Open Questions.

## Why

**Problem (the pain point).** From inside worktree A of a repo, there is no low-friction way to
*discover and jump to* a sibling worktree. The user must already know the target worktree's name
(`wt open <name>`), because the only worktree-selection menu `wt` offers is gated to the main repo
(`open.go:113`). No-arg `wt open` from within a worktree opens the current folder — useful, but it
gives no picker. The act of "pick which worktree" and the act of "launch a directory in a tool"
are fused into a single overloaded verb (`open`), so neither can be invoked on its own.

**Consequence if unfixed.** Day-to-day worktree-hopping within a repo stays a
type-the-exact-name operation. Users fall back to `wt list` + copy/paste, or to `hop` (which
requires re-typing the repo name even when you're already inside that repo). The overload also
blocks the clean division of labor the toolchain is otherwise built around: `wt open` *wants* to
be the pure launcher that `hop` delegates to, but it can't be while it also owns worktree
selection.

**Why this approach over alternatives.**
- *Rejected: delegate `wt open` → `hop open`.* Inverts the launcher-contract dependency direction
  and breaks Constitution I (single binary, git-only). The user confirmed keeping `wt` canonical.
- *Rejected: add `wt open --pick` only (keep one overloaded verb).* Considered and liked
  mid-discussion, but the user's final preference was the cleaner two-verb split: it de-overloads
  `open` (→ pure Act-1 launcher, the stable primitive `hop` consumes) and gives worktree
  navigation its own home (`wt go`, Act 2). `--go` becomes the ergonomic bridge.
- *Rejected: `wt go` as cross-repo navigator.* That is `hop`'s job and `hop` already does it;
  duplicating it in `wt` would require a cross-repo registry `wt` must not own.
- *Chosen: two orthogonal verbs (`go` = select, `open` = launch) + `--go` composition*, reusing
  the existing `WT_CD_FILE` shell-cd plumbing. Smallest mechanism, non-breaking for `wt open`,
  preserves every contract.

## What Changes

### 1. New verb: `wt go [name]` — current-repo worktree selection (Act 2)

A new cobra subcommand in `src/cmd/wt/go.go`, registered in `main.go`'s `root.AddCommand(...)`.
Its single responsibility is to **resolve a worktree of the current repo and navigate there** —
it does **not** launch any application. It is worktree-aware and **requires a git repository**
(it walks the current repo's worktree list).

**Behavior matrix:**

| Invocation | Behavior |
|------------|----------|
| `wt go` (no arg, in a git repo) | Show the **worktree-selection menu** for the current repo (the same "Select worktree to open:" menu `selectAndOpen` builds today — newest-first recency ordering, branch shown per entry). On selection, navigate to the chosen worktree (see cd mechanism below). Reachable from **anywhere in the repo** — main repo *or* inside a worktree. This is the capability the main-repo-only menu lacks today. |
| `wt go <name>` (in a git repo) | Resolve `<name>` as a worktree (case-insensitive, via the existing `resolveWorktreeByName` logic) and navigate there directly — no menu. |
| `wt go` / `wt go <name>` (not in a git repo) | Exit `ExitGitError` (3) with the standard what/why/fix message — worktree resolution requires a git repo. Mirrors how name-resolution-requires-a-repo is handled in `open.go:96-103`. |
| `wt go <unknown-name>` | Exit `ExitGeneralError` (1): "Worktree '<name>' not found" + "Use 'wt list' to see available worktrees" (same message structure as `open.go:80-84`). |

**Navigation mechanism (no new plumbing).** `wt go` reuses the existing shell-cd contract:

- It **writes the resolved worktree's absolute path to `WT_CD_FILE`** (when set). The `wt()` shell
  wrapper from `wt shell-init` (`src/cmd/wt/shell_init.go`) then `cd`'s the parent shell into that
  path — exactly the mechanism "Open here" already uses. No new env var; reuses
  `launcher-contract.md` §3 semantics (mode 0600, truncate-on-write, contents = resolved dir path).
- It **also prints the resolved absolute path to stdout** as the last line, so the no-wrapper /
  scripting path works: `cd "$(command wt go some-name)"`. Consistent with `wt list --path` and
  `wt create`'s stdout-path contract.
- When `WT_CD_FILE` is unset and no wrapper is loaded, behavior degrades to "print the path"
  (the stdout line is the usable output). The existing `WT_WRAPPER`-gated "shell wrapper not
  loaded" hint convention applies as it does for `open`.

> **Constitution VII compliance:** `wt go` never `cd`'s the parent shell directly — it prints /
> writes a path and the shell wrapper evaluates it. Identical discipline to the existing launcher.

**`--non-interactive`** (Constitution VI): `wt go` with no arg and `--non-interactive` MUST NOT
prompt. It exits `ExitGeneralError` (or selects nothing) deterministically — a no-arg menu has no
sensible non-interactive default. `wt go <name> --non-interactive` resolves directly (no menu, so
nothing to suppress). When stdout is not a TTY, the no-arg menu degrades per the existing menu
fallback in `src/internal/worktree/menu.go` (it already has a non-TTY fallback path).

### 2. `wt open` — narrowed toward a pure launcher (Act 1)

`wt open`'s **existing behavior is preserved** for all current invocations (non-breaking):

- `wt open` (no arg, inside a worktree) → still opens the **current** worktree/folder in a tool
  (`open.go:104` path unchanged).
- `wt open` (no arg, main repo) → still shows the worktree-selection menu then launches
  (`selectAndOpen`, unchanged — see Open Questions for whether this eventually delegates to `wt go`).
- `wt open <path>` / `wt open <name>` / `wt open --app <app>` → unchanged. `wt open <name>`
  continues to resolve-and-**launch** a worktree (this is the launcher-contract behavior `hop`
  depends on; it is NOT removed — see Assumptions).

The conceptual reframe is that `open` is now understood as the **launcher** and `go` as the
**selector**; the no-arg-in-main-repo menu is the one spot where `open` still performs selection,
and whether that should internally delegate to `wt go` is left as an Open Question rather than
changed now (keeps this change non-breaking and tightly scoped).

### 3. New flag: `wt open --go` — select-then-launch composition

A boolean `--go` flag on `wt open`. When set, `wt open` first performs **`wt go`'s selection**
(menu when no positional arg; resolve when `<name>` given) to obtain a worktree path, then runs
its **own launcher** (app menu / `--app`) against that path. Mechanically this is an *internal
composition* of the two commands' shared functions (no subprocess) — the selection helper and the
launch helper both already live in the `cmd/wt` package (`selectAndOpen`, `resolveWorktreeByName`,
`handleAppMenu`, `OpenInApp`).

```
wt open --go            # menu of current repo's worktrees -> launch the pick in a tool
wt open --go <name>     # resolve <name> -> launch it in a tool
wt open --go --app code # menu -> open the pick directly in `code` (skips the app menu)
```

`--go` + `--app` compose (select a worktree, then open it directly in the named app).
`--go` is incompatible only where the underlying selection is impossible (e.g. not in a git
repo → `ExitGitError`, same as `wt go`).

> **Implementation note for apply/plan:** factor the worktree-selection logic currently inside
> `selectAndOpen` (`open.go:254-315`) into a reusable helper that both `wt go` and `wt open --go`
> call, so the menu UX (recency ordering, branch display, shared `MenuSession`) has a single
> source of truth. This keeps `recency-ordering-contract` and `menu-navigation-contract` intact
> across both verbs.

### 4. Docs & specs

- `docs/specs/cli-surface.md` — add a `## wt go [name]` section; update `## wt open` to document
  the `--go` flag and the open=launcher / go=selector framing.
- `docs/specs/launcher-contract.md` — note that `wt go` reuses `WT_CD_FILE` (§3) for navigation;
  confirm the env-var contract is unchanged (no new vars), so no stability-guarantee amendment is
  needed.
- `wt go --help` / `wt open --help` long-form text.

## Affected Memory

- `wt-cli/recency-ordering-contract`: (modify) `wt go`'s no-arg menu is a new consumer of the
  shared `SortByRecency` newest-first ordering — add it alongside `wt list` / `wt open` / `wt delete`.
- `wt-cli/menu-navigation-contract`: (modify) `wt go`'s worktree-selection menu uses the shared
  `ShowMenu` / `MenuSession` — note the new caller and its non-TTY fallback behavior.
- `wt-cli/go-command-contract`: (new) Behavior contract for `wt go` — selection-only semantics,
  `WT_CD_FILE`-based navigation (no launch), stdout path emission, exit codes, and the
  `wt open --go` composition. Sibling to the launcher contract: `go` selects, `open` launches.

## Impact

**Code areas:**
- `src/cmd/wt/go.go` (new) + `src/cmd/wt/go_test.go` (new) — the `wt go` subcommand, unit tests.
- `src/cmd/wt/main.go` — register `goCmd()` in `root.AddCommand(...)`.
- `src/cmd/wt/open.go` — add the `--go` flag and its compose path; extract the selection helper
  out of `selectAndOpen` so `go` and `open --go` share it.
- `src/cmd/wt/integration_test.go` — end-to-end coverage for `wt go` (menu + name + WT_CD_FILE +
  non-git error) per Constitution IV (test what the user sees).
- `src/internal/worktree/` — likely no new logic; `wt go` composes existing exported helpers
  (`SortByRecency`, `ShowMenu`/`MenuSession`, worktree listing). Keep `cmd/` thin per
  Constitution V (selection orchestration in `cmd/`, no new business rules added to `internal/`).

**Exit codes:** `wt go` maps to the existing typed codes (Constitution III) — `ExitGitError` (3)
for non-git cwd / git list failure, `ExitGeneralError` (1) for unknown worktree name. No new exit
code constant is required.

**External consumers:** `hop`'s delegation to `wt open` is **unaffected** — `wt open`'s
launcher-contract surface (`WT_CD_FILE`, `WT_WRAPPER`, exit codes, path/name precedence) is
unchanged. `wt go` adds a new surface but does not alter the contract `hop` relies on.

**No constitution amendment needed:** no new env vars, no new runtime dependency, no module-path
change, no change to the stable launcher-contract surface.

## Open Questions

- Should the no-arg `wt open` from the **main repo** (which today shows a selection menu via
  `selectAndOpen`) eventually delegate to `wt go` internally, so selection lives in exactly one
  place? Deferred — keeping it as-is now makes this change non-breaking; revisit once `wt go`
  ships and the shared selection helper exists.
- Should `wt open` (no-arg, inside a worktree) ever switch its default from "open current" to
  "selection menu"? The user explicitly wants no-arg `open` to keep opening the current folder, so
  this stays out of scope — recorded only to mark it as a consciously-rejected option.
- **Follow-up in the `hop` repo (not this change):** `hop`'s worktree picker requires too much
  typing (`hop <repo>/<TAB>`; `hop ls --json` does not expose worktrees, and the shim is
  tab-completion only — no one-keystroke fuzzy picker). A low-typing cross-repo worktree picker
  belongs in `hop` (it needs `hop.yaml`). Track as a separate `/fab-new` in the `hop` repo.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Keep `wt` canonical; do NOT delegate `wt open` to `hop` | User confirmed explicitly; launcher-contract.md + Constitution I both make `wt` the self-contained launcher and `hop` the consumer — inverting breaks single-binary/git-only | S:95 R:80 A:100 D:95 |
| 2 | Certain | Name the new verb `wt go` (not `wt switch`) | User chose `go`; `switch` collides with `git switch` (in-place branch change), the wrong mental model for directory navigation | S:90 R:85 A:90 D:90 |
| 3 | Certain | `wt go` reuses the existing `WT_CD_FILE` shell-cd mechanism (no new plumbing) | User pointed to `wt shell-init`; the `wt()` wrapper already cd's to any path written to `WT_CD_FILE` on every invocation — Constitution VII satisfied, no new env var | S:90 R:85 A:95 D:90 |
| 4 | Confident | `wt go` is scoped to the **current repo's** worktrees only (not cross-repo) | `hop` already owns cross-repo nav (`hop <repo>/<wt>`, `hop ls --trees`); `wt` has no cross-repo registry and must not grow one (Constitution I). User agreed cross-repo is hop's job | S:80 R:75 A:90 D:80 |
| 5 | Confident | `wt open <name>` keeps its current resolve-AND-launch behavior (not narrowed to selection-only) | This is the launcher-contract surface `hop` depends on; removing launch from `wt open <name>` would be a breaking change to an external consumer. Keep `open`=launch, add `go`=select | S:75 R:65 A:90 D:80 |
| 6 | Confident | `wt go` also prints the resolved path to stdout (in addition to `WT_CD_FILE`) | Enables the no-wrapper / scripting path `cd "$(command wt go)"`; consistent with `wt list --path` / `wt create` stdout-path conventions; Constitution VI scriptability | S:70 R:80 A:85 D:75 |
| 7 | Confident | `--go` composes selection+launch internally (shared helpers), not via subprocess | Both the selection (`selectAndOpen`) and launch (`OpenInApp`/`handleAppMenu`) helpers already live in `cmd/wt`; an internal call is simpler and avoids re-exec overhead | S:70 R:80 A:85 D:80 |
| 8 | Tentative | `wt go` no-arg under `--non-interactive` / non-TTY exits deterministically rather than picking a default | A no-arg "pick a worktree" menu has no obviously-correct silent default; erroring is safer than guessing. But the exact code (ExitGeneralError vs a no-op) and whether to allow a "newest" default could be revisited in apply | S:55 R:70 A:60 D:45 |

8 assumptions (3 certain, 4 confident, 1 tentative, 0 unresolved).
