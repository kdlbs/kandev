---
spec: docs/specs/platform/requirements/shutdown-turn-failure-suppression.md
created: 2026-08-12
status: implemented
---

# Implementation Plan: Do not surface backend-shutdown turn aborts as agent failures

## Overview
Gate the lifecycle manager's terminal-failure path on graceful-shutdown state so
an in-flight turn aborted by SIGTERM becomes a benign stop instead of a
user-visible FAILED session. The change is a single-package backend fix in
`internal/agent/runtime/lifecycle`: guard `handleCompleteEventMarkState` (the
agent `error`-event chokepoint) and `MarkCompleted` (the process-exit chokepoint)
on the already-existing `Manager.IsShuttingDown()`. When shutting down, mark the
execution `STOPPED` and skip `AgentFailed` publication and
`classifyAndMaybeRemediate`, while still signalling `promptDoneCh` so waiters
unblock. No frontend or orchestrator change is required, because the orchestrator
already treats `AgentStopped` as resumable and never sets `last_agent_error` for
it.

---

## Root cause (confirmed)
Incident task `2a5cad30`, execution `5cb0f5e0`: backend SIGTERM at 19:03:06 tore
down the agent subprocess mid-turn. The agent emitted an `error` event carrying
the ACP transport error `-32603 ... peer disconnected before response`. Flow:
`handleErrorEvent` -> `handleCompleteEvent` -> `finishPromptCompletion` ->
`handleCompleteEventMarkState(isError=true)` -> `MarkCompleted(1, errorMsg)`,
which set status `FAILED`, published `events.AgentFailed`, and ran
`classifyAndMaybeRemediate`. The classifier had no shutdown context, so it logged
`routing_code=agent_runtime_error` (`confidence=low`,
`classifier_rule=phase.poststart.unknown`) and the orchestrator flipped the
session to a terminal FAILED state that the UI rendered as a red error banner.
`Manager.IsShuttingDown()` (set at the top of `StopAllAgents`) was already true at
that moment but was never consulted on the failure path.

---

## Backend

### Area 1 — Shutdown-race guard in the terminal transitions
Package: `apps/backend/internal/agent/runtime/lifecycle/`

- `manager_events.go` `handleCompleteEventMarkState(execution, event, isError)`
  (~L108): at the top of the `if isError` branch, when `m.IsShuttingDown()` is
  true, log WARN `"error completion during shutdown, treating as cancellation"`
  (execution_id, task_id, error) and mark the execution `STOPPED` via a shared
  helper (see Area 2), then `return` without calling `MarkCompleted(1, errorMsg)`.
  This is the primary incident path. `promptDoneCh` is already signalled earlier
  in `finishPromptCompletion` via `handleCompleteEventSignal`, so waiters still
  unblock — do not move or duplicate that.

- `manager_interaction.go` `MarkCompleted(executionID, exitCode, errorMessage)`
  (~L1397): after computing that the status would be `AgentStatusFailed` (the
  `else` branch of the exit-code/error test, ~L1421), when `m.IsShuttingDown()`
  is true, set `exec.Status = v1.AgentStatusStopped` instead of
  `AgentStatusFailed`, keep `FinishedAt`/`ExitCode`/`ErrorMessage` stamping and
  the `persistExecutorRunning`/`releaseActivity`/trace-span teardown, publish
  `events.AgentStopped` instead of `events.AgentFailed`, and do NOT call
  `classifyAndMaybeRemediate`. This covers process-exit paths that reach
  `MarkCompleted` directly (not only the agent `error` event). Keep the existing
  duplicate-terminal guard (~L1405) unchanged.

### Area 2 — Shared benign-stop helper (avoid duplicated blocks)
Add one small unexported helper on `*Manager`, e.g.
`markStoppedDuringShutdown(execution *AgentExecution, exitCode int, errorMessage string)`,
that performs the STOPPED status stamp + terminal teardown + `AgentStopped`
publish used by both call sites, so the two guards do not duplicate a ≥150-token
block (backend lint limit). `handleCompleteEventMarkState` calls it; the
`MarkCompleted` guard reuses the same status/publish decision. Keep each guarded
function under the 80-line / complexity limits by extracting rather than inlining.

