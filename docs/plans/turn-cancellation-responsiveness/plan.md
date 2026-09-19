---
spec: docs/specs/tasks/requirements/workflow-cancelled-turn-completion.md
created: 2026-08-03
status: complete
---

# Implementation Plan: Turn Cancellation Responsiveness

## Overview

Remove the cancellation/stream lock cycle without weakening the per-session serialization that protects coordinator stops and turn replacement. Use one backend-owned cancellation coordinator for explicit, silent, and peer cancellation sources, then tighten the existing desktop and mobile Playwright scenarios around the observable response-time contract. No ACP, WebSocket, persistence, or frontend component contract changes are required.

## Confirmed Root Cause

Commit `a100cb9169` made `handleAgentStreamEvent` acquire the same per-session `cancelInFlightGuard` that `Service.CancelAgent` holds while synchronously waiting in `agentManager.CancelAgent`. Codex accepts `session/cancel` and emits terminal frames in about 6 ms, but the ordered stream worker blocks on that guard before it can finish the prompt. Cancellation therefore waits for the lifecycle manager's 10-second `cancelWaitTimeout`; the fallback releases the cycle, producing the reproduced 10.37-second UI delay.

The smallest reliable regression is an orchestrator test whose mock lifecycle cancel does not return until a terminal stream callback completes. Current code times out because `CancelAgent` holds the callback's guard; the repaired code must let the callback finish, then reconcile the captured turn exactly once.

---

## Backend

### Unified cancellation operation ownership

Files:

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`

Changes:

- Replace the split explicit-user and silent cancellation markers with one per-session cancellation coordinator. Record the cancellation source, completion notification, result, and captured `(executionID, promptGeneration, turnID)` identity in one operation.
- Let the first authorized source own the operation. Concurrent explicit, silent, and peer requests join that operation; only the owner invokes the lifecycle manager, while joiners wait and then re-evaluate their source-specific queue or clarification action.
- Keep operation ownership active across the whole lifecycle wait and every reconciliation exit path. Remove the entry only after the owner publishes the final result, so the temporarily unlocked guard cannot admit a second user-cancel owner.
- Run the accepted operation with a bounded service-owned context derived from `context.WithoutCancel`; a disconnected caller stops waiting without aborting the shared lifecycle/reconciliation work.
- Preserve session authorization, tolerated `ErrNoExecutionForSession` and `ErrCancelEscalated` reconciliation, the visible cancel message, parked queued messages, workflow completion policy, and retry behavior after a settled operation.

### Two-phase per-session serialization

Files:

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`

Changes:

- Under `cancelInFlightGuard`, load the authoritative session and capture the active turn identity needed by cancellation bookkeeping. Retain the guard registry reference, but unlock its mutex before calling the blocking lifecycle cancel.
- While the mutex is released, keep cancellation intent/ownership active as an admission gate. Non-stream claimants (prompt claims, running/boot-ready, completion/failure, step completion, clarification cleanup, teardown, and fallback reset) must defer or drop their mutation; only ordered stream frames matching the captured execution/prompt generation may proceed.
- Allow the existing ordered stream handler to acquire the same guard and persist Codex's terminal usage, session-status, interruption-message, and completion frames. Do not bypass or remove the stream guard globally.
- Reacquire the guard after the lifecycle call returns, re-read the authoritative session and active turn, and reconcile only the captured cancellation generation. If a successor turn exists, fail closed or leave it untouched rather than closing it as part of the cancelled turn.
- Apply the same unlock-around-blocking-wait rule to silent clarification/peer-interrupt call sites that already enter lifecycle cancellation while owning the guard. Keep their distinct message, queue-drain, workflow, and visibility semantics unchanged.
- Leave `cancelWaitTimeout` and `cancelEscalationTimeout` unchanged; they remain the fallback for agents that do not finish after acknowledging cancellation.

---

## Frontend

### Existing shared cancel control

