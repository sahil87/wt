# Spec: List Recency Column

**Change**: 260601-73cv-list-recency-column
**Created**: 2026-06-01
**Affected memory**: `docs/memory/wt-cli/list-status-contract.md`, `docs/memory/wt-cli/recency-ordering-contract.md`

<!--
  Amends the `wt list` default-human-output contract: the recency-ordered human
  view (default human mode, or explicit `--sort=recent`) gains a `Last Active`
  column rendered from the already-computed recency sort key. JSON, --status, and
  the name/branch human modes are explicitly unchanged. Grounded in
  src/cmd/wt/list.go (handleFormattedOutput, sortEntries, relativeTime,
  resolveSort, listEntry) and the two affected memory files.
-->

## Non-Goals

- JSON / `--non-interactive` output changes — the machine-readable contract (Constitution VI) is untouched; `last_active` stays `omitempty` and `--status`-only.
- The `--status` 5-column dashboard view — its layout (Name / Branch / Status / Last Active / Path) is unchanged.
- `name` / `branch` human sort modes — they stay 3-column (Name / Branch / Path) and acquire no per-worktree `os.Stat`.
- A new recency signal, finer-grained or absolute timestamps, or any new flag — none are introduced; the change reuses `RecencyOf`, `relativeTime`, and the existing flag surface.
- Changes to the shared comparator/ordering in `src/internal/worktree/recency.go` — the ordering definition is unchanged.

## wt-cli/list-status-contract: Recency Human View

### Requirement: Last Active column in the recency-ordered human view
When the resolved sort mode is `recent` AND `--status` is NOT set AND output is human (neither `--json` nor `--non-interactive`), `wt list` SHALL render a **4-column** table with columns `Name`, `Branch`, `Last Active`, `Path` in that order. The `Last Active` cell SHALL be rendered via the existing `relativeTime(t time.Time)` helper (coarse buckets `just now`, `Nm ago`, `Nh ago`, `Nd ago`; a zero `time.Time` renders as `-`). The displayed value MUST be the same recency key the rows were sorted by — no second `os.Stat`, no `git` subprocess, no TOCTOU between sort key and displayed value.

#### Scenario: Default human view shows the relative time column
- **GIVEN** a repository with a main worktree and one or more non-main worktrees with distinct recent mtimes
- **WHEN** the user runs `wt list` with no flags (default human output → `recent`)
- **THEN** the table header SHALL read `Name`, `Branch`, `Last Active`, `Path` in that order
- **AND** each non-main row SHALL show a relative-time value in the `Last Active` column matching the row's worktree-directory mtime bucket

#### Scenario: Explicit `--sort=recent` shows the column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=recent` (human output)
- **THEN** the 4-column `Name / Branch / Last Active / Path` layout SHALL be rendered, identical to the default human view

#### Scenario: Recent and stale worktrees render distinct buckets
- **GIVEN** one worktree touched seconds ago and another whose mtime is several days old
- **WHEN** the user runs `wt list` (human, recent default)
- **THEN** the recently-touched worktree's `Last Active` SHALL render `just now`
- **AND** the older worktree's `Last Active` SHALL render `Nd ago`
- **AND** the recently-touched worktree SHALL appear above the older one (newest-first ordering preserved)

#### Scenario: Displayed value equals the sort key
- **WHEN** the recent-mode human view renders a row's `Last Active`
- **THEN** the value SHALL be derived from the same recency key used to order that row (the persisted `LastActive`), not from a fresh `os.Stat` performed in the render path

### Requirement: Persist the recency sort key into LastActive
For `recent` mode, `sortEntries` SHALL write the per-entry recency key it computes back into `entries[i].LastActive` instead of discarding it after ordering. In default/basic mode this key is the `wt.RecencyOf(e.Path)` value; under `--status` `LastActive` is already non-nil and SHALL be left as-is. This write SHALL reuse the stat already paid for the sort key and MUST NOT introduce a *second* `os.Stat` for the same entry or any `git` subprocess.

The main-worktree case requires explicit handling. Today `sortEntries` partitions the main worktree out (`start = 1` when `entries[0].IsMain`; only the non-main `rest` slice has keys computed) and does NOT stat the main entry in basic mode. Because the recent-mode human view SHALL display the main worktree's own `Last Active` (see "Current-worktree and main-worktree rendering preserved"), the implementation SHALL, when `persistKey` is true and `LastActive` is nil, populate the main entry's `LastActive` via a single `wt.RecencyOf(entries[0].Path)` — exactly one stat for the main entry, paralleling how `listEntriesEnriched` already stats every entry including main. Under `--status`, the main entry's `LastActive` is already non-nil and SHALL be left as-is. The main worktree's row position is unchanged (pinned first); only its `LastActive` is populated for display.
<!-- clarified: main-worktree LastActive — sortEntries partitions main out of the key-computation `rest` slice (list.go:186-190), so basic recent mode never stats main and its Last Active would wrongly render `-`. Resolved: when persistKey is true, populate the pinned main entry's nil LastActive via a single RecencyOf(Path). Grounded in list.go and the existing per-entry stat pattern in listEntriesEnriched. -->

