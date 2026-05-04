# Intake: Port wt from fab-kit

**Change**: 260504-4bl4-port-wt-from-fab-kit
**Created**: 2026-05-04
**Status**: Draft

## Origin

This change was initiated as the foundational change for the new standalone `wt` repository. The user intent: extract the existing `wt` Go command (currently part of the fab-kit monorepo at `~/code/sahil87/fab-kit/src/go/wt/`) into this dedicated repo so that fab-kit can later drop it.

User's raw input:

> Copy the wt command's implementation from ~/code/sahil87/fab-kit/ from there to this new repo. Copy its specs (from docs/specs) also. For the folder structure, use the conventions from ~/code/sahil87/hop/. The release script can also be copied from hop. We will later be removing wt from fab-kit. First lets discuss any hard coupling between fab-kit and wt or if its an easy move.

Mode: conversational. Before creating the intake, we performed a coupling analysis on the fab-kit codebase to confirm wt is movable without breaking changes. The analysis found:

- wt's only Go imports are `internal/worktree` (its own package) plus `cobra` — no fab/idea/fab-kit cross-imports.
- The only soft coupling is `internal/worktree/context.go:175-181` (`InitScriptPath`) which defaults to `"fab sync"` when `WORKTREE_INIT_SCRIPT` is unset. This is a runtime PATH lookup, not a build-time dependency, and is already designed to fail gracefully (`init.go:74` prints "Install fab-kit or set WORKTREE_INIT_SCRIPT").
- fab-kit's `justfile`, `scripts/just/package-brew.sh`, and `.github/workflows/release.yml` bundle wt into the same brew archive as fab/fab-kit/idea — that build glue stays in fab-kit and is replaced here by hop-style equivalents.
- `docs/specs/packages.md` in fab-kit has the only dedicated wt spec section. Other specs mention wt only in passing (architecture, naming, glossary) and are fab-flavored — they stay in fab-kit.

Three decisions were made conversationally before this intake was generated:

