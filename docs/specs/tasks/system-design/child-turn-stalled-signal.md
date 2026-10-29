---
status: current
system: tasks
requirements:
  - REQ-TASKS-CHILD-STALL-001
  - REQ-TASKS-CHILD-STALL-002
  - REQ-TASKS-CHILD-STALL-003
  - REQ-TASKS-CHILD-STALL-004
---

# Child-turn Stalled Signal System Design

## Boundary and evidence

The task system owns the candidate and delivery identity. The following source
was inspected on 2026-10-01 and 2026-10-04.

- `internal/task/service/service_turns.go`: `StartTurn` and `ReserveTurn` enter
  `createTurn`; `CompleteTurn` commits completion before publishing
  `events.TurnCompleted`. `AbandonOpenTurns` publishes that event too.
- `internal/task/repository/sqlite`: `CreateTurnWithStepStamp` and its
  conversation-receipt variant already stamp the workflow step at turn start in
  the same transaction as the turn insert.
- `internal/events/bus/bus.go`: `EventBus` exposes publication/subscription but
  no durable consumer acknowledgement or replay contract. A bus publish alone
  cannot satisfy this feature's recovery requirement.
- `internal/task/models/step_transitions.go` and
  `internal/task/repository/sqlite/step_transitions.go` own the task ledger.
  Use its task entry identity, not a step label or timestamp comparison. See
  [transition ledger](workflow-task-step-transition-ledger.md).
- `internal/mcp/handlers/parent_question.go`: an autopilot child's parent
  question is a `clarification_request` message with
  `parent_question_status=pending`; the question prompt is dispatched to the
  parent primary session through the ordinary task-message path.
- `internal/office/repository/sqlite/wake_receipts.go` tracks terminal child
  waves. Do not reuse its `parent_child_wake_receipts` identity for turn alerts.
- `internal/orchestrator/messagequeue/` owns queue persistence and recovery.
  Keyed admission (`queue_admission_receipts`) commits the receipt with the
  queue insertion or fold, and returns the original queue item on replay even
  after the item was consumed. `repository_dispatch_recovery.go` notes that the
  agent protocol lacks a prompt idempotency key. This design guarantees one
  queue/history item, not exactly-once external model execution after an
  ambiguous dispatch.

The owner selected option A on 2026-10-03: the producer classifies each live
settlement and enqueues qualified alerts directly to the parent's primary
session. No internal candidate feed, monitor classifier, consumer lease, or
separate receipt storage is used.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-CHILD-STALL-001 | Settlement capture; producer classification; parent questions; delivery boundary |
| REQ-TASKS-CHILD-STALL-002 | Durable admission; parent availability; folding; delivery boundary |
| REQ-TASKS-CHILD-STALL-003 | Visibility and retention |
| REQ-TASKS-CHILD-STALL-004 | Current-primary routing and idle-notice reconciliation |

## Settlement capture

The source turn row is the durable candidate record. Two compact metadata keys
live on it; no table or column is added.

- `child_stall_start` is written in the turn-insert transaction when the task
  has a parent: parent task ID, workspace ID, and the latest
  `task_step_transitions` ID. A missing ledger entry records no transition ID,
  which makes the turn unknown and ineligible. Tasks without a parent get no
  key and are never scanned.
- `child_stall` holds the settlement discriminator and the producer's state.
  Orphan abandonment writes `settlement=abandoned` in the same transaction as
  the abandon update. An explicit user cancellation writes
  `settlement=cancelled` on the captured turn before it is settled. Any other
  completion is a live settlement. Legacy turns without `child_stall_start`
  are ineligible; live provenance is never reconstructed from `had_output` or
  equal timestamps.

Existing `turn.completed` semantics stay unchanged. The event only wakes the
producer early; recovery never depends on it.

## Producer classification

An orchestrator-owned producer goroutine (`Start`/`Stop`, bounded by its
context) runs a pass on startup, on a fixed interval, and shortly after a
`turn.completed` event. Each pass reads child-task turns completed within the
scan window whose `child_stall` state is not terminal, using dialect-neutral
SQL and filtering metadata in Go. A turn is classified only after a short
settle grace, so the orchestrator has finished applying turn-end transitions.

Classification uses only structured evidence already available in the task
system. No external monitor, paid classifier, or unstructured transcript
interpretation is used.

