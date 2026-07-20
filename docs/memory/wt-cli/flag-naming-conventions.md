---
type: memory
description: "The `wt` CLI flag-surface conventions — no command-noun prefixes, `--no-*` negation, short flags only for common interactive use, the additive rename-via-MarkDeprecated back-compat contract (stderr warning, hidden from help, old names never removed), and the git/unix command aliases (new/rm/ls)."
---
# wt-cli: Flag Naming Conventions

**Domain**: wt-cli

> Post-implementation behavior capture for the cross-command flag-surface
> conventions the `wt` CLI follows.
> Source change: `260717-59u8-intuitive-flag-names`.

## Overview

This file records the flag-naming and back-compat conventions that govern every
`wt` subcommand's flag surface (`260717-59u8-intuitive-flag-names`). These are
**cross-command** rules — they sit above any single command contract. The
per-command flag *behavior* still lives in its own contract file
([`create-branch-semantics`](/wt-cli/create-branch-semantics.md),
[`go-command-contract`](/wt-cli/go-command-contract.md),
[`idle-staleness-contract`](/wt-cli/idle-staleness-contract.md),
[`update-command-contract`](/wt-cli/update-command-contract.md), etc.); this file
owns only the *naming* and *back-compat* shape those contracts share. Future
flag additions or renames on any `wt` command SHOULD conform to these
conventions unless an explicit spec amendment supersedes them.

These conventions refine, they do not replace, constitution Principle II (Cobra
Command Surface): long-form names, single-letter shorts only where they aid
common interactive use, `RunE`, `SilenceUsage`/`SilenceErrors`.

## Requirements

### No command-noun / command-verb prefixes on flags
A flag name SHALL NOT repeat its own command's noun or verb. A flag reads inside
its command's context, so `wt create --worktree-name` said "worktree" twice and
`wt delete --delete-branch` said "delete" twice. The prefix carries no
information the command name has not already established.

- Primary names and their permanent deprecated aliases: `--name`
  (`--worktree-name`), `--open` (`--worktree-open`), `--no-init`
  (`--worktree-init` — see negation below), `--all` (`--delete-all`),
  `--branch` (`--delete-branch`), `--no-remote` (`--delete-remote` — see
  negation below) (260717-59u8).

#### Scenario: a renamed flag drops its redundant prefix
- **GIVEN** the `wt create` command
- **WHEN** a user sets the worktree name
- **THEN** the primary flag is `--name` (not `--worktree-name`), because "create"
  already scopes the noun.

### Negation is expressed as `--no-*` with a real boolean
A flag that turns OFF a default-ON behavior SHALL be a **real boolean** named
`--no-<thing>`, not a string-typed boolean (`true|false`) and not an
inconsistent verb (`--skip-*`). The default (behavior ON) is preserved; passing
`--no-<thing>` disables it.

- Current `--no-*` booleans and their string-typed deprecated aliases:
  `--no-init` (alias `--worktree-init true|false`, string); `--no-remote`
  (alias `--delete-remote true|false`, string).
- **`--no-brew-update` is a `--no-*` boolean but NOT a rename** — its sibling
  `--skip-brew-update` is a **visible, non-deprecated** toolkit contract flag, not
  a deprecated alias (see the frozen-contract-flag carve-out below and
  [`update-command-contract`](/wt-cli/update-command-contract.md)). Both bind one
  bool; neither is hidden or warns.
- **Genuine tri-states stay strings.** `--branch` (`true|false|auto`) KEEPS its
  string type because `auto` is a real third state, not a
  boolean — see [`create-branch-semantics`](/wt-cli/create-branch-semantics.md)
  and the `wt delete` flag table in `docs/specs/cli-surface.md`.

#### Scenario: a string-bool default becomes a `--no-*` real bool
- **GIVEN** `wt create`, whose init script runs by default
- **WHEN** a user wants to skip it
- **THEN** they pass the real boolean `--no-init` (the old `--worktree-init false`
  string form still works as a deprecated alias — see back-compat below).

### Short flags only where they aid common interactive use
A single-letter short SHALL be added only for a flag reached often at an
interactive terminal, and only where it does not collide or mislead. Script-facing
or rare flags stay long-form-only (Principle II).

