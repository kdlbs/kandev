---
status: active
system: tasks
created: 2026-10-04
owners:
  - kandev
---

# Peer Report Batch Delivery Requirements

## Overview

A controller session that supervises child agents receives one peer report per
child completion. Each report occupies a queue slot and one turn on its own, so a
burst of reports from a wide fan-out can exhaust the queue while the controller
is still busy, and the operator sees `queue_full` rejections for reports that the
controller would have consumed minutes later.

The task system owns the controller's prompt queue, so it owns how a run of
already-accepted peer reports is handed to the controller. This requirement
defines that batching contract; it does not change admission, capacity, or
ordering for any other queue row.

## Requirements

### REQ-TASKS-PEER-REPORT-BATCH-001: Bounded batch delivery of ordinary peer reports

**Intent:** Deliver a burst of ordinary peer reports to a controller session
without spending one queue slot and one turn per report.

#### Acceptance criteria

- **AC-TASKS-PEER-REPORT-BATCH-001.1:** When a session drains its queue, a
  consecutive run of ordinary peer reports at the head shall be reserved and
  delivered as a single prompt containing every report of the run, in queue
  order.
- **AC-TASKS-PEER-REPORT-BATCH-001.2:** Every report in a batch shall keep its
  own attribution in the delivered prompt. A recipient shall be able to tell
  which task sent each report without inspecting queue internals.
- **AC-TASKS-PEER-REPORT-BATCH-001.3:** Batching shall stop at the first row that
  is not an ordinary peer report. User and operator instructions, managed input,
  durable delivery rows, lifecycle rows, plan comments, already reserved rows,
  edit-leased rows, and rows without a sender task shall each remain ordering
  boundaries that are delivered in their own turn.
- **AC-TASKS-PEER-REPORT-BATCH-001.4:** A batch shall be bounded by a maximum
  report count and a maximum combined report size. Reports beyond either bound
  shall remain queued for a later turn rather than being dropped or truncated.
- **AC-TASKS-PEER-REPORT-BATCH-001.5:** Reserving a batch shall not change queue
  accounting. A reserved row leaves the queue and is tracked in its dispatch
  claim, and a batch shall be settled exactly as a single reserved row is
  settled today, so `queue_full` accounting shall be unchanged by reservation.
- **AC-TASKS-PEER-REPORT-BATCH-001.6:** A dispatch that cannot start shall return
  every provably undispatched reserved row of the batch to the queue in its
  original order, so no report is lost and no later row overtakes it. An
  uncertain commit or executor outcome shall retain the current dispatch claims
  for recovery rather than replaying a possibly delivered report.
- **AC-TASKS-PEER-REPORT-BATCH-001.7:** Both session-identity drain paths shall
  behave identically and apply the current exact task, session incarnation and
  lifecycle generation checks before folding. A replaced or already bound claim
  shall follow current recovery ownership without a second dispatch.
- **AC-TASKS-PEER-REPORT-BATCH-001.8:** `queue_full` remains reachable while a
  controller turn is long-running and more reports arrive than one turn can
  absorb. Batching reduces how often the queue fills; it does not make queue
  exhaustion impossible.

## Compatibility and exclusions

No user-facing copy changes: the batch is delivered to the agent, not rendered
in the composer. Sessions with a single queued row, or a head row that is not an
ordinary peer report, keep today's behavior. Raising the per-session queue cap,
periodically delivering idle-session auto-runs, bounding manual over-ceiling
starts, indexing the clarification inbox, aging ceiling holds, and the ordering
of a direct `message.add` behind queued messages are separate candidates and are
not part of this requirement.

## Related contracts

- [Queue admission](queue-admission.md) owns durable prompt admission.
- [Resume prompt queue](resume-prompt-queue.md) owns startup eligibility and deferred dispatch.
- [Durable agent delivery](../../platform/system-design/durable-agent-delivery.md)
  preserves the existing queue dispatch-claim and executor submission ownership.

