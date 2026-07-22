# Intake: Make `wt go` and `wt open` Orthogonal

**Change**: 260722-0is3-go-open-orthogonality
**Created**: 2026-07-22

## Origin

Conversational — a `/fab-discuss` session (2026-07-22) explored the blurred distinction between
`wt go` and `wt open`, then converged on a redesign before invoking `/fab-new`.

> Right now the distinction between "wt open" and "wt go" isn't very clear. Can you find out their
> intended usage, and what cases their behaviours converge, and if we can make their behaviours
> more orthogonal and composable […] I am not so worried about the frozen contract for hop — we
> can update hop also. […] Agreed with this. Go ahead with creating intake with /fab-new

Key decisions from the discussion:

1. **Two-axis model adopted**: selection (which directory/worktree) × action (navigate vs. launch).
   Each menu lives in exactly one verb: `go` owns the "which worktree?" menu, `open` owns the
   "which app?" menu.
2. **Composition point is `wt go --open`**, mirroring `wt create --open` — not `wt open --select`.
3. **`wt open` becomes a pure launcher**: no-arg uniformly opens the *current context*; the
   main-repo worktree-selection menu is removed; `--select`/`--go` deprecated.
4. **The launcher-contract §6 freeze is explicitly waived by the user** ("we can update hop also")
   — the shell-cd mechanism is unified on `wt go`'s `navigateTo` contract and the launcher
   contract is revised to v2. The alternative "one mega-verb, `go` as sugar for
   `--app open_here`" was considered and rejected (navigation-as-an-app muddies the model; the
   selector/launcher vocabulary is already established in docs). The backlog idea [lvyj]
   ("transfer opening folders to hop") was likewise considered and rejected — `wt open` stays the
   toolchain's canonical launcher; this change is effectively lvyj's resolution.

The user explicitly agreed to this behavior matrix (the design contract for this change):

| Invocation | Worktree menu? | App menu? | Result |
|---|---|---|---|
| `wt go` | yes | no | cd to selection |
| `wt go frosty-fox` | no | no | cd directly |
| `wt go --open prompt` | yes | yes | launch selection in chosen app |
| `wt go --open code` | yes | no | launch selection in VS Code |
| `wt open` | no | yes | launch *current* dir (worktree root / repo root / cwd) |
| `wt open <name\|path>` | no | yes | launch that dir |
| `wt open --app code` | no | no | launch current dir in VS Code |

## Why

**Problem**: the selector/launcher split (`go` selects, `open` launches) is documented intent, but
the surface blurs it in four places:

1. `wt open` no-arg from the main repo ≡ `wt open --select` no-arg — `open` embeds a
   context-sensitive worktree selector, so it is not a pure launcher, and `--select` is a no-op
   half the time.
2. `wt go <name>` ≈ `wt open <name> --app open_here` — two implementations of the shell-cd
   contract with subtly different output shapes (`navigateTo`: `WT_CD_FILE` **and** bare path on
   stdout + stderr confirmation; `OpenInApp` "open_here": `WT_CD_FILE` **or** a `cd -- '<path>'`
   stdout line, no confirmation).
3. `wt open <name>` vs `wt open --select <name>` diverge only on path-stat precedence — `--select`
   with a name is really a "force worktree-name interpretation" disambiguator, a different job
   than "run the selector".
4. `wt open --app code` from the main repo errors `ExitInvalidArgs`, yet
   `wt open --select --app code` from the same cwd works — `--app` is not orthogonal to selection
   mode.

