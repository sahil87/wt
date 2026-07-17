## Conformance report — audited against shll v0.0.23

Standards were re-enumerated at apply time via `shll standards` + `shll standards <name>` (no `shll update` needed; `shll standards` exited 0). Runtime list matched the intake snapshot: **`principles`, `help-dump`, `readme-extraction`, `skill`**. The audited binary is `wt` built by `just build` (version `v0.0.24-2-g97b9f0e`, stamped from `git describe`). shll version row: `shll v0.0.23`.

Sections follow `shll standards` list order. Every gap carries exactly one disposition: *fixed in this change* or *deferred to [<backlog-id>]*.

### principles

Assessed per-command against actual behavior (`create`, `list`, `open`, `go`, `delete`, `init`, `shell-init`, `update`, root — non-TTY paths probed empirically).

- **№1 Non-interactive by default (MUST)**: gap — the interactive selection menus (`wt open` main-repo menu, `wt go` no-name, `wt delete` no-name) in a non-TTY reached EOF and surfaced a bare `reading input: EOF` (exit 1) — no hang, but not a refusal naming the escape. **fixed in this change**: the shared fallback-menu path (`internal/worktree/menu.go` `runFallbackMenu`) now returns a structured `Error:`/`Why:`/`Fix:` refusal stating the menu cannot run without a TTY and naming the escape (pass a worktree name, or `--non-interactive` where supported). `create`/`delete`/`go`/`list` already carry `--non-interactive`; `wt go --non-interactive` and `wt delete --non-interactive` already refuse cleanly. (The toolkit uses `--non-interactive`, not `--yes`; the principle is satisfied by any flag that makes the command non-interactive.)
- **№2 stdout is data, stderr is diagnostics (MUST)**: PASS for the machine-contract commands, with one deferred gap. `wt create` (path line on stdout; summary/separators/banner/prompts on stderr), `wt go` (resolved path on stdout, arrow confirmation on stderr), `wt init` (all diagnostics on stderr, no stdout contract), and `wt list` (`--json` for programmatic consumption; default table is a read command's data) all honor the split. Machine format stability is pinned by the help-dump `schema_version` evolution rule. gap — `wt delete`'s **non-warning** human copy (Worktree/Branch/Path, `Removing worktree...`, `Deleted worktree:`, `Cancelled.`, branch-cleanup lines, multi-delete summary, `No worktrees found.`) is written to stdout via `fmt.Printf`/`fmt.Println`; only the two pre-menu warnings were realigned to stderr (260622-log5). `wt delete` has no stdout machine contract, so nothing programmatic breaks today. **deferred to [ohwb]** — command-wide (~20 call-site) realignment, beyond the audit's small-additive boundary.
- **№3 Help is a published contract (MUST)**: PASS. Layered help (root `Long:` + per-command `Long:`/usage; README + `docs/site/workflows.md` carry examples). The hidden `help-dump` subcommand emits the JSON tree by walking the Cobra command tree (`internal/worktree/helpdump.go` `BuildHelpDump`), never by parsing `-h`; see the help-dump section below for the mechanical receipts. Pinned by `TestHelpDump_EmitsValidEnvelope`.
- **№4 Fail fast with actionable errors (MUST)**: PASS (with the №1 fix). Errors route through `ExitWithError(what, why, fix)` / `WtError` (`internal/worktree/errors.go`) across all commands, with typed exit codes from `errors.go` (`ExitSuccess 0`, `ExitGeneralError 1`, `ExitInvalidArgs 2`, `ExitGitError 3`, `ExitRetryExhausted 4`, `ExitByobuTabError 5`, `ExitTmuxWindowError 6`, `ExitInitFailed 7`) — `0`/`1`/`2` convention honored for success/operational/usage. The one unactionable path (non-TTY menu `reading input: EOF`) was **fixed in this change** (see №1).
- **№5 Visible mutation boundaries (MUST)**: gap. Read-vs-write is clear from command names/help (`list`/`open`/`go` read; `create`/`delete`/`init` write); destructive `wt delete` requires explicit consent (TTY confirmation, satisfiable by `--non-interactive`) per №1. gap — no destructive path supports `--dry-run` (a preview sharing the live code path). **deferred to [p5m9]** — a new flag plus a preview code path threaded through the deletion flow is restructuring-sized.
- **№6 Stateless, therefore retry-safe (MUST)**: PASS. State is re-derived from git at request time (`git worktree list --porcelain`, `git rev-parse`, no state files or global config writes — Constitution I). `wt create` arms a rollback (`internal/worktree/rollback.go`) that removes a partially-created worktree on failure/SIGINT; re-running after a partial failure converges. `wt delete` continues-on-error across multiple targets and re-derives the worktree set each run.
- **№7 Compose, don't reinvent (MUST)**: PASS. `wt` shells out to `git` and to `brew` (via `internal/update`) rather than reimplementing them; `wt open` is the toolkit's canonical launcher that peers (`hop`) delegate to, and `wt list --json` is the composition surface `hop ls --trees` consumes. `wt update` probes the callee's advertised flag (`--no-brew-update`/`--skip-brew-update`) rather than assuming it.
- **№8 Graceful degradation (MUST)**: PASS. Missing apps are omitted, not fatal (`BuildAvailableApps` detects; `handleAppMenu` prints `No supported applications detected.` and exits 0); color is `NO_COLOR`-gated (`errors.go` `init()` blanks the color vars) and box-drawing falls back to ASCII in `PhaseSeparator`; `wt init`'s built-in `fab sync` default gracefully skips (exit 0) in a non-fab repo; `wt update` reports `brew not found` as a single actionable line.
- **№9 Bounded, high-signal output (MUST)**: PASS. `wt list --status` caps its worker pool (`maxListConcurrency = 8`) and the default path runs a single git subprocess; `wt delete`'s unpushed-commit preview is explicitly capped (`... and N more`). No unbounded dump surface. (No `--quiet` flag exists; the principle's `--quiet` clause is conditional — "what survives `--quiet` where present" — and no surface is verbose enough to require one, so its absence is not a violation.)
- **№10 Agent-discoverable documentation (SHOULD)**: PASS for the two implemented halves; one half deferred. README + `docs/site/` follow the readme-extraction standard (see that section — one URL fixed, rest PASS) and are pulled/rendered by shll.ai. The `<tool> skill` bundle half is not yet adopted (see the skill section). №10 is a SHOULD, and per the skill standard's own Adoption note a tool without `skill` is "not yet in violation."

