# Plan: Adopt the Toolkit Skill Standard (`wt skill`)

**Change**: 260717-v7xy-adopt-skill-standard
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. The toolkit `skill` standard (`shll standards skill`)
     is the binding contract; requirements below restate its "Rules with teeth"
     and "Verifying conformance" as RFC-2119 statements scoped to wt. -->

### skill: Canonical bundle (`docs/site/skill.md`)

#### R1: A canonical agent usage bundle exists at `docs/site/skill.md`
The repo SHALL contain a new file `docs/site/skill.md` — an agent-first usage briefing, **static-only** (no timestamps, no environment lookups, no session state), in the "usage briefing" genre (not a README clone, not a flag table). It SHALL cover the five content areas the standard prescribes: when-to-use, capabilities map (one line per subcommand), composition patterns, output & exit-code contracts, and gotchas.

- **GIVEN** the standard requires a canonical `docs/site/skill.md` for every adopting tool
- **WHEN** the repo is inspected after this change
- **THEN** `docs/site/skill.md` exists, is static-only, and covers the five content areas in agent-first language
- **AND** because it is part of the pulled `docs/site/**` tree, it renders at `/wt/skill` on shll.ai with no extra wiring

#### R2: The bundle is ≤150 lines
The bundle SHALL be **≤150 lines** — a hard budget per the standard (principle №9), pinned by a build-time test so it can never silently exceed it.

- **GIVEN** the standard mandates a ≤150-line hard budget
- **WHEN** the line count of the embedded bundle is measured in a test
- **THEN** the count is ≤150; a bundle exceeding the budget fails the test (and thus CI)

### skill: Subcommand (`wt skill`)

#### R3: `wt skill` prints the bundle byte-identically to stdout, exit 0, empty stderr
The binary SHALL expose a **visible** (not `Hidden`) subcommand named exactly `skill` that prints the embedded bundle as **raw markdown to stdout**, byte-identical to `docs/site/skill.md`, with **stderr empty on success** and exit code **0** — no rendering, no pager, no added framing. It SHALL accept no positional arguments (`cobra.NoArgs`).

- **GIVEN** the invocation contract from `shll standards skill`
- **WHEN** `wt skill` is invoked with no arguments
- **THEN** stdout equals the embedded bundle bytes exactly, stderr is empty, and the process exits 0
- **AND GIVEN** an extra positional argument is supplied, **THEN** the command rejects it with a non-zero exit (`cobra.NoArgs`)
- **AND** `skill` is registered on the root command and appears in `wt -h` and the `help-dump` tree (it is not `Hidden`)

#### R4: The bundle is embedded via a committed copy + sync + drift-guard mechanism
The bundle SHALL be embedded into the binary via `//go:embed` from a **committed copy** inside the cmd package (`src/cmd/wt/skill.md`), refreshed from the canonical `docs/site/skill.md` by a sync script (`scripts/sync-skill.sh`), with a **drift-guard test** that fails when the committed copy diverges from the canonical file. This mirrors the mechanism `shll standards` established, adapted to wt's single-file case (no embed subdirectory).

- **GIVEN** the Go module root is `src/` and `docs/site/` sits above it, so `//go:embed` cannot reach the canonical file directly
- **WHEN** a clean `go build ./...` runs (which never runs the sync script)
- **THEN** it compiles against the committed `src/cmd/wt/skill.md` copy
- **AND GIVEN** the canonical `docs/site/skill.md` is edited without re-running the sync script, **WHEN** `go test ./...` runs, **THEN** the drift-guard test fails naming the fix (`just sync-skill`)

#### R5: Static-only invariance
The bundle output SHALL be identical across invocations (static-only) — it carries no dynamic, environment-derived content.

- **GIVEN** the standard's static-only rule (contrast `run-kit context`'s dynamic header)
- **WHEN** `wt skill` is invoked twice
- **THEN** the two stdout outputs are byte-identical

### skill: Build wiring & discoverability

#### R6: Sync wiring — script, `//go:generate`, justfile recipe
The repo SHALL provide `scripts/sync-skill.sh` (`set -euo pipefail`; `cd` to repo root; `cp -f docs/site/skill.md src/cmd/wt/skill.md`; one-line confirmation), a `//go:generate` directive in `skill.go` pointing at it, and a `sync-skill` justfile recipe that runs the script.

- **GIVEN** the intake's sync-wiring design mirroring shll's layout
- **WHEN** `just sync-skill` (or `scripts/sync-skill.sh`, or `go generate ./...`) runs
- **THEN** `src/cmd/wt/skill.md` is refreshed from `docs/site/skill.md` and a confirmation line is printed

#### R7: README discoverability row
`README.md`'s `### Command reference` table SHALL gain one row for `wt skill` (agent-facing discoverability, principle №10).

- **GIVEN** principle №10 favors agent-discoverable documentation
- **WHEN** the README command-reference table is read
- **THEN** it contains a `wt skill` row summarizing the command

### Non-Goals

