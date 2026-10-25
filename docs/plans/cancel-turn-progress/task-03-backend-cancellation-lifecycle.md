---
id: "03-backend-cancellation-lifecycle"
title: "Backend cancellation lifecycle"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/cancel-turn-progress.md"
---

# Task 03: Backend cancellation lifecycle

## Acceptance

- `orchestrator.Service.CancellationPending(sessionID)` is true from the first accepted cancellation
  reference until the final reference settles, isolated by session and false outside that window.
- `CancellationPendingSnapshot(sessionID)` returns the boolean and process-local revision from one
  critical section; revisions advance only on first-begin and last-end transitions.
- The transition publication queue is appended while the count/revision are mutated, so a successor
  generation cannot publish `true` before its predecessor's `false`.
- An accepted cancellation continues after the initiating request context is cancelled, while the
  existing lifecycle timeout/escalation and per-session deduplication remain intact.
- Overlapping requests invoke the agent manager once and publish exactly one pending transition and
  one idle transition; success and every error return clear the registry.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'TestCancellationPendingTracksReferencesAndPublishesTransitions|TestCancelAgent_(SurvivesCallerCancellation|DeduplicatesConcurrentCalls|ClearsCancellationPendingOnError)')
make -C apps/backend test
```

Follow TDD: first block the mock agent manager and assert the missing public state/transitions, then
add transition-aware registry publication and detach the accepted work from caller cancellation.

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/events/types.go`

## Dependencies

None.

## Parallelism

Sequential. Task 04 consumes the provider and event established here.

## Inputs

- Spec: `Data model`, `State machine`, `Failure modes`, and `Persistence guarantees`.
- Plan: `Cancellation operation ownership`.
- Decision: `ADR-2026-08-03-backend-owned-cancellation-progress`.
- Existing patterns: `cancelOperations`, `beginCancelInFlight`, `isCancelInFlight`, the
  `cancelInFlight` guard, and `PromptTask`'s use of `context.WithoutCancel` after admission.

## Output contract

Report the state-transition API, context ownership, exact Red/Green tests, duplicate and error-path
evidence, files changed, blockers/risks, and synchronize this task plus `plan.md` in the same primary
conversation.

## Results

Implemented the runtime projection and accepted-operation context boundary.

- Red: `cd apps/backend && go test ./internal/orchestrator -run 'TestCancellationPendingTracksReferencesAndPublishesTransitions|TestCancelAgent_ClearsCancellationPendingOnError|TestCancelAgent_SurvivesCallerCancellation' -count=1` initially failed because the public provider and transition implementation were absent.
- Green: `cd apps/backend && go test ./internal/orchestrator -run 'TestCancellationPendingTracksReferencesAndPublishesTransitions|TestCancelAgent_ClearsCancellationPendingOnError|TestCancelAgent_SurvivesCallerCancellation' -count=1` — 3 passed.
- Regression: `cd apps/backend && go test ./internal/orchestrator -run 'TestCancelAgent|TestHandleAgentBootReady_DoesNotDrainWhileCancelInFlight' -count=1` — 15 passed.
- Race: `cd apps/backend && go test -race ./internal/orchestrator -run 'TestCancellationPending|TestCancelAgent_SurvivesCallerCancellation' -count=1` — 5 passed; full orchestrator race suite — 1,453 passed.
- `git diff --check` — passed.

The registry remains process-local and reference-counted; revisions are retained for the lifetime of
the backend process so delayed snapshots/events cannot reuse an older identity. No schema, repository,
metadata, external side effect, or new trust boundary was added. Event publication failures are logged
and cannot change the cancellation result.
