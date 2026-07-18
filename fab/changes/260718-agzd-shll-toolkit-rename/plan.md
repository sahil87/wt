# Plan: Conform repo to the "shll toolkit" name

**Change**: 260718-agzd-shll-toolkit-rename
**Intake**: `intake.md`

## Requirements

### Documentation: Toolkit Name Conformance

#### R1: README head blockquote matches the standard's canonical line byte-for-byte
`README.md` line 3 (the blockquote in the mandated head slot between the `# wt` H1 and the badge run) SHALL be replaced with the readme-extraction standard's canonical line, byte-identical: `> Part of the [shll toolkit](https://shll.ai) — see all projects there.` (em dash U+2014 with a regular space on each side). The mandated head order (H1 line 1 → blockquote line 3 → badges line 5) MUST be preserved unchanged.

- **GIVEN** the readme-extraction standard prints the canonical blockquote `> Part of the [shll toolkit](https://shll.ai) — see all projects there.`
- **WHEN** the sweep replaces the old line-3 blockquote (`> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.`)
- **THEN** README line 3 equals the canonical line byte-for-byte, and the H1/blockquote/badge head order is intact

#### R2: README install prose uses the new toolkit/tools name
`README.md` SHALL replace the two remaining prose occurrences of the old name: line 19 `the entire sahil87 toolkit` → `the entire shll toolkit`, and line 49 `other sahil87 tools?` → `other shll tools?`. All other text on those lines — including the `github.com/sahil87/shll` URL on line 49 — MUST stay byte-identical.

- **GIVEN** README lines 19 and 49 carry `sahil87 toolkit` / `sahil87 tools` prose
- **WHEN** the sweep replaces only those two name fragments
- **THEN** line 19 reads `the entire shll toolkit`, line 49 reads `other shll tools?`, and every URL/identifier on both lines is unchanged

#### R3: docs/site/install.md heading uses the new tools name
`docs/site/install.md` line 48 heading `## Already use other sahil87 tools?` SHALL become `## Already use other shll tools?`.

- **GIVEN** the install page heading names the old toolkit
- **WHEN** the sweep edits line 48
- **THEN** the heading reads `## Already use other shll tools?` and nothing else on the page changes

#### R4: The skill.go doc comment uses the new toolkit name
`src/cmd/wt/skill.go` line 23 (inside the `skillCmd` doc comment) SHALL change `the sahil87 toolkit's \`skill\` standard` to `the shll toolkit's \`skill\` standard`. This is a Go comment only — no compiled string, exported symbol, or runtime output changes.

- **GIVEN** the `skillCmd` doc comment names the old toolkit
- **WHEN** the sweep edits the comment on line 23
- **THEN** the comment reads `the shll toolkit's \`skill\` standard`, the package still compiles, and no test golden or help-dump output moves

