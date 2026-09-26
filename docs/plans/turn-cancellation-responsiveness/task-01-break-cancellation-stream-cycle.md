---
id: "01-break-cancellation-stream-cycle"
title: "Break the cancellation stream cycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-cancelled-turn-completion.md"
---

# Task 01: Break the cancellation stream cycle

## Acceptance

- An acknowledged lifecycle cancellation can wait for terminal `handleAgentStreamEvent` callbacks without holding the callback's `cancelInFlightGuard`; the operation returns before the regression test's one-second deadline without using escalation.
- The owner reacquires the same guard and revalidates the captured turn before reconciliation. Terminal frames are persisted in order, the session becomes `WAITING_FOR_INPUT`, and cancellation message, turn closure, review/workflow behavior, and lifecycle cancel each occur exactly once.
- Explicit, silent clarification, and peer-interrupt requests share one per-session operation and lifecycle call; a disconnected owner cannot abort a live joiner, and joiners re-evaluate their source-specific action after settlement.
- Ordinary stream writes still wait on the guard outside the lifecycle wait, and a successor turn is never closed or mutated by the cancelled turn's reconciliation or stale frames.

## Verification

```bash
(
  cd apps/backend
  go test ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1
  go test ./internal/orchestrator -count=1 -timeout=120s
  go test -race ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1
  go test ./cmd/mock-agent -count=1
  go test ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent_' -count=1
)
```

Follow TDD: first add the channel-synchronized stream callback regression and confirm it fails because the callback cannot acquire the guard; then implement operation ownership and the two-phase lock interval.

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the shared per-session cancellation registry and guard semantics; Task 02 validates its completed behavior.

## Inputs

- Spec: prompt cancellation responsiveness, ordered terminal-frame persistence, duplicate-operation, and stale-successor scenarios.
- Plan: `Confirmed Root Cause`, `Explicit cancellation operation ownership`, and `Two-phase per-session serialization`.
- Reproduction evidence: `acp-debug/codex-acp-cancel-20260803-171437.jsonl` and the isolated 10.37-second browser/backend trace summarized in the task plan.
- Existing patterns: `beginCancelInFlight`, `isCancelInFlight`, `acquireCancelInFlightGuard`, `peekActiveTurnID`, `completeTurnIfCurrent`, and the channel-driven cancellation race tests.

## Risks

- Do not replace the shared guard with an unguarded stream fast path.
- Do not hold the guard while waiting on code that requires ordered stream callbacks.
- If backend-owned cancellation progress from `feature/when-i-have-a-task-w-ge1` is present, extend its operation record instead of adding a competing registry or changing its public projection.

## Output contract

Report the operation ownership/result semantics, Red/Green evidence, exact files and commands, lifecycle timeout behavior, duplicate and successor-turn evidence, blockers/risks, and synchronize this task plus `plan.md` in the same primary conversation.

## Results

- Replaced split explicit/silent cancellation markers with one per-session coordinator shared by explicit, silent clarification, and peer interrupts. The owner alone invokes lifecycle cancellation; joiners wait and then re-check their source-specific action.
- Accepted cancellation work now uses a bounded service-owned context (`context.WithoutCancel`), so an owner request disconnect cannot abort a live joiner.
- Added cancellation admission gates for prompt claims, running/boot-ready, completion/failure, step-completion, clarification cleanup, teardown, fallback reset, and stale stream identities.
- Split the per-session guard interval around the blocking lifecycle cancel. ACP terminal stream callbacks can acquire the guard, persist ordered frames, and release it before cancellation reconciliation reacquires it.
- Captured and revalidated the cancelled turn identity; successor turns fail closed and remain active. Ready, boot-ready, queue-drain, peer-interrupt, and clarification paths retain their serialization and cancellation semantics.
- RED/GREEN regression: the channel-synchronized stream callback failed against the old held-lock implementation and passes with the two-phase unlock, without reaching lifecycle escalation.
- `cd apps/backend && go test ./internal/orchestrator -run 'Test(CancelAgent|CancellationSources|CancellationStreamAdmission|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1` — focused ownership/admission suite passed.
- `cd apps/backend && go test ./internal/orchestrator -count=1 -timeout=180s` — passed after the final admission-gate and joined-source changes.
- `cd apps/backend && go test -race ./internal/orchestrator -run 'Test(CancellationSources|CancelAgent_JoinedSilentCancellationRunsExplicitReconciliation|CancelAgent_OwnerDisconnectDoesNotAbortJoinedOperation|CancelAgent_DefersLifecycleCompletionDuringCancellation|CancelAgentSilent_DoesNotCloseSuccessorTurn|QueueAndInterruptForPeerMessage_(ClosesStaleEarlyCheckRace|CancelFailureDoesNotStrandMessageWhenReadyIsRacing))' -count=1` — passed.
- `cd apps/backend && go test ./... -count=1 -timeout=300s` — all backend packages passed.
- `cd apps/backend && go test ./cmd/mock-agent -count=1` — package passed.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent_' -count=1` — lifecycle cancellation tests passed.
- `git diff --check` — passed. No schema, public API, ACP message, lifecycle timeout, external side effect, or new trust boundary was introduced.
