---
status: active
system: ui
created: 2026-07-31
owners:
  - kandev
---
# Merge Enqueued Messages Individually Requirements

## Overview

When a session is busy, users queue follow-up prompts that drain one-per-turn. A burst of small, related prompts (a correction, a follow-up question, extra context) is dispatched as several separate turns, which wastes agent turns and fragments the conversation. Users want to collapse queued messages they no longer want delivered separately — folding one queued message into the one above it — before the agent picks them up.

## Requirements

### REQ-UI-MESSAGE-QUEUE-MERGE-001: Merge Enqueued Messages Individually

**Intent:** When a session is busy, users queue follow-up prompts that drain one-per-turn. A burst of small, related prompts (a correction, a follow-up question, extra context) is dispatched as several separate turns, which wastes agent turns and fragments the conversation. Users want to collapse queued messages they no longer want delivered separately — folding one queued message into the one above it — before the agent picks them up.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-MERGE-001.1:** A queued message that has another queued message above it (i.e. it is not the head of the queue) offers a **Merge with above** control in the queue panel, next to the existing Edit and Remove controls.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.2:** Clicking **Merge with above** folds the message into the message directly above it: the above message keeps its identity (id, position, sender, queued-at time) and its content is concatenated with the folded message's content, separated by a blank line.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.3:** The folded message is removed from the queue; the queue count decreases by one and remaining entries keep their FIFO order.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.4:** Attachments from both messages are carried over to the merged message.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.5:** Entity references from both messages are carried over to the merged message, deduplicated by canonical reference.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.6:** Merging is **rejected atomically** when the deduplicated reference union would exceed the per-message reference cap (100, mirroring `entityrefs.maxReferencesPerMessage`). Nothing is truncated: both rows keep the references they already persisted, and the merge reports `merge_reference_overflow` instead of silently dropping source references at dispatch time.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.7:** The merge is atomic and race-safe: a concurrent queue drain can either win entirely (both rows untouched) or lose entirely (both rows updated); it can never interleave between updating the above message and removing the folded one.
- **AC-UI-MESSAGE-QUEUE-MERGE-001.8:** A message may only be merged into a message of the **same sender kind** — mismatching kinds never merge. A user-typed message merges only into another user-typed message; an agent (inter-task) message merges only into another agent message. Workflow and system messages are never a merge source or target.

## Migrated source detail

Queue-wide removal and capacity behavior is defined separately by
[Manage Pending Message Queues](message-queue-management.md), and admission-time
compaction is defined by
[Automatically Merge Consecutive Queued Messages](message-queue-auto-merge.md).
This spec governs only the manual merge operation. Its `merge_enabled` setting
does not control automatic merging, and `auto_merge_enabled` does not change
the behavior or compatibility rules below.

## Why

When a session is busy, users queue follow-up prompts that drain one-per-turn. A
burst of small, related prompts (a correction, a follow-up question, extra
context) is dispatched as several separate turns, which wastes agent turns and
fragments the conversation. Users want to collapse queued messages they no
longer want delivered separately — folding one queued message into the one
above it — before the agent picks them up.

## What

- A queued message that has another queued message above it (i.e. it is not the
  head of the queue) offers a **Merge with above** control in the queue panel,
  next to the existing Edit and Remove controls.
- Clicking **Merge with above** folds the message into the message directly
  above it: the above message keeps its identity (id, position, sender,
  queued-at time) and its content is concatenated with the folded message's
  content, separated by a blank line.
- The folded message is removed from the queue; the queue count decreases by
  one and remaining entries keep their FIFO order.
- Attachments from both messages are carried over to the merged message.
- Entity references from both messages are carried over to the merged message,
  deduplicated by canonical reference.
- Merging is **rejected atomically** when the deduplicated reference union
  would exceed the per-message reference cap (100, mirroring
  `entityrefs.maxReferencesPerMessage`). Nothing is truncated: both rows keep
  the references they already persisted, and the merge reports
  `merge_reference_overflow` instead of silently dropping source references at
  dispatch time.
- The merge is atomic and race-safe: a concurrent queue drain can either win
  entirely (both rows untouched) or lose entirely (both rows updated); it can
  never interleave between updating the above message and removing the folded
  one.
- A message may only be merged into a message of the **same sender kind** —
  mismatching kinds never merge. A user-typed message merges only into another
  user-typed message; an agent (inter-task) message merges only into another
  agent message. Workflow and system messages are never a merge source or
  target.
- Two agent messages merge only when they come from the **same sender task**
  (identical `sender_task_id` in metadata), so the merged message never loses
  or conflates message provenance. The merged message keeps the target's
  sender identity.
- The head message (first in the queue) has no merge control: there is no
  message above it.
- The merge button is hidden entirely when only one message is queued, or when
  the message above it has a different sender kind (or is not mergeable at
  all).

## API surface

New WebSocket action `message.queue.merge` (registered alongside the existing
`message.queue.update` / `message.queue.remove` actions in
`internal/orchestrator/handlers/queue_handlers.go`).

Request payload:

```
session_id  string  required
entry_id    string  required — the message to fold into the one above it
user_id     string  optional — defaults to "user"; reserved identities rejected
```

Response payload:

```
entry_id    string  the id of the surviving (above) message
```

Errors:

- `entry_not_found` — the folded message was already drained or does not
  exist, matching the `message.queue.update` / `message.queue.remove`
  semantics.
- `validation` — the folded message is the head of the queue (nothing above
  it), the message above it is not a valid merge target (different sender kind,
  an agent message from a different sender task), the folded message is a
  non-mergeable kind (workflow/system), or `session_id` / `entry_id` is
  missing.
