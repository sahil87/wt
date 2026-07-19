# Intake: Upgrade fab kit to 2.16.4

**Change**: 260719-ek69-upgrade-fab-kit-2-16-4
**Created**: 2026-07-19

## Origin

> fab upgrade-repo, then drive the resulting change through the full pipeline with /fab-fff. If fab upgrade-repo produced no diff, stop — do not run /fab-fff and do not run /git-pr.

One-shot invocation. `fab upgrade-repo` was executed in this session **before** this intake was created; it produced a diff (2.16.0 → 2.16.4), so the pipeline proceeds. The upgrade work itself is **already done** — this change exists to carry the resulting diff through review and ship.

## Why

1. **Problem**: the repo's fab-kit scaffolding was pinned at 2.16.0 while the installed kit is 2.16.4. Deployed skills, config reference fences, and migration state drift from the kit that operates on them.
2. **Consequence if not fixed**: skills and config reference comments in the repo describe an older kit's behavior; future `fab` invocations run against stale migration state (`.kit-migration-version` was at 2.15.8).
3. **Approach**: the standard `fab upgrade-repo` flow — the kit's own upgrade command performs the sync/migration. No alternative applies; this is the only supported upgrade path.

## What Changes

The substantive change is the output of `fab upgrade-repo` (already applied to the working tree — uncommitted): three upgrade files (the two version pins and `fab/project/config.yaml`). The PR also carries the standard fab change-record files under `fab/changes/260719-ek69-upgrade-fab-kit-2-16-4/` (`intake.md`, `plan.md`, `.status.yaml`, `.history.jsonl`), committed by the pipeline as part of every change. `.claude/` skill repairs (6 files) are gitignored and not part of the reviewable diff.

### Version pins

```diff
--- fab/.fab-version
-2.16.0
+2.16.4
--- fab/.kit-migration-version
-2.15.8
+2.16.4
```

### Config reference fence header

`fab/project/config.yaml` — only the regenerated fence header line changes (the fence body is regenerated on every upgrade; no overridden fields were touched):

```diff
-# >>> fab reference (kit 2.16.0) >>> ---------------------------------------
+# >>> fab reference (kit 2.16.4) >>> ---------------------------------------
```

**Apply-stage note**: no code is to be written. The apply stage's tasks are verification only — confirm the upgrade touched exactly the three files above (the rest of the PR diff being the standard fab change record), confirm no source (`src/`) files changed, and confirm the upgrade completed cleanly (`fab/.fab-version` reads `2.16.4`).

## Affected Memory

None — this touches fab-kit tooling scaffolding only. No `wt` CLI behavior, spec, or memory-documented contract changes. (`docs/memory/` covers wt-cli behavior contracts; `fab/` is excluded from true-impact via `true_impact_exclude`.)

## Impact

- `fab/.fab-version`, `fab/.kit-migration-version`, `fab/project/config.yaml` — metadata/version pins only
- No Go source or test files touched; `src/` untouched — gofmt/vet/test outcomes unaffected
- `fab/` is in `true_impact_exclude`, so the change has zero true-impact footprint
- Change type: `chore`

## Open Questions

None.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Apply stage verifies the already-applied upgrade diff rather than implementing anything new | `fab upgrade-repo` ran before intake creation; its diff is the change. Re-running would be a no-op (already at 2.16.4) | S:95 R:90 A:95 D:95 |
| 2 | Certain | Change type is `chore` (tooling version bump, no behavior change) | Matches prior art: commit 52cad02 "chore: Upgrade fab kit to 2.16.0 (#42)" | S:90 R:95 A:95 D:95 |
| 3 | Certain | No memory or spec updates needed (hydrate is a no-op) | `docs/memory/` documents wt-cli behavior contracts; fab scaffolding versions are not a documented contract | S:85 R:90 A:90 D:90 |
| 4 | Confident | Gitignored `.claude/` skill repairs (6 files) are out of scope for the reviewable diff | `git check-ignore` confirms `.claude/skills/` is ignored; only tracked files ship | S:80 R:85 A:90 D:85 |

4 assumptions (3 certain, 1 confident, 0 tentative, 0 unresolved).
