---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
---

# Cancelled turn completion: dispatch and stream coordination

## Boundary and requirement mapping

The task system owns prompt admission, turn cancellation, and workflow completion.
This design supplements the [existing requirement](../requirements/workflow-cancelled-turn-completion.md).
It covers the concurrency contract in AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
and the task reconciliation outcome in criterion 001.2.

The [queued ownership design](queued-session-ownership.md#replay-and-reconciliation-locking)
continues to own task admission and review reconciliation locking.
The [UI progress design](../../ui/system-design/cancel-turn-progress.md) owns the
existing cancellation projection and desktop/phone presentation.
This design describes the implemented repair and the compatibility invariants that govern it.

## Existing components

- `orchestrator.Service.promptTask` claims queued or lifecycle dispatch under the session guard.
- `preparePromptAdmissionCallback` releases the physical guard during callback-producing preparation, then reacquires it to revalidate logical dispatch ownership.
- `preparePromptDispatchCallback` releases the guard after provider acceptance and ownership publication.
- `executor.Executor` and `backendapp.lifecycleAdapter` carry admission and dispatch callbacks into `lifecycle.SessionManager`.
- `SessionManager.sendPrompt` serializes prompts with `AgentExecution.promptMu`.
- `preparePrompt` creates the effective prompt; `sendPrompt` materializes attachments and flushes prior stream/history before admission. It appends the new user message to history only after provider acceptance.
- `admitPrompt` allocates the prompt generation and performs the local buffer reset after admission.
- `streamCoalescer.add` and `flushBoundary` serialize publication with `emitMu`.
- `handleAgentStreamEvent` acquires the session guard during synchronous event publication.
- `runExplicitCancellationOwned` uses the same guard before calling lifecycle cancellation.

The former dispatch path held the session guard while waiting for `emitMu`.
An active publication held `emitMu` while waiting for the session guard.
Cancellation could not acquire that guard after this cycle formed.

## Preparation and final admission

Separate callback-producing preparation from final provider admission.
Keep the existing dispatch reservation as logical ownership while the physical session guard is released.
Do not add another queue, durable lease, or cancellation registry.

1. Under the session guard, capture the dispatch reservation, session incarnation,
   execution, turn, resume attempt, and workflow-entry binding where applicable.
2. Release the physical guard before lifecycle prompt serialization, prior completion
   waits, draining prior streamed assistant history, attachment preparation, or provider restart.
3. Drain the previous stream boundary through the existing ordered publication path.
   Keep each old frame's execution, turn correlation, and prompt generation intact.
4. At a new internal admission callback, reacquire the session guard and validate the
   captured ownership against current state. Return an error if it was cancelled or superseded.
5. Only the admitted attempt may allocate its new prompt generation and dispatch.
   Persist the new user message only after provider acceptance; rejection must leave it out of
   history and must not fail the still-accepted execution. Local buffer reset after admission
   must not flush streams or invoke guarded event consumers.
6. Retain the guard through provider acceptance and the existing ownership-publication callback.
   Release it before waiting for turn completion. Release it on every rejection or dispatch failure.

The callback is an internal runtime seam, not an HTTP or ACP contract.
Extend the existing callback path through executor, adapter, manager, and session manager.
Model-switch restarts send their initial prompt asynchronously, so register the same final
admission check with the startup callbacks. Fence an in-place model mutation and its persisted
snapshot with the cancellation revision; release the session guard before provider restart.
The initial-prompt callback reacquires and validates it after lifecycle preparation.
When a queued model-switch worker returns before that callback, keep its accepted reservation in
an awaiting-admission phase. Send Now remains blocked until provider acceptance promotes that
exact reservation to live, or admission failure clears only that reservation.
Production dispatch that requires admission must fail closed if the runtime lacks that seam.
Do not silently fall back to an implementation that skips revalidation.

Split `preparePrompt` so generation allocation and local reset do not precede final admission.
Moving only `flushBoundary` earlier is insufficient if another guarded path still invokes synchronous publication.
Audit model switches, fresh fallback, resumed prompts, queued prompts, and lifecycle automation.
Preserve accepted-turn ownership transfer in the [resume recovery design](../../agents/system-design/session-recovery-failures.md).

During the unlocked interval, cancellation may invalidate the captured dispatch reservation.
A completion, deletion, archive, replacement execution, or workflow move may also supersede it.
Final admission must reject each stale attempt without sending its prompt or closing a successor turn.
Keep existing queue disposition and retry rules; do not convert cancellation into automatic replay.

Capture the cancellation-pending revision when `promptTask` begins, before session preparation or a
model switch can release the physical guard. Explicit pause advances this revision even after its
operation is removed from the active cancellation registry. Restart admission and an in-place
switch continuation compare the captured revision under the session guard. A mismatch rejects the
attempt before provider dispatch; the in-place path checks before it creates or claims a successor
turn, and both paths check again at final provider admission. Do not rely only on the active
cancellation operation or a resume attempt, because an existing live session may have no resume
attempt and a completed pause no longer has an active operation.

## Locking invariants

| Boundary | Permitted work | Forbidden waits |
| --- | --- | --- |
| Session guard during preparation | Capture and validate ownership | Stream flush, prior prompt completion, provider restart, synchronous guarded callbacks |
| Lifecycle prompt serialization before admission | Prior stream drain and preparation | Holding the session guard while waiting for publication |
| Final admission through acceptance | Revalidation, generation allocation, transport acceptance, ownership publication | Turn completion or synchronous stream-consumer re-entry |
| Stream publication | Existing ordered delivery and guarded persistence | A reverse dependency from guarded dispatch to the same publisher |
| Cancellation reconciliation | Captured-identity validation and local writes | Lifecycle cancellation while holding the session guard |

Preserve `emitMu` ordering and `handleAgentStreamEvent` serialization.
Removing either lock or making each publication an independent goroutine would weaken existing guarantees.
Keep first-chunk, boundary flush, terminal-frame, history, and generation-isolation behavior.
Admission rejection uses a cleanup callback distinct from provider delivery failure, so a rejected
replacement cannot mark an already-running execution failed. Preparation failure callbacks must
run after any dispatch guard is released.

## Bounded cancellation guard acquisition

The existing `cancellationOperationTTL` remains the service-owned operation deadline.
Every guard acquisition performed by that operation must observe its context.
This includes initial preparation, reacquisition after lifecycle cancellation, explicit joined
reconciliation, and source-specific actions in `finishCancellationWithActions`.

Use the same reference-counted guard registry with a FIFO, context-aware lock.
`LockContext` places a blocked caller in the waiter queue, and unlock hands ownership directly to
the oldest live waiter. `TryLock` fails while an owner or queued waiter exists, so nonblocking
callbacks cannot bypass cancellation. Check context before acquisition and after a waiter is
granted; release immediately if the deadline won. Do not poll, allocate repeated wait timers, or
start a goroutine that can acquire the lock after its caller returns. Keep ordinary stream-handler
serialization unchanged.

If acquisition expires, return the operation error and settle every joined waiter.
Skip source-specific mutations that require the unavailable guard.
Remove only the matching operation and release its projection reference exactly once.
Publish the existing revisioned pending=false state; do not synthesize successful cancellation.
A late lock release cannot dispatch a joined action, cancel a successor, or clear a newer operation.
A later user request may retry through ordinary authorization and admission.

The initiating request may disconnect without aborting the accepted operation.
Runtime cancellation escalation and its timeouts remain unchanged.
If runtime cancellation succeeded but reconciliation could not acquire its guard,
return an error and preserve existing retry reconciliation instead of claiming completion.

## Compatibility and persistence

No database, public API, ACP message, runtime flag, or UI layout changes are required.
The design preserves cancellation-driven workflow policy, clarification barriers,
queued-message parking, sibling precedence, and the accepted execution's continuity.
UI clients keep using the existing cancellation boolean and revision.
The same shared control settles on desktop and phone.

## Verification

Use deterministic barriers to overlap a real coalescer publication with queued prompt preparation.
Assert ordered persistence, provider dispatch, and successful cancellation through the service path.
Exercise cancellation and replacement during unlocked preparation, including model-switch fallback.
Hold the session guard past an injected short operation deadline and assert waiter settlement,
pending release, retry, and absence of delayed side effects after the guard becomes available.

The [fix package](../../../plans/queued-prompt-pause-deadlock/plan.md) defines exact tests and commands.
Existing [cancellation responsiveness](../../../plans/turn-cancellation-responsiveness/plan.md),
[resumed-turn cancellation](../../../plans/resumed-turn-cancellation/plan.md), and
[ceiling replay](../../../plans/ceiling-replay-cancellation-deadlock/plan.md) results remain historical evidence.

## Decisions

The existing [backend-owned progress decision](../../../decisions/2026-08-03-backend-owned-cancellation-progress.md)
and [explicit cancellation decision](../../../decisions/2026-08-02-explicit-user-cancel-completion.md)
remain authoritative. This local repair preserves their contracts; its rationale fits this design.
