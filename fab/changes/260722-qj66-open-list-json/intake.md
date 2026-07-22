# Intake: wt open --list --json — machine-readable registry of detected apps

**Change**: 260722-qj66-open-list-json
**Created**: 2026-07-22

## Origin

Backlog item `[qj66]` (2026-07-22), invoked via `/fab-new qj66` (one-shot; zero clarifying questions — the backlog entry already records the scope decisions from a discussion on 2026-07-22):

> wt open --list --json: machine-readable registry of detected apps for external consumers (run-kit). Emit the detected host apps as a JSON array of {id, label, kind} records (kind: editor|terminal|file-manager), mirroring the wt list --json machine-surface pattern; honor --app-style resolution identically to the interactive menu. Motivation: run-kit's navbar Open split-button needs (a) the detected-app list to populate its 'open on host' dropdown (GET /api/open-apps wraps this command) and (b) a validation source for app ids before it launches via the existing non-interactive path: wt open <path> -a <app>. Scope boundary (decided in discussion 2026-07-22): detection + listing + launch only — NO deeplink/URL-scheme knowledge in wt. Client-side deeplink templates (vscode://vscode-remote/ssh-remote+{host}{path} etc.) live in run-kit's frontend, because only client-machine installs matter for deeplinks and the host cannot detect those; host detection would be a wrong/inverted signal (e.g. Windsurf on client but not host).

## Why

1. **The pain point**: `wt open`'s detected-app catalog (`BuildAvailableApps()` in `src/internal/worktree/apps.go`) is reachable only through the interactive "Open in:" menu. An external consumer — concretely run-kit's navbar Open split-button — has no way to ask "which apps can this host open a directory in?" without scraping menu output or reimplementing detection. run-kit needs that list to (a) populate its 'open on host' dropdown (its `GET /api/open-apps` endpoint wraps this command) and (b) validate an app id before launching via the already-existing non-interactive path `wt open <path> -a <app>`.

2. **Consequence of not fixing**: run-kit would have to fork the app-detection catalog (bundle IDs, .desktop files, PATH probing per OS) — exactly the duplication `docs/specs/launcher-contract.md` §1 exists to prevent ("Subprocess delegation lets the caller inherit `wt`'s view of the world … and avoids forking the app catalog"). The catalog and its copy would drift.

3. **Why this approach**: a `--list` flag on `wt open` keeps the catalog owner (`wt open`) as the single query surface, and `--json` mirrors the established `wt list --json` machine-surface pattern (opt-in machine output, deterministic ordering, documented consumer contract — see `docs/memory/wt-cli/list-status-contract.md`). Because the listing is derived from the same `BuildAvailableApps()` call the menu and `-a` resolution use, every id emitted is guaranteed resolvable by `ResolveApp` — the "validation source" property falls out for free. Per `launcher-contract.md` §6, adding new internal flags to `wt open` is an explicitly non-breaking evolution.

## What Changes

### 1. `wt open --list` — list detected apps (human table)

New `--list` bool flag on `wt open` (`src/cmd/wt/open.go`). When set, the command prints the detected host apps and exits — no menu, no launch, no positional target.

- Detection is the **same `wt.BuildAvailableApps()` call** the interactive menu and `-a` resolution use ("honor --app-style resolution identically to the interactive menu"). No second detection path.
- The listing **filters to launchable host applications** — the three kinds `editor | terminal | file-manager`. Menu entries that are *actions* rather than host apps are excluded from `--list` (but remain in the interactive menu and remain valid `-a` values, unchanged):
  - `open_here` (cooperative shell-cd action — meaningless to an external launcher),
  - `copy_macos` / `copy_linux` (host-clipboard action),
  - `byobu_tab` / `tmux_window` / `tmux_session` (multiplexer actions whose detection depends on `wt`'s *own* process environment (`IsTmuxSession()`/`IsByobuSession()`), i.e. the consumer server's session — a wrong signal for a browser user, analogous to the deeplink inversion argument in Origin).
- Human output (no `--json`): a small aligned table (Id / Label / Kind columns), mirroring `wt list`'s human-default/`--json`-opt-in split. No per-row detection cost beyond what `BuildAvailableApps()` already does.
- `--list` requires **no git repository** — app detection is host-only. The `--list` branch runs before the soft git-context detection in `openCmd`'s `RunE`, so `wt open --list` works from any cwd (run-kit's server may invoke it from anywhere).
- Ordering: preserves `BuildAvailableApps()` detection order (the same order the menu shows, minus filtered rows). The catalog construction is deterministic per host state, so machine consumers get stable output (Constitution VI).

