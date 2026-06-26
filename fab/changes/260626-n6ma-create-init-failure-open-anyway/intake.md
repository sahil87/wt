# Intake: wt create — graceful degrade on init-script failure (open-anyway prompt + wt go hint)

**Change**: 260626-n6ma-create-init-failure-open-anyway
**Created**: 2026-06-26

## Origin

> `wt create` should degrade gracefully on init-script failure instead of hard-exiting — prompt the
> user to open the worktree anyway, and always point them at `wt go` to reach the kept worktree.

Mode: one-shot synthesized dispatch (promptless-defer) from a live design conversation. The
conversation settled several load-bearing decisions, which are recorded as Certain/Confident
assumptions below:

- **Exit code stays `ExitInitFailed = 7` on every path** — explicitly decided with the user. The
  user rejected both "exit 0 if open succeeds" and "introduce a new exit code (8)" in favor of
  *always exit 7*.
- **The `wt go` hint goes in the banner itself** (not only the interactive branch), because it
  benefits every path including non-interactive.
- **The open-anyway prompt is interactive-only**, gated by the same TTY / `--non-interactive`
  discipline the rest of `create` uses.

## Why

Today `wt create`'s init phase hard-exits the moment the init script returns non-zero. In
`src/cmd/wt/create.go` (the init-failure block, currently lines 339–354) the code does:

```go
if err := wt.RunWorktreeSetupWithObserver(wtPath, initScript, ctx.RepoRoot, captureInit); err != nil {
    signal.Stop(initSigCh)
    close(initSigCh)
    if reclaimTTY {
        reclaimTerminalForeground(ttyFd, wtPgid)
    }
    rb.Disarm()
    wt.PrintInitFailureBanner(wtPath, finalName, err)
    os.Exit(wt.ExitInitFailed) // <-- hard exit; phase 5 (Open) is never reached
}
```

1. **Problem (pain point).** The worktree itself exists and is git-valid — only a user-supplied
   script failed (the worktree is kept via `rb.Disarm()`). But `os.Exit(7)` fires inline at the
   banner, so control never reaches the Open phase (phase 5). An interactive user who would happily
   open the just-created worktree and fix init by hand is instead dropped back to their shell. This
   is heavier-handed than necessary for a recoverable, kept-worktree situation.

2. **Consequence if unfixed.** The interactive user must manually `cd` into the worktree (or run
   `wt go`) after every init failure, even though `wt create` already had the Open menu one step
   away. There is also no on-screen pointer to `wt go` in the banner today (only `cd … && wt init`
   retry and `wt delete` hints), so the fastest path to *reach* the kept worktree is undiscoverable
   from the failure output.

3. **Why this approach over alternatives.** Falling through to the existing Open phase (rather than
   building a new open path) reuses the load-bearing pre-Open terminal-foreground reclaim that is
   already in place, so the open-anyway menu render cannot SIGTTOU. Adding the `wt go` hint to the
   *banner* (rather than only to the interactive branch) means the non-interactive/CI path also
   gains the discoverability benefit at zero behavior cost. Keeping the exit code at 7 on all paths
   preserves the documented init-failure contract that operators depend on (see Design Decisions).

## What Changes

Two files change: a behavior change in `src/cmd/wt/create.go`, and a one-line additive hint in
`PrintInitFailureBanner` in `src/internal/worktree/errors.go`.

### 1. `PrintInitFailureBanner` gains a `Go:` hint (`src/internal/worktree/errors.go`)

The banner currently emits (in order): status line + `(init output is above)`; `Worktree:`
(absolute path, kept-note); `Retry:   cd '<wtPath>' && wt init`; `Remove:  wt delete '<name>'`.

Add a `Go:  wt go '<name>'` hint line. It points the user at the selection-free navigation command
for the kept worktree.

- Single-quote the name via the existing `shellQuoteSingle` helper (`src/internal/worktree/apps.go`),
  consistent with the sibling `Retry`/`Remove` hints — the banner already single-quotes both path
  and name.
- Use a named banner label constant in the same `const (...)` block as `bannerLabelWorktree` /
  `bannerLabelRetry` / `bannerLabelRemove` (e.g. `bannerLabelGo = "Go:      "`), aligned to the
  existing label column width, so the canonical wording stays confined to this one helper (no
  duplication — anti-pattern per code-quality).
- This hint benefits EVERY caller path (interactive yes, interactive no, and non-interactive), which
  is why it lives in the banner itself, not in the interactive branch.

Placement relative to the existing hints (Retry/Remove) is a presentation detail; placing `Go:`
after `Worktree:` and alongside the other action hints is the natural grouping.

### 2. `wt create` init-failure block degrades gracefully (`src/cmd/wt/create.go`)