| Cause | Required evidence | Suppression |
| --- | --- | --- |
| `input_required` | Unresolved clarification/input message in the settling session | Already answered/resolved input |
| `quota` / `execution_error` | Session failed with a persisted route error class (quota) or terminal error | Cancellation or unrelated historical error |
| `missing_completion_signal` | The current step requires the completion signal and no signal is pending after turn-end handling settled | Optional signal, ordinary success, or pending transition processing |

Cause precedence for a single item is input, quota/error, then missing signal;
multiple causes do not create multiple items. Do not use no-output, elapsed
time, or the observed 5.8% rate as classification.

Before any cause applies, the settlement predicate must hold: live settlement,
child not archived or terminal, captured parent and workspace unchanged, no
active turn in the settling session, and the latest transition ID equal to the
captured one. A known state change suppresses the old candidate, even if a
later turn stops again; the later turn gets its own identity. Read failures
leave the candidate retryable. Classification never mutates workflow, task, or
session state and never acquires a parent slot.

The producer persists the outcome on the source turn: `suppressed` (with a
reason), `unknown`, `held` (parent question), `qualified`, `waiting_parent`,
`delivered`, or `failed`.

## Parent questions

A pending parent question already reached the parent through its own delivery.
When the settling child's input is a pending parent question, the candidate is
`held` and linked to the question ID instead of producing a second prompt.

A held candidate is promoted to `qualified` only when all of the following
hold: the question is still pending; no queued parent item carries the same
question ID; the parent primary session has no active turn and is idle; and a
parent turn that started after the question was created has settled at least
the settle grace ago. An answered question suppresses the candidate. This
yields at most one reminder per stalled child turn, so an unanswered question
cannot loop the parent. The producer never answers or resolves the question.

## Durable admission

The producer enqueues qualified alerts directly to the parent's primary
session using the existing message queue. The queue item and its admission
receipt are the durable delivery record.

Deduplication uses the stable operation identity
`(child_task_id, session_id, turn_id, step_transition_id_at_turn_start)`,
hashed into the caller-owned queue admission ID. Keyed admission writes the
receipt with the insertion or fold and returns the existing queue item on
replay. After admission the producer marks the source turn `delivered` with the
queue item ID and parent session. A crash between the two steps replays the
same keyed admission. A replacement parent session uses a new queue identity,
so a `delivered` source-turn marker is the cross-session guard.

Transient failures retry with waits of 1, 5, 30, and 120 seconds, up to five
attempts. Failed candidates keep their original identity and metadata and
remain inspectable and retryable by the operator.

## Missing-signal routing

A `missing_completion_signal` candidate does not wake the parent on its own.
The producer queues one short reminder for the child's own session through the
same keyed admission path, so the child's normal queue policy, capacity, and
Auto-run rules still govern dispatch. The parent alert that the operator turned
into a child nudge is thereby removed.

Escalation is bounded by a consecutive count keyed to the child session and the
workflow entry captured at turn start (`child_stall_nudge` session metadata,
updated with a single-key atomic write). The count advances only after a
successful delivery, so a retried candidate is not counted twice. The first two
consecutive nudged stalls on one entry stay with the child; the third alerts the
parent once and marks the streak escalated, after which further stalls on that
entry are suppressed. A change of workflow entry or cause starts a new streak.
Because a replaced child session carries no streak, its first stall after
replacement is nudged afresh. Every other cause continues to use direct parent
delivery unchanged.

The new outcomes are recorded on the existing counters: the child reminder under
`task_child_stall_outcome_total{nudged_child}`, the escalation under the existing
`delivered` outcome, and a suppressed escalated streak under reason
`nudge_escalated`; the cause counter still counts every missing-signal candidate.

## Folding

Alerts are queued with `queued_by=server` and alert metadata. A new alert folds
into the parent queue's tail when that tail is a pending, undispatched alert
for the same parent task, regardless of the session's Auto-merge setting; the
same rule lets an alert fold into a tail alert when the queue is full. Folding
concatenates the prompt text and unions the alert list in metadata. Each
candidate's admission receipt points at the folded item. Folding never crosses
a non-alert item, so FIFO order relative to other input is preserved.

## Parent availability

Resolve only the parent's current primary session. When it is missing, failed,
or cancelled, the candidate becomes `waiting_parent`, and the producer sends
one operator notification (`task.child_stall_undeliverable`) for that candidate
through the existing notification providers. Later passes deliver it once the
parent has a promptable primary session. The producer never creates a session
or selects a sibling.

