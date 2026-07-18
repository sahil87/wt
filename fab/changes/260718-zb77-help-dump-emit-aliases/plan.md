# Plan: Help-Dump Emit Aliases

**Change**: 260718-zb77-help-dump-emit-aliases
**Intake**: `intake.md`

## Requirements

### Help-Dump Contract: Node aliases field

#### R1: `HelpNode` gains an optional `aliases` field
The `HelpNode` struct in `src/internal/worktree/helpdump.go` SHALL carry an `Aliases []string` field tagged `json:"aliases,omitempty"`, placed immediately after `Name` (the "alongside name" shape). The `omitempty` tag MUST cause a nil/empty slice to be absent from the marshaled JSON, so non-aliased nodes and the root marshal byte-identically to today's output. The envelope shape (`{tool, version, schema_version, root}`), `schema_version` integer `1`, `text`, filter rules, discovery walk, and render-time detachment SHALL remain untouched.

- **GIVEN** the `HelpNode` struct
- **WHEN** a node has no aliases (nil or empty `Aliases`)
- **THEN** its marshaled JSON SHALL contain no `aliases` key
- **AND** the field SHALL sit immediately after `name` in struct/contract order
- **AND** `schema_version` SHALL remain the integer `1` (optional-field addition, no bump)

#### R2: `buildNode` populates `aliases` from `cmd.Aliases`
`buildNode` in `src/internal/worktree/helpdump.go` SHALL copy the command's registered `cmd.Aliases` into the emitted node's `Aliases` field. Aliased commands SHALL therefore surface their alias forms in the structured tree; non-aliased commands SHALL emit no `aliases` key (deliberate contrast with `commands`, which is a non-nil `[]`). No other builder behavior changes.

- **GIVEN** the live `wt` command tree with exactly three aliased commands (`list`→`ls`, `create`→`new`, `delete`→`rm`)
- **WHEN** the builder walks the tree
- **THEN** the `list` node SHALL carry exactly `["ls"]`, `create` exactly `["new"]`, and `delete` exactly `["rm"]`
- **AND** non-aliased nodes (e.g. `open`) and the root SHALL have no `aliases` key in their marshaled JSON

#### R3: Tests pin the aliases contract
Per Constitution IV and the standard's "keep a minimal test pinning the contract", `src/internal/worktree/helpdump_test.go` and `src/cmd/wt/help_dump_test.go` SHALL be extended to pin: aliased nodes carry exactly their expected alias list; non-aliased nodes and the root marshal with **no** `aliases` key (asserted on marshaled JSON, not just the struct, to pin `omitempty`); the command-level test confirms the emitted JSON's aliased nodes carry the field while the four-key envelope invariant holds. All pre-existing invariants (exit 0, empty stderr, four-key envelope, filter rules, live-tree restoration, 9 visible subcommands) SHALL keep passing untouched.

- **GIVEN** the extended test suites
- **WHEN** `go test ./internal/worktree/ ./cmd/wt/` runs
- **THEN** the new alias assertions SHALL pass
- **AND** every pre-existing help-dump invariant SHALL remain green

### Design Decisions

