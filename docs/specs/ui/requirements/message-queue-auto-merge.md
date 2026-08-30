---
status: active
system: ui
created: 2026-08-12
owners:
  - kandev
---
# Automatically Merge Consecutive Queued Messages Requirements

## Overview

Users and coordinating agents often send several short follow-ups while a session is busy. Delivering every follow-up as a separate turn adds latency and queue noise even when the messages came from the same source and can safely be handled together.

## Requirements

### REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-001: Automatically Merge Consecutive Queued Messages

**Intent:** Users and coordinating agents often send several short follow-ups while a session is busy. Delivering every follow-up as a separate turn adds latency and queue noise even when the messages came from the same source and can safely be handled together.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.1:** **Settings > Task Behavior > Message Queue** exposes an **Automatically merge consecutive messages** switch. The install-wide switch is on by default.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.2:** The switch applies immediately to messages admitted after the setting is read. It does not compact entries that were already pending.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.3:** When enabled, a newly admitted message is automatically folded into the pending tail entry only when the two entries are consecutive and compatible.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.4:** The earlier tail entry survives. It keeps its ID, FIFO position, queued time, task, model, plan-mode value, and sender identity. Content and additive context from the new entry follow the earlier entry in submission order.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.5:** User entries have the same source only when both have the same non-reserved `queued_by` identity. Agent entries have the same source only when both are agent-owned and carry the same non-empty `sender_task_id`.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.6:** Workflow, server, clarification, durable lifecycle, and reserved in-flight entries are never automatic merge sources or targets. An agent entry without `sender_task_id` is also excluded.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.7:** Entries must have the same `task_id`, `model`, and `plan_mode`. Metadata other than `entity_references` and `context_files` must be equivalent. This keeps plugin source, sender-session provenance, parent-question state, workflow state, and future metadata from being silently discarded.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-001.8:** Merged content uses one blank line between two non-empty bodies. When either body is empty, the other body is retained without an added leading or trailing separator.

## Migrated source detail

## Why

Users and coordinating agents often send several short follow-ups while a
session is busy. Delivering every follow-up as a separate turn adds latency and
queue noise even when the messages came from the same source and can safely be
handled together.

## What

- **Settings > Task Behavior > Message Queue** exposes an
  **Automatically merge consecutive messages** switch. The install-wide switch
  is on by default.
- The switch applies immediately to messages admitted after the setting is
  read. It does not compact entries that were already pending.
- When enabled, a newly admitted message is automatically folded into the
  pending tail entry only when the two entries are consecutive and compatible.
- The earlier tail entry survives. It keeps its ID, FIFO position, queued time,
  task, model, plan-mode value, and sender identity. Content and additive
  context from the new entry follow the earlier entry in submission order.
- User entries have the same source only when both have the same non-reserved
  `queued_by` identity. Agent entries have the same source only when both are
  agent-owned and carry the same non-empty `sender_task_id`.
- Workflow, server, clarification, durable lifecycle, and reserved in-flight
  entries are never automatic merge sources or targets. An agent entry without
  `sender_task_id` is also excluded.
- Entries must have the same `task_id`, `model`, and `plan_mode`. Metadata other
  than `entity_references` and `context_files` must be equivalent. This keeps
  plugin source, sender-session provenance, parent-question state, workflow
  state, and future metadata from being silently discarded.
- Merged content uses one blank line between two non-empty bodies. When either
  body is empty, the other body is retained without an added leading or trailing
  separator.
- Attachments retain target-then-source order. Automatic merging occurs only
  when the combined attachment count and payload size remain within the shared
  per-message limits.
- Entity references form a stable target-then-source union, deduplicated by
  canonical reference, and must remain within the per-message reference limit.
  Context files form a stable target-then-source union with exact duplicates
  removed and remain capped at 200 descriptors. Exceeding that cap keeps the
  newly admitted entry separate.
- If any compatibility, provenance, metadata, attachment, or reference check
  fails, the newly admitted message remains a separate FIFO entry. Automatic
  merge incompatibility never rejects an otherwise valid message.
