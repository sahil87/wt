# Plan: Implement `wt help-dump` for shll.ai pull integration

**Change**: 260603-qqkj-help-dump-command
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### help-dump: Command Surface & Wiring

#### R1: Hidden Cobra subcommand registered on root
A `helpDumpCmd() *cobra.Command` constructor SHALL be added in `src/cmd/wt/help_dump.go` and registered on the root command in `src/cmd/wt/main.go` via `root.AddCommand(...)`. The command SHALL set `Hidden: true`. Per Constitution II it SHALL inherit `SilenceUsage`/`SilenceErrors` (root sets these) and return errors via `RunE`. Per Constitution V the `cmd/` layer SHALL be thin: it only invokes the internal builder and writes JSON to stdout.

- **GIVEN** a built `wt` binary
- **WHEN** the user runs `wt -h`
- **THEN** `help-dump` SHALL NOT appear in the Available Commands list (it is Hidden)
- **AND** running `wt help-dump` SHALL still execute the command

#### R2: Typed exit code on error
Any error from the builder or JSON marshal SHALL be returned via `RunE`; the root handler in `main.go` maps a non-nil error to `wt.ExitGeneralError` (exit 1). On success the command SHALL exit 0.

- **GIVEN** the command runs successfully
- **WHEN** the JSON envelope is emitted
- **THEN** the process SHALL exit 0 with empty stderr
- **AND** any internal failure SHALL surface a non-zero exit via the root handler

### help-dump: Envelope & Node Shape (contract §3)

#### R3: Top-level envelope omits captured_at
The builder SHALL produce an envelope marshaling EXACTLY to `{tool, version, schema_version, root}`. It SHALL NOT emit `captured_at` (shll.ai stamps it post-capture). `tool` = `"wt"`; `version` = the built binary's version (passed in from `main.version` / `rootCmd.Version`), never hardcoded; `schema_version` = integer literal `1`.

- **GIVEN** the command emits its envelope
- **WHEN** the JSON is parsed
- **THEN** keys SHALL be exactly `tool`, `version`, `schema_version`, `root`
- **AND** `captured_at` SHALL be absent
- **AND** `schema_version` SHALL equal integer `1`

#### R4: Recursive Node shape
Each node SHALL marshal to `{name, path, short, usage, text, commands}` where `name`=`cmd.Name()`, `path`=`cmd.CommandPath()`, `short`=`cmd.Short`, `usage`=`cmd.UseLine()`, `text`= the raw `-h` render for that command, `commands`= the array of child Nodes (`[]`, never `null`, for a leaf).

- **GIVEN** the root node
- **WHEN** marshaled
- **THEN** it SHALL carry the six fields in contract order
- **AND** a leaf command's `commands` SHALL serialize as `[]`, not `null`

#### R5: `text` is the raw `-h` render, byte-for-byte
Each node's `text` SHALL be captured by rendering that command's help into a buffer (set the command's output to a `bytes.Buffer` and invoke Cobra's help rendering), trimmed of any trailing newline, matching the committed `help/wt.json` reference sample byte-for-byte (modulo `version`/`captured_at`). It SHALL NOT be produced by regex-parsing or manual Long+UsageString composition.

- **GIVEN** the `create` node
- **WHEN** its `text` is compared to the reference sample's `create.text`
- **THEN** they SHALL be byte-identical

### help-dump: Discovery & Filtering (contract §4, §5)

#### R6: Recursive full-depth discovery
The tree SHALL be discovered by walking `rootCmd.Commands()` recursively to full depth — never by parsing `-h` text. (wt is currently flat: root + 7 leaves, but the walk SHALL recurse for correctness under future nesting.)

- **GIVEN** the root command with its registered children
- **WHEN** the builder walks the tree
- **THEN** every non-filtered descendant SHALL appear at its correct depth

#### R7: Filter completion, help, and Hidden nodes
The walk SHALL DROP Cobra's auto-generated `completion` and `help` subcommands and any node with `cmd.Hidden == true`. Because `help-dump` is itself Hidden (R1), this rule self-filters it with no special-case logic.

- **GIVEN** the live command tree (which includes auto-generated `completion`, `help`, and the Hidden `help-dump`)
- **WHEN** the builder walks it
- **THEN** `completion`, `help`, and `help-dump` SHALL be absent from the output tree
- **AND** the remaining 7 subcommands SHALL be present

### help-dump: Conformance & Test (contract §8, Constitution IV)

