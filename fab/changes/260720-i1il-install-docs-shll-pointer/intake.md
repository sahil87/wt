# Intake: Conform Install Docs to Install-Composition Policy B

**Change**: 260720-i1il-install-docs-shll-pointer
**Created**: 2026-07-20

## Origin

One-shot `/fab-new` invocation:

> Conform this repo's install documentation to the shll toolkit's install-composition standard, Policy B. Read the authoritative standard first: /home/sahil/code/sahil87/shll/docs/site/standards/install-composition.md (rendered on https://shll.ai). Policy B: per-tool READMEs and doc pages must not carry per-formula "brew install sahil87/tap/<tool>" install instructions; installation points to https://shll.ai (curl bootstrap: curl -fsSL https://shll.ai/install | sh; subset installs remain supported via shll install <tool>). Task: audit README.md and docs/site/ for per-formula install instructions and replace them with the shll.ai pointer. IMPORTANT distinction: replace install *instructions* (sections telling the user how to install), but KEEP incidental mentions such as actionable error-hint examples in standards/conformance text (Policy A mandates those hints) and historical/changelog references. Mechanical docs-only change; keep all usage and feature content intact.

The intake-stage agent read the authoritative standard file and completed the audit of `README.md` and `docs/site/` before generating this intake — the findings below are verified against the working tree, not assumed.

## Why

1. **Problem**: The shll toolkit's `install-composition` standard (Policy B) centralizes install documentation on https://shll.ai. Per-tool repos must not carry per-formula `brew install sahil87/tap/<tool>` instructions — seven copies of the install dance drift, and every change to the install story (a tap-trust requirement, a bootstrap change) has to be chased across every repo plus the tap. This repo's `docs/site/install.md` still opens with a "Homebrew (preferred)" section whose body is exactly the prohibited per-formula instruction (`brew install sahil87/tap/wt`).
2. **Consequence if unfixed**: `wt` violates a published toolkit standard that the constitution's **Toolkit Standards** article binds it to ("Standards added or revised there bind this repo without further amendment"). The install story documented on the wt doc site can drift from the canonical shll.ai one.
3. **Why this approach**: The standard itself prescribes the fix — the install section links to https://shll.ai (curl bootstrap / `shll install`) instead of carrying per-formula brew lines. Individual formula installs remain *supported*; only *documenting* them per-repo is prohibited.

## What Changes

### Audit result (verified against the working tree)

Full-tree grep over `README.md` and `docs/site/` for `brew install`, `sahil87/tap`, and install-section content found exactly three hits:

| Location | Content | Verdict |
|----------|---------|---------|
| `docs/site/install.md:7-16` | `## Homebrew (preferred)` section: fenced `brew install sahil87/tap/wt` block + tap-formula link + upgrade note | **VIOLATION — replace** (this is an install instruction section) |
| `README.md:13-23` | `## Install` section: `curl -fsSL https://shll.ai/install \| sh -s -- wt` (subset) and `curl -fsSL https://shll.ai/install \| sh` (full toolkit) | **Already conformant — no change** (links to shll.ai, no per-formula brew line; satisfies the standard's verification bullet) |
| `docs/site/workflows.md:117-119` (`### wt update` section) | "If `wt` was installed via `just local-install` … `wt update` reports that and tells you to reinstall with `brew install sahil87/tap/wt`" | **KEEP** — documents the binary's own actionable missing/mismatch hint (the Policy A-mandated hint shape); this is behavior documentation, not an install instruction |

`docs/site/skill.md` carries no install instructions. The README's "Other ways to install" section (manual `git clone` + `just local-install` build) is a manual-build path, not a per-formula brew instruction — Policy B does not prohibit it, and `docs/site/install.md` keeps its equivalent "Manual" section for the same reason.

### `docs/site/install.md` — replace the Homebrew section (the only edit)

Replace lines 7–16 (the `## Homebrew (preferred)` section) with a shll.ai-pointer section mirroring the README's already-conformant install copy:

````markdown
## Via shll.ai (preferred)

```bash
curl -fsSL https://shll.ai/install | sh -s -- wt
```

Installs wt (plus the shll meta-CLI) via Homebrew, handling tap trust
automatically. To install the entire [shll toolkit](https://shll.ai) instead,
drop the `-s -- wt` suffix; if you already have the `shll` meta-CLI,
`shll install wt` does the same thing. For the full install story, see
[https://shll.ai](https://shll.ai). To upgrade later, `wt update` self-updates
via Homebrew (see the [workflows reference](./workflows.md#wt-update)).
````

Preserved from the old section: the upgrade pointer (`wt update` self-updates via Homebrew → workflows reference). Dropped: the `brew install sahil87/tap/wt` fenced block and the direct `sahil87/homebrew-tap` formula link (the per-formula documentation Policy B prohibits).

Everything else in `install.md` stays intact: the intro, the Manual (Go + `just`) section — including its "wt update will tell you to reinstall via `brew`" sentence, which is behavior documentation and mentions no formula — the Shell wrapper section, the `shll shell-install` cross-tool note, the "Where wt came from" history, and Next steps.

### No other file changes

- `README.md` — no change (already conformant).
- `docs/site/workflows.md` — no change (the `brew install sahil87/tap/wt` mention at line 119 documents the `wt update` binary's own reinstall hint; the task's carve-out and Policy A's mandated-hint shape both say keep it).
- `docs/site/skill.md` — no change (no install content).
- No source-code changes: the binary's own hint strings (e.g., `wt update`'s reinstall message) are Policy A territory and explicitly out of scope for this docs-only change.

## Affected Memory

- `wt-cli/toolkit-standards-conformance`: (modify) Record the `install-composition` standard's verdict for `wt` — Policy B PASS after this fix (docs/site/install.md was the only violation; README already conformant; workflows.md's mention is the documented binary hint, kept per Policy A). Note this as a post-v0.1.7 standard addition landing via the re-audit trigger, citing this change.

## Impact

- **Files**: `docs/site/install.md` (one section replaced). `docs/memory/wt-cli/toolkit-standards-conformance.md` at hydrate.
- **Code**: none — docs-only. No CLI surface, help output, or test changes.
- **Standards cross-check (re-audit trigger)**: the edit touches `docs/site/`, so the `readme-extraction` standard's docs/site-closure rules apply — the replacement section keeps all links relative-or-absolute exactly as the standard requires (shll.ai links absolute, intra-site links relative), and removes no anchor that other pages link to (`workflows.md` links to `./install.md#shell-wrapper-enables-open-here` and `./install.md` — both unaffected; nothing links to the `#homebrew-preferred` anchor).
- **Risk**: minimal — mechanical docs edit, individual formula installs remain functional and supported.

## Open Questions

None — the task statement, the authoritative standard, and the completed audit resolve all decision points.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `docs/site/install.md`'s Homebrew section is the only violation to fix; README.md is already conformant and stays untouched | Verified by grep + reading both files: README's Install section already carries the shll.ai curl bootstrap and no per-formula brew line, exactly matching the standard's verification bullet | S:90 R:90 A:95 D:90 |
| 2 | Certain | Keep `docs/site/workflows.md:119`'s `brew install sahil87/tap/wt` mention | User instruction explicitly carves out actionable error-hint examples; the line documents the `wt update` binary's own reinstall hint, which Policy A mandates in that exact shape | S:95 R:90 A:95 D:90 |
| 3 | Confident | Replacement section mirrors README's install copy (subset curl bootstrap first) and adds `shll install wt` as the subset alternative, retitled `## Via shll.ai (preferred)` | Keeps the two conformant surfaces (README, install.md) telling one story; the standard names both the curl bootstrap and `shll install` as the pointer targets; exact wording is easily revised | S:70 R:90 A:80 D:65 |
| 4 | Certain | Keep the rest of install.md intact (Manual build, Shell wrapper, history, Next steps) rather than reducing the page to a bare shll.ai link | Task says keep all usage/feature content; Policy B prohibits only per-formula brew instructions, and the Manual section documents a build-from-source path, not a formula | S:80 R:85 A:85 D:75 |
| 5 | Certain | Hydrate records the install-composition verdict inside the existing `toolkit-standards-conformance` memory file rather than a new file | That file is the audit receipt by its own design decision ("this file is the audit receipt: which standards, which version, PASS/gap + the fixing change"); no new behavior contract is created by a docs edit | S:75 R:90 A:85 D:80 |

5 assumptions (4 certain, 1 confident, 0 tentative, 0 unresolved).
