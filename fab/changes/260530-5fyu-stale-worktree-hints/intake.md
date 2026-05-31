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

3. **Why this approach**: Reuse Change A's single `recencyOf` definition rather than inventing a parallel staleness signal. Surface staleness as a non-destructive *hint* first (a marker in `wt list`), keeping any actual deletion behind existing explicit `wt delete` flows. A dedicated `wt prune` command is a possible extension but is out of scope here unless the spec elevates it.

## What Changes

### 1. Stale detection

Compute staleness from the `recencyOf` signal (Change A): a worktree is **stale** when `now - recencyOf(wt) > threshold`. Default threshold: 30 days (`[NEEDS CLARIFICATION]` — confirm default and whether it is configurable via flag/env).

### 2. Surface stale hints in `wt list`

Add a stale indicator to `wt list` output — e.g. a marker in the status column or a dedicated hint. Following the `--status` opt-in precedent, staleness display is likely opt-in (e.g. shown under `--status`, or behind its own `--stale`/`--show-stale` flag). Exact surface is a spec decision.

JSON mode emits a boolean/age field rather than a glyph, consistent with the audience-split design from Change A.

### 3. (Possible) prune affordance

A `wt prune` or `wt delete --stale` that batch-targets stale worktrees is a candidate extension. **Out of scope unless the spec elevates it** — start with hints only. Any deletion MUST go through the existing rollback-safe delete path and remain explicit (no auto-delete).

### THE SIGNAL-QUALITY OPEN QUESTION (carried from discussion)

Change A's chosen signal — worktree-directory mtime — is *noisy for this specific use case*. A worktree where everything was committed and pushed 40 days ago, but a `fab sync` or background build touched a file yesterday, will look **fresh** and silently escape stale detection. So dir-mtime systematically *under-reports* staleness.

This change MUST resolve one of:
- **(a)** Use a less-noisy signal *specifically for staleness* (e.g. last commit date, or `.git/worktrees/<id>/HEAD` mtime), accepting a divergence from Change A's display signal; or
- **(b)** Keep dir-mtime and honestly frame the hint as "untouched **on disk** for N days," not "you haven't worked here" — and document the limitation.

This is the core reason the change was deferred rather than bundled into Change A.

### Tests

- Stale detection given controlled mtimes / fake clock (threshold boundary cases: just under, just over).
- `wt list` stale-hint display (human + JSON).
- Honor `code-review.md` side-effect isolation rules for any delete-path tests.

## Affected Memory

- `wt-cli/list-status-contract.md`: (modify) — document the stale hint surface and its semantics.
- `wt-cli/recency-ordering-contract.md`: (modify) — note the staleness signal and, if it diverges from the display signal, document why.

## Impact

- **Depends on**: Change A (`260530-rtmf-recency-aware-listing`) for the `recencyOf` helper.
- **Code areas**: `src/cmd/list.go` (hint display), `src/internal/worktree/` (staleness predicate), possibly `src/cmd/delete.go` if a `--stale` selector is added.
- **External callers**: fab-kit operators — a JSON stale field is additive; keep default JSON output stable per Change A's contract.
- **Dependencies**: none new (stdlib `time`).

## Open Questions

- **Signal choice (blocking):** option (a) cleaner signal for staleness vs. option (b) keep dir-mtime + honest framing. This is the central decision.
- Default threshold (30 days?) and whether it is configurable (flag, env var, or fixed).
- Surface: opt-in under `--status`, a dedicated `--stale` flag, or always-on hint?
- Scope: hints only, or include a `wt prune` / `wt delete --stale` batch affordance?
- Should the main worktree ever be flagged stale? (Almost certainly never.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | This is a separate follow-up change, depending on Change A's recencyOf helper | Discussed — user chose "1 change + follow-up" packaging | S:95 R:80 A:90 D:90 |
| 2 | Certain | Stale = `now - recencyOf(wt) > threshold` | Direct application of the recency signal; definitionally the staleness predicate | S:90 R:75 A:90 D:85 |
| 3 | Confident | Hints first; actual deletion stays explicit via existing delete path (no auto-delete) | Safe default — non-destructive; matches the no-hidden-state constitution principle | S:75 R:70 A:80 D:75 |
| 4 | Confident | Stale display is opt-in and JSON-additive (keeps default JSON stable) | Mirrors Change A's audience-split design and the --status precedent | S:75 R:75 A:80 D:70 |
| 5 | Tentative | Default staleness threshold = 30 days | Reasonable default but not decided; reversible | S:50 R:75 A:55 D:55 |
| 6 | Unresolved | Which signal powers staleness: (a) cleaner per-staleness signal vs. (b) dir-mtime + honest framing | Asked — undecided; dir-mtime is noisy for staleness (fab sync/builds mask it). Central design question, deferred from discussion | S:35 R:45 A:45 D:30 |
| 7 | Tentative | No `wt prune` batch command in this change (hints only) | Scoped out unless spec elevates it; keeps the change focused | S:55 R:65 A:60 D:55 |

7 assumptions (2 certain, 2 confident, 3 tentative, 1 unresolved).