#### R8: Conformance to reference sample and schema
The emitted output, after shll.ai adds `captured_at`, SHALL validate against `HelpDocSchema`/`NodeSchema` and match the committed `help/wt.json` structure: same 7 subcommands, same field names/shapes, same `text`/`short`/`usage`/`path` per node. `schema_version` SHALL stay `1`; no new fields.

- **GIVEN** `wt help-dump` output with `version` normalized and `captured_at` inserted
- **WHEN** diffed against the committed reference sample
- **THEN** the structures SHALL be equivalent

#### R9: Dedicated tests for the command and builder
Tests SHALL be added (test-alongside): a builder test in `src/internal/worktree/helpdump_test.go` and a command test in `src/cmd/wt/help_dump_test.go` asserting exit 0, valid JSON, `tool=="wt"`, `schema_version==1`, no `captured_at` key, and `completion`/`help`/`help-dump` absent from the tree.

- **GIVEN** the test suite
- **WHEN** `go test ./cmd/... ./internal/...` runs
- **THEN** the new tests SHALL pass

### Non-Goals

- Push wiring from backlog `[pc47]` (producer CI, PR-opening, auto-merge, `SHLLAI_TOKEN`) — retired by the pull-model inversion; explicitly NOT built.
- shll.ai-side puller, `captured_at` stamping, JSON validation, commit — shll.ai's responsibility.
- Schema enrichment (new optional fields) — separate future change; stay at `schema_version: 1`.

### Design Decisions

1. **Builder takes a `*cobra.Command` (the root) as input**: the root is constructed in package `main`; the internal builder cannot reach it otherwise. — *Why*: keeps tree-walk/envelope logic in `internal/worktree/` (Constitution V) while the root stays owned by `main`. — *Rejected*: building the tree inside `cmd/` (violates V); having the builder construct its own root (would duplicate command registration and drift).
2. **`text` captured via buffer render of `cmd.Help()`**: set `cmd.SetOut(buf)` + `cmd.SetErr(buf)` and invoke the command's help func, then `strings.TrimRight(..., "\n")`. — *Why*: contract §3 + intake assumption #12 mandate the raw `-h` render verified byte-for-byte. — *Rejected*: manual `Long+UsageString` composition (drifts from real `-h`; intake explicitly disfavors it).
3. **`commands` is a non-nil slice**: initialize `[]Node{}` so leaves marshal to `[]` not `null`, matching `NodeSchema`/sample. — *Why*: schema is `z.array(NodeSchema)`; `null` would fail validation.

## Tasks

### Phase 1: Core Implementation (internal builder)

- [x] T001 Add `src/internal/worktree/helpdump.go`: define exported `HelpDoc` envelope struct (`tool`, `version`, `schema_version`, `root` JSON tags; NO `captured_at` field) and `HelpNode` struct (`name`, `path`, `short`, `usage`, `text`, `commands` JSON tags), plus `BuildHelpDump(root *cobra.Command, version string) (HelpDoc, error)` and a recursive `buildNode(cmd *cobra.Command) (HelpNode, error)` helper that walks children, filters `completion`/`help`/Hidden, captures `text` via buffer-rendered help, and returns `[]HelpNode{}` for leaves. <!-- R3 R4 R5 R6 R7 -->

### Phase 2: Cobra Wiring (thin cmd layer)

- [x] T002 Add `src/cmd/wt/help_dump.go`: `helpDumpCmd() *cobra.Command` with `Use: "help-dump"`, `Hidden: true`, `Args: cobra.NoArgs`, and a `RunE` that calls `worktree.BuildHelpDump(cmd.Root(), version)`, marshals with `json.MarshalIndent(doc, "", "  ")`, writes the JSON (+ trailing newline) to `cmd.OutOrStdout()`, and returns any error. <!-- R1 R2 R3 -->
- [x] T003 Register `helpDumpCmd()` on the root in `src/cmd/wt/main.go` via `root.AddCommand(...)`. <!-- R1 -->

### Phase 3: Tests

- [x] T004 [P] Add `src/internal/worktree/helpdump_test.go`: unit-test `BuildHelpDump` against a synthesized root (root + leaves + a Hidden cmd + auto `completion`/`help`): assert envelope fields, no `captured_at` (via JSON marshal key check), `schema_version==1`, Hidden/`completion`/`help` filtered, leaf `commands` is `[]`, and `text`/`path`/`usage` populated. <!-- R3 R4 R5 R6 R7 R9 -->
- [x] T005 [P] Add `src/cmd/wt/help_dump_test.go`: run the built binary `wt help-dump` via `runWt`; assert exit 0, empty stderr, valid JSON, `tool=="wt"`, `schema_version==1`, no `captured_at` key, `help-dump`/`completion`/`help` absent from the tree, 7 subcommands present, and that `help-dump` is absent from `wt -h`. <!-- R1 R2 R8 R9 -->

