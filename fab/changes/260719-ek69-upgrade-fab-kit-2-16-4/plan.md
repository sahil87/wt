# Plan: Upgrade fab kit to 2.16.4

**Change**: 260719-ek69-upgrade-fab-kit-2-16-4
**Intake**: `intake.md`

## Requirements

<!-- Verification-only chore. `fab upgrade-repo` already applied the diff before
     the pipeline started; these requirements describe what the applied working-tree
     diff MUST satisfy. No code is written in this change (intake § Apply-stage note). -->

### Upgrade: Version Pin Sync

#### R1: fab-kit version pin reads 2.16.4
The working tree SHALL pin the fab-kit version at `2.16.4` in `fab/.fab-version` and the kit-migration version at `2.16.4` in `fab/.kit-migration-version`, reflecting the completed `fab upgrade-repo` run.

- **GIVEN** `fab upgrade-repo` upgraded the repo from kit 2.16.0 (migration state 2.15.8) to 2.16.4
- **WHEN** the version-pin files are read
- **THEN** `fab/.fab-version` reads exactly `2.16.4`
- **AND** `fab/.kit-migration-version` reads exactly `2.16.4`

#### R2: config reference fence header reflects the new kit version
The regenerated reference fence header in `fab/project/config.yaml` SHALL name kit `2.16.4`, and no overridden fields (above the fence) SHALL be altered by the upgrade.

- **GIVEN** the upgrade regenerated the `# >>> fab reference (kit …) >>>` fence body
- **WHEN** `fab/project/config.yaml` is diffed against HEAD
- **THEN** the only changed line is the fence header, from `kit 2.16.0` to `kit 2.16.4`
- **AND** no field above the fence (project identity, source/test paths, true-impact excludes, providers, tiers) is modified

#### R3: change scope is the three upgrade files plus the fab change record — no source touched
The reviewable diff SHALL consist of the three upgrade files produced by `fab upgrade-repo` plus the standard fab change-record files under `fab/changes/260719-ek69-upgrade-fab-kit-2-16-4/`; no source (`src/`) file SHALL be touched.

- **GIVEN** the upgrade is a tooling-scaffolding version bump excluded from true-impact via `true_impact_exclude: [fab/, docs/]`
- **WHEN** `git status` and the PR diff are inspected
- **THEN** the tracked modifications are the three upgrade files — `fab/.fab-version`, `fab/.kit-migration-version`, and `fab/project/config.yaml` — plus the fab change-record files under `fab/changes/260719-ek69-upgrade-fab-kit-2-16-4/` (`intake.md`, `plan.md`, `.status.yaml`, `.history.jsonl`)
- **AND** `git status --porcelain -- src/` reports no changes

### Non-Goals

- Re-running `fab upgrade-repo` — the upgrade is already applied and re-running is a no-op at 2.16.4 (intake Assumption 1)
- Writing or modifying any source code, tests, or the three already-changed files
- Updating `docs/memory/` or specs — fab scaffolding versions are not a documented behavior contract (intake § Affected Memory)
- Reviewing the gitignored `.claude/` skill repairs — they are not part of the tracked diff (intake Assumption 4)

## Tasks

<!-- Verification-only tasks. No implementation; each task confirms a property of the
     already-applied diff. All marked [x] after verification below. -->

### Phase 1: Verify Version Pins

- [x] T001 Confirm `fab/.fab-version` reads exactly `2.16.4` <!-- R1 -->
- [x] T002 [P] Confirm `fab/.kit-migration-version` reads exactly `2.16.4` <!-- R1 -->

### Phase 2: Verify Diff Scope

- [x] T003 Diff `fab/project/config.yaml` against HEAD and confirm the sole change is the fence header `kit 2.16.0` → `kit 2.16.4`, with no field above the fence altered <!-- R2 -->
- [x] T004 Confirm the tracked diff is the three upgrade files (`fab/.fab-version`, `fab/.kit-migration-version`, `fab/project/config.yaml`) plus the fab change-record files under `fab/changes/260719-ek69-upgrade-fab-kit-2-16-4/`, and nothing else <!-- R3 -->
- [x] T005 [P] Confirm `git status --porcelain -- src/` reports no changes <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `fab/.fab-version` reads `2.16.4` and `fab/.kit-migration-version` reads `2.16.4`
- [x] A-002 R2: `fab/project/config.yaml`'s only diff hunk changes the fence header from `kit 2.16.0` to `kit 2.16.4`; no overridden field above the fence is touched
- [x] A-003 R3: The tracked diff is the three upgrade files plus the fab change-record files under `fab/changes/260719-ek69-upgrade-fab-kit-2-16-4/`; no `src/` file is modified

### Behavioral Correctness

- [x] A-004 R2: The regenerated fence body is default reference content (no overridden fields moved above the fence were disturbed by the upgrade)

### Scenario Coverage

- [x] A-005 R3: `git status --porcelain -- src/` produces empty output, confirming zero true-impact footprint (`fab/` is in `true_impact_exclude`)

### Code Quality

- [x] A-006 **N/A** Pattern consistency: No code written — verification-only chore
- [x] A-007 **N/A** No unnecessary duplication: No code written — verification-only chore

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- This change writes no code; `gofmt`/`vet`/`test` outcomes are unaffected (intake § Impact)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Apply tasks are verification-only — verify the already-applied `fab upgrade-repo` diff, write no code | Intake § Apply-stage note and Assumption 1 state the upgrade ran before intake; re-running is a no-op at 2.16.4 | S:95 R:90 A:95 D:95 |
| 2 | Certain | Code Quality acceptance items are marked N/A | No source code is authored, so pattern-consistency and duplication checks have no target (intake § Impact: `src/` untouched) | S:90 R:90 A:95 D:90 |

2 assumptions (2 certain, 0 confident, 0 tentative).
