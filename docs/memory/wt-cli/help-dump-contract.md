---
type: memory
description: "Contract for the hidden `wt help-dump` command — the JSON envelope shll.ai's scheduled puller consumes."
---
# wt-cli: Help-Dump Contract

> Post-implementation behavior capture for the Hidden `wt help-dump` command.
> Source change: `260603-qqkj-help-dump-command`.

This file documents the contract that `wt help-dump` honors. Future changes touching `src/internal/worktree/helpdump.go` or `src/cmd/wt/help_dump.go` should preserve these invariants unless an explicit spec amendment supersedes them.

`help-dump` is one half of a **cross-repo contract** with shll.ai: shll.ai's command-reference page for `wt` is refreshed by a scheduled puller that runs `wt help-dump`, `brew install`s the tool, and commits the captured JSON. The output shape, field semantics, and `schema_version` are fixed by that contract and are NOT open to local reinterpretation — see "Upstream forward contract" below. `wt`'s sole obligation is to emit valid help-dump output to stdout; everything downstream (capture, `captured_at` stamping, schema validation, commit) is shll.ai's job. This command is the producer half of the pull model — see § Design Decisions (Pull model over push).

## Requirements

### Hidden subcommand, self-filtering

- `help-dump` is declared `Hidden: true` in `helpDumpCmd()`. It NEVER appears in `wt -h`'s Available Commands list.
- Because it is Hidden, it also self-filters from its OWN output: the §4 filter drops every Hidden node, and `help-dump` is Hidden, so no special-case logic is needed to exclude it from the dumped tree.

- **GIVEN** a built `wt` binary
- **WHEN** the user runs `wt -h`
- **THEN** `help-dump` SHALL NOT appear in the Available Commands list
- **AND** running `wt help-dump` SHALL still execute the command
- **AND** `help-dump` SHALL be absent from the emitted tree

### Invocation contract: single JSON to stdout, empty stderr, exit 0

- On success, `wt help-dump` emits exactly ONE JSON envelope to **stdout** (pretty-printed via `json.MarshalIndent(doc, "", "  ")` with a single trailing newline), writes **nothing to stderr**, and **exits 0**.
- On ANY error (builder failure, JSON marshal failure, write failure), the command returns the error via `RunE`; the root handler in `main.go` maps the non-nil error to a typed non-zero exit code (`wt.ExitGeneralError`, per Constitution III). `Args: cobra.NoArgs` — a positional argument is rejected with a non-zero exit.
- The puller treats a non-zero exit as a **failed capture** and MUST NOT clobber its last-good `help/wt.json`. The single-JSON-to-stdout / empty-stderr / exit-0 triad is therefore the load-bearing success signal; any stray stderr write or partial stdout on the success path would be a contract violation.

- **GIVEN** the command runs successfully
- **WHEN** the JSON envelope is emitted
- **THEN** the process SHALL exit 0 with empty stderr and exactly one JSON document on stdout
- **AND** any internal failure SHALL surface a non-zero exit via the root handler

### Envelope shape: exactly `{tool, version, schema_version, root}`, no `captured_at`

- `HelpDoc` marshals to EXACTLY four keys: `tool`, `version`, `schema_version`, `root`. No more, no fewer.
- `tool` = `"wt"` (the `toolName` constant — the invoked binary name, not the file slug).
- `version` = the built binary's version, passed in from `main.version` (ldflags `-X main.version=...`). A plain `go build` reports `"dev"`; release builds inject the real version. It is NEVER hardcoded by the builder package.
- `schema_version` = the integer literal `1` (the `helpDumpSchemaVersion` constant; a Go `int`, not a string). Frozen for this contract revision.
- The envelope **MUST NOT** emit `captured_at`. `HelpDoc` has no `captured_at` field at all — not even an `omitempty` one, since adding it would drift from the contract. `captured_at` is **shll.ai-owned**: the puller stamps it post-capture. This is the deliberate asymmetry — the tool emits the structural help tree; shll.ai owns the timestamp.

- **GIVEN** the command emits its envelope
- **WHEN** the JSON top-level object is parsed
- **THEN** the keys SHALL be exactly `tool`, `version`, `schema_version`, `root`
- **AND** `captured_at` SHALL be absent
- **AND** `tool` SHALL equal `"wt"` and `schema_version` SHALL equal integer `1`

