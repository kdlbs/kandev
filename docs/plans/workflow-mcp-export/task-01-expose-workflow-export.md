---
id: "01-expose-workflow-export"
title: "Expose workflow export through MCP"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-mcp.md"
---

# Task 01: Expose Workflow Export Through MCP

## Intent

Add `export_workflow_kandev` to the configuration and external MCP surfaces. Reuse the existing single-workflow export service.

## Acceptance

- Configuration and external clients receive the new tool with one required `workflow_id` argument. Task and Office clients do not receive it.
- A successful call returns one version 1 `kandev_workflow` JSON document. The import handler accepts the text unchanged within its size limit.
- Missing, unknown, or inaccessible workflow IDs return tool errors without a partial document.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./pkg/websocket
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers_test.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers_test.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/config/prompts/config-context.md`

## Dependencies

None.

## Parallelism

Sequential. This task owns the MCP action, handler, registration, prompt contract, and focused backend tests.

## Inputs

- `docs/specs/integrations/requirements/external-mcp.md`, workflow export sections and scenarios.
- `docs/plans/workflow-mcp-export/plan.md`, Backend and Tests sections.
- The `import_workflow_kandev` action, handler, server bridge, and tests as the nearest pattern.
- `workflow.Service.ExportWorkflow` as the only workflow export assembly path.

## Risks

- Do not bypass the scoped request context.
- Do not create a second portable format or a new size limit.
- Keep internal service errors out of the MCP result.

## Output contract

Report the files changed, the red and green test results, remaining risks, and task status. Update this task and `plan.md` in the same conversation.

## Results

- Added `export_workflow_kandev` to the configuration and external MCP surfaces.
- Reused `workflow.Service.ExportWorkflow` and the existing JSON-text bridge.
- Added coverage for registration, portable output, unchanged import round trips, missing IDs, and service errors.
- Verification: `cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./pkg/websocket` passed with 696 tests.
- PR fixup added read-only, non-destructive, idempotent, closed-world tool annotations; documented the 1 MiB import limit in the tool description; and rejected empty or whitespace-only IDs before forwarding.
- Fixup regression verification: the new annotation and whitespace-ID tests failed before the patch, then passed after it.
- Fixup verification: `go test ./internal/mcp/handlers ./internal/mcp/server ./pkg/websocket` passed with 698 tests and `make -C apps/backend lint` reported 0 issues.