#### Scenario: Main worktree recency key is persisted in basic recent mode
- **GIVEN** default/basic recent mode (no `--status`), where `entries[0].IsMain` and its `LastActive` is nil on entry
- **WHEN** `sortEntries` runs with `persistKey == true`
- **THEN** the main entry's `LastActive` SHALL be set to `wt.RecencyOf(entries[0].Path)`
- **AND** exactly one `os.Stat` SHALL be performed for the main entry (no second stat, no `git` subprocess)

The write-back MUST be gated so it is NOT performed on the JSON path. In `list.go`, `sortEntries` runs (line 123) **before** the `jsonOut` branch (lines 125-127), so an unconditional write-back in `sortEntries` would cause `--json --sort=recent` to leak `last_active` (a non-nil `*time.Time` is serialized despite `omitempty`). The implementation SHALL therefore gate the persistence on the human-output path: `sortEntries` SHALL accept an explicit `persistKey bool` parameter (set `!jsonOut` by the caller, i.e. true only when output is human), and SHALL write the recency key into `entries[i].LastActive` only when `persistKey` is true. Under `--status`, `LastActive` is already non-nil from `listEntriesEnriched`, so `--status --json` continues to emit `last_active` regardless of `persistKey`. This keeps the displayed value equal to the sort key on the human path while guaranteeing the JSON path observes the same nil `LastActive` it does today.
<!-- clarified: write-back seam pinned to a `persistKey bool` parameter on `sortEntries` (caller passes `!jsonOut`). Grounded in list.go:123-129 where sortEntries precedes the jsonOut branch — an unconditional write would leak last_active into `--json --sort=recent`. Resolves the open implementation choice flagged in Assumption #8 and the serialization note. -->

#### Scenario: JSON path does not persist the recency key
- **GIVEN** `--json --sort=recent` (human output is false → `persistKey == false`)
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** it SHALL order the non-main entries newest-first using the computed keys
- **AND** it SHALL NOT write any value into `entries[i].LastActive` (the nil pointer is preserved, so `handleJSONOutput` omits `last_active` via `omitempty`)

#### Scenario: Sort key is persisted in default mode
- **GIVEN** default/basic mode (no `--status`), where `entries[i].LastActive` is nil on entry
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** each reordered non-main entry's `LastActive` SHALL be set to the `wt.RecencyOf(Path)` value used as its sort key
- **AND** no additional `os.Stat` beyond the one already computed for the sort key SHALL be performed

#### Scenario: Existing --status LastActive is not recomputed
- **GIVEN** `--status` mode, where `listEntriesEnriched` has already set `LastActive` to a non-nil pointer for every entry
- **WHEN** `sortEntries` runs in `recent` mode
- **THEN** the existing non-nil `LastActive` SHALL be used as the sort key and left unchanged (no overwrite, no re-stat)

#### Scenario: Vanished worktree yields zero time
- **GIVEN** a worktree whose directory cannot be stat'd (vanished / permission error)
- **WHEN** `sortEntries` computes its recency key via `wt.RecencyOf(Path)`
- **THEN** the key SHALL be the zero `time.Time`
- **AND** the persisted `LastActive` SHALL be that zero time, which `relativeTime` renders as `-` in the human view

### Requirement: Thread the resolved sort mode into the rendering decision
`handleFormattedOutput` SHALL receive the resolved `sortMode` (or a boolean derived from it) so it can select the 4-column recent-mode layout. It MUST NOT key the layout solely on `showStatus`. The layout selection SHALL be: `--status` → 5-column (Name / Branch / Status / Last Active / Path); else recent human mode → 4-column (Name / Branch / Last Active / Path); else (`name`/`branch` modes) → 3-column (Name / Branch / Path).

#### Scenario: Recent mode selects the 4-column layout
- **GIVEN** `showStatus == false` and resolved sort mode `recent`
- **WHEN** `handleFormattedOutput` renders
- **THEN** it SHALL emit the 4-column `Name / Branch / Last Active / Path` table

