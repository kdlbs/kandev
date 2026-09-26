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

Every lifecycle envelope carries immutable `task_id`, `lifecycle_epoch`, monotonic
`task_revision`, durable `queue_sequence`, `event_id`, terminal/tombstone state,
and canonical payload. Task/session events persist in
`task_lifecycle_event_outbox`; session rows add `session_id`, `session_event_id`,
and terminal session payload. A task epoch allocates one unique queue sequence to
each row. Delivery locks its watermark cursor
`(task_revision,queue_sequence)`, publishes only the exact next lexicographic
successor, then advances the cursor; crash/retry leaves that row pending and
serializes workers per epoch. The gateway rejects malformed envelopes, discards
older cursors, and suppresses all events after the accepted task tombstone.
Delayed session events never resurrect a deleted session or tombstoned task.

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

Client task projections retain accepted lifecycle epoch, revision, queue sequence,
event ID, and tombstone state alongside `Task.updated_at`. Within an epoch,
`(task_revision, queue_sequence)` owns event order; a tombstone blocks later
nonterminal task/session updates. Older cursors are discarded. `TaskStatusSummary`
revision still orders only the bounded status summary.

Workflow snapshot requests record the projection at request start. If a live event
advances state before the response completes, merge keeps the newer envelope;
snapshot responses cannot resurrect a tombstoned task. Independent placement,
status-summary, executor, autopilot, and auto-start-error merge rules remain.
The sidebar compares lifecycle revision/epoch before task timestamps and summary
revisions, selecting each projection's freshest valid value.

When status-summary revisions are equal, workflow snapshots may update only
independent summary fields such as `queued_prompt_count`; they must preserve the
freshest lifecycle epoch/revision/tombstone. The active `kanban` hydration and
workflow snapshot paths apply this same ordering before writing either projection.

## Control flow

1. The backend persists a task-state change.
2. The backend publishes `task.state_changed` before the running-session event.
3. The WebSocket handler updates active and multi-workflow projections.
4. A delayed workflow snapshot response reaches the client.
5. The snapshot merge compares lifecycle epoch/revision/tombstone.
6. The merge keeps the newer valid lifecycle envelope and independent summaries.
7. The sidebar aggregator selects the newest task-level projection.
8. `applyView` groups the task by that persisted state.

## Failure and recovery

A failed snapshot request does not clear the current task projection. The
existing foreground refresh can retry. An invalid or missing lifecycle envelope
is discarded; timestamps and status-summary revision cannot substitute for it.

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
