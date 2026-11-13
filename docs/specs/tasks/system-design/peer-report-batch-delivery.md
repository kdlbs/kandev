---
status: current
system: tasks
requirements:
  - REQ-TASKS-PEER-REPORT-BATCH-001
---

# Peer Report Batch Delivery System Design

## Boundary and mapping

The task system owns the controller prompt queue. This design extends the
existing drain path in `internal/orchestrator` and the queue service; it adds no
new persistence and no new admission rules.

| Criteria (AC-TASKS-PEER-REPORT-BATCH-001) | Sections |
| --- | --- |
| .1, .2, .4 | Batch reservation and composition |
| .3 | Eligibility boundary |
| .5, .6 | Settlement |
| .7 | Drain integration |
| .8 | Known limits |

## Batch reservation

`messagequeue.Service.ReservePeerReportBatchWithAutoRunForSession` mirrors
`ReserveQueuedWithAutoRunForSession`: it runs under `WithSessionAdmission`, it
returns the same `autoRun` meaning (false only when Auto-run is OFF), and it
reserves the head exactly as the single-row reserve does.

The reservation uses the existing session admission and ordered head reserve.
Eligible reports leave `queued_messages` into the existing pending dispatch
claims. Bound, durable and lifecycle rows remain separate. No new queue column
or admission rule is introduced. A persistent repository without atomic folding
support reserves a single row from the outset.

## Eligibility boundary

`IsOrdinaryPeerReport` is the single definition of an eligible row, shared by
reservation and tests. A row is eligible when it was queued by an agent, carries
a non-empty sender task id, is not managed input, is not a durable delivery row,
is not already reserved in flight, has no attachments, and carries no entity
references or context files that must stay separable. A non-eligible head is
delivered alone through the unchanged single-row path.

A joining report must also agree with the leading entry on model and plan mode,
so one batch is one dispatch shape.

## Bounds

`MaxPeerReportBatchEntries` (8) and `MaxPeerReportBatchBytes` (256 KiB of raw
report content) are named constants in `messagequeue`, not configuration.
Generated attribution labels are additional prompt text. Reports beyond either
bound stay queued in order.

## Composition

`ComposePeerReportBatch` renders each report with its index and sender task id,
in queue order, into the leading entry's content. The composed entry keeps the
leading entry's identity, model, plan mode, and reservation bookkeeping, so the
existing dispatch, in-flight marking, and lifecycle claim paths apply unchanged.

## Settlement

A batch has one prompt and N reserved claims. The SQL repository's private
atomic fold capability checks the exact task/session/incarnation, captured
session and lifecycle generations, attempt identity and every original fresh
unaccepted pending message before changing any claim. One existing-owner
transaction rewrites the leader with all reports and absorbs the trailing
claims. Absorption does not assert executor acceptance; only the leader may
subsequently dispatch or requeue the combined content.

A statement failure rolls the complete fold back, then returns the owned
originals through the existing FIFO requeue path. An already bound, accepted,
replaced or otherwise changed claim fails closed. A commit error cannot prove
rollback, so it preserves the existing claims and does not requeue or dispatch.
Startup follows current unknown-delivery recovery instead of replaying an
unaccepted report merely because it was reserved before a crash.

The historical fork rewrote and accepted folded claims in separate transactions.
This integration replaces that sequence with one atomic owner transaction so a
later folded write failure cannot lose or duplicate an earlier report. It does
not add a general settlement interface or change single-row delivery semantics.

## Drain integration

Both `drainQueuedMessageForPromptableSessionLockedForIdentity` and
`drainQueuedMessageForPromptableSessionLockedWithTaskAdmissionAndIdentity` use
`reserveAndDispatchQueueHeadWithCompletion`; the first delegates through its
`reserveAndDispatchQueueHead` wrapper with no completion effect. The completion
path preserves current main's predecessor settlement before dispatching a
successor. Settlement failure returns the owned reservations through the existing
FIFO requeue path. A single-row result takes the unchanged
`dispatchTakenQueuedMessageForSession` path, so non-batchable heads behave exactly
as before.

## Known limits

Batching does not remove `queue_full`. While one controller turn is long-running
and more reports arrive than a single bounded turn can absorb, admission still
rejects the surplus. Batching reduces how quickly a fan-out burst fills the
queue; it does not make exhaustion impossible, and no requirement claims
otherwise.

Folding also depends on the queue repository. The private optional atomic fold
capability is separate from current single-row settlement. A persistent
repository without it delivers one row. A repository that persists no dispatch
claims can compose in memory. Neither path raises queue capacity or changes
Auto-run policy.
