# Intake: wt open — generalize to any directory

**Change**: 260508-evbf-wt-open-any-directory
**Created**: 2026-05-08
**Status**: Draft

## Origin

> Generalize `wt open` to work on any directory, not just git worktrees. Today `wt open` hard-fails with `ExitGitError` if `ValidateGitRepo()` returns an error, which forces every caller (notably `hop <name> open`) to either be inside a git repo or work around it. The goal is to make `wt open` the universal directory launcher in our toolchain — `hop open` already delegates to it via the `WT_CD_FILE`/`WT_WRAPPER` env-var contract, and we want that delegation to keep working even when the resolved directory is not a git repo.

This change was scoped via a `/fab-discuss` session. Key conversational decisions:

- **`wt open` becomes the canonical launcher.** Considered an alternative of extracting a separate `wt launch` verb (or a standalone `launcher` binary) and rejected it: it fragments the menu defaults (`last-app` cache, `TERM_PROGRAM` detection, tmux/byobu session awareness) across multiple commands, and "launch this directory" is not a verb users type — they type `wt open` or `hop foo open`, with launching as the trailing step.
- **Subprocess delegation over shared library.** Considered extracting `internal/worktree/apps.go` into a public package that both `wt` and `hop` import. Rejected for now: subprocess delegation lets `hop` inherit `wt`'s view of the world (cached defaults, session detection) for free, and avoids duplicating config-resolution logic. The library extraction is left as a future option (Option 3 in discussion); this change does not preclude it.
- **Directories only.** Explicit guardrail: `wt open` will accept any *directory*, but not URLs or arbitrary files. Otherwise we re-invent `xdg-open` / `open(1)` and the abstraction collapses.
- **Change type: `refactor`.** Loosens an existing precondition and formalizes an existing implicit contract. No new user-visible verbs, no new commands, no behavioral change for any invocation that worked before.

## Why

`wt open` already does two structurally separable things:

1. **Resolve a target path** — from current dir, named worktree, explicit path arg, or interactive selection menu over the worktree list.
2. **Open that path in a detected app** — VSCode, Cursor, terminal-of-your-OS, byobu/tmux window/session, file manager, copy-to-clipboard, or `cd` via the shell wrapper.

Step 2 has zero git/worktree coupling. `OpenInApp(appCmd, path, repoName, wtName)` in `src/internal/worktree/apps.go` only uses `repoName` and `wtName` to compose the byobu/tmux tab name (`repoName + "-" + wtName`); everything else is "open this directory in this app." The OS detection, app catalog, last-choice cache, menu UX, and shell-wrapper `cd` integration are all generic.

But `openCmd().RunE` in `src/cmd/wt/open.go` gates the whole flow behind `wt.ValidateGitRepo()`, which exits with `ExitGitError` if the cwd is not a git repo. This forces every caller into a git-repo cwd:

```go
// src/cmd/wt/open.go:31-36 (current)
if err := wt.ValidateGitRepo(); err != nil {
    wt.ExitWithError(wt.ExitGitError,
        "Not a git repository",
        "This command requires a git repository",
        "Navigate to a git repository and try again")
}
```

The pain points this creates:

- **`hop` already works around it.** `hop <name> open` (in `github.com/sahil87/hop`, `src/cmd/hop/open.go:24-70`) resolves a name to a `repo.Path` and shells out to `wt open` with `cwd=repo.Path` plus `WT_CD_FILE` and `WT_WRAPPER=1`. This works only because `repo.Path` happens to be a git repo today. The moment `hop` adds a non-git target (notes folder, docs site, bare directory), the contract silently breaks: `wt open` rejects with `ExitGitError`.
- **`wt open <some/path>` fails when run outside a repo.** Even though the user is naming the path explicitly — i.e., resolution does not need git — the precondition still applies. `wt open ~/Downloads` from a non-git dir fails with the same `ExitGitError`.
- **The contract between `hop` and `wt` is implicit.** `WT_CD_FILE`, `WT_WRAPPER`, the "non-zero exit means don't read the cd file" rule, the `open_here` write semantics — none of these are documented anywhere except in the consumer's comments and the producer's switch case. A change in `wt`'s `OpenInApp` could break `hop` silently with no test coverage flagging it.

