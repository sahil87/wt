# Intake: Help-Dump Emit Aliases

**Change**: 260718-zb77-help-dump-emit-aliases
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation. Raw input:

> wt's help-dump command only emits each cobra command's canonical Name(), never its Aliases() -- so alias subcommands like 'ls' (alias for list, src/cmd/wt/list.go:56), 'new' (alias for create, create.go:32), and 'rm' (alias for delete, delete.go:32) don't appear in the generated help/wt.json, causing shll.ai's README-drift checker to flag 'wt ls', 'wt new', 'wt rm' from the README as unknown/nonexistent commands even though they work fine as real cobra aliases. Fix: extend help-dump's tree-walk to also emit registered aliases for each command node (as additional Node entries, or an aliases field alongside name -- match whatever shape shll's equivalent fix lands on for the shared help/*.json schema_version:1 contract used across all 7 tools) so alias-form invocations are discoverable in the dump.

Intake-time verification performed against the live code and the upstream repo (findings drove the Assumptions below):

- **shll's equivalent fix has NOT landed.** As of today, shll.ai's `sites/astro-starlight-terminal1/src/lib/schemas.ts` `NodeSchema` has no `aliases` field, and `docs/specs/help-dump-contract.md` never mentions command aliases. The published producer standard (`shll standards help-dump`) likewise defines the Node shape as exactly `{name, path, short, usage, text, commands}`. There is no upstream shape to match — wt is first mover.
- **The drift checker's blindness is structural.** shll.ai's `findUnknownTokens` (`sites/astro-starlight-terminal1/src/lib/extract-readme.ts:487`) walks `helpFacts.childrenOf`, which is built from `node.commands.map((c) => c.name)` only — aliases never enter the known-command set. shll's own `parse-help.ts` parses the `Aliases:` section out of `text` but deliberately does not surface it in the structured view.
- **The standard sanctions the fix shape.** Its Schema evolution rule: "When the schema evolves, new fields MUST be added as **optional**, so each tool adopts them on its own release cadence — no seven-repo flag-day, and older captures keep validating."
- **Alias inventory (grep-verified):** exactly three aliased commands — `list` → `ls` (`src/cmd/wt/list.go:56`), `create` → `new` (`src/cmd/wt/create.go:32`), `delete` → `rm` (`src/cmd/wt/delete.go:32`). The visible tree is currently 9 commands: create, list, open, go, delete, init, shell-init, update, skill (the in-repo memory's "7 subcommands" count predates `go`/`skill`).

## Why

1. **Pain point**: `wt help-dump` (producer half of the shll.ai pull contract) emits only `cmd.Name()` per node. The real cobra aliases `ls`/`new`/`rm` are invisible in the structured tree, so shll.ai's README-drift checker flags `wt ls`, `wt new`, `wt rm` — legitimate, working invocations documented in the README — as fabricated subcommands.
2. **Consequence if unfixed**: the drift gate stays red on correct documentation. The only workarounds are bad ones: strip the ergonomic alias forms from the README (worse docs — the aliases exist precisely to be used), or teach the checker to ignore them ad hoc (defeats the drift gate's purpose). Aliases are also undiscoverable to any other consumer of `help/wt.json`'s structured fields.
3. **Why this approach**: emit an optional `aliases` field on each Node. The published standard's schema-evolution rule explicitly blesses optional-field additions under `schema_version: 1`. The alternative in the raw input — additional Node entries per alias — would corrupt tree semantics: `helpFacts.commandPaths` would gain phantom paths, the site's command-reference tree would render `ls`/`new`/`rm` as separate full commands with duplicated `text`, and `path`/`usage` would misreport invocations. The alias is a property of a command, not a sibling command; the schema should say so.

## What Changes

### 1. `HelpNode` gains an optional `aliases` field (`src/internal/worktree/helpdump.go`)

Add `Aliases` to the `HelpNode` struct, placed immediately after `Name` (the "alongside name" shape), with `omitempty` so non-aliased nodes marshal byte-identically to today's output:

```go
type HelpNode struct {
	Name     string     `json:"name"`
	Aliases  []string   `json:"aliases,omitempty"`
	Path     string     `json:"path"`
	Short    string     `json:"short"`
	Usage    string     `json:"usage"`
	Text     string     `json:"text"`
	Commands []HelpNode `json:"commands"`
}
```

`buildNode` copies `cmd.Aliases` into the node. Nil/empty stays absent from the JSON (this is the deliberate contrast with `commands`, which must be a non-nil `[]` because shll's schema requires `z.array` — `aliases` will be `.optional()` upstream, and absence MUST be valid since the other six tools won't emit it until they adopt).

Expected output delta — exactly three nodes change, e.g. the `list` node:

```jsonc
{
  "name": "list",
  "aliases": ["ls"],
  "path": "wt list",
  ...
}
```

`create` gains `"aliases": ["new"]`, `delete` gains `"aliases": ["rm"]`. Every other node (root included) is byte-identical to today.

### 2. Everything else in the builder is untouched

- `text` needs no change: Cobra's default help template already renders the `Aliases:` section for aliased commands, so the raw `-h` render in `text` has always carried the alias info (shll's `parse-help.ts` anchors on that section today). The structured field is the only gap.
- Envelope (`{tool, version, schema_version, root}`), filter rules, discovery walk, render-time detachment, no-`captured_at` rule: all unchanged. `schema_version` stays integer `1` — optional-field addition is exactly the evolution the standard permits without a bump.
- `src/cmd/wt/help_dump.go` (thin cobra wiring) is untouched.

### 3. Tests (`src/internal/worktree/helpdump_test.go`, `src/cmd/wt/help_dump_test.go`)

Per Constitution IV and the standard's "keep a minimal test pinning the contract":

- Extend the node-shape coverage: the `list`/`create`/`delete` nodes carry exactly `["ls"]`/`["new"]`/`["rm"]`; a non-aliased node (e.g. `open`) and the root marshal with **no** `aliases` key at all (assert on marshaled JSON, not just the struct, to pin `omitempty`).
- Command-level test (`help_dump_test.go`): envelope still has exactly the four top-level keys; aliased nodes in the emitted JSON carry the field.
- Existing invariants (exit 0, empty stderr, filter rules, live-tree restoration) must keep passing untouched.

### 4. Cross-repo coordination (out of scope here, recorded for traceability)

wt ships the producer half only. The consumer half lands in shll.ai separately: `NodeSchema` gains `aliases: z.array(z.string()).optional()`, `helpFacts.childrenOf` includes child aliases, and the `help-dump` standard page documents the field. Emitting before shll updates is safe: `NodeSchema` is a non-strict `z.object`, so unknown keys pass validation today, and the scheduled puller commits captured stdout — the field simply rides along until the consumer reads it. The README-drift false positives disappear only once shll's consumer half lands; wt's obligation ends at emitting a conformant dump.

## Affected Memory

- `wt-cli/help-dump-contract`: (modify) Node shape section gains the optional `aliases` field (semantics, `omitempty` rule, contrast with non-nil `commands`); refresh the stale "7 visible subcommands" count to the current 9 (create, delete, go, init, list, open, shell-init, skill, update); note wt-as-first-mover and the pending shll.ai consumer half.

## Impact

- `src/internal/worktree/helpdump.go` — `HelpNode` struct + `buildNode` (a few lines).
- `src/internal/worktree/helpdump_test.go`, `src/cmd/wt/help_dump_test.go` — extended assertions.
- No new dependencies; no `cmd/` layer change; no exit-code or stderr behavior change.
- Downstream: shll.ai puller captures the new field on its next scheduled run after release; validation unaffected (non-strict schema). Consumer-side fix tracked separately in shll.ai.
- CI: gofmt gate applies (module root `src/`); standard test/vet.
- Toolkit standards: this is a CLI-surface (help output) change — per the constitution's Toolkit Standards article it was checked against `shll standards help-dump`; the addition conforms via the standard's own schema-evolution rule.

## Open Questions

None — the intake-time verification against shll.ai's live schema/spec resolved the raw input's one open point (which shape to match): nothing has landed upstream, so wt defines the shape per the standard's evolution rule.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Emit an optional `aliases` field on Node (alongside `name`, no `schema_version` bump), not additional Node entries per alias; wt moves first | Verified shll has no landed shape to match (NodeSchema/spec/standard all alias-free today); the standard's schema-evolution rule mandates optional-field additions under `schema_version: 1`; duplicate nodes would corrupt `helpFacts.commandPaths`, site tree rendering, and `path`/`usage` semantics | S:70 R:75 A:85 D:70 |
| 2 | Confident | `omitempty` — the field is absent for non-aliased nodes, not an always-present `[]` | Minimizes capture drift (only 3 nodes change); absence must validate anyway since the other 6 tools won't emit it until they adopt (`.optional()` upstream); deliberate contrast with `commands`' non-nil-`[]` rule, which exists only because that field is already required by `z.array` | S:40 R:90 A:70 D:55 |
| 3 | Certain | `text` field unchanged — no render work | Cobra's default help template already renders the `Aliases:` section for aliased commands; shll's `parse-help.ts` anchors on `Aliases:` in `text` today, confirming it is present in live captures | S:80 R:90 A:95 D:90 |
| 4 | Certain | Scope is wt's producer half only; shll.ai consumer fix (schema + `helpFacts` + standard page) is separate cross-repo work | Emitting early is safe: `NodeSchema` is a non-strict `z.object` (unknown keys pass validation); the drift-checker fix inherently requires the consumer half, which lives in shll.ai's repo, not this one | S:75 R:80 A:90 D:85 |
| 5 | Certain | Alias inventory is exactly `list→ls`, `create→new`, `delete→rm`; tests pin these three and assert absence elsewhere | Grep-verified across `src/cmd/wt/*.go` — the only three `Aliases:` declarations in the command tree | S:90 R:95 A:100 D:95 |

5 assumptions (3 certain, 2 confident, 0 tentative, 0 unresolved).
