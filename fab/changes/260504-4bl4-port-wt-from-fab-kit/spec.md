# Spec: Port wt from fab-kit

**Change**: 260504-4bl4-port-wt-from-fab-kit
**Created**: 2026-05-04
**Affected memory**: none (no memory hydration for this change — see Non-Goals)

## Non-Goals

- Removing wt from fab-kit — explicit follow-up change in the fab-kit repo, out of scope here.
- Changing wt's CLI surface, flags, exit codes, or runtime behavior — pure port.
- Renaming the binary (stays `wt`).
- Configuring the `HOMEBREW_TAP_TOKEN` GitHub repo secret — operator setup task documented but not a code change.
- Hydrating `docs/memory/` for this change — the port creates source code but no spec-level behavioral changes; memory is hydrated when wt-specific behavior is added/changed in subsequent changes.
- Migration documentation for end users.
- Bumping the kit migration version.

## Source: Code Port

### Requirement: Module Path Rewrite
The Go module SHALL be declared as `github.com/sahil87/wt`. All Go source files SHALL import the worktree internal package as `github.com/sahil87/wt/internal/worktree` (not `github.com/sahil87/fab-kit/src/go/wt/internal/worktree`).

#### Scenario: go build at repo root succeeds
- **GIVEN** the repo has been initialized with `src/go.mod` declaring `module github.com/sahil87/wt`
- **WHEN** the developer runs `cd src && go build ./cmd/wt`
- **THEN** the build SHALL succeed and produce a `wt` binary

#### Scenario: No fab-kit import paths remain
- **GIVEN** all Go source files have been ported
- **WHEN** the developer runs `grep -r 'sahil87/fab-kit' src/` from repo root
- **THEN** the command SHALL produce zero matches

### Requirement: Source File Layout
Source files SHALL be placed under `src/` per the hop convention:

| Source role | Path |
|---|---|
| Module manifest | `src/go.mod`, `src/go.sum` |
| CLI entry + subcommands | `src/cmd/wt/` |
| Worktree internal package | `src/internal/worktree/` |

The `src/cmd/wt/` directory SHALL contain `main.go` plus one file per subcommand (`create.go`, `list.go`, `open.go`, `delete.go`, `init.go`, `shell_setup.go`) plus their `*_test.go` siblings (`testutil_test.go`, `integration_test.go`, `edge_test.go`, and per-command unit tests). The `src/internal/worktree/` directory SHALL contain the full `internal/worktree` package from fab-kit (17 files: `git.go`, `context.go`, `platform.go`, `worktree.go`, `apps.go`, `stash.go`, `crud.go`, `errors.go`, `names.go`, `menu.go`, `rollback.go`, plus the matching `*_test.go` files).

#### Scenario: Layout matches convention
- **GIVEN** the port is complete
- **WHEN** the developer inspects `src/`
- **THEN** the structure SHALL be `src/{cmd/wt,internal/worktree,go.mod,go.sum}`
- **AND** there SHALL be no `src/go/wt/` nesting (that was fab-kit's monorepo layout, not hop's)

### Requirement: Tests Run Unchanged
All existing `*_test.go` files SHALL be ported verbatim except for module-path updates. The full test suite SHALL pass unchanged.

#### Scenario: All tests pass after port
- **GIVEN** the source port is complete and module paths have been rewritten
- **WHEN** the developer runs `cd src && go test ./...`
- **THEN** all tests SHALL pass with no failures, skips, or compilation errors

### Requirement: Constitution Adherence
The port SHALL satisfy the constitution as already ratified (`fab/project/constitution.md` v1.0.0):

- Single-binary CLI (Principle I)
- Cobra command surface in `src/cmd/` (Principle II)
- Typed exit codes via `internal/worktree/errors.go` (Principle III)
- Tests alongside code, integration tests under `cmd/integration_test.go` (Principle IV)
- `cmd/` thin, business logic in `internal/worktree/` (Principle V)
- `--non-interactive` flag where applicable (Principle VI)
- Shell integration via `wt shell-setup` eval (Principle VII)
- Module path `github.com/sahil87/wt` (Module Path Stability)

#### Scenario: Constitution principles satisfied
- **GIVEN** the port is complete
- **WHEN** a reviewer audits the source against `fab/project/constitution.md`
- **THEN** every principle and constraint SHALL be observably satisfied by the ported code

## Source: Init Protocol Preservation

### Requirement: Default init script value
`internal/worktree/context.go` SHALL define `InitScriptPath()` returning `"fab sync"` when the `WORKTREE_INIT_SCRIPT` environment variable is unset, and the env-var value otherwise.