- The existing manual **Merge with above** setting and action remain separate.
  `merge_enabled` controls only the manual action; `auto_merge_enabled` controls
  only admission-time automatic merging. Either switch may be enabled without
  the other.
- No automatic-merge control is added to the queue panel. The queue panel
  reflects the resulting authoritative queue after admission.

## Data model

The existing install-wide `settings` row with key `message_queue` gains one
boolean. Its normalized JSON value is:

```json
{
  "max_per_session": 10,
  "merge_enabled": true,
  "auto_merge_enabled": true
}
```

- `auto_merge_enabled` defaults to `true` for a fresh installation.
- A persisted record that predates this field and omits it also resolves to
  `true`.
- An explicit `false` remains false across reloads and backend restarts.
- The setting has no environment-variable override. The existing environment
  lock for `max_per_session` does not lock either merge switch.

No queued-message schema changes. A successful automatic merge updates the
surviving target's content, attachments, and additive metadata, then removes
the new source entry. A fallback leaves both rows intact.

## API surface

The existing HTTP settings routes retain their authorization and partial-PATCH
contract:

```text
GET   /api/v1/system/message-queue/settings
PATCH /api/v1/system/message-queue/settings
```

GET and PATCH responses include the new field in both views:

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

PATCH may name only the new field:

```json
{ "auto_merge_enabled": false }
```

Omitted fields remain unchanged. A capacity environment lock blocks only a
PATCH that names `max_per_session`; an auto-only PATCH leaves the environment's
effective live capacity unchanged.

No new WebSocket action is added. Existing admissions, including
`message.queue.add` and inter-task queued delivery, return the surviving target
entry when an automatic merge succeeds and return the new source entry when it
stays separate. `message.queue.status_changed` publishes the final queue state.
The explicit `message.queue.append`, coalescing, restoration, retry, edit,
reorder, and manual merge contracts do not gain automatic behavior.

## Admission lifecycle and concurrency

1. An admission snapshots `auto_merge_enabled` and the current queue capacity.
2. Normal validation and capacity admission run first. A queue that is already
   full still returns `queue_full`; automatic merge does not bypass the cap.
3. The new entry is durably inserted. File-backed attachments are claimed using
   the existing task/session ownership checks before any fold is attempted.
4. When the snapshotted setting is on, the backend atomically checks the new
   entry and its immediate predecessor and either folds the new entry into that
   predecessor or leaves both entries unchanged.
5. The final authoritative queue is published.

Concurrent admissions for one session preserve their admission order. The
per-session admission serialization spans insertion, any staged attachment
claim, and the final fold or rollback. A later admission therefore cannot merge
into a provisional source whose attachment claim may still fail. The tail
check, target rewrite, and source removal are one repository mutation, so a
drain, remove, reorder, or manual merge can win before or after it but cannot
observe a half-merged row. If a concurrent action removes either candidate
before the automatic fold, no unrelated row is selected as a replacement
target.

## Permissions

- Signed-in install members may read the configured and effective values under
  the existing system-settings rules.
- Only install admins may change `auto_merge_enabled`.
- Automatic merging grants no new queue mutation permission. It runs only as
  part of an already-authorized admission and uses persisted sender provenance;
  callers cannot name a target or claim a reserved sender identity.

## Failure modes

- **Incompatible tail:** the source remains a separate FIFO entry. No error is
  shown solely because automatic merge was skipped.
- **Combined limit exceeded:** both messages remain separate; no attachment,
  context file, or entity reference is truncated.
- **Attachment claim fails:** the newly inserted source entry is rolled back by
  its own ID before automatic merge runs. An older target is never removed or
  rewritten by this rollback.
- **Automatic fold storage failure after admission:** the transaction leaves
  both rows separate, the failure is logged, and the already accepted message
  is not reported as failed in a way that invites a duplicate retry.
- **Concurrent drain or mutation wins:** the fold is skipped or loses with no
  partial write; the final queue status reconciles clients.
- **Invalid legacy settings JSON:** the existing invalid-setting fallback
  applies. When the record is valid but omits `auto_merge_enabled`, the value is
  `true`.

## Persistence guarantees

The saved switch survives backend restarts in the existing settings store. A
restart applies it before new queue admissions. Already merged rows remain
merged; disabling the switch does not reconstruct source rows. Already separate
rows remain separate; enabling the switch does not sweep them.

