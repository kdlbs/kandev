---
status: active
system: ui
created: 2026-11-05
owners:
  - kandev
---

# Message Queue Auto-sort-merge Requirements

## Overview

A task that orchestrates several sub-tasks receives many queued messages from
those agents. Messages from different senders interleave, so Auto-merge (which
folds a message only into the entry directly above it) rarely applies. The
queue can then exceed its capacity even though it could shrink to a half or a
third of its size.

Auto-sort-merge lets a newly admitted agent message skip over other senders'
entries to merge into the latest pending entry from the same sender task. When a
session turns it on, the existing queue is compacted the same way. It is a
third queue automation switch beside Auto-run and Auto-merge. Its global default
and per-session override follow the Auto-merge inheritance rule, and it takes
effect only while Auto-merge is in effect.

The UI system owns this contract because it extends the queue automation
contracts in
[Per-session Automatic Message Merge Override](message-queue-auto-merge-session-overrides.md)
and [Message Queue Automation Controls](message-queue-automation-controls.md).

## Terminology

- **Sender task:** The task whose agent sent a queued agent message through
  task messaging. It is identified by the entry's non-empty `sender_task_id`.
  This is usually a sub-task, but parent and sibling senders behave the same.
- **Auto-sort-merge preference:** The session override when one exists;
  otherwise the current global value.
- **Effective Auto-sort-merge:** ON only when the session's effective
  Auto-merge is ON and its Auto-sort-merge preference is ON.
- **Same-sender target:** For a source entry, the nearest pending entry above
  it, including entries hidden from the queue panel, that has the same sender
  task. There is a target only when no barrier entry lies between the two
  entries. Entries from other agent sender tasks and user entries are not
  barriers.
- **Barrier entry:** A reserved or in-flight entry (including one hidden from
  the queue panel), a durable lifecycle, plan-comment, or managed-input
  delivery, or an entry queued by workflow, server, or task-move dispatch.
- **Sorted fold:** An automatic merge of a source entry into its same-sender
  target that is not the entry directly above it.
- **Compaction pass:** One atomic pass over a session's pending entries that
  applies sorted folds and ordinary adjacent automatic folds.

## Requirements

### REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001: Message Queue Auto-sort-merge

**Intent:** Reduce queue size and turn count for orchestrating tasks by
merging each sender task's queued messages, while keeping strict FIFO order
whenever a merge is not possible.

