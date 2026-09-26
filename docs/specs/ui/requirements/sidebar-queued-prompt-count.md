---
status: active
system: ui
created: 2026-08-04
owners:
  - kandev
---
# Sidebar Queued Prompt Count Badge Requirements

## Overview

A task with en-queued prompts looks identical to an idle one in the left task view: nothing distinguishes "the agent is busy and three follow-ups are waiting" from "nothing is waiting". Users bounce between the sidebar and each task's queue panel to find where their typed work is stuck. A compact per-row count on the metadata line (the same line that already carries the last update time and PR number) makes queued work visible at a glance, for tasks and subtasks alike.

## Requirements

### REQ-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001: Sidebar Queued Prompt Count Badge

**Intent:** A task with en-queued prompts looks identical to an idle one in the left task view: nothing distinguishes "the agent is busy and three follow-ups are waiting" from "nothing is waiting". Users bounce between the sidebar and each task's queue panel to find where their typed work is stuck. A compact per-row count on the metadata line (the same line that already carries the last update time and PR number) makes queued work visible at a glance, for tasks and subtasks alike.

#### Acceptance criteria

- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.1:** Every task row and subtask row in the left task view (`TaskSessionSidebar`) shows, on its metadata line, a badge with a mail icon followed by the number of prompts currently en-queued for that task.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.2:** The count covers all of the task's sessions. It counts persisted queue entries whose `task_id` is the task, excluding rows already reserved for in-flight delivery (the same pending semantics as `message.queue.get`).
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.3:** A count of `0` renders no badge: the metadata line is unchanged from today for tasks with nothing queued.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.4:** The badge is display-only: it is not a button, has no hover dependency, and never pushes the row to a second line.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.5:** **Initial load:** every task list/snapshot endpoint (HTTP list, workspace list, workflow/workspace snapshot, WS list, single task) carries the count in the task's status summary (`status_summary.queued_prompt_count`). The count is computed fresh from the queue table at assembly time so the sidebar is correct on first paint regardless of projector state.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.6:** **Live updates:** the task status summary projector subscribes to the internal `message.queue.status_changed` event, recomputes the affected task's pending count, and publishes the existing workspace-wide `task.status_summary.updated` broadcast. The sidebar therefore updates for every task in the workspace without any new per-session subscription.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.7:** **Queue panel payload:** the WebSocket `message.queue.status_changed` event (session-scoped, unchanged routing) gains an informational `task_id` field. Its `entries` content remains session-scoped; it is never broadcast workspace-wide.
- **AC-UI-SIDEBAR-QUEUED-PROMPT-COUNT-001.8:** **Lifecycle purge:** any path that empties a task's queue outside ordinary enqueue/dequeue/cancel (task archive, task delete, workspace cascade archive or delete) must still drive the live count path. After the queue rows are gone, the system publishes `message.queue.status_changed` for the task (with `task_id` set) so the projector recomputes pending = 0 and broadcasts `task.status_summary.updated`. The badge disappears on every live client without requiring a full list reload.

## Migrated source detail

## Why

A task with en-queued prompts looks identical to an idle one in the left
task view: nothing distinguishes "the agent is busy and three follow-ups are
waiting" from "nothing is waiting". Users bounce between the sidebar and each
task's queue panel to find where their typed work is stuck. A compact per-row
count on the metadata line (the same line that already carries the last
update time and PR number) makes queued work visible at a glance, for tasks
and subtasks alike.

## What

### The badge

- Every task row and subtask row in the left task view (`TaskSessionSidebar`)
  shows, on its metadata line, a badge with a mail icon followed by the number
  of prompts currently en-queued for that task.
- The count covers all of the task's sessions. It counts persisted queue
  entries whose `task_id` is the task, excluding rows already reserved for
  in-flight delivery (the same pending semantics as `message.queue.get`).
- A count of `0` renders no badge: the metadata line is unchanged from today
  for tasks with nothing queued.
- The badge is display-only: it is not a button, has no hover dependency, and
  never pushes the row to a second line.

### Data path

- **Initial load:** every task list/snapshot endpoint (HTTP list, workspace
  list, workflow/workspace snapshot, WS list, single task) carries the count in
  the task's status summary (`status_summary.queued_prompt_count`). The count
  is computed fresh from the queue table at assembly time so the sidebar is
  correct on first paint regardless of projector state.
- **Live updates:** the task status summary projector subscribes to the
  internal `message.queue.status_changed` event, recomputes the affected
  task's pending count, and publishes the existing workspace-wide
  `task.status_summary.updated` broadcast. The sidebar therefore updates for
  every task in the workspace without any new per-session subscription.
- **Queue panel payload:** the WebSocket `message.queue.status_changed` event
  (session-scoped, unchanged routing) gains an informational `task_id` field.
  Its `entries` content remains session-scoped; it is never broadcast
  workspace-wide.
- **Lifecycle purge:** any path that empties a task's queue outside ordinary
  enqueue/dequeue/cancel (task archive, task delete, workspace cascade archive
  or delete) must still drive the live count path. After the queue rows are
  gone, the system publishes `message.queue.status_changed` for the task
  (with `task_id` set) so the projector recomputes pending = 0 and broadcasts
  `task.status_summary.updated`. The badge disappears on every live client
  without requiring a full list reload.
- **Session delete:** deleting a session cancels/purges that session's pending
  queue entries and publishes the same queue-status event for the owning task
  so orphaned session rows cannot keep inflating the task badge.
- **Deleted task:** once the task row is removed, the frontend drops the task
  from every sidebar source (`kanban`, multi-workflow snapshots, and
  `sidebarArchivedTasks`). A later summary/count event for a deleted task id
  is a no-op; it never resurrects the row.