Restructure the init-failure block (currently lines 339–354) so it no longer `os.Exit(7)`s inline.

**Non-interactive / piped / CI — EXACT current behavior preserved (no prompt):**
- Print the banner (now including the new `Go:` line) and exit `ExitInitFailed = 7`.
- The new prompt sits behind the same gating the rest of `create` uses: the `nonInteractive` flag
  and the TTY check. Note `reclaimTTY` (already computed as `term.IsTerminal(ttyFd)` at
  `create.go:292`) is the existing interactivity signal in this block; the prompt is shown only when
  interactive (`!nonInteractive` AND stdin is a TTY).

**Interactive — new open-anyway flow:**
1. Acknowledge the error exactly as today: `wt.PrintInitFailureBanner(wtPath, finalName, err)` (init
   output already streamed above).
2. Prompt: `wt.ConfirmYesNo("Continue and open the worktree anyway?")` (`ConfirmYesNo` lives in
   `src/internal/worktree/menu.go`).

   - **Yes** → fall through into the existing Open phase (phase 5) so the user can open the kept
     worktree. The pre-Open terminal-foreground reclaim already runs before the Open separator/menu
     render, so the open-anyway menu render will NOT SIGTTOU.
   - **No** → do not open; show the user how to reach the already-created worktree (the banner's new
     `Go:` line already provides this; the No branch shows no app menu).
3. **Either way the process MUST still exit `ExitInitFailed = 7`** at the end — regardless of the
   open-anyway choice and regardless of whether the open succeeded.

**Implementation note (load-bearing):** do NOT `os.Exit(7)` inline at the banner. Instead set an
"init failed" flag (or otherwise restructure) so the open-anyway *Yes* path falls through to the
existing Open phase, and the function exits 7 at the end on all init-failure paths. The Open phase's
own normal-success exit (which today prints the path line and returns 0) must be overridden to 7 when
the init-failure flag is set, so a *successful open* never downgrades the exit to 0.

### Invariants that MUST be preserved on the new open-anyway branch

These already hold on today's single exit path and must continue to hold on the *Yes* fall-through:

- **Worktree is KEPT**: `rb.Disarm()` (NOT `rb.Execute()`) on init-script non-zero exit. The
  `defer rb.Execute()` still fires on any other failure.
- **SIGINT Option B teardown**: `signal.Stop(initSigCh)` + `close(initSigCh)` must run before the
  open phase on the open-anyway path too (the init-child signal handler must be torn down before the
  Open menu).
- **Terminal-foreground reclaim**: the load-bearing pre-Open reclaim (`reclaimTerminalForeground`,
  gated on `reclaimTTY`) must run before the open phase on the open-anyway path — the banner is a TTY
  write and the menu render is a TTY write; both would SIGTTOU if foreground were stranded by a
  shared-TTY init child.

### Out of scope / unchanged

- **`--reuse` path is exempt.** Its init step is a refresh (warn-but-continue), not a creation gate,
  and does not adopt `ExitInitFailed`. Unchanged.
- **Not-found path is distinct and unchanged.** Init-script-missing (`*InitNotFound`) is non-fatal:
  warning + exit 0. Only "resolver succeeded, process executed, exited non-zero" triggers this flow.
- **`wt init` (standalone)** is not in scope — only `wt create`'s init step changes behavior. (The
  banner hint change is shared via `PrintInitFailureBanner`, but `wt init` does not call it.)

## Affected Memory

- `wt-cli/init-failure-contract.md`: (modify) The kept-worktree + `ExitInitFailed` contract gains
  the interactive open-anyway prompt and the new `wt go` banner hint. Specifically: the
  "`wt create` exits with `ExitInitFailed`" requirement is amended so that exit-7 holds on ALL
  init-failure paths including a successful open-anyway open; the banner requirement gains the `Go:`
  hint line; the SIGINT-teardown + terminal-foreground-reclaim invariants are documented as also
  holding on the open-anyway fall-through.
- `wt-cli/create-output-phases.md`: (modify) The Open phase can now run after an init failure on the
  interactive *Yes* path — previously the Open separator/menu were reachable only on init success.

## Impact

- **Code**:
  - `src/cmd/wt/create.go` — restructure the init-failure block (lines ~339–354): replace inline
    `os.Exit(ExitInitFailed)` with a flag-based fall-through to Open + interactive prompt; ensure the
    function exits 7 on all init-failure paths (incl. successful open).
  - `src/internal/worktree/errors.go` — add the `Go:` hint line + label constant to
    `PrintInitFailureBanner`.
- **APIs / contracts**:
  - `ExitInitFailed = 7` exit-code contract — preserved (the whole point). No new exit code.
  - `PrintInitFailureBanner` signature unchanged (still `(wtPath, name string, err error)`); only its
    output gains a line.
