# Tasks: Port wt from fab-kit

**Change**: 260504-4bl4-port-wt-from-fab-kit
**Spec**: `spec.md`
**Intake**: `intake.md`

## Phase 1: Setup

- [x] T001 Replace `.gitignore` at repo root with hop-flavored Go gitignore. Source: `~/code/sahil87/hop/.gitignore`. Action: copy verbatim, then re-add the wt-specific tail (`.fab-*`, `/.agents`, `/.claude`, `/.cursor`, `/.opencode`, `/.codex`, `/.gemini`, `# fab` + `.fab-status.yaml`) that the current Node-template `.gitignore` already contains so we don't lose them. Result: a single `.gitignore` containing Go binary patterns + `bin/` + `dist/` + the fab/agent ignores.
- [x] T002 [P] Create `src/go.mod` declaring `module github.com/sahil87/wt`, `go 1.22`, and the `cobra v1.8.1` + transitive deps. Source: copy `~/code/sahil87/fab-kit/src/go/wt/go.mod` and rewrite the `module` line.
- [x] T003 [P] Create `src/go.sum`. Source: copy `~/code/sahil87/fab-kit/src/go/wt/go.sum` verbatim.

## Phase 2: Core Implementation

### Source code port

- [x] T004 [P] Port `src/cmd/wt/` from `~/code/sahil87/fab-kit/src/go/wt/cmd/`. Copy all 16 files: `main.go`, `create.go`, `create_test.go`, `delete.go`, `delete_test.go`, `edge_test.go`, `init.go`, `init_test.go`, `integration_test.go`, `list.go`, `list_test.go`, `open.go`, `open_test.go`, `shell_setup.go`, `shell_setup_test.go`, `testutil_test.go`. In each `.go` file, rewrite the import line `wt "github.com/sahil87/fab-kit/src/go/wt/internal/worktree"` to `wt "github.com/sahil87/wt/internal/worktree"`. The 6 cmd files that need the rewrite: `main.go`, `create.go`, `delete.go`, `init.go`, `list.go`, `open.go`. No other content changes.
- [x] T005 [P] Port `src/internal/worktree/` from `~/code/sahil87/fab-kit/src/go/wt/internal/worktree/`. Copy all 17 files: `apps.go`, `apps_test.go`, `context.go`, `context_test.go`, `crud.go`, `errors.go`, `errors_test.go`, `git.go`, `git_test.go`, `menu.go`, `names.go`, `names_test.go`, `platform.go`, `rollback.go`, `rollback_test.go`, `stash.go`, `worktree.go`. None of these files import the `cmd` package, so no import rewrites are needed. Verify with `grep -l 'sahil87/fab-kit' src/internal/worktree/` — must return zero matches.

### Build & release scripts

- [x] T006 [P] Create `scripts/build.sh`. Source: `~/code/sahil87/hop/scripts/build.sh`. Substitutions: `hop` → `wt`, `./cmd/hop` → `./cmd/wt`. Make executable (`chmod +x`).
- [x] T007 [P] Create `scripts/install.sh`. Source: `~/code/sahil87/hop/scripts/install.sh`. Substitutions: `hop` → `wt` in `DEST` path and the final `installed:` echo. Make executable.
- [x] T008 [P] Create `scripts/release.sh`. Source: `~/code/sahil87/hop/scripts/release.sh`. Copy verbatim — no substitutions needed (script does not reference `hop` or `wt` by name). Make executable.
- [x] T009 [P] Create `justfile` at repo root. Source: `~/code/sahil87/hop/justfile`. Substitutions: `hop` → `wt` everywhere it appears in user-facing recipe descriptions and the `bin/hop` reference. Recipes: `default`, `build`, `local-install`, `test` (`cd src && go test ./...`), `release bump="patch"`.

### Release workflow & formula

- [x] T010 [P] Create `.github/workflows/release.yml`. Source: `~/code/sahil87/hop/.github/workflows/release.yml`. Substitutions per spec table in `Release: GitHub Workflow / Requirement: release workflow`: all `hop-${os}-${arch}` → `wt-${os}-${arch}`, `./cmd/hop` → `./cmd/wt`, `-o "../dist/${output}/hop"` → `-o "../dist/${output}/wt"`, `-C "../dist/${output}" hop` → `-C "../dist/${output}" wt`, `Formula/hop.rb` → `Formula/wt.rb`, all four `dist/hop-{os}-{arch}.tar.gz` references → `dist/wt-{os}-{arch}.tar.gz`, commit message `hop v${version}` → `wt v${version}`. All other content (action SHA pins, base-tag logic, tap clone URL, `sed` substitution mechanics) preserved verbatim.
- [x] T011 [P] Create `.github/formula-template.rb`. Source: `~/code/sahil87/hop/.github/formula-template.rb`. Substitutions per spec table in `Release: GitHub Workflow / Requirement: Formula template`: `class Hop < Formula` → `class Wt < Formula`, `desc "Locate, open, list, and operate on repos from hop.yaml"` → `desc "Git worktree management CLI"`, `homepage "https://github.com/sahil87/hop"` → `homepage "https://github.com/sahil87/wt"`, all `hop-{os}-{arch}.tar.gz` URL paths → `wt-{os}-{arch}.tar.gz`, `bin.install "hop"` → `bin.install "wt"`, `shell_output("#{bin}/hop --version")` → `shell_output("#{bin}/wt --version")`. The `VERSION_PLACEHOLDER` and four `SHA_*` markers MUST be preserved literally so the workflow's `sed` step works without modification.

