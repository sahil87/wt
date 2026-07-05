# Plan: Graceful Default-Init Skip in Non-Fab-Managed Repos

**Change**: 260705-irnt-graceful-default-init-skip
**Intake**: `intake.md`

## Requirements

### Init: Default-Script Provenance

#### R1: `InitScriptPath` reports whether the value is the built-in default
`InitScriptPath()` (`src/internal/worktree/context.go`) SHALL report the provenance of the resolved init-script value in addition to the value itself, changing its signature to return `(script string, isDefault bool)`. `isDefault` MUST be true **only** when `WORKTREE_INIT_SCRIPT` is unset or empty (the built-in `"fab sync"` default), and false whenever the env var is set to any non-empty value — including the literal `"fab sync"`. Provenance is determined by env-var presence, never by string equality against `"fab sync"`.

- **GIVEN** `WORKTREE_INIT_SCRIPT` is unset (or empty)
- **WHEN** `InitScriptPath()` is called
- **THEN** it returns `("fab sync", true)`

- **GIVEN** `WORKTREE_INIT_SCRIPT="fab sync"` (explicit, string matches the default)
- **WHEN** `InitScriptPath()` is called
- **THEN** it returns `("fab sync", false)` — provenance is false because the value came from the env var

- **GIVEN** `WORKTREE_INIT_SCRIPT="custom/init.sh"`
- **WHEN** `InitScriptPath()` is called
- **THEN** it returns `("custom/init.sh", false)`

### Init: Run-Time Skip Classification