**Consequence of not fixing**: every future selection- or launch-adjacent feature (e.g. backlog
[qj66]'s app registry) has to reason about which of two overlapping verbs it extends, and users
keep hitting the `--app`/menu asymmetry and the `--select` no-op.

**Why this approach**: making `go` the sole selection verb with a uniform `--open` composition flag
matches the grammar `wt create --open` already established (target-producing verbs compose with
launching via `--open`), collapses the four blur points, and keeps `wt open` as the stable
subprocess launcher external callers (hop, run-kit) delegate to. The user waived the hop-contract
freeze, which unblocks the shell-cd unification that a purely additive approach could not deliver.

## What Changes

### 1. `wt go` gains `--open <prompt|default|skip|app>` (selection → launch composition)

- New string flag `--open` on `goCmd()` (`src/cmd/wt/go.go`), reusing `wt create --open`'s exact
  value grammar and its **explicit-value rule** (no bare form — `go` has the same
  optional-positional ambiguity `create` solved: a bare `--open code` would parse `code` as the
  positional `[name]`). Values:
  - `prompt` — after selection, show the "Open in:" app menu.
  - `default` — after selection, launch in the auto-detected default app.
  - `skip` — no launch; equivalent to bare `wt go` (navigate). Kept for grammar parity with create.
  - `<app>` — after selection, launch directly in the named app (e.g. `code`, `cursor`,
    `tmux_window`).
- **Semantics: a non-`skip` `--open` replaces navigation with launch** (exactly today's
  `wt open --select` behavior — it does not also cd the parent shell). `--open open_here` yields
  navigation via the unified helper (§3), so bare `wt go` ≡ `wt go --open open_here` in effect.
- Selection source is unchanged: no-arg → the shared `selectWorktree` menu (main pinned row 1,
  newest-first non-main, reachable from anywhere in the repo); `<name>` → `resolveWorktreeByName`
  (case-insensitive, stable `main` key, exact-basename precedence).
- Menu chaining: `wt go --open prompt` runs the selection menu and the "Open in:" menu on **one
  shared `MenuSession`** (the documented single-stdin-reader requirement), exactly as `openGo`
  does today.
- Launch path reuses `openInNamedApp` / `handleAppMenuWithSession` — `go --open` therefore gains
  the launcher exit codes (`ExitByobuTabError` 5, `ExitTmuxWindowError` 6, `ExitGeneralError` for
  unknown app). Existing `wt go` exit codes are unchanged.
- `--non-interactive` interplay mirrors `wt create --open`'s rules verbatim (resolve the exact
  create behavior during apply and copy it): no-name + `--non-interactive` still refuses
  deterministically as today; an explicit `--open prompt` under `--non-interactive` behaves as
  create's equivalent does.

### 2. `wt open` becomes a pure launcher

- **No-arg behavior collapses to one rule: open the current context.**
  - In a worktree → the worktree root (unchanged).
  - In the main repo (non-worktree git cwd) → **the repo root** (changed — today this shows the
    worktree-selection menu).
  - Outside git → the cwd (unchanged).
- The main-repo selection menu and `selectAndOpen` are removed from `open`'s no-arg path; the
  selection helpers (`selectWorktree`, `getBranchForPath`, `resolveWorktreeByName`,
  `errWorktreeNotFound`) remain shared — relocate them to whichever file makes the ownership
  clearest (they are now primarily `go`'s machinery with `open <name>` as a consumer).
- **`--app` becomes orthogonal to every selection mode**: `wt open --app code` from the main repo
  now opens the repo root in VS Code. The `ExitInvalidArgs` "(--app with the main-repo selection
  menu)" case is retired. (Note: the `--list`/`--json` flag-misuse `ExitInvalidArgs` cases shipped
  by `260722-…-open-list-json` / commit `18eed9d` are unrelated query-mode validation and remain
  unchanged — `--list` is a launch-free query surface that composes fine with this redesign; its
  mutual exclusion with `--select` simply follows `--select` into deprecation.)
- **`--select` and `--go` become functional deprecated aliases** (the repo's established
  deprecation convention: still accepted, hidden from `--help`, stderr deprecation warning). The
  warning text points at the replacement: `use "wt go --open" instead`. The `openGo` path they
  drive is retained internally until a later removal change. `--go` (already deprecated toward
  `--select`) now warns toward `wt go --open` directly.
- Migration aid: for one release, a no-arg `wt open` from the main repo prints a one-line stderr
  tip — `tip: to pick a worktree, use wt go (or wt go --open)` — since this is the one invocation
  whose behavior visibly changes. <!-- assumed: one-release transitional tip on the changed no-arg path — low-stakes UX judgment, not explicitly discussed -->
- Name/path resolution is otherwise unchanged: path-first stat precedence, worktree-name fallback
  requiring a git repo, the `main` key, soft git gating. The hop/run-kit-facing surface
  (`wt open <path|name> --app <app>`) is byte-identical.

### 3. Unified shell-cd contract (launcher-contract v2)

