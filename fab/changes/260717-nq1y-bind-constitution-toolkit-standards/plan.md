# Plan: Bind Constitution to sahil87 Toolkit Standards

**Change**: 260717-nq1y-bind-constitution-toolkit-standards
**Intake**: `intake.md`

## Requirements

### Constitution: Toolkit Standards Binding

#### R1: New Toolkit Standards article
`fab/project/constitution.md` MUST carry a new `### Toolkit Standards` article, appended under the existing `## Additional Constraints` section immediately after `### Module Path Stability`, matching the file's existing article structure. The article text MUST be the verbatim text supplied in the intake's What Changes §1 (em-dash punctuation matching the file's style) — no additions, reordering, or paraphrase.

- **GIVEN** the constitution currently has `## Additional Constraints` containing `### Test Integrity` and `### Module Path Stability`
- **WHEN** the amendment is applied
- **THEN** a `### Toolkit Standards` article appears directly after `### Module Path Stability` and before the `## Governance` section
- **AND** its body matches the intake's verbatim article text exactly

#### R2: Deliberate content constraint
The new article MUST NOT enumerate standard names, standard counts, or per-standard URLs. The only concrete references permitted are those in the verbatim text: the `shll standards` / `shll standards <name>` commands, the `sahil87/shll` repo's `docs/site/standards/` tree, and `https://shll.ai`.

- **GIVEN** the verbatim article text from the intake
- **WHEN** the article is written into the constitution
- **THEN** no standard name, count, or per-standard URL beyond the permitted references appears in the article

#### R3: Governance line version + date bump
The constitution's governance line MUST be updated from `**Version**: 1.0.1 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-05-09` to `**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18` — a MINOR version bump (new binding article), Last Amended set to today, Ratified unchanged.

- **GIVEN** the existing governance line at the end of the file
- **WHEN** the amendment is applied
- **THEN** the version reads `1.1.0`, Last Amended reads `2026-07-18`, and Ratified remains `2026-05-03`

### Non-Goals

- No source code, test, README.md, or docs/site/ changes — conformance fixes are explicitly out of scope.
- No copying of the live standards enumeration into the repo — `shll standards` remains the authority.

## Tasks

### Phase 1: Core Implementation

- [x] T001 Append the verbatim `### Toolkit Standards` article (from intake What Changes §1) under `## Additional Constraints`, immediately after the `### Module Path Stability` article, in `fab/project/constitution.md` <!-- R1 R2 -->
- [x] T002 Update the governance line in `fab/project/constitution.md` to `**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18` <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `### Toolkit Standards` article exists in `fab/project/constitution.md`, placed under `## Additional Constraints` immediately after `### Module Path Stability`, with body matching the intake's verbatim text exactly
- [x] A-002 R3: Governance line reads `**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18`

### Behavioral Correctness

- [x] A-003 R2: The new article contains no standard names, counts, or per-standard URLs beyond the permitted `shll standards` / `shll standards <name>` commands, the `sahil87/shll` `docs/site/standards/` tree, and `https://shll.ai`

### Code Quality

- [x] A-004 Pattern consistency: The new article follows the surrounding `###`-article structure of `## Additional Constraints` (heading + prose body, em-dash punctuation matching file style)
- [x] A-005 No unnecessary duplication: No standards content is duplicated into the repo — the article points at `shll standards` as the live enumeration

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Article appended under existing `## Additional Constraints`, after `### Module Path Stability`; no section creation needed | Section already exists in the current file (verified); intake instructs "create the section if this constitution lacks it" — it does not lack it | S:90 R:90 A:100 D:95 |
| 2 | Certain | Article text used verbatim from the intake, with `—` em-dashes matching the file's punctuation | Text fully specified in the intake's What Changes §1; only typographic style-matching applied | S:95 R:90 A:95 D:95 |
| 3 | Confident | Version bump is MINOR (1.0.1 → 1.1.0) | Governance line carries semver without explicit bump rules; a new binding article exceeds a PATCH clarification, and the file reserves MAJOR for module-path/breaking changes — standard constitution-semver convention | S:65 R:85 A:75 D:70 |

3 assumptions (2 certain, 1 confident, 0 tentative).
