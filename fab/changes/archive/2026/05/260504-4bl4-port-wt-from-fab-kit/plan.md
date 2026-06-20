# Plan: Port wt from fab-kit

**Change**: 260504-4bl4-port-wt-from-fab-kit
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

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


## Tasks

### Phase 1: Setup

- [x] T001 Replace `.gitignore` at repo root with hop-flavored Go gitignore. Source: `~/code/sahil87/hop/.gitignore`. Action: copy verbatim, then re-add the wt-specific tail (`.fab-*`, `/.agents`, `/.claude`, `/.cursor`, `/.opencode`, `/.codex`, `/.gemini`, `# fab` + `.fab-status.yaml`) that the current Node-template `.gitignore` already contains so we don't lose them. Result: a single `.gitignore` containing Go binary patterns + `bin/` + `dist/` + the fab/agent ignores.
- [x] T002 [P] Create `src/go.mod` declaring `module github.com/sahil87/wt`, `go 1.22`, and the `cobra v1.8.1` + transitive deps. Source: copy `~/code/sahil87/fab-kit/src/go/wt/go.mod` and rewrite the `module` line.
- [x] T003 [P] Create `src/go.sum`. Source: copy `~/code/sahil87/fab-kit/src/go/wt/go.sum` verbatim.

### Phase 2: Core Implementation

#### Source code port

- [x] T004 [P] Port `src/cmd/wt/` from `~/code/sahil87/fab-kit/src/go/wt/cmd/`. Copy all 16 files: `main.go`, `create.go`, `create_test.go`, `delete.go`, `delete_test.go`, `edge_test.go`, `init.go`, `init_test.go`, `integration_test.go`, `list.go`, `list_test.go`, `open.go`, `open_test.go`, `shell_setup.go`, `shell_setup_test.go`, `testutil_test.go`. In each `.go` file, rewrite the import line `wt "github.com/sahil87/fab-kit/src/go/wt/internal/worktree"` to `wt "github.com/sahil87/wt/internal/worktree"`. The 6 cmd files that need the rewrite: `main.go`, `create.go`, `delete.go`, `init.go`, `list.go`, `open.go`. No other content changes.
- [x] T005 [P] Port `src/internal/worktree/` from `~/code/sahil87/fab-kit/src/go/wt/internal/worktree/`. Copy all 17 files: `apps.go`, `apps_test.go`, `context.go`, `context_test.go`, `crud.go`, `errors.go`, `errors_test.go`, `git.go`, `git_test.go`, `menu.go`, `names.go`, `names_test.go`, `platform.go`, `rollback.go`, `rollback_test.go`, `stash.go`, `worktree.go`. None of these files import the `cmd` package, so no import rewrites are needed. Verify with `grep -l 'sahil87/fab-kit' src/internal/worktree/` — must return zero matches.

#### Build & release scripts

- [x] T006 [P] Create `scripts/build.sh`. Source: `~/code/sahil87/hop/scripts/build.sh`. Substitutions: `hop` → `wt`, `./cmd/hop` → `./cmd/wt`. Make executable (`chmod +x`).
- [x] T007 [P] Create `scripts/install.sh`. Source: `~/code/sahil87/hop/scripts/install.sh`. Substitutions: `hop` → `wt` in `DEST` path and the final `installed:` echo. Make executable.
- [x] T008 [P] Create `scripts/release.sh`. Source: `~/code/sahil87/hop/scripts/release.sh`. Copy verbatim — no substitutions needed (script does not reference `hop` or `wt` by name). Make executable.
- [x] T009 [P] Create `justfile` at repo root. Source: `~/code/sahil87/hop/justfile`. Substitutions: `hop` → `wt` everywhere it appears in user-facing recipe descriptions and the `bin/hop` reference. Recipes: `default`, `build`, `local-install`, `test` (`cd src && go test ./...`), `release bump="patch"`.

#### Release workflow & formula

