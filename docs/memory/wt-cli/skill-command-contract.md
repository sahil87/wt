---
type: memory
description: "The `wt skill` behavior contract — a visible subcommand printing a static ≤150-line agent usage bundle as raw markdown to stdout (byte-identical to docs/site/skill.md), empty stderr, exit 0, cobra.NoArgs; the committed-copy + scripts/sync-skill.sh + drift-guard/line-budget test mechanism that embeds it (Go module root is src/, so //go:embed cannot reach docs/site/ above it)."
---
# wt-cli: Skill Command Contract

**Domain**: wt-cli

> Post-implementation behavior capture for the `wt skill` subcommand.
> Source change: `260717-v7xy-adopt-skill-standard`.

## Overview

`wt skill` prints the agent usage bundle — a static, one-page markdown briefing for an agent operating an installed `wt` binary offline (embedded in the binary, versioned with it, no repo checkout or network trip needed). It is the `wt` half of the sahil87 toolkit's `skill` standard (agent-discoverable documentation, principle №10). The command emits raw markdown to stdout with nothing on stderr; the bundle is byte-identical to the canonical `docs/site/skill.md`, which also renders at `/wt/skill` on shll.ai as part of the pulled `docs/site/**` tree.

## Requirements

### Requirement: `wt skill` prints the bundle to stdout, exit 0, empty stderr

The binary SHALL expose a **visible** (not `Hidden`) subcommand named exactly `skill` that prints the embedded bundle as **raw markdown to stdout** — byte-identical to `docs/site/skill.md` — with **stderr empty on success** and exit code **0**. No rendering, no pager, no added framing (principle №2: stdout is the machine result, stderr is human copy). It SHALL accept no positional arguments (`cobra.NoArgs`). The logic stays in `cmd/`: writing embedded bytes to stdout is trivial orchestration with nothing non-trivial to push into `internal/worktree` (Constitution V).

#### Scenario: Successful invocation
- **GIVEN** the `skill` standard's invocation contract
- **WHEN** `wt skill` is invoked with no arguments
- **THEN** stdout equals the embedded bundle bytes exactly, stderr is empty, and the process exits 0

#### Scenario: Static-only invariance
- **GIVEN** the standard's static-only rule (no timestamps, no environment lookups — contrast run-kit's dynamic `context`)
- **WHEN** `wt skill` is invoked twice
- **THEN** the two stdout outputs are byte-identical

