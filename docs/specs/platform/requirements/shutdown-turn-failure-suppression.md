---
status: active
system: platform
created: 2026-08-12
owners:
  - cfl12
---
# Do not surface backend-shutdown turn aborts as agent failures Requirements

## Overview

When the Kandev backend receives SIGTERM while an agent turn is in flight, the
agent subprocess is torn down and the in-flight prompt fails with an ACP
transport error (`-32603 ... peer disconnected before response`). The lifecycle
manager currently treats that error completion as a genuine agent failure: it
marks the execution `FAILED`, publishes `AgentFailed`, and runs the routing
classifier, which labels the abort `agent_runtime_error` (low confidence,
`classifier_rule=phase.poststart.unknown`). The orchestrator then flips the
session to a terminal FAILED state and the UI renders a red error banner for
what was actually a clean, expected shutdown.

This is distinct from the `shutdown-log-noise` spec, which only downgrades log
severity and explicitly excludes any change to state transitions, returned
errors, or the classifier. This spec covers the state/UX regression: a
shutdown-race turn abort must not become a user-visible FAILED session.

Observed incident: task `2a5cad30-cfea-494f-a19c-99b31579879e`, execution
`5cb0f5e0`. Backend SIGTERM at 19:03:06 correlated with the execution failing at
19:03:08 and surfacing `agent_runtime_error` in the UI.

## Requirements

### REQ-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001: Do not surface backend-shutdown turn aborts as agent failures

**Intent:** When the Kandev backend receives SIGTERM while an agent turn is in flight, the agent
subprocess is torn down and the in-flight prompt fails with an ACP transport error (`-32603 ... peer
disconnected before response`). The lifecycle manager currently treats that error completion as a
genuine agent failure: it marks the execution `FAILED`, publishes `AgentFailed`, and runs the
routing classifier, which labels the abort `agent_runtime_error` (low confidence,
`classifier_rule=phase.poststart.unknown`). The orchestrator then flips the session to a terminal
FAILED state and the UI renders a red error banner for what was actually a clean, expected shutdown.
This is distinct from the `shutdown-log-noise` spec, which only downgrades log severity and
explicitly excludes any change to state transitions, returned errors, or the classifier. This spec
covers the state/UX regression: a shutdown-race turn abort must not become a user-visible FAILED
session. Observed incident: task `2a5cad30-cfea-494f-a19c-99b31579879e`, execution `5cb0f5e0`.
Backend SIGTERM at 19:03:06 correlated with the execution failing at 19:03:08 and surfacing
`agent_runtime_error` in the UI.

#### Acceptance criteria

- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.1:** When an in-flight agent turn receives an error completion **because backend graceful shutdown is in progress** (`Manager.IsShuttingDown()` is true), the lifecycle manager SHALL NOT mark the execution `FAILED`, SHALL NOT publish `AgentFailed`, and SHALL NOT run `classifyAndMaybeRemediate`. It SHALL instead treat the turn as a shutdown-induced stop, consistent with the existing `StopReasonBackendShutdown` teardown path.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.2:** The session SHALL remain in a resumable (non-terminal, non-FAILED) state after a shutdown-race turn abort, so the next launch or `Resume` continues cleanly. No `last_agent_error` / red error banner is presented to the user for this case.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.3:** The classifier log line `agent failure classified for provider routing` with `routing_code=agent_runtime_error` SHALL NOT be emitted for a shutdown-race abort.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.4:** A genuine agent failure on an active session (backend NOT shutting down) SHALL continue to mark the execution `FAILED`, publish `AgentFailed`, run the classifier, and surface to the UI exactly as today. Suppression is gated strictly on shutdown state.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.5:** The completion signal on `promptDoneCh` SHALL still fire so any waiter in `waitForPromptDone` / `dispatchInitialPrompt` unblocks; only the terminal FAILED marking and failure publication/classification are suppressed.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.6:** **GIVEN** an agent turn in flight and `Manager.IsShuttingDown()` is true, **WHEN** the agent error completion `-32603 peer disconnected before response` arrives, **THEN** the execution is not marked `FAILED`, no `AgentFailed` event is published, `classifyAndMaybeRemediate` is not called, and no `routing_code=agent_runtime_error` log line is emitted.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.7:** **GIVEN** the same shutdown-race abort, **WHEN** the session state is later read, **THEN** it is not in a terminal FAILED state and has no `last_agent_error`, so no red error banner is shown and the task is resumable.
- **AC-PLATFORM-SHUTDOWN-TURN-FAILURE-SUPPRESSION-001.8:** **GIVEN** a genuine agent error completion while the backend is running normally (`IsShuttingDown()` false), **WHEN** it arrives, **THEN** the execution is marked `FAILED`, `AgentFailed` is published, and the classifier runs exactly as today.

## Migrated source detail

## Why
When the Kandev backend receives SIGTERM while an agent turn is in flight, the
agent subprocess is torn down and the in-flight prompt fails with an ACP
transport error (`-32603 ... peer disconnected before response`). The lifecycle
manager currently treats that error completion as a genuine agent failure: it
marks the execution `FAILED`, publishes `AgentFailed`, and runs the routing
classifier, which labels the abort `agent_runtime_error` (low confidence,
`classifier_rule=phase.poststart.unknown`). The orchestrator then flips the
session to a terminal FAILED state and the UI renders a red error banner for
what was actually a clean, expected shutdown.