## Responsive and mobile behavior

- Desktop and mobile use the same Message Queue settings card, values, save
  contributor, permissions, and validation.
- The new switch is a one-column settings row with a touch-safe control and
  localized label and description.
- The existing `settings-scroll-container` remains the only page scroll owner.
  The switch stays clear of the shared floating save bar and safe-area inset.
- The mobile queue panel does not gain a new control or scroll region.

## Scenarios

- **GIVEN** a fresh install, **WHEN** an admin opens Message Queue settings,
  **THEN** automatic merge is shown as enabled.
- **GIVEN** a legacy `message_queue` setting without `auto_merge_enabled`,
  **WHEN** the backend starts, **THEN** automatic merge is enabled and the next
  save writes the normalized field.
- **GIVEN** automatic merge is enabled and two consecutive compatible user
  messages are admitted, **WHEN** the second admission completes, **THEN** one
  queued entry remains with both contents in order and the first entry's
  identity.
- **GIVEN** automatic merge is disabled, **WHEN** the same two user messages are
  admitted, **THEN** two FIFO entries remain.
- **GIVEN** two consecutive agent messages with the same non-empty
  `sender_task_id`, **WHEN** the second is admitted, **THEN** they form one
  agent-owned entry and the surviving ID can be targeted by immediate parent
  interrupt delivery.
- **GIVEN** two agent messages from different sender tasks or sender sessions,
  **WHEN** the later message is admitted, **THEN** it remains separate.
- **GIVEN** a user message follows an agent message, **WHEN** it is admitted,
  **THEN** the two remain separate.
- **GIVEN** a workflow, server, clarification, or reserved lifecycle entry is a
  source or tail candidate, **WHEN** another message is admitted, **THEN** no
  automatic fold uses that entry.
- **GIVEN** two same-source entries differ in task, model, plan mode, plugin
  source, parent-question metadata, or other non-additive metadata, **WHEN** the
  later entry is admitted, **THEN** both remain separate with their metadata
  unchanged.
- **GIVEN** compatible entries carry attachments, context files, and entity
  references within all limits, **WHEN** they merge, **THEN** every value is
  present in target-then-source order with only exact/canonical duplicates
  removed.
- **GIVEN** a combined attachment, context-file, or entity-reference limit would
  be exceeded, **WHEN** the later message is admitted, **THEN** both entries
  remain separate and the admission succeeds if queue capacity permits.
- **GIVEN** a compatible file-backed message whose attachment claim fails,
  **WHEN** admission rolls back, **THEN** the prior tail is byte-for-byte
  unchanged.
- **GIVEN** the queue is already at its capacity, **WHEN** a compatible message
  is submitted, **THEN** the existing `queue_full` response is returned and no
  row changes.
- **GIVEN** automatic merge is on and manual merge is off, **WHEN** compatible
  messages are admitted, **THEN** they merge automatically while the manual
  action remains unavailable; the inverse switch combination leaves admissions
  separate while preserving the manual action.
- **GIVEN** an environment value controls queue capacity, **WHEN** an admin
  saves only `auto_merge_enabled`, **THEN** the automatic setting changes and
  the environment-controlled effective and live capacity remain unchanged.
- **GIVEN** pending rows predate an off-to-on settings change, **WHEN** the
  setting is saved, **THEN** those rows remain unchanged and only later
  admissions are eligible.
- **GIVEN** a phone viewport, **WHEN** an admin changes the switch and saves,
  **THEN** the value persists without horizontal overflow, nested scrolling, or
  overlap with the shared save bar.

## Out of scope

- Retroactively compacting an existing queue.
- Merging non-consecutive entries or searching past an incompatible tail.
- A per-workspace, per-task, per-session, or queue-panel override.
- Changing the manual **Merge with above** compatibility or error behavior.
- Changing `message.queue.append`, workflow coalescing, delivery restoration,
  retry, send-now, edit, or reorder semantics.
- Bypassing queue capacity because a message might merge.

## Implementation plan

[Implementation plan](../../../plans/message-queue-auto-merge/plan.md)
