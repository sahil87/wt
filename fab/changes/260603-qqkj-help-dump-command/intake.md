# Intake: Implement `wt help-dump` for shll.ai pull integration

**Change**: 260603-qqkj-help-dump-command
**Created**: 2026-06-03
**Status**: Draft

## Origin

> /fab-new: "There's an update in the way we integrate with shll.ai. To understand it read
> https://github.com/sahil87/shll.ai/blob/main/docs/specs/help-dump-contract.md#teardown-directive-paste-to-a-tool-repo-agent .
> Implement the change."

**Mode**: Conversational (one clarifying question asked before folder creation).

The linked anchor is the **Teardown directive** in shll.ai's `help-dump-contract.md`. It instructs
a tool repo to *remove its now-dead push wiring* (producer CI, PR-opening step, auto-merge,
`SHLLAI_TOKEN`) because shll.ai has inverted the integration: it now **pulls** help by running
`<tool> help-dump` on a schedule, rather than each tool *pushing* a `help/<slug>.json` PR.

**Gap analysis (decisive)**: A whole-repo grep showed `wt` has **none** of the push wiring the
directive says to remove — no producer CI, no PR-opening step, no auto-merge, no `SHLLAI_TOKEN`
usage (it appears only inside `fab/backlog.md`), and crucially **no `help-dump` command in
`src/`**. Backlog item `[pc47]` *proposed* building the push producer but it was never
implemented. So the literal teardown is a no-op here.

Meanwhile the pull model the directive depends on **requires** `wt help-dump` to emit valid JSON,
and shll.ai already commits a reference `help/wt.json` (6514 bytes) described as "generated from
THIS binary". Under the new pull model, shll.ai running `wt help-dump` would fail outright today.

**Decision** (user-confirmed via clarifying question): scope this change as **implement the
`wt help-dump` command** per the contract — the genuinely missing, load-bearing obligation — and
treat it as **superseding the obsolete push half of backlog `[pc47]`**. The push producer / PR /
token wiring from `[pc47]` is dropped, not built.

## Why

1. **Problem**: shll.ai's command-reference page for `wt` is now refreshed by a scheduled puller
   that runs `wt help-dump`, `brew install`s the tool, and commits the captured JSON. `wt` has no
   `help-dump` command, so that pull fails and the `wt` command reference will silently freeze
   (stale-help gap) — the exact failure the contract's precondition warns about, but from the
   producer side.
2. **Consequence if unfixed**: the `wt` tool page on shll.ai shows stale help forever; `wt` is
   named the *reference tool* for the 7-tool rollout, so its absence is conspicuous.
3. **Why this approach over alternatives**: The contract is explicit and frozen at
   `schema_version: 1`. The tool's *single obligation* is "emit valid `help-dump` output to
   stdout"; everything downstream (capture, timestamping, validation, commit) is shll.ai's job.
   Building the producer-side command (not a transport) is the minimal, correct way to satisfy
   the pull contract. The retired push model (backlog `[pc47]`) is deliberately *not* built — it
   would add a cross-repo write token, PR-opening, and auto-merge that the contract has retired.

## What Changes

### New: `wt help-dump` Cobra subcommand

A new subcommand registered on the root command in `src/cmd/wt/main.go`, implemented in a new
`src/cmd/wt/help_dump.go` (with `src/cmd/wt/help_dump_test.go`). Per Constitution V, the tree-walk
and envelope-building logic belongs under `src/internal/worktree/` and is exercised through the
thin `cmd/` layer; `cmd/help_dump.go` only wires Cobra → the internal builder and writes JSON to
stdout.

**Behavior, per `help-dump-contract.md` §1–§8:**

- **§2 Hidden**: declared `Hidden: true` so it never appears in `wt -h`.
- **§1 Invocation**: emits a single JSON envelope to **stdout**; **stderr empty** on success;
  **exit 0** on success; non-zero on any error (so the puller treats it as a failed capture).
- **§3 Envelope** (tool-emitted shape — note the `captured_at` asymmetry): emit exactly
  `{tool, version, schema_version, root}` and **MUST NOT emit `captured_at`** (shll.ai stamps it
  post-capture). `tool` = `"wt"` (binary name); `version` = the built binary's version
  (`rootCmd.Version` / ldflags `main.version`), never hardcoded; `schema_version` = integer `1`.
