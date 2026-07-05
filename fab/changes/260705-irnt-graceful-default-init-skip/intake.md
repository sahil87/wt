# Intake: Graceful Default-Init Skip in Non-Fab-Managed Repos

**Change**: 260705-irnt-graceful-default-init-skip
**Created**: 2026-07-05

## Origin

Conversational (`/fab-discuss` session, 2026-07-05). The user shared a screenshot of `wt create` in a non-fab-managed repo (`planner`): the git phase succeeded, then the default init (`fab sync`) failed with `ERROR: not in a fab-managed repo. Run 'fab init' to set one up`, producing the init-failure banner, the open-anyway prompt, and exit 7.

> wt should continue to work without errors even in "non fab-kit managed repo". What changes do we need for this?

**Design revision (same day)**: the intake originally specified a wt-side marker probe (walk up looking for `fab/project/config.yaml`) because `fab sync` offered no distinguishable outcome — that fab-side contract was backlogged as fab-kit `[52i9]`. The user then implemented and released it (fab-kit PR #471): `fab sync` now exits **`ExitNotManaged = 3`** when not in a fab-managed repo, checked via a config walk-up *before* any git resolution, symmetric with `fab-kit migrations-status`. This intake was revised to key the skip on that exit code; the marker probe is now the rejected alternative.

## Why

1. **Pain point**: `wt`'s default init script is `"fab sync"` (`InitScriptPath()`, `src/internal/worktree/context.go:176`). The existing graceful-skip contract only covers `fab` *not being installed* (`CommandNotOnPath`) or a script file not existing (`FileNotFound`). When fab-kit IS installed but the repo is not fab-managed, `fab sync` runs and fails, and `wt` treats that as a real init failure: `ExitInitFailed = 7`, kept-worktree banner, interactive open-anyway prompt. The zero-config convenience default becomes an error in **every** non-fab repo.
2. **Consequence if unfixed**: every `wt create` / `wt init` in a non-fab repo errors and prompts; scripts and operators branching on exit codes see a spurious 7; users must set `WORKTREE_INIT_SCRIPT` per-repo or per-shell just to silence a default they never opted into.
3. **Why this approach**: the built-in default should skip gracefully when it does not apply; an explicitly configured script should keep failing hard (the user opted in, the failure is theirs to see). Detection: run `fab sync` as today and treat **exit code 3** (`ExitNotManaged`, fab-kit ≥ PR #471) as "default does not apply" — fab stays the sole authority on what is fab-managed; wt embeds no knowledge of fab's repo layout. Rejected alternative: the original wt-side marker probe (`fab/project/config.yaml` walk-up mirroring fab's `ResolveConfig`) — it avoided spawning `fab` but duplicated fab's detection logic in wt, with silent-under-sync drift risk if fab ever moved the marker. Superseded by the released exit-code contract.

## What Changes

### 1. `InitScriptPath()` signals defaultness (`src/internal/worktree/context.go`)

The init runner must know whether the value came from the built-in default or the env var:

```go
// Today:
func InitScriptPath() string {
    if v := os.Getenv("WORKTREE_INIT_SCRIPT"); v != "" {
        return v
    }
    return "fab sync"
}
```

Change to return `(script string, isDefault bool)` (or an equivalent small struct / second function — exact shape decided at apply). `isDefault` is true **only** when `WORKTREE_INIT_SCRIPT` is unset/empty. An explicit `WORKTREE_INIT_SCRIPT="fab sync"` yields `isDefault=false` even though the string matches — behavior must key on provenance, not string equality.

Callers to update: `src/cmd/wt/create.go:188` (reuse path), `src/cmd/wt/create.go:279` (normal path), `src/cmd/wt/init.go:63`.

### 2. Run-time skip classification (exit code 3, default only)

This is a **run-time** decision, not a resolve-time one — `ResolveInitInvocation` and the `InitNotFound` structure are untouched (resolution still succeeds; the script genuinely runs). After `cmd.Run()` returns a non-zero exit:

- `isDefault` **and** exit code == **3** (`ExitNotManaged`, extracted via `errors.As` → `*exec.ExitError`) → **not an init failure**: print the skip warning to stderr, treat init as a no-op (`nil` return / exit 0). No kept-worktree banner, no open-anyway prompt, `wt create` proceeds to the Open phase exactly as on init success.
- `isDefault` false → any non-zero exit (including 3) stays a hard failure, exactly as today. The provenance rule from the original design is unchanged.
- Exit codes other than 3 → hard failure regardless of provenance (`ExitInitFailed = 7` on every existing path).

Put the classification + warning rendering in one shared helper in `src/internal/worktree/init.go` (alongside `InitNotFound`, e.g. `func DefaultNotApplicable(err error, isDefault bool) bool` plus a canonical warning renderer) so the two run sites — `RunWorktreeSetupWithObserver` (`crud.go:166`) and `wt init`'s own `cmd.Run()` (`cmd/wt/init.go:115`) — cannot drift, mirroring how `RenderWarning()` already centralizes the not-found copy.

Agreed warning copy (final wording may be polished at apply, keeping both halves — the skip statement and the two escape hatches):

```
Warning: not a fab-managed repo — skipping init (default "fab sync" does not apply)
Set WORKTREE_INIT_SCRIPT to a custom script, or run 'fab init' to make this repo fab-managed.
```

**Accepted output tradeoff**: because the decision is post-run, the Init phase separator, `Running worktree init...`, and fab's own `ERROR: not in a fab-managed repo...` stderr line all print before the skip warning. That is acceptable (arguably more transparent than silently not running); the skip warning is the last word and the exit code is 0. `Worktree init complete.` MUST NOT print on the skip path.

### 3. Threading `isDefault` to the run sites

- `RunWorktreeSetup` / `RunWorktreeSetupWithObserver` (`src/internal/worktree/crud.go:134,143`) gain the `isDefault` knowledge (parameter or options struct — apply decides), applying the classification after `cmd.Run()`.
- `wt init` (`src/cmd/wt/init.go:115-127`) applies the same helper before its `os.Exit(wt.ExitInitFailed)`: default + exit 3 → skip warning + `return nil` (exit 0). Terminal-foreground reclaim ordering is preserved (reclaim before the warning write, as the failure path already does).
- `wt create`'s two call paths need no logic beyond passing `isDefault`: the skip surfaces as a `nil` return from the runner, so `initFailed` is never set — no banner, no prompt, exit 0, stdout final-path line unaffected. The `--reuse` path (`create.go:188`) inherits the skip the same way.

### 4. Old fab-kit degradation (accepted)

An installed fab-kit predating PR #471 exits 1 in non-fab repos → wt behaves exactly as today (hard fail, exit 7). No version detection, no fallback probe — the feature lights up when fab-kit updates. This is a strict no-regression posture: no existing behavior changes except "default script + exit 3", which old fab-kit never produces.

### 5. Tests

Unit (`src/internal/worktree/init_test.go`):
- Classification helper: (exit 3, default) → skip; (exit 3, explicit) → failure; (exit 1, default) → failure; (nil error) → not a skip; (non-`*exec.ExitError` error, default) → failure.
- Warning renderer output for the skip case.
- `InitScriptPath` provenance: env unset → (`"fab sync"`, true); env set to anything incl. `"fab sync"` → (value, false).

Integration (`src/cmd/integration_test.go`), using a stub `fab` in a `t.TempDir()` prepended to PATH (per `fab/project/code-review.md`, rely on `runWt` env isolation — no host side effects):
- Stub `fab` exits **3** + non-fab repo + default init → `wt create --non-interactive` exits 0, worktree created, skip warning on stderr, no failure banner; `wt init` exits 0.
- Stub `fab` exits **1** (old fab-kit) → `wt create` exits 7 with banner (unchanged).
- `WORKTREE_INIT_SCRIPT="fab sync"` explicit + stub exits 3 → exit 7 (provenance rule).

### 6. Spec update

`docs/specs/init-protocol.md`:
- § Lookup contract: `InitScriptPath` now reports defaultness (provenance, not string equality).
- § Graceful skip behavior: add **case 3** — default init not applicable: `fab sync` exited `ExitNotManaged = 3` (fab-kit ≥ PR #471); run-time classification, warning text, exit 0; explicit same-string value is never skipped; older fab-kit degrades to today's hard-fail.
- § Script failure semantics: note the exit-3 carve-out for the default script (all other non-zero exits keep `ExitInitFailed = 7` on every path).

Memory updates flow through hydrate (see Affected Memory).

## Affected Memory

- `wt-cli/init-failure-contract`: (modify) add the default-not-applicable skip as a run-time case that bypasses the failure contract entirely (no `ExitInitFailed`, no banner, no prompt) — provenance-gated (built-in default only) and keyed on fab-kit's documented `ExitNotManaged = 3`.

## Impact

- **Code**: `src/internal/worktree/context.go` (InitScriptPath signature), `src/internal/worktree/init.go` (classification helper + warning renderer), `src/internal/worktree/crud.go` (runner threading + post-run classification), `src/cmd/wt/create.go` (two call sites pass provenance), `src/cmd/wt/init.go` (call site + post-run classification before `os.Exit`).
- **Tests**: `init_test.go`, `context_test.go` (if InitScriptPath tests exist), `cmd/integration_test.go`.
- **Docs**: `docs/specs/init-protocol.md`; `docs/memory/wt-cli/init-failure-contract.md` via hydrate.
- **Behavior surface**: exit codes unchanged on all existing paths; the only new behavior is exit 0 + warning where today's outcome is exit 7 + banner (default script, `fab sync` exit 3). No new dependencies. External callers (`hop`, fab-kit operators) keep the `ExitInitFailed` signal for real failures. Cross-repo dependency: fab-kit ≥ the PR #471 release for the skip to activate (older fab-kit: unchanged behavior).
- **CI**: Go — `gofmt` enforced before vet/test (module root `src/`).

## Open Questions

*(none — all decision points were resolved in the /fab-discuss session and the post-#471 design revision; see Assumptions)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Skip keys on `fab sync` exit code `ExitNotManaged = 3` (fab-kit PR #471, released) — not a wt-side marker probe | Discussed — user shipped the fab-side contract specifically to supersede the interim probe; fab stays authoritative, no layout coupling | S:95 R:75 A:90 D:90 |
| 2 | Certain | Skip gates on provenance: built-in default only; explicit `WORKTREE_INIT_SCRIPT="fab sync"` still hard-fails on any non-zero exit incl. 3 | Discussed — agreed principle ("the user never opted into the default; an explicit script failure is theirs to see") | S:95 R:80 A:90 D:90 |
| 3 | Certain | Skip semantics: stderr warning, nil/exit 0, no banner, no open-anyway prompt, Open phase proceeds as on success | Matches the existing graceful-skip convention in init-protocol.md; call sites already treat nil as init-ok | S:80 R:90 A:90 D:85 |
| 4 | Certain | Run-time classification in a single shared helper in `init.go` (both run sites use it); `ResolveInitInvocation`/`InitNotFound` untouched | The decision needs the exit code, which only exists post-run; centralizing mirrors the existing `RenderWarning()` anti-drift pattern | S:70 R:85 A:85 D:75 |
| 5 | Confident | Pre-skip output noise accepted (separator, "Running worktree init...", fab's ERROR line print before the skip warning) | Inherent to post-run detection; more transparent than suppressing; skip warning is the last word | S:65 R:85 A:80 D:70 |
| 6 | Confident | Old fab-kit (exit 1) degrades to today's hard-fail — no version detection, no fallback probe | Strict no-regression; feature activates on fab-kit update; avoids reintroducing the coupling the revision removed | S:70 R:80 A:85 D:75 |
| 7 | Confident | Exact seam shape (signatures vs. options struct on `InitScriptPath` / runner) left to apply | Implementation detail; internal package, no external API; easily adjusted in review | S:65 R:85 A:80 D:60 |
| 8 | Confident | Warning copy: "not a fab-managed repo — skipping init …" + hint (set `WORKTREE_INIT_SCRIPT` or run `fab init`) | Reversible user-facing copy; follows stderr conventions; both escape hatches named | S:55 R:90 A:80 D:65 |

8 assumptions (4 certain, 4 confident, 0 tentative, 0 unresolved).
