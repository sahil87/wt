# Intake: Include Main Worktree in `wt go` Menu and Name Resolution

**Change**: 260718-daqj-go-include-main-worktree
**Created**: 2026-07-18

## Origin

Promptless dispatch via `/fab-proceed` from a discussion session. The feature description was synthesized from that discussion (all "verified in code" claims below were re-verified against the working tree during intake):

> Include the main worktree in `wt go`'s selection menu and in by-name resolution (`wt go main`), fixing the list/resolve naming inconsistency. `wt go` is a navigation verb reachable from anywhere in the repo, and the most common navigation from a worktree is back to the main repo — yet the selection menu excludes the main worktree. The exclusion lives in the shared `selectWorktree` helper and was inherited from `selectAndOpen` (`wt open`'s no-arg menu, which only ran from the main repo, where hiding the cwd made sense). When `wt go` (change `260620-3pp5`) reused the helper, the filter came along unexamined.

Key decisions were made in the discussion (drop the filter + pin main row 1; keep the newest-worktree menu default; stable `main` resolver key with exact-basename precedence; one-row menu instead of "No worktrees found.") — see What Changes and the Assumptions table for each, with rationale and rejected alternatives.

## Why

1. **The pain point**: `wt go` exists to navigate between a repo's worktrees, and the single most common navigation from inside a worktree is *back to the main repo* — yet the menu cannot offer it, and `wt go main` fails. The user must know the repo directory's basename (e.g. `wt go wt` for a repo checked out at `.../wt`) — an accidental, undocumented resolution path.
2. **The inconsistency**: `wt list` displays the main entry as `main`, pinned to row 1 in every sort mode (`sortEntries`, `src/cmd/wt/list.go`; `buildBaseEntry` sets `Name = "main"`). The not-found error for `wt go main` says "Use 'wt list' to see available worktrees" — pointing the user at a listing that advertises a name the resolvers cannot resolve. List and resolve disagree on the naming contract.
3. **If we don't fix it**: the most common navigation stays unreachable from the navigation verb; the error message actively misleads; and the accidental basename-of-repo-dir resolution remains the only (undocumented) way to reach main.
4. **Why this approach**: the exclusion lives at exactly one seam (the shared `selectWorktree` helper) and the resolution gap at exactly one other (`resolveWorktreeByName`) — both in `src/cmd/wt/open.go`, both shared by `wt go`, `wt open <name>`, `wt open --select`. Fixing them at the shared seam preserves the documented single-source-of-truth invariant (no per-caller forking) and copies the convention `wt list` already established: main included, named `main`, pinned first.

## What Changes

### 1. `selectWorktree` includes the main worktree, pinned to row 1

Current code (`src/cmd/wt/open.go`, `selectWorktree`, ~line 379):

```go
for _, e := range entries {
    if e.path == ctx.RepoRoot {
        continue
    }
    options = append(options, wtOption{path: e.path, name: filepath.Base(e.path)})
}
```

Change: drop the `ctx.RepoRoot` skip. Instead, build the main entry (the entry whose `path == ctx.RepoRoot`; equivalently the porcelain-first entry) as a `wtOption{path: <mainPath>, name: "main"}` and **pin it as row 1**, mirroring `wt list`'s pin-first convention (`sortEntries` partitions main out and reorders only non-main entries). Non-main worktrees keep their newest-first `wt.SortByRecency` ordering **below** the pinned main row — main is pinned *outside* the recency ordering, never sorted into it.

