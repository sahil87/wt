---
type: memory
description: "The `wt shell-init <shell>` strict contract — required `zsh|bash` argument, eval-safe wrapper on stdout with exit 0, and usage errors (missing/unsupported shell) that exit 2 with empty stdout and a stderr message via the direct-exit pattern; plus the eval-in-subshell test guard."
---
# wt-cli: Shell-Init Contract

**Domain**: wt-cli

## Overview

`wt shell-init <shell>` prints the shell wrapper function that Constitution Principle VII's eval-integration pattern depends on. It conforms to the shll toolkit `shell-init` standard: everything on stdout is eval-safe shell source, and every error path is a usage error with empty stdout. This file records the strict argument contract and the exit-code shape.

## Requirements

### Requirement: the shell argument is required and constrained to `zsh|bash`

`wt shell-init` SHALL require exactly one positional shell argument, and it MUST be `zsh` or `bash`. The command registers `Use: "shell-init <shell>"` with `Args: cobra.MaximumNArgs(1)` (not `ExactArgs(1)`) so the missing-argument branch is handled in `RunE`, routing through the exit-2 direct-exit path below rather than cobra's exit-1 arg validator. The supported set is a `map[string]bool{"zsh": true, "bash": true}` (`supportedShells`).

- There is NO `$SHELL` inference. The former path — `filepath.Base(os.Getenv("SHELL"))`, including its `filepath.Base("")` → `"."` special-case and the warn-and-emit-anyway branch for unknown shells — does not exist. A missing argument is a usage error, never a silent inference.

### Requirement: the explicit-shell path emits only the eval-safe wrapper

`wt shell-init zsh` and `wt shell-init bash` MUST write exactly `ShellWrapperFunc` to stdout, exit 0, with stderr empty. `ShellWrapperFunc` is a package constant (shared between the subcommand and its tests to avoid duplication) and is valid source for both supported shells — the wrapper content does not vary by shell name; the argument only gates which invocations are accepted.

#### Scenario: an explicit shell emits an eval-clean wrapper

- **GIVEN** a built `wt` binary
- **WHEN** `wt shell-init zsh` (or `bash`) runs
- **THEN** stdout equals the wrapper bytes exactly, stderr is empty, and the exit code is 0
- **AND** `eval`ing that stdout in the named shell exits cleanly

### Requirement: missing or unsupported shell is a usage error — exit 2, empty stdout, stderr message

Both a missing argument and an unsupported argument (any value not in `supportedShells`, e.g. `fish`) MUST be treated identically as a usage error: **empty stdout**, a usage message on **stderr**, and exit **`wt.ExitInvalidArgs` (2)**.

- The exit-2 path uses the **direct-exit pattern** — `wt.ExitWithError(wt.ExitInvalidArgs, what, why, fix)` inside `RunE`, the same in-repo precedent `update.go` uses for `ErrBrewNotFound`. Returning an error from `RunE` is wrong here because `main.go` maps all `RunE` errors to `ExitGeneralError` (1); the direct exit bypasses that mapper so the usage error carries the toolkit-convention exit-2 code (Constitution Principle III, typed exit codes).
- The `fix` field names the correction: `run "wt shell-init zsh" or "wt shell-init bash"`.
- **stdout emptiness on the error path is the eval-safety point**: the command's stdout is `eval`ed verbatim by shell profiles and composed by `shll shell-init`; an error must never leave junk on stdout to be evaluated.

#### Scenario: no argument is a usage error even when `$SHELL` is set

- **GIVEN** `SHELL=/bin/zsh` exported
- **WHEN** `wt shell-init` runs with no argument
- **THEN** stdout is empty, stderr carries a usage message naming `wt shell-init zsh|bash`, and the exit code is 2 — the environment variable no longer influences behavior

#### Scenario: an unsupported shell is the same usage error

- **GIVEN** a built `wt` binary
- **WHEN** `wt shell-init fish` runs
- **THEN** stdout is empty, stderr carries the usage message, and the exit code is 2 (no warn-and-emit-anyway)

### Requirement: documented invocation names the shell everywhere