### Docs

- [x] T012 Update `docs/specs/index.md` to list the 4 new spec files (with `cli-surface.md`, `worktree-layout.md`, `init-protocol.md`, `build-and-release.md` rows). Keep the existing preamble describing pre-implementation specs vs post-implementation memory. The table SHALL gain 4 rows.
- [x] T013 [P] Create `docs/specs/cli-surface.md`. Content: per-subcommand reference for `wt create`, `wt list`, `wt open`, `wt delete`, `wt init`, `wt shell-setup`. For each subcommand: one-line summary, flags table (long name, short name if any, default, description), positional args, exit codes (referencing `internal/worktree/errors.go`'s `ExitGeneralError`, `ExitGitError`, `ExitUserAbort`, etc. — read those constants from the ported source). Source material: the `## wt (Worktree Management)` section of `~/code/sahil87/fab-kit/docs/specs/packages.md` and the actual flag definitions in the ported `src/cmd/wt/*.go` files.
- [x] T014 [P] Create `docs/specs/worktree-layout.md`. Content: filesystem layout (`<repo>.worktrees/{name}/` siblings to the main repo), naming convention (random adjective-noun from `internal/worktree/names.go`'s word lists, `--worktree-name` override), branch ↔ worktree relationship (each worktree has its own branch; the branch is independent of the worktree name). Source material: the **Worktree Directory** subsection of `~/code/sahil87/fab-kit/docs/specs/naming.md` plus inferences from `src/internal/worktree/names.go` and `crud.go`.
- [x] T015 [P] Create `docs/specs/init-protocol.md`. Content: the init script lookup contract — `WORKTREE_INIT_SCRIPT` env var, `"fab sync"` default, command-vs-path detection (presence of space → command, otherwise → path resolved relative to `git rev-parse --git-common-dir`), execution working directory (`git rev-parse --show-toplevel` of the current worktree), graceful skip behavior when command not on PATH or path not found, exit semantics. Source material: `src/cmd/wt/init.go` and `src/internal/worktree/context.go` (after porting).
- [x] T016 Create `docs/specs/build-and-release.md`. Content: build flow (`just build` → `scripts/build.sh` → `bin/wt` stamped from `git describe`), local install flow (`just local-install` → `~/.local/bin/wt`), release flow (`just release [patch|minor|major]` → tag computed from `git describe` → push tag → workflow runs), cross-compile matrix (4 platforms: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64), GitHub Release publication, Homebrew tap update mechanism (clone tap, `sed` template, commit, push), and a **Pre-Release Setup** section enumerating: (1) `HOMEBREW_TAP_TOKEN` repo secret on `github.com/sahil87/wt` with push access to `github.com/sahil87/homebrew-tap`, (2) `Formula/wt.rb` placeholder in the tap repo (already present per coupling analysis).
- [x] T017 Replace `README.md` at repo root. Sections: title + one-paragraph elevator pitch, Install (Homebrew tap one-liner `brew install sahil87/tap/wt` + manual via `just local-install`), Usage (subcommand list with one-liners — derived from `cli-surface.md`), Specs (link to `docs/specs/index.md`), Footer note pointing back to the fab-kit toolkit hub for users arriving from fab-kit context.

## Phase 3: Integration & Edge Cases

- [x] T018 Run the full Go test suite to verify the port. Command: `cd src && go test ./...`. Expected: all tests pass with no compilation errors, no test failures, no skips. If any test fails: investigate the root cause — most likely a missed import-path rewrite. Re-check with `grep -r 'sahil87/fab-kit' src/`.
- [x] T019 Run `go vet ./...` and `go build ./...` from `src/` to catch any non-test issues (unused imports, type errors). Expected: clean output.
- [x] T020 Run `./scripts/build.sh` from repo root and verify `bin/wt --version` works. Expected: produces `bin/wt`, version string contains either `dev` (no tags yet) or a tag if one has been created. This catches build-script regressions.
- [x] T021 Run `just test` and `just build` to verify the justfile recipes work end-to-end. Expected: both succeed.

## Phase 4: Polish

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
