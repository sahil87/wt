# Intake: shll.ai README-extraction conformance

**Change**: 260608-pj57-shll-readme-conformance
**Created**: 2026-06-08
**Status**: Draft

## Origin

> Task: conform this repo to shll.ai's README-extraction contract.
>
> shll.ai (the toolkit landing page) renders your tool's page by mechanically pulling a
> slice of your README.md and your docs/site/** tree on a daily schedule — nothing is
> hand-copied, and you push nothing. Your job is to structure your repo so that pull
> renders cleanly.
>
> Follow the §Producer conformance directive end-to-end:
> https://github.com/sahil87/shll.ai/blob/main/docs/specs/readme-extraction-contract.md
> 1. Find this repo's row in the per-tool table for slug + reserved page names.
> 2. Part 1 — restructure README.md (head order, drop footer sections below the tail
>    denylist, absolute image URLs, render mermaid to a committed image, absolute
>    external links).
> 3. Part 2 (optional, encouraged) — add a docs/site/**/*.md tree using
>    docs/site/install.md / docs/site/workflows.md.
> 4. Run the Verify checklist before opening the PR. Ship a single PR; do not touch shll.ai.

**Mode**: one-shot directive (`/fab-new` → `/fab-fff`). The full contract was fetched and read
(`gh api repos/sahil87/shll.ai/contents/docs/specs/readme-extraction-contract.md`); a gap analysis
of the current `README.md` was run before this intake was generated, so most decisions below are
grounded in the actual contract text and the actual README state rather than assumptions.

## Why

1. **Problem.** shll.ai renders this tool's `/tools/wt/readme` page by pulling a *deduced slice*
   of `README.md` (§1 head + §2 tail + §6 strips) and renders each `docs/site/**/*.md` file as its
   own page. The current `README.md` is *mostly* conformant but has one fatal defect for the site:
   a **relative link to `docs/specs/cli-surface.md`** (line 77). `docs/specs/` is never pulled and
   the README slice is **not** rewritten or closure-linted, so that link renders as a **live 404**
   on the site with no warning (§9.1.2 / directive rule 5). A trailing toolkit-footer blockquote
   below the content also pulls onto the site as orphan chrome.
2. **Consequence if unfixed.** The `wt` readme page ships with a dead link and a stray footer.
   Because the pull is daily and verbatim, the defect persists on the live site every refresh
   cycle until the README is corrected. The page never *breaks the build* (degrades to placeholder
   / commits verbatim), so the rot is silent — exactly the drift the contract is designed to
   prevent.
3. **Why this approach.** The contract is explicit that the tool repo is **canonical** and the fix
   belongs in the README, never on shll.ai. We (a) make the one escaping link conformant and (b)
   take the directive's encouraged Part 2 path — move the per-flag CLI depth into a pulled
   `docs/site/` tree (`install.md`, `workflows.md`) and link into it *naturally* (`docs/site/<p>.md`,
   which the site rewrites to `/tools/wt/<p>`). This lights up real depth on the site instead of
   merely deleting a link, and keeps `docs/specs/` (a fab pre-implementation artifact) where it
   belongs — un-pulled.

## What Changes

### Per-tool table row (from the contract)

| Field | Value |
|-------|-------|
| Repo (file slug) | `wt` |
| Binary | `wt` |
| `content/<slug>/` collector | `content/wt/` (shll.ai side — not touched here) |
| URL space | `/tools/wt/` |
| Reserved static slugs (do NOT name a `docs/site/` page these) | `overview`, `readme`, `commands` |
| `install` / `workflows` | **NOT reserved** — owned by this repo via `docs/site/install.md` / `docs/site/workflows.md` |

### Part 1 — `README.md` restructure

- **Head (§1)** — *already conformant; preserve exactly.* Top is `# wt` H1 → canonical toolkit
  blockquote (`> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.`,
  exact text, `https://shll.ai` not `ai.shll.in`) → contiguous badge row → prose tagline. No
  frontmatter, no leading HTML comment, no `<h1>`. **No change needed** — verify only.
- **Tail (§2) — fix the trailing footer.** The current README ends (after `---`) with a repeated
  toolkit blockquote + "Originally extracted from fab-kit…" line. There is no denylisted heading,
  so the slice runs to EOF and this footer pulls onto the site as orphan chrome. **Remove the
  trailing `---` + toolkit-footer block** so the slice ends on real content (the `## Gotchas`
  section). The toolkit framing already lives in the head blockquote (which is skipped on the site
  but read on GitHub), so the footer repeat is redundant for both audiences once the head is intact.
  The fab-kit-origin note moves into `docs/site/install.md` where it is site-relevant context.
- **Images (§3)** — only the three badge images exist, all already absolute `https://img.shields.io/…`.
  **No change.**
- **Mermaid (§5)** — none in the README. **No change** (no rendered image to commit).
- **Site-escaping relative links (§9.1.2 — the core fix).** Line 77 currently reads:
  `see [`docs/specs/cli-surface.md`](docs/specs/cli-surface.md) for the full per-flag reference`.
  This relative link to `docs/specs/` 404s on the site. **Replace it** with a *natural* link into
  the new `docs/site/` tree: `see the [full command reference](docs/site/install.md#command-reference)`
  / a dedicated reference page — the site rewrites `docs/site/<p>.md` → `/tools/wt/<p>` automatically
  (directive rule 4). The in-page `[Gotchas](#gotchas)` anchor is an intra-slice anchor and is fine
  as-is.
