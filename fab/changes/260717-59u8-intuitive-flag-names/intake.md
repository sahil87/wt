# Intake: Intuitive Flag Names

**Change**: 260717-59u8-intuitive-flag-names
**Created**: 2026-07-17

## Origin

Synthesized from a live design conversation (promptless dispatch via `/fab-proceed`). All decisions below were made and confirmed by the user in that conversation ("ok for all"). Raw prompt:

> CLI flag simplification / intuitiveness pass across wt subcommands. Make `wt` more intuitive by removing the redundant `worktree-`/`delete-` flag prefixes, converting string-typed booleans to real bools, standardizing negation as `--no-*`, adding short flags where they aid common interactive use, and adding command aliases for git/unix muscle memory. Every rename is backward compatible: the old flag name remains as a hidden deprecated alias.

## Why

1. **Pain point**: Flag names like `--worktree-name` on `wt create` and `--delete-branch` on `wt delete` repeat the command's own noun/verb — the `worktree-`/`delete-` prefixes are pure redundancy inside their command context (`wt create --worktree-name` says "worktree" twice; `wt delete --delete-branch` says "delete" twice). Two flags (`--worktree-init`, `--delete-remote`) are string-typed booleans (`true|false`) that force users to type a value for what should be a boolean switch. Negation is inconsistent (`--skip-brew-update` vs. no `--no-*` convention). Common interactive flags lack short forms, and there are no command aliases for git/unix muscle memory (`new`, `rm`, `ls`).
2. **Consequence of not fixing**: Every interactive invocation carries avoidable typing overhead and a discoverability tax; the CLI reads as machine-generated rather than human-designed. The longer the old surface persists, the more external scripts accrete against the awkward names, raising the eventual migration cost.
3. **Why this approach**: Rename-with-deprecated-alias is strictly additive — no caller breaks, ever. pflag's `MarkDeprecated` gives auto-hiding from help plus a stderr warning for free, and the codebase already has the precedent (`src/cmd/wt/delete.go:152` marks `--worktree-name` deprecated on `wt delete`). Cobra `Aliases:` gives command aliases with zero behavior risk.

## What Changes

Every rename follows the same mechanism: register the **new** flag as primary, keep the **old** flag registered (bound so both feed the same behavior), and `cmd.Flags().MarkDeprecated("<old>", "use --<new> instead")`. pflag auto-hides deprecated flags from help and prints the deprecation warning to **stderr** (consistent with the project convention: stdout = machine result, stderr = human copy). Precedent: `src/cmd/wt/delete.go:152`.

### wt create (`src/cmd/wt/create.go`, flags at lines 558–564)

1. `--worktree-name <name>` → `--name <name>` with short flag `-n`. Old name kept as hidden deprecated alias.
2. `--worktree-open <mode>` → `--open <mode>` with short flag `-o`. Values unchanged: `prompt`, `default`, `skip`, or an app name (e.g. `code`). `--open` ALWAYS requires a value — explicitly do NOT give it a `NoOptDefVal` (see Rejected Alternatives: bare `--open code` would parse `code` as the positional `[branch]` argument — a silent footgun).
3. `--worktree-init true|false` (string-typed bool) → `--no-init` (real bool). Default behavior stays "run the init script". Old string flag kept as deprecated alias.
4. Command alias: `wt new` → `wt create` (cobra `Aliases: []string{"new"}`).

After: `wt create -n swift-fox -o code` ≡ today's `wt create --worktree-name swift-fox --worktree-open code`; `wt create --no-init` ≡ today's `wt create --worktree-init false`.

### wt delete (`src/cmd/wt/delete.go`, flags at lines 138–152)

5. `--delete-all` → `--all` with short flag `-a`. Old name deprecated alias. (Note: `-s` is already taken by `--stash`; `-a` is free.)
6. `--delete-branch true|false|auto` → `--branch true|false|auto`. Stays a **STRING** — `auto` makes it a genuine tri-state (see Rejected Alternatives). Old name deprecated alias.
7. `--delete-remote true|false` (string-typed bool) → `--no-remote` (real bool). Default behavior stays "delete the remote branch when the local branch is deleted". Old flag deprecated alias.
8. Command alias: `wt rm` → `wt delete` (cobra `Aliases: []string{"rm"}`).

Untouched on delete: `--stash`/`-s`, `--non-interactive`, `--stale` (incl. its `NoOptDefVal`), and the already-deprecated `--worktree-name`.

### wt open (`src/cmd/wt/open.go`, flags at lines 153–154)

9. `--go` → `--select` (says what it does — runs the worktree selector first — instead of naming the sibling `wt go` command). Old `--go` kept as hidden deprecated alias. No short flag.
10. `--app` gains short flag `-a` (long name unchanged).

### wt list (`src/cmd/wt/list.go`)

11. Command alias: `wt ls` → `wt list` (cobra `Aliases: []string{"ls"}`). No flag changes — `--path/--json/--status/--sort/--non-interactive` are already clean.