#### R5: The affected memory file uses the new toolkit name
`docs/memory/wt-cli/skill-command-contract.md` line 14 SHALL change `the sahil87 toolkit's \`skill\` standard` to `the shll toolkit's \`skill\` standard`. The rest of the paragraph — including the `docs/site/skill.md` and shll.ai references — MUST stay byte-identical. This edit is part of the sweep itself (the file's prose names the toolkit), not a hydrate-derived behavior update.

- **GIVEN** the memory file's Overview prose names the old toolkit
- **WHEN** the sweep edits line 14
- **THEN** the sentence reads `the shll toolkit's \`skill\` standard` and the surrounding references are unchanged

#### R6: The constitution's Toolkit Standards article uses the new toolkit name
`fab/project/constitution.md` line 36 SHALL change `This tool is part of the sahil87 toolkit` to `This tool is part of the shll toolkit`. Nothing else in the article changes — in particular the `sahil87/shll` repository reference later in the same line stays verbatim (it is the repo identifier). The governance line's `Last Amended` already reads `2026-07-18` (today), so no date bump is applied, and the Version stays `1.1.0` (cosmetic edit).

- **GIVEN** the Toolkit Standards article opens by naming the old toolkit and already carries today's `Last Amended` date
- **WHEN** the sweep edits only the opening `part of the sahil87 toolkit` fragment
- **THEN** the article opens with `part of the shll toolkit`, the `sahil87/shll` reference is unchanged, and the governance line stays byte-identical

### Non-Goals

- Identifiers stay byte-identical: `sahil87/tap` Homebrew formula references, all `github.com/sahil87/…` and `raw.githubusercontent.com/sahil87/…` URLs (including the module path `github.com/sahil87/wt`), the `sahil87/shll` canonical-source reference in the constitution, and any GitHub-owner constants in code.
- Historical artifacts under `fab/changes/` (archived changes mentioning the old name) stay untouched.
- No CLI help text, user-visible runtime string, or test golden changes — none carries the old name (verified by grep), so no golden updates and no `schema_version` bump.
- No `scripts/sync-skill.sh` re-run — neither `docs/site/skill.md` nor its embedded copy `src/cmd/wt/skill.md` contains the old name, so `TestSkill_EmbedMatchesCanonical` (the drift guard) is unaffected.
- No behavior change: no exit codes, flags, or output strings change.

### Design Decisions

1. **Blockquote replaced wholesale, not word-substituted**: R1 replaces the entire line-3 blockquote to reach the standard's exact canonical bytes — *Why*: the old and new phrasings differ in structure (`@sahil87's open source toolkit` → `the shll toolkit`), not just a single word — *Rejected*: a token substitution, which cannot produce the byte-exact canonical line.
2. **Memory + code-comment edits included in the sweep**: R4 and R5 are edited during apply even though neither is in the intake task's literal location list — *Why*: the task says "wherever they appear as prose"; both are living prose, not identifiers or archives — *Rejected*: deferring the memory edit to hydrate, which would leave the file out of conformance during review and split one mechanical sweep across two stages.

## Tasks

### Phase 1: Documentation Sweep

<!-- All edits are independent single-line prose replacements across five files; each touches a distinct file (README has three distinct lines), so all are parallelizable. -->

- [x] T001 [P] Replace `README.md` line 3 blockquote with the byte-exact canonical line `> Part of the [shll toolkit](https://shll.ai) — see all projects there.` (em dash U+2014, regular spaces), preserving H1→blockquote→badges head order <!-- R1 -->
- [x] T002 [P] In `README.md` line 19, change `the entire sahil87 toolkit` → `the entire shll toolkit`, leaving the rest of the line unchanged <!-- R2 -->
- [x] T003 [P] In `README.md` line 49, change `other sahil87 tools?` → `other shll tools?`, leaving the `github.com/sahil87/shll` URL and rest of the line unchanged <!-- R2 -->
- [x] T004 [P] In `docs/site/install.md` line 48, change heading `## Already use other sahil87 tools?` → `## Already use other shll tools?` <!-- R3 -->
- [x] T005 [P] In `src/cmd/wt/skill.go` line 23, change the doc comment `the sahil87 toolkit's` → `the shll toolkit's` (comment only) <!-- R4 -->
- [x] T006 [P] In `docs/memory/wt-cli/skill-command-contract.md` line 14, change `the sahil87 toolkit's` → `the shll toolkit's`, leaving surrounding references unchanged <!-- R5 -->
- [x] T007 [P] In `fab/project/constitution.md` line 36, change `part of the sahil87 toolkit` → `part of the shll toolkit`, leaving the `sahil87/shll` reference and governance line unchanged <!-- R6 -->

### Phase 2: Verification

- [x] T008 From `src/`, run `gofmt -l .` (must output nothing), `go vet ./...`, and `go test ./...` (all pass); confirm README line 3 is byte-identical to the canonical line via `grep -F` <!-- R1 R2 R3 R4 R5 R6 -->

## Execution Order

- T001–T007 are independent single-line edits in distinct locations; run in any order.
- T008 runs last, after all edits are applied.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `README.md` line 3 equals `> Part of the [shll toolkit](https://shll.ai) — see all projects there.` byte-for-byte (em dash U+2014, regular spaces), and the H1→blockquote→badges head order is preserved
- [x] A-002 R2: `README.md` line 19 reads `the entire shll toolkit` and line 49 reads `other shll tools?`, with every URL/identifier on both lines unchanged
- [x] A-003 R3: `docs/site/install.md` line 48 heading reads `## Already use other shll tools?`
- [x] A-004 R4: `src/cmd/wt/skill.go` line 23 comment reads `the shll toolkit's \`skill\` standard`
- [x] A-005 R5: `docs/memory/wt-cli/skill-command-contract.md` line 14 reads `the shll toolkit's \`skill\` standard`, surrounding references intact
- [x] A-006 R6: `fab/project/constitution.md` line 36 opens with `part of the shll toolkit`, the `sahil87/shll` reference and governance line unchanged

### Behavioral Correctness

- [x] A-007 R4: `src/cmd/wt/skill.go` still compiles and emits identical output — no compiled string, help-dump, or test golden changed by the comment edit

### Scenario Coverage

- [x] A-008 R1: a `grep -F` of the canonical blockquote line matches README line 3 exactly
- [x] A-009 R2 R6: identifiers preserved byte-identical — `sahil87/tap` refs, `github.com/sahil87/…` / `raw.githubusercontent.com/sahil87/…` URLs, the constitution's `sahil87/shll` reference

### Edge Cases & Error Handling

- [x] A-010 R4: full test suite green from `src/` — notably `TestSkill_EmbedMatchesCanonical` (drift guard) and the help-dump tests are undisturbed; no `scripts/sync-skill.sh` re-run needed

### Code Quality

- [x] A-011 Pattern consistency: edits change only the targeted name fragments; surrounding formatting, indentation, and phrasing follow the existing prose style of each file
- [x] A-012 No unnecessary duplication: no new files or content added; the sweep is pure in-place replacement
- [x] A-013 gofmt clean: `gofmt -l .` from `src/` outputs nothing after the `skill.go` comment edit (CI enforces gofmt before vet/test)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New README blockquote is the byte-exact canonical line (em dash U+2014, spacing verbatim) | Precondition verified live in intake: `shll standards readme-extraction` prints exactly this line | S:95 R:90 A:100 D:100 |
| 2 | Certain | The line-3 `@sahil87's open source toolkit` blockquote is the target the standard governs | Only blockquote in the README head; occupies the standard's mandated slot between H1 and badges | S:90 R:90 A:95 D:95 |
| 3 | Confident | Sweep includes the `docs/memory/wt-cli/skill-command-contract.md` and `src/cmd/wt/skill.go` doc-comment occurrences | Task says "wherever they appear as prose"; both are living prose, not identifiers or archives | S:65 R:90 A:75 D:70 |
| 4 | Certain | Identifiers stay byte-identical: `sahil87/tap`, `github.com/sahil87/…` / `raw.githubusercontent.com/sahil87/…` URLs, the `sahil87/shll` constitution reference, `fab/changes/` archives | Explicit intake item; occurrence inventory confirms each location | S:100 R:85 A:100 D:100 |
| 5 | Confident | Constitution `Last Amended` stays `2026-07-18` (bump is a no-op) and Version stays 1.1.0 | Same-day prior amendment already set today's date; cosmetic edits don't warrant a version bump | S:70 R:95 A:80 D:55 |
| 6 | Certain | No test goldens, no help-dump changes, no `scripts/sync-skill.sh` re-run | grep-verified: no user-visible string, golden, or skill-bundle content carries the old name — only the skill.go comment | S:85 R:90 A:95 D:95 |

6 assumptions (4 certain, 2 confident, 0 tentative).
