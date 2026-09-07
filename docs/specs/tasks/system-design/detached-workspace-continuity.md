---
status: current
system: tasks
requirements:
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
---

# Detached Workspace Continuity System Design

## Purpose and boundaries

Task lifecycle owns `task_environments` and task-owned worktree lifetime. This
design makes hierarchy detachment, shared-workspace stewardship, creation-time
attachment, and destructive cleanup use one durable ownership contract. The
workspace system continues to own workspace and repository configuration; it
does not own task-environment cleanup authority.

The design extends
[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
and applies the fail-closed rule from
[ADR-0009](../../../decisions/0009-fail-closed-gc-semantics.md). The ownership
generation and transaction rule is recorded in
[ADR-2026-09-04-generation-fenced-task-environment-ownership](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001` | [Ownership model](#ownership-model), [Atomic detachment](#atomic-detachment), [Cleanup fencing](#cleanup-fencing), [Restart and recovery](#restart-and-recovery) |
| `REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002` | [Canonical creation attachment](#canonical-creation-attachment), [Concurrency and failure behavior](#concurrency-and-failure-behavior) |

## Ownership model

`task_workspace_groups.owner_task_id` identifies the current workspace steward.
Exactly one active membership row for that group has role `owner`; all other
active memberships have role `member`. The group gains an
`ownership_generation` integer that starts at `1` and increments whenever the
steward changes.

`task_environments.task_id` remains the only physical environment owner. The
environment gains its own `ownership_generation`, also starting at `1` and
incremented by every successful owner transfer. A cleanup snapshot identifies
an environment by ID, owner task ID, and ownership generation. An environment
ID alone is never sufficient authority for a destructive operation.

The workspace group can remain shared after detachment. Stewardship may move to
another active member later; this does not copy the workspace or invalidate
sessions that already reference the canonical environment.

## Components and responsibilities

- `internal/task/repository/sqlite` owns the cross-table transaction for
  detachment, workspace-group stewardship, environment ownership generations,
  and task cleanup barriers. It supports SQLite and PostgreSQL through the
  existing dialect layer.
- `internal/task/service.Service.DetachTask` invokes only the transactional
  repository operation, then reloads and publishes the committed task state.
- `internal/task/service.Service.CreateTask` owns required creation-time
  workspace attachment before `task.created` is published or a launchable
  result is returned. A narrow injected coordinator reuses
  `HandoffService` policy resolution and membership behavior without making API
  handlers responsible for correctness.
- `internal/task/service` task-resource cleanup validates environment ownership
  and generation before teardown. Ownership transfers reject while the source
  task has an active cleanup barrier.
- `internal/task/service.HandoffService` group cleanup claims the current group
  generation before external cleanup and completes the status transition only
  for that same generation.

## Atomic detachment

The repository exposes one detachment operation that performs these steps in a
single database transaction:

1. Lock the child and former-parent task rows in deterministic task-ID order.
2. Reject when either task has a prepared, pending, running, or retry-wait
   resource-cleanup job.
3. Lock the child's active workspace-group row and its materialized environment
   row when present.
4. Clear `tasks.parent_id` and normalize `inherit_parent` to `shared_group`.
5. Set the group's steward and owner membership role to the child, demote the
   previous owner membership to `member`, and increment the group generation.
6. Transfer the materialized environment to the child and increment the
   environment generation.
7. Commit once. Any missing, ambiguous, archived, or concurrently changed row
   rolls the complete operation back.

Detaching an already-root task is an idempotent read of the current state. It
does not increment either generation.

SQLite's writer transaction provides serialization. PostgreSQL uses explicit
row locks in the order above. Archive and delete preparation already reserve a
task cleanup barrier; detachment uses the same barrier states so either the
transfer commits before cleanup inventory is captured or detachment is rejected
after cleanup wins.

## Canonical creation attachment

Workspace policy resolution and attachment become part of the task service's
required synchronous create sequence. REST, WebSocket, MCP, plugin Host API,
workflow child creation, and other internal callers pass parent and explicit
workspace-policy inputs through `CreateTask`; no caller performs an independent
post-create membership write.

The service resolves parent defaults before building task metadata. After the
task row is inserted, it records the workspace-group membership and sequential
blocker attachment before publishing `task.created` or returning a Created
outcome. If required attachment fails, the existing partial-task rollback path
removes the child and its dependent rows. Deduplicated external-ID outcomes do
not reapply workspace policy.

HTTP and MCP handlers stop calling `AttachWorkspacePolicy` directly. The legacy
raw WebSocket create path and internal `CreateChildTask` path thereby gain the
same attachment behavior rather than bypassing it.

## Cleanup fencing

Task-resource cleanup snapshots persist the environment owner task ID and
ownership generation captured after the cleanup barrier is reserved. Before
environment or associated worktree teardown, the repository compares the
snapshot with the live environment. A missing row is idempotent success only
when the resource is already positively known absent. An owner or generation
mismatch makes the destructive portion a safe no-op and records that the
snapshot was superseded.

Every environment-owner transfer uses a guarded repository method. It checks
the expected owner and generation, verifies that the source owner has no active
cleanup barrier, and increments the generation in the same transaction as the
owner change. The existing delete and cascade ownership-transfer paths use this
method; no unguarded `UPDATE task_environments SET task_id = ...` remains.

Workspace-group cleanup changes from check-then-delete to a generation claim.
The claim atomically verifies the group generation, ownership flags, cleanup
policy, and absence of active members before setting `cleanup_pending` for that
generation. Stewardship transfer is allowed only from `active` or retryable
`cleanup_failed` state with no active cleanup claim. Completion and retry writes
use the claimed generation; stale completions cannot mark a replacement group
cleaned or failed.

Filesystem and provider cleaners retain their existing managed-root and exact
resource-identity guards. Generation fencing is an additional authorization
condition, not a replacement for those checks.

## Concurrency and failure behavior

- Detach loses to an already-admitted parent or child cleanup and returns a
  conflict without clearing `parent_id`.
- Cleanup inventory captured after detachment cannot select the former parent's
  transferred environment because the environment owner has already changed.
- A delayed cleanup or ownership retry with an old generation is treated as
  superseded and performs no destructive work.
- Concurrent sibling detaches serialize on the shared group. Each committed
  transfer leaves one active steward and increments generations once.
- Creation returns success only after membership attachment. A concurrent
  parent lifecycle barrier causes creation to roll back instead of publishing a
  child that can launch without its workspace relationship.
- Repository or event-publication failures never compensate by deleting a
  workspace whose current ownership cannot be proven.

## Persistence and migration

Add `ownership_generation INTEGER NOT NULL DEFAULT 1` to
`task_workspace_groups` and `task_environments`. Fresh schemas include the
columns and replayable migrations add them to existing SQLite and PostgreSQL
databases. Existing rows retain their current owner at generation `1`; no
filesystem mutation occurs during migration.

All selects, inserts, model projections, test fixtures, and table-rebuild copy
lists that cover these tables include the new columns. Schema tests use the real
task/Office repository initialization order and cover fresh and replayed startup
for both dialects, following ADR-0027.

## Restart and recovery

Ownership generations live in the database and are included in durable cleanup
snapshots. On restart, cleanup workers re-read the current owner and generation
before acting. A snapshot prepared before a stewardship transfer cannot regain
authority after restart. The detached task's sessions continue resolving the
same materialized environment ID from the workspace group and their persisted
`task_environment_id` values.

## Observability

Structured logs identify the task, group, environment, expected owner and
generation, current owner and generation, and the resulting disposition for
ownership transfers and rejected cleanup. Logs do not include workspace file
contents, credentials, or restore secrets.