**User story:** As a user running a task that coordinates many sub-tasks, I
want each sub-task's queued messages to merge even when other senders' messages
interleave, so that the queue stays within capacity and the coordinator
handles fewer turns.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1:** **Settings > Task Behavior >
  Message Queue** shall expose an **Automatically sort and merge messages from
  the same sender task** switch. Its install-wide default shall be OFF, and a
  saved setting that omits the value shall resolve to OFF. Its localized
  description shall explain that it requires automatic merge and that sessions
  inherit it until changed in the session's queue panel.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.2:** While the global automatic
  merge switch is OFF in the settings form, the global Auto-sort-merge switch
  shall be disabled, render OFF, and keep its saved value. Saving shall not
  change the saved Auto-sort-merge value. Turning automatic merge ON in the form
  shall re-enable the switch and show its saved value.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3:** A session without an explicit
  Auto-sort-merge override shall use the current global value. The first
  successful Auto-sort-merge change in a session shall persist an explicit ON
  or OFF override, and later global changes shall not alter it. The override
  shall survive an empty queue, navigation, reload, and backend restart. It
  shall follow the same session-identity, deletion, archive, and transfer
  rules as the Auto-merge override.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.4:** When the queue panel is
  expanded, the header shall show a labeled **Auto-sort-merge** switch pill
  after the Auto-run and Auto-merge pills in the same responsive automation
  group. Its accessible name and description shall be localized and shall
  state that it requires Auto-merge.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.5:** While the session's effective
  Auto-merge is OFF, the Auto-sort-merge pill shall be disabled and render OFF.
  The session's preference shall be kept, and the pill shall show it again
  when Auto-merge returns to ON.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6:** When effective
  Auto-sort-merge is ON and an agent message with a sender task is admitted,
  the queue shall fold it into its same-sender target if the two entries are
  compatible under the Auto-merge rules. The target shall keep its ID and FIFO
  position. Its content and additive context shall be followed by the new
  message's, and its queued time shall become the later of the two. For
  queued A1, B1, A2, a compatible new B2 shall produce A1, B1+B2, A2.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7:** When a new agent message has
  no same-sender target, or its target is incompatible or being edited, the
  message shall not move ahead of other entries. Ordinary Auto-merge with the
  entry directly above it shall still apply. The message shall never be folded
  into an earlier entry from the same sender task than its same-sender target.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.8:** User, workflow, server,
  clarification, lifecycle, and agent entries without a sender task shall never
  be sources of a sorted fold. Barrier entries shall never be targets or be
  crossed by a sorted fold.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.9:** At or above a positive queue
  capacity, an admission that does not need a staged attachment claim shall
  first try a sorted fold when effective Auto-sort-merge is ON, and otherwise
  or when that fold is not possible shall try the existing ordinary adjacent
  Auto-merge fold. Either fold shall complete without adding a row. When
  neither fold succeeds, when effective Auto-merge is OFF, or when a staged
  attachment claim is required, the admission shall keep the existing
  `queue_full` outcome and leave the queue unchanged.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10:** When a session switch change
  turns effective Auto-sort-merge from OFF to ON while pending entries exist,
  the queue shall run one compaction pass in the same atomic change as the
  switch write. This applies when the Auto-sort-merge switch is turned ON
  while Auto-merge is ON, and when the Auto-merge switch is turned ON while the
  Auto-sort-merge preference is ON.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.11:** A compaction pass shall visit
  pending entries in FIFO order, one at a time. Each visited entry that can be
  a sorted-fold source shall fold into its compatible same-sender target among
  the entries already kept. Any other visited entry shall fold into the kept
  entry directly above it when that pair is compatible under Auto-merge. An
  entry being edited shall never be folded into another entry or receive a
  fold, and two entries admitted under different task-management claims (a
  different managing plugin installation, instance, or claim generation, or
  one managed and one not) shall never fold together. Apart from those
  exceptions, the result shall equal the queue that admitting the same
  entries in order would produce with effective Auto-sort-merge ON. Entries
  that were not merged shall keep their relative order.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.12:** A compaction pass shall not
  modify reserved or in-flight entries or entries that are being edited. It
  shall not change Auto-run or start delivery. Clients shall receive one final
  authoritative queue status. If the pass cannot commit, the switch change
  shall fail, the prior queue and policy shall remain, and the client shall
  restore authoritative state and show a localized error.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13:** Global Auto-sort-merge or
  global Auto-merge changes shall affect only later admissions and shall not
  compact any session queue. A switch change that leaves effective
  Auto-sort-merge ON, keeps it OFF, or turns it OFF shall not compact or split
  pending entries.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14:** If the backend cannot read
  the Auto-sort-merge override during an admission, that admission shall skip
  sorted folds and may still apply ordinary Auto-merge. If the backend cannot
  read the override during a queue status read, it shall report Auto-sort-merge
  as unavailable without inventing a value. The client shall then disable the
  pill and keep its last authoritative value until a newer successful status.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15:** A delayed queue snapshot,
  success, or unavailable result shall not replace newer authoritative
  Auto-sort-merge state for the same task-session incarnation. The ordering
  rules for Auto-merge policy shall apply.
- **AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.16:** Desktop and mobile shall
  expose the same pill, state, mutation, compaction, persistence, error, and
  dependency outcomes. The header groups shall wrap without document
  horizontal overflow, and coarse-pointer targets shall be at least 44 CSS
  pixels in each dimension.

## Out of scope

- Moving a message that cannot merge, or grouping a sender's entries without
  merging them.
- Sorted folds for user messages, or limiting the feature to direct child
  sub-tasks.
- Compacting queues because of a global settings change.
- A Follow global, reset-to-global, or inherited-versus-explicit indicator.
- Changes to manual **Merge with above**, reorder, Send Now, Auto-run, append,
  lifecycle coalescing, restore, retry, or the Auto-merge compatibility rules.
- Splitting entries that were already merged when the switch turns OFF.

## Related requirements

- [Per-session Automatic Message Merge Override](message-queue-auto-merge-session-overrides.md)
- [Message Queue Automation Controls](message-queue-automation-controls.md)
