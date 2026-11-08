---
id: "03-parent-delivery"
title: "Deliver and expose parent alerts"
status: done
wave: 3
depends_on:
  - "02-candidate-classification"
plan: "plan.md"
requirements:
  - REQ-TASKS-CHILD-STALL-001
  - REQ-TASKS-CHILD-STALL-002
  - REQ-TASKS-CHILD-STALL-003
acceptance_criteria:
  - AC-TASKS-CHILD-STALL-001.3
  - AC-TASKS-CHILD-STALL-002.1
  - AC-TASKS-CHILD-STALL-002.2
  - AC-TASKS-CHILD-STALL-002.3
  - AC-TASKS-CHILD-STALL-002.4
  - AC-TASKS-CHILD-STALL-003.1
  - AC-TASKS-CHILD-STALL-003.2
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
---

# Task 03: Deliver and Expose Parent Alerts

## Summary and scope

Make keyed parent-primary queue admission retry-safe, recheck eligibility at
admission and dispatch, expose queue diagnostics/retry under existing workspace
authorization, and render one attributed system item. The producer enqueues
qualified alerts directly to the parent's primary session; no separate consumer
or receipt table is used. Likely files:
`apps/backend/internal/orchestrator/messagequeue/`, orchestrator queued dispatch
and event handlers, task handlers/service and backend wiring; existing web
conversation message rendering and locale catalogs only if required for the new
system-item metadata. Also covers folding of alerts queued for the same parent
(AC-TASKS-CHILD-STALL-002.6) and the wait plus one operator notification when
the parent primary session is missing, failed, or cancelled
(AC-TASKS-CHILD-STALL-002.5). Read the scoped backend/web guidance before
implementation.

## Exclusions

No interrupt, new session, priority lane, Auto-run override, terminal-child wave
rewrite, permanent tests, deployment, or automatic answer to a pending question.

## Implementation acceptance

1. Concurrent deliveries and queue-insert/acknowledgement crashes produce one
   queue/history item for the candidate, with current parent-primary authorization.
2. Busy/offline, Auto-run OFF, pending question, full queue, primary replacement,
   and stale eligibility preserve existing dispatch policy and recovery without
   bypass or workflow mutation. Existing question delivery is not duplicated.
3. History attribution, queue status, failed diagnostic/retry, and queue lag
   can be inspected; the child link is usable on desktop and phone.

## ASCII UI preview

UI-01, [combined preview](plan.md#ascii-ui-preview), inside the existing parent
conversation on desktop and the dedicated phone conversation:

```text
System: Child needs attention
[Open child: Child title]
Step: Review
Cause: Input required
Turn: <turn reference>
```

Reuse conversation scrolling and task navigation. Wrap content on phone; retain
keyboard focus and touch-operable links. Text is illustrative and localized.
Queued remains in the existing queue; delivery moves the item into history.
Pre-queue failed items are operator diagnostics, not fake history messages.

## Verification

From `apps/backend`, explicitly run temporary overlay scenarios following Task 01:

```text
go test -p 2 -tags fts5 -overlay "$env:TEMP\kandev-child-turn-stalled-signal\overlay.json" ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/task/service -run '^TestChildTurnDelivery'
```

Cases: repeated/reordered candidate, queue insert crash before acknowledgement,
consumed queue replay, offline/busy parent, queue full, Auto-run OFF, pending
question dedup, changed primary, reparent/workspace/entry race, read error,
retry exhaustion/operator retry, stale queue dispatch, folding of three alerts
into one queue item, and failed parent primary with later replacement. Assert one attributed
history item and no task/session lifecycle mutation by the notification path.

For any locale/rendering changes, from `apps/web`:

```text
pnpm run typecheck
pnpm run i18n:check
```

Use an isolated mock fixture and browser for focused desktop and phone checks:
inspect queued status, permit ordinary dispatch, read the attributed item, open
the child link, and verify navigation/focus with narrow viewport and keyboard.
Record viewport, entrypoint, outcomes, and any limitation. This is temporary
rendered verification, not authorization for a permanent E2E file or operating
instance changes. Clean owned fixtures/overlays after results are recorded.

## Dependencies, risks, results

Depends on Task 02. The queue has no external-model exactly-once guarantee;
preserve its ambiguous-dispatch recovery rather than issuing another prompt.
Question delivery and admission locking need explicit integration verification.
Results (2026-10-04): temporary overlays passed:
`TestChildTurnDeliveryFoldsAlertsForOneParent` (SQLite and memory queue: three
alerts fold into one item with Auto-merge disabled, keyed replay, non-alert
adjacency, full-queue fold), and orchestrator scenarios with `-race` and the
package goroutine-leak check: missing signal with folding and dispatch-time
pruning, failed parent with one operator notification and delivery to the
replacement primary, unanswered parent question (held while queued or running,
one reminder after the parent settles), answered question suppression, and
producer start/stop. A temporary happy-dom render check of the alert summary
passed; a real desktop and phone browser check was not run.
