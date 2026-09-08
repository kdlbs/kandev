---
id: "02-replacement-turn-dispatch"
title: "Orchestrate replacement-turn dispatch"
status: done
wave: 2
depends_on: ["01-queue-send-now-claims"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 02: Orchestrate replacement-turn dispatch

## Intent

Compose the exact queue claim with Kandev's unified cancellation coordinator so
Send Now safely replaces a captured busy turn or directly prompts an idle
session without inheriting explicit Cancel workflow semantics.

## Acceptance

1. `message.queue.send_now` authorizes and dispatches exactly one selected entry
   or the exact bulk snapshot, returning the specified response/error contract.
2. Busy dispatch uses backend cancellation progress but never writes the normal
   cancel message, moves to review, or evaluates cancelled-turn completion;
   promptable dispatch does not issue a cancel.
3. Successor-turn, explicit-cancel, duplicate Send Now, queue-change, and prompt
   failure races fail closed or restore the original claim without a substitute
   FIFO dispatch.

## Files likely touched

- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_send_now_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/backendapp/gateway.go`

## Dependencies

- Task 01 exact queue claim, restoration, and acknowledgement contract.

## Parallelism

Sequential. It changes shared cancellation ownership and the public WebSocket
contract used by Task 03.

## Inputs

- Spec: API Surface, State and Concurrency, Permissions, Failure Modes.
- Plan: Backend / Non-completing cancel-and-dispatch and WebSocket action.
- ADR: `2026-08-05-queue-send-now-replaces-turn`.
- Existing patterns: `QueueAndInterruptForPeerMessage`,
  `cancelAgentSilentWithGuardActionKind`, `dispatchTakenQueuedMessage`, and
  `CancellationPendingSnapshot`.

## Verification

```bash
cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/handlers -run 'SendQueuedNow|SendNow' -count=1
```

Also run the focused existing cancellation/peer-interrupt regressions affected
by any shared coordinator edit:

```bash
cd apps/backend && go test -race ./internal/orchestrator -run 'Test(CancelAgent|CancellationSources|QueueAndInterruptForPeerMessage)' -count=1
```

## Output contract

Report files changed, cancellation-source behavior, wire contract/error mapping,
RED and GREEN test commands/results, race cases covered, blockers/risks, and
synchronize this task plus `plan.md` status/results.

## Results

- Added `Service.SendQueuedNow`, the `message.queue.send_now` action, stable validation/error mapping, queue status publication, exact selected/bulk dispatch, and the dedicated non-completing cancellation kind.
- Busy Send Now uses the backend-owned cancellation projection and captured-turn guard without joining explicit Cancel; promptable Send Now claims and prompts directly. Duplicate/cross-owner operations fail closed, and prompt/reset failures restore the complete claim.
- GREEN: `cd apps/backend && go test -race ./internal/orchestrator -run 'SendNow|ExplicitCancellationDoesNotJoin' -count=1 -v` — 6 tests passed; handler coverage with `-run 'SendNow|QueueHandler'` — 24 tests passed; affected existing cancellation/peer-interrupt regressions — 56 tests passed.
- PR fixup: carry the validated click-time source snapshot through cancellation and compare persisted queue fields before claiming; mark every source as `user_message_recorded` before prompt admission so ordinary and durable restoration cannot create duplicate transcript rows. Regression coverage includes changed-snapshot atomicity and a retry with a durable source.
- The WS contract validates scope before dispatch and returns `{session_id, dispatched, sent_count}` on success, with stable queue-empty, queue-changed, conflict, turn-changed, attachment, and reference errors.
