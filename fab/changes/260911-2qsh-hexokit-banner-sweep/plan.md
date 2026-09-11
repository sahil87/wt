# Plan: HexoKit Banner Sweep (wt)

**Change**: 260911-2qsh-hexokit-banner-sweep
**Intake**: `intake.md`

## Requirements

### README: Toolkit blockquote

#### R1: The README carries the revised mandated toolkit blockquote
`README.md` line 3 SHALL be exactly `> Part of [HexoKit](https://hexokit.com) — see all projects there.` (em-dash U+2014, no leading "the"), replacing the pre-C1 line `> Part of the [shll toolkit](https://shll.ai) — see all projects there.` — the text the `readme-extraction` standard's rule 1 mandates after shll change `ttoa` (PR shll#98).

- **GIVEN** the README head is `# wt` → blockquote → badge run → tagline
- **WHEN** the blockquote line is replaced
- **THEN** `sed -n 3p README.md` prints the revised line byte-for-byte
- **AND** no other line of `README.md` changes

#### R2: The README stays conformant with the `readme-extraction` standard
After the edit, the standard's § Verifying conformance checklist SHALL run clean: head order `#` H1 → toolkit blockquote → contiguous badges → prose; every relative link target points into `docs/site/`; no relative images; no `#gh-*-mode-only` fragments; no ```` ```mermaid ```` fences; no `docs/site/` page named `overview`/`readme`/`commands`; the README cross-links `docs/site/install.md`, `docs/site/workflows.md`, and the absolute command-reference URL `https://shll.ai/wt/commands/`.

- **GIVEN** the edited `README.md` and the unchanged `docs/site/` tree
- **WHEN** the grep checklist is executed
- **THEN** each check reports only conformant results and the head order is H1, blockquote, badges, prose

### Scope fence: substrate, historical, and site-naming surfaces untouched

#### R3: The diff contains only the blockquote edit and pipeline artifacts
The change's diff against `main` MUST touch only `README.md` (one line), `fab/changes/260911-2qsh-hexokit-banner-sweep/**`, and — at hydrate — `docs/memory/**`. In particular the following MUST be unchanged: install URLs `https://shll.ai/install` (README, `docs/site/install.md`); the command-reference URL `https://shll.ai/wt/commands/`; the "shll toolkit" / "shll tools" / "shll meta-CLI" prose; every `run-kit` occurrence in `docs/memory/`, `fab/`, and the `src/cmd/wt/skill_test.go:97` comment; `docs/site/**`, `docs/specs/**`, `src/**`.

- **GIVEN** the repo-wide grep for `run-kit|runkit|hexokit|\brk\b` (excluding `.git`, `.agents/`, `.claude/`, `fab/changes/archive/`)
- **WHEN** compared before and after apply
- **THEN** the only new `hexokit` hit is `README.md:3`, and no `run-kit` hit count changes
- **AND** `git diff --name-only main...HEAD` (excluding `fab/changes/260911-2qsh-*`) lists only `README.md` before hydrate

### Non-Goals

- Flipping "shll toolkit" / "shll tools" prose to "HexoKit toolkit" — outside plan row C7; hexokit.com is unannounced until X1 (intake Assumption 7).
- Rewiring install URLs to `hexokit.com/install` — S5 unmerged; D4/X2 keep `shll.ai/install` live.
- Renaming `run-kit` in `docs/memory/` narrative or `fab/` archives — D11 (historical tier); X3 owns the memory/specs identity sweep after R1/R2.
- Any code, test, help-output, or `docs/site/` change.

### Design Decisions