Note: `AgentStatusStopped` and the `events.AgentStopped` publish already mirror
exactly what `StopAgentWithReason` does for the `StopReasonBackendShutdown` path,
so the orchestrator's `handleAgentStopped` handles it as a known, non-FAILED,
resumable outcome with no new wiring.

---

## Frontend
> No user-facing change. Once the backend stops marking the session FAILED, the
> existing `FAILED`-gated red banner (`agent-status.tsx` STATE_CONFIG.FAILED) is
> simply never triggered for this case. No component, client, or store change.

---

## Tests
All backend, in `apps/backend/internal/agent/runtime/lifecycle/`. Use the
existing helpers: `newTestManager(t)` / `createTestManagerWithTracking()` +
`createTestExecution(...)`, and `MockEventBus.OnPublish` to capture published
subjects.

- **What:** shutdown-race error completion does not fail the execution.
  **File:** new `manager_events_shutdown_test.go` (avoid growing the large
  existing test file per the 800-line rule).
  **How:** build a Manager, `mgr.StopAllAgents(ctx)` (or set shutting-down) so
  `IsShuttingDown()` is true, add a RUNNING execution, call
  `handleCompleteEventMarkState(exec, errorEvent{is_error:true, error:"...peer
  disconnected before response"}, true)`. Assert final status is `STOPPED` (not
  `FAILED`), assert no `events.AgentFailed` subject was published (capture via
  `MockEventBus.OnPublish`), assert `events.AgentStopped` was published.
- **What:** classifier is not invoked on shutdown-race abort.
  **File:** same as above. **How:** inject a recording `remediateNpxCache` stub /
  spy on the classify log or use a seam; at minimum assert no `AgentFailed` and
  status `STOPPED` (classify only runs on the `AgentFailed` branch, so absence of
  `AgentFailed` is the observable proxy). Prefer asserting via the failure branch
  not being taken.
- **What:** genuine failure while running normally is unchanged.
  **File:** same file. **How:** with `IsShuttingDown()` false, call the same
  error completion; assert status `FAILED` and `events.AgentFailed` published
  (regression guard mirroring existing `TestManager_MarkCompleted_Failure`).
- **What:** `MarkCompleted` failure branch honors shutdown.
  **File:** `manager_lifecycle_test.go` sibling or the new file. **How:** set
  `IsShuttingDown()` true, `MarkCompleted("id", 1, "boom")`; assert status
  `STOPPED` + `AgentStopped`. With it false, assert `FAILED` + `AgentFailed`
  (extends existing `TestManager_MarkCompleted_Failure`).
- **What:** waiter still unblocks on shutdown-race abort (no hang).
  **File:** new file. **How:** confirm `promptDoneCh` receives the completion
  signal (the signal is emitted before the guard); assert a non-blocking receive
  succeeds.

Run: `cd apps/backend && go test ./internal/agent/runtime/lifecycle/...`
(package uses `goleak.VerifyTestMain`, so ensure `cleanupManagerStopCh` is used
via `newTestManager`).

---

## E2E Tests
> Skipped: no user-visible UI change (the outcome is the absence of a red
> banner). The behavior is fully covered by the backend unit tests above; an E2E
> that kills the backend mid-turn is out of proportion for a logging/state gate.

---

## Verification Results
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle/` -> `ok`
  (full package pass, ~48s).
- `golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m`
  -> `0 issues`; `gofmt -l` on changed files -> clean.
- See `task-01-shutdown-failure-guard.md` `## Results` for the full list.

---

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-shutdown-failure-guard](task-01-shutdown-failure-guard.md)
```

Single-task fix; no parallelism. The regression tests land in the same task as
the guard so the test fails before and passes after the code change.

## Open Questions
(none)