### wt update (`src/cmd/wt/update.go`, flag at line 33)

12. `--skip-brew-update` → `--no-brew-update` (aligns the negation convention to `--no-*`). Old name deprecated alias. Semantics unchanged: skip the internal `brew update` tap-metadata refresh; version check and upgrade still run.

### wt init (`src/cmd/wt/init.go`)

13. Sharpen the `Short:` description only — from `"Run worktree init script"` to wording like `"Run the init script in the current worktree"` — to counter the "git init"-style misreading ("initialize wt here"). No behavior change, no flags. <!-- assumed: exact Short wording — user gave "e.g." phrasing, final string is implementer's choice within that intent -->

### String→bool conversion mechanics

For the two string-bool conversions (`--worktree-init`→`--no-init`, `--delete-remote`→`--no-remote`), the old flag cannot share a variable with the new one (types differ: string vs bool). The old string flag stays registered against its existing string variable and is `MarkDeprecated`; `RunE` reconciles: when the new bool flag was explicitly set (`cmd.Flags().Changed("no-init")`), it wins; otherwise the old string value (if set) is honored via the existing parsing path. Cobra bools support `--flag=false`, so no string-bool is needed except genuine tri-states (`--branch auto`).

For same-type renames (`--worktree-name`/`--name`, `--worktree-open`/`--open`, `--go`/`--select`, `--delete-all`/`--all`, `--delete-branch`/`--branch`, `--skip-brew-update`/`--no-brew-update`), both flags may bind the same variable (pflag permits two flags sharing a pointer; an explicitly-set flag wins over an unset one — reconcile with `Changed()` where precedence matters).

### Ripple surfaces (updated in this change)

- `docs/specs/cli-surface.md` — per-subcommand flag reference: new names primary, old names noted as deprecated aliases. Also fix the pre-existing staleness found during intake: the `wt update` section claims "No flags" but `--skip-brew-update` exists today (and `docs/memory/wt-cli/update-command-contract.md` documents it).
- `docs/specs/worktree-layout.md` — references the `--worktree-name` override; update to `--name`.
- `README.md` line 78 — lists `--worktree-name` among `wt create` key flags; update to `--name`.
- `docs/memory/wt-cli/*` — updated at hydrate as usual (see Affected Memory).
- `wt help-dump` output regenerates automatically from cobra metadata — no manual work.

### Tests (Constitution IV)

Unit tests alongside each command file — `create_test.go`, `delete_test.go`, `open_test.go`, `list_test.go`, `update_test.go`, plus `integration_test.go` — covering both the new names (long + short forms, aliases `new`/`rm`/`ls`) and the deprecated old names still working (same behavior + stderr deprecation warning). Existing tests invoke old flag names ~160 times across 11 test files; they keep passing untouched and double as back-compat coverage — migration of existing test invocations to new names is selective, not mandatory. Tests exercising `--open`/`--app` codepaths must respect the side-effect policy in `fab/project/code-review.md` (non-side-effecting targets or `runWt` env isolation).

## Affected Memory

- `wt-cli/update-command-contract`: (modify) `--skip-brew-update` → `--no-brew-update` rename with deprecated alias; the cross-toolkit flag reference.
- `wt-cli/go-command-contract`: (modify) the `wt open --go` composition is now `wt open --select` (with `--go` as deprecated alias).
- `wt-cli/create-branch-semantics`: (modify) examples and flag references using `--worktree-name` → `--name`.
- `wt-cli/idle-staleness-contract`: (modify) references to `--delete-all` → `--all` in the delete-selector interplay.
- `wt-cli/flag-naming-conventions`: (new) the flag-surface convention this change establishes: no command-noun prefixes, `--no-*` negation, short flags only for common interactive use, rename-via-MarkDeprecated back-compat contract, command aliases. <!-- assumed: new dedicated convention file vs. folding into per-command contract files — hydrate may fold instead; either satisfies the intent -->

(Incidental old-flag mentions also exist in `create-output-phases`, `menu-navigation-contract`, `recency-ordering-contract` — hydrate sweeps those opportunistically.)

## Impact

- **Code**: `src/cmd/wt/create.go`, `delete.go`, `open.go`, `list.go`, `update.go`, `init.go` — flag registration blocks and `cobra.Command` metadata (`Aliases:`, `Short:`) only. No `src/internal/worktree/` logic changes (Constitution V: this is pure flag parsing/orchestration, which belongs in `cmd/`). No exit-code changes.
- **Tests**: the six command test files + `integration_test.go`; other test files referencing old flags (`edge_test.go`, `testutil_test.go`, pty/sigint tests) keep working via back-compat.
- **Docs**: `docs/specs/cli-surface.md`, `docs/specs/worktree-layout.md`, `README.md` (in-change); `docs/memory/wt-cli/` (at hydrate).
- **External callers**: fab-kit operators invoke the published `wt` binary (e.g. `fab batch switch` uses `wt create --worktree-name ... --non-interactive`) and external scripts exist — all keep working because NO old flag is removed. Deprecation warnings go to stderr only, so machine-parsed stdout is unaffected.
- **Not affected**: `wt go`, `wt shell-init`, `wt help-dump` (command surface unchanged; help-dump JSON regenerates from cobra metadata), `--non-interactive` everywhere, `--base`/`--checkout`/`--reuse`/`--stale`.

