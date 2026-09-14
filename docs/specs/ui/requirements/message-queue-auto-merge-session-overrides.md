---
status: active
system: ui
created: 2026-09-04
owners:
  - kandev
---

# Per-session Automatic Message Merge Override Requirements

## Overview

Automatic message merging reduces avoidable agent turns by folding compatible
consecutive prompts from one source. Install administrators set the default,
while each task session can retain an explicit override from the expanded queue
panel.

This requirement supersedes
`REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-001` when activated. It preserves the existing
compatibility and content/entity fold semantics while replacing the
install-wide-only policy, aligning queued-time behavior with the shipped
automatic fold, adding a session control, and allowing a compatible direct fold
at or above the persisted-row capacity.

## Terminology

- **Global value:** The current install-wide automatic-merge setting.
- **Session override:** An explicit ON or OFF value saved after a user changes
  the Auto-merge switch for one task session.
- **Effective value:** The session override when one exists; otherwise the
  current global value.
- **Staged-attachment admission:** An admission that must create a source row
  before it can claim uploaded attachments into session ownership.

## Requirements

### REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001: Per-session Automatic Message Merge Override

**Intent:** Let users override the install default for the session they are
actively managing without making untouched sessions stale copies of that
default.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.1:** \*\*Settings > Task Behavior
  > Message Queue** shall expose an **Automatically merge consecutive
  > messages\*\* switch whose install-wide default is ON. Its localized description
  > shall explain that sessions inherit this value until changed in the session's
  > queue panel.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2:** A session without an
  explicit override shall use the current global value. When the global value
  changes, existing sessions that have never changed Auto-merge shall use the
  new value for later admissions.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3:** The first successful
  Auto-merge change in a session shall persist an explicit ON or OFF override
  for that session. Later global changes shall not alter its effective value.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.4:** The expanded queue header
  shall expose the effective value through the labeled Auto-merge switch pill
  defined by `REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001`.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5:** The effective value shall
  apply only to messages admitted after it is resolved. Changing it shall not
  compact or split entries already pending.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.6:** When effective Auto-merge
  is ON, a newly admitted message shall fold into the pending tail only when the
  entries are consecutive and compatible under
  `REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-001`.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.7:** At or above a positive
  persisted-row limit, a compatible admission that does not require a staged
  attachment claim shall fold directly into the tail without increasing the
  row count. An incompatible admission or an admission with effective
  Auto-merge OFF shall fail with `queue_full` and leave the queue unchanged.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.8:** At or above a positive
  persisted-row limit, a staged-attachment admission shall fail with
  `queue_full` before claiming the attachment or changing the prior tail. It
  shall not use the direct candidate-fold path.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.9:** A successful automatic fold
  shall preserve the earlier tail's identity, FIFO position, task, model,
  plan-mode value, and sender identity while appending compatible content and
  additive context in target-then-source order. Its queued time shall become
  the later of the target and source queued times so pending activity reflects
  the newest accepted prompt.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.10:** Automatic merging shall
  remain independent from manual **Merge with above**. The two effective
  switches may differ without enabling or disabling each other's behavior.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.11:** A saved session override
  shall survive an empty queue, navigation, reload, and backend restart. A new
  task session shall have no override and shall inherit the current global
  value until changed.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.12:** A failed override mutation
  shall restore authoritative state and show a localized error. If the backend
  cannot read a session override during admission, that admission shall fail
  closed by skipping automatic merge rather than falling back to ON.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.13:** A delayed queue snapshot,
  successful status, or unavailable-status result shall not replace or disable
  newer authoritative Auto-merge state for the same task-session incarnation.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.14:** Desktop and mobile shall
  expose the same effective state, mutation, persistence, error, and global
  precedence outcomes without document horizontal overflow or nested queue
  scrolling.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15:** If the backend cannot
  resolve Auto-merge while reading queue status, it shall still reconcile queue
  entries but shall report the policy as unavailable rather than inventing an
  effective value. The client shall apply availability only from the newest
  status read, preserve any last authoritative value, and disable Auto-merge
  until a newer successful status restores availability.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.16:** An override shall belong
  to the exact task-session identity. Deleting that session, deleting its task,
  or deleting its workspace shall remove its queue-owned state atomically with
  the destructive lifecycle operation, so later reuse of the identity starts
  without an override. Archiving a task shall preserve the state because its
  session identity remains; workflow transfer shall remove the source state
  without copying its override over the destination's independent state. A
  post-delete queue recount shall be task-scoped, shall not carry the deleted
  session ID, and shall not be delivered to browser session reconciliation.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.17:** Every created task session
  shall receive an opaque server-generated incarnation identity that remains
  stable for that row's lifetime and changes if the same session ID is reused.
  Legacy rows shall receive distinct identities through a replay-safe migration
  that preserves them through every task-session table rebuild.
  Session-scoped queue status shall carry that identity. The client shall
  discard status for a different incarnation, clear queue metadata when the
  session is removed, and reset source/revision ordering when the current
  session ID is recreated with a new incarnation.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.18:** Every ordinary queue
  admission and Auto-merge override mutation shall carry the caller's expected
  session incarnation into the persistence boundary. Browser
  `message.queue.add` and `message.queue.append` requests shall require that
  token, and every internal ordinary or lifecycle-coalescing producer capable
  of inserting a row shall use the same typed identity contract. The backend
  shall acquire the owning task lock before the queue session lock, confirm the
  exact task/session/incarnation still exists, and reject a missing or changed
  incarnation through the existing non-enumerating session-not-found response
  without appending, inserting, replacing, folding, claiming attachments, or
  writing policy. Session deletion shall use the same lock order, validation,
  and atomic queue-state cleanup.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.19:** A browser queue read shall
  carry its expected task/session/incarnation through authorization and the
  identity-bound snapshot. Every session-scoped status shall label entries and
  policy with only that validated identity. Deletion or recreation during the
  read shall yield an old-incarnation snapshot or no payload, never replacement
  state or old rows labeled with the replacement incarnation.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.20:** Queue mutation/loading
  state shall belong to an exact session incarnation and operation generation.
  Completion or cleanup from an older incarnation or operation shall not clear
  a newer operation or re-enable controls for a replacement session that
  reused the same textual ID.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.21:** Queue persistence shall
  fail closed unless an injected task-session authority validates identities
  inside its mutation transaction or memory critical section. A session
  transfer shall validate exact source and destination task/session/incarnation
  identities after locking sorted task rows and then sorted queue-session locks
  in one transaction. A mismatch shall move no rows or policy, and concurrent
  transfer and destructive lifecycle operations shall not deadlock or leave
  old-incarnation rows.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.22:** An admission that claims
  staged attachments shall validate its exact session identity, insert the
  queue row, claim attachments, and perform any automatic fold in one
  task-row-then-session-lock transaction or memory critical section. Any claim
  or identity failure shall roll back every queue and attachment change.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.23:** Deferred transition apply
  shall atomically CAS the captured task/session/incarnation, pending move,
  expected source, and target, then return an immutable effect token containing
  the committed task transition generation and queue-session identity.
  Post-commit task effects shall CAS that generation; session/queue effects
  shall validate the token's identity and generation. Recreation or a successor
  move shall leave its state unchanged.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.24:** Every session-scoped queue
  mutation and its resulting response or status shall retain the caller's
  expected task/session/incarnation. A stale operation shall not add, append,
  reserve, acknowledge, drain, send, update, remove, merge, reorder, clear,
  restore, change policy, or publish replacement-session state.
- **AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.25:** Queue work, including
  deferred-transition work, that continues after a claim or transaction shall
  retain immutable session identity and operation generation through
  cancellation, prompt dispatch, restore, acknowledgement, and terminal
  cleanup. Delayed old work shall not prompt, restore into, block, or clear
  in-flight ownership for a replacement session.

## Exclusions

- No workspace-level, task-level, or user-level override.
- No Follow global or reset-to-global control.
- No inherited-versus-explicit badge in the queue header.
- No retroactive compaction or reconstruction of pending messages.
- No transfer of an override from one task session to another.

## Related decision

[Inherit Queue Auto-merge Until Overridden](../../../decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md)
