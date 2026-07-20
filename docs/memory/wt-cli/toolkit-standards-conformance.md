---
type: memory
description: "The sahil87-toolkit-standards conformance baseline for `wt` — the audited standards version (shll v0.1.7), the per-standard verdicts (all seven standards PASS after conformance fixes), and the re-audit trigger (any CLI-surface / README / docs-site change must be re-checked against the standards per the constitution's Toolkit Standards article)."
---
# wt-cli: Toolkit Standards Conformance

**Domain**: wt-cli

> Post-implementation behavior capture for the toolkit-standards conformance audits.

## Overview

`wt` is part of the sahil87 open-source toolkit and, per the constitution's **Toolkit Standards** article (added v1.1.0, change `260717-nq1y`), MUST conform to the toolkit's published standards — the ones `shll standards` enumerates. This file is the audit's durable receipt: the standards version audited against, the per-standard verdict, the exact fixes and deferrals, and the trigger that requires re-auditing. It is the enforcement baseline the constitution article implicitly demands but did not itself create.

The standards are enumerated at runtime by `shll standards`, each read with `shll standards <name>`; the list is authoritative over any snapshot. The full per-standard PASS/gap report produced by the first audit lives in that change folder (`conformance-report.md`) and is embedded in the PR body — this memory file is the distilled, durable record.

## Requirements

### Requirement: The audit records the shll version it was run against

The conformance verdicts SHALL be dated to a specific `shll` release, because the standards are versioned with the shll release and re-auditing against a different version can change the verdicts.

- **Current baseline: shll v0.1.7** — seven standards enumerated (runtime `shll standards`, list order): `principles`, `help-dump`, `readme-extraction`, `skill`, `update`, `version`, `shell-init`. The last three are **new since the shll v0.0.23 baseline** (which published only the first four); the `update` / `version` / `shell-init` conformance fixes below landed against v0.1.7 (`260719-32su`). All seven standards PASS after those fixes.
- The first four standards were first audited against **shll v0.0.23** (`260717-6end`); their verdicts are re-confirmed here.
- **Binary audited (v0.0.23 baseline)**: `wt` built by `just build`, version `v0.0.24-2-g97b9f0e` (stamped from `git describe`).

### Requirement: Per-standard verdicts (shll v0.1.7)

Each standard SHALL carry a single verdict, with each fix citing the change that landed it. The first four standards were established at the v0.0.23 baseline (`260717-6end`); the last three were audited and fixed at v0.1.7 (`260719-32su`).

