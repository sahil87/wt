# wt-cli: Help-Dump Contract

> Post-implementation behavior capture for the Hidden `wt help-dump` command.
> Source change: `260603-qqkj-help-dump-command`.

This file documents the contract that `wt help-dump` honors. Future changes touching `src/internal/worktree/helpdump.go` or `src/cmd/wt/help_dump.go` should preserve these invariants unless an explicit spec amendment supersedes them.

`help-dump` is one half of a **cross-repo contract** with shll.ai: shll.ai's command-reference page for `wt` is refreshed by a scheduled puller that runs `wt help-dump`, `brew install`s the tool, and commits the captured JSON. The output shape, field semantics, and `schema_version` are fixed by that contract and are NOT open to local reinterpretation — see "Upstream forward contract" below. `wt`'s sole obligation is to emit valid help-dump output to stdout; everything downstream (capture, `captured_at` stamping, schema validation, commit) is shll.ai's job. The prior **push** model (where `wt` would PR `help/wt.json` into shll.ai — backlog `[pc47]`) is **retired**; this command is the producer half of the inverted pull model and the push wiring was deliberately never built.

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

### Recursive Node shape: `{name, path, short, usage, text, commands}`

- Each `HelpNode` marshals to six fields in contract order:
  - `name` = `cmd.Name()`
  - `path` = `cmd.CommandPath()` (full invocation, e.g. `"wt create"`)
  - `short` = `cmd.Short`
  - `usage` = `cmd.UseLine()`
  - `text` = the raw `-h` render for that command (see below)
  - `commands` = the array of child `HelpNode`s.
- `commands` is a **non-nil slice** (`make([]HelpNode, 0, ...)`), so a leaf marshals to `"commands": []`, never `null`. shll.ai's `NodeSchema` requires `z.array(NodeSchema)`; a `null` would fail validation.

- **GIVEN** the root node
- **WHEN** marshaled
- **THEN** it SHALL carry the six fields in contract order
- **AND** a leaf command's `commands` SHALL serialize as `[]`, not `null`

### `text` is the raw `-h` render, captured into a buffer, byte-for-byte

- Each node's `text` is captured by `renderHelpText(cmd)`: it points `cmd.SetOut`/`cmd.SetErr` at a `bytes.Buffer`, invokes `cmd.Help()`, then returns `strings.TrimRight(buf.String(), "\n")` (the reference sample carries no trailing newline; Cobra's help render appends one, so it is trimmed to match byte-for-byte).
- `text` is NEVER produced by regex-parsing `-h` output, nor by manual `Long + UsageString()` composition — the capture-and-render-into-buffer method (verified byte-for-byte against the committed `help/wt.json` reference sample) is the mandated approach.

- **GIVEN** the `create` node
- **WHEN** its `text` is compared to the reference sample's `create.text`
- **THEN** they SHALL be byte-identical (modulo `version`/`captured_at`)

### Discovery: recursive walk of `rootCmd.Commands()`, never regex

- The tree is discovered programmatically by `buildNode` walking `cmd.Commands()` recursively to **full depth**. It NEVER regex-parses `-h` text to discover structure.
- `wt` is currently flat (root + 7 visible leaves), but the walk recurses for correctness under any future nesting.

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
- **AND** the remaining 7 visible subcommands (`create`, `delete`, `init`, `list`, `open`, `shell-init`, `update`) SHALL be present

### Schema is frozen at `schema_version: 1`

- `schema_version` stays the integer `1` for this contract revision. No new fields are added to the envelope or node shape in this change.
- Future enrichment is a separate, deliberate change and SHALL add new fields as **OPTIONAL** (so a consumer pinned to `schema_version: 1` keeps validating). A breaking shape change would bump `schema_version`.

### Conformance to the reference sample and upstream schema

- The emitted output, after shll.ai adds `captured_at` and with `version` normalized, SHALL validate against shll.ai's `HelpDocSchema`/`NodeSchema` and match the committed `help/wt.json` structure: same 7 subcommands, same field names/shapes, same `text`/`short`/`usage`/`path` per node.

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

### No `captured_at` field on the struct at all

`HelpDoc` deliberately has no `captured_at` field — not even an `omitempty` one. The contract §3 asymmetry is that the tool emits the structural tree and shll.ai stamps the timestamp post-capture; adding the field (even unset) would risk emitting it and drift from the contract. The tool's envelope is structurally incapable of carrying `captured_at`. (Source: change qqkj, intake assumption #2.)

## Upstream forward contract