- `OpenInApp`'s "open_here" path (`src/internal/worktree/apps.go` or wherever it lives — locate
  during apply) is replaced by / routed through **`navigateTo`'s contract**, which becomes the
  single shell-cd implementation:
  1. Write the resolved absolute path to `WT_CD_FILE` when set (mode `0600`,
     truncate-on-write) — unchanged semantics.
  2. **Always** print the bare resolved path to stdout as the last line (enables
     `cd "$(command wt …)"` scripting uniformly).
  3. Emit the two-line stderr confirmation `→ {repo} / {worktree}  ({branch})` + indented path —
     now also shown for `open_here` launches (repo/branch fields degrade gracefully outside a
     git context, e.g. plain `→ {basename}` — resolve exact rendering at apply).
  4. `WT_WRAPPER`-gated "shell wrapper not loaded" hint unchanged.
- **The `cd -- '<path>'` stdout fallback is retired.** Consumers that eval'd stdout switch to the
  `cd "$( … )"` form. The `wt shell-init` wrapper reads `WT_CD_FILE`, not stdout, so it is
  unaffected — verify during apply.
- **`wt create --open` interplay**: create currently suppresses its final stdout path line when
  the chosen app was `open_here` (because the wrapper consumed it via `WT_CD_FILE`). Under the
  unified contract stdout is uniformly "resolved path as last line", so this suppression special
  case is dropped — `wt create`'s stdout contract simplifies to: path is always the last line.
- **`docs/specs/launcher-contract.md` is revised to v2**: §3 rewritten for the unified semantics
  (single mechanism shared by `open_here` and `wt go`), §5 updated (the `ExitInvalidArgs` menu
  case removed), §6 stability list re-affirmed for the *new* semantics with a changelog note
  recording that this revision was an authorized amendment (user decision, 2026-07-22 discussion)
  per the §6 amendment rule. §2's invocation table updated for the new no-arg behavior.
- **hop coordination — VERIFIED 2026-07-22 (source read at `~/code/sahil87/hop`): no hop change
  required.** `hop <name> open` (`src/cmd/hop/open.go`) execs `wt open <path>` with an explicit
  path (path-first branch, unchanged), stdio inherited (stdout never parsed), exit code checked
  only; the cd handoff is owned by hop's shell shim (`shell_init.go` PASSTHROUGH arm), which
  exports `WT_CD_FILE` to a temp file and reads it after exit — semantics unchanged in v2.
  Cosmetic side effects only: users of `hop <name> open` → "Open here" will now see the bare
  path on stdout and the stderr `→` confirmation (inherited stdio); and a comment in hop's
  `open.go` referencing wt's main-repo selection menu goes stale — an optional doc touch-up in
  the hop repo, not a blocker. run-kit (backlog [qj66]) uses `wt open <path> -a <app>`
  non-interactively and is unaffected.

### 4. Docs and help text

- `docs/specs/cli-surface.md`: rewrite the `## wt go [name]` section (add `--open`, launcher exit
  codes) and the `## wt open [name|path]` section (current-context no-arg rule, `--select`
  deprecation, retired `ExitInvalidArgs` case, updated exit-code list). The agreed behavior
  matrix has already been added there as a `## Selection × action model` section carrying a
  "target model / in flight" annotation (added 2026-07-22, user request) — at apply, keep the
  section and **remove the annotation** once the per-command sections match it.
- `docs/specs/launcher-contract.md`: v2 revision per §3 above.
- Cobra `Long` help text for both commands rewritten around the two-menu ownership model
  ("go owns which-worktree, open owns which-app; compose with `wt go --open`").
- Toolkit standards: CLI-surface/help changes must be checked against `shll standards` per the
  constitution's Toolkit Standards section (do this at apply).

## Affected Memory

- `wt-cli/go-command-contract`: (modify) — major revision: `--open` composition semantics, the
  two-menu ownership model, `--select`/`--go` deprecation toward `wt go --open`, `navigateTo` as
  the single shell-cd implementation, helper relocation.
- `wt-cli/create-output-phases`: (modify) — the open_here stdout-suppression special case is
  dropped; create's stdout contract becomes "path always the last line".
- `wt-cli/flag-naming-conventions`: (modify) — `--select`/`--go` deprecation entries updated
  (target is now `wt go --open`).
- `wt-cli/menu-navigation-contract`: (modify) — menu caller inventory changes (`selectAndOpen`
  removed; `wt go --open prompt` becomes the chained-menu flow; shared-session requirement
  unchanged).

## Impact

