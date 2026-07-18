# worktree-cli Constitution

## Core Principles

### I. Single-Binary CLI, No Hidden State
`wt` SHALL ship as a single self-contained Go binary with no runtime dependencies beyond `git`. All persistent state SHALL live in the user's git repository (worktrees) or shell environment (via `wt shell-init` eval output). The CLI MUST NOT write hidden config files outside the repository or modify global git configuration without explicit user action. Rationale: a worktree helper that mutates global state is a footgun; users must be able to uninstall the binary and walk away cleanly.

### II. Cobra Command Surface
New subcommands SHALL be added via `cobra.Command` definitions in `src/cmd/`, registered on the root command in `main.go`. Each subcommand MUST set `SilenceUsage: true` and `SilenceErrors: true` and return errors via `RunE` so the root handler controls exit-code mapping. Flags SHALL use long-form names with single-letter short flags only when they aid common interactive use. Rationale: consistent command structure makes the tool predictable and keeps error rendering centralized.

### III. Typed Exit Codes
The CLI SHALL exit with a stable, documented set of exit codes defined in `internal/worktree/errors.go` (e.g., `ExitGeneralError`, `ExitUserAbort`). Subcommands MUST map domain errors to these codes — never to ad-hoc integers — so shell wrappers and scripts can branch on outcome reliably. Rationale: `wt` is invoked from shell aliases and scripts; ambiguous exit codes break automation silently.

### IV. Test What the User Sees
Every subcommand SHALL have unit tests covering its happy path and at least one failure path, plus integration tests in `cmd/integration_test.go` that exercise the binary end-to-end against a real git repo (using `t.TempDir()`). Test files SHALL live alongside the code they test (`create.go` ↔ `create_test.go`). Rationale: worktree operations touch the filesystem and git state — mocked tests miss the bugs that matter.

### V. Internal Package Boundary
All non-trivial logic SHALL live under `src/internal/worktree/` and be exercised through the `cmd/` layer. The `cmd/` package SHALL contain only flag parsing, user prompts, and orchestration — no git operations, no filesystem mutation, no business rules. Rationale: keeping `cmd/` thin makes commands easy to read and lets the worktree package be tested without a cobra harness.

### VI. Interactive by Default, Scriptable on Demand
Commands that prompt the user (open menus, confirmations, name selection) SHALL accept a `--non-interactive` flag that produces deterministic, non-prompting behavior suitable for scripts. When stdout is not a TTY, commands SHOULD degrade gracefully without requiring the flag. Rationale: `wt` is used both by humans at a terminal and by automation (operators, CI); both modes must be first-class.

### VII. Shell Integration via Eval
Features that require modifying the user's shell state (e.g., `cd` into a created worktree) SHALL be implemented via the `wt shell-init` pattern: the binary prints shell code to stdout, and the user evaluates it in their shell profile. The binary itself MUST NOT attempt to modify the parent shell directly. Rationale: a child process cannot change its parent's working directory; pretending otherwise leads to silent failures.

## Additional Constraints

### Test Integrity
Tests MUST conform to the implementation spec — never the other way around. When tests fail, the fix SHALL either (a) update the tests to match the spec, or (b) update the implementation to match the spec. Modifying implementation code solely to accommodate test fixtures or test infrastructure is prohibited. Specs are the source of truth; tests verify conformance to specs.

### Module Path Stability
The Go module path is `github.com/sahil87/wt`. Renames or relocations require a MAJOR constitution bump and coordinated migration of all downstream consumers (fab-kit operators, shell aliases). Rationale: import-path changes break every dependent without warning.

### Toolkit Standards

This tool is part of the shll toolkit and MUST conform to the toolkit's published standards. The standards are enumerated by running `shll standards` — each entry names what it governs; read one with `shll standards <name>`. Before changing the CLI surface, help output, README.md, or docs/site/, the change MUST be checked against the standards governing that surface. If shll is unavailable, the canonical sources are the sahil87/shll repository's docs/site/standards/ tree (rendered on https://shll.ai). Standards added or revised there bind this repo without further amendment to this constitution.

## Governance

**Version**: 1.1.0 | **Ratified**: 2026-05-03 | **Last Amended**: 2026-07-18
