---
status: active
system: ui
created: 2026-08-03
owners:
  - kandev
---
# Manage Pending Message Queues Requirements

## Overview

Long-running tasks can receive messages faster than their active session can drain them. Users need to discard stale messages and tune the queue capacity without shell access. Today the queue panel promises **Clear all**, but the backend removes only user-origin rows. A queue filled with ten inter-task agent messages reports success with zero removals, immediately reappears, and remains full. Those same rows have no individual remove action.

## Requirements

### REQ-UI-MESSAGE-QUEUE-MANAGEMENT-001: Manage Pending Message Queues

**Intent:** Long-running tasks can receive messages faster than their active session can drain them. Users need to discard stale messages and tune the queue capacity without shell access. Today the queue panel promises **Clear all**, but the backend removes only user-origin rows. A queue filled with ten inter-task agent messages reports success with zero removals, immediately reappears, and remains full. Those same rows have no individual remove action.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.1:** Every visible pending message offers **Remove**, regardless of whether its provenance is user, agent, workflow, or server.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.2:** **Clear all** removes every visible pending message in the current session and returns the exact number removed.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.3:** Removal does not change provenance rules for other operations. Only user-owned messages may be edited. Merge behavior remains defined by [Merge Enqueued Messages Individually](message-queue-merge.md), and admission-time compaction is defined by [Automatically Merge Consecutive Queued Messages](message-queue-auto-merge.md). The two merge settings are independent. The interrupt-and-dispatch behavior is defined by [Send Queued Messages Now](message-queue-send-now.md).
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.4:** Successful removal publishes the existing `message.queue.status_changed` event. All connected views reconcile to the backend result; the initiating view may update optimistically but must refetch on a race or failure.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.5:** Displayed queue positions are compact one-based ordinals derived from the current visible order. After remove, merge, or drain leaves gaps in durable FIFO position keys, the panel still displays `#1` through `#N` without gaps.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.6:** An entry already reserved for durable lifecycle delivery is not visible and cannot be removed by these controls. Task archive/delete retains its separate privileged purge behavior.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.7:** **Settings > Task Behavior > Message Queue** exposes the maximum number of persisted messages allowed per session alongside independent manual and automatic merge switches.
- **AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.8:** The default is `10`. A positive integer sets a cap; `0` means unlimited.

## Migrated source detail

## Why

Long-running tasks can receive messages faster than their active session can
drain them. Users need to discard stale messages and tune the queue capacity
without shell access. Today the queue panel promises **Clear all**, but the
backend removes only user-origin rows. A queue filled with ten inter-task
agent messages reports success with zero removals, immediately reappears, and
remains full. Those same rows have no individual remove action.

## What

### Queue controls

- Every visible pending message offers **Remove**, regardless of whether its
  provenance is user, agent, workflow, or server.
- **Clear all** removes every visible pending message in the current session
  and returns the exact number removed.
- Removal does not change provenance rules for other operations. Only
  user-owned messages may be edited. Merge behavior remains defined by
  [Merge Enqueued Messages Individually](message-queue-merge.md), and
  admission-time compaction is defined by
  [Automatically Merge Consecutive Queued Messages](message-queue-auto-merge.md).
  The two merge settings are independent. The
  interrupt-and-dispatch behavior is defined by
  [Send Queued Messages Now](message-queue-send-now.md).
- Successful removal publishes the existing
  `message.queue.status_changed` event. All connected views reconcile to the
  backend result; the initiating view may update optimistically but must
  refetch on a race or failure.
- Displayed queue positions are compact one-based ordinals derived from the
  current visible order. After remove, merge, or drain leaves gaps in durable
  FIFO position keys, the panel still displays `#1` through `#N` without gaps.
- An entry already reserved for durable lifecycle delivery is not visible and
  cannot be removed by these controls. Task archive/delete retains its separate
  privileged purge behavior.

### Capacity setting

