# wt-cli: Create Output Phases Contract

> Post-implementation behavior capture for the `wt create` / `wt init` phase-separator output.
> Source change: `260531-pnmi-add-phase-separators`.

This file documents the phase-separator output contract that `wt create` and `wt init` honor. Future changes touching `src/cmd/wt/create.go`, `src/cmd/wt/init.go`, or `src/internal/worktree/{crud.go,errors.go}` should preserve these invariants unless an explicit spec amendment supersedes them.

## Requirements

### `PhaseSeparator` is the sole separator producer

- `src/internal/worktree/errors.go` exposes `func PhaseSeparator(label string) string`, alongside the other output helpers (`WtError`, `PrintError`, `PrintInitFailureBanner`). It is the **single** source of phase-separator strings — call sites MUST NOT reinline the glyph/width/color logic.
- The returned string has **no trailing newline**; callers add it via the `fmt.Fprintln(os.Stderr, ...)` they already use.
- Fixed total **visible** width of `phaseSeparatorWidth = 40` columns (label + glyphs + spaces). It MUST NOT query the terminal size — fixed width keeps output deterministic for tests and matches the single-binary / no-hidden-state posture (Constitution I).
- Visible width is **rune-counted** (`utf8.RuneCountInString(label)`), not byte-counted, so a label with multi-byte runes (e.g. a non-ASCII `WORKTREE_INIT_SCRIPT` path) still renders at the fixed visible width.
- Layout: two leading glyphs, a space, the label, a space, then enough trailing glyphs to fill 40 visible columns. ANSI escapes around the label are NOT counted toward the width. Trailing glyph count is clamped at 0 for labels at or over the budget.
- Color detection uses the **blanked package-level color vars** (`colorEnabled := ColorReset != ""`), reusing the existing `init()` that blanks `ColorBold`/`ColorReset`/etc. when `NO_COLOR` is non-empty. It MUST NOT call `os.Getenv` afresh.
  - Color enabled: unicode rule `── Label ──…` (glyph `─`, U+2500 BOX DRAWINGS LIGHT HORIZONTAL) framing a `ColorBold`-wrapped label.
  - `NO_COLOR` set (color vars blanked): plain-ASCII rule `-- Label --…` (glyph `-`) with **no** ANSI escape sequences.

### `wt create` emits Git, Init, and Open separators on stderr, in order

- `wt create` emits up to three phase separators to **stderr** (never stdout), in order, each immediately preceding the output of its phase:
  1. `PhaseSeparator("Git")` — emitted in `create.go` immediately before the deferred summary block (`Created worktree:` / `Path:` / `Branch:`), joining the existing deferred-summary emission under the rollback handler.
  2. The **Init** separator — emitted by the init runner, NOT by `create.go` (see next requirement).
  3. `PhaseSeparator("Open")` — emitted immediately before the open phase. As of `260602-z4p7`, when the init phase ran, the Open separator + menu render are now preceded by an **unconditional foreground reclaim** (`reclaimTerminalForeground(ttyFd, wtPgid)` in `create.go`, just before the Open separator, gated on the same `term.IsTerminal` check as capture) so the Open phase can never SIGTTOU on a shared-TTY init child that stranded terminal foreground. The separator output contract itself is unchanged — this is a job-control ordering guarantee, not an output change. See `init-failure-contract.md` "Terminal foreground is reclaimed after the init child".