- **gh theme tricks (§4/§6)** — none. **No change.**
- **Command/flag accuracy (§7 — report-only).** The README's command/flag examples
  (`wt create/list/open/delete/init/shell-init`, flags `--base/--reuse/--worktree-name/--non-interactive/--status/--path/--json/--app`)
  are sanity-checked against the actual cobra surface. Report-only on the site; verified here so no
  `::warning::` is produced.

### Part 2 — `docs/site/` tree (the encouraged depth)

Author two pages following the four closed-set rules (§9.1):

- **`docs/site/install.md`** — the full install guide: Homebrew, manual (`just local-install`), the
  `eval "$(wt shell-init)"` shell-wrapper step, the `shll shell-install` cross-tool note, and the
  fab-kit-origin context. Renders at `/tools/wt/install`.
- **`docs/site/workflows.md`** — the deeper usage/command material currently crammed into the README
  (full per-flag command reference, the `--base` start-point table, the `wt open` context-aware
  launcher matrix, gotchas-in-depth). Renders at `/tools/wt/workflows`.

Closed-set rules applied to both pages:
1. **Closure** — every relative link/image stays inside `docs/site/`. Cross-page links use bare
   relative `.md` targets (`[workflows](./workflows.md)`), which the site resolves intra-set.
2. **External links absolute-by-author** — any link leaving the rendered set (GitHub repo, releases,
   `docs/specs/`, fab-kit) is written as an absolute `https://…` URL by hand.
3. **All images absolute** — N/A (no images planned); if any are added they are absolute.
4. **README → `docs/site/` links written naturally** — the README links in as `docs/site/<p>.md`.

Naming: neither page is `overview` / `readme` / `commands` (reserved). `install` / `workflows` are
explicitly the repo's to own. ✅

### Out of scope

- shll.ai itself — it already pulls + renders; we touch nothing there.
- `docs/specs/`, `docs/memory/`, source, tests — never pulled; left untouched (the cli-surface spec
  stays as the fab design artifact; the site now reads `docs/site/` instead).

## Affected Memory

<!-- This is a pure documentation/structure change to README.md + new docs/site/ pages.
     No spec-level behavior of the wt binary changes, so no docs/memory updates are warranted. -->

- *(none)* — no spec-level behavior change; README + `docs/site/` are documentation surfaces only.

## Impact

- **`README.md`** — remove trailing footer block; rewrite the line-77 relative spec link into a
  natural `docs/site/` link. Head/badges/images unchanged.
- **`docs/site/install.md`** (new), **`docs/site/workflows.md`** (new) — depth pages.
- **No code, no tests, no CI.** The `wt` binary, cobra commands, and `docs/specs/` are untouched.
- **External dependency**: shll.ai's daily pull (out of our control, additive — conforming only
  lights up the pages; deferring would leave a neutral placeholder).

## Open Questions

<!-- None blocking. The directive + contract are explicit; the per-tool table fixes slug and
     reserved names; the gap analysis fixed the concrete deltas. -->

- *(none blocking)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Slug `wt`; reserved page names `overview`/`readme`/`commands`; `install`/`workflows` are the repo's to own | Read directly from the contract's per-tool table (the `wt` row) | S:98 R:90 A:98 D:95 |
| 2 | Certain | Head already conformant — preserve `# wt` → canonical toolkit blockquote → badges → prose; no change | Verified the live README head matches §1 + directive rule 1 verbatim (exact blockquote, `https://shll.ai`) | S:95 R:85 A:95 D:92 |
| 3 | Certain | Fix the line-77 relative `docs/specs/cli-surface.md` link — it 404s on the site (not pulled, not rewritten) | §9.1.2 / directive rule 5 state relative non-`docs/site` links render as live 404s with no warning | S:95 R:75 A:95 D:88 |
| 4 | Confident | Route the per-flag depth into a `docs/site/` tree (`install.md` + `workflows.md`) rather than just rewriting the dead link to an absolute GitHub blob URL | Directive Part 2 explicitly encourages this and names these two pages; richer site result than a blob link; keeps `docs/specs/` un-pulled | S:80 R:70 A:80 D:65 |
| 5 | Confident | Remove the trailing `---` + toolkit-footer blockquote so the slice ends on real content | No denylisted heading exists, so the footer pulls as orphan chrome (§2); the head blockquote already carries the toolkit framing | S:78 R:80 A:85 D:75 |
| 6 | Confident | No memory/spec updates — this is a docs/structure change with no `wt` behavior change | Constitution + config scope memory to spec-level behavior; README/docs/site are doc surfaces | S:82 R:85 A:88 D:80 |
| 7 | Tentative | Split depth as install.md (install + shell-init + cross-tool note) vs workflows.md (command reference + `--base` table + `wt open` matrix + gotchas) | The directive names both pages but does not prescribe which content lands where; this split is a reasonable default, adjustable in apply <!-- assumed: install.md vs workflows.md content boundary — directive names the pages but not their exact contents --> | S:55 R:65 A:60 D:50 |

7 assumptions (3 certain, 3 confident, 1 tentative, 0 unresolved). Run /fab-clarify to review.