- **§3 Node** (recursive): each node is `{name, path, short, usage, text, commands: []}` where:
  - `name` = `cmd.Name()`
  - `path` = `cmd.CommandPath()` (e.g. `"wt create"`)
  - `short` = `cmd.Short`
  - `usage` = `cmd.UseLine()`
  - `text` = the **raw `-h` output byte-for-byte** for that command (Long + Usage + Flags as
    Cobra renders `-h`), newlines preserved — see Open Questions on the exact capture method.
  - `commands` = child Nodes, `[]` for a leaf.
- **§4 Filter**: drop Cobra's auto-generated `completion` and `help` subcommands, and any
  `cmd.Hidden == true` node — which now includes `help-dump` itself (so §2 + §4 make it
  self-filtering for free; no special-case logic).
- **§5 Discovery**: walk `rootCmd.Commands()` **recursively to full depth**; NEVER regex-parse
  `-h` text to discover structure. (`wt` is currently flat — root + 7 leaves — but the walk must
  be recursive to stay correct if nesting is added.)
- **§6 Version**: read from the built binary; the committed sample's `1.4.2` is a placeholder —
  real builds inject via ldflags (already wired: `main.version`).
- **§8 Schema**: keep `schema_version: 1`; do not add fields (future enrichment is a separate,
  optional-field change).

**Conformance target**: output MUST validate against `HelpDocSchema`/`NodeSchema` in shll.ai's
`sites/astro-starlight-terminal1/src/lib/schemas.ts` *after the puller stamps `captured_at`*, and
match the structure of the committed `help/wt.json` reference sample (same 7 subcommands, same
field shapes).

### Test (Constitution IV + contract directive)

Add a test that exercises `help-dump` directly (the contract explicitly asks for one now that no
push CI implicitly exercises it): asserts exit 0, stdout is valid JSON, `tool == "wt"`,
`schema_version == 1`, **no `captured_at` key present**, `completion`/`help`/`help-dump` absent
from the tree, and the structured fields match the live Cobra tree. Per project Test Strategy
(`test-alongside`) and the file-per-source convention, this lives in `help_dump_test.go` (unit)
with internal-package coverage for the builder; an integration assertion in
`cmd/integration_test.go` may exercise the built binary end-to-end.

### Out of scope (explicitly NOT built)

- The retired **push** wiring from backlog `[pc47]`: producer CI that opens a `help/wt.json` PR
  into `sahil87/shll.ai`, auto-merge, and `SHLLAI_TOKEN`. The pull model retires all of it.
- The shll.ai-side puller, `captured_at` stamping, JSON validation, and commit — all shll.ai's
  responsibility per the contract.
- Schema enrichment (new optional fields) — a separate future change; stay at `schema_version: 1`.

### Backlog hygiene

Backlog `[pc47]` SHALL be marked done/superseded by this change (`qqkj`) in `fab/backlog.md` — its
valuable producer half is realized here; its push half (PR-opening, auto-merge, `SHLLAI_TOKEN`) is
intentionally dropped per the contract inversion. <!-- clarified: user confirmed marking [pc47] superseded as part of this change -->

## Affected Memory

- `wt-cli/help-dump-contract`: (new) Behavior contract for the `wt help-dump` command — Hidden,
  stdout JSON envelope `{tool, version, schema_version:1, root}` (no `captured_at`), recursive
  tree walk, `completion`/`help`/Hidden filtering, exit-0/empty-stderr success semantics. Points
  at shll.ai's `help-dump-contract.md` as the upstream forward contract.

## Impact

- **Code**:
  - `src/cmd/wt/main.go` — register `helpDumpCmd()` on the root command.
  - `src/cmd/wt/help_dump.go` (new) — thin Cobra wiring; calls the internal builder, writes JSON
    to stdout, returns `error` via `RunE`.
  - `src/cmd/wt/help_dump_test.go` (new) — command-level test.
  - `src/internal/worktree/` (new file, e.g. `helpdump.go` + `helpdump_test.go`) — tree-walk +
    envelope builder per Constitution V (logic out of `cmd/`).
  - `src/cmd/wt/integration_test.go` — optional end-to-end assertion.
