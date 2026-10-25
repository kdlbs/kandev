---
id: "02-provider-reset-timeout"
title: "Bound provider reset requests"
status: done
wave: 2
depends_on:
  - "01-reset-cancellation-outcome"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.8
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 02: Bound provider reset requests

## Summary

Give the provider context-reset request a 10-second deadline.
Return timeout and caller cancellation without entering the ordinary restart fallback.

## In scope

- Add a named internal reset-request timeout and a child context in `Manager.ResetAgentContext`.
- Cover the request write and response wait with that context.
- Release the child context and client lease on every return.
- Preserve ordinary unsupported-reset fallback and its captured runtime configuration.
- Prove abandoned replies cannot publish reset success or alter successor completion ownership.

## Out of scope

General WebSocket timeout policy, mutex cancellation, new configuration, startup scheduling, and automatic restart after escalation.

## Acceptance

1. A silent peer receives the reset request, then the manager returns a deadline error within 10 seconds. A shorter caller deadline wins.
2. Timeout or caller cancellation never calls restart and never publishes reset-success or boot-ready events. Pending request registration is removed.
3. Fast reset and ordinary unsupported-reset fallback retain configuration restoration and idle dispatch-gate behavior.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestManager_ResetAgentContext_ResetRequestTimeout$' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestManager_ResetAgentContext_|TestManager_RestartAgentProcess_|TestWaitForPendingDispatchedPrompt_' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/agentctl -run 'TestResetSession_|TestSendStreamRequest' -count=1)
git diff --check
```

The RED regression must observe an actual unanswered request, not a missing test hook.
Use an outer cancellation solely to release the pre-fix test after its assertion fails.
Use fake time where the existing transport fixture permits it.
Do not mutate a global timeout concurrently.
Cover the default deadline, shorter parent deadline, disconnect, late response, and unsupported reset.
A timeout while waiting on another writer is separate from this unanswered-response test.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_reset_timeout_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_runtime_config_reset_test.go`
- `apps/backend/internal/agent/runtime/agentctl/agent_session_test.go`

Inspect `client_stream.go` for pending-request cleanup; change it only if the deadline regression proves a defect there.

## Dependencies

Task 01.

## Risks

The remote operation can outlive the local deadline.
Do not treat timeout as unsupported reset or use a detached goroutine to bypass cleanup.
Keep RPC correlation cleanup distinct from provider event generation checks.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#bound-the-provider-reset-request).
- `newRestartMockAgentctlServer` and existing runtime-configuration reset tests.
- `sendStreamRequest` and `newTestClientWithStream`.

## Results

Implemented the bounded provider reset request and verified that an unanswered
reset returns its deadline error without restart fallback or success events.
Pending request correlation is removed before a late response arrives.
The caller-deadline regression also returns at 100 ms, before the internal
10-second request bound. The timeout fixture verifies that terminal session
stop can acquire the reset lock and delete the execution after cleanup.

The ACP adapter regression uses a real adapter transport with a fast
`session/new` and a blocked superseded-session close. `ResetSession` returns
after the new session commits, while a concurrent load waits for serialized
cleanup. The lifecycle delayed-response regression proves that a provider
session created after the caller deadline remains fenced, and its late setup
event cannot repopulate the discarded session state.

Verification passed:

```text
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestManager_ResetAgentContext_ResetRequestTimeout$' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestManager_ResetAgentContext_(ResetRequestTimeout|CallerDeadlinePrecedesRequestTimeout)' -count=1 -v)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestManager_ResetAgentContext_|TestManager_RestartAgentProcess_|TestWaitForPendingDispatchedPrompt_' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/agentctl -run 'TestResetSession_|TestSendStreamRequest' -count=1)
git diff --check
```