- **Source**: `src/cmd/wt/go.go` (new `--open` flag + launch composition), `src/cmd/wt/open.go`
  (no-arg simplification, menu removal, deprecations, helper relocation), `src/cmd/wt/create.go`
  (open_here stdout-suppression removal), `src/internal/worktree/` open_here path (unification
  with `navigateTo` — final home of the shared helper decided at apply under Constitution V:
  orchestration stays in `cmd/`).
- **Tests**: `go_test.go`, `open_test.go`, `create_test.go`, `integration_test.go` — new coverage
  for `--open` grammar/composition, the changed main-repo no-arg behavior, the retired
  `ExitInvalidArgs` case, unified stdout contract; existing menu tests updated for removed
  `open` menu. Side-effect discipline per `fab/project/code-review.md` (prefer `open_here` /
  failing-resolution targets; no real tmux/byobu windows in unit tests).
- **Specs**: `cli-surface.md`, `launcher-contract.md` (v2).
- **External consumers**: hop (coordinated follow-up in its repo; verification task here),
  run-kit (unaffected), `wt shell-init` wrapper (unaffected — reads `WT_CD_FILE`).
- **Behavior changes users will notice**: `wt open` no-arg from the main repo opens the repo root
  instead of showing the worktree menu (mitigated by the transitional tip + deprecation warnings);
  `open_here` launches now print the path on stdout and a confirmation on stderr.

## Open Questions

- None. (The hop-consumption question was resolved by direct source verification on 2026-07-22 —
  see the hop coordination bullet in § What Changes item 3: hop sets `WT_CD_FILE` via its shell
  shim, inherits stdio without parsing stdout, and checks exit codes only, so no hop change is
  required.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Two-menu ownership model: `go` owns worktree selection, `open` owns app selection; composition via `wt go --open` | Explicitly agreed via the behavior matrix in the discussion ("Agreed with this") | S:95 R:70 A:90 D:95 |
| 2 | Certain | `wt open` no-arg opens the current context uniformly (worktree root / repo root / cwd); main-repo selection menu removed | Agreed in the behavior matrix (`wt open` row: no worktree menu, launch current dir) | S:90 R:65 A:85 D:90 |
| 3 | Certain | `go --open` gains launcher exit codes 5/6; the `ExitInvalidArgs` `--app`+menu case is retired; no new exit codes | Follows deterministically from the agreed model + Constitution III (typed codes, no ad-hoc additions) | S:70 R:80 A:90 D:85 |
| 4 | Confident | A non-`skip` `--open` replaces navigation with launch (does not also cd), mirroring today's `wt open --select`; `--open skip` ≡ bare `wt go`; grammar + explicit-value rule copied verbatim from `wt create --open` | Behavior matrix result column shows launch-only for `--open` rows; create's grammar is the established precedent | S:70 R:75 A:80 D:65 |
| 5 | Confident | `--select`/`--go` stay functional as hidden deprecated aliases warning toward `wt go --open`; removal deferred to a later change | Repo's established flag-deprecation convention (flag-naming-conventions memory); avoids a hard break | S:55 R:85 A:90 D:75 |
| 6 | Confident | Shell-cd unifies on `navigateTo`'s contract (WT_CD_FILE when set AND bare path always on stdout + stderr confirmation); `cd -- '<path>'` fallback retired; launcher-contract revised to v2 | User explicitly waived the §6 freeze ("we can update hop also"); medium reversibility since the contract change cascades to external consumers | S:85 R:55 A:80 D:80 |
| 7 | Confident | `wt create`'s open_here stdout-suppression special case is dropped (path uniformly the last stdout line) | Direct consequence of unification; simplifies create's documented contract without breaking "path as last line" | S:50 R:80 A:85 D:70 |
| 8 | Confident | One-release transitional stderr tip on no-arg `wt open` from the main repo | Not explicitly discussed; low-stakes, trivially reversible UX aid for the one visibly-changed invocation | S:35 R:90 A:80 D:50 |
| 9 | Confident | Single change covers all three pieces (go --open, open purification, shell-cd unification); hop update is an external follow-up, not a task here | User invoked /fab-new once with the full three-part description after the sequencing discussion | S:65 R:60 A:70 D:60 |
| 10 | Confident | `go --open` × `--non-interactive` interplay mirrors `wt create --open`'s rules verbatim (exact behavior resolved at apply by reading create's implementation) | Consistency-by-construction with the established grammar; details are codebase-answerable | S:40 R:85 A:75 D:60 |

10 assumptions (3 certain, 7 confident, 0 tentative, 0 unresolved).