- **Settings > Task Behavior > Message Queue** exposes the maximum number of
  persisted messages allowed per session alongside independent manual and
  automatic merge switches.
- The default is `10`. A positive integer sets a cap; `0` means unlimited.
- A saved setting applies immediately to later admissions. Existing entries
  are never trimmed. If a queue already exceeds a newly lowered limit, new
  messages are rejected with `queue_full` until its persisted count is below
  the limit.
- Previously accepted work may be restored or retried after a delivery
  failure even when the new cap is lower. Capacity limits new work; it does not
  turn a failed delivery into message loss.
- `KANDEV_QUEUE_MAX_PER_SESSION` remains supported and has higher precedence
  than the saved setting. A valid environment value makes the field read-only
  and the page explains the lock. Values at or below zero mean unlimited. An
  invalid value is logged and ignored.
- Signed-in members may view the effective setting. Admins may change it. With
  authentication disabled, the synthetic single user remains an admin.
- The settings route participates in the shared settings save coordinator. It
  does not add a page-local Save or Cancel button.

## API Surface

Existing WebSocket actions retain their payloads:

```text
message.queue.remove
  request:  { session_id: string, entry_id: string }
  response: { entry_id: string }

message.queue.cancel
  request:  { session_id: string }
  response: { session_id: string, removed: number }
```

Both actions require access to `session_id`. `message.queue.remove` returns
`entry_not_found` when the entry does not exist in that session, was drained,
was reserved before cancellation won, or was removed concurrently. Clear-all
is idempotent and returns `removed: 0` when no visible pending entries remain.

New HTTP endpoints:

```text
GET   /api/v1/system/message-queue/settings
PATCH /api/v1/system/message-queue/settings
```

GET response and PATCH response:

```json
{
  "settings": {
    "max_per_session": 10,
    "merge_enabled": true,
    "auto_merge_enabled": true
  },
  "effective": {
    "max_per_session": 10,
    "merge_enabled": true,
    "auto_merge_enabled": true,
    "source": "default",
    "locked": false
  }
}
```

`source` is `default`, `setting`, or `environment`. `settings` contains the
persisted value, or the default when no override exists. PATCH accepts:

```json
{ "max_per_session": 25 }
```

PATCH is partial: omitted capacity, manual-merge, and automatic-merge fields
remain unchanged. It rejects negative capacity with `400`, rejects a valid
environment capacity lock with `409` only when the patch names
`max_per_session`, and requires the admin role. GET requires an authenticated
install user under the existing system-route middleware.

## Data Model

No queue-table migration is required. Capacity uses the existing install-wide
`settings` table under key `message_queue` with JSON value:

```json
{
  "max_per_session": 25,
  "merge_enabled": true,
  "auto_merge_enabled": true
}
```

`queued_by` stays unchanged as immutable provenance. The existing
`metadata.lifecycle_reserved_in_flight` marker distinguishes a durable row
already being delivered from a pending row the user may discard.

## State and Concurrency

For one visible row:

```text
pending --reserve wins--> in flight --acknowledge--> removed
   |
   +--user remove wins-----------------------------> removed
```

Clear-all applies the same rule to every pending row in one session. Queue
repository mutations serialize per session and compare the persisted row state
before deletion. A concurrent reserve or content mutation may win; cancellation
must never delete an entry after it becomes in-flight and must never report a
row it did not remove.

The effective capacity is held by the queue service in concurrency-safe memory.
Each admission reads one value and uses that same snapshot for its repository
operation and error response. A successful settings PATCH persists first and
then applies the value live; after a process restart the same resolver applies
environment, persisted setting, and default precedence before the orchestrator
starts accepting work.

## Responsive and Mobile Behavior

- The existing inline queue panel remains the phone composition; the queue
  list remains its single internal scroll owner and the composer stays visible.
- Remove and clear controls are always discoverable on touch devices. Their
  interactive hit areas are at least 44 by 44 CSS pixels on coarse-pointer
  viewports; desktop may retain compact hover-revealed controls.
