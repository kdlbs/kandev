---
id: "01-release-settlement-guards"
title: "Release settlement acknowledgement guards"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
---

# Task 01: Release settlement acknowledgement guards

## Summary

Remove the cancellation acknowledgement edge from the confirmed three-lock cycle.
Keep logical cancellation ownership while lifecycle acknowledgement runs outside the session guard.
Apply the same rule to retained turn failure settlement.

## In scope

- Add the service regressions before production changes.
- Release the explicit cancellation guard before `acknowledgeRetainedPromptFailure`.
- Release the retained-turn failure guard before `acknowledgeRetainedTurnFailure`.
- Reacquire with the original context and revalidate identity before any remaining guarded state change.
- Add a real lifecycle/coalescer/synchronous-bus regression with deterministic ownership barriers.
- Preserve generation fences, stream order, joined callers, projection release, and sibling precedence.

## Out of scope

Deletion deadline primitives belong to Task 02.
Do not change acknowledgement signatures, event delivery semantics, provider protocols, workflow policy, or queue replay behavior.

## Acceptance

1. Both service regressions fail because acknowledgement holds the session guard, then pass after the correction.
   Duplicate callers share one cancellation. Pending progress clears once, and stale acknowledgement cannot release a successor.
2. `TestLateCompletionCoalescerAndSettlementAcknowledgementProgress` uses the real completion handler, coalescer, and synchronous event bus.
   It establishes all three ownership edges, drains ordered frames, and finishes without a leaked goroutine.
   A mock service callback alone is insufficient evidence for this cycle.
3. Existing cancellation, retained-failure, dispatch completion, and coalescer compatibility tests pass under the race detector.
   One working sibling and an unrelated task retain their existing outcomes.

## Verification

Run from the repository root. Record RED before implementation and GREEN after it.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Test(CancelAgent_SettlementAcknowledgementReleasesGuard|RetainedTurnFailure_AcknowledgementReleasesGuard)$' -count=1 -timeout=90s)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestLateCompletionCoalescerAndSettlementAcknowledgementProgress$' -count=1 -timeout=90s)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'Test(CancelAgent|RetainedTurnFailure|.*Cancellation|.*Cancel.*Review)' -count=1 -timeout=10m)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test(LateCompletionCoalescer|TransientTurnFailure|StreamCoalescer|.*DispatchCompletion)' -count=1 -timeout=10m)
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go`
- `apps/backend/internal/orchestrator/cancellation_completion_lock_order_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/completion_cancel_coalescer_test.go`

Use existing fixture and completion helpers without adding a production-only test API.

## Dependencies

None.

## Risks

The unlocked interval permits concurrent events. Captured execution, generation, and operation checks must reject stale work.
The pre-fix reproduction needs bounded cleanup or a subprocess so an expected deadlock cannot strand the test runner.

## Parallelism

`sequential`

## Inputs

- [Plan and observed cycle](plan.md#evidence-and-root-cause).
- [Acknowledgement design](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md#settlement-acknowledgement-and-late-completion).
- [Existing requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md).
- `task_operations_cancellation_deadline_test.go`, `event_handlers_turn_failure_test.go`, `manager_turn_failure_test.go`, and `stream_coalescer_test.go`.

## Results

Both service regressions failed on the original guard-availability assertion.
They passed after durable settlement completed before guard release and lifecycle acknowledgement.
The lifecycle/coalescer/synchronous-bus fixture passed as compatibility evidence for the unlocked protocol.
The service regressions provide the defect-specific RED evidence.

Passed commands:

- The two focused orchestrator regressions: 0.182 s.
- The real lifecycle completion/coalescer fixture: 0.071 s.
- The specified race-enabled orchestrator cancellation selection: 11.298 s.
- The specified race-enabled lifecycle completion/coalescer selection: 2.411 s.

No acknowledgement signature, provider protocol, event-delivery order, or workflow policy changed.
