# Plan: shll.ai README-extraction conformance

**Change**: 260608-pj57-shll-readme-conformance
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### README: shll.ai pull conformance

#### R1: Conformant head (preserve)
The `README.md` head SHALL remain conformant with the contract's §1: a `# wt` H1, immediately followed by the EXACT canonical toolkit blockquote `> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.`, then a contiguous badge row, then a prose tagline. There SHALL be no YAML frontmatter, no leading HTML comment, and no `<h1>` above the H1.

- **GIVEN** the current README head already matches §1 verbatim
- **WHEN** this change is applied
- **THEN** the head order (H1 → blockquote → badges → tagline) is preserved unchanged
- **AND** no churn is introduced to lines 1–11

#### R2: Tail ends on real content (drop orphan footer)
The `README.md` tail SHALL end on real content. The trailing `---` separator plus the repeated toolkit blockquote + "Originally extracted from fab-kit…" footer block (current lines ~147–149) SHALL be removed so the pulled slice ends on the `## Gotchas` section. The fab-kit-origin context SHALL be relocated to `docs/site/install.md` where it is site-relevant.

- **GIVEN** there is no denylisted heading (Contributing/Development/Building/License/Acknowledgements) in the README, so the §2 tail slice runs to EOF
- **WHEN** the trailing `---` + toolkit-footer block is removed
- **THEN** the README ends on the last `## Gotchas` bullet
- **AND** the fab-kit-origin note appears in `docs/site/install.md`

#### R3: No site-escaping relative link (the core fix)
The `README.md` SHALL NOT contain a relative link to any path outside the pulled set. The line-77 relative link to `docs/specs/cli-surface.md` (which 404s on the site per §9.1.2) SHALL be replaced with a natural link INTO the `docs/site/` tree written as `docs/site/<p>.md`, which the site rewrites to `/tools/wt/<p>`. The intra-slice anchor `[Gotchas](#gotchas)` SHALL be preserved.

- **GIVEN** `docs/specs/` is never pulled and the README slice is never closure-linted or rewritten
- **WHEN** the line-77 link is replaced with a `docs/site/workflows.md` link
- **THEN** the README contains no relative link to `docs/specs/` or any other un-pulled path
- **AND** the `[Gotchas](#gotchas)` anchor remains

#### R4: Absolute images, no theme tricks
All images in `README.md` SHALL be absolute `https://…` URLs (the three shields.io badges already are; no change). No `#gh-dark-mode-only` / `#gh-light-mode-only` theme-fragment tricks SHALL be present. No Mermaid blocks exist, so none need rendering to a committed image.

- **GIVEN** the README contains only the three absolute shields.io badge images and no mermaid
- **WHEN** the verification grep is run for relative images and theme fragments
- **THEN** zero relative images and zero `#gh-*-mode-only` fragments are found

#### R5: Command/flag accuracy (§7)
Every `wt` command and flag referenced in `README.md` and `docs/site/**` SHALL exist in the actual cobra surface (`src/cmd/wt/*.go`). No command or flag SHALL be invented.

- **GIVEN** the cobra surface defines commands `create`, `list`, `open`, `delete`, `init`, `shell-init`, `update` (plus hidden `help-dump`)
- **WHEN** the docs are authored
- **THEN** every command/flag mentioned resolves to a real cobra definition
- **AND** `wt update` (present in the binary but absent from the current README table) is documented in the depth page

### docs/site: pulled depth tree (§9.1 closed-set rules)

#### R6: `docs/site/install.md` exists and is closure-clean
A new `docs/site/install.md` SHALL be authored as the full install guide: Homebrew (`brew install sahil87/tap/wt`), manual (`just local-install`), the `eval "$(wt shell-init)"` shell-wrapper step, the cross-tool `shll shell-install` note, and the relocated fab-kit-origin context. It renders at `/tools/wt/install`.