- [x] T010 [P] Create `.github/workflows/release.yml`. Source: `~/code/sahil87/hop/.github/workflows/release.yml`. Substitutions per spec table in `Release: GitHub Workflow / Requirement: release workflow`: all `hop-${os}-${arch}` → `wt-${os}-${arch}`, `./cmd/hop` → `./cmd/wt`, `-o "../dist/${output}/hop"` → `-o "../dist/${output}/wt"`, `-C "../dist/${output}" hop` → `-C "../dist/${output}" wt`, `Formula/hop.rb` → `Formula/wt.rb`, all four `dist/hop-{os}-{arch}.tar.gz` references → `dist/wt-{os}-{arch}.tar.gz`, commit message `hop v${version}` → `wt v${version}`. All other content (action SHA pins, base-tag logic, tap clone URL, `sed` substitution mechanics) preserved verbatim.
- [x] T011 [P] Create `.github/formula-template.rb`. Source: `~/code/sahil87/hop/.github/formula-template.rb`. Substitutions per spec table in `Release: GitHub Workflow / Requirement: Formula template`: `class Hop < Formula` → `class Wt < Formula`, `desc "Locate, open, list, and operate on repos from hop.yaml"` → `desc "Git worktree management CLI"`, `homepage "https://github.com/sahil87/hop"` → `homepage "https://github.com/sahil87/wt"`, all `hop-{os}-{arch}.tar.gz` URL paths → `wt-{os}-{arch}.tar.gz`, `bin.install "hop"` → `bin.install "wt"`, `shell_output("#{bin}/hop --version")` → `shell_output("#{bin}/wt --version")`. The `VERSION_PLACEHOLDER` and four `SHA_*` markers MUST be preserved literally so the workflow's `sed` step works without modification.

#### Docs