- No CI workflow changes — the drift-guard and line-budget tests ride the existing `go test ./...` job (and the `ci-gate` required check).
- No `internal/worktree` logic — writing embedded bytes to stdout is pure orchestration; Constitution V pushes only *non-trivial* logic down (matches shll's `standards` call).
- No `--json` or other flags on `wt skill` — the standard specifies raw markdown to stdout only.
- Memory hydration (`docs/memory/wt-cli/skill-command-contract`, flipping `toolkit-standards-conformance`) is a hydrate-stage concern, not apply.
- Checking off the `[v7xy]` backlog entry is an archive-time concern, not apply.

### Design Decisions

1. **Single-file embed, no subdirectory**: `//go:embed skill.md` → `src/cmd/wt/skill.md`, embedded as `[]byte` — *Why*: shll's `standards/` dir exists only because it embeds four documents; one file needs no dir or `embed.FS`. *Rejected*: an `embed.FS` over a `skill/` dir (unnecessary indirection for one file).
2. **Bundle logic stays in `cmd/`**: no `internal/worktree` involvement — *Why*: Constitution V pushes only non-trivial logic down; writing embedded bytes to stdout is trivial orchestration. *Rejected*: a `worktree.SkillBundle()` accessor (adds a package boundary for a byte slice).
3. **Update the help-dump subcommand-count test**: adding a visible subcommand changes the live command tree from 8 to 9 subcommands — *Why*: `TestHelpDump_EmitsValidEnvelope` pins the exact count and the exact name set, so it MUST be updated in lockstep (Constitution: help is a published contract, `help-dump` rides the live tree). *Rejected*: making `skill` Hidden to avoid the test churn (the backlog explicitly requires "hidden-free").

## Tasks

### Phase 1: Setup

- [x] T001 Create the canonical bundle `docs/site/skill.md` — agent-first usage briefing, ≤150 lines, static-only, five content areas (when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas), authored from README.md + `docs/specs/*` + `docs/memory/wt-cli/*` <!-- R1 R2 R5 -->

### Phase 2: Core Implementation

- [x] T002 Create `scripts/sync-skill.sh` — `set -euo pipefail`, `cd "$(dirname "$0")/.."`, `cp -f docs/site/skill.md src/cmd/wt/skill.md`, echo a one-line confirmation; `chmod +x` <!-- R6 -->
- [x] T003 Create the committed embed copy `src/cmd/wt/skill.md` by running `scripts/sync-skill.sh` (do not hand-author — it MUST be a byte-identical copy of `docs/site/skill.md`) <!-- R4 -->
- [x] T004 Create `src/cmd/wt/skill.go` — `//go:generate ../../../scripts/sync-skill.sh`, `//go:embed skill.md` into `var skillBundle []byte`, and `skillCmd() *cobra.Command` (`Use: "skill"`, `Args: cobra.NoArgs`, visible, `RunE` writes `skillBundle` to `cmd.OutOrStdout()`) <!-- R3 R4 -->
- [x] T005 Register `skillCmd()` in `src/cmd/wt/main.go`'s `root.AddCommand(...)` list <!-- R3 -->

### Phase 3: Integration & Edge Cases

- [x] T006 Add `src/cmd/wt/skill_test.go` — unit tests driving `skillCmd()` with `bytes.Buffer`: (a) drift guard — embedded bytes == `os.ReadFile("../../../docs/site/skill.md")`, failure message names `just sync-skill`; (b) line budget — embedded bundle ≤150 lines; (c) command contract — writes exactly embedded bytes to stdout, nothing to stderr, `Hidden` is false, extra args rejected (`cobra.NoArgs`); (d) static-only — byte-identical across two invocations <!-- R2 R3 R4 R5 -->
- [x] T007 Add an end-to-end case to `src/cmd/wt/integration_test.go` — run the built binary: `wt skill` exits 0, stdout equals the canonical `docs/site/skill.md` bytes, stderr empty (uses `runWt` env isolation; no git state needed) <!-- R3 -->
- [x] T008 Update `TestHelpDump_EmitsValidEnvelope` in `src/cmd/wt/help_dump_test.go` — add `"skill"` to the expected visible-subcommand set and bump the expected count 8 → 9 (the new visible subcommand appears in the live tree) <!-- R3 -->

### Phase 4: Polish

- [x] T009 Add the `sync-skill` recipe to `justfile` (thin recipe running `./scripts/sync-skill.sh`, with a one-line comment) <!-- R6 -->
- [x] T010 Add one `wt skill` row to `README.md`'s `### Command reference` table <!-- R7 -->

## Execution Order

- T001 blocks T002–T004 (the canonical file must exist before the sync script copies it and the embed compiles).
- T002 blocks T003 (the script creates the committed copy).
- T003 blocks T004 (the embed target must exist to compile).
- T004 blocks T005–T008 (the command and its bytes must exist to register and test).
- T009, T010 are independent of the code path (can run alongside Phase 2/3).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/skill.md` exists, is static-only, and covers the five content areas (when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas) in agent-first usage-briefing genre
- [x] A-002 R2: the embedded bundle is ≤150 lines, pinned by a build-time test
- [x] A-003 R3: `wt skill` is a visible subcommand named exactly `skill` that prints the bundle byte-identically to stdout, exit 0, empty stderr, and rejects positional args (`cobra.NoArgs`)
- [x] A-004 R4: the bundle is embedded via a committed `src/cmd/wt/skill.md` copy refreshed by `scripts/sync-skill.sh`, with a drift-guard test that fails on divergence and names the fix
- [x] A-005 R5: `wt skill` output is byte-identical across two invocations (static-only)
- [x] A-006 R6: `scripts/sync-skill.sh` + `//go:generate` directive + `sync-skill` justfile recipe refresh the committed copy from the canonical file
- [x] A-007 R7: `README.md`'s command-reference table has a `wt skill` row

### Behavioral Correctness

- [x] A-008 R3: `skill` appears in `wt -h` and the `help-dump` tree (not `Hidden`); `TestHelpDump_EmitsValidEnvelope` expects 9 visible subcommands including `skill`
- [x] A-009 R4: a clean `go build ./...` compiles without running the sync script (compiles against the committed copy)

### Scenario Coverage

- [x] A-010 R3: an end-to-end integration test runs the built binary and asserts `wt skill` stdout == canonical file bytes, exit 0, stderr empty
- [x] A-011 R4: editing `docs/site/skill.md` without re-syncing makes the drift-guard test fail (verified by test design comparing embedded bytes to `os.ReadFile` of the canonical file)

### Edge Cases & Error Handling

- [x] A-012 R3: `wt skill extra` (unexpected positional arg) exits non-zero via `cobra.NoArgs`

### Code Quality

- [x] A-013 Pattern consistency: `skill.go` follows the repo's `xxxCmd() *cobra.Command` constructor pattern and mirrors `help_dump.go`'s thin `RunE`-writes-to-`OutOrStdout` style; `scripts/sync-skill.sh` mirrors shll's `sync-standards.sh`
- [x] A-014 No unnecessary duplication: reuses the existing `runWt`/`runWtSuccess` integration harness and the `//go:embed` + committed-copy mechanism rather than reinventing; no `internal/worktree` accessor added for a byte slice (composition over indirection)
- [x] A-015 No god functions: `skillCmd()` stays a thin constructor (<50 lines); `sync-skill.sh` is a single copy step
- [x] A-016 No magic strings: the embed path is a `//go:embed` directive on a named `var skillBundle`; the canonical path in the test is the sole literal, matching shll's precedent

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- The bundle content is static-only: no `wt --version`, no timestamps, no host lookups.

## Deletion Candidates

None — this change adds new functionality without making existing code redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse shll's committed-copy + sync-script + drift-guard mechanism, adapted to a single embedded file (no subdir, `[]byte` not `embed.FS`) | Backlog mandates reuse of the `shll standards` mechanism; inspected in the shll repo; wt shares the exact `src/`-module-root constraint. Single file needs no `embed.FS` | S:95 R:85 A:95 D:95 |
| 2 | Certain | Invocation contract: command named exactly `skill`, visible, raw markdown to stdout byte-identical to canonical, stderr empty, exit 0, `cobra.NoArgs` | The standard specifies each as a "Rule with teeth"; backlog's "hidden-free" confirms visibility | S:95 R:90 A:95 D:95 |
| 3 | Certain | Test surface: drift-guard + line-budget + command-contract + static-only unit tests in `skill_test.go`, plus one integration case | Constitution IV mandates unit (happy + failure) and integration tests per subcommand; the standard's "Verifying conformance" enumerates exactly what to pin | S:80 R:90 A:95 D:90 |
| 4 | Certain | Update `TestHelpDump_EmitsValidEnvelope` to expect 9 visible subcommands including `skill` | Adding a visible subcommand changes the live command tree the dump walks; the test hard-codes the exact count and name set, so it must move in lockstep or CI fails | S:90 R:85 A:95 D:90 |
| 5 | Confident | Sync wiring: `scripts/sync-skill.sh` + `//go:generate` in `skill.go` + `sync-skill` justfile recipe, mirroring shll's `sync-standards` naming | Mirrors shll's layout and wt's thin-justfile-over-scripts convention | S:65 R:85 A:80 D:75 |
| 6 | Confident | Bundle logic stays in `cmd/` (no `internal/worktree`) | Constitution V pushes only non-trivial logic down; shll makes the same call for `standards` | S:65 R:80 A:85 D:80 |
| 7 | Confident | Bundle content authored from README + docs/specs/* + docs/memory/wt-cli/* within the standard's five-area frame | The standard prescribes the genre and the repo's specs/memory contain all material; exact prose is apply-time authoring | S:75 R:80 A:75 D:70 |
| 8 | Confident | Add one `wt skill` row to README's command-reference table | Precedent mixed (`update`/`go` visible but unlisted) yet principle №10 favors discoverability; one row, trivially reversible | S:45 R:90 A:60 D:55 |

8 assumptions (4 certain, 4 confident, 0 tentative).
