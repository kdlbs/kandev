---
id: "05-map-missing-task-plan-responses"
title: "Map missing-task plan responses"
status: done
wave: 2
depends_on:
  - "04-classify-missing-task-plan-writes"
plan: "plan.md"
spec: "../../specs/tasks/requirements/documents.md"
---

# Task 05: Map missing-task plan responses

Return the stable `not_found` contract from both task-plan handler surfaces.

## Scope

- Add the shared task-not-found sentinel to `planws` create and update mappings.
- Keep `ErrTaskPlanNotFound` semantics unchanged.
- Add shared mapper coverage and integration coverage for browser and MCP plan
  creation against a missing task.
- Assert that responses contain no database constraint details.

## Exclusions

- Do not change success payloads or action names.
- Do not change plan-get or plan-delete behavior.
- Do not duplicate error mapping in each handler package.

## Traceability

- `REQ-TASKS-DOCUMENTS-001`
- `AC-TASKS-DOCUMENTS-001.2`
- Design: `docs/specs/tasks/system-design/plan-write-lifecycle.md`

## Acceptance

- Browser and MCP plan creation for a missing task returns
  `ws.ErrorCodeNotFound`.
- Both responses use a stable task-not-found message and omit storage details.
- Unrelated plan write errors remain `ws.ErrorCodeInternalError`.

## Verification

```bash
cd apps/backend && go test ./internal/task/planws -run TestMappersCoverEverySentinel -count=1
cd apps/backend && go test ./internal/task/handlers ./internal/mcp/handlers -run 'Test.*Plan.*MissingTask' -count=1
```

The handler tests must fail before the production change because the current
create mapping returns `internal_error` with the foreign-key message.

## Files likely touched

- `apps/backend/internal/task/planws/errors.go`
- `apps/backend/internal/task/planws/errors_test.go`
- `apps/backend/internal/task/handlers/task_plan_handlers_test.go`
- `apps/backend/internal/mcp/handlers/task_plan_handlers_test.go`

## Dependencies

- Task 04 must expose `repository.ErrTaskNotFound` from the write boundary.

## Results

Implemented the shared missing-task mapping for create and update plan
responses. Browser and MCP creation and update tests now verify the stable
`not_found` message and reject leaked database constraint details.

Verification:

```text
cd apps/backend && go test ./internal/task/planws -run TestMappersCoverEverySentinel -count=1
Go test: 14 passed in 1 packages

cd apps/backend && go test ./internal/task/handlers ./internal/mcp/handlers -run 'Test.*Plan.*MissingTask' -count=1
Go test: 14 passed in 2 packages
```
