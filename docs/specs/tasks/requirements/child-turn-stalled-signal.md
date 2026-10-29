---
status: active
system: tasks
created: 2026-10-01
owners:
  - task-lifecycle
---

# Child-turn Stalled Signal Requirements

## Overview

A parent needs an attributed notification when a child finishes a live turn
in an actionable stalled segment. Ordinary turns must not consume the serial
parent's agent capacity. Tasks owns this contract because child relationships,
turn identity, workflow entry, and parent-session delivery are task primitives;
Office terminal-child waves remain a separate contract.

The owner selected option A (producer-qualified direct parent delivery) on
2026-10-03 and requested implementation on 2026-10-04. The producer classifies
each live settlement and enqueues qualified alerts directly to the parent's
primary session. No internal candidate feed, monitor classifier, consumer
lease, or separate receipt storage is used. See the
[plan](../../../plans/child-turn-stalled-signal/plan.md).

## Terms

- **Live settlement:** a previously executing turn has ended, excluding orphan
  abandonment, rejected dispatch, and cleanup-only closure.
- **Workflow entry:** one transition generation, not merely a step name. Leaving
  and returning to the same step creates a different entry.
- **Candidate:** a live settlement that the producer has classified as an
  actionable stall.
- **Alert:** one classified item queued for the current parent's primary session.
- **Parent question:** a question an autopilot child asked its direct parent
  with `ask_parent_question_kandev`; it stays pending until the parent replies
  with `reply_to_question_id`.

## Requirements

### REQ-TASKS-CHILD-STALL-001: Eligible stalled settlements

**Intent:** Surface actionable child stalls without equating turn end with
workflow completion.

- **AC-TASKS-CHILD-STALL-001.1:** A live settlement shall qualify only while the
  child is non-archived and non-terminal, retains its captured parent and
  workspace, has no active execution or turn in the settling session, and
  retains the workflow entry captured at turn start.
- **AC-TASKS-CHILD-STALL-001.2:** An unresolved question/input, execution
  error/quota, or absence of a completion signal required for that turn shall
  be actionable. Successful ordinary turns, optional signal absence, explicit
  cancellation, and abandonment shall not alert. Unknown evidence shall not
  be classified as failure or success.
- **AC-TASKS-CHILD-STALL-001.3:** Eligibility shall be checked again immediately
  before parent delivery. A committed deferred transition, same-step re-entry,
  reparenting, archive, terminal state, or resumed execution shall prevent stale
  delivery. Failed reads shall defer delivery without workflow mutation.
- **AC-TASKS-CHILD-STALL-001.4:** A child waiting on a pending parent question
  shall not alert while the original question delivery is still queued or
  running for the parent. Once a parent turn that started after the question
  settles, the parent primary session is idle, and the question is still
  pending, the waiting child shall produce one alert for that child turn. An
  answered question suppresses the alert.

### REQ-TASKS-CHILD-STALL-002: Durable selective delivery

**Intent:** Preserve actionable alerts across failures without duplicate items
or unauthorized wakes.

- **AC-TASKS-CHILD-STALL-002.1:** Repeated or reordered observations and retries
  of the same child/session/turn/start-entry identity shall create at most one
  parent item, including crashes between queue insertion and acknowledgement.
- **AC-TASKS-CHILD-STALL-002.2:** An eligible alert shall use only its parent's
  primary session in the same workspace. An offline or busy parent shall retain
  a queued item under existing admission and Auto-run rules, without interruption,
  forced session creation, queue reordering, or bypass of pending questions.
- **AC-TASKS-CHILD-STALL-002.3:** Lost event delivery and producer restart shall
  leave candidates recoverable. Transient delivery failures shall retry with
  bounded backoff; exhausted retries shall remain visibly failed and available
  for operator retry using the original identity.
- **AC-TASKS-CHILD-STALL-002.4:** Ingestion, classification, retry, and notification
  shall not move either task's workflow, complete a task, answer a question,
  cancel a session, or authorize a new agent action independently of the existing
  queue dispatch policy.