- **`principles`** (foundation, №1–№10): PASS.
  - №1 (non-interactive by default) / №4 (fail-fast actionable errors): PASS — the shared fallback-menu path refuses a non-TTY EOF with a structured `Error:`/`Why:`/`Fix:` message naming the escape, never a bare `reading input: EOF` (260717-6end; see [menu-navigation-contract](/wt-cli/menu-navigation-contract.md) § Non-TTY EOF refusal).
  - №2 (stdout=data / stderr=diagnostics): PASS. For `wt delete`, ALL of `delete.go`'s non-error human copy is on stderr (260717-ohwb); its stdout is empty on every live path, and the sole stdout output is the `--dry-run` preview (`Dry run` header + `Would …` lines, 260717-p5m9) — the machine result the caller asked for (see [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md)). See [create-output-phases](/wt-cli/create-output-phases.md) § `wt delete`'s entire non-error human copy is on stderr. (The shared interactive menu rendering stays on stdout — cross-command infrastructure with its own `menu-navigation-contract.md`.)
  - №3 (help is a published contract): PASS — layered help + the hidden `wt help-dump` JSON tree (see [help-dump-contract](/wt-cli/help-dump-contract.md)).
  - №5 (visible mutation boundaries): PASS — read/write is clear from names and `wt delete` requires consent. `wt delete` has a `--dry-run` preview sharing the live code path (all decision logic runs live, only leaf mutations are gated — 260717-p5m9; see [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md)). The destructive-path audit confirmed that `wt delete` is the **only** command with wt-owned destructive writes — `create` rollback is self-scoped, init/open are delegated/side-effecting-not-destructive, `update` is delegated to brew, and `go`/`list`/`shell-init` are read-only — so **no other command requires `--dry-run`** (full dispositions in [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md) § destructive-path audit).
  - №6 (stateless / retry-safe): PASS — state re-derived from git each run; `wt create` rollback converges after partial failure.
  - №7 (compose, don't reinvent): PASS — shells out to `git` / `brew`; `wt list --json` is `hop`'s composition surface; `wt update` probes the callee's advertised flag.
  - №8 (graceful degradation): PASS — missing apps omitted not fatal, `NO_COLOR`-gated color, ASCII box-drawing fallback, non-fab `fab sync` skips cleanly.
  - №9 (bounded, high-signal output): PASS — `wt list --status` caps its worker pool; `wt delete`'s unpushed-commit preview is capped; no unbounded surface (no `--quiet` is required — the clause is conditional).
  - №10 (agent-discoverable docs, SHOULD): PASS — README + docs/site (per `readme-extraction`) and the `wt skill` agent bundle (260717-v7xy, see `skill` below).
- **`help-dump`** (binary, mechanical contract): **PASS**, checklist executed verbatim. Exit 0, valid JSON to stdout only, empty stderr; envelope exactly `{root, schema_version, tool, version}` with no `captured_at`; `completion` / `help` / hidden nodes (incl. `help-dump` itself) absent; 9 visible subcommands (`create, delete, go, init, list, open, shell-init, skill, update`); `version` = built binary. Pinned by `TestHelpDump_EmitsValidEnvelope` (expects the 9 subcommands, incl. the visible `skill` node — 260717-v7xy).
- **`readme-extraction`** (repo, mechanical contract): **PASS** (260717-6end). The command-reference link in `README.md` is `https://shll.ai/wt/commands/` per rule 8 (`https://shll.ai/<tool>/commands/`); the grep checklist (top-order, tail rule, image absoluteness, docs/site closure, reserved names, no mermaid, no `#gh-*`) runs clean.
- **`skill`** (binary+repo): **PASS — adopted** (260717-v7xy). `wt` ships a visible `wt skill` subcommand printing a static ≤150-line agent usage bundle byte-identical to the canonical `docs/site/skill.md`. The standard's "Verifying conformance" checklist passes: command named exactly `skill`, visible (not `Hidden`), raw markdown to stdout, stderr empty, exit 0, `cobra.NoArgs`; the ≤150-line hard budget is pinned by a build-time test; the bundle is byte-identical to `docs/site/skill.md` (drift-guarded); and `/wt/skill` renders from the pulled `docs/site/**` tree. Full behavior contract: [skill-command-contract](/wt-cli/skill-command-contract.md).
- **`update`** (binary): **PASS — fixed** (260719-32su). Two MUST violations against the v0.1.7 standard were fixed:
  - **flag-substring contract**: `wt update --help` MUST contain the literal `--skip-brew-update` (shll probes it via `strings.Contains` before every toolkit-wide run). The flag had been deprecated+hidden via `MarkDeprecated`; the fix removes the `MarkDeprecated` and registers both `--skip-brew-update` (contract flag, full help text) and `--no-brew-update` (visible alias) on one bool. Neither is hidden or warns. Pinned by the update-command help-substring / both-flags-visible / no-deprecation-warning tests.
  - **brew-handling safety**: no code path may SIGKILL brew mid-transaction, and `brew upgrade` may carry no short hard timeout. `brew upgrade` had a 120s `exec.CommandContext` SIGKILL bound (`brew update`/`brew info` 30s, same kill mode). The fix runs `brew upgrade` unbounded (plain `exec.Command`) and routes `brew update` (5 min) / `brew info` (60s) through a shared `newBoundedBrewCmd` helper whose `cmd.Cancel` sends SIGTERM with a 10s `WaitDelay` grace. Full contract: [update-command-contract](/wt-cli/update-command-contract.md) § Brew-handling safety.
- **`version`** (binary): **PASS — test added** (260719-32su). `wt --version` already conformed — exit 0, version on stdout, ≤2s with no network I/O, first non-empty line the cobra-canonical `wt version <token>` shape, binary name on PATH (`wt`) equal to the tool name. The only gap was SHOULD-level: no test covered `--version` anywhere. The fix adds a version test pinning exit 0 + stderr-empty + the first-line `^wt version \S+$` shape (documenting shll's `versionTokenRE`/`versionPrefixRE`); no version behavior changed.
- **`shell-init`** (binary): **PASS — fixed** (260719-32su). Two MUST violations: an unsupported/missing shell emitted the wrapper on stdout and exited 0, when any failure MUST exit non-zero with empty stdout, and a missing/unsupported shell MUST be a usage error (exit 2, usage on stderr, empty stdout). The fix makes the shell argument required and constrained to `zsh|bash`, removes `$SHELL` inference, and routes missing/unsupported through the exit-2 direct-exit pattern (`wt.ExitWithError(wt.ExitInvalidArgs, ...)`) with empty stdout. The SHOULD-level eval-in-subshell test was also added. Full contract: [shell-init-contract](/wt-cli/shell-init-contract.md).

### Requirement: The re-audit trigger

Any change touching the **CLI surface, help output, `README.md`, or `docs/site/`** SHALL be re-checked against the standards governing that surface **before** it lands — the constitution's Toolkit Standards article is a standing obligation, not a one-time audit. Mechanical contracts (`help-dump`, `readme-extraction`) have verbatim "Verifying conformance" checklists; the ten principles are assessed against actual per-command behavior. Standards added or revised upstream (sahil87/shll `docs/site/standards/`, rendered on https://shll.ai) bind this repo without a constitution amendment, so the enumeration MUST be re-run (`shll standards`) rather than read from this file — this baseline records what was true at shll v0.1.7, not a frozen contract. The v0.0.23→v0.1.7 re-audit (three new standards `update`/`version`/`shell-init`, `260719-32su`) is exactly this trigger firing: the standard set grew, and re-running the enumeration surfaced the new bindings.

## Design Decisions

### Runtime enumeration over a memorized standard list
**Decision**: The audit enumerates standards at apply time via `shll standards` + `shll standards <name>`, treating the runtime list as authoritative over any intake-time snapshot; if `shll standards` fails, run `shll update` once, else STOP — never audit from memory or the website.
**Why**: Standards are versioned with the shll release, so auditing from a stale snapshot could verify against the wrong version; the constitution article itself prescribes runtime enumeration.
**Rejected**: Hard-coding the standard list or auditing from the shll.ai website snapshot (both risk version drift).
*Introduced by*: `260717-6end-toolkit-standards-conformance`

### Proportionate fix boundary: fix small-additive, defer restructuring
**Decision**: Fix here = flag additions on existing commands, a misrouted stream line, unactionable error wording, doc-structure edits. Defer = new subcommands, prompt-flow redesign, cross-command output redesign, new machine-format schemas — each recorded as a `fab/backlog.md` entry (4-char id) and referenced from the report.
**Why**: A conformance audit's job is to establish the baseline and close cheap gaps without destabilizing the tool; restructuring gaps deserve their own scoped change. The one in-scope principle fix (the non-TTY menu refusal) is a single-choke-point error-wording change.
**Rejected**: Fixing the `wt delete` stdout→stderr realignment and adding `--dry-run` inline (both exceed the small-additive boundary and would churn a conformance change); GitHub issues or draft changes for the deferrals (backlog.md is this repo's live deferral convention).
*Introduced by*: `260717-6end-toolkit-standards-conformance`
The `--dry-run` gap landed as its own restructuring-sized change exactly as this boundary intends (260717-p5m9 — see [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md)).

### The three new-standard fixes moved to their own behavior contracts, not this receipt
**Decision**: the `update` / `version` / `shell-init` conformance fixes (`260719-32su`) record only a per-standard verdict here; the full behavioral contract lives in the command's own memory file ([update-command-contract](/wt-cli/update-command-contract.md), [shell-init-contract](/wt-cli/shell-init-contract.md)), and the flag-hiding carve-out lives in [flag-naming-conventions](/wt-cli/flag-naming-conventions.md).
**Why**: this file is the audit receipt (which standards, which version, PASS/gap + the fixing change); the durable behavior lives with the command it governs, so a future reader of `wt update` finds the contract next to the command rather than buried in the audit log. `version` gets no dedicated file — the conformance was test-only, so its verdict is the whole record.
**Rejected**: inlining each command's full behavioral contract into this receipt (duplicates the command's own memory file and drifts); a single omnibus "standards fixes" file (splits each command's contract from its command).
*Introduced by*: `260719-32su-conform-update-version-shell-init`

## Cross-references

- Constitution: **Toolkit Standards** article (v1.1.0, added by `260717-nq1y`) — the standing obligation this baseline enforces. Principle III (typed exit codes) and VI (interactive by default, scriptable on demand) are the constitution's local restatements that the toolkit principles №4 and №1 generalize.
- Source (v0.0.23 baseline fixes): `src/internal/worktree/menu.go` (`runFallbackMenu` EOF refusal), `README.md` line 85 (command-reference URL).
- Source (v0.1.7 fixes, `260719-32su`): `src/cmd/wt/update.go` (both flags visible, no `MarkDeprecated`), `src/internal/update/update.go` (`newBoundedBrewCmd` SIGTERM+grace helper, unbounded `brew upgrade`), `src/cmd/wt/shell_init.go` (required `zsh|bash` arg, exit-2 direct-exit), `src/cmd/wt/version_test.go` (version pin).
- Report: `fab/changes/260717-6end-toolkit-standards-conformance/conformance-report.md` — the full per-standard PASS/gap report with checklist receipts (embedded in the PR body at ship).
- Deferrals: `fab/backlog.md` — the three audit deferrals are closed: `[v7xy]` → `260717-v7xy` (`wt skill`), `[p5m9]` → `260717-p5m9` (`wt delete --dry-run`; see [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md)), `[ohwb]` → `260717-ohwb` (`wt delete` stderr realignment, №2 above).
- Sibling memory: [menu-navigation-contract](/wt-cli/menu-navigation-contract.md) — where the №1/№4 non-TTY fallback-menu fix is documented in behavioral detail; [help-dump-contract](/wt-cli/help-dump-contract.md) — the `help-dump` standard's PASS surface and the cross-repo pull contract with shll.ai; [skill-command-contract](/wt-cli/skill-command-contract.md) — the full behavior contract for the `wt skill` subcommand adopted by `260717-v7xy`; [delete-dry-run-contract](/wt-cli/delete-dry-run-contract.md) — the `wt delete --dry-run` preview that closes the principle №5 gap, with the full per-command destructive-path audit dispositions; [update-command-contract](/wt-cli/update-command-contract.md) — the `update` standard's flag-substring + brew-handling safety contract; [shell-init-contract](/wt-cli/shell-init-contract.md) — the `shell-init` standard's strict argument contract; [flag-naming-conventions](/wt-cli/flag-naming-conventions.md) — the carve-out that a standard-frozen contract flag (`--skip-brew-update`) may not be hidden via `MarkDeprecated`.
- External: `shll standards` (runtime enumeration), https://shll.ai (rendered standards), sahil87/shll `docs/site/standards/` (canonical source).
