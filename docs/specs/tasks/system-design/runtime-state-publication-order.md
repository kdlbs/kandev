---
status: current
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001
---

# Runtime Task-State Publication Order System Design

## Purpose and boundaries

The task system owns persisted task state and task-state publication. The web
client keeps several projections of each task for task details and task lists.

This design keeps these projections consistent across WebSocket and workflow
snapshot races. It does not create a second task-state authority in the client.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001` | [Publication contract](#publication-contract), [Client freshness contract](#client-freshness-contract), [Failure and recovery](#failure-and-recovery), [Responsive behavior](#responsive-behavior) |

## Publication contract

The backend follows
[ADR-2026-07-30](../../../decisions/2026-07-30-runtime-task-state-before-running-event.md).
It persists an eligible task as `IN_PROGRESS` before it publishes the owning
session as `RUNNING`.

The task event and session event use the same per-task publication queue. The
WebSocket gateway receives both event types through one ordered subscription.

The existing backend guards remain authoritative. They cover archive state,
Office ownership, terminal state, and concurrent session transitions.

## Client projections

The web client keeps two relevant projections:

- `kanban.tasks` supports the active workflow and open task surface.
- `kanbanMulti.snapshots` supplies tasks for the desktop sidebar and mobile
  task switcher.

`apps/web/lib/ws/handlers/tasks.ts` applies task events to both projections.
`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts` also refreshes each
workflow snapshot through HTTP.

`apps/web/components/task/task-session-sidebar-aggregate.ts` combines workflow
snapshots with the active workflow. Both task-list surfaces consume this shared
aggregation path before `applyView` groups tasks by state.

## Client freshness contract

Task-level freshness uses `Task.updated_at`, mapped to `KanbanTask.updatedAt`.
The newer task-level timestamp owns `state` and `updatedAt`.

`TaskStatusSummary.revision` orders only the bounded status summary. It does
not order task state or other task-level fields.

The workflow snapshot request records the task projection at request start. If
a live event advances task state before the response completes, the merge keeps
the newer state and task update time.

The snapshot merge retains its existing independent rules for workflow
placement, status summary, executor binding, autopilot, and auto-start errors.
This change does not make task state sticky. A snapshot with a newer task
update time remains authoritative.

The sidebar aggregator compares task update times before it compares status
summary revisions. It selects the freshest status summary independently, so a
new task state cannot erase newer summary data.

When status-summary revisions are equal, the workflow snapshot is treated as
the incoming reading. Snapshot responses can re-stamp `queued_prompt_count`
from a fresh queue read without incrementing the revision; the active summary
remains the fallback when the snapshot omits the summary.

The active `kanban` hydration path already rejects an older task by task update
time. The workflow snapshot path must apply the same task-state ordering before
it writes `kanbanMulti.snapshots`.

## Control flow

1. The backend persists a task-state change.
2. The backend publishes `task.state_changed` before the running-session event.
3. The WebSocket handler updates the active and multi-workflow projections.
4. A delayed workflow snapshot response reaches the client.
5. The snapshot merge compares task update times.
6. The merge keeps the newer state and task update time.
7. The sidebar aggregator selects the newest task-level projection.
8. `applyView` groups the task by that persisted state.

## Failure and recovery

A failed snapshot request does not clear the current task projection. The
existing foreground refresh can retry the request.

An invalid or missing task update time has the current fallback behavior. The
implementation must not use status-summary revision as a substitute task clock.

The existing workspace generation guard discards responses from an earlier
workspace context.

## Responsive behavior

The change only normalizes shared task data. It does not change layout,
navigation, scrolling, safe areas, pointer behavior, or touch targets.

The nearest mobile surface is
`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`. Its
`MobileTaskList` uses the shared aggregation result and `applyView` contract.

Targeted unit tests cover the shared data path. New mobile Playwright coverage
is not required for this state-only change.

## Tests

`use-all-workflow-snapshots-inflight.test.ts` uses a delayed snapshot response.
The test applies a newer live task state before it resolves the old response.

`task-session-sidebar-aggregate.test.ts` covers equal status-summary revisions
with different task update times and preserves the snapshot's re-stamped queue
count. It also covers equal task timestamps and keeps an independently newer
status summary while it selects the newer task state.

The existing clarification Playwright scenario proves that State grouping
moves a running task without a reload.

## Related decisions

- [Publish Task State Before Running Session State](../../../decisions/2026-07-30-runtime-task-state-before-running-event.md)
