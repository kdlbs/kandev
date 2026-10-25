---
spec: docs/specs/ui/requirements/message-queue-send-now.md
created: 2026-08-05
status: implemented
---

# Implementation Plan: Arbitrate Send Now Against FIFO Handoff

## Overview

Repair the narrow race between ordinary FIFO queue draining and the explicit
Send Now operation. The diagnostic bundle shows that FIFO removes/reserves the
head and starts an asynchronous worker after the active turn becomes ready;
Send Now then sees only the in-flight marker and returns `send_now_conflict`,
so two queued messages run as separate turns instead of one replacement turn.

Add an explicit per-session handoff state with a pre-acceptance ownership
boundary. Send Now may supersede and restore an unaccepted FIFO reservation,
then claim its exact row or all-scope snapshot. Once FIFO has accepted prompt
ownership, the existing fail-closed conflict remains the safe behavior. The
fix is backend-only apart from a browser regression assertion; no queue schema,
WebSocket shape, or frontend control change is planned.

## Confirmed Root Cause

- The bundle records two queue entries, then the active turn becomes ready.
- The normal drain reserves the FIFO head and launches
  `executeQueuedMessage` asynchronously.
- `SendQueuedNow` checks `isQueuedDispatchInFlight` in
  `queue_send_now.go` and returns `ErrSendNowConflict` before it can aggregate
  the queue.
- The worker can also record the ordinary user message and run turn-start
  effects before its final prompt claim, so allowing an uncoordinated
  supersession would risk duplicate transcript/workflow side effects.
- The result in the bundle is two independent queued prompts followed by the
  Send Now error, matching this race rather than a cancellation-acknowledgement
  failure.

## Backend

### Per-session queued-dispatch handoff

Files:

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`

Replace the bare session-to-entry in-flight marker with (or wrap it in) a
per-session reservation that records the exact queued source and whether the
automatic worker is still pre-acceptance or has claimed prompt ownership.
Keep the existing per-session cancellation guard as the single arbitration
point.

- Register the FIFO reservation before spawning the worker, as today.
- Make the worker claim ownership under the guard before recording an ordinary
  user message, running `on_turn_start` effects, or accepting the agent prompt.
- Let Send Now, while holding the same guard, supersede only a pre-acceptance
  FIFO reservation. Restore that exact ordinary or durable source, publish the
  authoritative queue state at the normal transition point, and continue
  through the existing exact-entry/bulk claim and replacement dispatch path.
- Preserve the existing `send_now_conflict` behavior for an accepted FIFO
  handoff, explicit cancellation, or another Send Now operation. Do not
  cancel or duplicate a successor that has already claimed the turn.
- Update all marker helpers and callers (`drainQueuedMessageForPromptableSession`,
  targeted queue takes, lifecycle dispatch checks, and `promptTask`) so a stale
  worker cannot clear a newer owner or perform side effects after losing.

### Restoration and lifecycle safety

Use the existing message-queue restoration and durable acknowledgement contract;
do not add a database migration. A superseded ordinary row must return to its
original FIFO position, and a durable lifecycle row must have its transient
reservation cleared. The winning Send Now claim remains the only source that
can create the replacement prompt and acknowledge durable sources.

Do not change explicit Cancel completion semantics, captured-turn checks,
backend-owned cancellation progress, normal FIFO ordering, or `Run next`
behavior outside this pre-acceptance arbitration window.

## Frontend and Protocol

No production frontend or WebSocket contract change is required. The existing
error mapping/refetch remains the correct response if FIFO has already accepted
its successor. Extend the desktop queue E2E assertion so the aggregate appears
as one user prompt, not merely as three strings that could have arrived in
separate turns. Re-run the existing mobile Send Now scenario because the
control and shared API path are unchanged.

## Tests

- **What:** a normal FIFO reservation can be superseded before prompt
  acceptance, restored, and included in one exact bulk Send Now claim.
  **File:** `apps/backend/internal/orchestrator/queue_send_now_test.go` or a
  focused `queued_dispatch_arbitration_test.go` beside it.
  **How:** channel-synchronized handoff barriers, no sleeps; assert one
  aggregate prompt, both source IDs restored/claimed correctly, FIFO order,
  and no stale worker transcript or workflow side effects.
- **What:** an accepted FIFO handoff remains terminal for Send Now.
  **File:** the same focused orchestrator test file.
  **How:** pause after ownership acceptance, issue Send Now, assert
  `ErrSendNowConflict`, no duplicate prompt/message, no cancellation of the
  successor, and untouched remaining queue.
- **What:** ordinary FIFO delivery and existing explicit-cancel/peer-interrupt
  paths retain their current semantics after the handoff state change.
  **Files:** focused existing queue/cancellation tests under
  `apps/backend/internal/orchestrator/`.
  **How:** run the targeted regression groups with `-race`; include durable
  lifecycle and retry paths because they share the reservation marker.
- **What:** the browser-visible bulk action produces one replacement user
  prompt containing both queued bodies in order.
  **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`.
  **How:** extend the existing header Send Now scenario to assert one
  `user-message-bubble` contains the aggregate, then verify the queue is empty
  and the turn completes. Re-run the existing mobile Send Now scenario for
  shared-path parity.

