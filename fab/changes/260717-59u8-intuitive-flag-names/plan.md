# Plan: Intuitive Flag Names

**Change**: 260717-59u8-intuitive-flag-names
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. Every rename is strictly additive: new flag primary,
     old flag kept + MarkDeprecated (auto-hidden from help, stderr warning on use).
     Mechanism precedent: src/cmd/wt/delete.go:152. pflag v1.0.5 / cobra v1.8.1:
     the deprecation warning fires inside Set() (only when the deprecated flag is
     passed) and prints to f.out() which defaults to os.Stderr — matching the
     stdout=machine / stderr=human convention. -->

### Back-Compat: The Shared Rename Mechanism

#### R1: Every renamed flag stays backward compatible via a hidden deprecated alias
Each rename SHALL register the **new** flag as primary and keep the **old** flag registered so both feed the same behavior, then call `cmd.Flags().MarkDeprecated("<old>", "use --<new> instead")`. NO old flag is ever removed. The deprecation warning MUST land on stderr (pflag default), never stdout.

- **GIVEN** any renamed flag in this change
- **WHEN** the user passes the **old** flag name
- **THEN** the command behaves exactly as it did before (same effect), and a `Flag --<old> has been deprecated, use --<new> instead` warning is printed to stderr.
- **AND** the old flag does NOT appear in `--help` output (pflag auto-hides deprecated flags).

- **GIVEN** any renamed flag
- **WHEN** the user passes the **new** flag name (no old flag)
- **THEN** the command behaves correctly and NO deprecation warning is printed.

### wt create (`src/cmd/wt/create.go`)

#### R2: `--worktree-name` → `--name` / `-n`
`wt create` SHALL accept `--name <name>` with short `-n` as the primary worktree-name flag; `--worktree-name` remains a hidden deprecated alias bound to the same value. `--reuse`'s requirement message and validation continue to work with either flag.

- **GIVEN** a repo
- **WHEN** the user runs `wt create --non-interactive -n swift-fox --no-init`
- **THEN** a worktree named `swift-fox` is created (equivalent to the old `--worktree-name swift-fox --worktree-init false`).

- **GIVEN** a repo
- **WHEN** the user runs `wt create --non-interactive --worktree-name legacy-fox`
- **THEN** the worktree `legacy-fox` is created and a deprecation warning naming `--name` is printed to stderr.

#### R3: `--worktree-open` → `--open` / `-o`
`wt create` SHALL accept `--open <mode>` with short `-o` as the primary open-mode flag; `--worktree-open` remains a hidden deprecated alias. Values are unchanged (`prompt`, `default`, `skip`, or an app name). `--open` MUST require an explicit value — it SHALL NOT be given a `NoOptDefVal` (bare `--open code` would parse `code` as the positional `[branch]`, a silent footgun).

- **GIVEN** a repo
- **WHEN** the user runs `wt create --non-interactive -o skip <newbranch>`
- **THEN** the open phase is skipped, exactly as `--worktree-open skip` did.

#### R4: `--worktree-init true|false` (string) → `--no-init` (real bool)
`wt create` SHALL accept a real boolean `--no-init` that, when set, disables the init script. Default behavior (init runs) is unchanged. The old string flag `--worktree-init` remains registered on its own string variable and is deprecated. When `--no-init` is explicitly set (`cmd.Flags().Changed("no-init")`), it wins; otherwise the old string value (`true`/`false`) is honored via the existing parsing path.

- **GIVEN** a repo
- **WHEN** the user runs `wt create --non-interactive --no-init <newbranch>`
- **THEN** no init script runs (equivalent to the old `--worktree-init false`).

- **GIVEN** a repo
- **WHEN** the user runs `wt create --non-interactive --worktree-init false <newbranch>`
- **THEN** no init script runs and a deprecation warning naming `--no-init` is printed to stderr.

#### R5: `wt new` command alias
`wt create` SHALL carry cobra `Aliases: []string{"new"}` so `wt new` invokes it identically.

- **GIVEN** a repo
- **WHEN** the user runs `wt new --non-interactive --no-init`
- **THEN** a worktree is created exactly as `wt create` would.

### wt delete (`src/cmd/wt/delete.go`)

#### R6: `--delete-all` → `--all` / `-a`
`wt delete` SHALL accept `--all` with short `-a` as the primary delete-all flag; `--delete-all` remains a hidden deprecated alias bound to the same bool variable. (`-s` is taken by `--stash`; `-a` is free.) All `--stale`/`--delete-all` and menu interplay is preserved.

