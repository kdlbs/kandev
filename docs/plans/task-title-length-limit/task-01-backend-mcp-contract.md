---
id: "01-backend-mcp-contract"
title: "Backend and MCP title contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/title-length-limit.md"
---

# Task 01: Backend and MCP title contract

## Acceptance

- Task creation and title-bearing updates accept 60-character titles and reject longer titles before persistence, while unrelated updates preserve legacy overlong titles.
- HTTP, WebSocket, and MCP callers receive their existing validation response for an overlong title.
- `create_task_kandev.title` advertises `maxLength: 60` and recommends a concise, few-word title.

## Verification

```bash
(
  cd apps/backend
  go test ./internal/task/service -run 'Test(CreateTaskAcceptsTitleAtLimit|CreateTaskRejectsTitleLongerThanLimit|UpdateTaskRejectsTitleLongerThanLimit|UpdateTaskWithoutTitlePreservesLegacyOverlongTitle)'
  go test ./internal/task/handlers -run 'Test(HTTP|WS).*(Create|Update).*Title'
  go test ./internal/mcp/server -run 'Test(CreateTask|UpdateTask)_ToolSchema'
  go test ./internal/mcp/handlers -run 'Test(ClassifyCreateTaskErrorMapsOverlongTitleToValidation|HandleCreateTask_RejectsOverlongTitle|HandleUpdateTask_TitleTooLongReturnsValidation)'
)
```

## Files likely touched

- `apps/backend/internal/task/service/task_title.go`
- `apps/backend/internal/task/service/task_title_test.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/task_update_repositories_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/config/prompts/kandev-context.md`

## Dependencies

None.

## Parallelism

Parallel-safe with Task 02 because backend/MCP files and frontend files are disjoint. Execute sequentially unless the user explicitly authorizes subagents.

## Inputs

- Spec sections: **What**, **API surface**, **Failure modes**, and the API/MCP scenarios.
- Plan sections: **Backend**, **MCP contract**, and **Tests**.
- Existing patterns: `Service.validateCreateTaskRequest`, `handlers.isValidationError`, `classifyCreateTaskError`, and `mcp.MaxLength` in mcp-go.

## Risks

- Do not validate unchanged legacy titles during non-title updates.
- Return the typed validation error before task writes, events, or auto-start metadata can cause side effects.

## Output contract

Report the changed files, exact test commands/results, blockers and remaining risks, then mark this task `done` and update its checkbox in `plan.md`.