### Recursive Node shape: `{name, aliases?, path, short, usage, text, commands}`

- Each `HelpNode` marshals to its fields in contract order:
  - `name` = `cmd.Name()`
  - `aliases` = `cmd.Aliases` — the command's registered Cobra aliases, **optional** (`json:"aliases,omitempty"`), positioned immediately after `name` (see below).
  - `path` = `cmd.CommandPath()` (full invocation, e.g. `"wt create"`)
  - `short` = `cmd.Short`
  - `usage` = `cmd.UseLine()`
  - `text` = the raw `-h` render for that command (see below)
  - `commands` = the array of child `HelpNode`s.
- `aliases` is the **optional** member of the node shape: a nil/empty `Aliases` marshals with **no `aliases` key at all** (`omitempty`), so non-aliased nodes and the root are byte-identical to a shape without the field. Exactly three nodes emit it — `create` → `["new"]`, `delete` → `["rm"]`, `list` → `["ls"]` — and `buildNode` populates it directly from `cmd.Aliases`. This is the deliberate contrast with `commands`: `aliases` is absent when empty, while `commands` is always a present `[]`.
- `commands` is a **non-nil slice** (`make([]HelpNode, 0, ...)`), so a leaf marshals to `"commands": []`, never `null`. shll.ai's `NodeSchema` requires `z.array(NodeSchema)`; a `null` would fail validation. `aliases` carries no such requirement — it is added as an `.optional()` upstream field, so its absence MUST validate.

- **GIVEN** the root node
- **WHEN** marshaled
- **THEN** it SHALL carry its fields in contract order (`name`, then `path`, `short`, `usage`, `text`, `commands`)
- **AND** a leaf command's `commands` SHALL serialize as `[]`, not `null`
- **AND** the root SHALL carry no `aliases` key (it has no aliases)

- **GIVEN** an aliased command node (`create`, `delete`, or `list`)
- **WHEN** marshaled
- **THEN** it SHALL carry an `aliases` array immediately after `name` holding exactly its registered alias(es) — `["new"]`, `["rm"]`, `["ls"]` respectively
- **AND** a non-aliased node (e.g. `open`) SHALL carry no `aliases` key at all (not `"aliases": []`, not `null`)

### `text` is the raw `-h` render, captured into a buffer, byte-for-byte

- Each node's `text` is captured by `renderHelpText(cmd)`: it points `cmd.SetOut`/`cmd.SetErr` at a `bytes.Buffer`, invokes `cmd.Help()`, then returns `strings.TrimRight(buf.String(), "\n")` (the reference sample carries no trailing newline; Cobra's help render appends one, so it is trimmed to match byte-for-byte).
- `text` is NEVER produced by regex-parsing `-h` output, nor by manual `Long + UsageString()` composition — the capture-and-render-into-buffer method (verified byte-for-byte against the committed `help/wt.json` reference sample) is the mandated approach.

- **GIVEN** the `create` node
- **WHEN** its `text` is compared to the reference sample's `create.text`
- **THEN** they SHALL be byte-identical (modulo `version`/`captured_at`)

### Discovery: recursive walk of `rootCmd.Commands()`, never regex

- The tree is discovered programmatically by `buildNode` walking `cmd.Commands()` recursively to **full depth**. It NEVER regex-parses `-h` text to discover structure.
- `wt` is currently flat (root + 9 visible leaves), but the walk recurses for correctness under any future nesting.

- **GIVEN** the root command with its registered children
- **WHEN** the builder walks the tree
- **THEN** every non-filtered descendant SHALL appear at its correct depth

### Filter rules: drop `completion`, `help`, and any Hidden node

