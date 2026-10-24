---
id: "05-apply-idle-session-mcp-changes"
title: "Apply idle-session MCP changes"
status: done
wave: 4
depends_on:
  - "04-resolve-effective-runtime-mcps"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.9
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 05: Apply Idle-Session MCP Changes

## Summary

Track desired and applied session MCP revisions, then reconnect idle ACP
sessions with the complete effective list. Preserve the prior applied state on
failure and defer unsupported providers to the next normal start.

## In scope

- Persist desired revision, applied revision, apply state, sanitized failure,
  and applied attachment-attempt identity.
- Add a task-session selection mutation that saves desired state under the
  existing session authorization and lifecycle lock.
- Defer provider work while a turn is active and trigger it when the session
  becomes idle.
- Add ACP `session/resume` request support through runtime client, agentctl
  control types, handler, and ACP adapter. `LoadSession` already takes an MCP
  list, filters it with `filterMcpServersWithDecisions`, and emits attachment
  evidence; reuse that path rather than adding a second delivery route.
- Prefer resume and fall back to load, reading both capabilities from the ACP
  `initialize` response. Do not gate on `agents.RuntimeConfig`
  `SupportsSessionResume`, which is a static registry flag set to `true` on
  nearly every ACP agent and does not mean the agent implements `session/resume`.
- Before taking the load fallback, report the session state it resets: pending
  and armed wakeups, async turn completions, dialect correlation state, and
  context-window samples.
- Mark passthrough or unsupported adapters `deferred_restart` without an
  automatic process restart.
- Create a new attachment attempt and advance applied revision only after a
  successful provider response.
- Publish apply-state changes through existing task-session state channels.

## Out of scope

- A new ACP protocol extension for in-place active-turn MCP mutation.
- Automatic retry loops or silent session replacement.
- Frontend selection controls.

## Acceptance

- An active turn receives no resume, load, restart, or MCP mutation request.
- Idle ACP sessions use resume, then load fallback, with the same provider
  session identity, working directory, and full effective list.
- Unsupported and failed attempts preserve desired state, prior applied state,
  transcript, profile, and task-session identity with an actionable status.
- A load fallback on a session with an armed wakeup either preserves that wakeup
  or reports its loss before the user confirms, and does not duplicate replayed
  transcript content.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/agentctl -run 'Test.*(ResumeSession|MCP)'
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Resume|Load).*MCP'
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*SessionMCP.*(Idle|Active|Resume|Load|Deferred|Failure)'
cd apps/backend && go test ./internal/task/handlers ./internal/task/service -run 'Test.*SessionMCP'
```

Write lifecycle state-machine tests first. The RED suite must prove that a
failed reconnect currently cannot preserve separate desired and applied state.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/agent_session_test.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_mcp_reconfiguration_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`

## Dependencies

- Task 04 supplies the complete effective resolver and attachment evidence.

## Risks

- Real agents can advertise resume or load but reject a changed MCP list. Tests
  must separate advertised support from successful application.
- An existing field named `SupportsSessionResume` already exists and is the wrong
  gate. Wiring it would send `session/resume` to every ACP agent.
- `session/load` replays the whole conversation under a suppression flag cleared
  at a FIFO barrier. A reconnect that is not followed by a prompt must not let
  replay frames escape as live transcript content.
- Resume and prompt admission share session state. Use existing locks and busy
  signals to avoid a prompt racing the reconnect.
- A failed attempt must not supersede the previous attachment report.

## Parallelism

`sequential`

## Inputs

- Requirement section 005.
- System-design sections `Idle-session reconfiguration`, `Failure and
  recovery`, and `Observability`.
- ACP v1 session setup contract and the real-agent probe results recorded during
  planning.
- ADR-2026-07-30 and ADR-2026-09-01.

## Results

- Added desired/applied revision state, sanitized failure details, attachment attempt identity, and session-scoped persistence.
- Added idle-only ACP reconfiguration with advertised capability checks, `session/resume` preference, `session/load` fallback, and next-start deferral for unsupported paths.
- Protected active turns with the existing lifecycle lock and preserved prior applied state on failed reconnects.
- Added runtime, adapter, lifecycle, handler, and state-channel tests for active, idle, deferred, resume, load, and failure outcomes.
- Verification passed through lifecycle and full backend tests plus desktop/mobile applied-state E2E coverage.
