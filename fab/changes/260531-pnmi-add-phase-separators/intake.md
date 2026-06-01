# Intake: Phase Separators in Worktree Creation Output

**Change**: 260531-pnmi-add-phase-separators
**Created**: 2026-05-31
**Status**: Draft

## Origin

> Add phase separators to `wt create` and `wt init` output so failures and oddities are
> attributable to a specific phase. The user observed (from a `wt create` screenshot) that when
> something fails during worktree creation, it is not clear *what* broke — the git work, the
> init step (`fab sync`), and the open step all stream together with no visual boundary.

**Interaction mode**: Conversational. The change emerged from a `/fab-discuss` session in which
the user shared a screenshot of `wt create` output, asked how the per-worktree init mechanism
works, and — after we traced it end to end — asked to add logging separators per phase.

**Key decisions reached in discussion** (encoded as Assumptions below):

1. **Scope** — separate *all three* phases (Git, Init, Open), not just the init boundary. User
   selected "All phases" when offered the narrower "Init boundary only" alternative.
2. **Stream discipline** — separators go to **stderr only**. The final worktree-path line on
   **stdout** is a stability contract consumed by external callers (`hop`, fab-kit operators) per
   `docs/specs/launcher-contract.md`; it MUST NOT be polluted. User selected "stderr only,
   NO_COLOR-aware" over "don't care about streams".
3. **NO_COLOR / non-TTY form** — strip ANSI color but **keep** a plain-ASCII labeled rule (e.g.
   `-- Git ------`). Rationale: the primary debugging use case is reading a *captured* failure
   log (piped to a file / operator), exactly when color is absent — so dropping separators
   entirely would remove the labels when they are most useful. User selected "Plain ASCII rule"
   over "Drop entirely".
4. **Init label content** — the Init separator label echoes the **resolved** init command's first
   token, e.g. `Init (fab sync)` or `Init (scripts/setup.sh)`, derived from
   `WORKTREE_INIT_SCRIPT` / the default `"fab sync"` — not hardcoded. User selected "Show
   resolved command" over "Generic 'Init'".

## Why

**Problem (pain point).** `wt create` runs three distinct phases in sequence — git worktree
operations, the init step (which shells out to `fab sync` or a user-supplied
`WORKTREE_INIT_SCRIPT`), and the open step — and streams their output to the terminal with no
visual demarcation. The init step in particular emits a wall of third-party output
(`.envrc: OK`, `Claude code: 29/29`, `Skipping opencode: not found in PATH`, hook-entry notices,
etc.) that is produced entirely by `fab sync`, not by `wt`. When something looks wrong or a
phase fails, the user cannot tell at a glance **which phase** owns a given line, nor where `wt`'s
own output ends and the external init script's output begins.

**Consequence of not fixing.** Debugging a failed or partially-successful worktree creation
requires re-deriving the phase structure from memory or from the source. This is most painful
exactly when the output has been captured to a log (CI, operator transcript) — the moment the
phase boundaries matter most.

**Why this approach.** Labeled phase separators are the minimal, non-invasive intervention: they
add visual structure without changing any existing message text, exit codes, or the stdout
contract. Existing hard-failure paths are *already* well-labeled (the init-failure banner with
retry/remove hints; the `Error:`/`Why:`/`Fix:` structured errors), so the gap is specifically the
**streaming / attribution** problem — separators address that directly. Alternatives considered
and rejected during discussion: (a) wrapping only the init boundary — rejected as too narrow
since git and open output also blur together; (b) ignoring stream discipline — rejected because
it risks the launcher-contract stdout parse.

## What Changes

A new output helper produces labeled phase separators, and `wt create` / `wt init` emit them
around their three phases. **No existing message strings, exit codes, rollback behavior, or
stdout output change.** All separators are additive lines on **stderr**.

### New helper: `PhaseSeparator(label string) string`

Added to `src/internal/worktree/errors.go`, alongside the existing output helpers (`WtError`,
`PrintError`, `PrintInitFailureBanner`) so the canonical wording and color handling live in one
place.

- **Colored (TTY, color enabled)** form: a unicode rule framing the label, e.g.
  `── Git ──────────────────────────────`. The rule MAY use the existing dim/bold treatment
  consistent with other helpers.
- **Plain (NO_COLOR set)** form: an ASCII rule with the same label, e.g.
  `-- Git ------------------------------`. The label is retained; only color/unicode is dropped.
- **Width**: a fixed modest width (~40 columns). The helper does **not** query terminal size —
  this keeps output deterministic for tests and dependency-free, consistent with the project's
  single-binary / no-hidden-state posture. (Tradeoff: separators do not span ultra-wide
  terminals; accepted for determinism.)
- **NO_COLOR mechanism**: reuse the existing package-level color vars that `errors.go`'s `init()`
  already blanks when `NO_COLOR` is set. When color is blank, the helper SHALL emit the
  plain-ASCII form.

Illustrative signature and behavior:

```go
// PhaseSeparator returns a labeled rule line (no trailing newline) for stderr.
// Colored form: "── Git ──…"; NO_COLOR form: "-- Git --…". Fixed ~40-col width.
func PhaseSeparator(label string) string
```