### Phase 4: Conformance & Backlog Hygiene

- [x] T006 Build the binary, run `wt help-dump`, diff against the committed `help/wt.json` (normalize `version`, drop `captured_at`); resolve any byte-level `text` mismatch by adjusting the capture method until identical. <!-- R5 R8 -->
- [x] T007 Edit `fab/backlog.md`: mark `[pc47]` done/superseded by `qqkj`, noting the push half is intentionally dropped per the pull-model inversion. <!-- R8 -->

## Execution Order

- T001 blocks T002 (cmd wires to the builder) and T004 (tests the builder).
- T002 blocks T003 and T005.
- T006 requires T001–T003 (needs a buildable binary).
- T004 and T005 are independent of each other once their deps are met.

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `help_dump.go` defines `helpDumpCmd()` with `Hidden: true`, registered on root in `main.go`; `help-dump` absent from `wt -h`.
- [ ] A-002 R2: errors return via `RunE`; success exits 0 with empty stderr; failure exits non-zero.
- [ ] A-003 R3: envelope marshals to exactly `{tool, version, schema_version, root}`, no `captured_at`, `tool=="wt"`, `schema_version==1` (integer), `version` from passed-in binary version.
- [ ] A-004 R4: each node has `{name, path, short, usage, text, commands}`; leaf `commands` is `[]` not `null`.
- [ ] A-005 R5: each node's `text` matches the committed reference sample byte-for-byte (modulo version/captured_at).
- [ ] A-006 R6: discovery walks `rootCmd.Commands()` recursively; no `-h` text parsing.
- [ ] A-007 R7: `completion`, `help`, and `help-dump` filtered from the tree; 7 subcommands remain.

### Behavioral Correctness

- [ ] A-008 R8: `wt help-dump` output (version normalized, `captured_at` inserted) is structurally equivalent to committed `help/wt.json`.

### Scenario Coverage

- [ ] A-009 R9: builder unit test and command test exist and pass under `go test ./cmd/... ./internal/...`.

### Edge Cases & Error Handling

- [ ] A-010 R7: a Hidden node anywhere in the tree (including `help-dump` itself) is excluded without special-case logic.
- [ ] A-011 R4: `commands` serializes as `[]` for leaves (no `null` that would fail `NodeSchema`).

### Code Quality

- [ ] A-012 Pattern consistency: new code follows the `xxxCmd() *cobra.Command` constructor pattern and internal-package boundary (Constitution V); naming matches surrounding files.
- [ ] A-013 No unnecessary duplication: reuses existing `version` var and root handler; no reimplemented help rendering.
- [ ] A-014 No god functions (>50 lines without reason); no magic strings — `"wt"`/schema constant are justified literals tied to the contract.
- [ ] A-015 gofmt clean (module root `src/`) and `go vet ./...` clean.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Builder `BuildHelpDump(root *cobra.Command, version string)` takes the root + version as args; `cmd/` passes `cmd.Root()` and `main.version` | Root lives in package `main`; passing it keeps logic in `internal/` per Constitution V. Intake Impact section prescribes this split | S:95 R:70 A:90 D:85 |
| 2 | Certain | `text` captured via `cmd.SetOut(buf)`+`cmd.Help()` then `TrimRight("\n")` | Reference sample has no trailing newline on `text`; Cobra's help render appends one — trim to match byte-for-byte (intake assumption #12) | S:90 R:60 A:80 D:75 |
| 3 | Certain | JSON emitted with `MarshalIndent(…, "", "  ")` + trailing newline to stdout | Reference `help/wt.json` is 2-space-indented pretty JSON; matches sample formatting | S:85 R:75 A:85 D:80 |
| 4 | Certain | `schema_version` modeled as a Go `int` field set to `1` | Contract §8 + `z.literal(1)` require integer `1`, not string | S:98 R:85 A:95 D:95 |
| 5 | Certain | `helpDumpCmd` uses `cobra.NoArgs` | Consistent with `update` cmd; help-dump takes no positional args | S:80 R:85 A:85 D:80 |

5 assumptions (5 certain, 0 confident, 0 tentative).
