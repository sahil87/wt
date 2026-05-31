# Intake: Recency-Aware Listing

**Change**: 260530-rtmf-recency-aware-listing
**Created**: 2026-05-30
**Status**: Draft

## Origin

Initiated from a `/fab-discuss` session exploring the request: *"We need the ability to sort the wt list output by date. In fact, 'recency' is an important aspect of wt. Where else can we apply it?"*

The discussion surfaced a key insight: **recency already exists in the codebase, but only as a hidden heuristic.** Both `wt open` (`open.go:256-287`) and `wt delete` (`delete.go:468-474`) already compute "newest worktree by directory mtime" to pre-highlight a menu default — duplicated inline, never user-visible. The real work is promoting that latent concept into a first-class, consistent, user-facing feature with a single definition.

Interaction mode: conversational. Decisions reached during discussion (see Assumptions for full SRAD scoring):

- **Recency signal = worktree-directory `os.Stat` mtime.** User chose this over `git HEAD mtime` and `git log -1` — it's free and already implemented. The noisiness caveat (any file write bumps it) is accepted for this change and deferred as a concern specific to the follow-up stale-hints change.
- **Default ordering splits by audience.** User chose "recency everywhere a human looks, deterministic for machines": TTY `wt list` → recency; `--json` and `--non-interactive` → stable name/porcelain order; `--sort` is an explicit override in any mode. This reconciles the user's "recency everywhere" instinct with Constitution VI (scriptable on demand) and the script-parsing risk from fab-kit operators.
- **Packaging.** This change bundles items 1–4 (shared helper, `list --sort`, menu reorder, last-active column) because they share the `recencyOf` helper and the `list` surface. Stale-hints is a separate follow-up change (`260530-???-stale-worktree-hints`).

## Why

1. **Problem it solves**: `wt list` has no sorting at all — worktrees print in raw `git worktree list --porcelain` order (main first, then git's internal traversal order). As the number of worktrees grows, the most relevant one (what you last touched) is buried at an arbitrary position. Meanwhile, "recency" is *already* computed in two places (`open`/`delete` menus) but inconsistently (copy-pasted loops) and invisibly (only highlights a default, doesn't order or display).

2. **Consequence of not fixing**: The recency concept stays fragmented and hidden. Users can't sort, can't see when a worktree was last active, and the two menus risk drifting apart (two definitions of "newest"). Recency — which the user identifies as "an important aspect of wt" — remains an implementation detail instead of a feature.

