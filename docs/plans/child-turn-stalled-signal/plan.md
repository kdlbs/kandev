---
created: 2026-10-01
status: implemented
requirements:
  - REQ-TASKS-CHILD-STALL-001
  - REQ-TASKS-CHILD-STALL-002
  - REQ-TASKS-CHILD-STALL-003
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
legacy_specs: []
---

# Child-turn Stalled Signal Design Package

## Scope and status

This package supplies the reviewable requirements, design, plan, and work
orders for the child-turn stalled signal. The owner requested implementation
on 2026-10-04; the work orders below are the implementation scope.

Tasks owns the durable parent/child, turn, entry, and notification contract.
The [requirements](../../specs/tasks/requirements/child-turn-stalled-signal.md)
are active and the [system design](../../specs/tasks/system-design/child-turn-stalled-signal.md)
is current. Existing stall recovery and peer-report batch designs are
adjacent, not dependencies.

## Owner decision

**Owner selection: A (producer-qualified direct parent delivery), recorded
2026-10-03.** The producer classifies each live settlement and enqueues
qualified alerts directly to the parent's primary session. No internal
candidate feed, monitor classifier, consumer lease, or separate receipt storage
is used.

The durable producer-versus-monitor boundary and its tradeoffs are recorded in
[ADR-2026-10-03-child-turn-stalled-producer-delivery](../../decisions/2026-10-03-child-turn-stalled-producer-delivery.md).

| Dimension | A: producer-qualified direct parent item |
| --- | --- |
| Parent demand | One item per producer-qualified candidate |
| Ordinary turns | Suppressed in producer |
| Slot/token cost | Up to approximately 45 items/day under supplied candidate estimate |
| Other processing | Eligibility/classification at producer; durable queue admission |
| Failure surface | Fewer components; classifier coupled to turn handling |
| Delay | Prompt queue admission after settlement |
| Serial parent | FIFO work can still be delayed by actionable alerts |

The supplied historical sample is 135 candidate segments / 2,335 turns over
three days: 5.78%, 45 candidate segments/day, 778.33 turns/day. These are
unverified task-context observations, not a production forecast. The child
subset and actual actionable fraction are unknown. Do not present 45 as a
hard cap.

For a mean parent alert turn of `t` minutes and `p` billable tokens, A's sizing
case is `45*t` slot-minutes and `45*p` tokens/day. Illustratively, at two
minutes each, 45 alerts occupy 90 slot-minutes/day; this is not a measured
runtime. Monetary cost cannot be calculated without model pricing and
input/output counts. No paid classifier is assumed or authorized; one would
require a revised cost comparison and explicit scope.

Selecting A moved the classifier to the producer. The 2026-10-04 implementation
request added three owner requirements, recorded as AC-TASKS-CHILD-STALL-001.4,
002.5, and 002.6: a reminder for an unanswered parent question, a wait plus
operator notification when the parent primary session is unavailable, and
folding of alerts queued for the same parent. Folding bounds parent turns when
alerts arrive while the parent is busy; the 45/day figure stays a sizing input,
not a cap.

## Delivery order

1. [Task 01: Capture live settlement and durable identity](task-01-live-settlement.md).
2. [Task 02: Classify at producer](task-02-candidate-classification.md), after 01.
3. [Task 03: Deliver and expose parent alerts](task-03-parent-delivery.md), after 02.

The owner's 2026-10-04 request authorizes execution in this order. Settlement
capture, classification, and queue delivery form sequential verification
boundaries, not independently released features; they ship as one feature.

## ASCII UI preview

UI-01: Existing parent conversation and queued-message region. Shared content
composition for desktop and phone; phone uses its dedicated task conversation,
single vertical scroll, wrapping labels, and accessible child navigation.

```text
Parent conversation
  System: Child needs attention
  [Open child: Child title]
  Step: Review
  Cause: Input required
  Turn: <turn reference>

Queued messages
  Child needs attention - Queued
```

Structure and attribution are required by AC-TASKS-CHILD-STALL-003.1; text and
spacing are illustrative, localized at implementation. Delivered items live in
history. Pre-queue failure and producer-to-queue lag are operator diagnostics, not a
fabricated delivered chat item. UI-01 does not add an interactive retry control
to the parent conversation. Verify desktop and phone readability and opening
the same child from the existing conversation route.

## Verification strategy and risks

Work orders specify targeted existing tests and explicitly invoked temporary Go overlays for new
behavior. Put temporary cases under the OS temporary directory, outside normal
test discovery; remove overlays after retaining concise results. Do not commit
new permanent tests without separate authorization. Focused rendered desktop
and phone checks cover UI-01 after implementation; do not run the whole suite.

Required scenario coverage across 01-03: successful ordinary turn, every cause,
optional versus required completion signal, abandonment, rejected dispatch,
same-step re-entry, deferred transition race, duplicate/reordered event, busy or
offline parent, Auto-run OFF, pending question, unanswered parent question
reminder, failed or missing parent primary session, folding of several alerts
for one parent, queue full, failed reads, crash
at queue insertion, producer restart, retry exhaustion, retention replay, and
zero workflow mutations. An unresolved external dispatch is not proof of an
exactly-once model turn. New source symbols and test names in work orders are
proposed, not existing coverage.

Key risks are live-end provenance, atomic entry capture, duplicate question
delivery, and the queue/insertion crash window. None is solved by treating
`turn.completed` as a workflow transition. The final integration must preserve
ordinary turn latency and existing task admission ownership.

## Design validation

On 2026-10-01, `python -X utf8 scripts/list-docs.py validate` passed (338
decisions, 1,285 specifications); `python -X utf8 scripts/lint-spec-files.py
--all` passed; `python -X utf8 scripts/lint-spec-files.test.py` passed all 36
tests. Catalog lookup discovers both new task specifications. Scoped
`git diff --check` passed, but does not examine untracked new files; those
files were read separately for links, encoding, and whitespace. Implementation
and rendered verification are not run and all work orders remain pending.

On 2026-10-04, after recording the owner's implementation request and added
requirements, `python -X utf8 scripts/list-docs.py validate` (344 decisions,
1,340 specifications) and `python -X utf8 scripts/lint-spec-files.py --all`
passed.