- `merge_reference_overflow` — the combined entity references exceed the
  per-message cap; both rows are left untouched.

After a successful merge the backend publishes the standard
`message.queue.status_changed` event so all connected clients refresh their
queue view, exactly like the other queue mutations.

## Data model

No schema change. The merge reuses the existing `queued_messages` table:

- The above entry's `content`, `attachments_json`, and `metadata_json` are
  rewritten in place (identity, `position`, `queued_by`, `queued_at`, `model`,
  `plan_mode`, `task_id` unchanged).
- The folded entry's row is deleted (same rule as `DeleteByID`: rows with
  `queued_by` in the reserved set are never deleted by this operation).
- Positions are not renumbered; gaps are allowed and ordering stays correct
  because the queue orders by `(session_id, position)`.

## Permissions

- The folded (source) entry and the above (target) entry must have the **same
  sender kind** — user merges into user, agent merges into agent.
- For a user-kind merge, both entries must have `queued_by` equal to the
  caller's identity and must not be in the reserved set (`agent`, `workflow`,
  `server`), mirroring the existing `UpdateContentAndMetadata` guard.
- For an agent-kind merge, both entries must have `queued_by` equal to `agent`
  and the source's `sender_task_id` metadata must match the target's; the merge
  is otherwise rejected because the merged entry keeps the target's sender
  identity. This is a deliberate carve-out of the ADR 0051 boundary that
  reserves agent-owned rows for backend mutation: consolidating additive
  one-way prompts from a single inter-task agent preserves the reserved row's
  provenance (the merged entry keeps the target's identity and the source's
  `sender_task_id`-matched lineage) while reducing deliveries. The gate is
  strict — identical non-empty `sender_task_id`, never caller-tunable — so the
  merge operation cannot move, reorder, or combine unrelated agent-owned rows.
  Authorized pending-row deletion is a separate operation governed by
  [Manage Pending Message Queues](message-queue-management.md).
- Workflow and system messages are never a merge source or target.
- The WS handler rejects client-supplied reserved `user_id` values, exactly like
  the update handler.

## Failure modes

- **Drain race:** if the agent drains the folded message (or the above message)
  between the UI rendering and the merge request, the backend returns
  `entry_not_found` and the frontend refetches the queue to resync, matching the
  existing edit/remove resync path.
- **Drain interleave:** the update-above and delete-source writes run inside one
  transaction (serialized per session by the repository's per-session lock and
  guarded by affected-row counts), so a concurrent drain can never observe a
  half-merged queue.
- **Reference overflow:** a merge whose deduplicated reference union exceeds the
  per-message cap is rejected atomically (`merge_reference_overflow`); the
  frontend surfaces a focused toast and the queue is unchanged.
- **Merge failure:** the frontend shows the existing toast error pattern and
  refetches; no partial state is left behind.

## Scenarios

- **GIVEN** two queued messages `A` and `B` with `B` behind `A`, **WHEN** the
  user clicks **Merge with above** on `B`, **THEN** the queue contains one
  message whose content is `A` + blank line + `B`, the queue count drops by
  one, and both messages' attachments and entity references are present.
- **GIVEN** a single queued message, **WHEN** the user opens the queue panel,
  **THEN** no **Merge with above** control is shown on it.
- **GIVEN** a queued message at the head of the queue, **WHEN** the merge
  action targets it, **THEN** the backend rejects it with a validation error
  and the queue is unchanged.
- **GIVEN** two consecutive user messages `A` and `B` with `B` behind `A`,
  **WHEN** the user clicks **Merge with above** on `B`, **THEN** the queue
  contains one user message whose content is `A` + blank line + `B`, and the
  merged entry keeps `A`'s identity.
- **GIVEN** two consecutive agent messages from the same sender task (same
  `sender_task_id`), **WHEN** the user clicks **Merge with above** on the later
  one, **THEN** the queue contains one agent message with the combined content
  that keeps the earlier entry's sender identity.
- **GIVEN** two consecutive agent messages from different sender tasks,
  **WHEN** the merge action targets the later one, **THEN** the backend rejects
  it and the queue is unchanged.
- **GIVEN** a user message whose above message is agent-owned, **WHEN** the
  user opens the queue panel, **THEN** no **Merge with above** control is shown
  on that message, and a direct merge request is rejected with a validation
  error.
- **GIVEN** a message queued by an agent (inter-task) whose above message is a
  user message, **WHEN** the user opens the queue panel, **THEN** no **Merge
  with above** control is shown on that message.
- **GIVEN** a merge request for a message that was just drained, **WHEN** the
  request arrives, **THEN** the backend returns `entry_not_found` and the
  frontend refetches the queue.
- **GIVEN** three queued messages `A`, `B`, `C`, **WHEN** the user merges `C`
  into `B` and then merges the resulting message into `A`, **THEN** the queue
  contains a single message with all three contents concatenated in order.
- **GIVEN** an above message with empty content (attachments only), **WHEN**
  the user merges a message into it, **THEN** the merged content is exactly the
  folded message's content (no leading blank line) and attachments combine.
- **GIVEN** two messages whose deduplicated entity-reference union exceeds 100,
  **WHEN** the user clicks **Merge with above** on the later one, **THEN** the
  backend returns `merge_reference_overflow`, both rows keep their references,
  and the frontend shows a focused error toast.

## Out of scope

- Merging delivered (already sent) messages — this operates only on the pending
  queue.
- Reordering queued messages (e.g. drag-to-move); only merge-into-above is
  offered.
- Merging messages of mismatching sender kinds (e.g. a user message into an
  agent message), or agent messages from different sender tasks.
- Merging workflow or system (server-owned) messages.
- A preview of the merged result before applying.
