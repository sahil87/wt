# Intake: Stale Worktree Hints

**Change**: 260530-5fyu-stale-worktree-hints
**Created**: 2026-05-30
**Status**: Draft

## Origin

Initiated from the same `/fab-discuss` session as `260530-rtmf-recency-aware-listing`, exploring *"recency is an important aspect of wt. Where else can we apply it?"*

This is the deliberately-deferred **follow-up** change. Once recency is first-class (Change A: `recencyOf` helper, `--sort`, last-active column), the natural next affordance is turning recency into a *cleanup* signal: flag worktrees untouched for N days as candidates for deletion.

Interaction mode: conversational. Key decision: this was split out from Change A because (a) it is an independent surface, and (b) it carries a genuine open design question about signal quality that Change A does not.

**Dependency**: This change builds on Change A's `recencyOf` helper. It SHOULD be implemented after Change A merges.

## Why

1. **Problem it solves**: Worktrees accumulate. Without a stale signal, users have no prompt to clean up branches they finished with weeks ago — they pile up, clutter `wt list` / `wt open` menus, and consume disk. Recency data (introduced in Change A) is exactly the signal needed to surface "you probably don't need this anymore."

2. **Consequence of not fixing**: Recency stays a *sorting/display* feature only. The cleanup loop — arguably the highest-value application of recency — is never closed. Users keep manually eyeballing which worktrees are old.

3. **Why this approach**: Reuse Change A's single `RecencyOf` definition rather than inventing a parallel idle signal. Surface idleness two ways — a non-destructive *marker* in `wt list`, and a *selector* in `wt delete` (stale-aware menu + `--stale` flag) that routes through the existing rollback-safe delete flow. Idleness only ever *selects* candidates; the per-worktree safety checks still gate every removal. A dedicated `wt prune` command is deliberately **not** added — the `--stale` selector on the existing `wt delete` covers batch cleanup without a new command surface or a duplicated safety path.

## What Changes

### 1. Idle detection

Compute idleness from the `RecencyOf` signal (Change A — `260530-rtmf`, now **merged**): a worktree is **idle** when `now - RecencyOf(wt) > threshold`. Default threshold: **7 days**, overridable via the `--stale` flag (see §3). The signal is **dir-mtime** — reusing the single recency definition in `internal/worktree`, so there is no new `git` subprocess and no second signal to maintain. It is framed honestly as "idle / untouched on disk for Nd," **not** "you haven't worked here" (see the resolved signal-quality note below).

The predicate itself (`now - RecencyOf(path) > threshold`) lives in `src/internal/worktree/` per Constitution V; `cmd/` only consumes it.

### 2. Surface idle marker in `wt list`

Add an **idle marker** to `wt list` recent-mode human output, reusing the existing `Last Active` column (added by `260601-73cv`) — e.g. `feature-x  feat-x  41d ago  ⚠ idle`. This adds **no** new `os.Stat`: the recency key is already computed and persisted into `entries[i].LastActive` on the recent human path, so the idle predicate reads the value already in hand.