### help-dump

**PASS** — the standard's "Verifying conformance" checklist executed verbatim against the built binary (`v0.0.24-2-g97b9f0e`):

- `wt help-dump` exits **0**, writes valid JSON to **stdout only**, **stderr empty** (0 bytes). ✓
- Envelope keys are exactly `{root, schema_version, tool, version}` — **no `captured_at`**. ✓
- `tool = "wt"`, `schema_version = 1` (integer), `version = v0.0.24-2-g97b9f0e` (reflects the built binary, not a literal). ✓
- Tree drops `completion`, `help`, and all hidden nodes (incl. `help-dump` itself); visible subcommands are exactly `create, delete, go, init, list, open, shell-init, update` (8). ✓
- `wt -h` does not list `help-dump` (declared `Hidden: true`). ✓
- Minimal pinning test present: `TestHelpDump_EmitsValidEnvelope` (`src/cmd/wt/help_dump_test.go`) asserts exit 0 + valid JSON + `tool`/`schema_version` + `captured_at` absence + filter rules. ✓

No fix required. Re-verified after this change's fixes — the command tree is unchanged (R2 edits only README prose; R4 edits only an error string), so the dump is byte-stable.

### readme-extraction

One gap, fixed in this change; the rest PASS.

- gap — the command-reference link in `README.md` was `https://shll.ai/tools/wt/commands/`, but rule 8 specifies `https://shll.ai/<tool>/commands/` = `https://shll.ai/wt/commands/`. **fixed in this change** (`README.md` line 85 → `https://shll.ai/wt/commands/`).
- PASS — README top order: `#` H1 → exact toolkit blockquote → badges → prose tagline.
- PASS — tail rule: no `Contributing`/`Development`/`Building`/`License`/`Acknowledgements` heading; the whole README is intentionally site-worthy.
- PASS — relative targets: README's only relative links are `docs/site/install.md` / `docs/site/workflows.md` (the auto-rewritten README→docs/site form); `docs/site/**` internal links are all between-page `./install.md` / `./workflows.md` (closure holds, no `..` escape); external links absolute.
- PASS — no relative images anywhere (the three badges are absolute `https://…`); no `#gh-*-mode-only` fragments; no ```` ```mermaid ```` fences.
- PASS — no `docs/site/` page named `overview`, `readme`, or `commands` (tree is `install.md`, `workflows.md`).
- PASS — README cross-links its `docs/site/` pages and (post-fix) the absolute command-reference URL.

Re-ran the grep checklist after the edit: the URL is the only change and no new violation was introduced.

### skill

**Deferred, not yet adopted** (phased per-repo adoption; no seven-repo flag-day). `wt` has no `skill` subcommand (`wt skill` → `unknown command`) and no canonical `docs/site/skill.md`. Per the standard's own Adoption section ("No tool ships `skill` today") and principle №10 being a SHOULD, this is not a current violation. Tracked as **[v7xy]** for eventual adoption (a hidden-free `wt skill` printing a static ≤150-line bundle byte-identical to a new `docs/site/skill.md`, embedded via a sync + drift-guard).

---

**Summary** — fixed in this change: readme-extraction command-reference URL (rule 8); principle №1/№4 non-TTY menu actionable refusal. Deferred: skill adoption **[v7xy]**; principle №2 `wt delete` stdout→stderr realignment **[ohwb]**; principle №5 `wt delete --dry-run` **[p5m9]**. help-dump and the rest of readme-extraction and principles №3/№6/№7/№8/№9/№10 PASS. Tests green; command tree unchanged (help-dump checklist re-verified).