## Open Questions

- None — all substantive decisions were made and confirmed by the user in the design conversation (see Assumptions).

## Rejected Alternatives (user-confirmed)

- **`-b` short flag for `--base`** — collides with `git worktree add -b` semantics (there `-b` NAMES the new branch); would actively mislead git users.
- **`NoOptDefVal` on the new `--open`** — because `wt create` takes a positional `[branch]`, bare `--open code` would parse `code` as the branch argument: a silent footgun. `--open` requires an explicit value.
- **Renaming `--non-interactive`** — named in Constitution VI, appears on four commands, and is script-facing where explicitness beats brevity. Unchanged.
- **Renaming the `wt init` / `shell-init` commands or touching `help-dump`** — out of scope; init gets only the Short-text wording fix.
- **Splitting `--delete-branch`'s tri-state into two bools** — messier than keeping the string.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Rename create flags: `--worktree-name`→`--name`/`-n`, `--worktree-open`→`--open`/`-o`, `--worktree-init true/false`→`--no-init` (real bool); alias `wt new` | Discussed — user confirmed each rename explicitly ("ok for all") | S:95 R:90 A:95 D:95 |
| 2 | Certain | Rename delete flags: `--delete-all`→`--all`/`-a`, `--delete-branch`→`--branch` (stays string tri-state), `--delete-remote true/false`→`--no-remote` (real bool); alias `wt rm` | Discussed — user confirmed; tri-state string retention explicitly chosen over two bools | S:95 R:90 A:95 D:95 |
| 3 | Certain | `wt open`: `--go`→`--select` (old `--go` hidden deprecated alias); `--app` gains `-a`; `wt list` gains alias `ls`; `wt update`: `--skip-brew-update`→`--no-brew-update` | Discussed — user confirmed each | S:95 R:90 A:95 D:95 |
| 4 | Certain | Back-compat is mandatory: NO old flag removed; mechanism is old flag registered + `MarkDeprecated` (auto-hidden, stderr warning), per precedent `src/cmd/wt/delete.go:152` | Discussed — user-stated constraint; fab-kit operators and external scripts call the published binary | S:95 R:85 A:95 D:95 |
| 5 | Certain | `--open` requires an explicit value — NO `NoOptDefVal` (positional `[branch]` would swallow bare `--open code`); no `-b` short for `--base`; `--non-interactive` unchanged everywhere | Discussed — user-confirmed rejected alternatives with stated rationale | S:95 R:85 A:95 D:95 |
| 6 | Certain | `wt init` change is Short-text wording only — no behavior, no flags, no command renames | Discussed — user scoped it explicitly | S:95 R:95 A:95 D:95 |
| 7 | Confident | String→bool reconciliation: old string flag stays registered on its own variable; `RunE` gives precedence to the new bool when `Flags().Changed()` reports it explicitly set, else honors the old string path | Types differ so one shared variable is impossible; explicit-new-wins is the conventional precedence; trivially reversible in one function | S:70 R:85 A:80 D:70 |
| 8 | Certain | Deprecation message wording: `"use --<new> instead"` style matching the existing delete.go:152 precedent | Codebase precedent gives the pattern; exact copy is low-stakes | S:65 R:95 A:85 D:80 |
| 9 | Certain | `wt init` Short text lands as "Run the init script in the current worktree" (user gave this as "e.g." — final wording implementer's choice within that intent) | User supplied example wording with delegation signal; UX copy is trivially reversible | S:70 R:95 A:80 D:75 |
| 10 | Certain | Doc ripple extends beyond the user-named `cli-surface.md`: also update `worktree-layout.md`'s `--worktree-name` mention, README line 78, and fix cli-surface's stale `wt update` "No flags" claim (pre-existing: `--skip-brew-update` exists today) | Grep-verified stale references; leaving renamed flags stale in docs contradicts the change's intent; pure docs, fully reversible | S:60 R:95 A:85 D:75 |
| 11 | Confident | Existing tests (~160 old-flag invocations across 11 files) stay on old names as living back-compat coverage; new tests added for new names, shorts, aliases, and deprecation warnings; migration of old invocations is selective | Constitution IV requires coverage of both surfaces; keeping old-name tests is the cheapest genuine back-compat proof | S:65 R:90 A:80 D:70 |
| 12 | Confident | Hydrate records the new flag-surface convention in a new `wt-cli/flag-naming-conventions` memory file (folding into per-command contracts instead is acceptable) | Domain layout choice — either location satisfies the intent; hydrate-time decision, fully reversible | S:55 R:90 A:70 D:55 |

12 assumptions (9 certain, 3 confident, 0 tentative, 0 unresolved).