- Shorts: `-n` (`--name`), `-o` (`--open`) on `wt create`; `-a` (`--all`)
  on `wt delete`; `-a` (`--app`) on `wt open`. `-s` is `--stash` on
  `wt delete`, so `--all` takes `-a`.
- **`-a` reused across commands is fine** — `wt delete --all` and `wt open --app`
  are different commands, no collision.
- **Deliberately NO short** for: `--select` on `wt open` (not frequent enough);
  `--base` on `wt create` (a `-b` would collide with `git worktree add -b`, which
  there NAMES the new branch — actively misleading to git users);
  `--non-interactive` (script-facing everywhere; explicitness beats brevity, and
  it is named in constitution Principle VI); `--dry-run` on `wt delete` (a
  preview/automation flag, not common interactive use — same rationale as
  `--non-interactive`; a `-d` would also risk a delete-adjacent misfire on the
  most destructive command). See
  [`delete-dry-run-contract`](/wt-cli/delete-dry-run-contract.md).

### `--open` requires an explicit value — no `NoOptDefVal`
`wt create --open` SHALL NOT be given a `NoOptDefVal`. Because `wt create` takes a
positional `[branch]`, a bare `--open code` would parse `code` as the positional
branch argument — a silent footgun. `--open` always requires an explicit value
(`prompt` / `default` / `skip` / an app name). (Contrast `wt delete --stale`,
which intentionally DOES carry a `NoOptDefVal` of `"7d"` — see
[`idle-staleness-contract`](/wt-cli/idle-staleness-contract.md); that is a
value-optional flag by design, `--open` is not.)

### Back-compat: rename-via-`MarkDeprecated` — old names are never removed
Every flag rename SHALL be **strictly additive**. The **new** name is registered
as primary; the **old** name is kept registered so both feed the same behavior;
then `cmd.Flags().MarkDeprecated("<old>", "use --<new> instead")` is called. NO
old flag is ever removed. Precedent: `src/cmd/wt/delete.go`'s pre-existing
`--worktree-name` deprecation.

