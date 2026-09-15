---
id: "01-live-fifo"
title: "Make live FIFO turns interruptible"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.2
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.7
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.8
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.9
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.10
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 01: Make live FIFO turns interruptible

## Summary

Promote ordinary FIFO reservations at the successful guarded prompt-claim
boundary already used by Send Now. Keep settlement ownership and all captured
turn, session incarnation, and cancellation protections.

## In scope

- In `queue_send_now_test.go`, add `TestSendQueuedNowCancelsLiveFIFOTurn` before
  production changes. Use real queue drain and service dispatch with a blocking
  mock executor; synchronize on prompt entry. Assert Send Now succeeds, cancels
  A, sends selected B once, and leaves C queued while B is held open.
- Cover entry/all scope, fixed profile, and a session whose logical dynamic
  profile differs from its persisted concrete Cursor profile. Seed existing
  execution attribution using repository test helpers. Assert cancellation uses
  that execution and introduces no queue-specific profile lookup or selection.
- Preserve a real accepted-handoff conflict test by placing a barrier before
  guarded claim completion. Keep pending-FIFO supersession coverage. Do not
  reinterpret blocked long-running `PromptAgent` I/O as an incomplete handoff.
- Add late-predecessor completion and claim-to-provider-I/O cancellation cases.
  Use channels and cleanup releases, not sleeps. Prove a superseded worker
  cannot dispatch after replacement, restore accepted A, clear B's reservation,
  or drain C while B is active.
- Add a deterministic predispatch race regression that pauses ordinary FIFO A
  after guarded claim completion and before provider admission. Keep the
  cancellation guard through the bounded acceptance callback, then prove Send
  Now B replaces A exactly once, C remains pending, and late A completion
  cannot mutate B's turn, ownership, or prompt-attempt record.
- Remove the origin-specific `liveEligible` gate, field, and setter if redundant;
  promote only the exact current reservation after successful claim effects.
  Keep accepted-record cleanup, turn binding, and identity fencing intact.
- Retain silent cancellation and workflow behavior. Update source comments
  that incorrectly describe accepted ownership as whole-turn exclusion.

## Out of scope

Provider adapters, routing policy, persistence changes, UI changes, and any
removal of cancellation or generation checks.

## Acceptance

1. The new live-FIFO test fails with `ErrSendNowConflict` before the fix and
   passes after it for both scopes and profile cases, with exact delivery/order.
2. Incomplete handoff and real overlapping cancellation still fail closed;
   late events and predispatch cancellation cannot affect a newer owner.
3. Existing Send Now workflow, restoration, successor, and pending-FIFO tests
   pass. Accepted A is never returned to pending storage by interruption.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestSendQueuedNowCancelsLiveFIFOTurn$' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'Test.*(SendNow|SendQueuedNow|QueuedDispatch|FIFOHandoff)' -count=1)
git diff --check
```

Record the first command's RED result before editing production code, then its
GREEN result. If new shared helpers require additional tests, add their exact
suite to this work order before marking it done.

## Files likely touched

- `apps/backend/internal/orchestrator/queued_dispatch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_test.go`
- `apps/backend/internal/orchestrator/queue_send_now_workflow_state_test.go`

## Dependencies

None.

## Risks

The most important boundary is guarded claim completion versus the return of a
long-running provider call. Tests must distinguish both without deleting the
accepted ownership record. Review the exact identity and cancellation guard
held at each test barrier to avoid a test-only deadlock.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/message-queue-send-now.md), .2/.7-.10.
- [Design](../../specs/ui/system-design/message-queue-send-now.md), transition
  to live and interruption/recovery sections.
- Existing `TestSendQueuedNowConflictsAfterFIFOHandoffAccepted`,
  `TestSendQueuedNowCancelsLiveReplacementTurn`, and
  `TestSendQueuedNowConflictsBeforeReplacementClaimsPrompt` patterns.

## Results

RED recorded before production changes:

```text
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestSendQueuedNowCancelsLiveFIFOTurn$' -count=1)
FAIL: both table cases returned send-now operation is already in progress.

GREEN and focused race verification after production changes:

```text
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestSendQueuedNowCancelsLiveFIFOTurn$' -count=1)
PASS

(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'Test.*(SendNow|SendQueuedNow|QueuedDispatch|FIFOHandoff)' -count=1)
PASS

TestStreamCompletePreservesLiveFIFOSuccessorForStalePromptGeneration
PASS

TestSendQueuedNowSerializesFIFOPredispatchAdmission
PASS
The test asserts exactly one B with C preserved and that late A completion
cannot change B's active turn, ownership, or prompt-attempt record.

git diff --check
PASS
```

The table-driven GREEN cases cover entry/all scope, fixed profile attribution,
and a logical dynamic profile with persisted concrete Cursor execution. The
accepted handoff remains a conflict until guarded claim completion, while the
live FIFO reservation can be cancelled during the provider call. A stale
predecessor completion leaves the live successor reservation intact. The
predispatch regression covers the separate claim-to-provider-admission fence:
ordinary FIFO A keeps the per-session cancellation guard until provider
acceptance, so a concurrent Send Now cannot supersede A before its dispatch
side effects are durable. It also checks that the provider receives exactly one
B and that the replacement prompt-attempt record remains after late A return.
```
