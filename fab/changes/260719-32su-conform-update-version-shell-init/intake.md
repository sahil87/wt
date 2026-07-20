# Intake: Conform update, version, and shell-init to the shll toolkit standards

**Change**: 260719-32su-conform-update-version-shell-init
**Created**: 2026-07-20

## Origin

One-shot `/fab-new` invocation:

> Bring this repo into conformance with the shll toolkit 'update', 'version', and 'shell-init' standards (docs/site/standards/update.md, version.md, and shell-init.md in the shll repo, or https://shll.ai/standards). Audit the update, --version, and shell-init subcommands against every MUST/SHOULD in all three standards, fix any gaps found, and add/update tests pinning the fixed behavior. If the audit finds the repo is already fully conformant with no code changes needed, skip /git-pr entirely — do not open an empty PR.

An intake-time preliminary audit was performed (standards read via `shll standards update|version|shell-init` at **shll v0.1.7**; implementations read; a built binary probed). It found **confirmed MUST violations** in `update` and `shell-init`, and one SHOULD gap in `version` — so the "already fully conformant, skip /git-pr" branch will NOT trigger; the conditional is recorded anyway as a pipeline instruction in case apply-time re-audit overturns a finding.

Context: the prior toolkit-standards audit (`260717-6end`, memory `wt-cli/toolkit-standards-conformance`) ran against **shll v0.0.23**, which published only 4 standards (`principles`, `help-dump`, `readme-extraction`, `skill`). The `update`, `version`, and `shell-init` standards are **new since that baseline** — this change is exactly the re-audit that memory's "re-audit trigger" requirement anticipates. Standards bind without constitution amendment (constitution § Toolkit Standards).

## Why

1. **Pain point**: `shll` composes the toolkit — `shll update` delegates to `wt update`, `shll version`/`shll doctor` probe `wt --version`, and `shll shell-init` composes `wt shell-init`'s stdout into an `eval`ed blob. Each standard freezes a textual/behavioral contract that `wt` currently breaks in specific, verified ways (enumerated in What Changes).
2. **Consequence if unfixed**:
   - `wt update --help` no longer contains the literal substring `--skip-brew-update` (the flag was deprecated+hidden by `260717-59u8`), so `shll update`'s substring probe silently degrades every toolkit-wide run to N redundant `brew update`s.
   - `wt update` runs `brew upgrade` under a **120-second hard timeout via `exec.CommandContext`, which sends SIGKILL on expiry** — the exact keg-corrupting incident (SIGKILL landing between `brew unlink` and `brew link`, observed 2026-07-19) that motivated the standard's brew-handling MUSTs.
   - `wt shell-init` with an unsupported shell prints a warning to stderr but **still emits the wrapper on stdout and exits 0**; the standard requires exit 2, usage on stderr, empty stdout.
3. **Approach**: fix in place per standard, smallest conformant change, preserving this repo's constitution (typed exit codes, cobra surface, internal-package boundary) and pinning every fixed behavior with tests — the standards' own "Verifying conformance" checklists are the test specs.

## What Changes

### Audit verdicts (intake-time, to be re-verified at apply)

| Standard | Clause | Current state | Verdict |
|----------|--------|---------------|---------|
| update | `update` subcommand exists, works standalone, in-place upgrade | `src/cmd/wt/update.go` + `src/internal/update/update.go` | PASS |
| update | help MUST contain literal `--skip-brew-update` | Flag exists but `MarkDeprecated` hides it; built-binary probe confirms help lacks the substring | **FAIL (MUST)** |
| update | flag MUST be honored (skip internal `brew update`) | Honored (shared bool with `--no-brew-update`) | PASS |
| update | exit 0 on success incl. already-up-to-date; non-zero only on genuine failure | `Run` returns nil on up-to-date and non-brew install | PASS |
| update | MUST NOT SIGKILL brew mid-transaction; MUST NOT short hard timeout on `brew upgrade`; any bound generous + SIGTERM+grace | `brewUpgradeTimeout = 120s` via `exec.CommandContext` (SIGKILL on expiry); `brew update` 30s, `brew info` 30s, same kill mode | **FAIL (MUST)** |
| update | self-update only when brew-installed, `/Cellar/` detection, clear degrade message | `isBrewInstalled()` does exactly this | PASS |
| update | one name across repo/roster/formula/binary; `v{semver}` tags | repo `sahil87/wt`, formula `sahil87/tap/wt`, binary `wt`; tags per `docs/specs/build-and-release.md` | PASS |
| version | `--version` exits 0, version on stdout | cobra `Version:` field → `wt version dev` / `wt version vX.Y.Z`, exit 0 | PASS |
| version | ≤ 2s, no network I/O | purely local | PASS |
| version | token on first non-empty line; canonical `<tool> version vX.Y.Z` shape | cobra default template emits exactly the canonical shape | PASS |
| version | binary name on PATH == tool name | `wt` == `wt` | PASS |
| version | SHOULD: minimal test pinning exit 0 + first-line shape | **No test covers `--version` anywhere** (grep of `src/cmd/wt/*_test.go` confirms) | **GAP (SHOULD)** |
| shell-init | `shell-init zsh`/`bash` emit ONLY eval-safe source on stdout, exit 0 | Wrapper is valid bash and zsh; nothing else on stdout | PASS |
| shell-init | diagnostics to stderr only | unsupported-shell warning already on stderr | PASS |
| shell-init | any failure exits non-zero; no stdout-junk-with-exit-0 path | Unsupported shell → warns but emits wrapper + exit 0 | **FAIL (MUST)** |
| shell-init | missing/unsupported shell arg → usage error: exit non-zero (convention 2), usage on stderr, **empty stdout** | Missing arg infers from `$SHELL` and emits wrapper; unsupported arg emits wrapper | **FAIL (MUST)** |
| shell-init | SHOULD: test that `eval`s output in a subshell and asserts clean exit | Tests assert output content only, never eval it | **GAP (SHOULD)** |

### Change area 1: `wt update` flag surface (`src/cmd/wt/update.go`)

Restore `--skip-brew-update` as a **visible, non-deprecated** flag so `wt update --help` contains the literal substring (the standard calls it a frozen textual contract, checked by `strings.Contains`). Keep `--no-brew-update` registered and visible too — both bind the same `skipBrewUpdate` bool, so behavior is identical whichever is passed. Remove the `MarkDeprecated` call: shll passes `--skip-brew-update` on every toolkit-wide run, and a deprecation warning on the contract flag on every run is wrong. Suggested help strings:

```go
cmd.Flags().BoolVar(&skipBrewUpdate, "skip-brew-update", false,
    "skip the internal `brew update` tap-metadata refresh (toolkit contract flag; version check and upgrade still run)")
cmd.Flags().BoolVar(&skipBrewUpdate, "no-brew-update", false,
    "alias for --skip-brew-update")
```

This partially reverses design decision "`--no-brew-update` primary over the cross-tool `--skip-brew-update`" (`260717-59u8`, memory `wt-cli/update-command-contract`): that decision predates the `update` standard, which now freezes the cross-tool name as a help-text substring contract. Nothing is removed — `--no-brew-update` keeps working and stays visible — only the deprecation/hiding of the contract flag is undone.

Ripple: `wt help-dump` emits flags (and aliases, per #44); the update command's flag set changes shape (`skip-brew-update` no longer hidden/deprecated). Help-dump tests that pin the update node must be updated. Existing tests `TestUpdate_SkipBrewUpdateDeprecated` and `TestUpdate_NoBrewUpdateFlagInHelp` change accordingly.

### Change area 2: brew-handling safety (`src/internal/update/update.go`)

- **`brew upgrade`**: drop `brewUpgradeTimeout`/`exec.CommandContext` entirely — plain `exec.Command`, no bound. It's an interactive, stream-inheriting subprocess; the user can Ctrl-C (SIGINT, which brew traps and unwinds). The standard: "MUST NOT impose a short hard timeout"; no bound is the simplest full conformance.
- **`brew update`**: keep a bound but make it generous and graceful — raise 30s to **5 minutes**, and replace the default SIGKILL cancel with `cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }` plus `cmd.WaitDelay` grace (e.g. 10s) so brew can unwind. It's a package-manager mutation (tap git refresh) covered by the no-SIGKILL MUST.
- **`brew info`**: read-only, but apply the same SIGTERM+grace cancel shape for uniformity; raise 30s to **60s** (network-tolerant).
- Result: **no code path can SIGKILL brew**, satisfying the MUST.

Testing note: the SIGTERM/WaitDelay wiring is hard to end-to-end test; pin what's cheap (e.g. a fake slow `brew` on PATH receiving SIGTERM, or at minimum unit assertions on the constants and an upgrade path built without a context). The exact test seam is an apply-time decision — the existing fake-`brew`-on-PATH + `WT_TEST_FORCE_BREW=1` seam is the established convention.

### Change area 3: `wt shell-init` strict argument contract (`src/cmd/wt/shell_init.go`)

New contract per the standard:

- `wt shell-init zsh` and `wt shell-init bash` → wrapper on stdout (unchanged content — it is valid source for both shells), exit 0.
- **Missing argument** (`wt shell-init`) → usage error: message on **stderr**, **empty stdout**, exit **2** (`wt.ExitInvalidArgs` — the constitution's typed-exit principle; toolkit convention is exit 2 for usage errors). The current `$SHELL`-inference path (including the `filepath.Base("")` → `"."` case) is removed.
- **Unsupported argument** (`wt shell-init fish`) → same usage error: stderr message, empty stdout, exit 2. No more warn-and-emit-anyway.
- Exit-code mechanics: `main.go` maps all `RunE` errors to `ExitGeneralError` (1), so shell-init must exit 2 via the established direct-exit pattern (as `update.go` does for `ErrBrewNotFound`) — e.g. `wt.ExitWithError(what, why, fix)` with `ExitInvalidArgs`, or an equivalent stderr+`os.Exit(2)` path. Usage message should name the fix (e.g. `wt shell-init zsh|bash`).

**This is a breaking change for existing profiles**: `eval "$(wt shell-init)"` (no arg) is the currently documented form. After this change it evals nothing (stdout empty — safe) and prints a usage line to stderr on every new shell until the user adds the shell name. The standard is unambiguous ("`<tool> shell-init` with no shell … is a usage error") and stdout-emptiness on the error path is exactly its eval-safety point.

Doc updates required (every occurrence of the no-arg form becomes `eval "$(wt shell-init zsh)"` — with bash mentioned where instructional):

- `README.md` (~3 occurrences: quick-start eval line, command table, troubleshooting)
- `docs/site/install.md`, `docs/site/workflows.md`
- `docs/site/skill.md` **and** `src/cmd/wt/skill.md` (byte-identical pair; keep the drift-guard test and ≤150-line budget green — see memory `wt-cli/skill-command-contract`)
- `docs/specs/cli-surface.md` § `wt shell-init` (spec is source of truth for tests per constitution § Test Integrity — amend the spec in the same change)
- Root command `Long` help in `src/cmd/wt/main.go` (the `eval "$(wt shell-init)"` line) and `shell_init.go`'s own `Long`

### Change area 4: tests pinning the contracts

- **update**: help output contains literal `--skip-brew-update` (substring check, mirroring shll's probe); passing `--skip-brew-update` produces **no deprecation warning** and skips `brew update` (existing `TestRunSkipBrewUpdate` seam); help-dump update node reflects the new flag shape.
- **version**: new test — `--version` exits 0, output goes to stdout, first non-empty line matches the standard's token/prefix contract (pin the canonical `wt version <v>` shape; a regex mirroring shll's `versionTokenRE`/`versionPrefixRE` documents the contract).
- **shell-init**: rewrite `TestShellInit_EmptyShell` / `TestShellInit_UnsupportedShell` / `TestShellInit_ShellArg_Unsupported` to the new exit-2/empty-stdout contract; keep zsh/bash happy-path tests; **add an eval-in-subshell test** — pipe stdout to `bash -c 'eval "$(...)"'` (and `zsh` when present on PATH, skipping otherwise) asserting exit 0 — the standard's recommended cheapest guard. Exit-code assertions for the usage paths need the integration harness (`cmd/integration_test.go` builds the real binary), since `os.Exit` paths can't be asserted in-process.
- CI reminder: run `gofmt -l` locally (CI fails fast on it; module root is `src/`).

### Pipeline conditional (from Origin)

If apply-time re-audit finds all three standards already fully conformant with no code change needed: stop after the audit, do **not** run /git-pr (no empty PR). Given four verified FAILs above, this branch is expected to be dead — the audit table must simply be re-verified at apply before implementation starts.

## Affected Memory

- `wt-cli/update-command-contract`: (modify) flag surface — `--skip-brew-update` un-deprecated/visible per the update standard (supersedes part of 260717-59u8's decision); brew-handling safety contract (no SIGKILL, no upgrade timeout, SIGTERM+grace bounds)
- `wt-cli/toolkit-standards-conformance`: (modify) extend the baseline to shll v0.1.7's three new standards (`update`, `version`, `shell-init`) with per-standard verdicts and fixes
- `wt-cli/flag-naming-conventions`: (modify) carve-out — a toolkit-standard frozen contract flag may not be hidden via MarkDeprecated; the rename-via-MarkDeprecated mechanism yields to published-standard substring contracts
- `wt-cli/shell-init-contract`: (new) the strict shell-init contract — eval-safe stdout, exit 2 usage errors, empty stdout on error paths, supported-shell set

## Impact

- **Code**: `src/cmd/wt/update.go`, `src/internal/update/update.go`, `src/cmd/wt/shell_init.go`, `src/cmd/wt/main.go` (root Long help only)
- **Tests**: `src/cmd/wt/update_test.go`, `src/internal/update/update_test.go`, `src/cmd/wt/shell_init_test.go`, `src/cmd/wt/integration_test.go` (exit-code + eval-subshell + version tests), `src/cmd/wt/help_dump_test.go` and/or `src/internal/worktree/helpdump_test.go` (flag-shape ripple)
- **Docs**: `README.md`, `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md` + `src/cmd/wt/skill.md` (byte-identical pair), `docs/specs/cli-surface.md`
- **Consumers**: `shll update` (regains `--skip-brew-update` discovery), `shll shell-init` (must pass the shell name — it already invokes per-tool with a shell per the standard's consumer contract), existing user profiles carrying `eval "$(wt shell-init)"` (stderr usage line until migrated; stdout stays empty so no shell poisoning)
- **No changes** to: worktree CRUD, menus, go/open/list/delete, release tooling

## Open Questions

- None blocking. (Whether `brew update`'s bound should be 5 minutes vs. unbounded is the softest call recorded below — the standard only requires generous + graceful, so either is conformant.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Un-deprecate and un-hide `--skip-brew-update`; keep `--no-brew-update` as a visible alias (nothing removed) | The update standard freezes the literal substring in help as a MUST and shll passes the flag every run (deprecation warnings would be noise); reverses part of user-confirmed 260717-59u8, but the standard postdates it and the constitution says standards bind without amendment | S:60 R:85 A:70 D:55 |
| 2 | Confident | `wt shell-init` with no argument becomes a usage error (exit 2, empty stdout) — dropping `$SHELL` inference despite it being the documented form | The standard states it verbatim as a usage error; user instruction says fix every MUST; stdout stays empty so existing profiles degrade to a stderr hint, never a poisoned eval; all docs updated in the same change | S:80 R:55 A:70 D:70 |
| 3 | Confident | `brew upgrade` runs with **no timeout at all** (plain `exec.Command`) rather than a generous bound | Simplest full conformance with "MUST NOT impose a short hard timeout"; interactive stream-inheriting call where Ctrl-C (SIGINT) remains available; hop/fab-kit convention per the standard | S:65 R:90 A:80 D:65 |
| 4 | Confident | `brew update` keeps a bound — raised to 5 minutes with SIGTERM + `WaitDelay` grace; `brew info` 60s same shape | Standard requires generous + graceful for any bound; 5 min covers slow-network metadata refresh; alternative (unbounded) also conformant — chosen bound keeps `wt update` from hanging forever on a wedged network | S:50 R:85 A:60 D:50 |
| 5 | Confident | Usage-error exit code is `wt.ExitInvalidArgs` (2) via the direct-exit pattern (as `update.go` does), since `main.go` maps `RunE` errors to exit 1 | Toolkit convention exit 2 for usage errors; constitution typed-exit principle; existing in-repo precedent for bypassing the generic mapper | S:70 R:80 A:85 D:75 |
| 6 | Certain | Version pinning test asserts the cobra default output shape (`wt version <version>`, first line, stdout, exit 0) rather than changing any version behavior | Probe of built binary shows full conformance; the standard's only gap is the SHOULD-level missing test | S:75 R:90 A:90 D:85 |
| 7 | Certain | Audit is re-verified at apply from `shll standards` runtime enumeration (v0.1.7 now), never from this intake snapshot or the website | Memory `wt-cli/toolkit-standards-conformance` Design Decision "Runtime enumeration over a memorized standard list" prescribes exactly this | S:85 R:90 A:95 D:90 |
| 8 | Confident | The other four standards (`principles`, `help-dump`, `readme-extraction`, `skill`) are out of scope except where this change's edits ripple into them (help-dump flag shape, skill.md byte-identity, README edits re-checked against readme-extraction) | User named exactly three standards; the re-audit trigger requires checking standards *governing the touched surface*, which the ripple handling covers | S:80 R:75 A:80 D:75 |

8 assumptions (2 certain, 6 confident, 0 tentative, 0 unresolved).
