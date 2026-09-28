---
id: "01-separate-prompt-admission"
title: "Separate stream preparation from prompt admission"
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

# Task 01: Separate stream preparation from prompt admission

## Summary

Remove the queued-dispatch and stream-publication lock cycle.
Keep logical dispatch ownership while preparation runs outside the session mutex.
Revalidate ownership at final admission before a new generation can reach the provider.

## In scope

- Write the deterministic deadlock regression before changing production code.
- Extend the internal callback path through executor, backend adapter, lifecycle manager, and session manager.
- Split stream drain/history preparation from admitted generation allocation and local buffer reset.
- Cover queued, lifecycle, resumed, and model-switch dispatch paths that share the guard.
- Preserve ordered publication, completion barriers, queue disposition, and accepted execution ownership.

## Out of scope

Cancellation deadline handling belongs to Task 02. Browser scenarios belong to Task 03.
Do not change event-bus delivery semantics, provider protocols, public APIs, queue policy, or UI components.

## Acceptance

1. `TestPromptTask_QueuedStreamBoundaryDoesNotDeadlock` fails on the original path and passes after the repair.
   The fixture uses real lifecycle coalescing and synchronous stream delivery; it proves dispatch and subsequent cancellation complete.
2. `TestPromptTask_PreparedDispatchRevalidatesOwnership` rejects cancellation, archive, replacement, and workflow-route supersession during preparation.
   Cover queued, resumed, lifecycle, and model-switch paths with applicable identities. No stale attempt sends or closes successor work.
3. `TestSessionManager_StreamBoundaryPrecedesAdmission` proves old output drains before admission and generation allocation.
   Assert the exact buffered tail, its message and generation identity, one dispatch, and the accepted execution's continuity.
   Existing prompt completion-barrier tests retain terminal ordering coverage; Task 03 verifies pause and follow-up on the same execution.

## Verification

Run from the repository root. The dedicated temporary directory avoids the known full host `/tmp`.
Remove only this owned directory after the commands finish.

```bash
task_tmp="$(mktemp -d /root/kp.XXXXXX)"
export TMPDIR="$task_tmp" GOTMPDIR="$task_tmp"
(cd apps/backend && go test ./internal/orchestrator -run '^TestPromptTask_(QueuedStreamBoundaryDoesNotDeadlock|PreparedDispatchRevalidatesOwnership)$' -count=1 -timeout=90s)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(SessionManager_StreamBoundaryPrecedesAdmission|StreamCoalescer)' -count=1 -timeout=120s)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1 -timeout=300s)
git diff --check
```

Record the first expected RED failure and every final command result.
Use channel barriers or `testing/synctest`; do not depend on a sleep to win the race.
Give the negative fixture a bounded escape path so a failed assertion cannot leak a locked goroutine.
Do not claim the real cycle is covered by a mock that returns before `flushBoundary`.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/task_operations_stream_boundary_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations_resumed_turn_cancellation_test.go`
- `apps/backend/internal/orchestrator/task_operations_model_switch_cancellation_test.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go` and its existing tests
- `apps/backend/internal/backendapp/adapters.go` and focused adapter tests
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and `session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_streaming.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer_test.go`

The new test filename is proposed. Existing interface declarations and test doubles may need matching callback changes.
Inspect `internal/agent/runtime/lifecycle` and `internal/orchestrator/executor` before selecting the final internal callback type.

## Dependencies

None. Read the paired design before implementation and compare the current base with the package's recorded revision.

## Risks

Releasing the physical guard does not revoke logical ownership.
Removing ownership checks would let cancellation race with late prompt dispatch.
Failure callbacks and model-switch fallback can create a second lock cycle if they retain the guard during publication.
Do not allocate a new generation merely to make the stream callback appear stale.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and compatibility matrix](plan.md)
- [Task requirements](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md)
- [Preparation and final admission](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md#preparation-and-final-admission)
- Existing `stream_coalescer_test.go`, resumed-turn cancellation tests, and cancellation/stream responsiveness fixtures.
- [Resumed-turn package](../resumed-turn-cancellation/plan.md), whose accepted-process guarantee must remain intact.

## Results

Implemented the preparation/admission split across lifecycle, executor, adapter, and orchestrator.
The deterministic stream-boundary and ownership-revalidation tests pass. The model-switch guard
regression was observed red with the guard held across restart and green after moving revalidation
to initial-prompt admission. The queued model-switch reservation now remains protected through
the async worker-return gap and becomes live only at provider acceptance.

Verification passed:

- Focused queued admission, stream-boundary, model-switch, and resume cancellation tests.
- The four-package race suite: orchestrator 337.177s, executor 3.506s, lifecycle 68.381s,
  backendapp 81.821s.
- The current lifecycle coalescer/event-bus admission test under the race detector.
- Backend build and final orchestrator/handler race suite are recorded in the plan results.

Review remediation (2026-09-28):

- `lifecycleAdapter` now forwards initial-prompt admission and dispatch callback registration, with a compile-time assertion for both methods. `TestLifecycleAdapter_RegistersInitialPromptAdmissionOnRestart` exercises the production adapter against a real lifecycle manager.
- `promptTask` captures the cancellation projection revision before preparation. Restart admission and queued in-place continuation reject a changed revision under the session guard, even after the pause operation leaves the active registry.
- `TestPromptTask_ModelSwitchFallbackRejectsAdmissionAfterCompletedPause` and `TestPromptTask_QueuedInPlaceModelSwitchRejectsAfterCompletedPause` verify cancellation-fenced rejection, zero provider calls, no generation advance, and no successor turn after explicit pause completes.
- The targeted regressions passed, and `go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1 -timeout=600s` passed across all four packages.

PR fixup findings (2026-09-28):

- Separate admission rejection from provider delivery failure in the lifecycle path.
  A rejected replacement prompt now runs cleanup without failing the accepted execution.
- Persist user history only after the provider accepts the prompt. Rejection leaves prior
  assistant history intact and does not create a new prompt generation.
- `TestDispatchInitialPromptAdmissionRejectionDoesNotFailExecutionOrPersistPrompt` exercises
  the real lifecycle initial-prompt callback wiring and covers execution status, generation,
  and history after rejection.
- The final four-package race suite passed after these fixes: orchestrator 411.827s, executor
  3.667s, lifecycle 87.349s, backendapp 92.980s.