- **GIVEN** a repo with worktrees
- **WHEN** the user runs `wt delete --non-interactive -a`
- **THEN** all worktrees are deleted (equivalent to the old `--delete-all`).

#### R7: `--delete-branch` → `--branch` (stays a STRING tri-state)
`wt delete` SHALL accept `--branch <true|false|auto>` as the primary branch-cleanup flag, keeping the string tri-state (`auto` is a genuine third state). `--delete-branch` remains a hidden deprecated alias bound to the same string variable.

- **GIVEN** a worktree whose branch name differs from the worktree name
- **WHEN** the user runs `wt delete <wt> --non-interactive --branch true`
- **THEN** the branch is force-deleted (equivalent to the old `--delete-branch true`).

#### R8: `--delete-remote true|false` (string) → `--no-remote` (real bool)
`wt delete` SHALL accept a real boolean `--no-remote` that, when set, disables remote-branch deletion. Default behavior (remote branch deleted with the local one) is unchanged. The old string flag `--delete-remote` remains registered on its own string variable and is deprecated. When `--no-remote` is explicitly set (`Changed("no-remote")`), it wins; otherwise the old string value is honored via the existing `deleteRemote == "true"` path.

- **GIVEN** a worktree with a remote branch that will be deleted
- **WHEN** the user runs `wt delete <wt> --non-interactive --branch true --no-remote`
- **THEN** the local branch is deleted but the remote branch is NOT (equivalent to the old `--delete-remote false`).

#### R9: `wt rm` command alias
`wt delete` SHALL carry cobra `Aliases: []string{"rm"}` so `wt rm` invokes it identically. Untouched on delete: `--stash`/`-s`, `--non-interactive`, `--stale` (incl. its `NoOptDefVal`), and the already-deprecated `--worktree-name`.

- **GIVEN** a repo with a worktree
- **WHEN** the user runs `wt rm <wt> --non-interactive`
- **THEN** the worktree is deleted exactly as `wt delete` would.

### wt open (`src/cmd/wt/open.go`)

#### R10: `--go` → `--select`
`wt open` SHALL accept `--select` (bool) as the primary "select a worktree first, then launch" flag; `--go` remains a hidden deprecated alias bound to the same bool variable. No short flag. Composition with `--app` and the git-repo precondition are unchanged.

- **GIVEN** a repo with a worktree `alpha`
- **WHEN** the user runs `wt open --select alpha --app open_here`
- **THEN** the worktree is selected then opened (equivalent to the old `--go alpha --app open_here`).

- **GIVEN** a repo with a worktree `alpha`
- **WHEN** the user runs `wt open --go alpha --app open_here`
- **THEN** the same behavior occurs and a deprecation warning naming `--select` is printed to stderr.

#### R11: `--app` gains short `-a`
`wt open` SHALL add short flag `-a` to the existing `--app` flag (long name unchanged).

- **GIVEN** a repo with a worktree `alpha`
- **WHEN** the user runs `wt open -a open_here alpha`
- **THEN** the worktree opens in the named app (equivalent to `--app open_here`).

### wt list (`src/cmd/wt/list.go`)

#### R12: `wt ls` command alias
`wt list` SHALL carry cobra `Aliases: []string{"ls"}` so `wt ls` invokes it identically. No flag changes.

- **GIVEN** a repo
- **WHEN** the user runs `wt ls`
- **THEN** the worktree list is printed exactly as `wt list` would.

### wt update (`src/cmd/wt/update.go`)

#### R13: `--skip-brew-update` → `--no-brew-update`
`wt update` SHALL accept a real boolean `--no-brew-update` as the primary flag; `--skip-brew-update` remains a hidden deprecated alias bound to the same bool variable. Semantics unchanged (skip the internal `brew update` tap-metadata refresh; version check and upgrade still run).

- **GIVEN** a non-brew binary
- **WHEN** the user runs `wt update --no-brew-update`
- **THEN** the flag is accepted and threaded into `update.Run` (equivalent to `--skip-brew-update`).

### wt init (`src/cmd/wt/init.go`)

#### R14: Sharpen the `Short:` description only
`wt init`'s `Short:` SHALL be changed from `"Run worktree init script"` to `"Run the init script in the current worktree"` to counter the "git init"-style misreading. No behavior change, no flag change, no command rename.