This is distinct from the `shutdown-log-noise` spec, which only downgrades log
severity and explicitly excludes any change to state transitions, returned
errors, or the classifier. This spec covers the state/UX regression: a
shutdown-race turn abort must not become a user-visible FAILED session.

Observed incident: task `2a5cad30-cfea-494f-a19c-99b31579879e`, execution
`5cb0f5e0`. Backend SIGTERM at 19:03:06 correlated with the execution failing at
19:03:08 and surfacing `agent_runtime_error` in the UI.

## What
- When an in-flight agent turn receives an error completion **because backend
  graceful shutdown is in progress** (`Manager.IsShuttingDown()` is true), the
  lifecycle manager SHALL NOT mark the execution `FAILED`, SHALL NOT publish
  `AgentFailed`, and SHALL NOT run `classifyAndMaybeRemediate`. It SHALL instead
  treat the turn as a shutdown-induced stop, consistent with the existing
  `StopReasonBackendShutdown` teardown path.
- The session SHALL remain in a resumable (non-terminal, non-FAILED) state after
  a shutdown-race turn abort, so the next launch or `Resume` continues cleanly.
  No `last_agent_error` / red error banner is presented to the user for this
  case.
- The classifier log line `agent failure classified for provider routing` with
  `routing_code=agent_runtime_error` SHALL NOT be emitted for a shutdown-race
  abort.
- A genuine agent failure on an active session (backend NOT shutting down) SHALL
  continue to mark the execution `FAILED`, publish `AgentFailed`, run the
  classifier, and surface to the UI exactly as today. Suppression is gated
  strictly on shutdown state.
- The completion signal on `promptDoneCh` SHALL still fire so any waiter in
  `waitForPromptDone` / `dispatchInitialPrompt` unblocks; only the terminal
  FAILED marking and failure publication/classification are suppressed.

## Affected code and target behavior
1. `internal/agent/runtime/lifecycle/manager_events.go`
   `handleCompleteEventMarkState(execution, event, isError=true)` (~L108) is the
   single chokepoint reached by both the agent `error` event
   (`handleErrorEvent` -> `handleCompleteEvent` -> `finishPromptCompletion`) and
   the `is_error` complete path. When `isError` and `m.IsShuttingDown()`, log a
   benign WARN ("error completion during shutdown, treating as cancellation")
   and skip the `MarkCompleted(1, errorMsg)` call. Leave `promptDoneCh`
   signalling (done earlier in `finishPromptCompletion` via
   `handleCompleteEventSignal`) untouched.
2. `internal/agent/runtime/lifecycle/manager_interaction.go` `MarkCompleted`
   (~L1397) is also reachable from process-exit paths. Guard the failure branch
   there too: when the computed status is `Failed` and `m.IsShuttingDown()`,
   mark the execution `Stopped` (terminal, non-failure) instead of `Failed`, and
   do not publish `AgentFailed` or call `classifyAndMaybeRemediate`. This keeps
   the guard correct regardless of which path reaches the terminal transition.
   `IsShuttingDown()` already exists (`manager_lifecycle.go` L174), set true at
   the very start of `StopAllAgents` before teardown begins.

## Failure modes
- Gating is on `Manager.IsShuttingDown()` only. It is impossible to suppress a
  real failure that happens while the backend is running normally, because the
  flag is set exclusively by `StopAllAgents` at the start of graceful shutdown.
- A turn that fails microseconds before SIGTERM (flag still false) is classified
  normally; this is acceptable and matches the existing conservative-classifier
  convention. The fix targets the dominant race where teardown is already in
  progress.

## Scenarios
- **GIVEN** an agent turn in flight and `Manager.IsShuttingDown()` is true,
  **WHEN** the agent error completion `-32603 peer disconnected before response`
  arrives, **THEN** the execution is not marked `FAILED`, no `AgentFailed` event
  is published, `classifyAndMaybeRemediate` is not called, and no
  `routing_code=agent_runtime_error` log line is emitted.
- **GIVEN** the same shutdown-race abort, **WHEN** the session state is later
  read, **THEN** it is not in a terminal FAILED state and has no
  `last_agent_error`, so no red error banner is shown and the task is resumable.
- **GIVEN** a genuine agent error completion while the backend is running
  normally (`IsShuttingDown()` false), **WHEN** it arrives, **THEN** the
  execution is marked `FAILED`, `AgentFailed` is published, and the classifier
  runs exactly as today.
- **GIVEN** any shutdown-race abort, **WHEN** `waitForPromptDone` /
  `dispatchInitialPrompt` are waiting, **THEN** the completion signal still
  fires and the waiter unblocks (no hang on shutdown).

## Out of scope
- Log-severity downgrades of the same teardown races (owned by the
  `shutdown-log-noise` spec).
- Changing shutdown ordering, timeouts, or which subprocesses are killed.
- Retry/fallback/provider-routing behavior for genuine failures.
- Frontend changes: no red banner is expected once the backend stops marking the
  session FAILED, so no frontend change is required.
