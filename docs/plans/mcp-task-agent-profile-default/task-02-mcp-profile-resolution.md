---
id: "02-mcp-profile-resolution"
title: "MCP profile resolution"
status: done
wave: 2
depends_on: ["01-backend-preference-contract"]
plan: "plan.md"
spec: "../../specs/tasks/mcp-task-agent-profile-default/spec.md"
---

# Task 02: MCP Profile Resolution

## Acceptance

- Omitted-profile `create_task_kandev` calls obey the saved policy for top-level tasks and subtasks, while explicit `agent_profile_id` always wins.
- `current_task` preserves the existing profile chain; `workspace_default` skips parent/source profiles, preserves workflow step/default precedence, then uses the new task's target workspace default and creates no task when neither default can be resolved.
- Executor/executor-profile inheritance and deferred-start metadata behavior remain unchanged.
- The `create_task_kandev` tool description explains explicit override and both omitted-profile policies without changing the tool input schema.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers/... ./internal/mcp/server/... ./internal/backendapp/...
```

## Files Likely Touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

- `01-backend-preference-contract` supplies the normalized enum and preference reader behavior.

## Inputs

- Spec: What, Failure modes, and all MCP creation scenarios.
- Plan: MCP profile resolution.
- Existing symbols: `handleCreateTask`, `resolveMCPLaunchMetadata`, `resolveMCPAutoStartConfigWithError`, `inheritFromTask`, `errMCPAgentProfileRequired`, and `registerMCPAndDebugRoutes`.
- Preserve the existing `create_task_kandev` tool schema and forwarding behavior in `apps/backend/internal/mcp/server/handlers.go`; update only the tool description and its registration coverage.

## Output Contract

Report policy resolution and wiring changes, table-driven cases added, exact tests run, files touched, blockers, and residual compatibility risks. Set this task to `done` and update its plan checkbox only after targeted verification passes.