- A separator is emitted **only when its phase produces output** — a separator never precedes a phase that emits nothing:
  - **Git** is emitted on every successful create, because the deferred summary block always prints (it is the Git phase's output).
  - **Init** is emitted only when the init phase actually runs an init command — NOT when init is disabled and NOT on the `*InitNotFound` path.
  - **Open** is emitted only when the open phase runs, gated on `worktreeOpen != "skip"`. `--worktree-open=skip` (and `--non-interactive`, which defaults open to `skip`) suppress it.
- The Git separator MUST stay inside the existing deferred-summary emission, **before** the init-phase `signal.Reset(syscall.SIGINT, syscall.SIGTERM)` call. It MUST NOT introduce new I/O or a prompt inside the tight reinstall window between `git worktree add` returning and that `signal.Reset` (see `init-failure-contract.md` "SIGINT during init").
- Existing summary/warning/prompt substrings are preserved verbatim — in particular `Created worktree:` and `Open in:`.

### Init separator is owned by the runner, labeled with the resolved command

- The Init separator is emitted by the code that owns the `Running worktree init...` line, so it is produced **exactly once** regardless of caller — a single emission point per path:
  - `RunWorktreeSetupWithObserver` in `src/internal/worktree/crud.go` (the `wt create` init step) — `crud.go:157`.
  - `runInitScript` in `src/cmd/wt/init.go` (the standalone `wt init` path) — `init.go:78`.
- The label is `Init (<cmd>)`, where `<cmd>` is the resolved init command as the user would recognize it (e.g. `Init (fab sync)`, or `Init (scripts/setup.sh)` for a file-path invocation). It is derived from the same init-script value resolution uses (`InitScriptPath()` / the resolved `*exec.Cmd`) and is **never hardcoded**.
- On the `*InitNotFound` path, the existing `RenderWarning()` is used unchanged and **no** Init separator is emitted — there is no command to label. The graceful-skip exit-0 behavior is unchanged (see `init-failure-contract.md`).
- The substrings `Running worktree init` and `Worktree init complete` remain present — the separator augments these markers, it does not replace them (preserves existing test assertions).

### STDOUT discipline: separators are stderr-only

- All phase separators are written to **stderr**. None ever appears on stdout.
- `wt create`'s stdout remains **solely** the final worktree-path line (`fmt.Println(wtPath)`), byte-identical to before this change. This preserves the launcher-contract guarantee that stdout is the machine-readable result. No separator, summary, or init output leaks to stdout.
- `wt init` was **realigned** so ALL its init diagnostics go to stderr: the `Running worktree init...` / `Worktree init complete.` banner (now `fmt.Fprintln(os.Stderr, ...)`), the Init separator, AND the init script's own stdout (the init child is wired with `cmd.Stdout = os.Stderr` at `init.go:83`, joining `cmd.Stderr = os.Stderr`). `wt init` has **no machine-readable stdout result** — it is a side-effecting command whose outcome is its exit code — so all of its output is diagnostic.
- The realignment changed only the stream (stdout → stderr) for `wt init`: exit-code behavior (`ExitInitFailed = 7` on script non-zero exit), graceful-skip behavior, and all output text are unchanged. Existing `wt init` tests assert on the combined `r.Stdout + r.Stderr`, so they stay green.

## Design Decisions

### Single helper in `errors.go` as the sole separator producer

`errors.go` already centralizes user-facing output formatting (`WtError`, `PrintInitFailureBanner`) and owns the `NO_COLOR`-blanking `init()`. Co-locating `PhaseSeparator` reuses that mechanism and keeps the canonical glyph/width/color wording in one place — mirroring how `RenderWarning` is the single source for not-found warnings. Inlining rule construction at each call site (`create.go`, `crud.go`, `init.go`) was rejected: it would duplicate the logic and invite drift.

### Init separator owned by the runner, not `create.go`

The runner already prints `Running worktree init...` and already holds the resolved `*exec.Cmd`, so it can label the separator with the resolved command and guarantee exactly one emission for both `wt create` and `wt init`. Emitting from `create.go` was rejected — it would either duplicate the resolution logic or double-label when `wt init` runs the same runner.

### `wt init` realigned to stderr (stream realignment in scope)

The convention captured by this change: **stdout = machine-readable result; stderr = diagnostics** (progress lines, banners, separators, prompts). `wt create` already followed it (stdout = the path line only). `wt init` used stdout for identical init diagnostics, an asymmetry that made separator placement inconsistent. Since `wt init` has no stdout contract (its tests assert on combined streams and no spec pins it to stdout), realigning it to stderr resolves the inconsistency and makes separator placement uniform across both commands. Keeping each command on its own stream was rejected — it enshrines the inconsistency.

### Fixed 40-column width, no terminal-size query

Deterministic output for unit tests; no dependency on terminal/ioctl; consistent with the single-binary, no-hidden-state posture (Constitution I). Dynamic width via terminal detection was rejected — non-deterministic in tests and adds a platform-specific code path for a cosmetic gain. (This mirrors `wt list`'s flag-based, no-`isatty` posture; see `list-status-contract.md`.)

## Cross-references

- Spec doc: none — phase-separator output is per-command diagnostic structure, not part of `docs/specs/cli-surface.md`'s per-subcommand flag surface. This contract lives in memory only.
- Source: `src/internal/worktree/errors.go` (`PhaseSeparator`, `phaseSeparatorWidth`, the `NO_COLOR`-blanking `init()`), `src/internal/worktree/crud.go` (`RunWorktreeSetupWithObserver` Init separator emission), `src/cmd/wt/create.go` (Git + Open separators, stdout path line), `src/cmd/wt/init.go` (`runInitScript` Init separator + stderr realignment).
- Tests: `src/internal/worktree/errors_test.go` (`PhaseSeparator` unit test: label presence, colored ANSI + `─`, NO_COLOR ASCII `-` with no `\033[`, 40-column visible width, no trailing newline); `src/cmd/wt/create_test.go` (`Created worktree:` stderr + one-line-stdout guard); `src/cmd/wt/init_test.go` (combined-stream `Running worktree init` / `Worktree init complete`).
- Constitution: Principle I (Single-Binary CLI, No Hidden State — motivated the fixed width, no terminal query), Principle VI (Interactive by Default, Scriptable on Demand — stdout=machine-result keeps `wt create`'s path line deterministic for launchers/operators).
- Sibling memory: `wt-cli/init-failure-contract.md` — the init runner / resolver / `*InitNotFound` contract this change builds on (the Init separator sits next to the `Running worktree init` line and respects the not-found and reinstall-window invariants). `wt-cli/list-status-contract.md` and `wt-cli/menu-navigation-contract.md` — same post-change invariant-capture pattern for sibling `wt` subcommands.

## Changelog

| Change | Date | Summary |
|--------|------|---------|
| `260531-pnmi-add-phase-separators` | 2026-05-31 | Added `PhaseSeparator(label string) string` to `errors.go` (sole producer: fixed 40-col rune-counted visible width, no trailing newline, unicode `── Label ──` colored / ASCII `-- Label --` NO_COLOR via the existing color-blanking `init()`, bold label). `wt create` emits Git / Init / Open separators on stderr in order (Git always, Init only when a command runs — not on `*InitNotFound`, Open only when `worktreeOpen != "skip"`). Init separator owned by the runner (`crud.go` + `init.go`), labeled `Init (<resolved cmd>)`, single emission point. Realigned `wt init` so all init diagnostics (banner + separator + init-child stdout via `cmd.Stdout = os.Stderr`) go to stderr; `wt create`'s stdout stays solely the worktree-path line. Captured the stdout = machine-result / stderr = diagnostics convention. |
| `260602-z4p7-wt-reclaim-tty-foreground-after-init` | 2026-06-02 | Light touch: the Open-phase separator + menu render is now preceded by an unconditional terminal-foreground reclaim (when the init phase ran and stdin is a TTY), so the Open phase can never SIGTTOU on a shared-TTY init child. Separator output contract unchanged — job-control ordering guarantee only. Full contract in `init-failure-contract.md`. |