#### R2: A shared helper classifies the default-not-applicable outcome
`src/internal/worktree/init.go` SHALL expose a single shared classification helper that decides, from `(err error, isDefault bool)`, whether an init-script non-zero exit is the "default does not apply" skip case. It MUST return true **only** when `isDefault` is true AND `err` unwraps (via `errors.As`) to an `*exec.ExitError` whose exit code is exactly `3` (fab-kit's `ExitNotManaged`). It MUST return false for: a nil `err`; any exit code other than 3; an `err` that does not unwrap to `*exec.ExitError`; and any case where `isDefault` is false (regardless of exit code, including 3). The classification is a run-time decision made after `cmd.Run()`; `ResolveInitInvocation` and `InitNotFound` are untouched.

- **GIVEN** the default script ran and exited 3, `isDefault=true`
- **WHEN** the helper classifies the outcome
- **THEN** it returns true (skip)

- **GIVEN** an explicit script (`isDefault=false`) exited 3
- **WHEN** the helper classifies the outcome
- **THEN** it returns false (hard failure — provenance rule)

- **GIVEN** the default script exited 1 (`isDefault=true`)
- **WHEN** the helper classifies the outcome
- **THEN** it returns false (hard failure — only exit 3 skips)

- **GIVEN** a nil error, or a non-`*exec.ExitError` error (`isDefault=true`)
- **WHEN** the helper classifies the outcome
- **THEN** it returns false (not a skip)

#### R3: A canonical warning renderer produces the skip copy
`src/internal/worktree/init.go` SHALL expose a single renderer producing the canonical two-line skip warning, so both run sites emit byte-identical copy (anti-drift, mirroring `RenderWarning()`). The two lines MUST convey (1) that the repo is not fab-managed and the default `"fab sync"` is being skipped, and (2) the two escape hatches (set `WORKTREE_INIT_SCRIPT`, or run `fab init`).

- **GIVEN** the skip case is classified
- **WHEN** the warning is rendered and written to stderr
- **THEN** it contains "not a fab-managed repo", "skipping init", `WORKTREE_INIT_SCRIPT`, and `fab init`

### Init: Skip Semantics at Run Sites

#### R4: The `wt create` runner treats the skip as an init no-op
`RunWorktreeSetup` / `RunWorktreeSetupWithObserver` (`src/internal/worktree/crud.go`) SHALL accept the `isDefault` provenance and, after `cmd.Run()` returns a non-zero error, apply the shared classification helper. On the skip case it MUST write the canonical skip warning to stderr and return `nil` (init treated as a no-op) WITHOUT printing `Worktree init complete.`. On every non-skip non-zero exit it MUST return the wrapped error exactly as today. Success and not-found paths are unchanged.

- **GIVEN** `wt create` runs the default init in a non-fab repo where `fab sync` exits 3
- **WHEN** the runner classifies the outcome
- **THEN** it prints the skip warning, returns nil, and does not print `Worktree init complete.`

- **GIVEN** `wt create` runs the default init and `fab sync` exits 1
- **WHEN** the runner classifies the outcome
- **THEN** it returns the wrapped init error (hard failure preserved)

#### R5: `wt create` proceeds normally on the skip (no banner, no prompt, exit 0)
`src/cmd/wt/create.go` SHALL pass the `isDefault` provenance to the runner at both init call sites (the `--reuse` path and the normal path). Because the skip surfaces as a `nil` return, `initFailed` is never set, so no init-failure banner is printed, no open-anyway prompt is shown, the Open phase proceeds exactly as on init success, and the process exits 0 with the stdout final-path line unaffected. The `--reuse` path inherits the skip identically (its init step is already warn-but-continue).

- **GIVEN** a non-fab repo where the default `fab sync` exits 3
- **WHEN** `wt create --non-interactive` runs
- **THEN** the worktree is created, the skip warning is on stderr, no failure banner appears, and the exit code is 0

#### R6: `wt init` treats the skip as success (exit 0), preserving reclaim ordering
`src/cmd/wt/init.go` SHALL pass the `isDefault` provenance and apply the shared classification helper on its own `cmd.Run()` failure path, before `os.Exit(wt.ExitInitFailed)`. On the skip case it MUST reclaim the terminal foreground first (preserving the existing reclaim-before-write ordering), then write the skip warning to stderr and `return nil` (exit 0) — NOT printing `Worktree init complete.`. On every non-skip non-zero exit it MUST exit `ExitInitFailed` exactly as today.

- **GIVEN** a non-fab repo where the default `fab sync` exits 3
- **WHEN** `wt init` runs
- **THEN** the terminal foreground is reclaimed, the skip warning is on stderr, and the exit code is 0

- **GIVEN** an explicit `WORKTREE_INIT_SCRIPT` script exits 3, or the default exits any non-3 code
- **WHEN** `wt init` runs
- **THEN** it exits `ExitInitFailed = 7` (hard failure preserved)

### Init: No-Regression for Older fab-kit

#### R7: Older fab-kit (exit 1) degrades to today's hard-fail
The feature SHALL NOT introduce version detection or a fallback probe. An installed fab-kit predating PR #471 exits 1 in non-fab repos, which is not exit 3, so `wt` behaves exactly as today (hard fail, `ExitInitFailed = 7`, banner). The only behavior that changes is "built-in default script + exit 3".

- **GIVEN** an older fab-kit that exits 1 in a non-fab repo, default init
- **WHEN** `wt create` runs
- **THEN** it exits 7 with the failure banner, unchanged from today

### Docs: Spec Update

#### R8: `docs/specs/init-protocol.md` documents the skip contract
`docs/specs/init-protocol.md` SHALL be updated per intake §6: the Lookup contract notes `InitScriptPath` reports provenance (not string equality); the Graceful skip behavior section adds case 3 (default init not applicable — `fab sync` exited `ExitNotManaged = 3`, run-time classification, warning text, exit 0; explicit same-string value never skipped; older fab-kit degrades to hard-fail); and Script failure semantics notes the exit-3 carve-out for the default script (all other non-zero exits keep `ExitInitFailed = 7`).

- **GIVEN** a reader consulting the init-protocol spec
- **WHEN** they read the Lookup contract, Graceful skip, and Script failure sections
- **THEN** they find the provenance report, the case-3 skip, and the exit-3 carve-out accurately described

### Design Decisions

1. **Skip keys on fab-kit's `ExitNotManaged = 3`, not a wt-side marker probe**: `wt` runs `fab sync` as today and treats exit code 3 as "default does not apply". — *Why*: fab stays the sole authority on what is fab-managed; wt embeds no knowledge of fab's repo layout. — *Rejected*: the original wt-side `fab/project/config.yaml` walk-up (duplicated fab's detection in wt, silent-drift risk if fab moved the marker; superseded by the released exit-code contract, fab-kit PR #471).
2. **Provenance-gated skip (built-in default only)**: an explicit `WORKTREE_INIT_SCRIPT="fab sync"` still hard-fails on any non-zero exit including 3. — *Why*: the user never opted into the default; an explicit script's failure is theirs to see. — *Rejected*: string-equality gating (would silence an explicitly-configured script the user chose).
3. **One shared classification helper + warning renderer in `init.go`**: both run sites (`crud.go`, `cmd/wt/init.go`) route through it. — *Why*: the decision needs the post-run exit code and must not drift, mirroring the existing `RenderWarning()` anti-drift pattern. — *Rejected*: inlining the exit-code check at each site (drift risk).
4. **Post-run detection accepts pre-skip output noise**: the Init separator, `Running worktree init...`, and fab's own `ERROR: not in a fab-managed repo...` line print before the skip warning. — *Why*: inherent to run-time detection; more transparent than suppressing; the skip warning is the last word and the exit code is 0.

### Non-Goals

- Version-detecting fab-kit or probing for the config marker as a fallback (rejected — reintroduces the coupling the design revision removed).
- Changing any existing exit code or the failure banner/prompt on real failures.
- Suppressing fab's own pre-skip stderr output.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Change `InitScriptPath()` in `src/internal/worktree/context.go` to return `(script string, isDefault bool)` — `isDefault` true only when `WORKTREE_INIT_SCRIPT` is unset/empty. Update its doc comment. <!-- R1 -->
- [x] T002 Add the shared classification helper and canonical skip-warning renderer to `src/internal/worktree/init.go` (e.g. `func DefaultNotApplicable(err error, isDefault bool) bool` using `errors.As` → `*exec.ExitError` with `ExitCode()==3`, plus `func RenderDefaultSkipWarning() string`). Import `errors`. <!-- R2 --> <!-- R3 -->
- [x] T003 Thread `isDefault` through `RunWorktreeSetup` and `RunWorktreeSetupWithObserver` in `src/internal/worktree/crud.go`; after `cmd.Run()` non-zero, apply the classification helper: skip case → print `RenderDefaultSkipWarning()` to stderr, return nil (no `Worktree init complete.`); otherwise return the wrapped error as today. <!-- R4 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Update both `wt create` init call sites in `src/cmd/wt/create.go` — the `--reuse` path (`RunWorktreeSetup`) and the normal path (`RunWorktreeSetupWithObserver`) — to capture and pass the `isDefault` provenance from `InitScriptPath()`. No other logic change; the skip surfaces as a nil return so `initFailed` stays false. <!-- R5 -->
- [x] T005 Update `runInitScript()` in `src/cmd/wt/init.go` — capture provenance from `InitScriptPath()`, and on the `cmd.Run()` failure path apply the classification helper: skip case → reclaim terminal foreground first (preserve reclaim-before-write ordering), write `RenderDefaultSkipWarning()` to stderr, `return nil` (exit 0, no `Worktree init complete.`); otherwise exit `ExitInitFailed` as today. <!-- R6 --> <!-- R7 -->

### Phase 4: Tests & Docs

- [x] T006 [P] Update `InitScriptPath` provenance unit tests in `src/internal/worktree/context_test.go` (`TestInitScriptPath_Default` → `("fab sync", true)`; `TestInitScriptPath_Custom` → `(value, false)`; add explicit `WORKTREE_INIT_SCRIPT="fab sync"` → `("fab sync", false)`). <!-- R1 -->
- [x] T007 [P] Add classification-helper table test and warning-renderer test to `src/internal/worktree/init_test.go`: (exit 3, default)→skip; (exit 3, explicit)→no-skip; (exit 1, default)→no-skip; (nil err, default)→no-skip; (non-`*exec.ExitError`, default)→no-skip; and assert the rendered warning contains the fab-managed/skip copy plus both escape hatches. <!-- R2 --> <!-- R3 -->
- [x] T008 Add integration tests to `src/cmd/wt/integration_test.go` using a stub `fab` in a `t.TempDir()` prepended to PATH (no host side-effects): stub exits 3 + default init → `wt create --non-interactive` exits 0 (worktree created, skip warning on stderr, no failure banner) and `wt init` exits 0; stub exits 1 → `wt create` exits 7 with banner (unchanged); explicit `WORKTREE_INIT_SCRIPT="fab sync"` + stub exits 3 → exit 7 (provenance rule). <!-- R5 --> <!-- R6 --> <!-- R7 -->
- [x] T009 Update `docs/specs/init-protocol.md` per intake §6: Lookup contract (provenance report), Graceful skip behavior (case 3), Script failure semantics (exit-3 carve-out for the default script). <!-- R8 -->

## Execution Order

- T001 and T002 are the foundation; T003 depends on both (needs the new signature + helper).
- T004 and T005 depend on T001–T003 (call-site threading needs the new signatures/helper).
- T006 and T007 [P] depend only on T001/T002 respectively; T008 depends on T003–T005 (exercises the built binary end-to-end).
- T009 (docs) is independent of code and may run any time after the design is fixed.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `InitScriptPath()` returns `(script, isDefault)` with `isDefault` true only when `WORKTREE_INIT_SCRIPT` is unset/empty.
- [x] A-002 R2: A single shared classification helper in `init.go` decides the default-not-applicable skip from `(err, isDefault)`.
- [x] A-003 R3: A single canonical renderer in `init.go` produces the two-line skip warning; both run sites use it.
- [x] A-004 R4: `RunWorktreeSetup`/`RunWorktreeSetupWithObserver` treat the skip as an init no-op (warning + nil, no `Worktree init complete.`).
- [x] A-005 R5: `wt create` passes provenance at both call sites and proceeds normally on the skip (no banner, no prompt, exit 0).
- [x] A-006 R6: `wt init` reclaims the foreground, warns, and exits 0 on the skip; hard-fails (exit 7) otherwise.
- [x] A-007 R8: `docs/specs/init-protocol.md` documents provenance, the case-3 skip, and the exit-3 carve-out.

### Behavioral Correctness

- [x] A-008 R1: An explicit `WORKTREE_INIT_SCRIPT="fab sync"` yields `isDefault=false` (provenance, not string equality).
- [x] A-009 R2: Only `isDefault=true` AND exit code exactly 3 classifies as skip; explicit-3, default-non-3, nil, and non-`*exec.ExitError` all classify as not-skip.
- [x] A-010 R4: The skip path does not print `Worktree init complete.`.

### Scenario Coverage

- [x] A-011 R5: Integration — stub `fab` exit 3 + default init → `wt create --non-interactive` exits 0, worktree created, skip warning on stderr, no failure banner.
- [x] A-012 R6: Integration — stub `fab` exit 3 + default init → `wt init` exits 0.
- [x] A-013 R6: Integration — explicit `WORKTREE_INIT_SCRIPT="fab sync"` + stub exit 3 → exit 7 (provenance rule).
- [x] A-014 R1/R2/R3: Unit tests cover `InitScriptPath` provenance, the classification helper table, and the warning renderer.

### Edge Cases & Error Handling

- [x] A-015 R7: Integration — stub `fab` exit 1 (older fab-kit) + default init → `wt create` exits 7 with banner, unchanged from today.
- [x] A-016 R6: `wt init` skip path reclaims the terminal foreground BEFORE writing the skip warning (reclaim-before-write ordering preserved).

### Code Quality

- [x] A-017 Pattern consistency: New code follows the naming and structural patterns of surrounding code (typed helpers on/near `init.go`, `fmt.Fprintln(os.Stderr, ...)` for diagnostics, RFC-style file-per-source tests).
- [x] A-018 No unnecessary duplication: The exit-3 classification and skip-warning copy live in one shared helper each (mirroring `RenderWarning()`); no re-implementation at the two run sites.
- [x] A-019 No magic numbers: The exit code `3` carries a named/commented reference to fab-kit's `ExitNotManaged`; no unexplained literal.
- [x] A-020 Test integrity: Integration tests use a stub `fab` in a `t.TempDir()` PATH with no host side-effects (per code-review.md).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- The intake references `src/cmd/integration_test.go`; the actual path is `src/cmd/wt/integration_test.go` (see Assumptions row 3).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `InitScriptPath` returns `(string, bool)` (two return values), not an options struct | Intake left the exact seam to apply (assumption 7); a second return value is the minimal, idiomatic Go shape and matches the sibling resolver's multi-return style; internal package, trivially reversible | S:70 R:90 A:85 D:75 |
| 2 | Certain | Runner threading via an added `isDefault bool` parameter on `RunWorktreeSetup`/`RunWorktreeSetupWithObserver`, not an options struct | Intake left the seam to apply; a bool param matches the existing positional-arg signatures (`wtPath, initScript, repoRoot`); internal API, only two callers, reversible in review | S:70 R:85 A:85 D:75 |
| 3 | Certain | Integration tests go in `src/cmd/wt/integration_test.go` (intake said `src/cmd/integration_test.go`) | The repo has no `src/cmd/integration_test.go`; the actual integration suite and `runWt`/`createTestRepo` helpers live in `src/cmd/wt/integration_test.go` — the intake path was a shorthand; verified by directory listing | S:95 R:90 A:95 D:95 |
| 4 | Confident | Helper names `DefaultNotApplicable(err, isDefault) bool` and `RenderDefaultSkipWarning() string` | Intake suggested `DefaultNotApplicable` verbatim as an example; the renderer name mirrors the existing `RenderWarning()`; both are internal, easily renamed in review | S:75 R:90 A:80 D:70 |
| 5 | Confident | Skip warning copy uses the intake's exact two lines (`Warning: not a fab-managed repo — skipping init (default "fab sync" does not apply)` + the escape-hatch line) | Intake gave the wording and said it MAY be polished at apply; adopting it verbatim keeps both required halves and follows stderr conventions; reversible | S:70 R:90 A:85 D:75 |
| 6 | Confident | Exit code `3` is referenced via a named local const / commented literal citing fab-kit `ExitNotManaged` rather than importing from fab-kit | wt has no dependency on fab-kit (separate repo, no import per context.md); a documented literal is the no-new-dependency choice consistent with Constitution I; the value is fab-kit's documented contract | S:70 R:85 A:85 D:70 |

6 assumptions (3 certain, 3 confident, 0 tentative).