- **GIVEN** the four closed-set rules (§9.1)
- **WHEN** `docs/site/install.md` is authored
- **THEN** every relative link/image inside it resolves INSIDE `docs/site/` (cross-page links as bare relative `.md`, no `..` escape)
- **AND** every link leaving the rendered set (GitHub repo/releases, fab-kit, shll) is an absolute `https://…` URL
- **AND** the page is NOT named `overview`/`readme`/`commands`

#### R7: `docs/site/workflows.md` exists and is closure-clean
A new `docs/site/workflows.md` SHALL be authored carrying the deeper usage material: the full per-flag command reference table, the `wt create --base` start-point table, the `wt open` context-aware launcher matrix, and in-depth gotchas. It renders at `/tools/wt/workflows` and is the target of the README line-77 link.

- **GIVEN** the four closed-set rules (§9.1) and the depth sourced from README + `docs/specs/*`
- **WHEN** `docs/site/workflows.md` is authored
- **THEN** every relative link/image inside it resolves INSIDE `docs/site/` and external links are absolute `https://…`
- **AND** the per-flag reference matches the real cobra surface
- **AND** the page is NOT named `overview`/`readme`/`commands`

### Non-Goals

- shll.ai itself — already pulls + renders; nothing touched there.
- `docs/specs/`, `docs/memory/`, `src/`, tests, CI — never pulled; left untouched. `docs/specs/cli-surface.md` stays as the fab design artifact; the site reads `docs/site/` instead.
- A wholesale move of README content — the README keeps its concise Why/Install/Usage/Gotchas; docs/site pages add DEPTH and coexist (the contract accepts this coexistence; Install is pulled verbatim per §2).

### Design Decisions

1. **README line-77 link points at `docs/site/workflows.md`**: the page that now carries the full per-flag reference. — *Why*: the dead link's original intent was "full per-flag reference"; workflows.md is exactly that page. — *Rejected*: rewriting to an absolute GitHub blob URL (loses the on-site depth that Part 2 is meant to light up).
2. **Split: install.md = install/shell-init/cross-tool/origin; workflows.md = command reference + `--base` table + `wt open` matrix + gotchas**: — *Why*: matches the directive's named pages and a natural install-vs-usage boundary. — *Rejected*: a single combined page (the directive names two pages; splitting keeps each focused).

## Tasks

### Phase 1: README restructure

- [x] T001 Verify and preserve the README head (lines 1–11): `# wt` → exact canonical toolkit blockquote → contiguous badge row → prose tagline; no frontmatter/HTML-comment/`<h1>`. No edit unless drift is found. `README.md` <!-- R1 -->
- [x] T002 Remove the trailing `---` separator + repeated toolkit blockquote + "Originally extracted from fab-kit…" footer block (lines ~147–149) so the slice ends on `## Gotchas`. `README.md` <!-- R2 -->
- [x] T003 Replace the line-77 relative `docs/specs/cli-surface.md` link with a natural `docs/site/workflows.md` link; keep `[Gotchas](#gotchas)` as-is. `README.md` <!-- R3 -->

### Phase 2: docs/site depth tree

- [x] T004 [P] Author `docs/site/install.md`: Homebrew + manual (`just local-install`) install, `eval "$(wt shell-init)"` shell-wrapper step, `shll shell-install` cross-tool note (absolute URL), relocated fab-kit-origin context (absolute fab-kit URL), and a closure-clean link to `./workflows.md`. `docs/site/install.md` <!-- R6 -->
- [x] T005 [P] Author `docs/site/workflows.md`: full per-flag command reference (incl. `wt update`), `wt create --base` start-point table, `wt open` launcher matrix, in-depth gotchas, sourced from README + `docs/specs/*`; closure-clean link back to `./install.md`; external links absolute. `docs/site/workflows.md` <!-- R7 -->

### Phase 3: Verification

