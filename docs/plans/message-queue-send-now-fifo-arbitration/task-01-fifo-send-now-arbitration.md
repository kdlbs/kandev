---
id: "01-fifo-send-now-arbitration"
title: "Arbitrate FIFO and Send Now handoff"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 01: Arbitrate FIFO and Send Now handoff

## Acceptance

- A normal FIFO drain registers a per-session handoff with an explicit
  pre-acceptance phase and an accepted/terminal phase.
- Send Now, under the existing cancellation guard, can supersede only the
  pre-acceptance FIFO handoff, restore its exact ordinary or durable source,
  and claim the requested exact row or all-scope snapshot without partial
  mutation.
- With two queued ordinary messages at the ready boundary, header Send Now
  produces one aggregate replacement prompt containing both bodies in FIFO
  order; the losing FIFO worker produces no duplicate visible message,
  workflow turn-start effect, or prompt.
- An accepted FIFO handoff returns the existing `send_now_conflict` behavior;
  it does not cancel or duplicate the successor, and remaining queue entries
  remain authoritative.
- Existing normal FIFO, explicit Cancel, peer-interrupt, lifecycle reservation,
  retry, captured-turn, and cancellation-progress behavior remains unchanged
  outside the new pre-acceptance arbitration window.

## Verification

```bash
cd apps/backend
go test -race ./internal/orchestrator -run 'Test(SendNow|QueuedDispatch|ExecuteQueuedMessage)' -count=1
go test -race ./internal/orchestrator/messagequeue -run 'Test.*SendNow' -count=1
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_test.go` or a new
  focused `queued_dispatch_arbitration_test.go`

## Dependencies

None.

## Parallelism

Sequential. The task changes the shared per-session dispatch ownership
mechanism and must be implemented and verified as one concurrency surface.

## Inputs

- Amended spec scenarios: FIFO reservation superseded before prompt
  acceptance, and accepted FIFO handoff failing closed.
- `apps/backend/internal/orchestrator/queue_send_now.go` current conflict gate.
- `dispatchTakenQueuedMessage` and `executeQueuedMessage` in
  `event_handlers_workflow.go`/`event_handlers_agent.go`.
- `claimSessionRunningForPrompt` and the existing dispatch token checks in
  `task_operations.go`.
- Existing queue lifecycle and cancellation tests that use
  `markQueuedDispatchInFlight`.

## TDD Sequence

1. Add a deterministic test barrier after the FIFO source is reserved but
   before the worker claims prompt ownership. Trigger header Send Now with two
   queued sources and assert the current implementation returns the conflict
   or otherwise fails the new expected one-prompt invariant (RED).
2. Add the accepted-handoff race case and assert the existing fail-closed
   contract, plus a baseline normal FIFO case so the arbitration does not turn
   every automatic drain into a Send Now claim.
3. Introduce the reservation state/ownership transition and move ordinary
   message recording and turn-start effects behind the worker's guarded
   ownership claim.
4. Implement Send Now supersession/restoration for only the pre-acceptance
   state, then route the winner through the existing exact claim and aggregate
   dispatch path.
5. Add durable lifecycle and stale-worker assertions, run the focused `-race`
   commands, and confirm no marker cleanup path can remove a newer owner.

## Output Contract

Report the deterministic RED failure, the reservation states and guarded
ownership boundary, restoration behavior for ordinary/durable rows, exact
focused command results, files changed, and any remaining race or blocker.
Mark this task `done` and update its plan checkbox only after the tests pass.

## Results

- RED reproduced the reported conflict before the fix: the pending FIFO
  handoff test returned `send-now operation is already in progress`.
- Added a typed per-session reservation with pending, accepted, and superseded
  phases. The FIFO worker claims the reservation under `cancelInFlight` before
  transcript or workflow side effects; Send Now can supersede only pending
  FIFO work and restores its exact source first.
- Durable lifecycle restoration clears only the transient reservation; the
  aggregate Send Now claim remains responsible for final acknowledgement.
  The stale FIFO worker is covered by a synchronous no-requeue assertion.
- Focused command results: 25 orchestrator tests passed with `-race`, and 11
  messagequeue Send Now tests passed with `-race`.
- Broader queue/cancellation command result: 179 tests passed with `-race`.