#### Scenario: Status mode still selects the 5-column layout
- **GIVEN** `showStatus == true` (any sort mode)
- **WHEN** `handleFormattedOutput` renders
- **THEN** it SHALL emit the unchanged 5-column `Name / Branch / Status / Last Active / Path` table

### Requirement: Name and branch human modes stay three columns
When the resolved sort mode is `name` or `branch` AND `--status` is NOT set, `wt list` SHALL render the unchanged 3-column `Name / Branch / Path` table with no `Last Active` column. These modes MUST NOT perform any per-worktree `os.Stat` for the purpose of rendering recency.

#### Scenario: `--sort=name` human view has no time column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=name` (human output)
- **THEN** the header SHALL read `Name`, `Branch`, `Path` with no `Last Active` column
- **AND** the rows SHALL be ordered by `Name` ascending (main pinned first)

#### Scenario: `--sort=branch` human view has no time column
- **GIVEN** a repository with multiple worktrees
- **WHEN** the user runs `wt list --sort=branch` (human output)
- **THEN** the header SHALL read `Name`, `Branch`, `Path` with no `Last Active` column

#### Scenario: Name/branch modes add no recency stat
- **GIVEN** `name` or `branch` mode without `--status`
- **WHEN** the list is rendered
- **THEN** no `os.Stat` SHALL be invoked to populate a recency value for display (the single-`git`-subprocess basic-mode contract is preserved)

### Requirement: Current-worktree and main-worktree rendering preserved
The current-worktree green `*` marker and the bold `(main)` name rendering SHALL be preserved in the new 4-column recent-mode layout, identical to their behavior in the existing 3- and 5-column layouts. The main worktree SHALL remain pinned to the first output row under recent mode and SHALL display its own `Last Active` value.

#### Scenario: Main worktree row in recent mode
- **GIVEN** a repository whose main worktree participates in the recent-mode human view
- **WHEN** `wt list` renders the 4-column recent view
- **THEN** the main worktree SHALL be the first data row, rendered bold as `(main)`
- **AND** its `Last Active` cell SHALL show the relative time of its own directory mtime (or `-` if zero)

#### Scenario: Current worktree marker in recent mode
- **GIVEN** the user's CWD is inside one of the listed worktrees
- **WHEN** the recent-mode 4-column view renders
- **THEN** that worktree's row SHALL be prefixed with the green `*` marker

### Requirement: Empty and single-worktree lists render the new header without rows
The recent-mode 4-column header SHALL render correctly when there are zero non-main worktrees (header plus the main row, or header plus the total line), without panic or misalignment.

#### Scenario: Repository with only the main worktree
- **GIVEN** a repository with no non-main worktrees
- **WHEN** the user runs `wt list` (human, recent default)
- **THEN** the 4-column header SHALL be emitted
- **AND** the main worktree row (with its `Last Active`) and the `Total: 1 worktree(s)` line SHALL follow without error

## wt-cli/list-status-contract: Machine Output Unchanged

### Requirement: JSON output is unchanged by this change
`--json` output SHALL NOT gain a `last_active` value as a result of this change. `last_active` SHALL remain `omitempty` on `listEntry` and SHALL be emitted only under `--status` (where it is already populated by `listEntriesEnriched`). `--json` and `--json --sort=recent` SHALL NOT emit `last_active` unless `--status` is also set. `--json` without `--sort` SHALL keep its stable `name` default order; `--json --sort=recent` SHALL order recent but still omit `last_active`. `--non-interactive` SHALL likewise keep its stable `name` default and gain no field.

#### Scenario: `--json` without `--status` omits last_active
- **GIVEN** any repository state
- **WHEN** the user runs `wt list --json` (no `--status`)
- **THEN** no object in the JSON array SHALL contain a `last_active` key
- **AND** the order SHALL be stable `name` ascending (main first)