No production frontend change is planned. `chat-input-area.tsx` already awaits the existing `agent.cancel` response, and `chat-input-toolbar-primitives.tsx` already disables the shared cancel button and shows progress during that request. Once the backend response is no longer delayed by the lock cycle, both desktop and mobile controls settle promptly through the existing path; the 15-second client request timeout remains a transport fallback.

### Mobile design contract

- **Desktop outcome and mobile entry point:** both task-detail composers use the existing shared cancel control and leave the progress state promptly after acknowledged cancellation.
- **Nearest shipped exemplar:** `apps/web/e2e/tests/workflow/mobile-workflow-cancel-completion.spec.ts` already exercises the compact task composer by touch and is extended rather than adding a new surface.
- **Hierarchy and presentation:** unchanged inline composer control; no drawer, route, navigation, copy, or layout change.
- **Geometry and scrolling:** existing touch target, chat scroll ownership, safe-area behavior, and zero-horizontal-overflow contract remain unchanged.
- **Shared logic:** the same `SubmitButton` request state and backend operation serve desktop and mobile.
- **Mobile proof:** the existing `mobile-chrome` cancellation flow asserts the control and session settle inside the bounded responsiveness window.

---

## Tests

- **What:** acknowledged cancellation can receive terminal stream callbacks while the lifecycle cancel is waiting, then reconciles the captured turn exactly once.
  **Files:** `apps/backend/internal/orchestrator/event_handlers_test.go`, `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** add a mock cancel hook that starts the real `handleAgentStreamEvent` callback and waits for it; use channel synchronization and a short context deadline so current code fails deterministically without a ten-second sleep.
- **What:** ordinary stream side effects still serialize with cancellation reconciliation outside the lifecycle wait.
  **File:** `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** retain and extend `TestAgentStreamEventWaitsForCancellationGuard` to prove the guard remains authoritative before snapshot and after lifecycle completion.
- **What:** concurrent user cancel requests join one operation and observe one result, message, turn closure, and completion evaluation.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** extend the existing channel-driven deduplication test for joining callers, shared result delivery, cleanup, and a later independent retry.
- **What:** cancellation cannot reconcile a successor turn created after the captured turn, and stale terminal frames cannot mutate it.
  **Files:** `apps/backend/internal/orchestrator/task_operations_test.go`, `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** stage turn replacement at the unlocked boundary and assert the captured-turn ownership check leaves the successor active.
- **What:** silent clarification and peer-interrupt cancellation do not reproduce the same stream/cancel cycle and preserve their existing no-visible-message or queue-delivery contracts.
  **Files:** `apps/backend/internal/orchestrator/event_handlers_clarification_test.go`, `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** reuse channel-synchronized lifecycle hooks and existing semantic assertions.
- **What:** cancellation sources share one lifecycle call, a disconnected owner cannot abort a live joiner, and a peer message arriving during explicit cancellation remains queued.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** stage the lifecycle call with channels, race explicit/silent and silent/silent sources, cancel the owner context, and assert one manager call plus an undispatched queue entry.

## E2E Tests

- **Scenario:** a delayed mock-agent turn acknowledges cancellation and emits terminal events; the desktop cancel control disappears and the input becomes promptable within two seconds after those terminal frames, while enabled/disabled completion policy still advances or stays exactly once.
  **File:** `apps/web/e2e/tests/workflow/workflow-cancel-completion.spec.ts`.
- **Scenario:** the same acknowledged cancellation settles through the compact composer within two seconds after terminal-frame publication when invoked by touch, while the existing mobile workflow transition and overflow assertions remain valid.
  **File:** `apps/web/e2e/tests/workflow/mobile-workflow-cancel-completion.spec.ts`.

## Verification Results

