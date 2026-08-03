---
id: "02-configuration-surfaces"
title: "Workflow configuration surfaces"
status: done
wave: 2
depends_on: ["01-step-contract-and-template"]
plan: "plan.md"
spec: "../../specs/workflow/cancelled-turn-completion/spec.md"
---

# Task 02: Workflow configuration surfaces

## Acceptance

- HTTP workflow-step create/update/list/get contracts and workflow-step WebSocket events expose `cancel_triggers_turn_complete` with correct omitted create, omitted update, explicit true, and explicit false behavior.
- Config-mode MCP create/update schemas forward the optional field, list output returns it explicitly, and config-agent prompt documentation names it.
- Existing workflow mutation authorization and synced-workflow read-only behavior remain unchanged.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/workflow/controller ./internal/workflow/handlers ./internal/mcp/handlers ./internal/mcp/server -run 'Test.*CancelTriggersTurnComplete' -count=1)
```

## Files Likely Touched

- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/controller/controller_test.go`
- `apps/backend/internal/workflow/handlers/handlers.go`
- `apps/backend/internal/workflow/handlers/handlers_test.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers_test.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers_test.go`
- `apps/backend/config/prompts/config-context.md`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 03 after Task 01: files are limited to controller/handler/MCP configuration surfaces and do not overlap orchestrator files.

## Inputs

- Spec `API Surface`, `Permissions`, and configuration scenarios.
- Existing `AutoAdvanceRequiresSignal` request, event, MCP schema, and list-output pattern.

## Risks

- Pointer semantics are required on update; replacing them with a plain boolean would make omission indistinguishable from explicitly disabling the setting.
- The MCP schema allow-list and forwarding allow-list must both include the field or the tool may advertise a value it silently drops.

## Output Contract

Report contract shapes, omission semantics, files changed, focused test results, blockers, and residual risks. Update this task and `plan.md` status in the same conversation.

## Results

Implemented the optional pointer contract for HTTP/WebSocket workflow-step create/update requests, emitted the effective flag in workflow-step mutation events, exposed the flag through config MCP schemas/forwarding/list output, updated the config-agent prompt, and included both signal-gate fields in the task workflow-step DTO converter.

Verification:

- `rtk go test -tags fts5 ./internal/workflow/handlers ./internal/mcp/handlers ./internal/mcp/server -run 'Test.*CancelTriggersTurnComplete' -count=1` — 7 tests passed.
- `rtk go test -tags fts5 ./internal/task/dto -run 'TestFromWorkflowStep_PreservesCancelTriggersTurnComplete' -count=1` — 1 test passed.
- `rtk go test -tags fts5 ./internal/workflow/controller ./internal/workflow/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/task/dto -run 'Test.*CancelTriggersTurnComplete' -count=1` — 8 tests passed across 5 packages.
- `rtk go test -tags fts5 ./internal/workflow/controller ./internal/workflow/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/task/dto` — 511 tests passed across 5 packages.
