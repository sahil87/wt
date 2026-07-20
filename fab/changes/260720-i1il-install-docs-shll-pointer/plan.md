# Plan: Conform Install Docs to Install-Composition Policy B

**Change**: 260720-i1il-install-docs-shll-pointer
**Intake**: `intake.md`

## Requirements

### Docs: Install Documentation Centralization (Policy B)

#### R1: install.md points to shll.ai instead of a per-formula brew instruction
`docs/site/install.md` MUST NOT carry a per-formula `brew install sahil87/tap/wt` install instruction section. The `## Homebrew (preferred)` section (lines 7–16: fenced `brew install sahil87/tap/wt` block, `sahil87/homebrew-tap` formula link, upgrade note) SHALL be replaced with a `## Via shll.ai (preferred)` section carrying the shll.ai pointer — the subset curl bootstrap (`curl -fsSL https://shll.ai/install | sh -s -- wt`), the full-toolkit variant (drop the `-s -- wt` suffix), the `shll install wt` alternative, a link to https://shll.ai for the full install story, and the preserved upgrade pointer (`wt update` self-updates via Homebrew → `./workflows.md#wt-update`). The replacement text is specified verbatim in the intake's "What Changes" section and SHALL be used as given.

- **GIVEN** `docs/site/install.md` with its current `## Homebrew (preferred)` section
- **WHEN** the section is replaced per the intake's verbatim replacement block
- **THEN** the file contains no `brew install sahil87/tap/wt` fenced instruction and no `sahil87/homebrew-tap` formula link
- **AND** the new section links to the shll.ai curl bootstrap, mentions `shll install wt`, and retains the `wt update` upgrade pointer to `./workflows.md#wt-update`

#### R2: All other content and files stay intact
Every other section of `docs/site/install.md` (intro, Manual build, Shell wrapper, `shll shell-install` cross-tool note, "Where wt came from" history, Next steps) SHALL remain byte-identical, and no other file SHALL change: `README.md` (already conformant), `docs/site/workflows.md` (its line ~119 `brew install sahil87/tap/wt` mention documents the `wt update` binary's own reinstall hint — the Policy A-mandated hint shape — and MUST be kept), `docs/site/skill.md` (no install content), and all source code (binary hint strings are Policy A territory, out of scope). Per the `readme-extraction` standard's docs/site-closure rules, no anchor that other pages link to may be removed (`workflows.md` links to `./install.md` and `./install.md#shell-wrapper-enables-open-here` — both unaffected; nothing links to `#homebrew-preferred`).

- **GIVEN** the working tree after the R1 edit
- **WHEN** `git diff --name-only` is inspected
- **THEN** `docs/site/install.md` is the only modified file
- **AND** the `## Shell wrapper (enables "Open here")` heading (the `#shell-wrapper-enables-open-here` anchor) and all other install.md sections are unchanged
- **AND** `docs/site/workflows.md` still carries its `wt update` reinstall-hint mention

### Non-Goals

- No source-code changes — the binary's own install-hint strings are Policy A territory (explicitly out of scope for this docs-only change)
- No README.md edit — its Install section already conforms to Policy B
- No reduction of install.md to a bare shll.ai link — Policy B prohibits only per-formula brew instructions; the Manual build path, shell-wrapper docs, and history stay

## Tasks

### Phase 2: Core Implementation

- [x] T001 Replace the `## Homebrew (preferred)` section (lines 7–16) of `docs/site/install.md` with the `## Via shll.ai (preferred)` section, verbatim per the intake's replacement block <!-- R1 -->

### Phase 3: Integration & Edge Cases

- [x] T002 Verify conformance and closure: grep `README.md` and `docs/site/` confirming no per-formula install-instruction section remains (only the kept `workflows.md` hint mention), confirm `git diff --name-only` shows only `docs/site/install.md`, and confirm the `#shell-wrapper-enables-open-here` anchor and the `./workflows.md#wt-update` link target still exist <!-- R2 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/install.md` contains a `## Via shll.ai (preferred)` section with the subset curl bootstrap (`curl -fsSL https://shll.ai/install | sh -s -- wt`), the full-toolkit variant, the `shll install wt` alternative, and a link to https://shll.ai
- [x] A-002 R2: `git diff` for this change touches only `docs/site/install.md`; README.md, workflows.md, skill.md, and all source code are unchanged

### Behavioral Correctness

- [x] A-003 R1: The fenced `brew install sahil87/tap/wt` block and the `sahil87/homebrew-tap` formula link are gone from `install.md`, while the upgrade pointer (`wt update` self-updates via Homebrew → `./workflows.md#wt-update`) is preserved in the new section

### Removal Verification

- [x] A-004 R1: No per-formula install instruction remains anywhere in `README.md` or `docs/site/` — the only `brew install sahil87/tap/wt` occurrence left is `workflows.md`'s documented `wt update` reinstall hint (kept per Policy A)

### Scenario Coverage

- [x] A-005 R2: All other install.md sections (intro, Manual, Shell wrapper, `shll shell-install` note, history, Next steps) are byte-identical to before; the `#shell-wrapper-enables-open-here` anchor survives (workflows.md links to it), and the new section's `./workflows.md#wt-update` link resolves to an existing heading

### Code Quality

- [x] A-006 Pattern consistency: The replacement section matches the site's doc style (H2 section, fenced `bash` block, ~80-col prose wrapping, relative intra-site links, absolute shll.ai links) and tells the same install story as README.md's conformant Install section
- [x] A-007 No unnecessary duplication: The section links to shll.ai/workflows.md for detail rather than restating the install story or duplicating upgrade docs

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Apply the intake's replacement block verbatim, including its line wrapping and link forms | The intake specifies the exact replacement markdown ("reproduce them in full" state-transfer rule); its link forms already satisfy the readme-extraction closure rules (relative intra-site, absolute shll.ai) | S:95 R:90 A:95 D:95 |

1 assumptions (1 certain, 0 confident, 0 tentative).
