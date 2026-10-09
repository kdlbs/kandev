---
status: draft
system: tasks
requirements:
  - REQ-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001
---

# Deferred Move on_enter Dispatch Identity System Design

## Purpose and boundaries

The task system owns the workflow-step transition ledger and the `on_enter`
dispatch that follows a committed transition. This design covers only the
identity a deferred move's own `on_enter` dispatch presents to the existing
staleness guard. It does not change the guard's decision logic, the ledger's
write path, or the forward-move dispatch path.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001` | [Transition identity threading](#transition-identity-threading) |

## Components and responsibilities

`workflowStore.applyTransition` (private, `orchestrator/workflow_store.go`)
computes and durably records the ledger transition ID for every
step-changing write, including a deferred move, via `updateTaskTx`. Its
public wrappers `ApplyTransition` (part of `engine.TransitionStore`) and
`ApplyDeferredMoveTransition` previously discarded that value.

`Service.applyPendingMove` and `Service.processStepExitAndEnterForDeferredMove`
(`orchestrator/event_handlers_workflow.go`) drive the deferred-move commit and
its own `on_enter` dispatch call.

`processOnEnter`'s staleness guard (`workflowEntryDispatchIsCurrentForSession`
/ `workflowEntryDispatchMayReplaceRoute` in `ceiling_entry.go`) decides
whether an `on_enter` dispatch is the current entry for a session, using the
transition/entry identity the caller supplies.

## Transition identity threading

`workflowStore.applyTransition`, `ApplyDeferredMoveTransition`, and the
private `applyTransition` return the `int64` ledger transition ID alongside
their existing error. `ApplyTransition`'s public, broadly-implemented
interface signature is unchanged; only `ApplyDeferredMoveTransition`, which
has exactly one production call site, gains the new return value.

`applyPendingMove` captures the transition ID from both the atomic
(`ApplyDeferredMoveTransition`) and unfenced-fallback (`applyTransition`)
commit branches, and threads it through `processStepExitAndEnterForDeferredMove`
into `processOnEnter`, replacing a hardcoded `0`. The staleness guard's
existing `workflowEntryDispatchMayReplaceRoute` escape hatch then recognizes
the dispatch as current by matching it against `task.WorkflowStepTransitionID`,
the same identity the transition write already produced.

This changes only what identity a deferred move's own dispatch presents to
the guard. It does not change the guard's rejection logic for a genuinely
stale callback, and does not affect the forward-move (`E1`) path, which
already threads its own transition identity through
`stepentry.AllocationResult`.
