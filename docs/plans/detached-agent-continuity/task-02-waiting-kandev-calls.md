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
  - ../../specs/platform/system-design/detached-agent-continuity-01.md
  - ../../specs/platform/system-design/detached-agent-continuity-02.md
---

# Task 02: Kandev tool calls wait while detached

## Summary

A Kandev MCP call made while no backend stream is attached waits for reattach
and keeps the agent's MCP client alive. It fails only on budget expiry, and
then with text that tells the agent to stop. Launch guarantees that the
harness tool timeout exceeds the budget.

## In scope

- **`ChannelBackendClient.RequestPayload`** (`mcp/server/backend_client.go`):
  - the send loop from the design section "Send loop": a `Snapshot` first;
    while attached, the 5 s bound with a re-check on timeout; while detached,
    wait on `AttachedCh`, `BudgetExhausted`, or `ctx` only, never
    `requestCh`;
  - the writer reads `requestCh` only while its stream is current and
    confirmed, and stops before its stream's pending set is failed, so an
    unconfirmed reconnect stream never carries a call;
  - `writeAgentStreamMCPRequest` binds before it writes: a call is sent once
    `BindRequestToStream` succeeds. Binding to a stream that
    `FailStreamRequests` already failed returns an error, and the writer
    completes the call with `errRequestNotSent`, which returns it to the send
    loop. A write error after the bind calls `FailRequest` with
    `ErrKandevCallOutcomeUnknown`. `FailRequest` on a completed call is a
    no-op.
- **`FailStreamRequests`:** defined in `mcp/server/backend_client.go`; its
  call site in `agentctl/server/api/agent.go`
  runs on every stream end and now passes `ErrKandevCallOutcomeUnknown`. It
  fails only sent calls. It never resends.
- **Harness tool timeout:** an agent's `RuntimeConfig` declares its
  tool-timeout key (Claude: `MCP_TOOL_TIMEOUT`). Launch raises that managed
  default to at least the budget plus 10 minutes, and fails with the typed
  `ErrToolTimeoutBelowOfflineBudget` when a higher-precedence value is below
  the budget plus 1 minute.
- **Keepalive:** one wrapper around `emitKeepAlivePings`
  (`askQuestionKeepAliveInterval`) for every handler that calls
  `RequestPayload`.
- **Error text:** the `ErrOfflineBudgetExhausted` and
  `ErrKandevCallOutcomeUnknown` constants, with the exact text from the
  system design.
- **System prompt:** a `connectionLossSection` in `sysprompt.go` and
  `config/prompts/kandev-context.md`.
- **Codex measurement:** measure Codex's behavior for an MCP call that waits
  longer than 60 s. Record the observed limit in Results. If Codex has a
  fixed limit that cannot be raised, declare it, and launch rejects a budget
  above that limit minus 1 minute with `ErrToolTimeoutBelowOfflineBudget`.

## Out of scope

- Permission requests. `sendPermissionNotification` already parks them.
- Queuing or replaying calls.
- Backend-side changes.

## Acceptance

1. While detached, a call blocks, including while an unconfirmed stream is
   current. After the backend confirms a stream it completes with the
   backend's answer. A call bound to a stream that a new stream supersedes
   returns `ErrKandevCallOutcomeUnknown`; a call not yet bound goes to the new
   stream once it is confirmed. It sends progress notifications at 20 s intervals or
   less while waiting. A call made on a new instance before its first
   confirmation waits on episode 1's channels, as task 01 defines them.
2. Budget expiry returns `ErrOfflineBudgetExhausted` to every waiting call. A
   call that was sent before the drop returns `ErrKandevCallOutcomeUnknown`.
   A call not yet sent when the stream drops waits instead, including one
   racing the drop, under `-race`. A call whose bind finds the stream failed
   waits. A call whose write fails returns `ErrKandevCallOutcomeUnknown`.
3. A 1440-minute budget launches Claude with `MCP_TOOL_TIMEOUT` of at least
   1450 minutes. A profile environment value below the budget plus 1 minute
   fails launch with `ErrToolTimeoutBelowOfflineBudget`.
4. The rendered Kandev context contains the connection-loss guidance.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/mcp/server/... -run 'TestRequestPayload|TestSendRacesDetach|TestFailStreamRequests|TestKandevCallKeepAlive|TestBindToFailedStreamNotSent|TestWriteErrorUnknownOutcome|TestUnconfirmedStreamCarriesNoCall')
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/api/... -run 'TestWriteAgentStreamMCPRequestBindsBeforeWrite')
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... ./internal/agent/agents/... -run 'TestToolTimeoutCoversOfflineBudget')
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
- `apps/backend/internal/agent/agents/claude_acp.go` and
  `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`:
  the declared tool-timeout key and the launch check

## Dependencies

- Task 01: the attachment state and `Snapshot`.

## Risks

- **Stale answers.** A waiting call could be answered by a stream that later
  drops. Bind requests to the stream at write time, as today.
- **Harness tool timeouts.** A harness that times out a call on its own would
  break the wait. The launch check covers declared keys; the Codex
  measurement covers the rest.

## Parallelism

`sequential` after task 01. It can run in parallel with task 04.

## Inputs

- System design part 2 sections: Kandev tool calls while detached; Agent
  guidance.
- `docs/specs/agents/system-design/mcp-timeout-budgets.md`.

## Results

Pending.
