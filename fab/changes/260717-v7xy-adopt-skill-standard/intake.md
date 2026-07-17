# Intake: Adopt the Toolkit Skill Standard (`wt skill`)

**Change**: 260717-v7xy-adopt-skill-standard
**Created**: 2026-07-18

## Origin

One-shot invocation of `/fab-new v7xy` (backlog ID). Raw backlog entry:

> [v7xy] 2026-07-18: Adopt the toolkit `skill` standard -- add a hidden-free `wt skill` subcommand that prints a static, <=150-line agent usage bundle (byte-identical to a new canonical docs/site/skill.md) to stdout, embedded at build time via a sync + drift-guard test (reuse the mechanism `shll standards` established). Deferred from change 6end (toolkit-standards-conformance) as "not yet adopted" per the standard's phased per-repo rollout (no seven-repo flag-day; no tool ships `skill` today). This is a new subcommand + repo file + build wiring, out of scope for a conformance audit's small-additive boundary.

No prior conversation preceded this invocation. Intake-time verification: `shll standards skill` runs and returns the full standard text; the shll reference implementation was inspected at `~/code/sahil87/shll` (`src/cmd/shll/standards.go`, `standards_test.go`, `scripts/sync-standards.sh`); `wt` currently has no `skill` subcommand and no `go:embed` usage anywhere under `src/`.

## Why