- Menu row rendering: `main (<branch>)` via the existing per-entry `getBranchForPath` — identical `"name (branch)"` format to every other row.
- The `name` return for the main row is the stable key `"main"` (feeds `wt open`'s tab-naming flows as `{repo}/main`).
- Because the helper is **shared**, `wt open`'s no-arg main-repo menu and `wt open --select`'s menu gain the main row too. This is **intentional consistency** — the documented invariant is that `selectWorktree` is the single source of truth and must NOT fork per-caller behavior (`docs/memory/wt-cli/go-command-contract.md` § Shared `selectWorktree` helper). Selecting main in a launch flow launches an app on the main repo — a meaningful action.

### 2. The pre-selected menu default stays the newest worktree (NOT main)

The default highlight remains on the most-recent *worktree*, preserving existing enter-key muscle memory (create → go → newest). With main pinned as row 1, the index arithmetic becomes:

- `defaultIdx = 2` when at least one non-main worktree exists (main is row 1, newest worktree is row 2);
- `defaultIdx = 1` when main is the only row.

This mirrors `wt delete`'s existing prepended-row pattern (`firstWorktreeIdx` shifting `defaultIdx` 2→3 when "All idle" is present — `docs/memory/wt-cli/recency-ordering-contract.md`). Note: this was the agent's recommendation in the discussion; the user did not explicitly confirm it (Assumption 2, Confident — genuinely arguable, defaulting to main was the rejected alternative).

### 3. `resolveWorktreeByName` learns a stable `main` key (exact-basename precedence)

Current code (`src/cmd/wt/open.go:281-295`) matches `filepath.Base(e.path)` case-insensitively across ALL porcelain entries **including main** — so `wt go <repo-dir-basename>` already navigates to main (accidental, undocumented), while `wt go main` returns `errWorktreeNotFound`.

Change: after the existing exact-basename loop finds no match, resolve `main` (case-insensitive) to the porcelain-first entry's path:

```go
for _, e := range entries {
    if strings.EqualFold(filepath.Base(e.path), name) {
        return e.path, nil
    }
}
if strings.EqualFold(name, "main") && len(entries) > 0 {
    return entries[0].path, nil // porcelain-first entry is always the main worktree
}
return "", errWorktreeNotFound
```

- **Exact-basename match takes precedence**: a worktree directory literally named `main` keeps today's behavior (resolves to that worktree, not the repo root).
- The existing accidental resolution (`wt go <repo-dir-basename>` → main) is left unchanged — the `main` key is purely additive.
- One resolver fixes all three verbs at once: `wt go main`, `wt open main`, `wt open --select main` (all route through `resolveWorktreeByName`).
- Error mapping is untouched: `errWorktreeNotFound` → `ExitGeneralError` (1); git-list failure → `ExitGitError` (3).
- This matches the stable-key convention `internal/worktree/worktree.go` already implements (`List` sets `Name = "main"` for the main entry, rendered `"(main)"` by display layers; `FindByName` matches the stable key) — but that API has **zero callers in `cmd/`** (a parallel unused seam; see Assumption 7 and Non-Goals).

### 4. The "No worktrees found." path becomes the one-row menu

`selectWorktree` currently prints `No worktrees found.` (to stdout) and returns `cancelled=true, noWorktrees=true` when the repo has no non-main worktrees. With main always present in-repo (both `wt go` and the `--select`/no-arg `wt open` paths gate on a validated git repo, where `git worktree list --porcelain` always yields ≥ 1 entry), that branch becomes unreachable: show the one-row menu (just `main (<branch>)`) instead — navigating to main is still meaningful.

This amends the documented cancel/no-worktrees split (`go-command-contract.md` § Cancel vs. no-worktrees split): the `noWorktrees` return flag and the `No worktrees found.` message are retired and the three callers' `if !noWorktrees` guards simplified (Assumption 5, Confident — removing the dead path rather than keeping it defensively). Existing test `TestGo_NoWorktrees_NoConfirmation` (`src/cmd/wt/go_test.go:87`) pins the message and must be rewritten to assert the new one-row-menu behavior (confirmation block still absent until a selection is made; Cancel still prints `Cancelled.` and exits 0).

### 5. Spec and memory amendments (this change IS the spec amendment)

`docs/memory/wt-cli/go-command-contract.md` explicitly pins "filter out the main repo" as a `selectWorktree` invariant and names an explicit spec amendment as the way out ("Future changes … should preserve these invariants unless an explicit spec amendment supersedes them"). This change supersedes:

- **`docs/specs/cli-surface.md`**: the `## wt go [name]` no-arg bullet (menu contents/ordering/default wording — main pinned first, newest-first below, newest pre-selected) and name resolution (`main` resolves to the main worktree); the `## wt open [name|path]` "Omitted, called from the main repo" bullet and name-resolution note gain the same main-row/`main`-key semantics.
- **`docs/memory/wt-cli/go-command-contract.md`** (hydrate): menu contents, the "current-worktree-included" framing (now "all worktrees including main"), the `selectWorktree` helper description (filter dropped, pin added), the cancel/no-worktrees split (flag retired), the resolver's `main` key.
- **`docs/memory/wt-cli/recency-ordering-contract.md`** (hydrate): open/go menus now pin main outside the recency ordering (mirroring `wt list`'s documented pin), non-main ordering unchanged; the `defaultIdx = 1` claim becomes the 1/2 arithmetic above.
- **`docs/memory/wt-cli/menu-navigation-contract.md`** (hydrate, verify-only expected): the `ShowMenu`/`MenuSession` contract itself is untouched — check cross-references that describe the go/open menus' contents.

### Constraints (non-negotiable, unchanged)

- **`wt go`'s stdout machine contract**: bare resolved path as the last stdout line; `WT_CD_FILE` write semantics (mode `0600`, truncate-on-write) per `launcher-contract.md` §3; navigation confirmation stays on stderr. `navigateTo` (`src/cmd/wt/go.go`) is not modified by this change.
- **The shared `selectWorktree` helper must NOT fork per-caller behavior** — all three callers get the identical row set; only the `prompt` string differs (documented invariant).
- **Constitution V**: selection orchestration stays in `cmd/`; no new `internal/` business rule (the change composes existing exported helpers exactly as the current helper does).
- Exit codes, `--non-interactive` refusal, non-TTY fallback, and the `MenuSession` single-reader contract are all untouched.

### Non-Goals

- **`wt delete`'s main exclusion is correct and untouched** at all its list sites (selection map, `--all` collection, `--stale` candidates per its R21) — explicitly out of scope.
- **No wiring-in of `internal/worktree/worktree.go`'s `List`/`FindByName`** (the parallel stable-key API with zero `cmd/` callers). This change patches the `cmd/`-local resolver/helper per the existing pattern; the unused internal API is noted here as a future reconciliation candidate (reuse seam or dead code), not addressed now (Assumption 7).
- No change to the menu default-to-main alternative (rejected), no per-caller filtering fork (rejected), no new flags, no new env vars.

## Affected Memory

- `wt-cli/go-command-contract`: (modify) Menu includes main pinned row 1 rendered `main (<branch>)`; `selectWorktree` filter dropped; `resolveWorktreeByName` gains the stable `main` key with exact-basename precedence; cancel/no-worktrees split amended (`noWorktrees` flag + `No worktrees found.` retired); default-index arithmetic 1/2.
- `wt-cli/recency-ordering-contract`: (modify) Open/go selection menus now pin main to row 1 outside the recency ordering (mirroring `wt list`'s pin-first convention); non-main newest-first ordering and the `SortByRecency` single call site unchanged; `defaultIdx` claim updated.
- `wt-cli/menu-navigation-contract`: (modify) Verification-level touch expected — the `ShowMenu`/`MenuSession` contract is unchanged; update only cross-references that describe the go/open menu contents if any state the main-exclusion.

## Impact

- **Source**: `src/cmd/wt/open.go` — `selectWorktree` (filter drop, main pin, defaultIdx arithmetic, `noWorktrees` retirement), `resolveWorktreeByName` (`main` key), `selectAndOpen` + `openGo` (caller simplification); `src/cmd/wt/go.go` — caller simplification only (`navigateTo` untouched). No `internal/` changes.
- **Tests**: `src/cmd/wt/go_test.go` (`TestGo_NoWorktrees_NoConfirmation` rewritten for the one-row menu; new `wt go main` resolution coverage), `src/cmd/wt/open_test.go` (`TestOpen_MenuOrdersNewestFirst` and menu-content assertions updated for the pinned main row; `wt open main` resolution), `src/cmd/wt/integration_test.go` (`TestIntegration_Go_MenuOrdersNewestFirst` updated; end-to-end `wt go main` from a sibling worktree). Non-TTY fallback tests exercise the same menus and may need row-index updates.
- **Docs**: `docs/specs/cli-surface.md` (`wt go` / `wt open` sections). Memory files via hydrate (see Affected Memory).
- **Behavior surface**: `wt go` (menu + by-name), `wt open` no-arg main-repo menu, `wt open --select` (menu + by-name), `wt open <name>`. Exit codes and the launcher contract (`hop` delegation surface) unchanged.
- **Scale**: single-file core change + caller touch-ups + test/doc updates; no new dependencies.

## Open Questions

- None — all decision points were resolved in the pre-intake discussion or graded as assumptions below (no Unresolved rows; promptless dispatch deferred nothing).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Drop the `ctx.RepoRoot` filter in shared `selectWorktree`; pin main as row 1 rendered `main (<branch>)`, across all three menu callers (`wt go`, `wt open` no-arg, `wt open --select`) | Discussed — explicit decision; single-source-of-truth invariant makes the `wt open` menus gaining the row intentional consistency; copies `wt list`'s pin-first convention | S:90 R:75 A:90 D:90 |
| 2 | Confident | Pre-selected menu default stays the newest worktree (`defaultIdx = 2` with main pinned + ≥1 non-main; `1` when main is the only row), NOT main | Agent's recommendation preserving enter-key muscle memory (create → go → newest); user did not explicitly confirm — genuinely arguable, main-as-default was the rejected alternative | S:80 R:85 A:70 D:55 |
| 3 | Certain | `resolveWorktreeByName` gains a stable `main` key resolving to the porcelain-first entry, with exact-basename match taking precedence (a worktree literally named `main` keeps today's behavior); fixes `wt go main` / `wt open main` / `wt open --select main` in one place | Discussed — explicit decision; matches `wt list`'s displayed name and the internal stable-key convention | S:90 R:80 A:90 D:85 |
| 4 | Certain | In-repo, the "No worktrees found." outcome is replaced by the one-row menu (just main) — navigating to main is still meaningful | Discussed — explicit decision | S:85 R:80 A:85 D:80 |
| 5 | Confident | Retire the `noWorktrees` return flag and the `No worktrees found.` message entirely (simplify the three callers) rather than keep a defensive dead path | Mechanics not discussed, only the outcome; in a validated git repo porcelain always yields ≥1 entry so the branch is unreachable; code-quality bars dead code; trivially reversible | S:60 R:85 A:75 D:65 |
| 6 | Confident | The accidental repo-dir-basename resolution (e.g. `wt go wt` → main) stays unchanged; the `main` key is purely additive to the resolver | Implied by the exact-basename-precedence decision; removing it would be an unrelated breaking change to an existing (if undocumented) behavior | S:70 R:85 A:80 D:70 |
| 7 | Confident | Fix in `cmd/`'s existing resolver/helper; do NOT wire in `internal/worktree`'s parallel `List`/`FindByName` API (zero `cmd/` callers) — note it as a future reconciliation candidate (reuse seam vs. dead code), out of scope here | Constitution V keeps selection orchestration in `cmd/`; the internal API's error shape lacks the `errWorktreeNotFound` sentinel the exit-code mapping needs; minimal-diff | S:60 R:80 A:70 D:50 |
| 8 | Certain | `wt delete`'s main exclusion at all its list sites is untouched | Discussed — explicit decision (correct as-is; deleting main must stay impossible) | S:95 R:90 A:95 D:95 |
| 9 | Certain | `wt go`'s stdout machine contract is unchanged: bare resolved path as last stdout line, `WT_CD_FILE` mode `0600`/truncate semantics, confirmation on stderr; `navigateTo` untouched | Pinned non-negotiable constraint (`launcher-contract.md` §3, go-command-contract, stdout/stderr stream discipline) | S:95 R:85 A:95 D:95 |
| 10 | Confident | `selectWorktree`'s `name` return for the main row is the stable key `"main"` (so `wt open` launch flows tab-name as `{repo}/main`) | Not explicitly discussed; consistent with `wt list` display and the internal stable-key convention; menu row text and returned name should agree | S:65 R:85 A:80 D:70 |

10 assumptions (5 certain, 5 confident, 0 tentative, 0 unresolved).
