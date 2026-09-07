# ADR-2026-09-04-generation-fenced-task-environment-ownership: Fence Task Environment Ownership by Generation

**Status:** accepted
**Date:** 2026-09-04
**Area:** backend

## Context

ADR-2026-08-08 makes `task_environments.task_id` the canonical owner of a task
workspace and permits shared-environment ownership transfer. Subtask detachment
currently changes only `tasks.parent_id` and workspace-mode metadata. The shared
workspace group and materialized environment remain owned by the former parent.

Archive and delete cleanup persist resource snapshots and can retry after a
restart. An environment ID or path does not prove that the task named by an old
snapshot still owns that resource. A delayed cleanup can therefore act after
ownership has moved, while handler-specific creation attachment can leave some
child tasks outside the workspace group entirely.

## Decision

Persist a monotonically increasing ownership generation on every task
environment and shared workspace group. Treat the tuple of resource ID, owner
task ID, and ownership generation as the authority for ownership transfer and
destructive cleanup.

Detachment of a task using a shared workspace changes hierarchy, workspace-group
stewardship, membership roles, environment owner, and both ownership generations
in one database transaction. The transaction uses the same task cleanup barrier
as archive and delete so detachment either precedes cleanup inventory or is
rejected after cleanup has been admitted.

Every owner transfer is a compare-and-swap against the expected owner and
generation and increments the generation. Cleanup claims the expected
generation for the duration of external teardown; transfers reject an active
claim. Stale cleanup, retry, rollback, and status updates fail closed when the
owner or generation differs.

Workspace-policy attachment is required synchronous task-creation work owned by
the task service. API handlers and internal callers do not implement separate
membership attachment sequences.

## Consequences

- Parent lifecycle operations cannot destroy a detached child's current
  environment after stewardship transfers.
- Durable cleanup snapshots remain safe across retries and backend restarts.
- SQLite and PostgreSQL require replayable schema changes and dialect-specific
  concurrency tests.
- Ownership-transfer callers must carry expected owner and generation instead
  of issuing unguarded owner updates.
- Group cleanup needs a generation-scoped claim around filesystem or provider
  operations.
- Task creation does not report a child as complete until required workspace
  membership exists.

## Alternatives Considered

1. **Preserve group membership without changing ownership.** Rejected because
   task cleanup authority still belongs to the former parent.
2. **Copy the physical workspace during detachment.** Rejected because it cannot
   atomically move active sessions, is expensive for multi-repository tasks, and
   changes the established shared-workspace behavior.
3. **Check the current owner immediately before cleanup without a generation.**
   Rejected because ownership can change after the check and before the external
   destructive operation.
4. **Serialize with in-process mutexes.** Rejected because process restart and
   PostgreSQL multi-connection execution are durable concurrency boundaries.
5. **Keep attachment in each API handler.** Rejected because WebSocket, plugin,
   workflow, and future internal task creation can bypass handler-owned logic.