- **AC-TASKS-CHILD-STALL-002.5:** When the parent has no primary session or its
  primary session is failed or cancelled, the candidate shall wait for the
  parent's next promptable primary session and the operator shall receive one
  notification for that candidate. The producer shall not create a session or
  select a sibling session.
- **AC-TASKS-CHILD-STALL-002.6:** A new alert for a parent session whose queue
  tail is a pending, undispatched alert shall fold into that item, so alerts
  accumulated while the parent is busy are handled in one parent turn. Each
  folded candidate keeps its own replay identity.
- **AC-TASKS-CHILD-STALL-002.7:** A `missing_completion_signal` candidate shall
  be delivered as one short reminder to the child's own session, not to the
  parent. The parent shall receive one alert only after the same child stalls on
  the same workflow entry for three consecutive nudged turns; further stalls on
  that escalated streak shall not alert the parent again. The consecutive count
  is keyed to the child session and the captured workflow entry, and resets when
  the entry or the cause changes. Every other cause keeps direct parent delivery.

### REQ-TASKS-CHILD-STALL-003: Attribution and operating visibility

**Intent:** Make the alert and its delivery state reviewable.

- **AC-TASKS-CHILD-STALL-003.1:** The parent history shall show an attributed
  system item containing the child link, workflow step, cause category, and
  turn reference. The same information shall be readable and the child link
  operable on desktop and phone, without a hidden prompt-only delivery.
- **AC-TASKS-CHILD-STALL-003.2:** Operators shall be able to inspect queued,
  delivered, failed, and suppressed outcomes plus producer-to-queue lag and
  bounded error reasons, without reading credentials or raw message bodies.
- **AC-TASKS-CHILD-STALL-003.3:** Resolved queue items and source-turn markers
  shall use compact metadata with bounded retention, not a new permanent
  per-turn payload archive.
  Cleanup shall not permit replay to recreate an already resolved parent item.

### REQ-TASKS-CHILD-STALL-004: Current-primary routing and idle-notice lifecycle

**Intent:** Keep a delivered child-attention notice pointed at the parent's
current promptable primary and bound the review backlog it can create, without
ever touching a user message, a question, an answer, an approval, or another
sender's queue entry.

- **AC-TASKS-CHILD-STALL-004.1:** A delivered notice shall remain bound to the
  parent's current promptable primary. While a candidate is delivered, the
  producer's existing scan shall recheck whether its receiving session is still
  that primary; when it is not, the producer shall reconcile the notice —
  re-home it to the current promptable primary when one exists, otherwise
  withdraw it from the session queue and hold the candidate for the parent's
  next promptable primary.
- **AC-TASKS-CHILD-STALL-004.2:** A notice shall be withdrawn from the queue when
  the parent task is archived or terminal, or when the source child turn no
  longer qualifies as a stall. Withdrawal removes only child-attention alert
  entries.
- **AC-TASKS-CHILD-STALL-004.3:** Equivalent notices for the same child task
  under the same parent shall collapse to one pending item; a newer eligible
  notice supersedes an older one, and the surviving item keeps the newest child
  state, cause, and question reference. This reconciliation shall not create,
  reorder, or remove user, agent, workflow, or non-alert server entries, and
  shall not touch questions, answers, or approvals.
- **AC-TASKS-CHILD-STALL-004.4:** Reconciliation shall run inside the existing
  producer scan with no new watchdog, collector, or scheduler; it shall be
  idempotent, and a read failure shall leave the notice retryable rather than
  dropping it.
- **AC-TASKS-CHILD-STALL-004.5:** Withdrawal and re-home shall be observable
  through the existing child-attention outcome counters, using a bounded reason
  set with no task, session, or turn identifier as a label, and through
  structured logs.

## Exclusions

No deployment, settings changes, public webhook, new autonomous recovery
policy, transcript scanning model, parent priority lane, automatic answer to a
parent question, or replacement of Office all-children-terminal wake receipts
is part of this contract. Traffic observations are sizing input, not runtime
thresholds. Existing question and task-completion contracts remain
authoritative.