#### Scenario: env var unset → default
- **GIVEN** `WORKTREE_INIT_SCRIPT` is not set in the environment
- **WHEN** `InitScriptPath()` is called
- **THEN** it SHALL return `"fab sync"`

#### Scenario: env var set → override
- **GIVEN** `WORKTREE_INIT_SCRIPT=custom/init.sh`
- **WHEN** `InitScriptPath()` is called
- **THEN** it SHALL return `"custom/init.sh"`

### Requirement: Init script invocation contract
`cmd/init.go` SHALL detect whether the resolved init-script value is a command invocation (contains spaces, e.g. `"fab sync"`) or a file path. For commands, the binary SHALL fall back gracefully when the command is not on PATH (printing a guidance message that mentions both `fab-kit` and `WORKTREE_INIT_SCRIPT`). For file paths, the binary SHALL resolve the path relative to the git common dir and skip with guidance if missing.

#### Scenario: command not on PATH
- **GIVEN** `WORKTREE_INIT_SCRIPT` is unset (so the value is `"fab sync"`) AND `fab` is not on PATH
- **WHEN** the user runs `wt init`
- **THEN** the binary SHALL print `Warning: "fab" not found on PATH, skipping init`
- **AND** SHALL print `Install fab-kit or set WORKTREE_INIT_SCRIPT to a custom script.`
- **AND** SHALL exit 0

#### Scenario: file path missing
- **GIVEN** `WORKTREE_INIT_SCRIPT=scripts/init.sh` AND that file does not exist
- **WHEN** the user runs `wt init`
- **THEN** the binary SHALL print guidance pointing at the expected location
- **AND** SHALL exit 0

## Layout: hop Convention

### Requirement: Repo root layout matches hop
The repo root SHALL contain the following entries (created by this change unless already present):

| Entry | Status |
|---|---|
| `LICENSE` | already present |
| `README.md` | replaced (was placeholder) |
| `docs/` | already present (empty memory + specs indices) |
| `fab/` | already present |
| `src/` | created |
| `scripts/` | created |
| `justfile` | created |
| `.github/workflows/release.yml` | created |
| `.github/formula-template.rb` | created |

#### Scenario: Layout audit
- **GIVEN** the change is complete
- **WHEN** the developer runs `ls` at repo root
- **THEN** the listed entries SHALL all be present
- **AND** there SHALL be no `bin/`, `dist/`, or `internal/` at repo root (those live under `src/` in hop convention or are gitignored)

## Build: Scripts and justfile

### Requirement: Build script
`scripts/build.sh` SHALL build the `wt` binary from `src/cmd/wt`, stamp `main.version` from `git describe --tags --always`, and write the binary to `bin/wt` at repo root. The script SHALL be a verbatim copy of hop's `scripts/build.sh` with `hop` → `wt` substitutions in (a) the `go build` output path, (b) the `./cmd/hop` package path, and (c) the final `built:` echo.

#### Scenario: build produces stamped binary
- **GIVEN** the repo is at a tagged commit (e.g. `v0.1.0`)
- **WHEN** the developer runs `./scripts/build.sh`
- **THEN** the script SHALL produce `bin/wt`
- **AND** `bin/wt --version` SHALL contain the tag string

### Requirement: Install script
`scripts/install.sh` SHALL invoke `scripts/build.sh` and copy `bin/wt` to `${HOME}/.local/bin/wt`. The script SHALL be a verbatim copy of hop's `scripts/install.sh` with `hop` → `wt` substitution in the `DEST` path.

#### Scenario: install puts binary on PATH
- **GIVEN** `~/.local/bin` is on `$PATH`
- **WHEN** the developer runs `./scripts/install.sh`
- **THEN** `which wt` SHALL print `${HOME}/.local/bin/wt`

### Requirement: Release script
`scripts/release.sh` SHALL be a verbatim copy of hop's `scripts/release.sh`. The script is tag-driven: it computes the next semver tag from `git describe`, creates the tag, and pushes it. The script SHALL NOT modify any tracked files (no VERSION file, no commit). No `wt` ↔ `hop` substitution is required because the script does not reference either binary by name.

#### Scenario: release tag creation
- **GIVEN** the working tree is clean and the most recent tag is `v0.1.0`
- **WHEN** the developer runs `./scripts/release.sh patch`
- **THEN** the script SHALL create local tag `v0.1.1` AND push it to origin
- **AND** SHALL not produce any commits