Doing nothing means: `hop` cannot grow beyond git repos without forking the launcher, the implicit contract drifts, and the launching subsystem stays artificially scoped to "what `wt` happens to support today" rather than "any directory the user can name."

The chosen approach (loosen precondition + document contract) is the smallest change that resolves all three pain points without committing to a refactor that may not pay back. It is reversible: if a future shared-library extraction (Option 3) becomes warranted, this change does not increase its cost.

## What Changes

### 1. Soft git-context detection in `wt open`

**File**: `src/cmd/wt/open.go`

Replace the hard `ValidateGitRepo()` fail at the top of `openCmd().RunE` with soft detection. Reorganize the resolution flow so git context is *enrichment*, not a *precondition*:

- **In a worktree** (`wt.IsWorktree()` returns true): existing behavior unchanged. Smart defaults from `DetectDefaultApp`, name-based resolution via `resolveWorktreeByName`, tmux/byobu tab names use `repoName-wtName`.
- **In a git main repo** (git repo but not in a worktree): existing behavior unchanged. With no args → `selectAndOpen` shows worktree menu. With `<path>` arg → opens path. With `<name>` arg → resolves via `resolveWorktreeByName`.
- **In a non-git directory**: new behavior.
  - With no args → open the current directory (cwd), equivalent to `wt open .`.
  - With `<path>` arg (where path is an existing directory) → open the path.
  - With `<name>` arg (not an existing directory path) → fail with `ExitGeneralError` and a clear message: `name resolution requires a git repository — pass a path, or run from a git repo`. Name-based resolution is gated to git context because it walks the worktree list.

Concrete sketch of the new top-of-`RunE` flow (illustrative, not final):

```go
// Determine if we have git context. Don't fail if we don't.
inRepo := wt.ValidateGitRepo() == nil
var ctx *wt.RepoContext
if inRepo {
    ctx, err = wt.GetRepoContext()
    if err != nil { /* existing error path */ }
}

var wtPath, wtName, repoName string

if target != "" {
    // Path-first: if the arg looks like an existing dir, use it.
    if info, err := os.Stat(target); err == nil && info.IsDir() {
        wtPath = target
        wtName = filepath.Base(wtPath)
        if inRepo { repoName = ctx.RepoName }
    } else if inRepo {
        // Try name resolution within the repo.
        path, err := resolveWorktreeByName(target, ctx)
        // ... existing error path
        wtPath = path
        wtName = target
        repoName = ctx.RepoName
    } else {
        wt.ExitWithError(wt.ExitGeneralError,
            fmt.Sprintf("Cannot open '%s'", target),
            "name resolution requires a git repository — pass a path, or run from a git repo",
            "Example: wt open /absolute/path/to/dir")
    }
} else if inRepo && wt.IsWorktree() {
    // existing: open current worktree
} else if inRepo {
    // existing: selection menu
    return selectAndOpen(ctx)
} else {
    // No args, no git — open cwd.
    wtPath, _ = os.Getwd()
    wtName = filepath.Base(wtPath)
}
```

The `repoName` variable is passed to `OpenInApp`. When there's no git context, `repoName` stays empty.

### 2. Tab-naming fallback in `OpenInApp`

**File**: `src/internal/worktree/apps.go`

`OpenInApp(appCmd, path, repoName, wtName string)` currently composes byobu/tmux tab names as `repoName + "-" + wtName` (lines 239, 254, 264). When `repoName` is empty (non-git invocation), the resulting tab name would be `-<wtName>`, which is ugly and could trip tmux's name-validation rules.

Add a small helper (or inline guard) so the tab-naming logic produces a sensible name in all three cases:

```go
func tabName(repoName, wtName string) string {
    if repoName == "" {
        return wtName
    }
    return repoName + "-" + wtName
}
```

Replace the three call sites in the `byobu_tab`, `tmux_window`, and `tmux_session` cases. No other behavior changes in `OpenInApp`.

### 3. Document the launcher contract

**File**: `docs/specs/launcher-contract.md` (new)

Spec the env-var contract that `hop` (and any future caller) relies on. Sections:

- **Purpose**: `wt open` is the canonical directory launcher; external callers MAY delegate to it via subprocess invocation rather than reimplementing the menu / app detection.
- **Invocation**: `wt open [<path>|<name>] [--app <app>]`. Path arg accepts any existing directory regardless of git status. Name arg requires git context. No args defaults to current worktree (in worktree), worktree menu (in main repo), or cwd (non-git).
- **Environment variables (consumer-facing)**:
  - `WT_CD_FILE` — path to a writable file. When the user selects "Open here" from the menu (or when `--app open_here` is passed), `wt` writes the resolved directory path to this file (mode 0600) instead of printing a `cd --` shell line. Consumers (e.g., `hop`'s shell shim) read this file after a successful `wt open` invocation and `cd` the parent shell.
  - `WT_WRAPPER` — when set to `1`, `wt` suppresses the "shell wrapper not loaded" hint that would otherwise be printed to stderr on `open_here`. Consumers that handle the `cd` themselves SHOULD set `WT_WRAPPER=1` to avoid confusing users with a hint that doesn't apply.
- **Exit-code contract**: documents what each exit code means in the launcher context. `ExitGeneralError` (1) for arg-resolution failures, `ExitGitError` (2) ONLY when git operations are required (i.e., name resolution); never for path-only invocations. `ExitTmuxWindowError` / `ExitByobuTabError` for tab-creation failures. Non-zero exit means the consumer MUST NOT trust the contents of `WT_CD_FILE`.
- **Stability guarantees**: env-var names, exit-code semantics, and "Open here" file-write behavior are stable. `wt`-internal additions (new app types, new menu items, internal flags) do NOT count as breaking changes. Changes to the documented contract require a constitution amendment.
- **Non-goals**: not a general-purpose `xdg-open` replacement; only opens directories, not URLs or arbitrary files.

### 4. Integration test for the contract

**File**: `src/cmd/wt/integration_test.go` (extend) or new `*_test.go`

Add at least one integration test that exercises the env-var contract end-to-end:

- Set up a non-git temp directory (`t.TempDir()`).
- Set `WT_CD_FILE` to a path inside the temp dir, `WT_WRAPPER=1`, `appFlag=open_here` (or use `--app open_here`).
- Invoke `wt open <temp-dir>` as a subprocess.
- Assert: exit code 0, `WT_CD_FILE` contains the temp dir path, no "shell wrapper not loaded" hint on stderr.

Existing integration tests for git-repo invocations (`integration_test.go` already builds the binary) MUST continue to pass — they validate the backwards-compat guarantee.

### 5. Update `cli-surface.md`

**File**: `docs/specs/cli-surface.md` (existing)

Update the `wt open` entry to reflect the loosened precondition. Cross-reference the new `launcher-contract.md` spec.

## Affected Memory

- (none) — no memory files exist yet (`docs/memory/index.md` is empty). Spec changes are documented in `docs/specs/` (see What Changes #3 and #5).

## Impact

- **Source code**:
  - `src/cmd/wt/open.go` — `RunE` flow rework (precondition → soft detection)
  - `src/internal/worktree/apps.go` — tab-name helper for byobu/tmux/tmux-session cases
- **Specs**:
  - `docs/specs/launcher-contract.md` (new)
  - `docs/specs/cli-surface.md` (update `wt open` entry)
- **Tests**:
  - `src/cmd/wt/open_test.go` — unit tests for new non-git paths
  - `src/cmd/wt/integration_test.go` — integration test for env-var contract
  - `src/internal/worktree/apps_test.go` — unit tests for `tabName` helper
- **External**: `hop` is unchanged. The existing `WT_CD_FILE`/`WT_WRAPPER` contract continues to work; this change extends the precondition surface but does not alter the contract's semantics.
- **Dependencies**: none added. No changes to `go.mod`.
- **Backwards compatibility**: every existing `wt open` invocation that worked before MUST continue to work identically. This is enforced by keeping all existing branches of the resolution flow intact and adding the non-git case as a new branch only when prior branches don't apply.

## Open Questions

- (none) — all key decisions resolved during `/fab-discuss`. Edge cases to consider during spec generation are tracked as Tentative assumptions below rather than open questions.

## Clarifications

### Session 2026-05-08 (tentative resolution)

| # | Action | Detail |
|---|--------|--------|
| 13 | Confirmed | `os.Stat` + `IsDir()` pattern (matches existing fallback code) |
| 14 | Confirmed | Error suggests passing a path only; does not suggest cd'ing to a git repo |
| 15 | Confirmed | `tabName` helper lives in `apps.go` next to `OpenInApp` |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Integrate within `wt open`; do not add a new `wt launch` verb. | Discussed — user explicit choice; rationale documented in Origin. | S:95 R:80 A:90 D:95 |
| 2 | Certain | `wt open` becomes the canonical directory launcher; `hop open` continues delegating via subprocess. | Discussed — user agreed; preserves `wt`'s ownership of menu/defaults state. | S:95 R:80 A:90 D:95 |
| 3 | Certain | Change type is `refactor` (not `feat`) — loosens precondition + formalizes implicit contract; no new verbs. | Per scope description; matches keyword-based change-type inference rule (#2 "refactor"). | S:95 R:90 A:95 D:95 |
| 4 | Certain | Non-git invocation with `<name>` arg fails with `ExitGeneralError` and a clear message. | Constitution III (typed exit codes) requires a documented exit code; `ExitGeneralError` is the existing fit (no git op was attempted, so `ExitGitError` would be misleading). | S:90 R:75 A:95 D:90 |
| 5 | Certain | `wt open` with no args in a non-git directory opens cwd. | Per scope description; matches the principle of least surprise (parallel to "open current worktree" in worktree context). | S:95 R:80 A:95 D:95 |
| 6 | Certain | Backwards compatibility: every prior `wt open` invocation behaves identically. | Per scope description; enforced by leaving existing branches intact and adding non-git case as a new fall-through branch. | S:100 R:60 A:95 D:100 |
| 7 | Certain | Out of scope: extracting apps subsystem to shared package, renaming `wt open`, non-directory targets, app menu/detection changes, `hop` changes. | Per scope description (item 6). | S:100 R:90 A:95 D:100 |
| 8 | Confident | Subprocess delegation over shared library; defer Option 3 extraction. | Discussed — preserves `wt`'s state ownership; library extraction left as future option. Reversible if pressure mounts. | S:80 R:70 A:85 D:80 |
| 9 | Confident | Directories only — no URLs, no arbitrary files. | Discussed — explicit guardrail to prevent `wt open` from reinventing `xdg-open`. | S:85 R:70 A:90 D:85 |
| 10 | Confident | Tab-naming fallback uses `filepath.Base(path)` when `repoName` is empty. | Per scope description (item 4); only place worktree identity leaks into app layer. Simplest sensible default. | S:85 R:80 A:90 D:85 |
| 11 | Confident | Document the contract in a new `docs/specs/launcher-contract.md` file. | Per scope description (item 5); aligns with existing `docs/specs/` convention (cli-surface.md, init-protocol.md, etc.). | S:90 R:90 A:95 D:90 |
| 12 | Confident | Add at least one integration test exercising `WT_CD_FILE`/`WT_WRAPPER` end-to-end. | Per scope description (item 5); Constitution IV (test what the user sees) supports integration over unit-only coverage for this contract. | S:90 R:85 A:90 D:85 |
| 13 | Certain | Path arg detection uses `os.Stat` + `IsDir()` (existing pattern in current code). | Clarified — user confirmed. Reuses existing pattern from worktree-name fallback. | S:95 R:80 A:80 D:70 |
| 14 | Certain | When invoked from a non-git cwd with `<name>` arg, error suggests passing a path; does not suggest "cd to a git repo first." | Clarified — user confirmed. Less prescriptive is friendlier. | S:95 R:90 A:75 D:65 |
| 15 | Certain | The `tabName` helper lives in `apps.go` (next to `OpenInApp`) rather than in a new file. | Clarified — user confirmed. Co-located with the only caller per existing pattern. | S:95 R:95 A:85 D:80 |

15 assumptions (10 certain, 5 confident, 0 tentative, 0 unresolved).