- **GIVEN** the built binary
- **WHEN** the user runs `wt init --help` or `wt --help`
- **THEN** the `init` command's short description reads "Run the init script in the current worktree".

### Docs & Help Ripple

#### R15: In-change doc surfaces reflect the new flag names
The following in-change docs SHALL be updated so new names are primary and old names are noted as deprecated aliases where a table lists them:
- `docs/specs/cli-surface.md` — per-subcommand flag reference (create/delete/open flags, the `--go`/`--select` flag, resolution-order text). Also fix the pre-existing staleness: the `wt update` section claims "No flags" but `--skip-brew-update`/`--no-brew-update` exists today.
- `docs/specs/worktree-layout.md` — the `--worktree-name` override references → `--name`; the `--delete-branch auto` reference → `--branch auto`.
- `README.md` line 78 — the `wt create` key-flags list: `--worktree-name` → `--name`.

- **GIVEN** the updated docs
- **WHEN** a reader consults the create/delete/open flag tables
- **THEN** the primary names shown are the new ones, with old names marked deprecated where enumerated.

### Non-Goals

- Renaming `--non-interactive` (Constitution VI; script-facing; four commands) — unchanged.
- Renaming/removing `wt init`, `wt shell-init`, `wt help-dump`, `wt go` commands — out of scope.
- A `-b` short for `--base` (collides with `git worktree add -b`) — rejected.
- `NoOptDefVal` on `--open` (positional `[branch]` would swallow bare `--open code`) — rejected.
- Splitting `--delete-branch`'s tri-state into two bools — rejected (keep the string).
- Migrating existing test invocations of old flags — selective, not mandatory; existing old-flag tests double as back-compat coverage.

### Design Decisions

1. **String→bool conversions use two variables with `Changed()` reconciliation**: For `--worktree-init`→`--no-init` and `--delete-remote`→`--no-remote`, the types differ (string vs bool), so a shared variable is impossible. The old string flag stays on its own variable (deprecated); `RunE` reconciles by giving the new bool precedence when `cmd.Flags().Changed("<new>")` reports it explicitly set, else honoring the old string path. — *Why*: explicit-new-wins is the conventional precedence and is trivially reversible in one function. — *Rejected*: forcing one shared variable (impossible across types); dropping the old flag (breaks back-compat).
2. **Same-type renames share one variable**: For `--worktree-name`/`--name`, `--worktree-open`/`--open`, `--go`/`--select`, `--delete-all`/`--all`, `--delete-branch`/`--branch`, `--skip-brew-update`/`--no-brew-update`, both flags bind the same pointer (pflag permits this). Where precedence could matter, an explicitly-set flag wins over an unset one; in practice a user passes at most one of the pair. — *Why*: simplest correct mechanism; the shared pointer means the last-set value is honored and `MarkDeprecated` handles the warning. — *Rejected*: two variables + reconciliation for same-type flags (needless complexity when the pointer can be shared).
3. **Deprecation message wording `"use --<new> instead"`** matches the existing `delete.go:152` precedent (`"use positional arguments instead"` form). — *Why*: codebase precedent; low-stakes copy.

## Tasks

### Phase 1: Command implementation (per-file, parallelizable)