### Requirement: justfile
The `justfile` SHALL provide `default` (list recipes), `build` (calls `scripts/build.sh`), `local-install` (calls `scripts/install.sh`), `test` (`cd src && go test ./...`), and `release bump="patch"` (calls `scripts/release.sh`). It SHALL be derived from hop's `justfile` with the binary name updated where it appears in user-facing recipe descriptions.

#### Scenario: just test runs the suite
- **GIVEN** the source port is complete
- **WHEN** the developer runs `just test`
- **THEN** the recipe SHALL execute `cd src && go test ./...`
- **AND** all tests SHALL pass

## Release: GitHub Workflow

### Requirement: release workflow
`.github/workflows/release.yml` SHALL trigger on tag push matching `v*`, cross-compile the `wt` binary for `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`, package each as `wt-{os}-{arch}.tar.gz` containing the bare `wt` binary, create a GitHub Release with auto-generated notes, and update the Homebrew tap. The workflow SHALL be derived from hop's `release.yml` with these exact substitutions:

| Original (hop) | Replaced (wt) |
|---|---|
| `hop-${os}-${arch}` | `wt-${os}-${arch}` |
| `./cmd/hop` | `./cmd/wt` |
| `-o "../dist/${output}/hop"` | `-o "../dist/${output}/wt"` |
| `-C "../dist/${output}" hop` | `-C "../dist/${output}" wt` |
| `Formula/hop.rb` | `Formula/wt.rb` |
| `dist/hop-darwin-arm64.tar.gz` (and 3 siblings) | `dist/wt-darwin-arm64.tar.gz` (and 3 siblings) |
| `hop v${version}` (commit message) | `wt v${version}` |

All other workflow content (action SHA pins, step ordering, base-tag logic, tap clone URL, sed substitution mechanics) SHALL be preserved verbatim.

#### Scenario: tag push triggers release
- **GIVEN** the repo has been pushed to GitHub with `release.yml` in place AND a `HOMEBREW_TAP_TOKEN` secret with push access to `github.com/sahil87/homebrew-tap` is configured
- **WHEN** a tag `v0.1.0` is pushed
- **THEN** the workflow SHALL build 4 platform binaries
- **AND** SHALL create a GitHub Release with all 4 tarballs attached
- **AND** SHALL render `Formula/wt.rb` in the tap repo with the release version + tarball SHA256s
- **AND** SHALL push that formula update to the tap

### Requirement: Formula template
`.github/formula-template.rb` SHALL be a Homebrew formula declaring `class Wt < Formula` with `desc "Git worktree management CLI"`, `homepage "https://github.com/sahil87/wt"`, `license "MIT"`, and platform-specific `url`/`sha256` blocks for the 4 release targets. The placeholder strings (`VERSION_PLACEHOLDER`, `SHA_DARWIN_ARM64`, `SHA_DARWIN_AMD64`, `SHA_LINUX_ARM64`, `SHA_LINUX_AMD64`) SHALL be the literal markers that the workflow's `sed` step substitutes. The template SHALL be derived from hop's `.github/formula-template.rb` with these substitutions:

| Original (hop) | Replaced (wt) |
|---|---|
| `class Hop < Formula` | `class Wt < Formula` |
| `desc "Locate, open, list, and operate on repos from hop.yaml"` | `desc "Git worktree management CLI"` |
| `homepage "https://github.com/sahil87/hop"` | `homepage "https://github.com/sahil87/wt"` |
| `hop-darwin-arm64.tar.gz` (and 3 siblings) | `wt-darwin-arm64.tar.gz` (and 3 siblings) |
| `bin.install "hop"` | `bin.install "wt"` |
| `shell_output("#{bin}/hop --version")` | `shell_output("#{bin}/wt --version")` |
| `releases/download/v#{version}/hop-...` | `releases/download/v#{version}/wt-...` |

#### Scenario: rendered formula installs
- **GIVEN** the workflow has rendered `Formula/wt.rb` in the tap repo with concrete version + SHA256 values
- **WHEN** a user runs `brew install sahil87/tap/wt`
- **THEN** brew SHALL download the appropriate platform tarball
- **AND** SHALL install the `wt` binary to brew's prefix
- **AND** `wt --version` SHALL succeed

## Docs: Specs Set

### Requirement: 5-file split spec set
`docs/specs/` SHALL contain exactly these wt-product specs at the end of this change:

| File | Purpose |
|---|---|
| `index.md` | Updated landing page with one row per spec |
| `cli-surface.md` | Per-subcommand reference (`create`, `list`, `open`, `delete`, `init`, `shell-setup`) — flags, positional args, exit codes |
| `worktree-layout.md` | Filesystem layout (`<repo>.worktrees/`), naming convention (random adjective-noun, `--worktree-name`), branch ↔ worktree relationship |
| `init-protocol.md` | Init script lookup contract: `WORKTREE_INIT_SCRIPT`, `"fab sync"` default, command-vs-path detection, working-dir resolution, error/skip behavior |
| `build-and-release.md` | Build (justfile/scripts), tag-driven release flow, cross-compile matrix, Homebrew tap update |

#### Scenario: All specs present and indexed
- **GIVEN** the change is complete
- **WHEN** a reader opens `docs/specs/index.md`
- **THEN** the index SHALL list all 4 detail specs
- **AND** each entry SHALL link to a file that exists and is non-empty

### Requirement: Specs are wt-specific
The specs SHALL describe the wt product alone. Specs SHALL NOT include broader fab-kit content (architecture overview, change-types, SRAD, operator, assembly-line, glossary). Spec text MAY reference `fab sync` only in the context of the init-script default value.

#### Scenario: No fab-flavored content
- **GIVEN** the spec set has been written
- **WHEN** a reader greps the specs for fab-kit-flavored terms (`SRAD`, `operator`, `assembly line`, `change folder`, `change type`, `intake`)
- **THEN** matches SHALL only appear in places where they describe wt's relationship to fab tools (e.g., the init-protocol spec mentioning that `fab sync` is the default invocation), not as wt-internal concepts

## Docs: README

### Requirement: README content
`README.md` SHALL describe wt as a standalone Git worktree management CLI: a one-paragraph elevator pitch, an Install section (Homebrew tap + manual install via `just local-install`), a Usage section listing the subcommands with one-line descriptions, a Specs section linking to `docs/specs/index.md`, and a Footer note pointing back to the fab-kit toolkit hub for users who arrive there from fab-kit context. The README SHALL replace the existing placeholder.

#### Scenario: README replaces placeholder
- **GIVEN** the change is complete
- **WHEN** a reader opens `README.md`
- **THEN** the README SHALL describe wt's purpose and usage
- **AND** SHALL NOT be the original placeholder ("README with link to central toolkit hub")
- **AND** SHALL include a Homebrew install one-liner referencing `sahil87/tap/wt`

## Docs: Pre-Release Setup

### Requirement: Setup checklist for first release
The build/release spec (`docs/specs/build-and-release.md`) SHALL include a "Pre-Release Setup" section documenting the operator-side prerequisites that must be satisfied before the first tag push:

1. The repo `github.com/sahil87/wt` SHALL exist and have a `HOMEBREW_TAP_TOKEN` repo secret with push access to `github.com/sahil87/homebrew-tap`.
2. The `homebrew-tap` repo SHALL contain a `Formula/wt.rb` file (placeholder is acceptable; the workflow overwrites it on first release).

This section is documentation, not code. The change does NOT configure the secret or create the formula file in the tap repo.

#### Scenario: Setup section exists
- **GIVEN** the change is complete
- **WHEN** a reader opens `docs/specs/build-and-release.md`
- **THEN** the doc SHALL contain a section enumerating the two setup prerequisites above

## Design Decisions

1. **Layout flatten (`src/cmd/wt/` + `src/internal/worktree/` with single `src/go.mod`) instead of fab-kit's `src/go/wt/{cmd,internal}/`**
   - *Why*: The hop repo, which the user explicitly cited as the convention source, uses this exact layout. A single-binary repo doesn't need the `src/go/{binary}/` nesting that fab-kit uses to host multiple binaries side by side.
   - *Rejected*: Mirroring fab-kit's `src/go/wt/...` — would carry over a monorepo-flavored layout into a single-product repo without justification.

2. **Tag-driven release (hop's `release.sh`) instead of VERSION-file-driven (fab-kit's `release.sh`)**
   - *Why*: Hop's flow is simpler — the git tag is the single source of truth, no commit is created, no file is mutated. Aligns with the user's instruction to copy from hop.
   - *Rejected*: Copying fab-kit's `release.sh` — that script bumps `src/kit/VERSION` which is a fab-kit-specific file with no analog here.

3. **Active Homebrew tap update (not commented or removed)**
   - *Why*: The tap repo (`~/code/sahil87/homebrew-tap/`) already contains a placeholder `Formula/wt.rb`. The workflow's `sed`+`git push` mechanism will overwrite it on first release. No reason to gate this behind a deferral.
   - *Rejected*: Commenting out the tap step pending operator setup — adds friction for no benefit since the placeholder is in place; the only remaining setup is the `HOMEBREW_TAP_TOKEN` secret which is a one-time GitHub UI action documented in the spec.