- **pflag gives three behaviors for free** on a `MarkDeprecated` flag:
  1. the old flag is **auto-hidden** from `--help`;
  2. passing the old flag prints `Flag --<old> has been deprecated, use --<new>
     instead` to **stderr** (pflag's `Set()` → `f.out()` default is `os.Stderr`),
     never stdout — so machine-parsed stdout is unaffected (the stdout=machine /
     stderr=human convention, see
     [`create-output-phases`](/wt-cli/create-output-phases.md));
  3. passing the **new** name prints no warning.
- **Deprecation message wording** is the consistent `"use --<new> instead"` form.
- **Same-type renames share one variable.** For `--name`/`--worktree-name`,
  `--open`/`--worktree-open`, `--all`/`--delete-all`, `--branch`/`--delete-branch`,
  `--select`/`--go`, both flags bind the SAME pointer (pflag permits this) — the
  internal variable name and any downstream signature are unchanged, so the rename
  is purely `cmd/`-layer flag surface. (`--skip-brew-update`/`--no-brew-update` also
  share one pointer, but that pair is NOT a `MarkDeprecated` rename — see the
  frozen-contract-flag carve-out below.)
- **String→bool conversions use two variables reconciled via `Changed()`.** For
  `--no-init` (vs. old string `--worktree-init`) and `--no-remote` (vs. old string
  `--delete-remote`), the types differ, so a shared pointer is impossible. The old
  string flag stays registered on its own variable and is deprecated; `RunE`
  reconciles by giving the new bool precedence when
  `cmd.Flags().Changed("<new>")` reports it explicitly set, else honoring the old
  string parse path. Default behavior (flag absent) is preserved byte-for-byte.

#### Scenario: an old flag name still works but warns on stderr
- **GIVEN** any renamed flag
- **WHEN** the user passes the OLD name (e.g. `wt create --worktree-name foo`)
- **THEN** the command behaves exactly as before, a
  `Flag --worktree-name has been deprecated, use --name instead` warning is
  printed to **stderr**, and the old flag does NOT appear in `--help`.

#### Scenario: the new flag name warns nothing
- **GIVEN** any renamed flag
- **WHEN** the user passes the NEW name (no old flag)
- **THEN** the command behaves correctly and NO deprecation warning is printed.

### Carve-out: a toolkit-standard frozen contract flag may not be hidden via `MarkDeprecated`
A flag whose exact name a **published shll toolkit standard freezes as a `--help`
substring contract** SHALL stay **visible and non-deprecated** — the
`MarkDeprecated` rename mechanism above yields to the standard. Hiding such a flag
(the pflag auto-hide) breaks the standard's `strings.Contains` probe, and printing
a deprecation warning on a flag the toolkit passes on every run is noise. When the
repo's `--no-*` convention also wants a name, register **both** as visible flags
bound to one bool (no `MarkDeprecated`); the standard-frozen name carries the
canonical help text and the convention name reads `alias for --<standard-name>`.

- **Instance**: `--skip-brew-update` is frozen by the shll `update` standard (probed
  via `strings.Contains` before every toolkit-wide run), so it is visible and
  non-deprecated; `--no-brew-update` is a visible alias satisfying the `--no-*`
  convention. Neither is hidden and neither warns
  ([`update-command-contract`](/wt-cli/update-command-contract.md),
  [`toolkit-standards-conformance`](/wt-cli/toolkit-standards-conformance.md)).
- This is the one place the additive-rename shape does NOT apply: the standard's
  substring contract is the higher authority (Constitution § Toolkit Standards binds
  published standards without amendment).

#### Scenario: a contract flag stays visible instead of being deprecated
- **GIVEN** `wt update`, whose `--skip-brew-update` name is a standard-frozen `--help` substring
- **WHEN** the repo also wants the `--no-brew-update` negation name
- **THEN** both are registered visible on one bool (no `MarkDeprecated`), so
  `wt update --help` contains the literal `--skip-brew-update` and neither flag prints a warning.

### Command aliases for git/unix muscle memory
Commands with a familiar git/unix equivalent SHALL carry a cobra `Aliases:` entry
so the muscle-memory name invokes them identically (zero behavior risk — cobra
aliases are a pure dispatch shim).

- `wt new` → `wt create` (`Aliases: []string{"new"}`)
- `wt rm` → `wt delete` (`Aliases: []string{"rm"}`)
- `wt ls` → `wt list` (`Aliases: []string{"ls"}`)
- `wt open` gained **no** alias (no obvious git/unix equivalent worth adding).

#### Scenario: an alias invokes the command identically
- **GIVEN** a repo
- **WHEN** the user runs `wt new --non-interactive --no-init`
- **THEN** a worktree is created exactly as `wt create` would.

## Design Decisions

### Additive rename over a hard flag break
**Decision**: every rename keeps the old flag as a permanent hidden deprecated
alias (`MarkDeprecated`); no old flag is ever removed.
**Why**: `wt` is invoked by fab-kit operators (e.g. `fab batch switch` calls
`wt create --non-interactive --reuse --worktree-name <name> <branch>`) and by
external shell scripts, all against the published binary. A hard rename would
break every such caller. Additive rename breaks nothing — deprecation warnings go
to stderr only, so machine-parsed stdout is untouched — while modernizing the
human-facing surface. Removal is a future move that needs an announced deprecation
window, not this change.
**Rejected**: hard rename (breaks external callers with no migration path);
leaving the awkward names (the surface reads machine-generated, and every day of
delay accretes more external scripts against the old names).
*Introduced by*: `260717-59u8-intuitive-flag-names`.

### `--no-*` real booleans over string-typed booleans
**Decision**: default-ON behaviors are toggled off with a real boolean
`--no-<thing>`, not a string `--<thing> true|false`.
**Why**: a string-typed boolean forces the user to type a value (`--worktree-init
false`) for what is conceptually a switch, and cobra bools already support
`--flag=false` for the rare explicit-false case. `--no-*` is the idiomatic Go/CLI
negation.
**Rejected**: keeping the string booleans (needless typing, reads as
machine-generated); splitting the `--delete-branch` tri-state into two bools
(messier than one string when `auto` is a genuine third state — so `--branch`
stays a string).
*Introduced by*: `260717-59u8-intuitive-flag-names`.

### `--open` gets no `NoOptDefVal` (positional footgun)
**Decision**: `wt create --open` requires an explicit value.
**Why**: `wt create` takes a positional `[branch]`. A `NoOptDefVal` would let bare
`--open code` silently parse `code` as the branch positional instead of the open
mode — a silent, hard-to-debug footgun.
**Rejected**: a `NoOptDefVal` of `prompt` (the positional-swallow footgun).
*Introduced by*: `260717-59u8-intuitive-flag-names`.

## Cross-references

- Spec doc: [`docs/specs/cli-surface.md`](../../specs/cli-surface.md) — the
  per-subcommand flag tables where each new primary name is listed and each old
  name is enumerated as a deprecated alias; the command aliases in the headers.
- Sibling memory: [`update-command-contract`](/wt-cli/update-command-contract.md)
  — the `--skip-brew-update` visible contract flag / `--no-brew-update` visible
  alias (the frozen-contract-flag carve-out instance) and the brew-handling safety
  contract.
- Sibling memory: [`toolkit-standards-conformance`](/wt-cli/toolkit-standards-conformance.md)
  — the shll `update` standard that freezes `--skip-brew-update` as a `--help`
  substring, the authority behind the carve-out above.
- Sibling memory: [`go-command-contract`](/wt-cli/go-command-contract.md) — the
  `wt open --select` composition flag (formerly `--go`, now a deprecated alias)
  and the `--app` `-a` short.
- Sibling memory: [`create-branch-semantics`](/wt-cli/create-branch-semantics.md)
  — the `--name`/`-n`, `--open`/`-o`, `--no-init` renames and the `wt new` alias
  on `wt create`; the `--reuse requires --name` message.
- Sibling memory: [`idle-staleness-contract`](/wt-cli/idle-staleness-contract.md)
  — the `--all`/`-a` (and `--branch`, `--no-remote`) renames referenced in the
  `--stale` mutex/composition text, and the `wt rm` alias on `wt delete`.
- Sibling memory: [`delete-dry-run-contract`](/wt-cli/delete-dry-run-contract.md)
  — the `--dry-run` long-only / no-short flag on `wt delete` (recorded in the
  short-flags rule above) and its preview contract.
- Sibling memory: [`create-output-phases`](/wt-cli/create-output-phases.md) — the
  canonical stdout=machine / stderr=human stream-discipline contract that the
  deprecation-warning-on-stderr rule honors.
- Source: `src/cmd/wt/create.go` (`--name`/`-n`, `--open`/`-o`, `--no-init` +
  deprecated `--worktree-name`/`--worktree-open`/`--worktree-init`, `Aliases:
  ["new"]`), `src/cmd/wt/delete.go` (`--all`/`-a`, `--branch`, `--no-remote` +
  deprecated `--delete-all`/`--delete-branch`/`--delete-remote`, `Aliases:
  ["rm"]`; `--dry-run` long-only, added by `260717-p5m9`),
  `src/cmd/wt/open.go` (`--select` + deprecated `--go`, `--app`/`-a`),
  `src/cmd/wt/list.go` (`Aliases: ["ls"]`), `src/cmd/wt/update.go`
  (both `--skip-brew-update` contract flag and `--no-brew-update` alias visible on
  one bool, no `MarkDeprecated` — the carve-out instance), `src/cmd/wt/init.go`
  (Short-text sharpening, no flag change).
- Tests: `src/cmd/wt/create_test.go`, `delete_test.go`, `open_test.go`,
  `list_test.go`, `update_test.go`, `init_test.go`, `integration_test.go` — cover
  the new names/shorts/aliases plus the old names still working with a stderr
  deprecation warning and hidden from `--help`; the ~160 pre-existing old-flag
  invocations across the suite double as living back-compat coverage.
- Constitution: Principle II (Cobra Command Surface — long-form names, shorts only
  where they aid common interactive use, `RunE`), III (Typed Exit Codes —
  unaffected; no exit-code changes), V (Internal Package Boundary — all changes are
  `cmd/` flag parsing / cobra metadata; no `internal/worktree` logic touched),
  VI (`--non-interactive` deliberately unchanged everywhere).
