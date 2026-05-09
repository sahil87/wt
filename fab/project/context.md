# Project Context

## Overview

`worktree-cli` (binary name: `wt`) is a Git worktree management CLI written in Go. It provides ergonomic commands to create, list, open, and delete worktrees, plus a shell-init eval flow for `cd`-into-worktree behavior. The code is being migrated from `fab-kit/src/go/wt/` into this standalone repo.

## Tech Stack

- **Language**: Go 1.22
- **CLI framework**: `github.com/spf13/cobra` v1.8.x
- **Module path**: `github.com/sahil87/wt`
- **Testing**: Go stdlib `testing` package, `t.TempDir()` for filesystem isolation, real `git` invocations for integration tests
- **External tools**: `git` (required at runtime), `direnv` (for `.envrc`), `just` (for build orchestration)

## Repository Layout

```
src/
  cmd/          # cobra subcommand definitions + main.go
                #   create.go, list.go, open.go, delete.go,
                #   init.go, shell_init.go, integration_test.go
  internal/
    worktree/   # core logic — git ops, naming, menus, rollback, etc.
                #   apps.go, context.go, crud.go, errors.go,
                #   git.go, menu.go, names.go, platform.go,
                #   rollback.go, stash.go, worktree.go
go.mod
go.sum
```

The original layout under `fab-kit` was `src/go/wt/{cmd,internal/worktree}`. This repo flattens the Go-specific prefix (`src/go/wt`) into `src/{cmd,internal}` and rebases the module path accordingly.

## Conventions

- **Subcommand registration**: each command exposes a `xxxCmd() *cobra.Command` constructor; the root in `cmd/main.go` calls `root.AddCommand(...)` for each
- **Error handling**: subcommands return `error` from `RunE`; `main.go` prints to stderr and exits with `wt.ExitGeneralError` on non-nil
- **Exit codes**: defined as constants in `internal/worktree/errors.go`
- **Tests**: file-per-source — `foo.go` paired with `foo_test.go` in the same package; integration tests in `cmd/integration_test.go` build the binary and shell out to it
- **Shared test helpers**: `cmd/testutil_test.go`
- **Naming**: random adjective-noun worktree names (e.g., `swift-fox`) generated in `internal/worktree/names.go`

## Build & Release

In the source repo (`fab-kit`), wt was built via a top-level `justfile`:

```
just build-target {os} {arch}   # cross-compile single binary
just build-all                  # all 4 platforms
```

Equivalent build tooling will need to be created or adapted for this standalone repo (likely a slimmer `justfile` or `Makefile` covering `test`, `build`, `release`).

CI release flow in fab-kit: `scripts/release.sh <bump>` tags + pushes; GitHub Actions cross-compiles, packages, and publishes a release.

## Out-of-Scope (vs. fab-kit)

This repo houses **only** the wt binary. The `fab`, `fab-kit`, and `idea` binaries remain in fab-kit. Cross-repo coordination (fab-kit operators that spawn `wt`) happens via the published binary, not direct imports.
