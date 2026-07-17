# Intake: Show base ref in wt create summary

**Change**: 260717-aeka-create-summary-base-ref
**Created**: 2026-07-17

## Origin

One-shot `/fab-new` invocation, natural-language input:

> wt create's output prints 'Branch: <name>' but not the resulting branch's base/parent commit -- add a line showing what ref the new branch was created from (or 'checked out directly onto <ref>' when it's the existing-branch path), so it's visible at creation time which branch-vs-checkout path was taken.

No prior conversation context — the design below was derived from the request plus the existing
`wt create` contracts (`docs/memory/wt-cli/create-branch-semantics.md`,
`docs/memory/wt-cli/create-output-phases.md`) and `src/cmd/wt/create.go`.

## Why

1. **Pain point**: `wt create`'s Git-phase summary (`create.go:316-318`) prints
   `Created worktree:` / `Path:` / `Branch:` but never says what the branch was based on.
   After `260717-2af2-explicit-base-checkout-flags` there are three distinct creation modes
   (bare exploratory, positional new-branch, `--checkout` existing-branch), and the summary
   output is identical across all of them — a user cannot tell from the output whether a NEW
   branch was cut (and from which ref) or an EXISTING branch was checked out directly.
2. **Consequence if unfixed**: the exact confusion 2af2 was built to prevent survives at the
   output layer. A user who ran `wt create --checkout shared-branch` (or a script that did)
   sees the same summary shape as a fresh scratch branch — one `git commit` away from polluting
   a shared branch without realizing the worktree sits directly on it. Likewise, a new branch
   silently cut from an unexpected HEAD (e.g. user forgot they were on a feature branch, not
   `main`) is invisible until `git log` surprises them later.
3. **Why this approach**: one additional summary line in the existing deferred Git-phase block
   is the minimal change that makes the mode + base visible at creation time. It reuses the
   established stdout/stderr discipline (summary = stderr, per `create-output-phases.md`) and
   the existing single emission point, so no new output machinery is needed.

## What Changes

### 1. A fourth summary line: `From:`

The deferred Git-phase summary block in `src/cmd/wt/create.go` (currently the single
`fmt.Fprintf(os.Stderr, "Created worktree: %s\nPath: %s\nBranch: %s\n", ...)` at
`create.go:316-318`) gains a fourth line, `From: <value>`, emitted in the **same** Fprintf
call (stderr, still inside the deferred-summary emission under the rollback handler).
The value depends on the creation mode:

| Mode | `From:` value | Example |
|------|---------------|---------|
| Positional new branch, `--base` given | the `--base` ref verbatim as typed | `From: main` |
| Positional new branch, no `--base` | resolved HEAD label (current branch; short SHA when detached) | `From: 6lkr` |
| Bare exploratory create | same as positional (base verbatim, else HEAD label) | `From: main` |
| `--checkout <branch>` | fixed copy naming the existing-branch path | `From: existing branch 'feature/auth' (checked out directly)` |

Example full output (new branch, no `--base`, invoked from branch `main`):

```
── Git ─────────────────────────────────
Created worktree: swift-fox
Path: /home/u/repo.worktrees/swift-fox
Branch: swift-fox
From: main
```

Example (`--checkout feature/auth`):

```
── Git ─────────────────────────────────
Created worktree: auth
Path: /home/u/repo.worktrees/auth
Branch: feature/auth
From: existing branch 'feature/auth' (checked out directly)
```

The remote-only `--checkout` variant (fetch-then-checkout) uses the same checkout copy — the
user-visible outcome (worktree directly on the existing branch) is identical.

### 2. HEAD resolution helper in `internal/worktree`

A new helper in `src/internal/worktree/context.go` (next to the existing `CurrentBranch()`),
e.g.:

```go
// DescribeHead returns a display label for the current HEAD: the branch name,
// or the short SHA when detached. Best-effort — returns "HEAD" on any git
// error so a display label can never fail the create.
func DescribeHead() string
```

Implementation: `git rev-parse --abbrev-ref HEAD`; when the output is the literal `HEAD`
(detached), fall back to `git rev-parse --short HEAD`. On any error, return `"HEAD"` — the
line is informational and MUST NOT fail or abort the create.

Placement in `internal/` (not inline in `cmd/create.go`) per Constitution V — `cmd/` contains
no git operations; it already calls sibling helpers like `wt.CurrentBranch()`.

### 3. Resolution timing: before the create dispatch, outside the reinstall window

`cmd/create.go` computes the `From:` value **before** the create-dispatch switch (before any
`git worktree add` runs), stored in a local alongside `createdSummaryBranch`
(e.g. `createdSummaryFrom`):

- `checkout != ""` → the fixed checkout copy (no git query)
- `base != ""` → `base` verbatim (no git query)
- else → `wt.DescribeHead()` (one git query, executed pre-dispatch)

This ordering is load-bearing: the reinstall-window contract
(`create-output-phases.md` / spec § Signal Handling During Init) forbids new I/O or
nontrivial work between `git worktree add` returning and the init-phase `signal.Reset` —
the summary emission sits inside that window, so the git query must happen earlier. Resolving
HEAD pre-dispatch is semantically identical: `wt create` never moves the invoking worktree's
HEAD, so HEAD at dispatch time equals HEAD at summary time.

### 4. Out of scope / unchanged

- **`--reuse` path**: unchanged. It prints `Reusing existing worktree:` and no summary block;
  no branch is created or checked out, so there is no base ref to show.
- **stdout**: byte-identical. The machine-readable path line contract
  (`create-output-phases.md` § STDOUT discipline) is untouched — the new line is stderr-only.
- **`wt init` / Open phase / init-failure paths**: untouched; the new line rides inside the
  existing Git-phase emission only.
- **Existing summary lines**: `Created worktree:` / `Path:` / `Branch:` preserved verbatim
  (existing test assertions stay green).

### 5. Tests

Per Constitution IV (test what the user sees), extend `src/cmd/wt/create_test.go`:

- new branch with `--base <ref>` → stderr contains `From: <ref>`
- new branch without `--base` → stderr contains `From: <current branch name>` (test knows the
  branch it created the fixture repo on)
- `--checkout <existing>` → stderr contains the checkout copy (`existing branch` /
  `checked out directly` substrings)
- bare exploratory create → stderr contains `From: <current branch>`

Unit-test `DescribeHead()` in `src/internal/worktree/context_test.go` (or the file-pattern
sibling): on-branch → branch name; detached HEAD → short SHA. Run `gofmt` before finishing —
CI fails fast on `gofmt -l` (module root `src/`).

## Affected Memory

- `wt-cli/create-output-phases`: (modify) The Git-phase summary block gains the fourth
  `From:` line — update the "existing summary substrings preserved" requirement and examples.
- `wt-cli/create-branch-semantics`: (modify) Note that the three creation modes are now
  visually distinguished in the summary output (`From:` line per mode); no contract semantics
  change.

## Impact

- `src/cmd/wt/create.go` — compute `createdSummaryFrom` pre-dispatch; extend the deferred
  summary Fprintf with the `From:` line.
- `src/internal/worktree/context.go` — new `DescribeHead()` helper (+ test).
- `src/cmd/wt/create_test.go` — new assertions per mode.
- `docs/specs/cli-surface.md` § `wt create` documents flags/exit codes, not the summary line
  format — no spec change expected (phase/summary output lives in memory only, per
  `create-output-phases.md` cross-references).
- External consumers (launcher contract, fab-kit `batch switch`) parse stdout/exit codes only —
  a new stderr line is additive and safe.

## Open Questions

None — all decision points resolved as graded assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New line lives in the deferred Git-phase summary block on stderr; stdout byte-identical | `create-output-phases.md` pins stdout = machine path line only; summary block is the established emission point | S:85 R:90 A:95 D:95 |
| 2 | Confident | Label is `From:` as a fourth `key: value` summary line | Matches the existing `Created worktree:`/`Path:`/`Branch:` shape; user asked for "a line showing what ref the new branch was created from" | S:70 R:90 A:60 D:60 |
| 3 | Confident | Checkout-path copy: `From: existing branch '<branch>' (checked out directly)` | Echoes the user's suggested "checked out directly onto <ref>" phrasing while keeping the key: value shape; copy is trivially adjustable | S:60 R:90 A:55 D:45 |
| 4 | Confident | HEAD resolved pre-dispatch (before `git worktree add`), emitted inside the existing Fprintf | Reinstall-window contract forbids new subprocess work between worktree-add and signal.Reset; HEAD cannot move in between, so pre-resolution is equivalent | S:55 R:80 A:90 D:80 |
| 5 | Confident | New `DescribeHead()` helper in `internal/worktree`, best-effort (`"HEAD"` fallback on git error — never fails the create) | Constitution V keeps git ops out of `cmd/`; an informational line must not abort a successful create | S:50 R:85 A:90 D:80 |
| 6 | Certain | `--reuse` path unchanged (no summary block, no branch created — nothing to show) | Reuse short-circuits before branch dispatch per `create-branch-semantics.md`; out of scope | S:65 R:90 A:90 D:85 |
| 7 | Confident | `--base` shown verbatim as typed, not resolved to a SHA | The user's own token is the most recognizable label; resolution adds a query for no clarity gain | S:55 R:90 A:70 D:65 |

7 assumptions (2 certain, 5 confident, 0 tentative, 0 unresolved).