- **Consumers**: operators (fab-kit, `hop`, shell wrappers) that branch on `$?` to detect "worktree
  exists, init didn't complete" — behavior preserved (still exit 7).
- **Tests** (`src/cmd/wt/create_test.go`, and any banner test in
  `src/internal/worktree/errors_test.go`):
  - Banner test gains a `wt go` / `Go:` substring assertion (the contract tests assert on information
    surface, not byte equality).
  - Non-interactive init-failure path: still banner + exit 7, NO prompt.
  - Interactive init-failure path: new prompt; *Yes* falls through to Open; *No* skips open; both exit
    7. Per `code-review.md`, any test exercising the open path MUST be side-effect-free (use a
    non-side-effecting `--worktree-open` target or rely on `runWt`'s env isolation / the
    `WT_TEST_NO_LAUNCH=1` seam — actual window creation is not exercised in the unit suite).
- **Spec**: `docs/specs/init-protocol.md` "Script failure semantics" section is updated to describe
  the interactive open-anyway prompt and the `wt go` hint (and to reaffirm exit 7 on all paths).
  Memory hydrate / spec hydrate handle the doc edits downstream.

## Open Questions

- None blocking. The exact prompt string (`"Continue and open the worktree anyway?"`) and whether the
  *No* branch needs any message beyond the banner's `Go:` line are minor copy decisions resolved
  inline (see Assumptions) and easily tuned via `/fab-clarify`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Process exits `ExitInitFailed = 7` on every init-failure path, including a successful open-anyway open (no exit 0, no new code 8). | Explicitly decided with the user; exit 7 is a documented, depended-on contract (Constitution III; `init-failure-contract.md`). Letting a successful open downgrade to 0 would overload `0` and erase the init-failure signal — a contract break. | S:98 R:88 A:95 D:95 |
| 2 | Certain | The `wt go '<name>'` hint goes in `PrintInitFailureBanner` itself (not only the interactive branch), single-quoted via `shellQuoteSingle`, with a named label constant alongside the existing banner labels. | Explicitly stated: benefits every path incl. non-interactive; mirrors the existing `Retry`/`Remove` hints' single-quoting + label-constant pattern; banner already centralizes wording (no duplication). | S:95 R:90 A:95 D:90 |
| 3 | Certain | The open-anyway prompt is interactive-only, gated by `!nonInteractive` AND the existing TTY check (`reclaimTTY`/`term.IsTerminal`); non-interactive/piped/CI keeps today's exact banner + exit 7 with NO prompt. | Constitution VI (Interactive by Default, Scriptable on Demand); the description pins the gate to the same TTY/`--non-interactive` discipline the rest of `create` uses; `reclaimTTY` is already computed in this block. | S:95 R:85 A:95 D:90 |
| 4 | Certain | On *Yes*, control falls through to the EXISTING Open phase (phase 5) rather than a new open codepath; SIGINT-teardown (`signal.Stop`/`close(initSigCh)`) and the pre-Open terminal-foreground reclaim run before the Open menu on this path too. | Reuses the load-bearing reclaim that already guards the Open menu against SIGTTOU; the description names these invariants as MUST-preserve; `init-failure-contract.md` documents them for the existing success path. | S:92 R:80 A:90 D:90 |
| 5 | Certain | The worktree is KEPT (`rb.Disarm()`, not `rb.Execute()`) on init-script non-zero exit — unchanged; `--reuse` and the not-found path remain exempt/unchanged. | Existing contract restated verbatim in the description; no change requested to rollback, reuse, or not-found semantics. | S:95 R:88 A:95 D:95 |
| 6 | Confident | Implementation uses an "init failed" flag (set in the failure block) rather than inline `os.Exit`, so the *Yes* path falls through to Open and the function exits 7 at the end; the Open phase's normal exit-0/return is overridden to 7 when the flag is set. | The description gives this as the implementation note; it is the natural Go structuring, but the exact mechanism (flag vs. labeled restructure vs. early helper) is an implementation choice the apply agent finalizes. Reversible, low blast radius. | S:80 R:75 A:80 D:70 |
| 7 | Confident | Exact prompt wording is `"Continue and open the worktree anyway?"`, and the *No* branch shows no additional message beyond the banner's `Go:` line (which already shows how to reach the worktree). | The description suggests this exact string ("e.g. …") and says "show how to reach the worktree" — the banner's new `Go:` line satisfies that, so no extra No-branch copy is assumed. Reversible copy decision via `/fab-clarify`; a clear front-runner default exists. | S:60 R:80 A:65 D:45 |

7 assumptions (5 certain, 2 confident, 0 tentative, 0 unresolved).
