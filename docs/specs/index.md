# Specifications Index

> **Specs are pre-implementation artifacts** — what you *planned*. They capture conceptual design
> intent, high-level decisions, and the "why" behind features. Specs are human-curated,
> flat in structure, and deliberately size-controlled for quick reading.
>
> Contrast with [`docs/memory/index.md`](../memory/index.md): memory files are *post-implementation* —
> what actually happened. Memory files are the authoritative source of truth for system behavior,
> maintained by `/fab-continue` (hydrate).
>
> **Ownership**: Specs are written and maintained by humans. No automated tooling creates or
> enforces structure here — organize files however makes sense for your project.

| Spec | Description |
|------|-------------|
| [`cli-surface.md`](cli-surface.md) | Per-subcommand reference: flags, positional args, exit codes for `wt create / list / open / delete / init / shell-init`. |
| [`worktree-layout.md`](worktree-layout.md) | Filesystem layout (`<repo>.worktrees/`), random adjective-noun naming, `--name` override, branch ↔ worktree relationship. |
| [`init-protocol.md`](init-protocol.md) | Init script lookup contract: `WORKTREE_INIT_SCRIPT` env var, `"fab sync"` default, command-vs-path detection, working-dir resolution, graceful skip behavior. |
| [`launcher-contract.md`](launcher-contract.md) | Subprocess-delegation contract for `wt open`: `WT_CD_FILE` / `WT_WRAPPER` env vars, exit-code semantics, stability guarantees. Used by external callers (e.g. `hop`) that delegate launching to `wt`. |
| [`build-and-release.md`](build-and-release.md) | Local build (`just build`/`scripts/build.sh`), tag-driven release flow, cross-compile matrix, Homebrew tap update, pre-release setup. |