#### Scenario: `--json --sort=recent` sorts recent but still omits last_active
- **GIVEN** a repository with multiple worktrees of distinct mtimes
- **WHEN** the user runs `wt list --json --sort=recent` (no `--status`)
- **THEN** the array SHALL be ordered newest-first (non-main entries)
- **AND** no object SHALL contain a `last_active` key (because `sortEntries` is called with `persistKey == false` on the JSON path, the nil `LastActive` pointer is preserved and `omitempty` omits the key — see Design Decisions #1 and the serialization note)

#### Scenario: `--status --json` still emits last_active
- **GIVEN** a repository with worktrees
- **WHEN** the user runs `wt list --status --json`
- **THEN** every object SHALL contain a `last_active` key (unchanged `--status` behavior), a vanished worktree serialized as `"0001-01-01T00:00:00Z"`

## wt-cli/recency-ordering-contract: Sort-Key Reuse

### Requirement: Recent-mode list persists the computed recency key for display
The `recent`-mode list path SHALL persist the recency key it computes (previously discarded after sorting) into `LastActive` for the rendering layer to display. The comparator and ordering definition (`RecencyOf` / `RecencyLess`, newest-first with Name-ascending tie-break) SHALL be unchanged — this change only stops discarding the already-computed key.

#### Scenario: Ordering definition is unchanged
- **GIVEN** the same set of worktree mtimes
- **WHEN** `wt list --sort=recent` orders them
- **THEN** the resulting order SHALL be identical to the order produced before this change (newest-first, Name-ascending tie-break)
- **AND** only the side-effect of persisting the key into `LastActive` SHALL differ

## Testability (Constitution IV / VI)

### Requirement: Behavior is covered by unit and integration tests
Every behavior above SHALL be covered by tests that assert what the user sees: the 4-column recent-mode header and a relative-time value in default human output; that `name`/`branch` human modes remain 3-column; that `--json` (with and without `--sort=recent`) omits `last_active` without `--status`; and that `--status` is unchanged. Integration tests SHALL exercise the binary end-to-end against a real git repo via `t.TempDir()`. Tests MUST NOT leak side-effects to the host (no real worktree-open/app shell-outs).

#### Scenario: Recent human header asserted by test
- **GIVEN** a temp-repo fixture with worktrees of controlled mtimes (e.g., via `os.Chtimes`)
- **WHEN** the test runs `wt list` (human default)
- **THEN** the captured stdout SHALL contain the `Last Active` header and a relative-time string for a non-main row

#### Scenario: Machine-output stability asserted by test
- **GIVEN** a temp-repo fixture
- **WHEN** the test runs `wt list --json` and `wt list --json --sort=recent`
- **THEN** in both cases no object SHALL contain a `last_active` key
- **AND** plain `--json` SHALL be `name`-ordered while `--json --sort=recent` SHALL be recency-ordered

## Design Decisions

1. **Persist the discarded sort key rather than re-stat in the render path**
   - *Why*: `sortEntries` already computes the recency key (`wt.RecencyOf(e.Path)` in basic mode, or reuses the non-nil `*LastActive` under `--status`) and discards it. Writing it back into `entries[i].LastActive` shows the value at zero additional cost and guarantees the displayed value equals the sort key.
   - *Gated by `persistKey`*: because `sortEntries` runs before the `jsonOut` branch (list.go:123-129), the write-back is gated on a `persistKey bool` parameter (caller passes `!jsonOut`). Human output persists the key (and renders it); the JSON path passes `persistKey == false`, leaving `LastActive` nil so `omitempty` keeps `last_active` out of `--json --sort=recent`.
   - *Rejected*: re-statting in `handleFormattedOutput` — reintroduces a per-worktree `os.Stat`, creates a TOCTOU drift window between the sort key and the displayed value, and duplicates recency logic the render layer should not own.
   - *Rejected*: unconditional write-back in `sortEntries` — would serialize the transiently-set `LastActive` into `--json --sort=recent`, breaking the JSON present-vs-uncomputed contract.

2. **Scope the column to recent mode only**
   - *Why*: `name`/`branch` human modes do zero per-worktree work today; adding a `Last Active` column there would force a per-worktree `os.Stat` purely for display, violating the cheap-default-path spirit of the list contract.
   - *Rejected*: showing the column in all human modes (adds stats to cheap modes); an absolute timestamp column (wider, less scannable than the relative buckets).

3. **Thread the resolved sortMode into handleFormattedOutput**
   - *Why*: the function keys layout only on `showStatus` today; recency rendering needs to know the sort mode. Passing the already-resolved `sortMode` (or a derived bool) is the minimal seam — no new state, no recomputation.
   - *Rejected*: recomputing `resolveSort` inside `handleFormattedOutput` (duplicates resolution logic and its flag inputs); inferring recency from non-nil `LastActive` (ambiguous — `--status` also sets it, and that path must stay 5-column).

4. **Leave JSON / `--non-interactive` untouched**
   - *Why*: the request is explicitly about the human recency view. Constitution VI requires deterministic machine output; fab-kit operators parse stable `name`-ordered `--json`. `last_active` stays `omitempty`/`--status`-only.
   - *Rejected*: emitting `last_active` whenever it is set (would leak the transiently-persisted recent-mode key into JSON and break the present-vs-uncomputed contract) — see the JSON-unchanged requirement and the note below.

> **Serialization note (resolved, for plan/apply)**: because `recent`-mode `sortEntries` writes back into `entries[i].LastActive`, the implementation MUST ensure `--json` does not begin emitting `last_active` as a side effect. Correction to an earlier draft: `sortEntries` runs at list.go:123 **before** the `jsonOut` branch at list.go:125-127, so the write-back IS observable on the JSON path — the order does NOT protect JSON on its own. The resolved seam: `sortEntries` takes a `persistKey bool` (caller passes `!jsonOut`); the key is written into `LastActive` only on the human path. For `--json --sort=recent`, `persistKey == false`, the nil pointer is preserved, and `omitempty` omits `last_active`. Bare `--json` additionally stays `name`-ordered via `resolveSort`. This fully resolves the implementation choice flagged in Assumptions #8 — no open question remains.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = recent mode only (default human view + `--sort=recent`); `name`/`branch` human modes stay 3-column with no recency stat | Confirmed from intake #1. Avoids adding `os.Stat` to modes that do zero per-worktree work; preserves the cheap-default-path contract in `list-status-contract.md` | S:95 R:80 A:90 D:90 |
| 2 | Certain | Reuse the existing `relativeTime()` coarse buckets (`just now`/`Nm`/`Nh`/`Nd`; zero → `-`) for the column value | Confirmed from intake #2. Zero new formatting logic, consistent with the existing `--status` Last Active column | S:95 R:85 A:95 D:90 |
| 3 | Certain | Treat the `Name/Branch/Path` default-human-layout change as an intentional amendment to `list-status-contract.md`, documented at hydrate | Confirmed from intake #3. The "Default human output: Name/Branch/Path" invariant is superseded for recency-ordered output by explicit spec amendment | S:90 R:70 A:85 D:85 |
| 4 | Certain | Persist the discarded `sortEntries` recency key into `entries[i].LastActive` rather than re-statting in the render path | Confirmed from intake #4. Reuses the already-paid `os.Stat`; avoids TOCTOU between sort key and displayed value; the re-stat alternative reintroduces a second stat and a drift window | S:95 R:75 A:85 D:80 |
| 5 | Certain | JSON / `--non-interactive` output unchanged — `last_active` stays `omitempty`/`--status`-only; machine modes keep stable `name` default | Confirmed from intake #5. Preserves Constitution VI machine-stability and the existing JSON present-vs-uncomputed contract; the request is about the human view | S:95 R:70 A:90 D:85 |
| 6 | Certain | Thread the resolved `sortMode` (or a derived bool) into `handleFormattedOutput` to select the 4-column layout | Confirmed from intake #6. `handleFormattedOutput` keys only on `showStatus` today; recency rendering needs the sort mode. Threading the already-resolved mode is the minimal seam | S:95 R:75 A:80 D:75 |
| 7 | Certain | Column order = `Name / Branch / Last Active / Path` (4 columns) | Confirmed from intake #7. Mirrors the `--status` layout's relative placement (Last Active immediately before the wide variable-width Path) and keeps the time column left-aligned | S:95 R:85 A:60 D:55 |
| 8 | Certain | The render/JSON seam keeps `last_active` out of `--json --sort=recent` by gating the `LastActive` write-back on a `persistKey bool` parameter to `sortEntries` (caller passes `!jsonOut`); JSON path passes `false`, leaving the pointer nil so `omitempty` omits the key | Clarified during auto clarify against list.go. `sortEntries` (line 123) runs before the `jsonOut` branch (lines 125-127), so an unconditional write WOULD leak `last_active` into `--json --sort=recent` — the earlier "JSON invoked before persistence" reasoning was wrong. The gated `persistKey` seam is the single correct mechanism: it's grounded in the real call order, low-blast-radius, and has one obvious outcome. No spec ambiguity remains | S:95 R:85 A:90 D:90 |
| 9 | Certain | Populate the pinned main worktree's `LastActive` via a single `wt.RecencyOf(Path)` when `persistKey` is true and it is nil — `sortEntries` partitions main out of the key-computation `rest` slice today, so basic recent mode never stats main | Clarified during auto clarify against list.go:186-190. Without this, the main row's `Last Active` would render `-` instead of its real mtime, contradicting the "main worktree displays its own Last Active" requirement. One extra stat for the single main entry parallels the existing per-entry stat in `listEntriesEnriched`; one obvious correct outcome | S:90 R:85 A:85 D:85 |

9 assumptions (9 certain, 0 confident, 0 tentative, 0 unresolved).
<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