4. **Specs split into 5 files (index + cli-surface + worktree-layout + init-protocol + build-and-release) instead of one file**
   - *Why*: Mirrors hop's functional split style (`cli-surface.md`, `config-resolution.md`, `build-and-release.md`). Each file has a single concern, which makes future updates targeted and reduces merge friction.
   - *Rejected*: A single `wt.md` — would conflate the CLI reference, layout convention, init protocol, and release flow into one file that needs editing for any small change.

5. **Default init script value `"fab sync"` is preserved verbatim**
   - *Why*: Existing wt users (installed via fab-kit today) get unchanged UX. The env-var override (`WORKTREE_INIT_SCRIPT`) means standalone wt users without fab-kit get a clean fallback path. The `init.go` warning message that mentions both fab-kit and the env var stays accurate in both worlds.
   - *Rejected*: Changing to `:` (no-op), empty string, or a heuristic — would silently break existing users mid-transition for no concrete benefit.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Module path is `github.com/sahil87/wt` | Confirmed in intake; constitution v1.0.0 already encodes this in the Module Path Stability constraint | S:100 R:60 A:95 D:100 |
| 2 | Certain | Default init script remains `"fab sync"` (env-var override unchanged) | Confirmed in intake; preserves UX continuity for fab-kit users | S:100 R:80 A:95 D:100 |
| 3 | Certain | Spec set is split into 5 files (index, cli-surface, worktree-layout, init-protocol, build-and-release); fab-kit-flavored specs are excluded | Confirmed in intake; mirrors hop's functional split | S:95 R:80 A:90 D:90 |
| 4 | Certain | Source layout: `src/cmd/wt/`, `src/internal/worktree/`, `src/go.mod` (single Go module at `src/`) | Confirmed in intake; constitution Principle V already states `src/internal/worktree/` and `src/cmd/`. Hop pattern verified | S:100 R:70 A:95 D:95 |
| 5 | Certain | Release script comes from hop verbatim (tag-driven, no VERSION file, no per-binary substitutions needed) | Confirmed in intake; verified hop's release.sh has no `hop`-name references | S:100 R:80 A:95 D:95 |
| 6 | Certain | Binary name remains `wt`; no rename | Confirmed in intake; required for transition compat with existing fab-kit users | S:100 R:50 A:95 D:95 |
| 7 | Certain | All `*_test.go` files port verbatim; suite runs unchanged after module-path rewrite | Confirmed in intake; coupling analysis verified imports are local-only | S:95 R:80 A:95 D:95 |
| 8 | Certain | wt is NOT removed from fab-kit in this change | Confirmed in intake; explicit user deferral | S:100 R:75 A:95 D:100 |
| 9 | Certain | Homebrew tap update step is active in `release.yml`; `Formula/wt.rb` placeholder confirmed in `~/code/sahil87/homebrew-tap/` | Confirmed in intake; tap-repo placeholder verified to exist | S:95 R:70 A:90 D:95 |
| 10 | Certain | README is rewritten to describe wt standalone (placeholder replaced) | Confirmed in intake; current README is generic | S:95 R:90 A:90 D:95 |
| 11 | Certain | `.github/formula-template.rb` is added to this repo (wt-flavored copy of hop's), with the same `VERSION_PLACEHOLDER` / `SHA_*` markers so the workflow's `sed` step works without modification | Confirmed in intake; cleanest integration with hop's release.yml mechanism | S:95 R:80 A:90 D:95 |
| 12 | Certain | `HOMEBREW_TAP_TOKEN` secret configuration is documented in the build-and-release spec but NOT performed by this change | Confirmed in intake; secret is a one-time operator action via GitHub UI, not a code change | S:95 R:90 A:90 D:95 |
| 13 | Certain | No `docs/memory/` files are created or modified in this change; memory hydration is reserved for changes that introduce or modify wt-specific behavior | Code port creates source code but no behavioral spec changes; existing constitution already covers the principles, so no per-change memory entry is warranted | S:90 R:90 A:85 D:90 |
| 14 | Certain | `cd src && go test ./...` is the canonical test command (matches hop and the new justfile recipe) | Hop's pattern; constitution Principle IV requires tests pass | S:95 R:90 A:95 D:95 |
| 15 | Certain | Repo root will gain `bin/` and `dist/` directories at build/release time; both SHALL be gitignored | Hop's `.gitignore` does this; required so `just build` doesn't dirty the working tree | S:90 R:95 A:90 D:90 |

15 assumptions (15 certain, 0 confident, 0 tentative, 0 unresolved).