- [x] T012 Update `docs/specs/index.md` to list the 4 new spec files (with `cli-surface.md`, `worktree-layout.md`, `init-protocol.md`, `build-and-release.md` rows). Keep the existing preamble describing pre-implementation specs vs post-implementation memory. The table SHALL gain 4 rows.
- [x] T013 [P] Create `docs/specs/cli-surface.md`. Content: per-subcommand reference for `wt create`, `wt list`, `wt open`, `wt delete`, `wt init`, `wt shell-setup`. For each subcommand: one-line summary, flags table (long name, short name if any, default, description), positional args, exit codes (referencing `internal/worktree/errors.go`'s `ExitGeneralError`, `ExitGitError`, `ExitUserAbort`, etc. — read those constants from the ported source). Source material: the `## wt (Worktree Management)` section of `~/code/sahil87/fab-kit/docs/specs/packages.md` and the actual flag definitions in the ported `src/cmd/wt/*.go` files.
- [x] T014 [P] Create `docs/specs/worktree-layout.md`. Content: filesystem layout (`<repo>.worktrees/{name}/` siblings to the main repo), naming convention (random adjective-noun from `internal/worktree/names.go`'s word lists, `--worktree-name` override), branch ↔ worktree relationship (each worktree has its own branch; the branch is independent of the worktree name). Source material: the **Worktree Directory** subsection of `~/code/sahil87/fab-kit/docs/specs/naming.md` plus inferences from `src/internal/worktree/names.go` and `crud.go`.
- [x] T015 [P] Create `docs/specs/init-protocol.md`. Content: the init script lookup contract — `WORKTREE_INIT_SCRIPT` env var, `"fab sync"` default, command-vs-path detection (presence of space → command, otherwise → path resolved relative to `git rev-parse --git-common-dir`), execution working directory (`git rev-parse --show-toplevel` of the current worktree), graceful skip behavior when command not on PATH or path not found, exit semantics. Source material: `src/cmd/wt/init.go` and `src/internal/worktree/context.go` (after porting).
- [x] T016 Create `docs/specs/build-and-release.md`. Content: build flow (`just build` → `scripts/build.sh` → `bin/wt` stamped from `git describe`), local install flow (`just local-install` → `~/.local/bin/wt`), release flow (`just release [patch|minor|major]` → tag computed from `git describe` → push tag → workflow runs), cross-compile matrix (4 platforms: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64), GitHub Release publication, Homebrew tap update mechanism (clone tap, `sed` template, commit, push), and a **Pre-Release Setup** section enumerating: (1) `HOMEBREW_TAP_TOKEN` repo secret on `github.com/sahil87/wt` with push access to `github.com/sahil87/homebrew-tap`, (2) `Formula/wt.rb` placeholder in the tap repo (already present per coupling analysis).
- [x] T017 Replace `README.md` at repo root. Sections: title + one-paragraph elevator pitch, Install (Homebrew tap one-liner `brew install sahil87/tap/wt` + manual via `just local-install`), Usage (subcommand list with one-liners — derived from `cli-surface.md`), Specs (link to `docs/specs/index.md`), Footer note pointing back to the fab-kit toolkit hub for users arriving from fab-kit context.

### Phase 3: Integration & Edge Cases

- [x] T018 Run the full Go test suite to verify the port. Command: `cd src && go test ./...`. Expected: all tests pass with no compilation errors, no test failures, no skips. If any test fails: investigate the root cause — most likely a missed import-path rewrite. Re-check with `grep -r 'sahil87/fab-kit' src/`.
- [x] T019 Run `go vet ./...` and `go build ./...` from `src/` to catch any non-test issues (unused imports, type errors). Expected: clean output.
- [x] T020 Run `./scripts/build.sh` from repo root and verify `bin/wt --version` works. Expected: produces `bin/wt`, version string contains either `dev` (no tags yet) or a tag if one has been created. This catches build-script regressions.
- [x] T021 Run `just test` and `just build` to verify the justfile recipes work end-to-end. Expected: both succeed.

### Phase 4: Polish

- [x] T022 Audit: `grep -r 'sahil87/fab-kit' src/ docs/specs/` (excluding the change folder) — must return zero matches in source/specs. Spec-side fab-kit references are allowed only in: README footer (link to fab-kit hub), `init-protocol.md` (mentioning fab-kit as one source of `fab sync`), `build-and-release.md` (only if it references fab-kit's tap setup pattern, which it does not need to).
- [x] T023 Audit: verify `docs/specs/index.md` lists exactly 4 detail specs (cli-surface, worktree-layout, init-protocol, build-and-release) and each linked file exists and is non-empty.
- [x] T024 Audit: verify the constitution's principles are observably satisfied — module path declared (`src/go.mod`), `cmd/` thin, `internal/worktree/` houses logic, exit codes via `errors.go`, tests alongside code.

---

## Execution Order

- **Phase 1** (T001–T003) is independent; T002+T003 can run in parallel.
- **Phase 2** (T004–T017): T004 and T005 are independent ports; can run in parallel. T006–T011 are independent script/workflow file creations; can run in parallel. T012 (specs index update) depends on T013–T016 having defined what specs exist; do T013–T016 first then T012. T017 (README) is independent. Within Phase 2, the recommended grouping:
  - Group A (parallel): T004, T005, T006, T007, T008, T009, T010, T011, T013, T014, T015, T016, T017
  - Then sequentially: T012 (after T013–T016)
- **Phase 3** (T018–T021) requires Phase 2 complete (specifically T002, T003, T004, T005, T006, T009 for tests/build/justfile).
  - T018 (go test) blocks T019 (go vet) — same source state.
  - T020 (build.sh) blocks T021 (just build) — same machinery.
- **Phase 4** (T022–T024) requires Phase 3 complete.

## Acceptance

### Functional Completeness

- [x] CHK-001 Module Path Rewrite: `src/go.mod` declares `module github.com/sahil87/wt`; `grep -r 'sahil87/fab-kit' src/` returns zero matches
- [x] CHK-002 Source File Layout: `src/cmd/wt/` exists with all 16 cmd files; `src/internal/worktree/` exists with all 17 internal files; no `src/go/wt/` nesting
- [x] CHK-003 Tests Run Unchanged: `cd src && go test ./...` passes with zero failures, zero compilation errors, zero unexpected skips
- [x] CHK-004 Constitution Adherence: All 7 principles + 2 constraints from `fab/project/constitution.md` v1.0.0 are observably satisfied by the ported code
- [x] CHK-005 Default init script value: `internal/worktree/context.go` `InitScriptPath()` returns `"fab sync"` when `WORKTREE_INIT_SCRIPT` is unset; returns env-var value when set (verified by existing `context_test.go` lines 70-85)
- [x] CHK-006 Init script invocation contract: `cmd/init.go` correctly handles command-vs-path, missing-on-PATH, and missing-file cases (verified by existing `init_test.go` — line 40-44 covers `not found on PATH` / `skipping init`)
- [x] CHK-007 Repo root layout matches hop: `LICENSE`, `README.md`, `docs/`, `fab/`, `src/`, `scripts/`, `justfile`, `.github/workflows/release.yml`, `.github/formula-template.rb` all present
- [x] CHK-008 Build script: `scripts/build.sh` exists, executable, builds `bin/wt` from `src/cmd/wt`, stamps version from `git describe`
- [x] CHK-009 Install script: `scripts/install.sh` exists, executable, copies `bin/wt` to `${HOME}/.local/bin/wt`
- [x] CHK-010 Release script: `scripts/release.sh` exists, executable, copied verbatim from hop (no `hop` references introduced; tag-driven)
- [x] CHK-011 justfile: `default`, `build`, `local-install`, `test`, `release` recipes present and functional
- [x] CHK-012 release workflow: `.github/workflows/release.yml` matches hop's workflow with all 7 substitutions per spec table
- [x] CHK-013 Formula template: `.github/formula-template.rb` matches hop's template with all 7 substitutions; `VERSION_PLACEHOLDER` and `SHA_*` markers preserved literally
- [x] CHK-014 5-file split spec set: `docs/specs/{index,cli-surface,worktree-layout,init-protocol,build-and-release}.md` all exist and are non-empty
- [x] CHK-015 Specs are wt-specific: no `SRAD`, `operator`, `assembly line`, `change folder`, `change type`, or `intake` content describing them as wt concepts (one match for "operator" in build-and-release.md uses the English meaning "human operator action", not the fab-kit concept)
- [x] CHK-016 README content: replaces placeholder; includes elevator pitch, Install (brew + manual), Usage (subcommand list), Specs link, fab-kit hub footer
- [x] CHK-017 Setup checklist: `docs/specs/build-and-release.md` contains a Pre-Release Setup section listing `HOMEBREW_TAP_TOKEN` secret + `Formula/wt.rb` placeholder requirements

### Behavioral Correctness

- [x] CHK-018 No CLI surface change: `wt --help` and each subcommand's `--help` output is identical to fab-kit's wt (modulo Go binary version string). Spot-check `wt create --help`, `wt list --help`, `wt delete --help`.
- [x] CHK-019 No exit code change: typed exit codes from `internal/worktree/errors.go` are unchanged (no constants renamed, removed, or repurposed) — `ExitSuccess=0`, `ExitGeneralError=1`, `ExitInvalidArgs=2`, `ExitGitError=3`, `ExitRetryExhausted=4`, `ExitByobuTabError=5`, `ExitTmuxWindowError=6`
- [x] CHK-020 Init protocol behavior unchanged: `wt init` with `WORKTREE_INIT_SCRIPT` unset and `fab` not on PATH prints the documented warning and exits 0 (verified by `init_test.go:40-46`)

### Scenario Coverage

- [x] CHK-021 Scenario "go build at repo root succeeds" — verified by `cd src && go build ./cmd/wt`
- [x] CHK-022 Scenario "No fab-kit import paths remain" — verified by `grep -r 'sahil87/fab-kit' src/` returning zero matches
- [x] CHK-023 Scenario "Layout matches convention" — verified by directory inspection (no `src/go/wt/` nesting; structure is `src/{cmd/wt,internal/worktree,go.mod,go.sum}`)
- [x] CHK-024 Scenario "All tests pass after port" — verified by `cd src && go test ./...` (both `cmd/wt` and `internal/worktree` packages PASS, no skips)
- [x] CHK-025 Scenario "Constitution principles satisfied" — verified by audit (CHK-004)
- [x] CHK-026 Scenario "build produces stamped binary" — `./scripts/build.sh` produces `bin/wt`, and `bin/wt --version` prints e.g. `wt version 88cff5e`. Fix applied during rework: `src/cmd/wt/main.go` now declares `var version = "dev"` and sets `root.Version = version` per hop's pattern, so the build script's `-X main.version=...` ldflag stamps the binary correctly.
- [x] CHK-027 Scenario "just test runs the suite" — verified by running `just test` (executes `cd src && go test ./...`, all pass)
- [x] CHK-028 Scenario "All specs present and indexed" — verified by inspecting `docs/specs/index.md` (4 detail rows: cli-surface, worktree-layout, init-protocol, build-and-release) and confirming each linked file exists and is non-empty
- [x] CHK-029 Scenario "No fab-flavored content" — grep shows only one benign match for "operator" (used in the English sense in `build-and-release.md` Pre-Release Setup) and no SRAD/assembly-line/change-folder/change-type/intake terms
- [x] CHK-030 Scenario "README replaces placeholder" — verified by reading `README.md` (elevator pitch, brew install one-liner, usage, specs link, fab-kit footer)

### Edge Cases & Error Handling

- [x] CHK-031 Missing init command edge case: with `WORKTREE_INIT_SCRIPT` unset, no `fab` on PATH → graceful skip with guidance message (existing test coverage in `init_test.go:40-46`)
- [x] CHK-032 Missing init file edge case: with `WORKTREE_INIT_SCRIPT` set to nonexistent file → graceful skip with guidance (existing test coverage in `init_test.go:26-37`)
- [x] CHK-033 Build at untagged commit: `git describe --tags --always` returns the short SHA when no tags exist; `|| echo dev` only triggers if `git describe` itself errors (e.g. outside a git repo). `scripts/build.sh` matches hop's pattern verbatim. Doc in `build-and-release.md` corrected during rework to describe this accurately.

### Code Quality

- [x] CHK-034 Pattern consistency: ported source files preserve fab-kit's internal patterns exactly; no reformatting, renames, or stylistic edits (file-list diff against fab-kit's `src/go/wt/cmd/` and `src/go/wt/internal/worktree/` shows identical filenames)
- [x] CHK-035 No unnecessary duplication: no helper functions added beyond what exists in fab-kit's wt; no shadow copies of existing utilities
- [x] CHK-036 Readability and maintainability: import paths uniformly rewritten (no half-converted files mixing both module paths) — `grep -r 'sahil87/fab-kit' src/` returns zero matches
- [x] CHK-037 Follow existing project patterns: scripts, justfile, release.yml, formula-template all derived structurally from hop with only mechanical substitutions
- [x] CHK-038 Anti-pattern: no god functions introduced — port preserves fab-kit's existing structure as-is
- [x] CHK-039 Anti-pattern: no magic strings — `VERSION_PLACEHOLDER` and `SHA_*` markers in formula-template are documented as deliberate placeholders, not magic numbers (spec table at lines 191-202 enumerates them explicitly)

### Notes

- Check items as you review: `- [x]`
- All items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] CHK-NNN **N/A**: {reason}`
- No Security category — this is a pure source port with zero new attack surface; the existing wt codebase's security posture (no network, no shell injection, no untrusted input handling beyond what fab-kit's wt already shipped) is preserved verbatim.
- No Removal Verification category — no deprecated requirements; nothing is being removed in this change.
