# Intake: Open-List Locus Registry

**Change**: 260902-ps3l-open-list-locus-registry
**Created**: 2026-09-02

## Origin

> System backlog item `[i2ap]` (idea --system, 2026-09-02): "wt: agent-driven tab opening is
> undiscoverable — wt open --list omits the tmux-window/byobu launchers (returns empty inside
> $TMUX) and the wt skill bundle never names valid --app ids (e.g. -a 'tmux window'); an agent
> following the bundle's discovery path concludes nothing is openable. Fix --list detection +
> teach app ids in the skill bundle (shll/wt repo)"

Conversational mode: the design was worked out in a `/fab-discuss` session before this intake.
Key decisions from that discussion (user-confirmed):

1. User asked for the greenfield shape assuming run-kit (the current `--list --json` consumer)
   could be rewritten. Agreed shape: a complete machine registry with two orthogonal fields —
   `kind` (what the entry is) and `locus` (where its effect lands) — with all filtering pushed
   to consumers.
2. User then narrowed it: **"Let json be expanded. Can --list be kept simple only - like right
   now"** — the completeness invariant lives on `--list --json` only; plain `--list` (human
   table) stays exactly today's simple host-app table.
3. A visual artifact showing example outputs in both environments (headless tmux host, macOS
   laptop), consumer jq filters, and the resulting skill-bundle paragraph was reviewed by the
   user and refreshed to reflect decision 2.

## Why

**The pain point.** `wt open --list` bakes one consumer's policy into the registry data itself.
The current filter (`ListableApps` keeps only rows with non-empty `Kind`) was designed for
run-kit's `GET /api/open-apps` browser dropdown, where session-bound launchers are inverted
signals. But a second consumer exists that the design never considered: **an agent running on
the host itself, inside the tmux session**. For that agent the multiplexer launchers are exactly
the right signal — yet on a headless Linux server (no GUI editors/terminals/file managers
installed), `wt open --list --json` prints `[]` and the human form prints "No launchable
applications detected." Meanwhile `wt open <wt> -a tmux_window` works perfectly.

**The consequence of not fixing.** The skill bundle (`docs/site/skill.md`) points agents at
`--list` as the app registry and never names a single valid `-a` id, so an agent following the
documented discovery path concludes nothing is openable and gives up — backlog `[i2ap]` is a
real observed failure of this path.

**Why this approach.** The root cause is that "launchability" is consumer-relative, and a single
`kind` field cannot express it: `tmux_window` and `ghostty_linux` are the same *kind* of thing
("a shell at this path") but land in different places. Adding a `locus` field states the actual
requirement each consumer has, and moving filtering to consumers makes the registry honest —
the machine invariant becomes "every emitted id round-trips through `-a`, and every id `-a`
accepts here is emitted". Rejected alternatives (from the discussion): a `--all` flag (extra
surface; user chose the expanded-JSON shape instead); skill-bundle-only fix (leaves `--list
--json` a lying registry); including action rows in the *human* table too (user explicitly kept
plain `--list` simple — humans already have the full interactive menu).

## What Changes

### 1. `AppInfo` gains `Locus`; `Kind` covers every catalog row

`src/internal/worktree/apps.go`:

- New field `Locus string` on `AppInfo`, with named constants:
  `LocusGUI = "gui"` (window on the host display), `LocusSession = "session"` (effect inside the
  invoking tmux/byobu server), `LocusCaller = "caller"` (cooperates with the invoking shell via
  `WT_CD_FILE`/stdout), `LocusHost = "host"` (mutates host state observable only locally, e.g.
  clipboard).
- New `Kind` constants for the former action rows: `AppKindMultiplexer = "multiplexer"`,
  `AppKindShell = "shell"`, `AppKindClipboard = "clipboard"`.
- Every append site in `BuildAvailableApps()` populates both fields per this fixed mapping:

| Cmd key | Kind | Locus |
|---------|------|-------|
| `code`, `cursor` | `editor` | `gui` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` | `gui` |
| `finder`, `nautilus`, `dolphin` | `file-manager` | `gui` |
| `tmux_window`, `tmux_session`, `byobu_tab` | `multiplexer` | `session` |
| `open_here` | `shell` | `caller` |
| `copy_macos`, `copy_linux` | `clipboard` | `host` |

- `ListableApps` (the human-table filter) changes its predicate from "non-empty `Kind`" to
  `Locus == LocusGUI` — behavior-identical row set (host applications only), required because
  every row now carries a non-empty `Kind`. Name, order-preservation, and non-nil-return
  contract unchanged.
- Detection gating is unchanged: `tmux_window`/`tmux_session` still appear only when
  `IsTmuxSession()`, `byobu_tab` only when `IsByobuSession()`, `copy_*` only when the clipboard
  binary exists. The interactive menu, `ResolveApp`, `DetectDefaultApp`, `SaveLastApp`, and
  `OpenInApp` are untouched.

### 2. `wt open --list --json` becomes the complete registry

`src/cmd/wt/open.go`:

- The JSON emitter switches from the `ListableApps` filtered set to the **full**
  `BuildAvailableApps()` catalog, in detection order. Record shape:

```json
[
  { "id": "open_here",   "label": "Open here",   "kind": "shell",       "locus": "caller" },
  { "id": "tmux_window", "label": "tmux window", "kind": "multiplexer", "locus": "session", "default": true },
  { "id": "tmux_session","label": "tmux session","kind": "multiplexer", "locus": "session" }
]
```

- `id`, `label`, `kind`, `locus` are present on every record (no `omitempty` on those four).
- `"default": true` is a new marker on the row `DetectDefaultApp()` selects (what `-a default`
  resolves to); emitted only when true (`omitempty` bool), absent from all other rows and
  absent entirely when `DetectDefaultApp` returns -1. JSON only — the human table does not show it.
- **Machine invariant** (the load-bearing change): every emitted `id` is resolvable via
  `wt open <target> -a <id>` in this environment, and every id `-a` resolves here is emitted.
- Encoding unchanged: `json.MarshalIndent(…, "", "  ")` + trailing newline; slice initialized
  non-nil (`open_here` is unconditionally present, so the array is never empty in practice, but
  the `[]`-not-`null` guarantee stays).
- Flag exclusivity rules unchanged (`--list`+positional / `--list`+`--app` / `--list`+`--select` /
  `--json` without `--list` → `ExitInvalidArgs`).

### 3. Plain `--list` (human) stays simple; only the empty-case copy improves

- The human table keeps today's exact shape: `Id` / `Label` / `Kind` columns over the
  `ListableApps` (host-app) set. No `Locus` column, no new rows.
- The zero-host-apps message changes from `No launchable applications detected.` (the lie that
  caused `[i2ap]`) to:
  `No host applications detected. {N} other target(s) available — see 'wt open --list --json' or the interactive menu.`
  where `{N}` = full catalog size minus host-app rows (≥ 1 always, since `open_here` is
  unconditional). Exit 0 unchanged.

### 4. Skill bundle teaches the id vocabulary

`docs/site/skill.md` (canonical) + `src/cmd/wt/skill.md` (committed embed copy, refreshed via
`just sync-skill` / `scripts/sync-skill.sh`): rewrite the `open` capability line to:

```
- `open [name|path]` — launch a worktree in a detected target. `--list` prints
  the host-app table; `--list --json` prints the FULL registry: every emitted
  `id` is a valid `-a` value here and now (`kind` = what it is, `locus` = where
  the effect lands: gui / session / caller / host). Inside tmux/byobu the
  multiplexer targets (`tmux_window`, `tmux_session`, `byobu_tab`) are the
  usual choice on a headless host: `wt open <wt> -a tmux_window`. The row
  marked `default: true` is what `-a default` resolves to.
