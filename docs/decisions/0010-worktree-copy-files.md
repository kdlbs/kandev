# 0010: Worktree copy-files — per-repo, idempotent, host-local

**Status:** accepted
**Date:** 2026-05-19
**Area:** backend, frontend

## Context

Git worktrees don't carry gitignored files. The most painful case for kandev users is `.env` and friends — every new task creates a fresh worktree, and the agent's first step is usually `pnpm dev` or `make run`, which fails immediately because there's no `.env`. Users were copying files manually after each task creation. Issue #946 asked for a built-in solution, citing vibe-kanban's `copy_files` setting as prior art.

The feature has several axes of choice that aren't obvious from the issue text alone, and a couple are surprising enough that future maintainers will reasonably question them.

## Decision

**A per-repository, comma-separated list of paths/globs (`repositories.copy_files`). Copied from the source repo dir into each new worktree at task creation, in the Go process on the host. Target-exists check makes the copy idempotent.**

Concretely:

1. **Scope: per-repository, not per-task or per-workspace.** The set of secret files in a repo (`.env`, `.env.local`, `config/local.yml`) is a property of the repo, not of any individual task. Storing it once per repo means users configure it during workspace setup and never think about it again. A per-task field would push friction back onto every task creation, defeating the purpose. A per-workspace field would force every repo in the workspace to share the same list — wrong for polyglot workspaces.

2. **Wire format: comma-separated string of paths or `filepath.Glob` patterns.** Matches vibe-kanban exactly, which means users migrating between tools don't relearn syntax. Stdlib `filepath.Glob` supports `*`, `?`, `[...]` — enough for `.env`, `*.local`, `config/*.json`. **No `**`/doublestar in v1**: it's a single transitive dependency we don't need to take to ship the feature, and the literal-file-plus-directory-recursion path (`config/` recursively walks all files) covers the rest of the issue's examples. Adding doublestar later is additive.

3. **Trigger: at worktree creation, between `persistAndCacheWorktree` and `runWorktreeSetupScript`.** Setup scripts often read `.env` (e.g. `pnpm install` with `NODE_ENV`-gated postinstalls), so the copy must complete first. Placing it after `persistAndCacheWorktree` means a failed copy doesn't leave a phantom DB row — the worktree is already committed, and copy errors are recoverable.

4. **Host-local Go copy, not an agentctl RPC.** The worktree filesystem is on the host (today). The copy is `io.Copy` between two `*os.File`s under a path-traversal guard. No subprocess, no shell, no Windows `cp` issue, no agentctl round-trip.

5. **Idempotent: skip if target exists.** Re-running the copy on the same worktree (e.g. future "re-prepare worktree" actions, or session resume that re-enters this code path) is a no-op for already-copied files. The cost is that **updating `copy_files` in settings does not propagate to existing worktrees** — only future ones. The frontend helper text says so explicitly. The alternative — overwrite-on-update — would clobber files the agent had already mutated mid-task, which is worse.

6. **Missing files = warning, not error.** Acceptance criterion in the issue. A typo in `copy_files` should not block task creation; the agent failing later with a clearer "ENOENT: .env" is better than the worktree refusing to spawn.

7. **Path-traversal guard: canonicalize, then `filepath.Rel` under canonical root.** `EvalSymlinks(sourceDir)` is called once; all pattern joins and result paths use that canonical root, not the raw `sourceDir` argument. This matters because macOS resolves `/tmp` → `/private/tmp` and many users have `~/code` → `/data/code` style symlinks; without canonicalization, *every* match looks "outside source dir" via `filepath.Rel` and the feature silently no-ops. (Regression test: `TestCopy_SymlinkedSourceDir`.) Resolved matches outside the canonical root are still rejected — symlink traversal as an exfiltration vector is closed.

## Consequences

- **Remote executors** (`remote_docker`, `k8s`, planned) will need a different copy strategy — likely run the copy on the remote side after clone, or ship the bytes over agentctl. Today's worktree is host-local, so v1 is safe; the abstraction point is `worktree.Manager`, which already differs per executor type at a higher layer. When remote executor work begins, this ADR's "host-local" claim must be revisited.
- **Disk-fill DoS** is self-inflicted but unbounded — a user adding `**` or `node_modules/` would explode disk usage across many tasks. v1 has no size cap. If support tickets surface this, add a soft warning at >100 MB / >1000 files.
- **No UI feedback for warnings.** Missing files only surface in backend zap logs today. If users complain that typos are invisible, attach warnings to the worktree record (mirror `FetchWarning`/`FetchWarningDetail` already on `Worktree`) and render in the task UI.
- **The `worktree.RepositoryAdapter` indirection** stays — it maps `models.Repository` → `worktree.Repository` (a minimal struct exposing only `{ID, SetupScript, CleanupScript, CopyFiles}`). Adding more cross-cutting per-repo settings should go through this adapter, not by widening the worktree package's import surface.

## Alternatives considered

- **Extend `setup_script`.** Users could already write `cp /path/to/.env .` in setup. Rejected: requires every user to write shell, fights Windows (no `cp`), doesn't canonicalize source paths, and surfaces secrets in script output that gets streamed to the session UI. A dedicated field is purpose-built.
- **Per-task override on the task creation dialog.** Reasonable for one-off needs, but adds friction to the common path. Defer until a user asks.
- **Watch and re-copy on `copy_files` setting change.** Would make settings updates "live" for existing worktrees. Rejected: existing worktrees may have agent-modified copies, and silent re-copy risks data loss. The current design forces the user to re-create the task if they want the new files, which is the safe default.
- **JSON or YAML for the spec.** Considered for future expressiveness (`{src, dest, mode}` tuples). Rejected for v1: vibe-kanban's flat comma-separated format is proven, trivial to migrate from, and we have no use case yet for `dest` differing from `src`.
