# Intake: Toolkit Standards Conformance

**Change**: 260717-6end-toolkit-standards-conformance
**Created**: 2026-07-18

## Origin

One-shot invocation via `/fab-new`. User's raw input:

> Task: Bring this repo and its tool into conformance with the sahil87 toolkit standards.
>
> Precondition: `shll standards` runs on this machine (if the subcommand is missing, run `shll update`; if it still fails, stop and report -- do not proceed from memory or the website). This repo's constitution carries the Toolkit Standards article; this task is the conformance work it mandates.
>
> 1. Enumerate at runtime: run `shll standards`, then `shll standards <name>` for every listed entry. The list is authoritative -- do not assume which standards exist or what they require.
> 2. Audit this repo against each standard. For mechanical contracts (machine help output, README/docs-site structure), execute the standard's own verification checklist verbatim. For the principles, assess each numbered principle against the tool's actual behavior -- prompts and TTY handling, stdout/stderr separation, --json/--dry-run/--yes coverage, exit codes and error wording, idempotency, output volume.
> 3. Fix what is proportionate here: all mechanical-contract violations, and principle gaps that are small and additive (a missing flag, a misrouted stream, an unhelpful error). Larger gaps that would restructure the tool are NOT for this change -- record each as a draft change or issue per this repo's convention and reference it.
> 4. Deliverable: one fab change whose PR body contains a conformance report -- one section per standard with PASS or the gaps found, each gap dispositioned as fixed here (with the commit) or deferred to <ref>. Include the shll version audited against (`shll version`'s shll row), since standards are versioned with the shll release. Tests green; if the command tree changed, re-verify the machine-help contract afterward.
>
> Note on the "skill" standard specifically: if this repo has not yet implemented a `<tool> skill` subcommand, that is a known, deferred gap (per the toolkit's phased per-repo adoption -- no seven-repo flag-day) -- report it as "deferred, not yet adopted" rather than treating it as an in-scope fix for this change.

**Precondition verified at intake time**: `shll standards` runs and lists 4 standards (`principles`, `help-dump`, `readme-extraction`, `skill`). `shll version` reports `shll v0.0.23`. No `shll update` was needed.

## Why

1. **The pain point**: Constitution v1.1.0 (amended 2026-07-18, change 260717-nq1y "Bind Constitution to sahil87 Toolkit Standards") added the Toolkit Standards article: this tool "MUST conform to the toolkit's published standards" as enumerated by `shll standards`. The article binds the repo but no conformance audit has ever been executed — the repo's actual standing against the published standards is unknown.
2. **The consequence of not fixing**: Unaudited drift. The standards govern surfaces with live consumers — shll.ai's scheduled puller consumes `wt help-dump` and the README/docs-site tree daily; agents operate `wt` under the assumptions the principles encode (non-interactive safety, stream separation, exit-code semantics). A silent violation breaks those consumers without warning, and future CLI-surface changes have no verified baseline to preserve.
3. **Why this approach**: Runtime enumeration (`shll standards` → `shll standards <name>`) is what the constitution article itself prescribes, and standards are versioned with the shll release — auditing from memory or a website snapshot could audit against the wrong version. One fab change with a per-standard PASS/gap report gives the constitution article its first enforcement receipt and a durable baseline for future changes.

## What Changes

### 1. Runtime enumeration (audit input — MUST be re-run at apply)

The apply-stage agent MUST re-run `shll standards` and `shll standards <name>` for every listed entry at audit time — the list below is the intake-time snapshot for planning, NOT the audit input. If the runtime list differs from this snapshot, the runtime list wins. If `shll standards` fails at apply time: run `shll update` once; if it still fails, STOP and report — do not proceed from memory or the website.

Intake-time snapshot (shll v0.0.23):

| Standard | Scope | Governs |
|----------|-------|---------|
| `principles` | foundation | The ten toolkit CLI principles (all MUST except №10 SHOULD) |
| `help-dump` | binary | Machine-readable JSON help contract from hidden `help-dump` |
| `readme-extraction` | repo | README + `docs/site/` structure shll.ai pulls and renders |
| `skill` | binary+repo | `<tool> skill` subcommand serving `docs/site/skill.md` |

### 2. Audit procedure per standard

**Mechanical contracts — execute the standard's own "Verifying conformance" checklist verbatim:**

- **help-dump** (checklist from the standard): `wt help-dump` exits 0, valid JSON to stdout only, stderr empty; envelope is `{tool, version, schema_version, root}` with no `captured_at`; `completion`/`help`/hidden commands absent from the tree; `version` reflects the built binary, not a literal; a minimal test pins exit 0 + valid JSON + expected `tool`/`schema_version`. Note: wt already implements this (`src/internal/worktree/helpdump.go` + tests; contract memory `wt-cli/help-dump-contract`) — the audit verifies rather than assumes.
- **readme-extraction** (checklist from the standard): README top is `#` H1 → toolkit blockquote (exact line `> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.`) → badges → prose tagline; grep for relative targets (`](./`, `](../`, `](docs/`) — each either points into `docs/site/` from the README, stays inside `docs/site/` between tree pages, or is absolute; no relative images anywhere (all images absolute `https://…`); no ```` ```mermaid ```` fences destined for the site; no `#gh-*-mode-only` fragments; no `docs/site/` page named `overview`, `readme`, or `commands`; README cross-links its `docs/site/` pages and the absolute command-reference URL.
  - **Candidate gap spotted at intake**: README line 85 links the command reference as `https://shll.ai/tools/wt/commands/`, but the standard's rule 8 specifies `https://shll.ai/<tool>/commands/` (i.e. `https://shll.ai/wt/commands/`). Verify against the runtime standard text and fix if confirmed.
  - Current tree: `docs/site/install.md`, `docs/site/workflows.md` (no reserved names). README has no footer headings (`Contributing`/`Development`/`Building`/`License`/`Acknowledgements`) — the tail rule simply doesn't trigger; confirm the whole README is intentionally site-worthy.
- **skill**: NOT audited as a fixable gap. wt has no `skill` subcommand (verified: `wt skill` → `unknown command`). Per the standard's own Adoption section ("No tool ships `skill` today"; phased per-repo, no seven-repo flag-day) and the task note, the report section for this standard reads **"deferred, not yet adopted"** with a deferral ref (backlog entry) for eventual adoption.

**Principles (№1–№10) — assess each against `wt`'s actual behavior**, command by command (`create`, `list`, `open`, `go`, `delete`, `init`, `shell-init`, `update`, root):

1. **Non-interactive by default**: every command runnable without a human; confirmations satisfiable by flag (`--yes`/`-y`); non-TTY stdin → refusal naming the flag, never a hang. Audit wt's prompts (delete confirmation, create/init open-anyway prompt, menus) and their TTY gating / `--non-interactive` coverage (constitution Principle VI already mandates this — check the flag exists where prompts exist and that non-TTY degrades correctly).
2. **stdout data / stderr diagnostics**: verify per-command stream split (memory contracts `wt-cli/create-output-phases`, `wt-cli/list-status-contract` document the intended split — audit verifies reality); `--json` on programmatically-consumed output (`wt list --json` exists; assess whether other outputs warrant it — additions only if small).
3. **Help is a published contract**: layered help (short summary + usage examples); hidden `help-dump` per the mechanical contract above.
4. **Fail fast, actionable errors**: what failed / why / what next; exit codes documented per subcommand, `0`/`1`/`2` convention (`internal/worktree/errors.go` defines typed codes — constitution Principle III); error wording audit.
5. **Visible mutation boundaries**: read vs write clear from name+help; destructive writes support `--dry-run` sharing the real code path + explicit consent per №1. Audit `wt delete` (and any other destructive path) for `--dry-run`/consent coverage.
6. **Stateless, retry-safe**: state re-derived from git at request time; idempotent re-runs after partial failure (rollback machinery exists in `internal/worktree/rollback.go`).
7. **Compose, don't reinvent**: shells out to peers (e.g. `hop ls --trees` composes `wt list --json`); capability probing not assumption (`wt update`'s `--no-brew-update` family).
8. **Graceful degradation**: missing optional deps skip not error; TTY-gated color/box-drawing; typed "unavailable" results.
9. **Bounded, high-signal output**: unbounded surfaces capped with explicit truncation notices; `--quiet` (where present) leaves data + errors only.
10. **Agent-discoverable documentation** (SHOULD): README/docs-site per readme-extraction (covered above); `CLAUDE.md`/`AGENTS.md` point at standards rather than restating; `<tool> skill` — deferred (see above).

### 3. Fix policy (proportionality)

- **Fix in this change**: ALL mechanical-contract violations (help-dump, readme-extraction), and principle gaps that are small and additive — a missing flag on an existing command, a misrouted stream line, an unhelpful/unactionable error message, a missing truncation notice, a doc-structure fix.
- **Defer**: gaps requiring restructuring — a new subcommand (including `skill`), prompt-flow redesign, cross-command output redesign, new machine-format surfaces with schema commitments. Each deferred gap is recorded as a `fab/backlog.md` entry (this repo's deferral convention — 4-char ID + dated line) and referenced from the report as `deferred to [<id>]`.
- Every fix follows the existing constitution: cobra commands in `src/cmd/wt/`, logic in `src/internal/worktree/`, typed exit codes, tests alongside (unit + integration in `cmd/wt/integration_test.go` where user-visible), stdout/stderr convention (stdout = machine result, stderr = human copy, errors via `ExitWithError(what, why, fix)`).

### 4. Deliverable: conformance report in the PR body

PR body structure — one section per standard, in `shll standards` list order:

```markdown
## Conformance report — audited against shll v0.0.23

### principles
- №1 Non-interactive by default: PASS | gap: <desc> — fixed in <commit> | deferred to [<backlog-id>]
- ... (all ten, individually dispositioned)

### help-dump
PASS (checklist executed verbatim: <receipts>) | gaps...

### readme-extraction
PASS | gaps...

### skill
Deferred, not yet adopted (phased per-repo adoption; no seven-repo flag-day). Tracked as [<backlog-id>].
```

The shll version row comes from `shll version` output at audit time (intake-time value: `shll v0.0.23`). Every gap carries exactly one disposition: `fixed in <commit>` or `deferred to <ref>`.

### 5. Verification

- Tests green: `go test ./...` from `src/` (module root). `gofmt -l` clean — CI fails fast on gofmt before vet/test.
- If the audit's fixes change the command tree (new flags/commands), re-execute the help-dump verification checklist afterward and confirm the pinned help-dump tests still pass.
- README/docs-site fixes re-checked against the readme-extraction checklist after edits.

## Affected Memory

Exact set depends on audit findings; the structural certainty is:

- `wt-cli/toolkit-standards-conformance`: (new) The conformance baseline — which standards version was audited (shll v0.0.23), per-standard verdicts, what was fixed vs deferred, and the re-audit trigger (CLI-surface/README/docs-site changes must be checked against the standards per the constitution article).
- `wt-cli/help-dump-contract`: (modify — only if the audit finds and fixes help-dump gaps; otherwise untouched)
- `wt-cli/flag-naming-conventions`: (modify — only if small-additive flag fixes land, e.g. a new `--yes`/`--dry-run`/`--quiet` on an existing command)
- `wt-cli/list-status-contract`, `wt-cli/create-output-phases`: (modify — only if stream-routing fixes land on those surfaces)

## Impact

- **Code**: `src/cmd/wt/*.go` (flag additions, error wording), `src/internal/worktree/*.go` (stream routing, error text, any small behavior fixes) — scope bounded by audit findings; possibly zero code changes if principles PASS.
- **Docs**: `README.md`, `docs/site/install.md`, `docs/site/workflows.md` (readme-extraction fixes — at least the command-reference URL candidate gap).
- **Backlog**: `fab/backlog.md` gains one entry per deferred gap (at minimum: `wt skill` adoption).
- **Tests**: new/updated unit + integration tests for any behavioral fix; existing help-dump pinned tests re-verified.
- **External consumers**: shll.ai pull pipeline (README/docs-site/help-dump) — fixes make pulls cleaner, no consumer-breaking changes allowed (help-dump schema evolution rule: new fields optional only).

## Open Questions

*(none — the task text prescribes procedure, fix policy, deferral policy, and deliverable; remaining judgment calls are graded below)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Audit set = whatever `shll standards` enumerates at apply time (intake snapshot: principles, help-dump, readme-extraction, skill @ shll v0.0.23); runtime list is authoritative over this intake | Task mandates runtime enumeration explicitly; precondition verified working at intake | S:95 R:90 A:95 D:95 |
| 2 | Certain | `skill` standard reported as "deferred, not yet adopted", tracked as a backlog entry — not implemented in this change | Task note prescribes exactly this; the standard's own Adoption section confirms no tool ships it yet | S:95 R:90 A:100 D:95 |
| 3 | Certain | Mechanical-contract audits execute each standard's own "Verifying conformance" checklist verbatim against the built binary and repo tree; help-dump checklist re-run after any command-tree change | Task prescribes verbatim checklist execution; checklists exist in both mechanical standards | S:90 R:85 A:90 D:90 |
| 4 | Certain | Report includes the shll version audited against, read from `shll version`'s shll row at audit time | Task prescribes it verbatim; value observable at runtime | S:90 R:95 A:95 D:90 |
| 5 | Confident | Deferred gaps recorded as `fab/backlog.md` entries (4-char ID convention) rather than GitHub issues or draft fab changes | Task says "draft change or issue per this repo's convention"; backlog.md is the repo's live deferral convention (existing dated `[id]` entries, some later promoted to changes); a draft change remains a valid alternative for any gap that is already fully specified | S:70 R:85 A:75 D:60 |
| 6 | Confident | Proportionality boundary: fix = flag additions on existing commands, stream rerouting, error/notice wording, doc-structure edits; defer = new subcommands, prompt-flow redesign, cross-command output redesign, new machine-format schemas | Task defines the policy with examples on both sides; per-finding judgment applied at apply within that policy | S:80 R:70 A:75 D:65 |
| 7 | Confident | Principle audit is per-command against actual behavior (all 8 user-facing commands + root), with each of the ten principles individually dispositioned in the report | Task lists the assessment dimensions (TTY, streams, flags, exit codes, idempotency, volume); per-command sweep is the only honest way to claim PASS | S:75 R:80 A:85 D:75 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