- [x] T001 [P] `wt create` (`src/cmd/wt/create.go`): add `name` string var bound to `--name` with short `-n` (help "Set worktree name (skips name prompt)"); keep `--worktree-name` registered on the same `worktreeName` var and `MarkDeprecated("worktree-name", "use --name instead")`. Add `open` handling: register `--open`/`-o` on the same `worktreeOpen` var (help copied from the old flag) and `MarkDeprecated("worktree-open", "use --open instead")`. Add real-bool `noInit` var bound to `--no-init` (help "Skip the worktree init script"); keep `--worktree-init` string flag registered + `MarkDeprecated("worktree-init", "use --no-init instead")`. Reconcile in `RunE`: if `cmd.Flags().Changed("no-init")` then treat init as `!noInit`, else fall back to the existing `worktreeInit` string default/parse. Add `Aliases: []string{"new"}` to the command. Do NOT set `NoOptDefVal` on `--open`. <!-- R2 R3 R4 R5 -->
- [x] T002 [P] `wt delete` (`src/cmd/wt/delete.go`): register `--all`/`-a` on the same `deleteAll` bool var and `MarkDeprecated("delete-all", "use --all instead")`. Register `--branch` on the same `deleteBranch` string var (help copied from `--delete-branch`) and `MarkDeprecated("delete-branch", "use --branch instead")`. Add real-bool `noRemote` var bound to `--no-remote` (help "Do not delete the branch on the origin remote when the local branch is deleted"); keep `--delete-remote` string flag registered + `MarkDeprecated("delete-remote", "use --no-remote instead")`. Reconcile in `RunE`: if `cmd.Flags().Changed("no-remote")` then set `deleteRemote` to `"false"` when `noRemote` else `"true"`, else keep the existing `deleteRemote == "" → "true"` default/parse. Add `Aliases: []string{"rm"}`. Keep the existing `--worktree-name` deprecation and `--stale` NoOptDefVal untouched. <!-- R6 R7 R8 R9 -->
- [x] T003 [P] `wt open` (`src/cmd/wt/open.go`): register `--select` on the same `goFlag` bool var (help "Select a worktree (menu or by name) first, then launch it") and `MarkDeprecated("go", "use --select instead")`. Add short `-a` to the existing `--app` flag (`StringVarP`). <!-- R10 R11 -->
- [x] T004 [P] `wt list` (`src/cmd/wt/list.go`): add `Aliases: []string{"ls"}` to the command. <!-- R12 -->
- [x] T005 [P] `wt update` (`src/cmd/wt/update.go`): add real-bool `noBrewUpdate` var bound to `--no-brew-update` sharing behavior with `skipBrewUpdate`; register both on the same bool var (`BoolVar` twice on the same pointer) and `MarkDeprecated("skip-brew-update", "use --no-brew-update instead")`. Keep threading the resolved value into `update.Run`. <!-- R13 -->
- [x] T006 [P] `wt init` (`src/cmd/wt/init.go`): change `Short:` from `"Run worktree init script"` to `"Run the init script in the current worktree"`. No other change. <!-- R14 -->

### Phase 2: Tests (test-alongside)

- [x] T007 [P] `create_test.go`: add tests for new names/shorts/alias/deprecation — `-n`/`--name` creates a named worktree; `-o`/`--open` controls the open phase; `--no-init` skips init; `wt new` alias works; passing `--worktree-name`/`--worktree-open`/`--worktree-init` still works and prints a deprecation warning to stderr (assert `Stderr` contains "deprecated"); `--worktree-name`/`--worktree-open`/`--worktree-init` are absent from `wt create --help` stdout while `--name`/`--open`/`--no-init` are present. Use non-side-effecting open targets (`skip`/`open_here`) per code-review.md. <!-- R2 R3 R4 R5 R1 -->
- [x] T008 [P] `delete_test.go`: add tests for `--all`/`-a` deleting all; `--branch true` force-deletes; `--no-remote` suppresses remote deletion; `wt rm` alias works; `--delete-all`/`--delete-branch`/`--delete-remote` still work and warn on stderr; deprecated old names absent from `wt delete --help` while `--all`/`--branch`/`--no-remote` present. <!-- R6 R7 R8 R9 R1 -->
- [x] T009 [P] `open_test.go`: add tests for `--select` composing select-then-launch (mirror an existing `--go` test, using `open_here`); `--go` still works and warns on stderr; `-a`/`--app` short form works; `--go` absent from `wt open --help` while `--select` present. <!-- R10 R11 R1 -->
- [x] T010 [P] `list_test.go`: add a test that `wt ls` produces the same output shape as `wt list` (alias works). <!-- R12 -->
- [x] T011 [P] `update_test.go`: add tests that `--no-brew-update` is accepted (reaches `update.Run`), appears in `wt update --help`, and that `--skip-brew-update` still works and warns on stderr and is absent from `--help`. <!-- R13 R1 -->
- [x] T012 [P] `init_test.go`: add/adjust a test asserting `wt init --help` (or `wt --help`) shows the new Short "Run the init script in the current worktree". <!-- R14 -->

### Phase 3: Docs ripple