- The Message Queue settings page uses the existing mobile settings sheet for
  navigation and the shared `settings-scroll-container` as its only page scroll
  owner. Content is one column; the numeric input is at least 44 pixels high
  and remains visible above the shared save bar and safe-area inset.
- Desktop and mobile expose the same queue removal and capacity behavior.

## Failure Modes

- **Drain or reserve wins:** individual remove returns `entry_not_found`; the
  frontend refreshes status without showing a stale row. Clear-all reports only
  rows it actually deleted.
- **Backend mutation fails:** the UI restores/refetches queue state and shows a
  localized error. It does not leave an optimistic empty queue on screen.
- **Queue setting is environment-locked:** the input is disabled and PATCH
  returns `409` if called directly.
- **Invalid saved data:** the backend logs the problem, uses the default, and
  allows an admin to replace it with a valid value.
- **Invalid environment data:** the backend logs the value, ignores it, and
  falls through to the persisted setting or default.
- **Concurrent settings writes:** last successful persisted write wins; the
  response carries the value applied to the running service.

## Scenarios

- **GIVEN** a session queue contains ten agent-origin messages, **WHEN** its
  owner clicks **Clear all**, **THEN** all ten pending rows are removed, the
  queue panel disappears, and a new message can be admitted.
- **GIVEN** one visible agent-origin message, **WHEN** the owner clicks its
  **Remove** action, **THEN** that entry disappears and other entries retain
  FIFO order and provenance, with displayed positions compacted to `#1`
  through `#N`.
- **GIVEN** visible user, agent, workflow, and server messages in one queue,
  **WHEN** the owner clicks **Clear all**, **THEN** every visible pending row is
  removed regardless of origin.
- **GIVEN** a durable lifecycle head is reserved while a later pending message
  remains visible, **WHEN** the owner clears the queue, **THEN** the later
  message is removed and the hidden in-flight row remains available for
  acknowledgement.
- **GIVEN** a caller cannot access another user's session, **WHEN** they submit
  its session ID and a disclosed queue-entry ID, **THEN** the operation reveals
  no queue contents and performs no mutation.
- **GIVEN** the saved limit is `20` and no environment override exists, **WHEN**
  an admin changes it to `3`, **THEN** subsequent status reports `max: 3` and
  new admissions are rejected once three persisted rows are present.
- **GIVEN** a queue already contains ten rows, **WHEN** the admin lowers the
  limit to `3`, **THEN** all ten remain, removals and draining still work, and
  new admissions remain blocked until fewer than three rows persist.
- **GIVEN** an accepted message is dequeued and delivery fails after the limit
  was lowered below the current count, **WHEN** the orchestrator restores it,
  **THEN** the original message identity and FIFO position are preserved.
- **GIVEN** `KANDEV_QUEUE_MAX_PER_SESSION=50`, **WHEN** the page loads, **THEN**
  it shows effective value `50`, identifies the environment source, and keeps
  the input read-only.
- **GIVEN** an admin saves `0`, **WHEN** they enqueue more than ten messages,
  **THEN** no capacity error is produced solely by queue length and the page
  explains that the queue is unlimited.
- **GIVEN** a phone viewport, **WHEN** the user opens the queue panel and the
  Task Behavior settings menu, **THEN** remove/clear are touch-sized and Message
  Queue is reachable without horizontal overflow or a second page scroll
  owner.

## Out of Scope

- Editing agent-, workflow-, or server-origin message content.
- Changing the shipped merge compatibility rules.
- Interrupting an active prompt to dispatch pending work; that separate action
  is governed by [Send Queued Messages Now](message-queue-send-now.md).
- Per-workspace, per-task, or per-session capacity overrides.
- Automatically pruning old messages when a lower limit is saved.
- Bulk selection beyond **Clear all**. Reordering pending messages is a
  separate capability governed by [Reorder Queued Messages](message-queue-reorder.md).
