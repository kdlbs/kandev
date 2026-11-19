---
status: active
system: ui
created: 2026-11-05
owners:
  - kandev
---

# Message Queue Sorted Auto-merge Requirements

## Overview

Coordinating tasks receive interleaved messages from agent sub-tasks. Auto-merge
combines compatible adjacent messages and additionally folds an agent message
into its sender task's latest pending entry across intervening ordinary rows.
Sorted folding is included in Auto-merge, which defaults ON. There is no
separate Auto-sort-merge setting, session override, switch, or status policy.
Turning a session's Auto-merge from OFF to ON immediately compacts its pending
queue; global changes affect later admissions only.

This revises the original #5086 design without changing the Auto-merge
compatibility rules. The existing UI-owned queue-automation requirements remain
the owner of the session interaction and its cross-surface outcomes.

## Terminology

- **Sender task:** The task identified by an agent entry's non-empty
  `sender_task_id`. Parent, sibling, and sub-task senders use the same rules.
- **Effective Auto-merge:** The explicit session override when present;
  otherwise the current install-wide Auto-merge value.
- **Same-sender target:** The latest entry above the source with the same agent
  sender task, including hidden persisted rows, with no barrier between them.
  An incompatible or edited latest target never permits choosing an older one.
- **Barrier:** A reserved/in-flight or durable lifecycle, plan-comment, or
  managed-input entry, or a workflow, server, or task-move entry.
- **Sorted fold:** A compatible agent source merges into its same-sender target
  across intervening ordinary messages. User messages may be crossed but never
  move themselves.
- **Compaction:** One atomic FIFO pass over pending entries, applying sorted
  agent folds and compatible adjacent folds for all eligible messages.

## Requirements

### REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001: Sorted folding within Auto-merge

**Intent:** Reduce queue size and avoidable turns without a second merge policy,
while retaining the order of messages that cannot merge.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1:** The install-wide Auto-merge
  setting shall default ON, including legacy settings that omit it. Its
  localized description shall explain adjacent compatible merging and agent
  sub-task messages merging into their sender's latest queued entry.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.2:** Settings shall expose no separate
  Auto-sort-merge option or dependent saved preference. Auto-merge OFF shall
  disable both adjacent and sorted automatic folds.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3:** Sorted folding shall use the same
  global/session inheritance, persistence, identity, deletion, archive, and
  transfer rules as Auto-merge. There shall be no independent sort override.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.4:** The expanded queue header shall
  expose Auto-run and Auto-merge only. Auto-merge help shall describe combined
  folding and immediate pending-queue compaction when enabled in a session.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.5:** Auto-merge OFF shall preserve all
  pending rows without splitting earlier merged content. Its control shall
  remain available for enabling both automatic fold paths together.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6:** With effective Auto-merge ON, an
  admitted agent message with a sender task shall fold into its compatible
  same-sender target. The target shall retain its ID and FIFO position, append
  source content and additive context, and keep the later queued time. Given
  A1, B1, A2, a compatible new B2 shall yield A1, B1+B2, A2.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7:** If the latest same-sender target
  is absent, incompatible, or edited, the source shall not choose an older
  same-sender target. Compatible ordinary adjacent folding shall remain the
  fallback; otherwise the source shall retain its tail position.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.8:** User messages and agent messages
  without sender tasks shall never be sorted-fold sources. Workflow, server,
  clarification, lifecycle, and reserved entries shall remain excluded under
  existing compatibility rules. Sorted folds shall not cross barriers.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.9:** At or above positive capacity,
  an eligible admission without staged attachment claims shall try its sorted
  target and then its adjacent target without adding a row. If neither folds,
  Auto-merge is OFF, or staged attachments require insertion, it shall return
  `queue_full` without changing the queue or claiming attachments.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10:** A session Auto-merge switch
  change from effective OFF to ON shall compact pending entries atomically with
  the override write. An inherited global OFF value counts as effective OFF.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.11:** Compaction shall visit entries
  in FIFO order and fold agent rows into their compatible latest same-sender
  kept entry, with adjacent folding for all eligible sources. Edited rows shall
  neither move nor receive a fold. Rows admitted under different management
  claims shall not merge. Unmerged rows shall keep their relative order.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.12:** Compaction shall preserve
  reserved/in-flight rows and Auto-run and shall not start delivery. Clients
  shall receive final authoritative status. A failed compaction shall roll back
  both policy and queue, restore authoritative client state, and show a localized
  error.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13:** Global Auto-merge changes shall
  affect later admissions only and never compact pending rows. Session ON-to-ON
  and OFF writes shall not compact. Disabling shall never split merged rows.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14:** An unreadable Auto-merge override
  shall disable both fold paths for that admission. Unavailable status shall
  preserve readable entries and the client's last authoritative policy value,
  disabling the control until a newer successful status. An explicit readable
  session override shall still govern admissions if the global loader fails.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15:** Delayed status, success, and
  unavailable results shall obey Auto-merge's incarnation, epoch/generation,
  source, and revision ordering. No separate sort-policy tuple shall be exposed.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.16:** Desktop and mobile shall expose
  the same Auto-merge state, mutation, compaction, persistence, and error
  outcomes. Whole pills shall wrap without horizontal overflow, and coarse
  pointer targets shall be at least 44 CSS pixels in each dimension. Removing
  sort help shall preserve the remaining tooltips' Escape boundary.

## Out of scope

- Moving messages that cannot merge or sorted folding of user messages.
- Changing `buildAutoMergedEntry` compatibility, manual merge, reorder,
  Send Now, Auto-run, append, lifecycle coalescing, restore, or retry semantics.
- Global-triggered compaction, resetting an override to inherited, and splitting
  already merged content.

## Related requirements

- [Per-session Automatic Message Merge Override](message-queue-auto-merge-session-overrides.md)
- [Message Queue Automation Controls](message-queue-automation-controls.md)
