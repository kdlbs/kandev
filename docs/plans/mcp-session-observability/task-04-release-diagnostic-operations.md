---
id: "04-release-diagnostic-operations"
title: "Add release diagnostic operations"
status: pending
wave: 4
depends_on: ["03-persist-attachment-reports"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 04: Add release diagnostic operations

## Acceptance

- Authorized users can test one server from the owning executor by
  `session_id` and stored `server_name`; arbitrary URLs, commands, headers,
  args, and environment are rejected.
- The test performs bounded initialize + `tools/list`, cleans up network and
  stdio resources on every exit, and stores only a sanitized reachability
  result.
- Test success never promotes attachment evidence to Active; test failure is
  labelled separately from an earlier agent attachment failure.
- Authorized users can request at most 50 recent stderr lines/16 KiB from a
  live execution.
- Agent output is never persisted or included in the attachment report.
- Session authorization executes before lifecycle, repository, executor, or
  agentctl dependencies.

## Verification

- `cd apps/backend && go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers ./internal/gateway/websocket`

Begin with RED tests for arbitrary-target rejection, guard-before-dependencies,
timeout cleanup, stdio child cleanup, test-result classification, output caps,
and no metadata write containing stderr.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/agent_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

- Task 03 provides authorized session lookup, current effective report, and
  persistence of sanitized test results.

## Parallelism

Sequential. Task 05 consumes the request/response contract.

## Inputs

- Existing `agent.stderr` stream request and `GetAgentStderr`
- Existing session-keyed authorization invariants
- Current MCP server config types
- Task 01 sanitizer and endpoint-test result types

## Output contract

Report request/response shapes, authorization proof, network/stdio cleanup,
stderr bounds, persistence/redaction audit, RED/GREEN results, files changed,
blockers, and risks. Mark this task `done` and update its plan checkbox.