## Delivery boundary

Immediately before keyed queue admission, re-read child relationship/workspace,
archive/terminal status, settling session's active turn, workflow entry, and
cause evidence, under the parent session's queue admission lock. A transition
committed first suppresses the alert; a transition after admission cannot
retroactively invalidate an already observed history item.

Preserve Auto-run OFF, pending questions, WIP, cancellation, queue capacity,
FIFO, session incarnation, and normal prompt admission. No interrupt, priority
bypass, or duplicate ordinary child report. When a queued alert is about to be
dispatched, each folded candidate is rechecked; stale candidates are dropped
from the prompt, and an alert with no eligible candidate is discarded without
creating a model turn.

## Visibility and retention

Reuse the parent conversation and queued-message surfaces for the attributed
system item. Alert metadata supplies child link, step, cause, turn reference,
and queue status; UI copy uses localization. No new dashboard is proposed.
Operators inspect and retry candidates through a narrowly scoped task operation
using existing workspace authorization. Expose outcome and cause counters,
pending count, and oldest pending age through expvar and structured logs.
Task/session identifiers are not metric labels.

The content-only phone path uses the existing dedicated mobile task conversation
(`apps/web/components/task/task-layout.tsx` and `components/task/mobile/`).
Keep its single conversation scroll owner and navigation; child links are
keyboard accessible and have coarse-pointer touch targets. No new overlay or
desktop layout compressed into a phone is needed. See the plan's UI-01 preview.

Resolved markers stay on their source turn for the turn's lifetime and are a
few fields each; they are deleted only with the task. The producer scans only
turns completed within seven days, and a resolved marker prevents an old event
or scan from recreating its item. Queue admission receipts follow the queue's
existing task/session deletion. No transcript payload is copied into the queue
item.

## Current-primary routing and idle-notice reconciliation

A delivered candidate is not fully resolved at queue admission: its source-turn
marker stays scannable while its notice is still pending, so the producer's
existing pass can keep it pointed at the parent's current promptable primary.
This replaces the previous behavior where `delivered` was terminal and the queue
item could be left on a session that later stopped being the parent's primary.

Each pass reconciles every delivered candidate in the scan window:

- A candidate whose source turn no longer qualifies as a stall is resolved, and
  its notice entry is withdrawn.
- When the parent task is archived or terminal, the notice can never be
  delivered, so its entry is withdrawn and the candidate is resolved.
- When the parent has no promptable primary, the notice is withdrawn from the
  session queue and the candidate returns to `waiting_parent`, preserving the
  single operator notification. The candidate stays on the source turn and is
  delivered when the parent next has a promptable primary; it no longer occupies
  a dashboard queue row meanwhile.
- When the parent's current primary is promptable and is not the session holding
  the notice, the notice is re-homed to that primary through the existing
  keyed queue admission, so content, metadata, and the folded alert list are
  preserved and the destination's own fold/merge rules apply. A re-home with no
  remaining entry resolves the candidate.
- When the notice is already on the promptable primary, it is kept; it is
  resolved only once the entry is gone (dispatched, withdrawn, or consumed).

Reconciliation is idempotent and bounded by the existing producer scan window
(`childStallScanWindow`); a scan read failure leaves the candidate pending so
the next pass retries. It is the same producer goroutine, so no second
watchdog, collector, or scheduler is introduced.

Alongside the per-candidate reconcile, each session that held a pending notice
runs the existing stale/superseded prune, so equivalent notices for the same
child collapse to one item even when the session never drains. Both paths are
gated on `IsChildStallAlert`, so user, agent, workflow, and non-alert server
entries, questions, answers, and approvals are never created, reordered, or
removed. Withdrawal and re-home record bounded outcomes
(`withdrawn_parent_gone`, `held_waiting_parent`, `rehomed_notice`) on the
existing `task_child_stall_outcome_total` counter and stay out of metric labels.

## Decision status and compatibility

The owner selected option A on 2026-10-03; its boundary and tradeoffs are
recorded in [ADR-2026-10-03-child-turn-stalled-producer-delivery](../../../decisions/2026-10-03-child-turn-stalled-producer-delivery.md).
Existing terminal-child waves,
notification-provider events, and draft peer-report batching stay independent.