- The authoritative cross-repo contract lives in shll.ai: `docs/specs/help-dump-contract.md`. That document (its §1–§8) defines invocation, envelope, node shape, filtering, discovery, version sourcing, and schema-freeze rules; this memory file captures `wt`'s conforming implementation of it. When the two diverge, the shll.ai contract is authoritative.
- The machine-checkable conformance anchor is shll.ai's `sites/astro-starlight-terminal1/src/lib/schemas.ts` — `HelpDocSchema` and `NodeSchema`. After the puller stamps `captured_at`, `wt help-dump` output MUST validate against these Zod schemas. A change to `wt`'s output shape that breaks those schemas is a contract violation on `wt`'s side.

## Cross-references

- Source: `src/internal/worktree/helpdump.go` — `HelpDoc`, `HelpNode`, `BuildHelpDump`, `initHelpTree`, `buildNode`, `isFilteredCommand`, `renderHelpText`, the `helpDumpSchemaVersion = 1` and `toolName = "wt"` constants. `src/cmd/wt/help_dump.go` — `helpDumpCmd()` (thin Cobra wiring). `src/cmd/wt/main.go` — registers `helpDumpCmd()` on root; owns `var version = "dev"` (ldflags-injected).
- Tests: `src/internal/worktree/helpdump_test.go` — `TestBuildHelpDump_Envelope`, `TestBuildHelpDump_OmitsCapturedAt`, `TestBuildHelpDump_FiltersCompletionHelpHidden`, `TestBuildHelpDump_RecursiveDiscovery`, `TestBuildHelpDump_NodeShape` (asserts `-h, --help` line present + leaf `commands: []`), `TestBuildHelpDump_RestoresLiveTree` (live tree unmutated). `src/cmd/wt/help_dump_test.go` — `TestHelpDump_EmitsValidEnvelope` (exit 0, empty stderr, exactly the four top-level keys, no `captured_at`, 7 subcommands, banned names absent), `TestHelpDump_HiddenFromRootHelp`, `TestHelpDump_RejectsArgs` (`cobra.NoArgs`).
- Constitution: Principle II (Cobra command surface — `RunE`, `SilenceUsage`/`SilenceErrors` inherited from root), III (Typed exit codes — builder/marshal errors map to `ExitGeneralError`), IV (test what the user sees — builder unit test + command-level test), V (internal package boundary — tree-walk/envelope logic in `internal/worktree`, `cmd/` thin).
- Sibling memory: `wt-cli/init-failure-contract.md`, `wt-cli/list-status-contract.md`, `wt-cli/update-command-contract.md` — same pattern of post-change invariant capture for other `wt` subcommands. `update-command-contract.md` documents another **cross-toolkit** contract (`--skip-brew-update`) whose semantics are likewise fixed externally and not open to local reinterpretation.
- Backlog: `[pc47]` is marked **superseded by qqkj** in `fab/backlog.md` — its producer half is realized here; its push half (build-time CI step, PR-opening into sahil87/shll.ai, auto-merge, `SHLLAI_TOKEN`) was intentionally dropped per the pull-model inversion.
- Upstream: shll.ai `docs/specs/help-dump-contract.md` (authoritative cross-repo contract) and `sites/astro-starlight-terminal1/src/lib/schemas.ts` (`HelpDocSchema`/`NodeSchema`, machine-checkable conformance anchor).

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260603-qqkj-help-dump-command` | 2026-06-03 | Added the Hidden `wt help-dump` Cobra subcommand for shll.ai's scheduled pull integration. Emits a single JSON envelope to stdout (exactly `{tool, version, schema_version, root}`; `tool=="wt"`, `version` from `main.version` ldflags, `schema_version` integer `1`) with empty stderr and exit 0 on success; non-zero (typed via `RunE` → `ExitGeneralError`) on any error so the puller treats it as a failed capture. The envelope deliberately OMITS `captured_at` (shll.ai stamps it post-capture). Recursive `HelpNode` shape `{name, path, short, usage, text, commands}` with `text` = the raw `-h` render captured into a buffer (trailing newline trimmed, byte-for-byte vs the reference sample) and `commands` a non-nil slice (`[]` for leaves, never `null`). Tree discovered by recursively walking `rootCmd.Commands()` (never regex-parsing `-h`), filtering `completion`, `help`, and any Hidden node (self-filtering `help-dump`). Two Cobra subtleties handled: `initHelpTree` initializes the lazily-added help/version/completion affordances across the whole tree before rendering, and `renderHelpText` temporarily detaches filtered children during each node's render (working around Cobra's `help`-special-casing usage template) then restores them so the live tree is unmutated. Logic lives in `src/internal/worktree/helpdump.go` (`BuildHelpDump`); thin wiring in `src/cmd/wt/help_dump.go`. Superseded the obsolete push half of backlog `[pc47]`. |