1. **Optional `aliases` field on Node, `omitempty`, no `schema_version` bump** — an `[]string` field placed after `Name`, absent when empty. *Why*: the published shll.ai help-dump standard's schema-evolution rule mandates new fields be added as optional under `schema_version: 1`; `omitempty` minimizes capture drift (only 3 of 9 nodes change) and absence must validate anyway since the other six tools won't emit it until they adopt (`.optional()` upstream). *Rejected*: additional Node entries per alias (would corrupt `helpFacts.commandPaths`, render `ls`/`new`/`rm` as phantom sibling commands, and misreport `path`/`usage`); an always-present `[]` (unnecessary capture drift, and the non-nil-`[]` rule on `commands` exists only because that field is already required by shll's `z.array`).
2. **`text` unchanged** — Cobra's default help template already renders the `Aliases:` section for aliased commands, so `text` already carries the alias info; only the structured field was missing.

### Non-Goals

- shll.ai consumer half (`NodeSchema` gaining `aliases: z.array(z.string()).optional()`, `helpFacts.childrenOf` including child aliases, the standard page documenting the field) — separate cross-repo work in the shll.ai repo. Emitting early is safe: `NodeSchema` is a non-strict `z.object`, so unknown keys pass validation today.
- Any change to `src/cmd/wt/help_dump.go` (thin cobra wiring), exit codes, or stderr behavior.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `Aliases []string` field (tagged `json:"aliases,omitempty"`) to the `HelpNode` struct in `src/internal/worktree/helpdump.go`, immediately after `Name`; update the struct doc comment to note the optional alias field. <!-- R1 -->
- [x] T002 Populate `Aliases: cmd.Aliases` in the `HelpNode` literal returned by `buildNode` in `src/internal/worktree/helpdump.go`. <!-- R2 -->

### Phase 3: Tests

- [x] T003 Extend `src/internal/worktree/helpdump_test.go`: add an aliased command (and confirm a non-aliased one) to `newTestRoot`, then assert via marshaled JSON that aliased nodes carry exactly their alias list and non-aliased nodes + root have no `aliases` key (pins `omitempty`). <!-- R3 -->
- [x] T004 Extend `src/cmd/wt/help_dump_test.go`: add the `aliases` field to `helpDumpNode`, and assert the emitted `list`/`create`/`delete` nodes carry `["ls"]`/`["new"]`/`["rm"]` while non-aliased nodes and the root have no `aliases` key (raw-JSON assertion), keeping the four-key envelope invariant. <!-- R3 -->

### Phase 4: Verification

- [x] T005 Run `go test ./internal/worktree/ ./cmd/wt/` from `src/` and `gofmt -l` on the touched files; fix any failures or formatting. <!-- R3 -->

## Execution Order

- T001 blocks T002 (same struct/builder file).
- T002 blocks T003 and T004 (tests assert the new field).
- T005 runs last (verifies R1–R3).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `HelpNode` carries `Aliases []string` tagged `json:"aliases,omitempty"`, positioned immediately after `Name`; `schema_version` stays integer `1`.
- [x] A-002 R2: `buildNode` copies `cmd.Aliases` into every emitted node.
- [x] A-003 R3: Both test files are extended with alias assertions and all help-dump tests pass.

### Behavioral Correctness

- [x] A-004 R2: The emitted `list` node carries exactly `["ls"]`, `create` exactly `["new"]`, and `delete` exactly `["rm"]`.
- [x] A-005 R1: A non-aliased node (e.g. `open`) and the root marshal with **no** `aliases` key (verified against marshaled JSON, not just the struct).

### Scenario Coverage

- [x] A-006 R3: A struct-level test (`helpdump_test.go`) pins the aliased/non-aliased/`omitempty` contract via marshaled JSON.
- [x] A-007 R3: A command-level test (`help_dump_test.go`) confirms the live `wt help-dump` output carries the alias field on the three aliased nodes while the four-key envelope invariant holds.

### Edge Cases & Error Handling

- [x] A-008 R1: Non-aliased nodes emit no `aliases` key at all (not `"aliases":[]` and not `null`) — `omitempty` is verified.

### Code Quality

- [x] A-009 Pattern consistency: New struct field, JSON tag, and builder assignment follow the existing `HelpNode`/`buildNode` conventions; test additions mirror the existing table/marshaled-JSON assertion style.
- [x] A-010 No unnecessary duplication: Alias population reuses `cmd.Aliases` directly; no new helper or parsing logic introduced.
- [x] A-011 No magic strings: Test expectations reference the actual alias literals (`ls`/`new`/`rm`) that match the command declarations, not ad-hoc constants.
- [x] A-012 gofmt clean: `gofmt -l` reports no changes on touched files (CI fails fast on it).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. (The optional `aliases` field is purely additive; the alias-parsing that becomes redundant — `parse-help.ts` anchoring on the `Aliases:` section of `text` — lives in the shll.ai repo, not here.)

## Assumptions

<!-- SCORING SOURCE NOTE: `fab score` reads intake.md only — this section records
     graded decisions made while co-generating ## Requirements. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Emit an optional `aliases` field on Node (after `name`, `omitempty`, no `schema_version` bump), not per-alias Node entries; wt moves first | Fully determined by the intake's verified findings and the published standard's schema-evolution rule (optional-field additions under `schema_version: 1`); shll has no landed shape to match; per-alias nodes would corrupt tree/path semantics | S:80 R:80 A:90 D:85 |
| 2 | Certain | `text` field and all other builder behavior (envelope, filters, discovery, render detachment) unchanged | Cobra's default help template already renders the `Aliases:` section into `text`; the intake and memory contract confirm the structured field is the only gap | S:85 R:90 A:95 D:90 |
| 3 | Certain | Struct-level test extends `newTestRoot` with an aliased command rather than relying only on the command-level test, mirroring the existing per-invariant test split | Constitution IV (test what the user sees, builder unit test + command-level test) and the existing `helpdump_test.go` structure deterministically prescribe this; a synthetic aliased command exercises `omitempty` in isolation from the live tree | S:70 R:90 A:90 D:80 |

3 assumptions (3 certain, 0 confident, 0 tentative).
