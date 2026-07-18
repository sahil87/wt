# Intake: Conform repo to the "shll toolkit" name

**Change**: 260718-agzd-shll-toolkit-rename
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation. User's raw input:

> Task: Conform this repo to the toolkit's standardized name — "shll toolkit".
>
> The toolkit formerly named "sahil87 toolkit" is now the **shll toolkit** (sahil87/shll#56). The readme-extraction standard's canonical README blockquote changed accordingly. This repo's constitution already binds it to revised standards without amendment — this task is the conformance work.
>
> Precondition: `shll standards readme-extraction` runs on this machine and shows the new blockquote (below). If not, run `shll update`; if it still shows the old line, stop and report — do not proceed from memory.
>
> Make this change:
>
> 1. **README blockquote** — replace the toolkit blockquote with this exact line, byte-identical, keeping the mandated head order (H1 -> blockquote -> badges): `> Part of the [shll toolkit](https://shll.ai) — see all projects there.`
> 2. **Prose sweep** — replace remaining `sahil87 toolkit` -> `shll toolkit` and `sahil87 tool(s)` -> `shll tool(s)` wherever they appear as prose: README, `docs/site/**` (including the skill bundle `docs/site/skill.md` if present), CLI help text and user-visible strings (update their test goldens), and `fab/project/` files. If this repo embeds docs in the binary (skill bundle or similar), re-run its sync step so drift-guard tests pass.
> 3. **Constitution (cosmetic, same PR)** — in the Toolkit Standards article, change "part of the sahil87 toolkit" to "part of the shll toolkit" and bump `Last Amended` per the file's governance line. Nothing else in the article changes.
> 4. **Do NOT touch identifiers**: `sahil87/tap` formula names, `github.com/sahil87/…` and `raw.githubusercontent.com/sahil87/…` URLs, the `sahil87/shll` canonical-source reference in the constitution article, and any GitHub-owner constants in code. Historical artifacts (`fab/changes/` archives) stay untouched.
>
> Ship per this repo's normal flow (one fab change -> PR). Tests green; if help text changed, the help-dump JSON shape is unchanged (text-only edits — no `schema_version` bump).

**Precondition verified at intake time**: `shll standards readme-extraction` was run on this machine and shows the new canonical blockquote exactly: `> Part of the [shll toolkit](https://shll.ai) — see all projects there.` (inside a ```markdown fence under "README structure" rule 1). Proceeding is authorized; no `shll update` was needed.

**Occurrence inventory verified at intake time** (grep of the whole tree, `.git/` and `fab/changes/` excluded): the old name appears in exactly 5 files — `README.md` (lines 3, 19, 49), `docs/site/install.md` (line 48), `src/cmd/wt/skill.go` (line 23, a Go comment), `docs/memory/wt-cli/skill-command-contract.md` (line 14), `fab/project/constitution.md` (line 36). No CLI help text, no user-visible runtime string, no test golden, and no `docs/site/skill.md` content carries it.

## Why

The toolkit this repo belongs to was renamed from "sahil87 toolkit" to **shll toolkit** (sahil87/shll#56), and the readme-extraction standard's canonical README blockquote changed with it. The constitution's Toolkit Standards article binds this repo to revised standards **without further amendment** — so the repo is now out of conformance until this sweep lands.

Concretely, if we don't fix it:
- The README head blockquote (`> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.`) no longer matches the standard's byte-exact canonical line, breaking the "same line in all seven repos" invariant that shll.ai's extraction relies on.
- Stale "sahil87 toolkit" / "sahil87 tools" branding persists in the README install prose, `docs/site/install.md`, a Go doc comment, a memory file, and the constitution itself — inconsistent with every other toolkit repo after the rename.

Approach: a single mechanical prose sweep in one fab change → one PR. No alternatives were considered because the task fully prescribes the edits; the only judgment calls are sweep-boundary ones (recorded in Assumptions).

## What Changes

Five files, seven line edits, all prose. No behavior change, no new capability, no removals.

### README.md (3 edits)

1. **Line 3 — the toolkit blockquote** (task item 1). Replace:

   ```markdown
   > Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.
   ```

   with this exact line, **byte-identical** (em dash U+2014 with a space on each side, exactly as the standard prints it):

   ```markdown
   > Part of the [shll toolkit](https://shll.ai) — see all projects there.
   ```

   The mandated head order (H1 → blockquote → badges) already holds — the H1 `# wt` is line 1, blockquote line 3, badge run line 5 — and MUST be preserved unchanged.

2. **Line 19** — `Installs wt (plus the shll meta-CLI) via Homebrew, handling tap trust automatically. To install the entire sahil87 toolkit instead:` → change `the entire sahil87 toolkit` to `the entire shll toolkit`. Rest of the line untouched.

3. **Line 49** — `> 💡 Have other sahil87 tools? [\`shll shell-install\`](https://github.com/sahil87/shll#shll-shell-install--wire-the-rc-file-recommended) handles all of their shell integrations and autocompletions at once.` → change `other sahil87 tools?` to `other shll tools?`. The `github.com/sahil87/shll` URL in the same line is an identifier and stays byte-identical.

### docs/site/install.md (1 edit)

Line 48 heading: `## Already use other sahil87 tools?` → `## Already use other shll tools?`

### src/cmd/wt/skill.go (1 edit, comment only)

Line 23, inside the `skillCmd` doc comment: `// bundle mandated by the sahil87 toolkit's \`skill\` standard. It prints the` → `sahil87 toolkit's` becomes `shll toolkit's`. This is a Go comment — no compiled string changes, so no test goldens move.

### docs/memory/wt-cli/skill-command-contract.md (1 edit)

Line 14: `…It is the \`wt\` half of the sahil87 toolkit's \`skill\` standard (agent-discoverable documentation, principle №10)…` → `sahil87 toolkit's` becomes `shll toolkit's`. Rest of the paragraph (including the `docs/site/skill.md` and shll.ai references) untouched. This memory edit is part of the sweep itself (the file's prose names the toolkit), not a hydrate-derived behavior update.

### fab/project/constitution.md (1 edit + governance line check)

In the **Toolkit Standards** article (line 36): `This tool is part of the sahil87 toolkit and MUST conform…` → `part of the sahil87 toolkit` becomes `part of the shll toolkit`. **Nothing else in the article changes** — in particular the sentence `…the canonical sources are the sahil87/shll repository's docs/site/standards/ tree…` keeps `sahil87/shll` verbatim (it is the repo identifier, task item 4).

Governance line: `**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18`. The task says bump `Last Amended`; it already reads today's date (2026-07-18), so the bump is a no-op — the line stays byte-identical. No Version bump (cosmetic edit).

### Explicitly NOT changed (verified against the tree)

- `sahil87/tap` Homebrew formula references — `docs/site/workflows.md` lines 105 and 114 (`sahil87/tap/wt`, `brew install sahil87/tap/wt`).
- All `github.com/sahil87/…` and `raw.githubusercontent.com/sahil87/…` URLs (module path `github.com/sahil87/wt`, release links, shll repo links).
- `fab/changes/` archives (two 260717 changes mention "sahil87 toolkit" in their intake/plan/history — historical artifacts, untouched).
- `docs/site/skill.md` and its embedded copy `src/cmd/wt/skill.md` — neither contains the old name (only generic "the toolkit's"), so **no `scripts/sync-skill.sh` re-run is needed** and `TestSkill_EmbedMatchesCanonical` (the drift guard) is unaffected.
- `docs/site/workflows.md` generic "the toolkit" phrasing (lines 59, 105 area) — not the old name, stays.
- CLI help text / help-dump: no user-visible string carries the old name, so no golden updates and no `schema_version` concern — that task clause is vacuously satisfied.

## Affected Memory

- `wt-cli/skill-command-contract`: (modify) one-word prose edit — "sahil87 toolkit's `skill` standard" → "shll toolkit's `skill` standard". Applied during the sweep; hydrate verifies no other memory file names the toolkit.

## Impact

- **Files**: 5 (README.md, docs/site/install.md, src/cmd/wt/skill.go, docs/memory/wt-cli/skill-command-contract.md, fab/project/constitution.md). One is a `.go` file but comment-only.
- **Runtime behavior**: none. No exit codes, no flags, no output strings change.
- **Tests**: no test content changes expected; full suite must stay green (notably `TestSkill_EmbedMatchesCanonical` and the help-dump tests, which this change must not disturb). CI also enforces `gofmt -l` from module root `src/` — run it after the skill.go comment edit.
- **Site extraction**: README head becomes conformant with the readme-extraction standard's rule 1 (canonical blockquote, byte-identical across repos).

## Open Questions

None — the task prescribes exact edits, the precondition was verified live, and the occurrence inventory is exhaustive.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New README blockquote is the byte-exact line from the standard (em dash, spacing verbatim) | Precondition verified live: `shll standards readme-extraction` on this machine prints exactly this line | S:95 R:90 A:100 D:100 |
| 2 | Certain | The existing line-3 blockquote (`@sahil87's open source toolkit` variant) is "the toolkit blockquote" the task targets | Only blockquote in the README head; occupies the standard's mandated slot between H1 and badges | S:90 R:90 A:95 D:95 |
| 3 | Confident | Sweep includes `docs/memory/wt-cli/skill-command-contract.md` and the `src/cmd/wt/skill.go` doc comment, though neither is in the task's location list | Task says "wherever they appear as prose" — the list reads as illustrative; both are living prose, not identifiers or archives | S:65 R:90 A:75 D:70 |
| 4 | Certain | Identifiers stay byte-identical: `sahil87/tap` formula refs, `github.com/sahil87/…` / `raw.githubusercontent.com/sahil87/…` URLs, the `sahil87/shll` constitution reference, `fab/changes/` archives | Explicit task item 4; occurrence inventory confirms where each lives | S:100 R:85 A:100 D:100 |
| 5 | Confident | Constitution `Last Amended` stays `2026-07-18` (already today — bump is a no-op) and Version stays 1.1.0 | Task says bump Last Amended only; a same-day prior amendment already set it; cosmetic edits don't warrant a version bump | S:70 R:95 A:80 D:55 |
| 6 | Certain | No test goldens, no help-dump changes, no `scripts/sync-skill.sh` re-run | Verified by grep: no user-visible string, golden, or skill-bundle content carries the old name — only the skill.go comment | S:85 R:90 A:95 D:95 |
| 7 | Confident | `change_type` overridden from inferred `feat` to `docs` | Prose/documentation-only sweep (one code comment, zero behavior change) — docs is the honest type | S:60 R:95 A:80 D:70 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