### `wt create` (`src/cmd/wt/create.go`) — Git and Open separators

- Emit `PhaseSeparator("Git")` to stderr immediately **before** the deferred summary block
  (`Created worktree:` / `Path:` / `Branch:`, currently around line 264). This is the first point
  at which there is git-phase output to label. It MUST be emitted *after* the signal-handler swap
  considerations are respected — i.e., it joins the existing "deferred summary" emission that
  already runs under the rollback handler, and introduces no new I/O inside the tight
  reinstall-window between git-add returning and `signal.Reset` (per the spec comment at
  create.go §"Reinstall-window contract").
- Emit `PhaseSeparator("Open")` to stderr immediately **before** the open phase (currently around
  line 308, before the `worktreeOpen` branch). The existing `Open in:` menu prompt follows.
- The Init separator is **not** emitted by `create.go` — it is owned by the init runner (below) to
  avoid double-labeling, since the runner is what knows the resolved command.

### Init separator — owned by the runner

The init separator is emitted by the code that already owns the `Running worktree init...` line,
so it is printed exactly once regardless of caller:

- `RunWorktreeSetupWithObserver` (`src/internal/worktree/crud.go`) — used by `wt create`. It
  already calls `ResolveInitInvocation` and thus knows the resolved `*exec.Cmd`. Emit
  `PhaseSeparator("Init (<token>)")` to stderr where `<token>` is the resolved init command's
  first token (e.g. `fab sync` → `fab sync`, or a file path → the path). Replace/precede the
  existing `Running worktree init...` line; the substring `Running worktree init` SHOULD remain
  present for backward-compatible test assertions (see Impact).
- `runInitScript` (`src/cmd/wt/init.go`) — the standalone `wt init` path. Apply the same Init
  separator for consistency. **This path is also realigned to the stdout/stderr convention**:
  today it prints its banner lines via `fmt.Println` (stdout) and wires `cmd.Stdout = os.Stdout`
  for the init script's output. Since `wt init` has no machine-readable result (it is a
  side-effecting command), these are diagnostics and SHALL move to **stderr** — matching
  `wt create`'s init runner (`crud.go`, which already uses stderr). The existing
  `Running worktree init` / `Worktree init complete` substrings are preserved (only the stream
  changes), so the `wt init` tests — which assert on combined `stdout + stderr` — stay green.
  This realignment is in scope for this change (confirmed during clarify); it both makes the
  separator placement uniform across both commands and fixes a pre-existing inconsistency.

The label's `<token>` is derived from the same value `InitScriptPath()` returns — for the
not-found case (command not on PATH / file missing), the existing `RenderWarning()` path is
unchanged and no Init separator/label is required (resolution failed before a command exists).

### Resulting output shape (illustrative, colored TTY)

```
── Git ──────────────────────────────
Created worktree: humid-dolphin
Path: /home/.../humid-dolphin
Branch: humid-dolphin

── Init (fab sync) ───────────────────
  .envrc: OK
  Claude code: 29/29
  Skipping opencode: not found in PATH
Worktree init complete.

── Open ──────────────────────────────
Open in: tmux window
```

(Under `NO_COLOR`, the `──` rules become `--` rules; labels are identical.)

## Affected Memory

- `wt-cli/create-output-phases`: (new) Behavior contract for the phased stderr output of
  `wt create` / `wt init` — the three phase labels (Git, Init, Open), stderr-only placement, the
  stdout final-path contract that separators MUST NOT violate, the NO_COLOR plain-ASCII form, and
  the resolved-command Init label. Created at hydrate. (This sits alongside the existing
  `wt-cli` contract files: `init-failure-contract.md`, `list-status-contract.md`,
  `menu-navigation-contract.md`.)

## Impact

**Code:**

- `src/internal/worktree/errors.go` — new `PhaseSeparator` helper; reuses existing color vars.
- `src/cmd/wt/create.go` — two `PhaseSeparator` calls (Git, Open) on stderr in the existing
  deferred-summary and open-phase regions. No change to signal handling, rollback, or stdout.
- `src/internal/worktree/crud.go` (`RunWorktreeSetupWithObserver`) — Init separator with resolved
  token; keep the `Running worktree init` substring.
- `src/cmd/wt/init.go` (`runInitScript`) — matching Init separator; **realign banner +
  `cmd.Stdout`/`cmd.Stderr` wiring from stdout → stderr** (diagnostics belong on stderr); keep
  existing `Running worktree init` / `Worktree init complete` substrings (tests assert on
  combined streams, so they stay green).

**Specs:**

- `docs/specs/init-protocol.md` — add a "Phase separators" subsection documenting the Init label
  derivation and stderr placement.
- `docs/specs/cli-surface.md` — if it documents `wt create` output shape, add a note about the
  three phase separators and stderr-only discipline.
- `docs/specs/launcher-contract.md` — no normative change, but the new behavior must be checked
  against it (stdout final-path line unaffected). Reference only.

**Tests (Constitution §IV — Test What the User Sees; Test Integrity):**