Every runnable eval form across docs, help text, and Go string literals SHALL name the shell — `eval "$(wt shell-init zsh)"` (and `bash` where instructional). No runnable no-arg `eval "$(wt shell-init)"` form remains anywhere, because after this contract that form emits nothing and prints a usage line. Surfaces carrying the eval form: `README.md`, `docs/site/install.md`, `docs/site/workflows.md`, the byte-identical `docs/site/skill.md` / `src/cmd/wt/skill.md` pair, `docs/specs/cli-surface.md`, the root command `Long` in `src/cmd/wt/main.go`, `shell_init.go`'s own `Use`/`Long`, and the runtime `WT_WRAPPER`-gated stderr hints in `src/internal/worktree/apps.go` and `src/cmd/wt/go.go` (plus `go.go`'s `Long`).

## Design Decisions

### Usage errors use the direct-exit pattern, keeping `MaximumNArgs(1)`
**Decision**: missing/unsupported shell exits via `wt.ExitWithError(wt.ExitInvalidArgs, ...)` inside `RunE`, and the command keeps `cobra.MaximumNArgs(1)` so the missing-argument branch is ours to handle.
**Why**: the toolkit convention is exit 2 for usage errors, but `main.go` maps every `RunE`-returned error to exit 1, and a `cobra.ExactArgs(1)` failure would take that same exit-1 path. The direct-exit pattern (precedent: `update.go`'s `ErrBrewNotFound` handling) is the established in-repo bypass that lets a usage error carry exit 2.
**Rejected**: returning an error from `RunE` (exit 1 — violates the standard's exit-2 usage convention); a custom `Args` validator or `cobra.ExactArgs(1)` (same exit-1 mapping).
*Introduced by*: 260719-32su-conform-update-version-shell-init

### `$SHELL` inference dropped in favor of a required argument
**Decision**: the shell argument is required; there is no `$SHELL`-based inference and no default.
**Why**: the shll `shell-init` standard states verbatim that invoking `shell-init` with no shell is a usage error, and a wrapper is only guaranteed valid source for the shell it targets — inferring the shell silently is the non-conformance the standard forbids. Existing no-arg profiles degrade safely: stdout stays empty (nothing poisonous to eval) and a usage line appears on stderr until the user adds the shell name.
**Rejected**: keeping `$SHELL` inference as a fallback (silent non-conformance; can emit a wrapper for the wrong shell); emitting the wrapper anyway on an unknown shell with a warning (the warn-and-emit-anyway path the standard rejects).
*Introduced by*: 260719-32su-conform-update-version-shell-init

## Cross-references

- Source: `src/cmd/wt/shell_init.go` — `shellInitCmd`, `ShellWrapperFunc` constant, `supportedShells` map, the two `wt.ExitWithError(wt.ExitInvalidArgs, ...)` direct-exit calls.
- Tests: `src/cmd/wt/shell_init_test.go` — zsh/bash byte-exact happy paths, missing-arg and unsupported-arg exit-2/empty-stdout/stderr-usage assertions (incl. `SHELL` set/unset variants proving inference is gone), and the eval-in-subshell guard (`bash` always; `zsh` skipped when absent from `PATH`) asserting a clean exit 0; the `os.Exit`-path exit codes are asserted via the built-binary integration harness, since they cannot be asserted in-process.
- Constitution: Principle VII (Shell Integration via Eval — the binary prints shell code, never mutates the parent shell), Principle III (Typed Exit Codes — `wt.ExitInvalidArgs` (2) for usage errors via the direct-exit pattern), Principle II (Cobra Command Surface — `RunE`, `SilenceUsage`/`SilenceErrors`).
- Sibling memory: [toolkit-standards-conformance](/wt-cli/toolkit-standards-conformance.md) — the `shell-init` standard's PASS surface and the shll v0.1.7 re-audit that this contract satisfies; [go-command-contract](/wt-cli/go-command-contract.md) and the launcher hints that share the `WT_WRAPPER`-gated `eval "$(wt shell-init zsh)"` hint convention; [skill-command-contract](/wt-cli/skill-command-contract.md) — the byte-identical skill bundle that carries the shell-named eval example.
- External: `shll standards shell-init` (runtime enumeration, shll v0.1.7), https://shll.ai — the canonical standard text; `shll shell-init` composes this command's stdout into an eval'ed blob (the downstream consumer that relies on eval-safety and empty-stdout-on-error).
