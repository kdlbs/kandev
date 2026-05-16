# 0009: Fail-closed GC semantics for filesystem and container cleanup

**Status:** accepted
**Date:** 2026-05-16
**Area:** backend

Spec: [office-gc-worktree-safety](../specs/office-gc-worktree-safety/spec.md)

## Context

Kandev's office service runs a background garbage collector that, every three hours, walks `~/.kandev/tasks/` and removes "orphaned" worktree directories. It also walks kandev-labeled Docker containers and removes ones whose tasks are gone or terminal. The intent was good — agents crash, leave state behind, and disks fill. The implementation was not.

The original `sweepWorktrees` passed each directory's on-disk name (a semantic slug like `locstat-github-actio_5gz`, produced by `worktree.SemanticWorktreeName(title, suffix)`) into `GetTaskBasicInfo`, which keys on `tasks.id` (a UUID). Every lookup missed. An error or missing row was treated as "orphan → delete." On the first sweep — which runs immediately at startup, not on a delay — `os.RemoveAll` ran against every directory under the base. A user checked out `feature/orchestrate` on a machine carrying a production DB and lost 307 active worktrees. Every in-progress task's working copy was gone before the user noticed.

The container sweep had the same fail-open shape, scoped to kandev-labeled containers: any error reading the task row was classified as "orphan → remove." The blast radius was smaller (only containers we manage, only when the lookup actually erred), but the anti-pattern was identical.

The bug was not "wrong table." It was "destructive action on a negative signal." A correctly-keyed lookup that fails for any reason — DB closed, schema mismatch, transient I/O — would have produced the same outcome on the same code path.

## Decision

**GC and cleanup paths in kandev never perform destructive actions on a negative or uncertain signal. Deletion requires a positive "this is not tracked" signal from an authoritative inventory query.**

Concretely:

1. **Authoritative inventory first.** Each sweep starts by querying the source of truth for what is alive: `task_session_worktrees.worktree_path` for worktrees, `tasks` for containers. Anything not in the inventory is a *candidate* for deletion, not yet condemned.

2. **Fail-closed on error.** A failure to read the inventory — query error, closed pool, missing dependency — aborts the sweep with a logged warning. The GC never proceeds with an empty or partial inventory.

3. **Positive signal required.** For containers, "task missing" is distinguished from "lookup errored" by a sentinel error (`sqlite.ErrTaskNotFound`). Only the sentinel permits removal. For worktrees, absence from the inventory is the signal, and the directory's mtime must additionally be older than a 24-hour grace period — covering the race where a worktree is created on disk before its DB row is inserted, plus operator-created scratch directories.

4. **Ancestor protection.** Worktree paths in multi-repo tasks have the shape `{base}/{taskDir}/{repoName}`. The sweep iterates `{base}` at depth-1, so the live-paths set is augmented with every ancestor of every live path. A `{taskDir}` whose `{repoName}` child is live is never deleted.

5. **Path normalization at the boundary.** Live paths are run through `filepath.Abs(filepath.Clean(...))` before comparison so trailing slashes, `..` components, and home-dir variants do not produce false orphans.

## Consequences

- The GC will sometimes leave true orphans alive when a DB read transiently fails. We accept this. The cost of holding onto a stale directory for one more 3-hour cycle is bounded; the cost of deleting a live one is not.
- The container sweep's "no task row" path still removes the container, but only via the sentinel. Other callers of `GetTaskExecutionFields` are unaffected — the sentinel is a wrapped error, not a contract change.
- New cleanup code — future scheduled GC, manual purge endpoints, retention policies — must follow this model. Reviewers should reject patterns where deletion fires on `err != nil`, missing rows, or "not found" inferred without a typed signal.
- The worktree manager exposes `ListActiveWorktreePaths(ctx)` as a thin pass-through to the store. This is a cross-package dependency (`office/infra` imports a method satisfied by `worktree.Manager`), wired by the main process. The interface lives in `office/infra` so the dependency points outward from the consumer.

## Alternatives considered

- **Disable worktree GC entirely.** Tempting given how badly it failed, but defers the underlying disk-leak problem. The redesign is small enough to be worth shipping.
- **Tombstone files in each worktree directory.** Write a `.kandev-worktree` marker at creation; GC only deletes directories whose marker is old and whose DB row is absent. Eliminates the race window but requires changes to worktree creation. The 24h grace period covers the same race for a fraction of the cost.
- **Dry-run rollout flag.** Land deletes behind `KANDEV_OFFICE_GC_DELETE=true`, default log-only, flip after bake-in. Rejected by the user — the corrected algorithm is small, well-tested, and shipping it gated would extend the period during which orphans accumulate.

## References

- Spec: [`docs/specs/office-gc-worktree-safety/spec.md`](../specs/office-gc-worktree-safety/spec.md)
- Plan: [`docs/specs/office-gc-worktree-safety/plan.md`](../specs/office-gc-worktree-safety/plan.md)
- Implementation: `apps/backend/internal/office/infra/gc.go`, `apps/backend/internal/worktree/store.go`, `apps/backend/internal/office/repository/sqlite/tasks.go`
