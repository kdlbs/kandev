---
id: "03-retire-decision-mcp"
title: "Retire the decision MCP transport"
status: done
wave: 3
depends_on:
  - "02-inject-decision-skill"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUORUM-RECORDING-001
acceptance_criteria:
  - AC-TASKS-QUORUM-RECORDING-001.9
  - AC-TASKS-QUORUM-RECORDING-001.12
system_design:
  - ../../specs/tasks/system-design/workflow-quorum-decision-recording.md
---

# Task 03: Retire the Decision MCP Transport

## Summary

Remove the now-replaced Office MCP schema and websocket transport. Preserve the
normal Kanban catalog and update internal and public Office inventories to the
11-tool surface plus the task-bound CLI command.

## In scope

- Remove the Office decision tool group and implementation.
- Remove its websocket action, handler dependency/wiring, and transport-only
  test files.
- Update exact Office context/tool-count tests and add negative schema checks.
- Update the public Office MCP/runtime CLI reference and coverage inventory.

## Out of scope

- Removing `DashboardService.RecordAgentDecision` or its engine tests.
- Changing `step_complete_kandev` or any normal Kanban MCP registration.
- Reusing or changing `agentctl kandev approvals decide`.

## Acceptance

- Office `tools/list` and first-turn context omit
  `record_step_decision_kandev`; the Office tool count is 11.
- Normal Kanban tool registration is byte-for-byte unchanged in intent and has
  no Office decision metadata or instructions.
- No dead websocket decision action, handler, dashboard dependency, or
  transport-only test remains.

## Verification

```bash
cd apps/backend && rtk go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -run 'Test.*(ModeOffice|ModeTask|Sysprompt|Decision|HandlerRegistration)' -count=1
cd ../.. && rtk node --test scripts/validate-public-docs.test.mjs && rtk node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/config/prompts/office-context.md`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/internal/mcp/server/agent_decision_tool.go`
- `apps/backend/internal/mcp/server/agent_decision_tool_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/agent_decision_handlers.go`
- `apps/backend/internal/mcp/handlers/agent_decision_handlers_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/pkg/websocket/actions.go`
- `docs/public/automation-and-mcp.md`
- `docs/public/coverage.json`

## Dependencies

- Task 02 verifies the replacement skill and prompt path before deletion.

## Risks

- Exact context-inventory tests must change with the registry so documentation
  cannot drift from `tools/list`.
- Removing shared handler wiring must not affect other Office or Kanban tools.

## Parallelism

`sequential`

## Inputs

- The verified outputs of Tasks 01 and 02.
- Current Office MCP registry, prompt inventory, and public CLI reference.

## Results

Removed the Office decision MCP tool, websocket action, handler dependency and
composition wiring, plus their transport-only tests. The Office context and
public reference now describe the 11-tool catalog and the task-bound CLI
decision command. Kanban registration remains unchanged and negative tests
cover the absence of Office decision metadata from both surfaces.

Verification:

```text
cd apps/backend && rtk go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -run 'Test.*(ModeOffice|ModeTask|Sysprompt|Decision|HandlerRegistration)' -count=1
cd ../.. && rtk node --test scripts/validate-public-docs.test.mjs && rtk node scripts/validate-public-docs.mjs
```

Passed: 22 backend tests, 61 public-doc tests, and validation of 46 published
docs pages.