- [x] T013 `docs/specs/cli-surface.md`: update the `wt create` flags table (`--name`/`-n`, `--open`/`-o`, `--no-init` primary; note old names deprecated), the `wt delete` flags table (`--all`/`-a`, `--branch`, `--no-remote` primary; old names deprecated) and its resolution-order text, the `wt open` `--select` flag (was `--go`) and the `--app`/`-a` short, add `wt new`/`wt rm`/`wt ls` aliases where the command headers are, and fix the stale `wt update` "No flags" claim to document `--no-brew-update` (old `--skip-brew-update` deprecated). <!-- R15 -->
- [x] T014 [P] `docs/specs/worktree-layout.md`: replace `--worktree-name` override references with `--name`; update the `--delete-branch auto` reference to `--branch auto`. <!-- R15 -->
- [x] T015 [P] `README.md` line 78: change `--worktree-name` to `--name` in the `wt create` key-flags list. <!-- R15 -->

## Execution Order

- Phase 1 (T001–T006) are independent per-file and can run in parallel; each is a prerequisite for its matching Phase-2 test task (T007↔T001, T008↔T002, T009↔T003, T010↔T004, T011↔T005, T012↔T006).
- Phase 3 (T013–T015) is independent of code and may run any time; no ordering constraint among them.

## Acceptance

### Functional Completeness

- [x] A-001 R1: Every renamed flag keeps its old name as a hidden deprecated alias; no old flag is removed; the deprecation warning is on stderr.
- [x] A-002 R2: `wt create -n <name>` / `--name` creates the named worktree; `--worktree-name` still works (deprecated).
- [x] A-003 R3: `wt create -o <mode>` / `--open` controls the open phase; `--worktree-open` still works (deprecated); `--open` has no `NoOptDefVal`.
- [x] A-004 R4: `wt create --no-init` skips init (real bool); `--worktree-init false` still works (deprecated); `Changed("no-init")` gives the new flag precedence.
- [x] A-005 R5: `wt new` invokes `wt create` identically.
- [x] A-006 R6: `wt delete -a` / `--all` deletes all; `--delete-all` still works (deprecated).
- [x] A-007 R7: `wt delete --branch <true|false|auto>` controls branch cleanup (string tri-state); `--delete-branch` still works (deprecated).
- [x] A-008 R8: `wt delete --no-remote` suppresses remote-branch deletion (real bool); `--delete-remote false` still works (deprecated); `Changed("no-remote")` gives the new flag precedence.
- [x] A-009 R9: `wt rm` invokes `wt delete` identically; `--stash`/`--stale`/`--non-interactive`/existing `--worktree-name` deprecation untouched.
- [x] A-010 R10: `wt open --select` composes select-then-launch; `--go` still works (deprecated), no short flag.
- [x] A-011 R11: `wt open -a <app>` / `--app` opens in the named app.
- [x] A-012 R12: `wt ls` invokes `wt list` identically.
- [x] A-013 R13: `wt update --no-brew-update` skips the brew metadata refresh; `--skip-brew-update` still works (deprecated).
- [x] A-014 R14: `wt init`'s Short reads "Run the init script in the current worktree"; no behavior/flag change.
- [x] A-015 R15: `cli-surface.md`, `worktree-layout.md`, and `README.md` reflect the new primary names; the stale `wt update` "No flags" claim is fixed.

### Behavioral Correctness

- [x] A-016 R1: Passing a NEW flag name prints NO deprecation warning; passing an OLD flag name prints the `Flag --<old> has been deprecated, use --<new> instead` warning to stderr and behaves identically to before.
- [x] A-017 R1: Deprecated old flags are absent from each command's `--help` stdout; the new primary flags are present.
- [x] A-018 R4 R8: For the string→bool conversions, when neither the new bool nor the old string flag is passed, default behavior (init runs / remote deleted) is preserved byte-for-byte.

### Scenario Coverage

- [x] A-019 R2 R3 R4 R5: `create_test.go` exercises new names/shorts, the `new` alias, and old-name deprecation + help-hiding.
- [x] A-020 R6 R7 R8 R9: `delete_test.go` exercises `--all`/`-a`, `--branch`, `--no-remote`, the `rm` alias, and old-name deprecation + help-hiding.
- [x] A-021 R10 R11: `open_test.go` exercises `--select`, `-a`, and `--go` deprecation.
- [x] A-022 R12 R13 R14: `list_test.go` (`ls` alias), `update_test.go` (`--no-brew-update` + `--skip-brew-update` deprecation), `init_test.go` (new Short).

### Edge Cases & Error Handling