3. **Why this approach**: Consolidate to **one `recencyOf(Info) → time.Time` helper, defined once and consumed everywhere** (`list`, `open`, `delete`). This removes the existing duplication *before* adding features, so the new sort, menu ordering, and display column all share a single source of truth. Mirrors the opt-in, JSON-aware design already established by the `--status` flag (PR #9), keeping the codebase internally consistent.

## What Changes

### 1. Shared recency helper + sort comparator (`internal/worktree/`)

Extract a single helper into the `internal/worktree` package:

```go
// recencyOf returns the recency signal for a worktree: the mtime of its
// working-directory root. Returns zero time if the path cannot be stat'd.
func recencyOf(info Info) time.Time
```

Plus a shared comparator usable by both `list` sorting and the interactive menus, so "newest first" means the same thing everywhere. Replace the two duplicated inline mtime loops:
- `src/cmd/open.go` (~lines 256-287) — currently stats each entry, tracks `newestTime`/`newestPath` inline
- `src/cmd/delete.go` (~lines 468-474) — identical copy-pasted logic

Both call sites switch to the shared helper/comparator. **Behavior-preserving requirement**: the default-highlight selection (which worktree is pre-selected) MUST remain identical for the existing menus after the refactor; only the *ordering* of menu items changes (see item 3).

### 2. `wt list --sort=recent|name|branch` + audience-split default

Add a `--sort` flag to `wt list` accepting `recent`, `name`, or `branch`.

Default ordering depends on output mode:

| Invocation | Default order | Rationale |
|---|---|---|
| `wt list` (TTY/human) | `recent` (newest first) | Ergonomic — what a human wants |
| `wt list --json` | `name` (stable) | Deterministic for machine parsers |
| `wt list --non-interactive` | `name` (stable) | Constitution VI — scriptable |
| `wt list --sort=<x>` | explicit `<x>`, any mode | Escape hatch both directions |

`--sort=recent` uses the shared `recencyOf` comparator. `--sort=name` sorts by worktree name. `--sort=branch` sorts by branch ref. The **main worktree is pinned to position 1 regardless of sort mode**; only the non-main worktrees are reordered below it.
<!-- clarified: main worktree pinned first under all sort modes — matches `git worktree list --porcelain` convention and existing IsMain semantics; predictable anchor -->

**Interaction with `--path` lookup mode**: `wt list --path` is a lookup mode (returns a single path, not a table). `--sort` combined with `--path` MUST **error clearly** with a message such as `--sort cannot be used with --path` and a typed non-zero exit code (per Constitution III) — it MUST NOT silently ignore the flag.
<!-- clarified: --sort + --path errors with typed exit code rather than silent no-op — honors Constitution III and surfaces the user's mistake -->

### 3. Reorder `open`/`delete` menus to full recency ordering

Change the `wt open` and `wt delete` interactive menus from *highlight-only-the-newest* to *list items in recency order (newest first)* via the shared comparator. The newest worktree moves to the top of the menu (position 1) rather than being highlighted at an arbitrary position. The pre-selected default remains the newest (now naturally at the top).

### 4. "Last active" recency column in `wt list --status`

Add a human-readable recency column to `wt list --status` output (e.g. `2h ago`, `3d ago`, `just now`). This makes recency sorting *legible* — sorting by an invisible value is frustrating.

Follow the existing opt-in pointer pattern on `listEntry` (`list.go:27-35`, which uses `Dirty *bool` and `Unpushed *int` with `omitempty`): add a field such as:

```go
LastActive *time.Time `json:"last_active,omitempty"`  // populated only under --status
```

The human display formats it relative ("2h ago"); JSON emits the timestamp (or omits when nil in default mode). Exact field type (raw `*time.Time` vs. pre-formatted) is a spec-level decision.

### Tests

- Extend `TestList_StatusOrderingPreserved` (`list_test.go:379-428`) so the porcelain/stable order invariant still holds in **stable mode** (`--json` / `--non-interactive` / `--sort=name`).
- Add recency-sort ordering tests (`--sort=recent` orders newest-first; deterministic given controlled mtimes).
- Add menu-ordering tests for `open`/`delete` (recency order, newest at top).
- Add an integration test asserting `wt list --json` default order stays stable (name/porcelain), not recency.
- Honor `code-review.md` rule: tests exercising menu/open codepaths MUST NOT leak side-effects (use non-side-effecting targets or `runWt` env isolation).

## Affected Memory

- `wt-cli/list-status-contract.md`: (modify) — document the new `--sort` flag, the audience-split default ordering (TTY recency vs. JSON/non-interactive stable), and the `last_active` field added under `--status`.
- `wt-cli/recency-ordering-contract.md`: (new) — define the `recencyOf` signal (worktree-dir mtime), the shared comparator semantics, and the recency-ordering behavior of the `open`/`delete` menus. May be folded into an existing contract if the spec prefers.

## Impact

- **Code areas**: `src/cmd/list.go`, `src/cmd/open.go`, `src/cmd/delete.go`, `src/internal/worktree/` (new helper — likely `worktree.go` or a new `recency.go`).
- **Data model**: `listEntry` gains a `LastActive` opt-in pointer field. The core `Info` struct may stay unchanged (recency derived on demand).
- **External callers**: fab-kit operators parse `wt list` / `wt list --json`. The audience-split default specifically protects them — JSON/non-interactive order is unchanged. This is the primary compatibility constraint.
- **Dependencies**: none new (uses stdlib `os`, `sort`, `time`).
- **CLI surface spec**: `docs/specs/cli-surface.md` will need the `--sort` flag documented (hydrate-time).

## Open Questions

- ~~Should the main worktree be pinned first under all sort modes, or sorted like any other entry?~~ **Resolved** (clarify 2026-05-31): pinned first under all sort modes.
- ~~For `--sort` + `--path` lookup mode: ignore silently, ignore with a note, or error?~~ **Resolved** (clarify 2026-05-31): error clearly with a typed non-zero exit code.
- `LastActive` field shape: raw `*time.Time` (formatted at display) vs. pre-formatted string in the struct? *(spec-level detail — low blast radius)*
- Should `--sort=recent` in `--json` mode be allowed (explicit opt-in) even though recency-default-in-JSON is disallowed? (Leaning yes — explicit override is fine; only the *default* must stay stable.)

## Clarifications

### Session 2026-05-31

| # | Question | Answer |
|---|----------|--------|
| 8 | Main worktree position under sorting? | Pin main first under all sort modes (only non-main entries reorder) |
| 9 | `--sort` + `--path` behavior? | Error clearly with a typed non-zero exit code; no silent no-op |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Recency signal = worktree-directory `os.Stat` mtime | Discussed — user explicitly chose dir mtime over git HEAD mtime and git log; it's free and already implemented | S:95 R:70 A:90 D:90 |
| 2 | Certain | Default order splits by audience: TTY→recency, `--json`/`--non-interactive`→stable name | Discussed — user chose "TTY recency, JSON stable"; aligns with Constitution VI and `--status` precedent | S:95 R:65 A:85 D:90 |
| 3 | Certain | One shared `recencyOf` helper + comparator consumed by list/open/delete | Discussed — explicit goal was "one definition of recency, used everywhere"; removes existing duplication | S:90 R:75 A:90 D:85 |
| 4 | Certain | This change bundles items 1-4; stale-hints is a separate follow-up change | Discussed — user chose "1 change + follow-up" packaging | S:95 R:80 A:90 D:90 |
| 5 | Confident | `--sort` accepts `recent|name|branch` | Stated in the request; `name`/`branch` are the obvious stable axes alongside `recent` | S:85 R:80 A:80 D:75 |
| 6 | Confident | `LastActive` follows the `*bool`/`*int` opt-in pointer pattern on `listEntry` | Mirrors the established `Dirty *bool`/`Unpushed *int` design from PR #9 | S:80 R:80 A:85 D:75 |
| 7 | Confident | Menu reorder is behavior-preserving for the default-highlight, behavior-changing for item order | Discussed — flagged as an explicit verification point | S:80 R:70 A:80 D:75 |
| 8 | Certain | Main worktree pinned first under all sort modes | Clarified — user confirmed; matches porcelain convention and IsMain semantics | S:95 R:65 A:55 D:55 |
| 9 | Certain | `--sort` + `--path` errors clearly with a typed non-zero exit code | Clarified — user confirmed; honors Constitution III, no silent no-op | S:95 R:70 A:55 D:50 |

9 assumptions (6 certain, 3 confident, 0 tentative, 0 unresolved).