1. **Pain point**: An agent operating an installed `wt` binary has no offline, version-locked usage briefing. `-h`/`help-dump` is flag reference (structure, not judgment); README/docs-site needs the repo checked out or a network trip; `fab/project` context is contributor-scoped, not caller-scoped. The toolkit `skill` standard exists precisely to close this gap.
2. **Consequence of not fixing**: `wt` stays out of conformance with toolkit principle №10 (agent-discoverable documentation) as sibling tools adopt, and the planned `shll agent-setup` aggregator (which concatenates every installed tool's `<tool> skill` output) will silently skip `wt`. The deferral recorded in change 6end (`toolkit-standards-conformance`) remains open.
3. **Why this approach**: The standard mandates the shape — a `skill` subcommand printing a static bundle byte-identical to `docs/site/skill.md`, embedded via the committed-copy + sync-script + drift-guard mechanism `shll standards` established. Reuse over invention: the mechanism is proven, and `wt` shares the exact constraint that motivated it (Go module root is `src/`, so `//go:embed` cannot reach `docs/site/` above it — a committed copy inside the package bridges the gap).

## What Changes

### 1. New canonical bundle: `docs/site/skill.md`

A new agent-first usage briefing, **≤150 lines** (hard budget — a drift-guard test pins it), **static-only** (no timestamps, no environment lookups — contrast `run-kit context`'s dynamic header). Genre per the standard: usage briefing, NOT a README clone and NOT a flag table. Content areas (per `shll standards skill` § Content):

- **When to use** — parallel work on multiple branches without stash/switch churn; when NOT (plain branch switching, no worktree needed).
- **Capabilities map** — one line per subcommand, keyed to it: `create` (new-branch worktree, `--checkout` for existing, `--base` for start-point), `list` (`--status`, `--json`, `--path`), `open` (app launcher menu, `--app`), `go` (selection-only navigation), `delete` (`--stale`), `init` (re-run init script), `shell-init` (eval wrapper), `update` (self-upgrade).
- **Composition patterns** — the `wt shell-init` eval flow (binary prints shell code; child processes can't `cd` the parent); the `WT_CD_FILE`/`WT_WRAPPER` launcher contract external callers like `hop` use (see `docs/specs/launcher-contract.md`); `WORKTREE_INIT_SCRIPT` init protocol (default `fab sync`, see `docs/specs/init-protocol.md`).
- **Output & exit-code contracts** — stdout = machine result, stderr = all human copy; errors as `what/why/fix` via `ExitWithError`; typed exit codes from `internal/worktree/errors.go` (e.g. `ExitGeneralError`, `ExitUserAbort`, `ExitInitFailed`); `--json` on `list`; `--non-interactive` for scripts, graceful non-TTY degradation.
- **Gotchas** — worktrees live at `<repo>.worktrees/` beside the repo, not inside it; random adjective-noun names unless `--name`; init failure keeps the worktree (`ExitInitFailed`, not rollback); positional arg to `create` is new-branch-only (`--checkout` is the existing-branch opt-in).

Because `docs/site/skill.md` is part of the pulled `docs/site/**` tree, the bundle renders at `/wt/skill` on shll.ai automatically — no extra wiring.

Source material for authoring: README.md, `docs/specs/cli-surface.md`, `docs/specs/launcher-contract.md`, `docs/specs/init-protocol.md`, `docs/specs/worktree-layout.md`, and the behavior contracts under `docs/memory/wt-cli/`.

### 2. New subcommand: `wt skill` (`src/cmd/wt/skill.go`)

Visible ("hidden-free" per the backlog — contrast `help-dump`, which sets `Hidden: true`). Follows the repo's constructor pattern:

```go
//go:generate ../../../scripts/sync-skill.sh

// skillFS / embedded copy: src/cmd/wt/skill.md, synced from docs/site/skill.md
//
//go:embed skill.md
var skillBundle []byte  // or string; single file — embed.FS not needed for one file

func skillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent usage bundle (static markdown)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write(skillBundle)
			return err
		},
	}
}
```

Invocation contract (from the standard, non-negotiable): command name exactly `skill`; raw markdown to **stdout**, byte-identical to `docs/site/skill.md`; **stderr empty on success**; exit code **0**; no rendering, pager, or added framing. Registered in `main.go`'s `root.AddCommand(...)` list. `SilenceUsage`/`SilenceErrors` are set on the root command (existing pattern) — no per-command override needed. Printing embedded bytes is trivial orchestration, so no `internal/worktree` logic is required (Constitution V keeps `cmd/` thin; there is nothing non-trivial to push down).

### 3. Committed embedded copy: `src/cmd/wt/skill.md`

The `//go:embed` source. Committed so a clean `go build ./...` (which never runs sync scripts) compiles. Byte-honesty is enforced by the drift-guard test (below), exactly as `shll`'s `TestStandardsEmbedMatchesCanonical` does for its `standards/` copies. Single file embedded directly — no subdirectory needed (shll uses a `standards/` dir only because it embeds four documents).

### 4. Sync script + build wiring: `scripts/sync-skill.sh`, justfile recipe

`scripts/sync-skill.sh` (mirrors `shll`'s `scripts/sync-standards.sh`): `set -euo pipefail`, `cd` to repo root via `$(dirname "$0")/..`, `cp -f docs/site/skill.md src/cmd/wt/skill.md`, echo a one-line confirmation. A `//go:generate` directive in `skill.go` points at it, and the justfile gains a thin recipe:

```
# Refresh the embedded skill bundle from the canonical docs/site/skill.md.
sync-skill:
    ./scripts/sync-skill.sh
```

No CI changes: the drift-guard test rides the existing `go test ./...` job (and the existing `ci-gate` required check), so divergence fails CI with no new required jobs.

### 5. Tests: `src/cmd/wt/skill_test.go` + integration case

Unit tests (Constitution IV — happy path + failure path; file alongside source):

- **Drift guard**: embedded bytes == `os.ReadFile("../../../docs/site/skill.md")`, failure message naming the fix (`just sync-skill`). This is the load-bearing test of the whole mechanism.
- **Line budget**: the embedded bundle is ≤150 lines (pins the standard's hard budget at build time).
- **Command contract**: `skillCmd()` writes exactly the embedded bytes to stdout, nothing to stderr; `Hidden` is false; extra args rejected (`cobra.NoArgs` — the failure path).
- **Static-only**: byte-identical across two invocations (cheap invariance check).

Integration test (in `src/cmd/wt/integration_test.go`, existing end-to-end harness): run the built binary — `wt skill` exits 0, stdout equals the canonical file's bytes, stderr empty. No git repo state is needed for this case, but it follows the existing harness conventions (`runWt` env isolation).

### 6. README + conformance record

- README `### Command reference` table gains one row for `wt skill` (agent-facing discoverability, principle №10 — a Confident judgment call; see Assumptions #8).
- `docs/memory/wt-cli/toolkit-standards-conformance.md` (hydrate-time): the `skill` standard's verdict flips from "deferred, not yet adopted → [v7xy]" to adopted.
- `fab/backlog.md`: the `[v7xy]` entry is checked off at archive time (existing convention — not part of apply).

## Affected Memory

- `wt-cli/skill-command-contract`: (new) The `wt skill` behavior contract — visible subcommand, raw-markdown stdout byte-identical to `docs/site/skill.md`, empty stderr, exit 0, ≤150-line static bundle, committed embedded copy + `scripts/sync-skill.sh` + drift-guard test mechanism.
- `wt-cli/toolkit-standards-conformance`: (modify) Flip the `skill` standard's per-standard verdict from "deferred, not yet adopted" (deferral ref `[v7xy]`) to adopted, with the conformance checklist from the standard's "Verifying conformance" section.

## Impact

- **Code**: `src/cmd/wt/skill.go` (new), `src/cmd/wt/skill.md` (new, committed embed source), `src/cmd/wt/skill_test.go` (new), `src/cmd/wt/main.go` (one `AddCommand` entry), `src/cmd/wt/integration_test.go` (one case).
- **Repo files**: `docs/site/skill.md` (new canonical), `scripts/sync-skill.sh` (new), `justfile` (one recipe), `README.md` (one table row).
- **Runtime surface**: one new visible subcommand; `help-dump` output gains it automatically (it walks the live command tree). No behavior change to any existing command; no new dependencies; no CI workflow changes (drift guard rides `go test`).
- **External**: shll.ai renders `/wt/skill` from the pulled docs/site tree automatically; the future `shll agent-setup` aggregator picks `wt` up once released.

## Open Questions

*(none — the standard, the backlog entry, and the shll reference implementation resolve all mechanism questions; remaining judgment calls are graded below)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse shll's mechanism verbatim: committed embedded copy inside the cmd package + sync script + drift-guard test comparing embedded bytes to the canonical `docs/site/` file | Backlog mandates "reuse the mechanism `shll standards` established"; mechanism inspected in the shll repo and `wt` shares the exact `src/`-module-root constraint that motivated it | S:95 R:85 A:95 D:95 |
| 2 | Certain | Invocation contract: command named exactly `skill`, visible (not Hidden), raw markdown to stdout byte-identical to `docs/site/skill.md`, stderr empty, exit 0, `cobra.NoArgs` | The standard specifies each of these as "Rules with teeth"; the backlog's "hidden-free" confirms visibility | S:95 R:90 A:95 D:95 |
| 3 | Certain | Test surface: drift-guard + line-budget + command-contract unit tests in `skill_test.go`, plus one end-to-end case in `integration_test.go` | Constitution IV mandates unit (happy + failure path) and integration tests per subcommand; the standard's "Verifying conformance" section enumerates exactly what to pin | S:75 R:90 A:95 D:90 |
| 4 | Confident | Embed as a single file (`//go:embed skill.md` → `src/cmd/wt/skill.md`), no subdirectory | shll's `standards/` dir exists only because it embeds four documents; one file needs no dir. Trivially restructured later if more bundles appear | S:70 R:85 A:80 D:75 |
| 5 | Confident | Sync wiring: new `scripts/sync-skill.sh` + justfile `sync-skill` recipe + `//go:generate` directive in `skill.go` | Mirrors shll's layout and wt's own thin-justfile-over-scripts/ convention; naming follows `sync-standards` precedent | S:60 R:85 A:80 D:70 |
| 6 | Confident | Bundle logic stays in `cmd/` (no `internal/worktree` involvement) | Constitution V pushes *non-trivial* logic down; writing embedded bytes to stdout is pure orchestration — shll makes the same call for `standards` | S:65 R:80 A:85 D:80 |
| 7 | Confident | Bundle content: five content areas from the standard (when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas), authored from README + docs/specs/* + docs/memory/wt-cli/* | The standard prescribes the genre and the repo's specs/memory contain all the material; exact prose is apply-time authoring within a prescribed frame | S:75 R:80 A:75 D:70 |
| 8 | Confident | Add one `wt skill` row to README's Command reference table | Precedent mixed (`wt update`/`wt go` visible but unlisted) yet principle №10 favors discoverability; one row, trivially reversible either way | S:40 R:90 A:55 D:50 |

8 assumptions (3 certain, 5 confident, 0 tentative, 0 unresolved).
