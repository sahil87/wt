# Intake: Bind Constitution to sahil87 Toolkit Standards

**Change**: 260717-nq1y-bind-constitution-toolkit-standards
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation with a fully-specified task description (verbatim):

> Task: Amend this repo's fab constitution to bind it to the sahil87 toolkit standards.
>
> This repo is part of the sahil87 toolkit. The toolkit publishes binding, producer-facing standards — CLI design principles plus mechanical contracts (machine-readable help output, README/docs-site structure, and others over time). They are canonically authored in the sahil87/shll repository's docs/site/standards/ tree, rendered on https://shll.ai, and readable offline via the `shll standards` command. This change adds a constitution article so every future pipeline run in this repo loads and enforces the obligation.
>
> Make this change:
>
> 1. In fab/project/constitution.md, add a new article under Additional Constraints (create the section if this constitution lacks it, matching the file's existing structure):
>
>    ### Toolkit Standards
>
>    This tool is part of the sahil87 toolkit and MUST conform to the toolkit's published standards. The standards are enumerated by running `shll standards` — each entry names what it governs; read one with `shll standards <name>`. Before changing the CLI surface, help output, README.md, or docs/site/, the change MUST be checked against the standards governing that surface. If shll is unavailable, the canonical sources are the sahil87/shll repository's docs/site/standards/ tree (rendered on https://shll.ai). Standards added or revised there bind this repo without further amendment to this constitution.
>
> 2. Bump the constitution's Last Amended date (and version, per this file's own governance line).
> 3. Deliberate constraint: do NOT copy standard names, counts, or per-standard URLs into the constitution — `shll standards` is the enumeration, and the article must stay correct as standards evolve.
>
> Ship per this repo's normal flow (docs-type fab change → PR). Nothing else is in scope — no conformance fixes in this change.

No prior conversation preceded the invocation — all decisions below derive from this description.

## Why

1. **Problem**: `wt` is part of the sahil87 toolkit, whose binding producer-facing standards (CLI design principles plus mechanical contracts: machine-readable help output, README/docs-site structure, and others over time) live outside this repo — in sahil87/shll's `docs/site/standards/` tree, rendered on https://shll.ai and readable via `shll standards`. Nothing in this repo currently obligates a pipeline run to consult them, so changes to the CLI surface, help output, README, or docs site can silently drift from toolkit standards.
2. **Consequence if unfixed**: every future fab change touching a standards-governed surface relies on the human operator remembering an out-of-repo obligation. Agents running the pipeline load `fab/project/constitution.md` on every run (it is in the always-load layer) but would never learn the standards exist. Drift accumulates until a costly conformance sweep is needed.
3. **Why this approach**: the constitution is the one artifact every pipeline stage loads and treats as binding (MUST/SHOULD rules). Adding a pointer-article there makes the obligation self-enforcing on every future run, while deliberately NOT copying standard names/counts/URLs into the article — `shll standards` remains the live enumeration, so the article stays correct as standards are added or revised without further constitution amendments.

## What Changes

### 1. New article in `fab/project/constitution.md`

The constitution already has an `## Additional Constraints` section (containing `### Test Integrity` and `### Module Path Stability`), so no section creation is needed. Append a new `###` article after `### Module Path Stability`, matching the existing article structure. Article text, verbatim (em-dash punctuation matching the file's existing style):

```markdown
### Toolkit Standards

This tool is part of the sahil87 toolkit and MUST conform to the toolkit's published standards. The standards are enumerated by running `shll standards` — each entry names what it governs; read one with `shll standards <name>`. Before changing the CLI surface, help output, README.md, or docs/site/, the change MUST be checked against the standards governing that surface. If shll is unavailable, the canonical sources are the sahil87/shll repository's docs/site/standards/ tree (rendered on https://shll.ai). Standards added or revised there bind this repo without further amendment to this constitution.
```

### 2. Governance line bump

Current line (last line of the file):

```markdown
**Version**: 1.0.1 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-05-09
```

New line:

```markdown
**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18
```

MINOR bump (1.0.1 → 1.1.0): a new article is added — new binding guidance, not a wording clarification (PATCH) and not a removal/redefinition of existing principles (MAJOR; cf. the file's own `### Module Path Stability`, which reserves MAJOR for module-path changes). `Ratified` is unchanged.

### 3. Deliberate constraint (what the article must NOT contain)

Do NOT copy standard names, standard counts, or per-standard URLs into the constitution. `shll standards` is the enumeration; the article must stay correct as standards evolve. The only concrete references permitted are the ones in the verbatim text above: the `shll standards` / `shll standards <name>` commands, the sahil87/shll repo's `docs/site/standards/` tree, and https://shll.ai.

## Affected Memory

None — governance-only change. The constitution amendment binds future pipeline runs but changes no `wt` CLI behavior, so no `wt-cli` memory files are created, modified, or removed.

## Impact

- `fab/project/constitution.md` — the only file modified (one new article + governance-line bump).
- No source code, tests, README.md, or docs/site/ changes — conformance fixes are explicitly out of scope for this change.
- Downstream effect: every future pipeline run loads the amended constitution (always-load layer) and inherits the check-against-standards obligation for changes touching the CLI surface, help output, README.md, or docs/site/.
- Ship flow: normal docs-type change → PR against `main`.

## Open Questions

None — the task description specifies the article text verbatim, its placement, the governance-line bump, and the scope boundary.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Article text used verbatim as provided in the task description (with `—` em-dashes matching the file's existing punctuation style) | Text supplied in full by the user; only typographic normalization applied | S:95 R:90 A:95 D:95 |
| 2 | Certain | No new section needed — append the article under the existing `## Additional Constraints`, after `### Module Path Stability` | Task says "create the section if this constitution lacks it"; it does not lack it — verified in the current file | S:90 R:90 A:100 D:95 |
| 3 | Confident | Version bump is MINOR: 1.0.1 → 1.1.0 | Governance line carries semver but no explicit bump rules; the file's own Module Path Stability article reserves MAJOR for breaking changes, and adding a new binding article is materially more than a PATCH clarification — standard constitution-semver convention | S:65 R:85 A:75 D:70 |
| 4 | Certain | Last Amended = 2026-07-18; Ratified unchanged | Today's date; ratification date never moves on amendment | S:90 R:95 A:100 D:100 |
| 5 | Certain | Change type = docs; scope is the constitution file only, no conformance fixes | Explicit in the task: "docs-type fab change → PR. Nothing else is in scope" | S:95 R:90 A:100 D:100 |

5 assumptions (4 certain, 1 confident, 0 tentative, 0 unresolved).