- [x] A-023 R3: `wt create --open` given no value errors (no `NoOptDefVal`) rather than swallowing a following positional as the value.
- [x] A-024 R4 R8: When BOTH the new bool and the old string are somehow set, the new bool (`Changed()`) wins deterministically.

### Code Quality

- [x] A-025 Pattern consistency: New flag registrations and `MarkDeprecated` calls follow the existing `delete.go:152` precedent and the surrounding cobra flag-registration shape (constructor `xxxCmd() *cobra.Command`, `cmd.Flags().XxxVar(...)`, `return cmd`).
- [x] A-026 No unnecessary duplication: Same-type renames share one variable rather than duplicating parsing; string→bool conversions reuse the existing parse path via `Changed()` reconciliation, not a reimplemented parser.
- [x] A-027 No magic strings: Deprecation messages use the consistent `"use --<new> instead"` wording; no ad-hoc exit codes introduced (Constitution III unaffected — no exit-code changes).
- [x] A-028 cmd/ stays thin (Constitution V): all changes are flag parsing / cobra metadata in `cmd/`; no `internal/worktree` logic touched.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `src/cmd/wt/update_test.go: TestUpdate_SkipBrewUpdateFlagHiddenFromHelp` — its hidden-from-help assertion is duplicated verbatim inside `TestUpdate_NoBrewUpdateFlagInHelp` (added by this same change); one of the two tests is redundant.
- `src/cmd/wt/create.go:575-589, src/cmd/wt/delete.go:157-169, src/cmd/wt/open.go:159-160, src/cmd/wt/update.go:37-39` (the eight deprecated old-flag registrations + `MarkDeprecated` calls) — superseded by the new primary flags but deliberately retained per R1's back-compat mandate; deletable only after an announced deprecation window, not now.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Mechanism: new flag primary + old flag kept + `MarkDeprecated("<old>", "use --<new> instead")`; pflag warning fires only when the old flag is passed and lands on stderr (f.out() default) | Verified pflag v1.0.5 source (Set() → f.out()=os.Stderr) and the delete.go:152 precedent + its `TestDelete_DeprecatedFlagStillWorks` asserting stderr "deprecated" | S:95 R:85 A:95 D:95 |
| 2 | Certain | Same-type renames (`--name`,`--open`,`--branch`,`--all`,`--select`,`--no-brew-update`) bind the SAME variable as their old flag; string→bool (`--no-init`,`--no-remote`) use a separate bool var reconciled via `Changed()` | Intake § "String→bool conversion mechanics" states this verbatim; pflag permits two flags sharing a pointer | S:95 R:85 A:95 D:90 |
| 3 | Certain | `--open` gets NO `NoOptDefVal`; `--select` gets no short flag; `-a` used for both `wt open --app` and `wt delete --all` (different commands, no collision); `-n`/`-o` on create; `-a` free on delete (`-s`=stash) | Intake specifies each explicitly incl. the rejected-alternatives rationale for `--open` | S:95 R:90 A:95 D:95 |
| 4 | Confident | `--no-init` help text "Skip the worktree init script"; `--no-remote` help "Do not delete the branch on the origin remote when the local branch is deleted"; new same-type flags copy the old flag's help string verbatim | Intake gives semantics but not exact new-flag help copy; UX copy is trivially reversible and low-stakes | S:65 R:95 A:80 D:75 |
| 5 | Confident | `wt init` Short lands as "Run the init script in the current worktree" (the intake's "e.g." wording, delegated to implementer) | Intake assumption #9 supplies this as example wording with an explicit delegation signal | S:70 R:95 A:80 D:75 |
| 6 | Confident | Doc ripple: update cli-surface.md (incl. fixing the stale `wt update` "No flags" claim), worktree-layout.md `--worktree-name`/`--delete-branch` refs, and README.md line 78; document old names as deprecated where tables enumerate them | Intake § Ripple surfaces + assumption #10 name these exactly; pure docs, fully reversible | S:70 R:95 A:85 D:80 |
| 7 | Confident | Tests add coverage for new names/shorts/aliases/deprecation-warnings + help-hiding, using non-side-effecting open targets (`skip`/`open_here`) per code-review.md; existing old-flag tests stay as living back-compat coverage | Constitution IV + code-review.md side-effect policy; keeping old-name tests is the cheapest back-compat proof (intake assumption #11) | S:70 R:90 A:85 D:75 |

7 assumptions (3 certain, 4 confident, 0 tentative).
