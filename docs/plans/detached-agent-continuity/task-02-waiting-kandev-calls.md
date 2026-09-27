---
id: "02-waiting-kandev-calls"
title: "Kandev tool calls wait while detached"
status: pending
wave: 2
depends_on: ["01-offline-budget"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-005.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-005.2
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 02: Kandev tool calls wait while detached

## Summary

A Kandev MCP call made while no backend stream is attached waits for reattach
and keeps the agent's MCP client alive. It fails only on budget expiry or at
the harness's tool timeout, and then with text that tells the agent to stop.

## In scope

- **`ChannelBackendClient.RequestPayload`** (`mcp/server/backend_client.go`):
  - keep the 5 s send bound while attached;
  - while detached, wait on `requestCh`, `attached`, `budgetExhausted`, or
    `ctx`;
  - cap the wait at the smaller of the budget and the declared tool timeout,
    minus 1 minute.
- **`FailStreamRequests`** returns `ErrKandevCallOutcomeUnknown` for calls
  that were written but not answered. It never resends.
- **Keepalive:** one wrapper around `emitKeepAlivePings`
  (`askQuestionKeepAliveInterval`) for every handler that calls
  `RequestPayload`.
- **Error text:** the `ErrOfflineBudgetExhausted` and
  `ErrKandevCallOutcomeUnknown` constants, with the exact text from the
  system design.
- **System prompt:** a `connectionLossSection` in `sysprompt.go` and
  `config/prompts/kandev-context.md`.
- **Codex measurement:** measure Codex's behavior for an MCP call that waits
  longer than 60 s. Record the observed limit in Results. If it is shorter
  than the budget, pass it as the declared tool timeout.

## Out of scope

- Permission requests. `sendPermissionNotification` already parks them.
- Queuing or replaying calls.
- Backend-side changes.

## Acceptance

1. While detached, a call blocks. After a stream attaches it completes with
   the backend's answer. It sends progress notifications at 20 s intervals or
   less while waiting.
2. Budget expiry returns `ErrOfflineBudgetExhausted` to every waiting call. A
   call that was written before the drop returns `ErrKandevCallOutcomeUnknown`.
3. The rendered Kandev context contains the connection-loss guidance.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/mcp/server/... -run 'TestRequestPayload|TestFailStreamRequests|TestKandevCallKeepAlive')
(cd apps/backend && go test -race -count=1 ./internal/sysprompt/... -run 'TestKandevContextHasConnectionLossSection')
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/api/...)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/mcp/server/backend_client.go` and
  `backend_client_test.go`
- `apps/backend/internal/mcp/server/handlers.go`,
  `apps/backend/internal/mcp/server/server.go`: the keepalive wrapper
- `apps/backend/internal/agentctl/server/api/agent.go`: wiring the
  `AttachmentWaiter`
- `apps/backend/internal/sysprompt/sysprompt.go`,
  `apps/backend/config/prompts/kandev-context.md`

## Dependencies

- Task 01: the `budgetExhausted` signal and `WaitSignals`.

## Risks

- **Stale answers.** A waiting call could be answered by a stream that later
  drops. Bind requests to the stream at write time, as today.
- **Harness tool timeouts.** The harness may time out a waiting call on its
  own. The Codex measurement above decides the cap.

## Parallelism

`sequential` after task 01. It can run in parallel with task 04.

## Inputs

- System design sections: Kandev tool calls while detached; Agent guidance.
- `docs/specs/agents/system-design/mcp-timeout-budgets.md`.

## Results

Pending.
