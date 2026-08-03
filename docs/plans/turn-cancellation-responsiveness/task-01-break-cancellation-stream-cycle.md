---
id: "01-break-cancellation-stream-cycle"
title: "Break the cancellation stream cycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow/cancelled-turn-completion/spec.md"
---

# Task 01: Break the cancellation stream cycle

## Acceptance

- An acknowledged lifecycle cancellation can wait for terminal `handleAgentStreamEvent` callbacks without holding the callback's `cancelInFlightGuard`; the operation returns before the regression test's one-second deadline without using escalation.
- The owner reacquires the same guard and revalidates the captured turn before reconciliation. Terminal frames are persisted in order, the session becomes `WAITING_FOR_INPUT`, and cancellation message, turn closure, review/workflow behavior, and lifecycle cancel each occur exactly once.
- Concurrent explicit requests join one operation and share its result; a later request after settlement can start a fresh operation. Silent clarification and peer-interrupt paths also avoid the lock cycle while retaining their existing semantics.
- Ordinary stream writes still wait on the guard outside the lifecycle wait, and a successor turn is never closed or mutated by the cancelled turn's reconciliation or stale frames.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1
cd apps/backend && go test ./internal/orchestrator -count=1 -timeout=120s
cd apps/backend && go test -race ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1
cd apps/backend && go test ./cmd/mock-agent -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestCancelAgent' -count=1
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

- Implemented explicit user-cancel operation ownership and shared completion notification. Concurrent requests join one lifecycle cancel and receive its stored result; a later request can start a new operation.
- Split the per-session guard interval around the blocking lifecycle cancel. ACP terminal stream callbacks can acquire the guard, persist ordered frames, and release it before cancellation reconciliation reacquires it.
- Captured and revalidated the cancelled turn identity; successor turns fail closed and remain active. Ready, boot-ready, queue-drain, peer-interrupt, and clarification paths retain their serialization and cancellation semantics.
- RED/GREEN regression: the channel-synchronized stream callback failed against the old held-lock implementation and passes with the two-phase unlock, without reaching lifecycle escalation.
- `cd apps/backend && go test ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1` — 45 tests passed.
- `cd apps/backend && go test ./internal/orchestrator -count=1 -timeout=120s` — passed in 59.160s.
- `cd apps/backend && go test -race ./internal/orchestrator -run 'Test(CancelAgent|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1` — passed.
- `cd apps/backend && go test ./cmd/mock-agent -count=1` — package passed.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestCancelAgent' -count=1` — no matching tests; command exited successfully.
- `git diff --check` — passed. No schema, public API, ACP message, lifecycle timeout, external side effect, or new trust boundary was introduced.
