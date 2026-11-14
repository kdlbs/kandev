---
id: coordinator-manual-task-visibility-design
title: Manual session task lifecycle visibility design
status: current
system: coordinator
owners:
  - kandev
created: 2026-10-25
last_updated: 2026-10-25
requirements:
  - REQ-COORDINATOR-MANUAL-VISIBILITY-001
---

# Manual session task lifecycle visibility System Design

## Boundary

This capability follows
[ADR-2026-10-14-session-bound-task-visibility](../../../decisions/2026-10-14-session-bound-task-visibility.md).
It applies to a trusted, explicitly granted manual session. The managed
coordinator copilot keeps its separate origin, mode, and tool policy in
[copilot](copilot.md#principal-and-mode).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-MANUAL-VISIBILITY-001` | [Grant and dispatch](#grant-and-dispatch), [Lifecycle evidence](#lifecycle-evidence) |

## Grant and dispatch

`task/models.TaskVisibilityGrant` is stored in task-session metadata by a
trusted backend operation. It binds version, task ID, session ID, and workspace
ID. `HasTaskVisibilityGrant` accepts only an exact version-1 match on a manual
task and its session. MCP profile discovery and each dispatch read the persisted
session again. The hidden request principal comes from that session; payload
fields cannot select another caller or workspace. The authorized surface adds
`get_task_visibility_kandev` only, with a target task ID. Missing or mismatched
grant evidence fails closed. Revocation clears the metadata for later reads.

## Lifecycle evidence

`task/service.GetTaskVisibilityForRequest` reads a same-workspace snapshot of
the task and `TaskLifecycleReceipt`. The compact response includes the fields
defined by `TaskVisibility`; it omits relations and content. An unavailable
target or absent receipt has no synthetic success response. The receipt has no
foreign key to the task row, so deletion can retain a compact disposition.
Repository transition writes update task and receipt atomically; refused,
rolled-back, and replayed transitions do not advance its revision. SQLite and
PostgreSQL use the same behavior matrix, with PostgreSQL verification required
before final task acceptance.
