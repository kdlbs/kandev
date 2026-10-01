---
status: current
system: tasks
created: 2026-10-01
requirements:
  - REQ-TASKS-WORKFLOW-STEP-ORDERING-001
  - REQ-TASKS-COMPLETION-001
---

# Workflow Step Ordering System Design

## Purpose and requirement mapping

The task system's workflow repository owns the positional transaction. This
design specifies the approved correction to ordinary step reorder; it does not
change whole-row content-save semantics or the version-fenced Host command.

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STEP-ORDERING-001` | Authorization and entry, Positional transaction, Failure and verification |
| `REQ-TASKS-COMPLETION-001` (`AC-TASKS-COMPLETION-001.4`) | Positional transaction |

## Authorization and entry

`internal/workflow/controller.Controller.ReorderSteps` retains
`AuthorizeWorkflow`, `EnsureWorkflowMutable`, and `ValidateStepOrder`, including
session-target predecessor restrictions. `service.Service.ReorderSteps` retains
its independent workflow authorization and delegates persistence to a new
`repository.Repository.ReorderSteps` method instead of using `UpdateStep`.

Requests still carry `workflow_id` and `step_ids`. No route, response shape,
event contract, or frontend layout changes. Foreign or missing step membership
must retain a sanitized not-visible/not-found classification at the service
boundary; do not disclose the owning workflow. Invalid order errors follow
existing request failure handling without introducing new public status codes.

## Positional transaction

The repository accepts workflow identity and ordered IDs, never caller-owned
step snapshots. In one writer transaction, validate uniqueness, the complete
membership of the named workflow, and each requested ID's membership. Close
membership query rows before subsequent writes so single-connection pools do
not deadlock. Empty ordering is accepted only for empty membership.

Update only `position` and `updated_at`, scoping every write by both step ID and
workflow ID and requiring exactly one affected row. Use one UTC timestamp for
all positions. A query, write, affected-row, or commit failure propagates after
rollback; never return success for a partial order. Bind placeholders through
the transaction's `Rebind` for SQLite and PostgreSQL.

Membership validation happens on the writer transaction rather than a replica
or separate read handle. Preserve transaction safety under simultaneous
reorders; competing operations either commit a complete order or fail without
partial writes. On PostgreSQL, lock the parent workflow with `SELECT ... FOR UPDATE` before
reading membership. This serializes reorders and blocks FK-backed step inserts
until commit. SQLite relies on its writer transaction; a competing writer can
reject an attempted read-to-write upgrade without partial changes. No new
cross-process lock service or schema is needed. Concurrent membership
mutations are not made atomic with settings Save by this work.

`ReorderStepsIfUnchanged` remains the existing version-fenced command. Reuse
its positional-update pattern without converting ordinary reorder into a
whole-step version conflict: a concurrent content edit must survive a
successful ordinary reorder.

This satisfies completion-setting preservation because the positional write
never supplies `complete_task_on_enter`, prompt, events, profile, session-target,
or other content columns. `order_revision` and task positions remain untouched.

## Failure and verification

Use real file-backed SQLite with separate read and writer handles. Hold the
writer pool's sole connection and use `DBStats.WaitCount` as the acquisition
barrier; save content through the independent handle, release the connection,
and assert both reorder success and preserved content. A test-only trigger
rejects the second position write and proves rollback of positions/timestamps.

Repository tests cover exact membership, duplicate/foreign/missing/omitted IDs,
empty membership, and complete successful ordering. Controller and service
tests retain authorization, immutable synced workflows, and invalid session
target ordering. Real PostgreSQL tests use the existing isolated-schema helper
and `KANDEV_TEST_POSTGRES_DSN`, covering changed SQL, rollback, and multi-connection
concurrent edits/reorders. Tests are env-gated when no disposable PostgreSQL is supplied.

Existing reorder logs describe the outcome; no new telemetry, retry loop,
migration, or public configuration is needed.

## Related sources

- [Task completion design](task-completion.md)
- [Settings Save coordination decision](../../../decisions/0046-settings-route-save-coordinator.md)
- [Implementation plan](../../../plans/workflow-step-reorder-atomicity/plan.md)