- Existing assertions use `assertContains` on substrings (`"Created worktree:"`,
  `"Running worktree init"`, `"Worktree init complete"`, `"Open in:"`) and do **not** pin line
  boundaries or leading characters — confirmed via grep. Preserving these substrings keeps the
  existing suite green.
- New unit test for `PhaseSeparator`: asserts the label appears, asserts colored vs. NO_COLOR
  form (ANSI present/absent; `──` vs `--`), asserts fixed width and no trailing newline surprises.
- New integration/cmd assertions: the three phase labels appear on **stderr** (not stdout) for a
  real `wt create`; under `NO_COLOR` the plain-ASCII form appears; the stdout output remains the
  bare worktree path line (launcher-contract guard).

**External callers / dependencies:** none broken. `hop` and fab-kit operators parse the stdout
path line, which is untouched. No new third-party dependencies (uses stdlib `strings` for the
rule, consistent with `list.go`).

## Open Questions

_(Both resolved during clarify — see ## Clarifications.)_

- ~~Should `wt init`'s separators match `wt create`'s stream?~~ **Resolved**: realign `wt init`'s
  diagnostics (banner + script output) from stdout → stderr, so both commands are uniform and the
  pre-existing convention violation is fixed (assumption #8).
- ~~Exact rule glyph/width/shade?~~ **Resolved**: `──` unicode (colored TTY) / `--` ASCII
  (NO_COLOR), fixed ~40 cols, bold label (assumption #9).

## Clarifications

### Session 2026-05-31

| # | Question | Resolution |
|---|----------|------------|
| 8 | `wt create`'s init runner uses stderr but `wt init` uses stdout — how should the Init separator handle the split? | After discussing the stdout/stderr convention (stdout = machine result; stderr = diagnostics), **align `wt init` to stderr**. `wt init` has no machine-readable result, so its banner + script output are diagnostics and move to stderr — uniform with `wt create`, and fixes a latent inconsistency. In scope for this change. |
| 9 | Exact separator rule glyph / width / shade? | `── Label ──` unicode on colored TTY; `-- Label --` ASCII under NO_COLOR; fixed ~40-col width; label rendered bold (`ColorBold`). Confirmed the intake's default proposal over `===` and blank-line variants. |

### Session 2026-05-31 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 7 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = all three phases (Git, Init, Open), not just the init boundary | Discussed — user explicitly chose "All phases" over the narrower "Init boundary only" option | S:98 R:80 A:90 D:95 |
| 2 | Certain | Separators emitted on stderr only; stdout final-path line untouched | Discussed — user chose "stderr only, NO_COLOR-aware"; reinforced by `launcher-contract.md` stdout stability guarantee | S:98 R:70 A:95 D:95 |
| 3 | Certain | NO_COLOR/non-TTY form keeps a plain-ASCII labeled rule (`-- Git --`), does not drop separators | Discussed — user chose "Plain ASCII rule" over "Drop entirely"; rationale = captured-log debugging is the primary use case | S:97 R:85 A:90 D:95 |
| 4 | Certain | Init separator label echoes the resolved init command's first token (`Init (fab sync)` / `Init (<path>)`) | Discussed — user chose "Show resolved command" over generic "Init" | S:96 R:85 A:88 D:92 |
| 5 | Certain | New `PhaseSeparator(label)` helper lives in errors.go, reuses existing NO_COLOR color vars, fixed ~40-col width, no terminal-size query | Clarified — user confirmed (bulk). Strong codebase signal: errors.go already centralizes output helpers + NO_COLOR blanking; fixed width keeps tests deterministic and matches single-binary posture | S:95 R:80 A:90 D:75 |
| 6 | Certain | Init separator owned by the runner (crud.go / init.go), not create.go, to avoid double-labeling | Clarified — user confirmed (bulk). The runner already owns `Running worktree init...` and the resolved cmd; single emission point prevents drift (mirrors the existing RenderWarning single-source pattern) | S:95 R:75 A:88 D:80 |
| 7 | Certain | Existing substring assertions (`Created worktree:`, `Running worktree init`, etc.) preserved; new tests added for separators | Clarified — user confirmed (bulk). Grep confirmed assertions are substring-based, not boundary-pinned; Constitution §IV requires tests-verify-spec | S:95 R:60 A:90 D:80 |
| 8 | Certain | Align `wt init` to stderr: move its init banner + script output from stdout → stderr, matching `wt create`. Both commands stream init diagnostics on stderr; separator placement is uniform | Clarified — user confirmed after discussion of the stdout/stderr convention (stdout = machine result, stderr = diagnostics). `wt init` has no machine-readable result, so its diagnostics belong on stderr; this also fixes a latent inconsistency. Existing `wt init` tests assert on combined stdout+stderr (init_test.go), so they stay green | S:95 R:65 A:85 D:90 |
| 9 | Certain | Rule glyph = `──` (unicode) on colored TTY / `--` (ASCII) under NO_COLOR; fixed ~40-col width; label rendered with existing `ColorBold` | Clarified — user confirmed the intake's default proposal over `===` and blank-line variants | S:95 R:90 A:70 D:90 |

9 assumptions (9 certain, 0 confident, 0 tentative, 0 unresolved).