#### Scenario: Rejecting positional arguments
- **GIVEN** `cobra.NoArgs` on the command
- **WHEN** `wt skill extra` is invoked with an unexpected positional argument
- **THEN** it exits non-zero (mapped to `ExitGeneralError` by `main.go`'s silenced-root error path) and does NOT emit the bundle

#### Scenario: Visibility in help
- **GIVEN** the command is not `Hidden` (the backlog's "hidden-free" requirement)
- **WHEN** `wt -h` or the `help-dump` tree is inspected
- **THEN** `skill` appears among the visible subcommands — the live tree is 9 subcommands (`create, delete, go, init, list, open, shell-init, skill, update`), and `TestHelpDump_EmitsValidEnvelope` pins that count and name set (see [help-dump-contract])

### Requirement: The bundle is a static, ≤150-line usage briefing

The canonical bundle `docs/site/skill.md` SHALL be a **static-only**, agent-first usage briefing (not a README clone, not a flag table) of **≤150 lines** — a hard budget per the standard, pinned by a build-time test so it can never silently exceed it. It covers five content areas: when-to-use, capabilities map (one line per subcommand), composition patterns (the `wt shell-init` eval flow, the `WT_CD_FILE`/`WT_WRAPPER` launcher contract, the `WORKTREE_INIT_SCRIPT` init protocol), output & exit-code contracts, and gotchas.

#### Scenario: Line budget enforced at build time
- **GIVEN** the standard's ≤150-line hard budget
- **WHEN** `go test ./...` runs `TestSkill_LineBudget`
- **THEN** a bundle exceeding 150 lines fails the test (and thus CI)

### Requirement: Embed via committed copy + sync + drift guard

The bundle SHALL be embedded via `//go:embed skill.md` from a **committed copy** inside the cmd package (`src/cmd/wt/skill.md`), refreshed from the canonical `docs/site/skill.md` by `scripts/sync-skill.sh` (wired as a `//go:generate` directive in `skill.go` and a `sync-skill` justfile recipe). A **drift-guard test** (`TestSkill_EmbedMatchesCanonical`) fails whenever the committed copy diverges from the canonical file, naming the fix (`just sync-skill`). A single file is embedded directly as `[]byte` — no `embed.FS`, no subdirectory. No CI workflow changes: the drift-guard and line-budget tests ride the existing `go test ./...` job and the `ci-gate` required check.

#### Scenario: Clean build compiles against the committed copy
- **GIVEN** the Go module root is `src/` and `docs/site/` sits above it, so `//go:embed` cannot reach the canonical file directly
- **WHEN** a clean `go build ./...` runs (which never runs the sync script)
- **THEN** it compiles against the committed `src/cmd/wt/skill.md` copy

#### Scenario: Divergence fails CI
- **GIVEN** `docs/site/skill.md` is edited without re-running the sync script
- **WHEN** `go test ./...` runs
- **THEN** `TestSkill_EmbedMatchesCanonical` fails, naming `just sync-skill` as the fix

## Design Decisions

### Single-file embed, no subdirectory
**Decision**: `//go:embed skill.md` → `src/cmd/wt/skill.md`, embedded as a `[]byte` (not `embed.FS` over a `skill/` dir).
**Why**: `shll`'s `standards/` dir exists only because it embeds four documents; `wt` has one bundle, which needs no directory or `embed.FS`. Trivially restructured later if more bundles appear.
**Rejected**: an `embed.FS` over a `skill/` directory — unnecessary indirection for one file.
*Introduced by*: `260717-v7xy-adopt-skill-standard`

### Bundle logic stays in `cmd/`
**Decision**: `skillCmd()` is a thin constructor whose `RunE` writes `skillBundle` to `cmd.OutOrStdout()`; no `internal/worktree` accessor.
**Why**: Constitution V pushes only *non-trivial* logic down; writing embedded bytes to stdout is pure orchestration — `shll` makes the same call for `standards`.
**Rejected**: a `worktree.SkillBundle()` accessor — adds a package boundary for a byte slice.
*Introduced by*: `260717-v7xy-adopt-skill-standard`

### Reuse shll's committed-copy + sync + drift-guard mechanism
**Decision**: Mirror `shll standards`' embed mechanism verbatim (committed copy inside the package + `scripts/sync-skill.sh` mirroring `sync-standards.sh` + a drift-guard test comparing embedded bytes to `os.ReadFile` of the canonical file), adapted to `wt`'s single-file case.
**Why**: the backlog mandates reuse of the proven `shll standards` mechanism, and `wt` shares the exact `src/`-module-root constraint that motivated it — `//go:embed` cannot reach `docs/site/` above the module root, so a committed copy inside the package bridges the gap.
**Rejected**: `go:embed` with a relative path escaping the module root (not permitted by `go:embed`); a build-time codegen step reading the canonical file (heavier than a committed copy + drift guard).
*Introduced by*: `260717-v7xy-adopt-skill-standard`

## Cross-references

- Constitution: **Toolkit Standards** article — `wt skill` adopts the toolkit's `skill` standard; Principle II (Cobra command surface), III (typed exit codes — `cobra.NoArgs` errors map to `ExitGeneralError`), IV (test what the user sees), V (internal package boundary — logic stays thin in `cmd/`).
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md) — where the `skill` standard's verdict is recorded as adopted (this change closed the `[v7xy]` deferral); [help-dump-contract](/wt-cli/help-dump-contract.md) — the live-tree count (`skill` bumps it 8 → 9) and the shll.ai pull contract.
- Source: `src/cmd/wt/skill.go` (`skillCmd()` + `//go:embed`), `src/cmd/wt/skill.md` (committed embed copy), `src/cmd/wt/skill_test.go` (drift-guard / line-budget / contract / static-only unit tests), `src/cmd/wt/integration_test.go` (`TestIntegration_SkillBundle`, `TestIntegration_SkillRejectsArgs`), `src/cmd/wt/main.go` (`root.AddCommand(skillCmd())`), `scripts/sync-skill.sh`, `justfile` (`sync-skill` recipe), `README.md` (command-reference row).
- Canonical bundle: `docs/site/skill.md` — the byte-identical source, rendered at https://shll.ai/wt/skill.
- External: `shll standards skill` (the binding standard), sahil87/shll `src/cmd/shll/standards.go` + `scripts/sync-standards.sh` (the reference implementation this mechanism mirrors).