JSON mode emits an **additive** `idle` boolean/age field rather than a glyph; default JSON output stays stable (Change A's audience-split contract — `--json`/`--non-interactive` order and shape unchanged).

### 3. Stale-aware delete surface (was deferred — now in scope)

Promoted from "possible extension" to **in scope**, as a *menu annotation + selector*, not a new `wt prune` command:

- **Interactive menu**: `handleDeleteMenu` annotates idle rows (`feature-x (feat-x) — 41d, idle`) and adds an **"All idle (N)"** entry beside the existing "All (N worktrees)". Selecting it routes the idle subset through the existing `handleDeleteMultiple` flow — per-worktree stash/unpushed safety prompts, confirm, rollback, branch cleanup. **No new deletion code path.** `defaultIdx` shifts 2→3 to keep the newest worktree as the pre-selected default (amends `recency-ordering-contract.md`).
- **Non-interactive selector**: a `--stale` flag that pre-selects idle candidates. Single flag using pflag `NoOptDefVal`: bare `--stale` = 7d default; `--stale=30d` overrides the threshold. **The `=` is required** — `--stale 30d` parses `30d` as a positional worktree name (`wt delete` takes `cobra.ArbitraryArgs`). To convert that silent trap into a loud error, `--stale` combined with positional names exits `ExitInvalidArgs` ("mutually exclusive"), matching the existing `--path`↔`--status` mutex idiom.
- **Empty state**: the "All idle (N)" entry is hidden when no worktree crosses the threshold (no "All idle (0)" row).
- **Safety invariant**: idleness is **never** the sole delete gate. The existing per-worktree `HasUnpushedCommits` / `HasUncommittedChanges` / rollback flow always runs. mtime under-reporting is safe-by-direction (it hides idle worktrees, never exposes unsafe ones).
- **Main worktree**: structurally excluded — the menu already skips `ctx.RepoRoot`.

### Signal-quality decision (RESOLVED)

Change A's signal — worktree-directory mtime — is *noisy for staleness*: a worktree committed + pushed 40 days ago but touched yesterday by a `fab sync`/build looks **fresh** and escapes detection, so dir-mtime systematically *under-reports* idleness.

**Resolved: option (b)** — keep dir-mtime, with honest "idle / untouched on disk for Nd" framing. Rationale:
- One signal definition across `list`/`open`/`delete`; zero new `git` subprocesses.
- The delete menu only *offers* deletion — the existing per-worktree safety flow gates the actual removal — so mtime's under-reporting is safe-by-direction.
- `--help` and the spec state plainly that the signal is filesystem mtime, not commit activity.

Option (a) (a cleaner per-staleness signal such as last-commit-date) was rejected for this change: it costs a `git` subprocess per worktree and a second, divergent signal definition, for accuracy the safety flow already backstops. It remains a future option if the under-reporting proves painful.

### Tests

- Stale detection given controlled mtimes / fake clock (threshold boundary cases: just under, just over).
- `wt list` stale-hint display (human + JSON).
- Honor `code-review.md` side-effect isolation rules for any delete-path tests.

## Affected Memory

- `wt-cli/list-status-contract.md`: (modify) — document the idle marker in the recent-mode human view (reusing the `Last Active` column) and the additive JSON `idle` field.
- `wt-cli/recency-ordering-contract.md`: (modify) — note the idle predicate built on `RecencyOf`, the 7d default + `--stale[=Nd]` flag, and the `defaultIdx` 2→3 shift in the delete menu for the "All idle" entry.

## Impact

- **Depends on**: Change A (`260530-rtmf-recency-aware-listing`) for the `RecencyOf` helper — **merged**, dependency satisfied.
- **Code areas**: `src/cmd/wt/list.go` (idle marker + JSON field), `src/cmd/wt/delete.go` (menu annotation, "All idle" entry, `--stale` flag, positional mutex), `src/internal/worktree/` (idle predicate + threshold constant/parse).
- **External callers**: fab-kit operators — the JSON `idle` field is additive; default JSON output stays stable per Change A's contract.
- **Dependencies**: none new (stdlib `time`).

## Open Questions (all RESOLVED)

- ~~Signal choice (blocking)~~ → **dir-mtime + honest "idle/untouched on disk" framing** (option b).
- ~~Default threshold / configurability~~ → **7d default, overridable via `--stale=Nd`** (`=` required; bare `--stale` = 7d).
- ~~Surface~~ → **`wt list` idle marker (reusing `Last Active` column) + stale-aware `wt delete` menu** (both surfaces in scope).
- ~~Scope (hints vs prune)~~ → **delete menu + `--stale` selector in scope**; no dedicated `wt prune` command.
- ~~Main worktree flagged?~~ → **never** — menu structurally skips `ctx.RepoRoot`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Separate follow-up change, building on Change A's `RecencyOf` helper (now merged) | Discussed — user chose "1 change + follow-up" packaging; dependency satisfied | S:95 R:80 A:90 D:90 |
| 2 | Certain | Idle = `now - RecencyOf(wt) > threshold` | Direct application of the recency signal; definitionally the idle predicate | S:90 R:75 A:90 D:85 |
| 3 | Confident | Deletion stays gated by the existing per-worktree safety flow (stash/unpushed/rollback); idleness only *selects* candidates, never the sole gate | Safe default — non-destructive; matches no-hidden-state constitution principle | S:80 R:70 A:85 D:80 |
| 4 | Confident | Idle display is JSON-additive (keeps default JSON stable); human marker reuses the recent-mode `Last Active` column | Mirrors Change A's audience-split design; reuses the already-persisted recency key (no new `os.Stat`) | S:80 R:80 A:80 D:75 |
| 5 | Confident | Default threshold = **7 days**, overridable via `--stale=Nd` | Decided with user; reversible, day-suffixed value matches the `Nd ago` display buckets | S:80 R:80 A:75 D:75 |
| 6 | Confident | Signal = **dir-mtime + honest "idle/untouched on disk" framing** (option b) | Decided with user — resolves the former blocking question; one signal definition, zero new git subprocesses, safety flow backstops under-reporting | S:80 R:70 A:75 D:80 |
| 7 | Certain | Delete surface = stale-aware `wt delete` menu + `--stale` selector (no dedicated `wt prune` command) | Reversed from "deferred" — user elevated the delete menu into scope; routes through existing `handleDeleteMultiple`, no new delete path | S:85 R:65 A:80 D:80 |
| 8 | Confident | `--stale` is a single flag (pflag `NoOptDefVal`=7d, `=` required for override); `--stale` + positional names → `ExitInvalidArgs` mutex | Decided with user; the positional mutex converts the `--stale 30d` parse collision into a loud error, matching the `--path`↔`--status` idiom | S:80 R:75 A:75 D:75 |
| 9 | Confident | "All idle (N)" menu entry hidden when zero worktrees are idle; `defaultIdx` shifts 2→3 | Sensible empty-state + default-preservation; amends `recency-ordering-contract.md` | S:75 R:80 A:80 D:75 |

9 assumptions (3 certain, 6 confident, 0 tentative, 0 unresolved). Run /fab-clarify to bulk-confirm the Confident assumptions, or /fab-continue to advance to spec.
