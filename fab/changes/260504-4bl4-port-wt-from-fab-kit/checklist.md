# Quality Checklist: Port wt from fab-kit

**Change**: 260504-4bl4-port-wt-from-fab-kit
**Generated**: 2026-05-04
**Spec**: `spec.md`

## Functional Completeness

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

## Behavioral Correctness

- [x] CHK-018 No CLI surface change: `wt --help` and each subcommand's `--help` output is identical to fab-kit's wt (modulo Go binary version string). Spot-check `wt create --help`, `wt list --help`, `wt delete --help`.
- [x] CHK-019 No exit code change: typed exit codes from `internal/worktree/errors.go` are unchanged (no constants renamed, removed, or repurposed) — `ExitSuccess=0`, `ExitGeneralError=1`, `ExitInvalidArgs=2`, `ExitGitError=3`, `ExitRetryExhausted=4`, `ExitByobuTabError=5`, `ExitTmuxWindowError=6`
- [x] CHK-020 Init protocol behavior unchanged: `wt init` with `WORKTREE_INIT_SCRIPT` unset and `fab` not on PATH prints the documented warning and exits 0 (verified by `init_test.go:40-46`)

## Scenario Coverage

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

## Edge Cases & Error Handling

- [x] CHK-031 Missing init command edge case: with `WORKTREE_INIT_SCRIPT` unset, no `fab` on PATH → graceful skip with guidance message (existing test coverage in `init_test.go:40-46`)
- [x] CHK-032 Missing init file edge case: with `WORKTREE_INIT_SCRIPT` set to nonexistent file → graceful skip with guidance (existing test coverage in `init_test.go:26-37`)
- [x] CHK-033 Build at untagged commit: `git describe --tags --always` returns the short SHA when no tags exist; `|| echo dev` only triggers if `git describe` itself errors (e.g. outside a git repo). `scripts/build.sh` matches hop's pattern verbatim. Doc in `build-and-release.md` corrected during rework to describe this accurately.

## Code Quality

- [x] CHK-034 Pattern consistency: ported source files preserve fab-kit's internal patterns exactly; no reformatting, renames, or stylistic edits (file-list diff against fab-kit's `src/go/wt/cmd/` and `src/go/wt/internal/worktree/` shows identical filenames)
- [x] CHK-035 No unnecessary duplication: no helper functions added beyond what exists in fab-kit's wt; no shadow copies of existing utilities
- [x] CHK-036 Readability and maintainability: import paths uniformly rewritten (no half-converted files mixing both module paths) — `grep -r 'sahil87/fab-kit' src/` returns zero matches
- [x] CHK-037 Follow existing project patterns: scripts, justfile, release.yml, formula-template all derived structurally from hop with only mechanical substitutions
- [x] CHK-038 Anti-pattern: no god functions introduced — port preserves fab-kit's existing structure as-is
- [x] CHK-039 Anti-pattern: no magic strings — `VERSION_PLACEHOLDER` and `SHA_*` markers in formula-template are documented as deliberate placeholders, not magic numbers (spec table at lines 191-202 enumerates them explicitly)

## Notes

- Check items as you review: `- [x]`
- All items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] CHK-NNN **N/A**: {reason}`
- No Security category — this is a pure source port with zero new attack surface; the existing wt codebase's security posture (no network, no shell injection, no untrusted input handling beyond what fab-kit's wt already shipped) is preserved verbatim.
- No Removal Verification category — no deprecated requirements; nothing is being removed in this change.

<!-- Migrated to plan.md on 2026-05-08 — safe to delete. -->