### 2. `wt open --list --json` — machine-readable JSON

New `--json` bool flag on `wt open`, valid only together with `--list`. Emits a JSON array of records, one per listed app:

```json
[
  {"id": "code", "label": "VSCode", "kind": "editor"},
  {"id": "cursor", "label": "Cursor", "kind": "editor"},
  {"id": "ghostty_macos", "label": "Ghostty", "kind": "terminal"},
  {"id": "iterm", "label": "iTerm2", "kind": "terminal"},
  {"id": "finder", "label": "Finder", "kind": "file-manager"}
]
```

- **`id`** — the internal command key (`AppInfo.Cmd`, e.g. `code`, `ghostty_macos`, `terminal_app`), emitted verbatim. This is the exact token `wt open <path> -a <id>` accepts (`ResolveApp` matches `Cmd` first), which is what makes the output a validation source for run-kit's launch path. Platform-suffixed keys (`ghostty_macos` vs `ghostty_linux`) are emitted as-is — the consumer round-trips ids, never interprets them.
- **`label`** — the display name (`AppInfo.Name`, e.g. `VSCode`, `Terminal.app`) for the dropdown UI.
- **`kind`** — one of `editor | terminal | file-manager` (closed enum for this change).
- Zero detected apps (possible on a minimal Linux host once action rows are filtered) emits `[]` — an empty JSON array, **not** `null` — and exits 0. (Implementation note: initialize the output slice non-nil; a nil Go slice marshals to `null`.)
- All three keys are always present on every record — no omitempty/pointer-field machinery needed here since no field is conditionally computed (contrast with `wt list --status`'s opt-in pointer fields).

### 3. `AppInfo.Kind` — kind classification at the catalog

`AppInfo` (`src/internal/worktree/apps.go`) gains a `Kind` field, populated where each app is appended in `BuildAvailableApps()`:

| Cmd key | Kind |
|---------|------|
| `code`, `cursor` | `editor` |
| `ghostty_macos`, `ghostty_linux`, `iterm`, `terminal_app`, `gnome_terminal`, `konsole` | `terminal` |
| `finder`, `nautilus`, `dolphin` | `file-manager` |
| `open_here`, `copy_macos`, `copy_linux`, `byobu_tab`, `tmux_window`, `tmux_session` | (empty — action rows, excluded from `--list`) |

The `--list` filter is "Kind non-empty". Menu behavior, `-a` resolution, `DetectDefaultApp`, and `SaveLastApp` are untouched — `Kind` is additive metadata.

### 4. Flag exclusivity

Following `wt list`'s `ExitWithError(wt.ExitInvalidArgs, …)` mutual-exclusion idiom (`src/cmd/wt/list.go`):

- `--list` + positional arg → `ExitInvalidArgs` ("mutually exclusive"-style what/why/fix message)
- `--list` + `--app` → `ExitInvalidArgs`
- `--list` + `--select` (or deprecated `--go`) → `ExitInvalidArgs`
- `--json` without `--list` → `ExitInvalidArgs` (`wt open` has no other JSON surface)

Validation happens at flag-check time, before any detection or git work.

### 5. Out of scope (decided 2026-07-22, recorded in backlog)

- **No deeplink/URL-scheme knowledge in `wt`.** Client-side deeplink templates (`vscode://vscode-remote/ssh-remote+{host}{path}` etc.) live in run-kit's frontend — only client-machine installs matter for deeplinks, and the host cannot detect those; host detection would be an inverted signal.
- No changes to the launch path (`-a` resolution, `OpenInApp`), the menu, default-app detection, or the `WT_CD_FILE`/`WT_WRAPPER` contract.
- No `default`-app marker in the JSON record — the shape stays the backlog-specified minimal `{id, label, kind}`; a marker would be an additive follow-up if run-kit ever needs preselection.

## Affected Memory

- `wt-cli/open-list-contract.md`: (new) The `wt open --list` / `--list --json` output contract — record shape, id-equals-`-a`-token guarantee, kind enum, action-row filtering, ordering, flag exclusivity, exit codes, no-git-required.

## Impact

- **Source**: `src/cmd/wt/open.go` (two flags, exclusivity checks, `--list` branch + human table + JSON emit), `src/internal/worktree/apps.go` (`Kind` field + population; possibly a small `ListableApps()`/filter helper so the filter rule lives beside the catalog per Constitution V — cmd/ stays orchestration-only).
- **Tests** (Constitution IV): `src/cmd/wt/open_test.go` — happy path (`--list`, `--list --json` shape/keys, `[]`-not-`null`), failure paths (each exclusivity pair), no-git-repo invocation; `src/internal/worktree/apps_test.go` — Kind population/filtering; integration coverage in `cmd/wt` integration tests as the existing patterns dictate. `WT_TEST_NO_LAUNCH` is irrelevant here (`--list` never launches).
- **Specs (human-curated, flag with the PR)**: `docs/specs/cli-surface.md` `wt open` flag table gains `--list`/`--json`; `docs/specs/launcher-contract.md` §2 invocation surface gains the query form (non-breaking per its own §6: "Adding new internal flags to `wt open`").
- **Toolkit standards**: CLI surface + help output change → per the constitution's Toolkit Standards article, check against `shll standards` (and note the re-audit in `wt-cli/toolkit-standards-conformance.md` only if a verdict changes). Verify whether `docs/site/skill.md` (the `wt skill` bundle, drift-guarded) mentions `wt open` flags and needs the sync script re-run.
- **External consumers**: run-kit's `GET /api/open-apps` is the intended consumer; `hop` (existing launcher-contract consumer) is unaffected — launch semantics unchanged.
- **Exit codes**: only existing constants used (`ExitSuccess`, `ExitInvalidArgs`); no new codes.

## Open Questions

*(none — the backlog entry resolves scope, shape, and motivation; remaining choices graded below)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | JSON record shape is exactly `{id, label, kind}` with `kind` one of `editor` / `terminal` / `file-manager` | Backlog entry specifies the shape and enum verbatim | S:90 R:70 A:95 D:95 |
| 2 | Certain | `id` = internal command key (`AppInfo.Cmd`, e.g. `code`, `ghostty_macos`); `label` = display name (`AppInfo.Name`) | run-kit feeds ids back to `wt open -a`, and `ResolveApp` matches `Cmd` keys first — only `Cmd` satisfies the validation-source motivation | S:85 R:80 A:95 D:90 |
| 3 | Confident | `--list` excludes action rows (`open_here`, `copy_*`, `byobu_tab`, `tmux_window`, `tmux_session`); they stay in the menu and `-a` unchanged | The three-kind enum implies host apps only; shell-cd/clipboard/multiplexer rows depend on `wt`'s own process env and are wrong signals for an external consumer | S:70 R:85 A:75 D:70 |
| 4 | Certain | Kind mapping: `code`/`cursor` → editor; `ghostty_*`/`iterm`/`terminal_app`/`gnome_terminal`/`konsole` → terminal; `finder`/`nautilus`/`dolphin` → file-manager | Unambiguous classification of the existing catalog against the backlog's enum | S:85 R:85 A:95 D:90 |
| 5 | Confident | `--list` without `--json` prints a human table; `--json` opts into machine output | Mirrors the established `wt list` human-default/`--json`-opt-in split named in the backlog ("mirroring the wt list --json machine-surface pattern") | S:55 R:85 A:80 D:65 |
| 6 | Confident | `--list` is mutually exclusive with the positional arg, `--app`, and `--select`; `--json` requires `--list`; violations exit `ExitInvalidArgs` | `wt list`'s mutex idiom and Constitution III typed exit codes; a query flag composing with launch flags has no coherent meaning | S:60 R:90 A:85 D:75 |
| 7 | Certain | Output preserves `BuildAvailableApps()` detection order (menu order minus filtered rows) | Deterministic machine output per Constitution VI; reusing the single catalog order avoids a second ordering definition | S:60 R:90 A:85 D:80 |
| 8 | Confident | No `default`-app marker field in the record | Backlog defines the minimal shape; preselection is not requested; the field would be a purely additive follow-up | S:65 R:90 A:70 D:60 |
| 9 | Confident | Zero listed apps emits `[]` (empty array, not `null`) and exits 0 | Machine consumers parse an array; an empty host list is a valid answer, not an error — matches `wt list --json` array semantics | S:50 R:90 A:85 D:80 |
| 10 | Certain | `--list` requires no git repository (branch runs before git-context detection) | App detection is host-only; `wt open` already soft-detects git context, and run-kit's server invokes from arbitrary cwds | S:60 R:85 A:90 D:85 |
| 11 | Certain | No deeplink/URL-scheme knowledge in `wt`; deeplink templates live in run-kit's frontend | Decided in discussion 2026-07-22 and recorded verbatim in the backlog entry (host detection would be an inverted signal for client installs) | S:95 R:80 A:95 D:95 |

11 assumptions (6 certain, 5 confident, 0 tentative, 0 unresolved).