- Backend RED/GREEN: the channel-synchronized stream-drain regression first failed on the old implementation because `CancelAgent` held the per-session guard while the lifecycle cancel waited for `handleAgentStreamEvent`; after the two-phase unlock, it passes without escalation. Cross-kind ownership, owner-disconnect, prompt/event admission, ready-vs-cancel parking, successor-turn ownership, silent cancellation, and ordinary stream-guard coverage are green.
- `cd apps/backend && go test ./internal/orchestrator -run 'Test(CancelAgent|CancellationSources|CancellationStreamAdmission|QueueAndInterruptForPeerMessage|CancelAgentSilent|AgentStreamEventWaitsForCancellationGuard)' -count=1` — focused regression suite passed.
- `cd apps/backend && go test ./internal/orchestrator -count=1 -timeout=180s` — passed after the final admission-gate and joined-source changes.
- `cd apps/backend && go test -race ./internal/orchestrator -run 'Test(CancellationSources|CancelAgent_JoinedSilentCancellationRunsExplicitReconciliation|CancelAgent_OwnerDisconnectDoesNotAbortJoinedOperation|CancelAgent_DefersLifecycleCompletionDuringCancellation|CancelAgentSilent_DoesNotCloseSuccessorTurn|QueueAndInterruptForPeerMessage_(ClosesStaleEarlyCheckRace|CancelFailureDoesNotStrandMessageWhenReadyIsRacing))' -count=1` — passed.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/orchestrator -count=1 -timeout=180s` — lifecycle and backendapp packages passed; orchestrator package was covered by the preceding full run.
- `cd apps/backend && go test ./... -count=1 -timeout=300s` — all backend packages passed.
- `cd apps && pnpm --filter @kandev/web run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web run lint` — passed.
- `cd apps && pnpm --filter @kandev/web run i18n:ratchet && pnpm --filter @kandev/web run i18n:check` — passed.
- `cd apps/backend && go test ./cmd/mock-agent -count=1` — package passed, including the cancellation-aware prompt fixture test.
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestManager_CancelAgent_' -count=1` — lifecycle cancellation tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed.
- Desktop E2E: `cd apps/web && pnpm e2e:run --no-build --project chromium tests/workflow/workflow-cancel-completion.spec.ts -- --retries=0` — 2 tests passed; both enabled and disabled completion-policy outcomes settled through the real cancel control.
- Mobile E2E: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts -- --retries=0` — 1 test passed; touch entry, compact composer settlement, workflow transition, and overflow assertions passed. A final repeat with `--repeat-each=3 --workers=1` passed 3/3 after rebuilding the cancellation-aware mock agent.
- `git diff --check` — passed. No ACP protocol, WebSocket, persistence schema, public API, frontend production component, user-facing copy, or new trust boundary changed. Lifecycle cancellation timeouts remain unchanged.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation; the E2E task depends on the backend behavior and neither task is parallel-safe.

Wave 1:

- [x] [Task 01: Break the cancellation stream cycle](task-01-break-cancellation-stream-cycle.md)

Wave 2:

- [x] [Task 02: Prove responsive cancellation end to end](task-02-prove-responsive-cancellation-e2e.md)

## Risks and Out of Scope

- The shared guard was introduced to prevent stream persistence from racing coordinator-stop and turn-replacement decisions. The fix must narrow its hold interval, not remove the guard or let stream handlers bypass it.
- Releasing the mutex during the lifecycle wait creates a deliberate concurrency window. Cancellation operation ownership and captured-turn revalidation are mandatory so duplicate requests, peer interrupts, or a successor prompt cannot steal that window.
- The pending `feature/when-i-have-a-task-w-ge1` branch also evolves backend cancellation ownership in `service.go` and `task_operations.go`. Implementation must reconcile that branch's backend-owned progress contract if it lands first; do not overwrite it or reintroduce a second independent registry.
- The retained ACP captures under `acp-debug/` are diagnostic evidence, not automated fixtures and do not need to ship with the fix.
- This plan does not reduce lifecycle timeouts, change ACP messages, synthesize completion, alter cancellation copy/layout, drain queued messages, or change workflow cancellation policy.