#### Revised standard text sourced from the C1 PR branch at apply time; receipt cites the release
**Decision**: At apply time (intake-time snapshot) the mandated blockquote was taken verbatim from shll PR #98's diff to `docs/site/standards/readme-extraction.md` (shll change `ttoa`), because the installed `shll standards readme-extraction` (v0.1.30) printed the pre-C1 line and #98 was unreleased. By review-pr time #98 had merged and shipped as **shll v0.1.31**; the conformance receipt therefore cites v0.1.31 and the tagged standard text (identical line), and notes that a local binary older than the latest release is not the standard.
**Why**: The rebrand plan gates C7 on C1 being *up and reviewed*, not released; the blockquote text has no pipeline coupling (the consuming site's extractor matches any leading blockquote), so starting before the release was safe — and the release then landed within the same day.
**Rejected**: Waiting for a shll release before editing satellite READMEs (would have delayed X1 for no correctness gain); recording the PR as the audited revision once a release exists (a release version is the durable citation the receipt's version-record requirement asks for).
*Introduced by*: 260911-2qsh-hexokit-banner-sweep

## Tasks

### Phase 2: Core Implementation

- [x] T001 Replace `README.md` line 3 with `> Part of [HexoKit](https://hexokit.com) — see all projects there.` (exact text; em-dash; nothing else in the file changes) <!-- R1 -->

### Phase 3: Integration & Edge Cases

- [x] T002 Run the `readme-extraction` § Verifying conformance grep checklist against `README.md` + `docs/site/` (head order, relative-link targets, relative images, `#gh-*`, mermaid, reserved names, cross-links) and run `go test ./...` from `src/` as the regression baseline; record both receipts under `## Notes` <!-- R2 -->
- [x] T003 Re-run the repo-wide `run-kit|runkit|hexokit|\brk\b` grep and `git diff --name-only main...HEAD`; confirm the only new hit is `README.md:3` and the only non-`fab/changes/` file in the diff is `README.md`; record the receipt under `## Notes` <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `sed -n 3p README.md` prints exactly `> Part of [HexoKit](https://hexokit.com) — see all projects there.`
- [x] A-002 R2: The `readme-extraction` verifying-conformance checklist runs clean against the edited README (receipt recorded in `## Notes`)
- [x] A-003 R3: `git diff --name-only main...HEAD` outside `fab/changes/` and `docs/memory/` lists only `README.md`

### Behavioral Correctness

- [x] A-004 R1: `git diff main...HEAD -- README.md` shows exactly one removed and one added line (the blockquote); the H1, badge run, and tagline are byte-identical

### Scenario Coverage

- [x] A-005 R3: The pre-C1 line `Part of the [shll toolkit]` no longer occurs anywhere in `README.md`, and `hexokit` occurs in `README.md` only on line 3
- [x] A-006 R3: Install URLs (`https://shll.ai/install`), the command-reference URL (`https://shll.ai/wt/commands/`), and every `run-kit` occurrence in `docs/memory/`, `fab/`, and `src/cmd/wt/skill_test.go` are unchanged

### Edge Cases & Error Handling

- [x] A-007 R1: The blockquote uses U+2014 (em-dash), not `--` or a hyphen — `grep -c ' — see all projects there\.$' README.md` is 1

### Code Quality

- [x] A-008 Pattern consistency: The README head keeps the standard's exact ordering (H1 → blockquote → badges → prose) with no blank-line or whitespace drift
- [x] A-009 No unnecessary duplication: No second banner, note, or redirect line was added alongside the blockquote

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

### Apply receipts (2026-09-11)

- **T001** — `README.md:3` now reads `> Part of [HexoKit](https://hexokit.com) — see all projects there.`; `git diff --stat -- README.md` = 1 insertion, 1 deletion.
- **T002 checklist** — head order H1 → blockquote → badges → tagline ✓; relative link targets: `README.md:52` → `docs/site/install.md`, `README.md:88` → `docs/site/workflows.md` (both into `docs/site/`) ✓; relative images: none ✓; `#gh-*-mode-only`: none in README or `docs/site/` ✓; mermaid fences: none ✓; reserved `docs/site/` names (`overview`/`readme`/`commands`): none ✓; cross-links present: `docs/site/install.md`, `docs/site/workflows.md`, `https://shll.ai/wt/commands/` ✓; em-dash line count 1 ✓; pre-C1 banner occurrences 0 ✓.
- **T002 test baseline** — `go test ./...` (from `src/`): `internal/update` ok, `internal/worktree` ok, `cmd/wt` FAIL on `TestCreate_InitFailureInteractive_OpenAnyway` only (`create_init_failure_pty_test.go:111`, expected output to contain "cd needs the shell wrapper"). **Pre-existing and unrelated**: the identical failure reproduces on the `main` checkout with no README change; the test exercises a PTY "open anyway" hint path, which reads nothing from `README.md`. Not addressed by this change (out of scope — a docs-only row); flagged for the user.
- **T003 scope fence (apply-time tree, before hydrate)** — repo-wide grep: the only `hexokit` hit outside this change folder is `README.md:3`; `run-kit` hit count excluding this change folder is unchanged at 38 (all `docs/memory/`, `fab/`, and the `skill_test.go:97` comment); `git diff --name-only main -- . ':!fab/changes'` = `README.md` only. **Post-hydrate**: the receipt in `docs/memory/wt-cli/toolkit-standards-conformance.md` intentionally adds `HexoKit` (the audited blockquote text) and one `sahil87/run-kit` repository reference (the rebrand plan's home — a repo link, which R2 of that plan keeps as-is); no pre-existing `run-kit` mention was altered. R3's fence covers content edits to existing mentions, not the receipt's own citations.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Verification is the standard's own grep checklist plus a `go test ./...` baseline; no new test is added to pin the banner | No existing test pins README prose; a banner test would couple the Go suite to a cross-repo standard's wording | S:85 R:95 A:95 D:90 |
| 2 | Certain | `docs/memory/` is touched only at hydrate (the `toolkit-standards-conformance` receipt), never during apply | Memory is hydrate's output; the apply diff is `README.md` alone | S:90 R:95 A:95 D:95 |

2 assumptions (2 certain, 0 confident, 0 tentative).