## API Surface

No new actions or endpoints. Payload additions only:

```text
task status summary (HTTP DTO, WS task.status_summary.updated)
  status_summary.queued_prompt_count: int   // omitted when 0

message.queue.status_changed (WebSocket, session-scoped as today)
  task_id: string                            // informational addition
```

## Data Model

No table migration. The count derives from the existing `queued_messages`
table (`task_id` column, written at insert from the session's owning task).
Pending semantics match `messagequeue.Service.GetStatus`: a row carrying
`lifecycle_reserved_in_flight` metadata is excluded; durable lifecycle rows
not yet reserved remain pending and count. The persisted
`task_status_summaries` payload gains `queued_prompt_count` through the
existing `SemanticJSON` envelope; old rows decode with the field absent
(0).

## State and Concurrency

- The projector refreshes the count only when the queue event's computed count
  differs from its current value; unchanged counts do not bump the summary
  revision or republish.
- The projector restores the last persisted count on restart and corrects it on
  the next queue event. List/snapshot assembly always reads the count fresh,
  so a missed event or projector startup ordering cannot leave a stale badge on
  initial load.
- A queue mutation and a list assembly racing read the same queue table; the
  list response may carry a count from either side of the mutation, both
  authoritative snapshots.
- Reserved-in-flight rows are excluded exactly as in `GetStatus`; a prompt
  already handed to a dispatch attempt is not "queued".

## Responsive and Mobile Behavior

- The badge renders inside the existing metadata row on both desktop and
  mobile sidebar layouts; it is a static text+icon pair, so it adds no hit
  target, no hover behavior, and no scroll owner.
- The row's ellipsis/overflow behavior is unchanged: the badge is inline-flex
  and shrink-safe, so narrow viewports truncate the title rather than wrap the
  badge.
- The accessible name is localized; the visible text is only the count.

## Failure Modes

- **Queue event missed (startup ordering, projector down):** initial list
  loads still show the correct count (fresh assembly); the next queue event
  repairs the live path.
- **Session→task resolution fails at a publish site:** the event is published
  without `task_id`; the projector skips it and the next successful event (or
  the next list load) reconciles.
- **Summary row missing and count is 0:** nothing to show; the row behaves as
  today.
- **Summary row missing and count > 0 (hydration failed):** the badge is
  absent on that row until a summary exists; the queue panel remains the
  fallback surface. Same degradation class as other summary-only fields.
- **Count query failure at assembly:** the list endpoint logs and serves the
  rest of the row without the badge: the response DTO clears any existing
  `queued_prompt_count` to `0` for that request only. The cleared value is
  never persisted back to the summary store, so a later successful count or
  projector event restores the badge. Matching the existing status-summary
  load behavior.
- **Queued-counter provider unavailable (unwired):** the DTO leaves the
  projected `queued_prompt_count` untouched instead of stamping a zero over
  it, so a previously projected positive count survives list loads.
- **Lifecycle purge without queue-status event (pre-fix):** archive/delete
  emptied `queued_messages` in SQL but never published
  `message.queue.status_changed`. Live clients kept the last projected positive
  count until a full list reload; archive kept the badge on the archived row
  via preserved `statusSummary`. The lifecycle-purge rule above closes this.
- **Session delete leaving orphan queue rows (pre-fix):** session delete
  removed the session row without cancelling its pending queue entries. Task
  list assembly and the badge continued to count those orphans. Session delete
  must purge the session queue and publish the status event.

## Scenarios

- **GIVEN** a task with three prompts queued across its sessions, **WHEN** the
  sidebar loads, **THEN** its row shows a mail icon and `3` on the metadata
  line.
- **GIVEN** a task with an empty queue, **WHEN** the sidebar loads, **THEN**
  its row shows no mail badge and the metadata line is unchanged.
- **GIVEN** a task with two queued prompts, **WHEN** one is drained, **THEN**
  the badge updates to `1` without a reload, and to nothing when the last one
  drains.
- **GIVEN** a prompt reserved for in-flight delivery while a later prompt
  remains pending, **WHEN** the sidebar renders, **THEN** only the pending
  prompt counts.
- **GIVEN** a subtask with one queued prompt, **WHEN** the sidebar renders the
  subtree, **THEN** the subtask row shows the same badge treatment as a
  top-level task.
- **GIVEN** a phone viewport, **WHEN** the sidebar renders a row with queued
  prompts, **THEN** the badge is visible without hover and the row does not
  overflow horizontally.
- **GIVEN** a live task whose status summary projects `queued_prompt_count = 11`
  and whose queue has eleven pending rows, **WHEN** the task is archived,
  **THEN** the queue is purged, a `message.queue.status_changed` event fires for
  that `task_id`, the projector persists `queued_prompt_count = 0`, and every
  live sidebar surface that still shows the task (including the archived
  filter) renders no mail badge without a page reload.
- **GIVEN** a live task with pending prompts, **WHEN** the task is deleted,
  **THEN** the task disappears from active and archived sidebar sources and no
  residual badge remains for that task id.
- **GIVEN** a multi-session task with pending prompts on one session only,
  **WHEN** that session is deleted, **THEN** those pending rows are cancelled,
  a queue-status event fires for the task, and the badge reflects only the
  remaining sessions' pending count (or nothing when none remain).

## Out of Scope

- Clicking the badge to open the queue panel (the row click still navigates).
- Per-session breakdown of the count.
- WIP overflow queues (`queued_for_step_id`) — a different queue concept with
  its own UI.
- Queue capacity display, queue contents, or provenance in the sidebar.
- Changing the session-scoped routing of `message.queue.status_changed`.