1. **Default init script**: keep `"fab sync"` as the default value of `InitScriptPath`. Preserves UX continuity for users with fab-kit installed; the env-var override remains for standalone use.
2. **Spec scope**: lift only wt-specific content. The dedicated `wt` section of `packages.md` plus the worktree directory naming convention from `naming.md` move here; broader fab architecture content stays in fab-kit.
3. **Module path**: rename to `github.com/sahil87/wt` (matches hop's `github.com/sahil87/hop`).

## Why

**Problem**: `wt` is a self-contained worktree management CLI that has no Go-level dependencies on fab-kit's other binaries. Keeping it inside the fab-kit monorepo couples its release cadence, Homebrew distribution, and build matrix to a workflow product it isn't part of. Users who want only the worktree tool today have to install the entire fab-kit bundle.

**Consequence if we don't extract**: wt cannot be released, versioned, or distributed independently. fab-kit's release workflow continues to bundle wt into archives even when no wt code changed. The implicit "wt is a fab-kit subproject" framing makes it harder to position wt as a standalone tool with a separate value proposition.

**Why this approach (full extract, not symlink/submodule)**: wt has no shared internal libraries with fab-kit, so extraction is a clean import-path rewrite. Keeping the fab-kit copy in place during the transition (the user said "We will later be removing wt from fab-kit") avoids a hard cutover — both copies coexist briefly while users migrate. Submodules / monorepo-thin-clones add tooling complexity for no benefit when the code is already independent.

## What Changes

### Source code port

Copy these directories from fab-kit, preserving file structure and rewriting only the module path:

| From (fab-kit) | To (new repo) |
|---|---|
| `src/go/wt/cmd/` | `src/cmd/wt/` |
| `src/go/wt/internal/worktree/` | `src/internal/worktree/` |
| `src/go/wt/go.mod` | `src/go.mod` |
| `src/go/wt/go.sum` | `src/go.sum` |

**Layout choice (matches hop)**: hop uses `src/cmd/hop/main.go` + `src/internal/{repos,proc,...}/` with a single `src/go.mod`. We adopt the same flat structure — no `src/go/wt/` nesting, since this repo only contains `wt`.

**Module path rewrite**: The 7 files that import `github.com/sahil87/fab-kit/src/go/wt/internal/worktree` (`cmd/main.go`, `cmd/init.go`, `cmd/list.go`, `cmd/create.go`, `cmd/delete.go`, `cmd/open.go`, plus the `go.mod` declaration) update to `github.com/sahil87/wt/internal/worktree`. The `go.mod` `module` line becomes `github.com/sahil87/wt`.

**Files to copy verbatim** (no edit needed beyond module path):

```
cmd/main.go            cmd/shell_setup.go      cmd/testutil_test.go
cmd/create.go          cmd/delete.go           cmd/list.go
cmd/open.go            cmd/init.go             cmd/create_test.go
cmd/delete_test.go     cmd/list_test.go        cmd/open_test.go
cmd/init_test.go       cmd/shell_setup_test.go cmd/integration_test.go
cmd/edge_test.go

internal/worktree/git.go        internal/worktree/git_test.go
internal/worktree/context.go    internal/worktree/context_test.go
internal/worktree/platform.go   internal/worktree/worktree.go
internal/worktree/apps.go       internal/worktree/apps_test.go
internal/worktree/stash.go      internal/worktree/crud.go
internal/worktree/errors.go     internal/worktree/errors_test.go
internal/worktree/names.go      internal/worktree/names_test.go
internal/worktree/menu.go       internal/worktree/rollback.go
internal/worktree/rollback_test.go
```

**Default init script — keep `"fab sync"` (decision 1)**:

`internal/worktree/context.go:175-181`:

```go
// InitScriptPath returns the path to the init script, respecting WORKTREE_INIT_SCRIPT env var.
func InitScriptPath() string {
    if v := os.Getenv("WORKTREE_INIT_SCRIPT"); v != "" {
        return v
    }
    return "fab sync"
}
```

This stays unchanged. The accompanying message in `cmd/init.go:74` (`"Install fab-kit or set WORKTREE_INIT_SCRIPT to a custom script."`) also stays — it correctly tells the user that fab-kit is one (now external) source of `fab sync`.

### Docs / specs port

Copy only wt-specific content into `docs/specs/`:

Spec set is split functionally (matches hop's `docs/specs/` style — `architecture.md`, `cli-surface.md`, `config-resolution.md`, `build-and-release.md`, `index.md`):

1. **`docs/specs/index.md`** (new) — landing page with one-line summary + link per spec.
2. **`docs/specs/cli-surface.md`** (new) — every subcommand (`wt create / list / open / delete / init`), flags, positional args, exit codes. Sourced from existing `cmd/*.go` files and the fab-kit `packages.md` `## wt` section.
3. **`docs/specs/worktree-layout.md`** (new) — where worktrees live (`<repo>.worktrees/`), the random adjective-noun naming convention, the `--worktree-name` override, and the branch ↔ worktree relationship. Sourced from the **Worktree Directory** subsection of fab-kit's `docs/specs/naming.md` plus inferences from `internal/worktree/names.go`.
4. **`docs/specs/init-protocol.md`** (new) — the init script lookup contract: `WORKTREE_INIT_SCRIPT` env var, `"fab sync"` default, command-vs-path detection, working directory resolution (`git rev-parse --git-common-dir` for repo root, `--show-toplevel` for current worktree), error/skip behavior. Sourced from `cmd/init.go` and `internal/worktree/context.go`.
5. **`docs/specs/build-and-release.md`** (new) — mirrors hop: tag-driven release flow, cross-compile matrix (4 platforms), Homebrew tap update flow.

No `architecture.md` — wt's surface is too focused to warrant one.

Memory files (`docs/memory/`) start empty for now — to be hydrated later from existing fab-kit content if useful.

### Build / release glue (copy from hop)

Copy and adapt from `~/code/sahil87/hop/`:

| File | Action |
|---|---|
| `scripts/build.sh` | Copy, change `hop` → `wt` (binary name + build target `./cmd/wt` instead of `./cmd/hop`) |
| `scripts/install.sh` | Copy, change `hop` → `wt` (DEST path) |
| `scripts/release.sh` | Copy verbatim (tag-driven, no per-binary references) |
| `justfile` | Copy, change `hop` → `wt` in `build`, `local-install`, `release` recipes |
| `.github/workflows/release.yml` | Copy, change `hop-${os}-${arch}` → `wt-${os}-${arch}`, `./cmd/hop` → `./cmd/wt`, `Formula/hop.rb` → `Formula/wt.rb`, `hop v${version}` commit msg → `wt v${version}`. **Tap step active** — the placeholder `Formula/wt.rb` already exists in `~/code/sahil87/homebrew-tap/` and gets overwritten on first release. |
| `.github/formula-template.rb` | Copy from hop with `class Hop` → `class Wt`, `desc` updated to "Git worktree management CLI", `homepage` → `https://github.com/sahil87/wt`, all `hop` → `wt` substitutions in URL paths and `bin.install`. Markers (`VERSION_PLACEHOLDER`, `SHA_*`) stay identical so workflow `sed` works unchanged. |
| `.envrc` | Copy verbatim if present |
| `.gitignore` | Copy verbatim, then merge any wt-specific patterns from fab-kit |

The hop `release.sh` is tag-driven (no VERSION file) — this is the model we adopt. We do **not** copy fab-kit's `scripts/release.sh` because that one bumps a `src/kit/VERSION` file which is fab-kit-specific.

### Repo-level files

| File | Source | Action |
|---|---|---|
| `README.md` | Currently a placeholder pointing to the fab-kit central hub | Replace with a proper wt README — usage, install, link back to fab-kit toolkit hub kept as a footer reference |
| `LICENSE` | Already exists | Keep as-is |
| `CONTRIBUTING.md` | None yet | Skip for this change (not requested) |

### Pre-release setup (not blocking this change, but blocks first tag push)

- Configure `HOMEBREW_TAP_TOKEN` repo secret on `github.com/sahil87/wt` with push access to `github.com/sahil87/homebrew-tap`. Without this, the tap-update step of `release.yml` fails on the first tag push (the GitHub Release itself still publishes successfully).

### Out of scope (explicit non-goals)

- Removing wt from fab-kit. The user said "We will later be removing wt from fab-kit" — that's a follow-up change in the fab-kit repo, not this one.
- Migrating users / writing migration docs.
- Changing wt's CLI surface or behavior. This is a pure port — no feature changes, no refactors, no rename of binary.
- Bumping kit migration version (`8a4e0cc Bump kit migration version to 1.6.1` was a setup commit; further bumps are a separate fab concern).

## Affected Memory

No memory files yet — this is the first non-setup change in the repo. Memory will be hydrated lazily as wt-specific knowledge accumulates (e.g., decisions about the init script protocol, worktree naming rules) in subsequent changes.

## Impact

**Code areas**:
- New: `src/cmd/wt/`, `src/internal/worktree/`, `src/go.mod`, `src/go.sum`
- New: `scripts/{build,install,release}.sh`, `justfile`, `.github/workflows/release.yml`
- New: `docs/specs/{index,wt,naming}.md`
- Replaced: `README.md`

**Dependencies**: `github.com/spf13/cobra v1.8.1` + transitive `mousetrap`/`pflag`. Same as fab-kit's wt today. No new deps.

**External systems**:
- GitHub Releases (workflow runs on tag push, cross-compiles 4 platforms).
- Homebrew tap (formula update step exists in workflow but tap config is deferred).

**Test surface**: All existing wt tests come along (`*_test.go` files listed above). They reference only the local `internal/worktree` package and stdlib — module path rewrite is the only change needed. Run via `cd src && go test ./...` (matches hop's `just test` recipe).

## Open Questions

- None blocking. The three decisions above (init script default, spec scope, module path) cover the design choices that were ambiguous before discussion.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Module path becomes `github.com/sahil87/wt` | Confirmed by user; matches hop's pattern | S:95 R:60 A:90 D:95 |
| 2 | Certain | Default init script stays `"fab sync"` | Confirmed by user; preserves UX continuity for fab-kit users, env-var override remains | S:95 R:80 A:90 D:95 |
| 3 | Certain | Spec port limited to wt-specific content | Confirmed by user; other fab-kit specs are fab-flavored and stay in fab-kit | S:95 R:75 A:90 D:90 |
| 4 | Certain | Folder layout flattens to `src/cmd/wt/` + `src/internal/worktree/` with single `src/go.mod` | Clarified — user bulk-confirmed; matches hop's `src/cmd/hop/` + `src/internal/{repos,proc,...}/` exactly | S:95 R:70 A:90 D:90 |
| 5 | Certain | Release script comes from hop (tag-driven), not fab-kit (VERSION-file-driven) | Clarified — user bulk-confirmed; user said "release script can also be copied from hop" | S:95 R:80 A:95 D:95 |
| 6 | Certain | Binary name remains `wt` (not renamed) | Clarified — user bulk-confirmed; CLI compat with existing fab-kit users matters during transition window | S:95 R:50 A:90 D:90 |
| 7 | Certain | Tests are copied verbatim and run unchanged after module-path rewrite | Clarified — user bulk-confirmed; tests import only local `internal/worktree` and stdlib (verified during coupling analysis) | S:95 R:80 A:90 D:95 |
| 8 | Certain | wt is NOT removed from fab-kit in this change | Clarified — user bulk-confirmed; user said "We will later be removing wt from fab-kit" — explicit deferral | S:95 R:75 A:95 D:95 |
| 9 | Certain | Homebrew tap update is **active** in `release.yml` (placeholder `Formula/wt.rb` already exists in `~/code/sahil87/homebrew-tap/`) | Clarified — user confirmed tap repo readiness; workflow renders `.github/formula-template.rb` into `Formula/wt.rb` on each release. Pre-release setup: `HOMEBREW_TAP_TOKEN` secret needed | S:95 R:70 A:90 D:90 |
| 10 | Certain | README is rewritten to describe wt standalone | Clarified — user bulk-confirmed; current README is a placeholder pointing at the toolkit hub | S:95 R:90 A:85 D:90 |
| 11 | Certain | Specs split into 5 files: `index.md`, `cli-surface.md`, `worktree-layout.md`, `init-protocol.md`, `build-and-release.md` | Clarified — user said "feel free to break it up"; mirrors hop's functional split (`cli-surface.md`, `config-resolution.md`, `build-and-release.md`) | S:90 R:85 A:80 D:85 |
| 12 | Certain | Copy hop's `.github/formula-template.rb` (wt-flavored) into this repo; existing tap placeholder gets overwritten on first release | Clarified — user confirmed homebrew-tap repo has placeholder `Formula/wt.rb`; cleanest path is to reuse hop's template + sed-substitution mechanism unchanged | S:90 R:80 A:85 D:90 |

12 assumptions (12 certain, 0 confident, 0 tentative, 0 unresolved). Run /fab-clarify to review.
