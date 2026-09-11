# Intake: HexoKit Banner Sweep (wt)

**Change**: 260911-2qsh-hexokit-banner-sweep
**Created**: 2026-09-11

## Origin

> Per fab/plans/sahil/26-09-10-hexokit-rebrand.md row C7 (hexokit-banner-sweep), applied to the wt repo, gated on C1 (shll change ttoa, PR shll#98 up, review-pr done): apply the readme-extraction standard's revised mandated blockquote to this repo's README (-> "Part of [HexoKit](https://hexokit.com) -- see all projects there"); flip any PRESENT-TENSE "run-kit" PRODUCT mentions (prose referring to the dashboard product, not the `rk` binary/verbs) to HexoKit. The `rk`/`run-kit` SUBSTRATE (binary name, verbs, options) is UNTOUCHED. Repo links stay as-is until R2. Read the plan doc's Decision log (D1-D14) and row C7 first; run `shll standards` and read `readme-extraction` before editing.

**Interaction mode**: one-shot `/fab-new`, autonomous session (no user reachable mid-task). No prior in-conversation discussion; the design authority is the cross-repo plan document, read in full at intake time:

- **Plan doc**: `fab/plans/sahil/26-09-10-hexokit-rebrand.md` in the **run-kit** repo (`/home/sahil/code/sahil87/run-kit/…`) — Decision log D1–D14, Naming tiers, row C7, Pickup protocol.
- **Gate C1**: shll fab change `ttoa` (`260911-ttoa-hexokit-banner-and-policy`), PR [shll#98](https://github.com/sahil87/shll/pull/98) — an open draft with review-pr done when this intake was written (2026-09-11), **merged 17:51Z and released as shll v0.1.31 at 17:53Z the same day** (confirmed at review-pr time). Its diff to `docs/site/standards/readme-extraction.md` is the source of the revised blockquote text (see What Changes); the tagged `v0.1.31` file carries the identical line.
- **Installed `shll`** (v0.1.30; `shll standards readme-extraction`) prints the *pre-C1* blockquote (`> Part of the [shll toolkit](https://shll.ai) — see all projects there.`) — at intake time because shll#98 was unreleased, and at review-pr time because the local binary lags the v0.1.31 release. The user's gate wording ("PR up, review-pr done") made the PR-branch text authoritative for this change — recorded as Assumption 2; the released `v0.1.31` text confirms it.

Key decisions carried from the plan (do not re-open — plan § Pickup protocol #1): D1 (HexoKit names the dashboard product; companions incl. `wt` keep their names), D2 (binary stays `rk`; substrate untouched), D11 (historical text — `fab/` archives, `docs/memory/` narrative, git history — is not renamed), D14 (standards not renamed; C1 edits the blockquote content only; site-naming `shll.ai` mentions wait for X4).

## Why

1. **The pain point.** The first line under the H1 on every toolkit repo page is the mandated toolkit blockquote. After C1 it reads "Part of HexoKit" in the standard and in shll's own README, while `wt`'s README still says "Part of the shll toolkit" — the exact two-brand split the rebrand exists to remove (plan D14: "leaving it reintroduces the two-brand split for a saving of seven lines"). The blockquote is the most visible cross-repo brand surface, so a stale copy on `wt` undercuts the rename on a page users actually land on.
2. **Consequence of not fixing.** `wt` falls out of conformance with the `readme-extraction` standard's rule 1 the moment shll#98 merges (the standard mandates "this exact line in all seven repos"), and the constitution's **Toolkit Standards** article binds this repo to revised standards "without further amendment". The `toolkit-standards-conformance` memory's re-audit trigger also fires on any README edit, so the receipt would silently go stale. Phase 2's X1 depends on "C3, C7 merged" — an un-swept satellite blocks the site cutover.
3. **Why this approach.** Content-only, one row of the plan, scoped exactly as the plan's C7 row: banner via the standard + present-tense `run-kit` product mentions → HexoKit. No repo-link changes (R2), no install-URL rewiring (S5 unmerged; D4 keeps `shll.ai/install` live forever), no "shll toolkit" prose sweep (that pass is X4's domain for the standards and is not in C7's scope for satellites). Alternatives rejected: (a) waiting for shll#98 to merge and release before touching `wt` — the banner text is pipeline-free (the consuming site's extractor matches *any* leading blockquote, D14), so ordering buys nothing and delays X1; (b) sweeping every `shll.ai` / "shll toolkit" mention now — hexokit.com is unannounced until X1 and the plan deliberately keeps that window (plan § Risks, "Two-site window").

## What Changes

### 1. README blockquote (the one mandated edit)

`README.md` line 3 changes from the pre-C1 line to the revised mandated line. The exact new text is taken verbatim from shll#98's diff to `docs/site/standards/readme-extraction.md` (em-dash, not `--`; the `--` in the invocation is terminal rendering):

```diff
 # wt

-> Part of the [shll toolkit](https://shll.ai) — see all projects there.
+> Part of [HexoKit](https://hexokit.com) — see all projects there.

 [![Latest release](https://img.shields.io/github/v/release/sahil87/wt)](…) …
```

Head order stays `#` H1 → blockquote → contiguous badge run → tagline prose (rule 1). Nothing else in the head moves.

### 2. Present-tense `run-kit` product mentions → HexoKit: **none on live surfaces**

The repo-wide grep (`run-kit|runkit|hexokit|\brk\b`, excluding `.git`, `.agents/`, `.claude/`, `fab/changes/archive/`) was run at intake. Every hit is out of scope by plan tier; **no prose flip is required in this repo**. Dispositions, so apply does not re-derive them:

| Location | Hits | Tier / disposition |
|----------|------|--------------------|
| `README.md`, `docs/site/{install,skill,workflows}.md`, `docs/specs/*.md`, `src/cmd/wt/skill.md` | 0 `run-kit` | Live surfaces — nothing to flip |
| `docs/memory/wt-cli/open-list-contract.md` (6×), `docs/memory/wt-cli/skill-command-contract.md:28` | narrative naming run-kit as the `GET /api/open-apps` consumer / "contrast run-kit's dynamic `context`" | **Historical** (D11: `docs/memory/` narrative is not renamed; X3 owns the memory/specs identity sweep after R1/R2) — leave |
| `fab/backlog.md` `[qj66]`, `fab/changes/*/{intake,plan}.md`, `.history.jsonl` | archives / plans | **Historical** (D11) — leave |
| `src/cmd/wt/skill_test.go:97` — comment `// dynamic, environment-derived content (contrast run-kit context).` | 1 | Names the `rk context` verb (substrate tier, D2) inside a test comment — not a brand surface — leave |

`rk`, `RK_*`, `@rk_*`, `rk-*` identifiers: zero occurrences in this repo; nothing to protect, and nothing is renamed (plan § Pickup protocol #3).

### 3. Explicitly unchanged (scope fence for apply and review)

- **Install URLs** `https://shll.ai/install` (README `## Install`, `docs/site/install.md` `## Via shll.ai (preferred)`): unchanged. S5 (`hexokit.com/install`) is in progress and unmerged; D4/X2 keep `shll.ai/install` live as a byte copy forever. C7's scope does not include install rewiring.
- **Command-reference link** `https://shll.ai/wt/commands/` (README, rule 8 of the standard): unchanged — C1 did not touch rule 8; the site-naming flip is X4.
- **"shll toolkit" / "shll tools" / "shll meta-CLI" prose** (README `## Install` and the 💡 tip; `docs/site/install.md` §§ "Via shll.ai", "Already use other shll tools?"): unchanged — outside C7's row scope. Recorded as Assumption 7 so `/fab-clarify` can flip it if the user wants the satellite prose swept in this PR.
- **Repo links** — this repo has no `sahil87/run-kit` links; the R2 fence is moot here.
- **Code, tests, help output, `docs/site/skill.md` / `src/cmd/wt/skill.md`**: untouched. No test pins the README banner (grep of `src/`, `scripts/`, `justfile`, `.github/` for `README|Part of|shll toolkit` finds only unrelated fixture writes), so no test changes.

### 4. Conformance verification (the standard's own checklist — apply runs it, review re-runs it)

Per the `readme-extraction` standard § Verifying conformance, against the revised rule 1:

- README top is `#` H1 → **the revised** toolkit blockquote → badges; first prose line is the tagline ("A small CLI that wraps `git worktree` …").
- `grep -nE '\]\(\./|\]\(\.\./|\]\(docs/' README.md` — each relative target points into `docs/site/` (`docs/site/install.md`, `docs/site/workflows.md`); no relative images.
- No `#gh-*-mode-only` fragments; no ```` ```mermaid ```` fences; no `docs/site/` page named `overview`/`readme`/`commands`.
- README cross-links its `docs/site/` pages and the absolute command-reference URL.

Expected: the checklist runs clean with only the blockquote diff.

### 5. Cross-repo bookkeeping (out-of-repo, not part of this repo's diff)

Plan § Pickup protocol #4: "Update the Status line and your row when you create/merge a change." The C7 row's Status cell in `run-kit`'s `fab/plans/sahil/26-09-10-hexokit-rebrand.md` gets a `wt:` entry naming this change (`2qsh`) at intake, and the PR URL at ship. This is an edit in the run-kit checkout's working tree, left uncommitted for the user (that repo's commits are not this change's to make). Recorded as Assumption 9.

## Affected Memory

- `wt-cli/toolkit-standards-conformance`: (modify) `readme-extraction` verdict re-confirmed against the **revised** rule 1 (blockquote names HexoKit — C1 / shll#98, shll change `ttoa`; unreleased at apply time, so the receipt records the PR/change rather than a shll release version). Add this change to the re-audit-trigger narrative as the trigger firing on a README edit driven by an upstream *content revision* of an already-audited standard (a third shape, after "new standards published" and "new standard bound on a docs/site edit"). Cross-reference the HexoKit rebrand plan (run-kit repo) and D11/D14 as the scope fence (why `docs/memory/` `run-kit` narrative is intentionally not renamed here).

No other memory file changes: no CLI behavior, output, exit code, or docs/site contract moves.

## Impact

- **Files changed in this repo**: `README.md` (1 line). Hydrate: `docs/memory/wt-cli/toolkit-standards-conformance.md` (+ regenerated `docs/memory/wt-cli/index.md` via `fab docs-index`; `docs/memory/wt-cli/log.md` is derived from git history and did not change on this branch).
- **Files changed outside this repo**: run-kit `fab/plans/sahil/26-09-10-hexokit-rebrand.md` C7 row Status cell (bookkeeping, uncommitted).
- **Code / tests / CI**: none. `go test ./...` is a no-op for this change (nothing in `src/` moves); run it anyway as the review's regression baseline.
- **Published surfaces**: shll.ai `/wt/readme` re-pulls the README daily; the extractor matches any leading blockquote, so the slice shape is unchanged and the new banner text simply flows through (D14). hexokit.com `/wt/readme` (S3 pipeline) picks it up the same way.
- **Dependencies / ordering**: gated on C1 being *up and reviewed*, not merged/released (Assumption 2). Nothing in this repo breaks if shll#98 merges before or after this PR. Phase 2's X1 depends on this row merging (all C7 satellites).
- **Risk**: near zero — a one-line prose change on a surface with no machine consumer that parses the text. Reverting is a one-line change.

## Open Questions

- None blocking. The one judgment call — whether to also flip the README / `docs/site/install.md` "shll toolkit" prose to "HexoKit toolkit" in this PR — is recorded as Assumption 7 (left unchanged per the C7 row's scope) and can be reversed via `/fab-clarify`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New blockquote is exactly `> Part of [HexoKit](https://hexokit.com) — see all projects there.` (em-dash, no "the") | Verbatim from shll#98's diff to `docs/site/standards/readme-extraction.md`; identical to plan D14/C1 wording and shll's own README on that branch. The `--` in the invocation is terminal rendering of the em-dash | S:95 R:95 A:95 D:95 |
| 2 | Confident | The revised standard text is taken from the shll#98 PR branch, not from the installed `shll standards readme-extraction` (v0.1.30, which prints the pre-C1 line) | User's gate is "PR up, review-pr done", not "merged/released". Banner text is pipeline-free (D14), so ordering is safe. Superseded in effect at review-pr time: #98 merged and shipped as shll v0.1.31 the same day, so the receipt cites v0.1.31 and the tagged standard text | S:80 R:90 A:70 D:70 |
| 3 | Certain | No present-tense `run-kit` product mention exists on this repo's live surfaces (README, docs/site, docs/specs, skill bundle); no prose flip beyond the banner | Repo-wide grep at intake; every hit is `docs/memory/` narrative, `fab/` archives, or a substrate-verb test comment (D2/D11 tiers — leave) | S:90 R:95 A:95 D:90 |
| 4 | Certain | Install URLs (`https://shll.ai/install`) unchanged | S5 (`hexokit.com/install`) unmerged; D4/X2 keep `shll.ai/install` live forever; C7 scope excludes install rewiring | S:85 R:95 A:90 D:90 |
| 5 | Certain | Command-reference link `https://shll.ai/wt/commands/` unchanged | Standard rule 8 not touched by C1 (verified in the PR diff); site-naming flips are X4 | S:85 R:95 A:90 D:90 |
| 6 | Certain | No repo-link edits (R2 fence) | This repo has zero `sahil87/run-kit` links; nothing to defer | S:90 R:95 A:95 D:95 |
| 7 | Confident | "shll toolkit" / "shll tools" / "Via shll.ai" prose in README and `docs/site/install.md` left unchanged in this PR | C7 row scope = banner + `run-kit` product mentions; the "shll toolkit → HexoKit toolkit" flip is X4 for the standards and unassigned for satellites; hexokit.com is unannounced until X1. Trivial to flip later; flagged for `/fab-clarify` | S:80 R:90 A:65 D:60 |
| 8 | Certain | Affected memory = `wt-cli/toolkit-standards-conformance` (modify) only; change type `docs` | The receipt's own re-audit trigger names README edits; no behavior contract moves. `fab status refresh` keyword inference should land on `docs` — verified after creation | S:85 R:90 A:90 D:90 |
| 9 | Certain | Update the run-kit plan doc's C7 row Status cell (uncommitted edit in that checkout) at intake and again at ship | Plan § Pickup protocol #4 asks for it explicitly; cross-repo, so this change does not commit it — the user commits the run-kit side | S:75 R:95 A:80 D:75 |
| 10 | Certain | No test or CI change; verification is the standard's own grep checklist + a `go test ./...` baseline | No test pins the README banner (grep of `src/`, `scripts/`, `justfile`, `.github/`) | S:85 R:95 A:95 D:95 |

10 assumptions (8 certain, 2 confident, 0 tentative, 0 unresolved).