## E2E Tests

- **Scenario:** while a desktop agent is busy, queue multiple messages and
  activate header Send Now. Verify the replacement transcript contains one
  user prompt with the concatenated bodies in FIFO order, rather than one
  visible prompt per source entry, and that the queue disappears.
  **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`.
- **Scenario:** on Pixel 5, run the existing row Send Now flow and verify the
  shared arbitration result still leaves neighboring queue entries ordered,
  preserves the 44px controls, and keeps document horizontal overflow at zero.
  **File:** `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`.

## Verification Results

- RED before the arbitration implementation: `cd apps/backend && go test
  -race ./internal/orchestrator -run
  '^TestSendQueuedNowSupersedesPendingFIFOHandoff$' -count=1` failed with
  `send-now operation is already in progress`, reproducing the diagnostic-bundle
  race.
- Focused backend: `cd apps/backend && go test -race ./internal/orchestrator
  -run 'Test(SendNow|QueuedDispatch|ExecuteQueuedMessage)' -count=1` — 25
  passed.
- Queue repository: `cd apps/backend && go test -race
  ./internal/orchestrator/messagequeue -run 'Test.*SendNow' -count=1` — 11
  passed.
- Broader queue/cancellation regression: `cd apps/backend && go test -race
  ./internal/orchestrator -run
  'Test(.*Queued.*|.*Queue.*|.*Cancel.*|.*Interrupt.*|.*Peer.*)' -count=1` —
  179 passed.
- Full orchestrator package: `cd apps/backend && go test -race
  ./internal/orchestrator -count=1` — 1546 passed.
- Changed-file Go lint: `cd apps/backend && golangci-lint run ./... --new-from-rev
  5c51dde562e2264ae63544993be88fd7f383222d --timeout=5m` — 0 issues.
- Web lint: `cd apps && pnpm --filter @kandev/web lint` — passed.
- Web typecheck: `cd apps/web && pnpm run typecheck` — no errors.
- E2E formatting: `cd apps && pnpm exec prettier --check
  web/e2e/tests/chat/message-queue.spec.ts` — passed.
- Desktop aggregate E2E: `cd apps/web && pnpm e2e:run
  tests/chat/message-queue.spec.ts -- --grep 'header Send Now' --retries=0` — 1
  passed.
- Mobile parity E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome
  tests/chat/mobile-message-queue-management.spec.ts -- --grep 'Send Now'
  --retries=0` — 1 passed.
- Orchestrator compile check: `cd apps/backend && go test
  ./internal/orchestrator/... -run '^$' -count=1` — no tests found; compile
  completed successfully.
- `git diff --check` is run before commit.

## Implementation Waves and Parallel Candidates

The default is sequential execution in the primary conversation. These waves do
not authorize subagents.

Wave 1:

- [x] [Task 01: Arbitrate FIFO and Send Now handoff](task-01-fifo-send-now-arbitration.md)

Wave 2:

- [x] [Task 02: Prove one aggregate replacement prompt](task-02-aggregate-replacement-e2e.md)

Task 02 depends on Task 01 and is not parallel-safe with it because it verifies
the changed backend timing and prompt ownership behavior.

## Risks and Boundaries

- The ownership boundary must precede visible message creation and workflow
  turn-start effects; a late check can still create a duplicate transcript row.
- Restoring a reserved lifecycle entry must clear only the transient delivery
  reservation and must not acknowledge or discard the durable source.
- Once an automatic prompt is accepted, combining it with a later Send Now
  request is intentionally out of scope because it would require retracting or
  rewriting an already visible user message. The operation fails closed there.
- No schema migration, new error code, queue reordering, arbitrary subset
  selection, or change to normal FIFO/Run next semantics is planned.

## Open Questions

None. The diagnostic evidence and existing `message-queue-send-now` contract
define the repair boundary.
