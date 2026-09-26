---
id: "03-acp-mode-confirmation"
title: "Confirm ACP mode reports correctly"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.16
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 03: Confirm ACP mode reports correctly

## Summary

Attribute mode observations to the request active when they arrive. An early
report must not be lost, and a cached pre-request mode must not confirm a new
request.

## In scope

- Capture observation sequence before `session/set_mode` and serialize calls per session.
- Settle from an early, late, clamped, or absent report without fabricating an effective mode.
- Emit confirmed/effective/requested values consistently to the orchestrator.

## Out of scope

- Changing the ACP wire protocol.
- Executor startup overlays.

## Acceptance

1. A report before RPC return confirms the current request and persists the reported effective mode.
2. A pre-request report, timeout, or cancellation does not confirm success.
3. Concurrent calls cannot consume each other's report as confirmation.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/adapter/transport/acp -run 'Test(Mode|SetMode|SessionMode)' -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_state.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_state_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_event_test.go`

## Dependencies

None.

## Risks

ACP notifications have no per-request ID. Use adapter-session serialization
and deterministic test barriers rather than a sleep-based race test.

## Parallelism

`parallel-safe`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `002.1`–`002.3`, `002.16`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), mode confirmation.

## Results

Mode reports now carry the active ACP session identity and a monotonic
observation generation. `SetMode` serializes per adapter, captures the
generation before `session/set_mode`, and confirms the first later report even
when it arrives before the RPC response. Stale-session reports are dropped.
Timeout and cancellation return no effective mode, and the event builder keeps
the requested mode separate instead of persisting it as observed state.

Tests use a real in-process ACP connection with deterministic report barriers
to cover an early clamp, competing requests, a pre-request mode, stale-session
reports, and cancellation.

Validation passed:

```bash
cd apps/backend
go test ./internal/agentctl/server/adapter/transport/acp -run 'Test(Mode|SetMode|SessionMode)' -count=1
go test ./internal/agentctl/server/adapter/transport/acp -count=1
```
