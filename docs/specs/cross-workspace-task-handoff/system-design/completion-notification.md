---
status: current
system: cross-workspace-task-handoff
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001
---

# Completion notification System Design

## Ownership and authority

The task state-change event identifies a delivery task that entered a terminal
state. The Office service reads the trusted forward `handoff_source` metadata
and current source assignee. The backend composition policy checks the source
task, its Office workspace, the source session, the source agent, the
delivery task, and the source task's protected reverse link. This uses durable
task provenance. It does not query prunable run history.

The policy reads the directional collaboration grant for the delivery task
and source workspace. The `report` grant authorizes a stored source-task
comment. The `prompt` grant authorizes a source-agent wake. These rights are
independent. Native task/session identity and workspace checks remain in force.

## Data and delivery

The Office repository writes the optional comment and one
`office_handoff_completion_outbox` receipt in one database transaction. The
receipt key is stable for the delivery task and terminal state. It stores the
source and delivery identities, the delivery title and state, and independent
pending flags for comment-event publication and source-task wake delivery.
Completed receipts remain as deduplication records after run history expires.

The comment uses the source agent profile as its author. Its creation event is
internal and does not relay the comment to a linked external channel. A
prompt-only grant creates no comment. Its run payload carries the delivery
task title and terminal state. A report-and-prompt grant also carries the
comment id, so prompt assembly can load the stored report.

Before an outbox wake is retried, the service re-reads the handoff relation,
the current source assignee, and the directional prompt grant. A changed
relation, changed assignee, or revoked grant resolves the wake without a run.
An authorized wake uses the source task's task-boundary carrier and the
`handoff_completed` reason. It skips coalescing, so another delivery report
cannot replace this run's payload or idempotency key.

## Lifecycle

```text
Delivery task enters terminal state
  -> Office subscriber reads protected handoff metadata
  -> backend policy validates source/delivery/session/agent links and grants
  -> optional report comment + durable outbox receipt commit together
  -> publish report event; queue authorized source-task wake
  -> mark each action complete independently

Temporary failure or source workspace pause
  -> leave that action pending in the outbox
  -> Office recovery tick retries it
  -> recheck prompt grant and source assignee before a wake
```

The report and wake have separate delivery status because event publication
can fail while queueing succeeds, and a pause can block a wake after the
comment has been stored. The recovery tick does not use the generic deferred
assignment table. It retains this completion's reason, payload, and key.

## Requirement mapping

| Requirement | Design |
| --- | --- |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001` | Trusted provenance, independent grants, durable receipt, and lifecycle above |