```

Stay within the ≤150-line bundle budget (`TestSkill_LineBudget`) and keep the drift guard green
(`TestSkill_EmbedMatchesCanonical`).

### 5. Spec + doc updates

- `docs/specs/launcher-contract.md` §2: update the `--list --json` record shape (4 always-present
  keys + optional `default`), the full-catalog semantics, and the round-trip invariant.
- `docs/specs/cli-surface.md`: update the `wt open` `--list`/`--json` rows to note the
  human/machine split.

### Non-Goals

- **No run-kit change in this repo.** run-kit (shll repo) must add a `locus == "gui"` filter to
  `GET /api/open-apps` and deploy **before** this `wt` release ships — the filter is compatible
  with both old and new output. This coordination is recorded in the PR body and memory, not
  implemented here.
- No changes to `-a` resolution, its error messages, the interactive menu, `DetectDefaultApp`
  logic, or `OpenInApp` launch behavior. (The structured what/why/fix error shown in the
  discussion artifact was illustrative only.)
- No new filter flags on the CLI — consumers filter the JSON.

## Affected Memory

- `wt-cli/open-list-contract`: (modify) — rewrite the contract: JSON = complete locus-aware
  registry with round-trip invariant and optional `default` marker; human table = unchanged
  host-app view with the new empty-case copy; `ListableApps` predicate now `Locus == LocusGUI`;
  amend the "action rows are filtered" and "minimal record / no omitempty" design decisions
  (superseded for the JSON form; `default` is deliberately omitempty).

## Impact

- `src/internal/worktree/apps.go` — `AppInfo.Locus`, new `Kind`/`Locus` constants, append-site
  population, `ListableApps` predicate; `apps_test.go` — kind/locus classification tables,
  `ListableApps` filter tests.
- `src/cmd/wt/open.go` — `openAppRecord` (+`locus`, +optional `default`), `printOpenListJSON`
  source set (full catalog), human empty-case message; `open_test.go` — JSON shape/order/round-trip
  tests, empty-case copy test, id round-trip under `WT_TEST_NO_LAUNCH=1`.
- `docs/site/skill.md` + `src/cmd/wt/skill.md` (sync) + line-budget/drift-guard tests already in CI.
- `docs/specs/launcher-contract.md`, `docs/specs/cli-surface.md`.
- CI: `gofmt -l` gate first, then `go vet` / `go test ./...` (module root `src/`).
- External: run-kit's `/api/open-apps` sees 1–3 extra rows + 1–2 extra keys until its filter
  deploys — coordination note only.

## Open Questions

- (none — design decisions were resolved in the preceding discussion)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Expanded registry on `--list --json` only; plain `--list` stays today's simple host-app table | Discussed — user decided explicitly ("Let json be expanded. Can --list be kept simple only - like right now") | S:95 R:70 A:95 D:95 |
| 2 | Certain | `ListableApps` predicate becomes `Locus == LocusGUI` (behavior-identical host-app set) | Forced by every row now carrying a non-empty Kind; internal mechanics, easily reversed, one obvious implementation | S:60 R:85 A:90 D:80 |
| 3 | Confident | Locus enum `gui` / `session` / `caller` / `host` with the fixed per-row mapping table | Discussed and shown in the reviewed artifact; user approved the shape | S:85 R:60 A:80 D:75 |
| 4 | Confident | Kind extended with `multiplexer` / `shell` / `clipboard` for former action rows | Discussed — completes the "every row classified" property; names follow existing kebab style | S:80 R:60 A:80 D:70 |
| 5 | Confident | `default: true` marker (JSON only, omitted when false) mirroring `DetectDefaultApp` | Shown in the reviewed examples; omitempty presentation is agent's choice — a false-on-every-row key is noise | S:70 R:75 A:75 D:60 |
| 6 | Confident | Empty host-app copy: "No host applications detected. {N} other target(s) available — see 'wt open --list --json' or the interactive menu." | Agent-proposed in discussion, user did not object; wording easily adjusted | S:60 R:90 A:80 D:70 |
| 7 | Confident | run-kit coordination out of scope: locus filter is a separate shll-repo change that deploys first; recorded in PR body + memory | User acknowledged the coordination consequence when choosing the expanded-JSON shape | S:75 R:55 A:70 D:65 |
| 8 | Confident | No changes to `-a` resolution/errors, menu, or launch behavior (artifact's structured error was illustrative) | Scope discipline — nothing in the backlog item or discussion asks for it | S:55 R:80 A:75 D:65 |

8 assumptions (2 certain, 6 confident, 0 tentative, 0 unresolved).