- `isFilteredCommand(cmd)` drops a node when ANY of:
  - `cmd.Hidden == true` (this self-filters `help-dump`), OR
  - `cmd.Name()` is `"completion"` or `"help"` (Cobra's auto-generated subcommands).
- Filtering is applied both when deciding which children to recurse into AND, during each node's text render, by temporarily detaching the filtered children so the rendered "Available Commands" listing matches the dumped tree (see "Render-time child detachment" below).

- **GIVEN** the live command tree (which includes auto-generated `completion`, `help`, and the Hidden `help-dump`)
- **WHEN** the builder walks it
- **THEN** `completion`, `help`, and `help-dump` SHALL be absent from the output tree
- **AND** the remaining 9 visible subcommands (`create`, `delete`, `go`, `init`, `list`, `open`, `shell-init`, `skill`, `update`) SHALL be present

### `schema_version: 1`; optional fields evolve without a bump

- `schema_version` is the integer `1`. Under the standard's schema-evolution rule, a **new OPTIONAL field is added without a `schema_version` bump** — each tool adopts it on its own release cadence, older captures keep validating, and a consumer pinned to `schema_version: 1` is unaffected. The node's optional `aliases` field is exactly such an addition (present on the three aliased nodes, absent elsewhere), added under `schema_version: 1` without a bump.
- A **breaking** shape change (removing or renaming a field, changing a field's type, or making a previously-optional field required) is the only kind that bumps `schema_version`; it is a separate, deliberate change.
- The envelope shape is fixed at exactly `{tool, version, schema_version, root}` — its four keys are frozen (an added envelope-level field would itself be an optional-field evolution, but none exists today).

### Conformance to the reference sample and upstream schema

- The emitted output, after shll.ai adds `captured_at` and with `version` normalized, SHALL validate against shll.ai's `HelpDocSchema`/`NodeSchema` and match the committed `help/wt.json` structure: same 9 subcommands, same field names/shapes, same `text`/`short`/`usage`/`path` per node, plus the optional `aliases` on the three aliased nodes.

- **GIVEN** `wt help-dump` output with `version` normalized and `captured_at` inserted
- **WHEN** diffed against the committed reference sample
- **THEN** the structures SHALL be equivalent

## Internal API

- `worktree.BuildHelpDump(root *cobra.Command, version string) (HelpDoc, error)` is the single entry point. The builder takes the root command and the version as arguments because the root is constructed in package `main` and the internal package cannot reach it otherwise — this keeps the tree-walk/envelope logic in `internal/worktree/` (Constitution V) while `main` retains ownership of command registration.
- The `cmd/wt/help_dump.go` layer is **thin** (Constitution V): `helpDumpCmd()` only wires Cobra to the builder, passing `cmd.Root()` and the package-`main` `version`, marshals the result, writes JSON to `cmd.OutOrStdout()`, and returns any error via `RunE`.
- `HelpDoc` / `HelpNode` are exported structs; `buildNode`, `initHelpTree`, `isFilteredCommand`, and `renderHelpText` are unexported helpers internal to the builder.

## Design Decisions

### Builder takes the root `*cobra.Command` as input

`BuildHelpDump(root *cobra.Command, version string)` receives the root and version rather than constructing its own tree. The root is owned by package `main`; passing it keeps tree-walk/envelope logic in `internal/worktree/` per Constitution V. Building the tree inside `cmd/` was rejected (violates V); having the builder construct its own root was rejected (would duplicate command registration and drift from the live tree). (Source: change qqkj, plan Design Decision 1.)

### Initialize Cobra's lazy help affordances across the whole tree before rendering

Cobra adds the `-h, --help` flag, the `-v, --version` flag, and the auto-generated `help`/`completion` subcommands **lazily** — normally during `Execute()`, and only on the command actually being run. When `help-dump` is the executed command, the *root* is initialized but its descendants are not: each leaf's rendered `-h` would omit the `-h, --help` line and drop the `[flags]` suffix from its `UseLine()`, and the root's `-h` would lack `-v, --version`. `initHelpTree(root)` walks the whole tree up front and calls `InitDefaultHelpFlag`, `InitDefaultVersionFlag` (a no-op when `cmd.Version` is empty), `InitDefaultHelpCmd` (on commands with children), and `InitDefaultCompletionCmd` (root only) so every rendered `-h` matches a real `command -h` invocation and the reference sample. All initializers are idempotent. (Source: change qqkj.)

### Render-time child detachment, not a `Hidden` toggle

`renderHelpText` temporarily **detaches** filtered children (`completion`/`help`/Hidden) via `cmd.RemoveCommand(...)` during the buffer render, then re-attaches them with a `defer cmd.AddCommand(...)`. This makes each command's rendered "Available Commands" listing reflect the dumped tree (matching the reference sample, which omits those entries). Detachment is required rather than toggling `Hidden`: Cobra's usage template special-cases the `help` command with an explicit `(eq .Name "help")` clause that lists it even when Hidden — only removing it from the children slice keeps it out of the listing. The detached children are re-attached before returning (Cobra re-sorts on `AddCommand`, restoring order), and the `SetOut`/`SetErr` overrides are restored via `defer`, so the **live tree is provably unmutated** — a normal `wt -h` for real users is unaffected after a dump (asserted by `TestBuildHelpDump_RestoresLiveTree`). (Source: change qqkj.)

### `commands` is a non-nil slice (`[]`, not `null`)

`HelpNode.Commands` is initialized to `make([]HelpNode, 0, ...)` so a leaf marshals to `"commands": []`. shll.ai's `NodeSchema` is `z.array(NodeSchema)`; a `null` (which a nil slice would produce) fails validation. (Source: change qqkj, plan Design Decision 3.)

### `aliases` is an optional node field (`omitempty`), populated straight from `cmd.Aliases`

**Decision**: The node carries an optional `Aliases []string` tagged `json:"aliases,omitempty"`, placed immediately after `Name`; `buildNode` copies `cmd.Aliases` verbatim, so a command's registered Cobra aliases surface in the structured tree (e.g. `list` → `["ls"]`). A node with no aliases marshals with no `aliases` key.
**Why**: The alias forms `ls`/`new`/`rm` are real, working invocations, but they lived only inside `text` (Cobra's `Aliases:` section) — invisible to any consumer that reads the structured fields. shll.ai's README-drift checker builds its known-command set from `node.commands[].name` alone, so alias-form invocations documented in the README were flagged as fabricated commands. An optional structured field makes them discoverable. `omitempty` (absence, not an always-present `[]`) minimizes capture drift — only the three aliased nodes change — and absence MUST validate, because the field is `.optional()` upstream and the other toolkit binaries won't emit it until they adopt. This is the deliberate contrast with `commands`' non-nil-`[]` rule, which exists only because that field is already required by shll's `z.array`.
**Why an optional field, not per-alias sibling nodes**: An alias is a *property of a command*, not a sibling command. Emitting `ls`/`new`/`rm` as additional Node entries would corrupt tree semantics — phantom `helpFacts.commandPaths`, duplicated `text`, and misreported `path`/`usage` — rendering them as separate full commands in the site's command-reference tree.
**Rejected**: additional Node entries per alias (corrupts tree/path semantics as above); an always-present `"aliases": []` (unnecessary capture drift; the non-nil-`[]` rule is `commands`-specific).
*Introduced by*: 260718-zb77-help-dump-emit-aliases

wt is the **first mover** on this field — it defined the shape per the standard's schema-evolution rule (260718-zb77). The **consumer half is pending in shll.ai** (separate cross-repo work, not in this repo): `NodeSchema` gains `aliases: z.array(z.string()).optional()`, `helpFacts` folds child aliases into the known-command set, and the standard page documents the field. Emitting ahead of the consumer is safe — `NodeSchema` is a non-strict `z.object`, so the unknown `aliases` key passes validation and rides along in the puller's captured JSON until the consumer reads it. The README-drift false positives clear only once shll.ai's consumer half ships; wt's obligation ends at emitting a conformant dump.

### No `captured_at` field on the struct at all

`HelpDoc` deliberately has no `captured_at` field — not even an `omitempty` one. The contract §3 asymmetry is that the tool emits the structural tree and shll.ai stamps the timestamp post-capture; adding the field (even unset) would risk emitting it and drift from the contract. The tool's envelope is structurally incapable of carrying `captured_at`. (Source: change qqkj, intake assumption #2.)

### Pull model over push

**Decision**: shll.ai *pulls* — its scheduled puller runs `wt help-dump`, `brew install`s the tool, stamps `captured_at`, and commits the captured JSON. `wt` ships no push wiring: no build-time CI step, no PR-opening into sahil87/shll.ai, no `SHLLAI_TOKEN` use.
**Why**: the pull model keeps `wt`'s obligation to a single deterministic stdout emission and centralizes capture, stamping, validation, and commit on the consumer side — avoiding the multi-repo push race the push design had to work around.
**Rejected**: the push model (backlog `[pc47]`, marked superseded — `wt` PR-ing `help/wt.json` into shll.ai with auto-merge); its wiring was deliberately never built and must not be reintroduced.
*Introduced by*: `260603-qqkj-help-dump-command`

## Upstream forward contract

- The authoritative cross-repo contract lives in shll.ai: `docs/specs/help-dump-contract.md`. That document (its §1–§8) defines invocation, envelope, node shape, filtering, discovery, version sourcing, and schema-freeze rules; this memory file captures `wt`'s conforming implementation of it. When the two diverge, the shll.ai contract is authoritative.
- The machine-checkable conformance anchor is shll.ai's `sites/astro-starlight-terminal1/src/lib/schemas.ts` — `HelpDocSchema` and `NodeSchema`. After the puller stamps `captured_at`, `wt help-dump` output MUST validate against these Zod schemas. A change to `wt`'s output shape that breaks those schemas is a contract violation on `wt`'s side.

## Cross-references

- Source: `src/internal/worktree/helpdump.go` — `HelpDoc`, `HelpNode`, `BuildHelpDump`, `initHelpTree`, `buildNode`, `isFilteredCommand`, `renderHelpText`, the `helpDumpSchemaVersion = 1` and `toolName = "wt"` constants. `src/cmd/wt/help_dump.go` — `helpDumpCmd()` (thin Cobra wiring). `src/cmd/wt/main.go` — registers `helpDumpCmd()` on root; owns `var version = "dev"` (ldflags-injected).
- Tests: `src/internal/worktree/helpdump_test.go` — `TestBuildHelpDump_Envelope`, `TestBuildHelpDump_OmitsCapturedAt`, `TestBuildHelpDump_FiltersCompletionHelpHidden`, `TestBuildHelpDump_RecursiveDiscovery`, `TestBuildHelpDump_NodeShape` (asserts `-h, --help` line present + leaf `commands: []`), `TestBuildHelpDump_NodeAliases` (aliased node carries its exact alias list; non-aliased node + root emit no `aliases` key, asserted on marshaled JSON to pin `omitempty`), `TestBuildHelpDump_RestoresLiveTree` (live tree unmutated). `src/cmd/wt/help_dump_test.go` — `TestHelpDump_EmitsValidEnvelope` (exit 0, empty stderr, exactly the four top-level keys, no `captured_at`, 9 subcommands, banned names absent), `TestHelpDump_EmitsAliases` (emitted `list`/`create`/`delete` carry `["ls"]`/`["new"]`/`["rm"]`; non-aliased nodes + root carry no `aliases` key), `TestHelpDump_HiddenFromRootHelp`, `TestHelpDump_RejectsArgs` (`cobra.NoArgs`).
- Constitution: Principle II (Cobra command surface — `RunE`, `SilenceUsage`/`SilenceErrors` inherited from root), III (Typed exit codes — builder/marshal errors map to `ExitGeneralError`), IV (test what the user sees — builder unit test + command-level test), V (internal package boundary — tree-walk/envelope logic in `internal/worktree`, `cmd/` thin).
- Sibling memory: `wt-cli/init-failure-contract.md`, `wt-cli/list-status-contract.md`, `wt-cli/update-command-contract.md` — same pattern of post-change invariant capture for other `wt` subcommands. `update-command-contract.md` documents another **cross-toolkit** contract (`--skip-brew-update`) whose semantics are likewise fixed externally and not open to local reinterpretation.
- Backlog: `[pc47]` is marked **superseded by qqkj** in `fab/backlog.md` — its producer half is realized here; its push half (build-time CI step, PR-opening into sahil87/shll.ai, auto-merge, `SHLLAI_TOKEN`) was intentionally dropped per the pull-model inversion.
- Upstream: shll.ai `docs/specs/help-dump-contract.md` (authoritative cross-repo contract) and `sites/astro-starlight-terminal1/src/lib/schemas.ts` (`HelpDocSchema`/`NodeSchema`, machine-checkable conformance anchor).