- [x] T006 Sanity-check every `wt` command/flag in README + docs/site against the cobra surface (`src/cmd/wt/*.go`); no invented commands/flags; `wt update` documented in workflows.md. <!-- R5 -->
- [x] T007 Grep README.md + docs/site/** for relative link/image targets (`](./`, `](../`, `](docs/`, `src="./`, `src="../`) and `#gh-*-mode-only`; confirm README links into `docs/site/` or absolute, tree links stay inside `docs/site/`, no relative images, no theme tricks; confirm head order intact. <!-- R3 R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: README head (lines 1–11) is intact: `# wt` → exact `> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.` → badge row → tagline; no frontmatter/comment/`<h1>`.
- [x] A-002 R2: README ends on the `## Gotchas` section; the trailing `---` + toolkit-footer block is gone.
- [x] A-003 R3: README contains no relative link to `docs/specs/` (or any un-pulled path); line-77 now links to `docs/site/workflows.md`; `[Gotchas](#gotchas)` preserved.
- [x] A-004 R6: `docs/site/install.md` exists, covers Homebrew/manual/shell-init/cross-tool/origin, and is closure-clean (internal links inside `docs/site/`, external links absolute, not a reserved page name).
- [x] A-005 R7: `docs/site/workflows.md` exists, covers the per-flag reference + `--base` table + `wt open` matrix + gotchas, is closure-clean, and is not a reserved page name.

### Behavioral Correctness

- [x] A-006 R3: The site-escaping relative link is replaced by a `docs/site/<p>.md` form the site rewrites to `/tools/wt/<p>` — no live 404.

### Scenario Coverage

- [x] A-007 R5: Every command/flag in README + docs/site resolves to a real cobra definition; `wt update` is documented in workflows.md.
- [x] A-008 R4: Verification grep finds zero relative images and zero `#gh-*-mode-only` fragments across README + docs/site.

### Edge Cases & Error Handling

- [x] A-009 R6 R7: No `..` escape in any docs/site relative link; cross-page links use bare relative `.md` (e.g. `./workflows.md`).

### Code Quality

- [x] A-010 Pattern consistency: New docs match the README's tight, practical voice and the repo's existing doc style.
- [x] A-011 No unnecessary duplication: docs/site pages carry DEPTH, not a verbatim copy of the README's concise sections.

## Notes

- Check items as you review: `- [x]`
- This is a `docs` change — no build/test runs; verification is the relative-link/theme-trick/head-order grep audit.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Slug `wt`; reserved page names `overview`/`readme`/`commands`; `install`/`workflows` owned by this repo | Read from the contract's per-tool table (the `wt` row); carried verbatim from the intake | S:98 R:90 A:98 D:95 |
| 2 | Certain | README head already conformant — preserve lines 1–11 unchanged | Verified the live README head matches §1 verbatim (exact blockquote, `https://shll.ai`, badge row, tagline) | S:95 R:85 A:95 D:92 |
| 3 | Confident | README line-77 link targets `docs/site/workflows.md` (not `install.md#command-reference` as the intake floated) | workflows.md is the page that now holds the full per-flag reference — the dead link's original intent; dispatch prompt explicitly directs this target | S:85 R:75 A:88 D:80 |
| 4 | Confident | Remove the trailing `---` + toolkit-footer; relocate the fab-kit-origin note into `docs/site/install.md` | No denylisted heading exists so the footer pulls as orphan chrome (§2); head blockquote already carries the toolkit framing | S:80 R:80 A:85 D:78 |
| 5 | Confident | Document `wt update` in workflows.md (present in cobra surface, absent from the current README table) | §7 command/flag accuracy is report-only-but-fix-it; the binary registers `updateCmd()` in `main.go` | S:82 R:80 A:90 D:78 |
| 6 | Tentative | Content split: install.md = install/shell-init/cross-tool/origin; workflows.md = command reference + `--base` table + `wt open` matrix + gotchas | The directive names both pages but not their exact contents; this split is a reasonable default <!-- assumed: install.md vs workflows.md content boundary — directive names the pages but not their exact contents --> | S:58 R:65 A:62 D:52 |

6 assumptions (2 certain, 3 confident, 1 tentative).