- **Specs**: `docs/specs/cli-surface.md` documents the per-subcommand surface; `help-dump` is
  Hidden so its placement there is informational. A short note (or the new memory file) records
  the contract. `docs/specs/index.md` may gain a row if a dedicated spec is warranted.
- **Dependencies**: none new — uses existing `spf13/cobra` and stdlib `encoding/json`.
- **Exit codes** (Constitution III): map any build error to a typed exit code from
  `internal/worktree/errors.go` (e.g. `ExitGeneralError`) via `RunE` + the root handler.
- **Cross-repo**: producer-side only; no write path into shll.ai. shll.ai consumes via its puller.

## Open Questions

- ~~**Exact `text` capture method**~~ — **Resolved** (clarify 2026-06-03): capture each node's
  `text` by rendering its `-h` output into a buffer (Cobra's help func / `cmd.SetOut(buf)`),
  then verify **byte-for-byte** against the committed `help/wt.json` during planning/apply.
  <!-- clarified: text capture = render -h into buffer + verify vs committed help/wt.json sample, not manual Long+UsageString composition -->

## Clarifications

### Session 2026-06-03

| # | Action | Detail |
|---|--------|--------|
| 12 | Changed | Capture `text` by rendering `-h` into a buffer, verified byte-for-byte vs committed `help/wt.json` (over manual Long+UsageString composition) |
| 13 | Confirmed | Mark backlog `[pc47]` superseded by qqkj as part of this change |

### Session 2026-06-03 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 8 | Confirmed | — |
| 9 | Confirmed | — |
| 10 | Confirmed | — |
| 11 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = implement `wt help-dump` (producer command), not the literal teardown | Gap analysis: wt has zero push wiring to remove and no `help-dump` command; user confirmed this scope via clarifying question | S:95 R:70 A:90 D:90 |
| 2 | Certain | Emit `{tool, version, schema_version, root}` to stdout and OMIT `captured_at` | Contract §3 is explicit: `captured_at` is shll.ai-owned, stamped post-capture; tool must not emit it | S:98 R:80 A:95 D:98 |
| 3 | Certain | `schema_version` is integer literal `1`; add no new fields | Contract §8: frozen at 1 for this revision; enrichment is a separate optional-field change | S:98 R:85 A:95 D:98 |
| 4 | Certain | `Hidden: true`; self-filters via §4 Hidden-drop (no special-case) | Contract §2 + §4 spell this out exactly | S:95 R:85 A:95 D:95 |
| 5 | Certain | Filter `completion`, `help`, and any `Hidden` node from the tree | Contract §4 enumerates the three drop rules | S:98 R:85 A:95 D:98 |
| 6 | Certain | Discover the tree by walking `rootCmd.Commands()` recursively; never regex `-h` | Contract §5 mandates programmatic, full-depth discovery | S:98 R:80 A:95 D:95 |
| 7 | Certain | `version` from the built binary (`main.version` ldflags), never hardcoded | Contract §6; ldflags injection already wired in main.go (`var version = "dev"`) | S:95 R:80 A:98 D:95 |
| 8 | Certain | Tree-walk/envelope logic lives in `internal/worktree/`; `cmd/` stays thin | Clarified — user confirmed | S:95 R:65 A:90 D:80 |
| 9 | Certain | Add a dedicated `help-dump` test (exit 0, valid JSON, tool/schema_version, no captured_at) | Clarified — user confirmed | S:95 R:70 A:90 D:85 |
| 10 | Certain | Do NOT build backlog `[pc47]`'s push producer/PR/auto-merge/`SHLLAI_TOKEN` | Clarified — user confirmed | S:95 R:60 A:90 D:85 |
| 11 | Certain | Conform output to committed `help/wt.json` + `schemas.ts` (validate after captured_at added) | Clarified — user confirmed | S:95 R:70 A:90 D:85 |
| 12 | Certain | Capture `text` by rendering each node's `-h` into a buffer, verified byte-for-byte against the committed `help/wt.json` | Clarified — user chose capture-and-verify over manual Long+UsageString composition | S:95 R:55 A:65 D:55 |
| 13 | Certain | Mark backlog `[pc47]` superseded by qqkj (push half dropped per the inversion) | Clarified — user confirmed backlog hygiene | S:95 R:80 A:60 D:60 |

13 assumptions (13 certain, 0 confident, 0 tentative, 0 unresolved).
