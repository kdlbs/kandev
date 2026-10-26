---
id: "01-priority-fields"
title: "Carry priority through existing task tools"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-PRIORITY-001
acceptance_criteria:
  - AC-TASKS-MCP-PRIORITY-001.1
  - AC-TASKS-MCP-PRIORITY-001.2
  - AC-TASKS-MCP-PRIORITY-001.4
  - AC-TASKS-MCP-PRIORITY-001.5
  - AC-TASKS-MCP-PRIORITY-001.6
  - AC-TASKS-MCP-PRIORITY-001.7
system_design:
  - ../../specs/tasks/system-design/mcp-task-priority.md
---

# Task 01: Carry priority through existing task tools

## Summary

Add priority to the existing create and generic update tools and their backend
request mappings. Reuse canonical validation and the task service's field-scoped
priority update without changing task-state semantics.

## In scope

- Add the four-token enum to both schemas, with `medium` default metadata only
  on creation. Use existing model constants.
- Carry creation priority through the server payload, `mcpCreateTaskRequest`,
  and `service.CreateTaskRequest`.
- Carry update priority through the server payload and a nullable backend
  pointer into `service.UpdateTaskRequest`. Leave other omitted fields nil.
- Validate backend priority before repository resolution or deferred-prompt edits.
- Add the server and handler regressions named in the plan. Use real SQLite,
  task service, and event-bus fixtures for persistence and publication.
- Prove all four tokens, omission, parent non-inheritance, repeated values,
  combined updates, and preservation of unrelated task and session fields.
- Prove invalid schema values never dispatch. Prove invalid backend strings do
  not write task fields or replace a pending launch prompt.
- Prove retries with a changed or omitted priority return the original task.
- Update the existing exact creation-property assertion for the new field.

## Out of scope

- External catalog expansion, public docs, or new tool/action names.
- Changes to state aliases, deferred-prompt semantics, or storage contracts.

## Acceptance

- The new priority regressions fail against the original code for the observed
  schema or persistence gap, then pass through the real task service.
- Valid create and update writes retain default, omission, event, and retry
  behavior specified by the referenced criteria.
- Invalid priority fails before mutation, and priority-only writes preserve
  unrelated fields and session state.

## Verification

Run from the repository root. Record the initial failing regression run before
production edits. Then run the complete block after implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/server ./internal/mcp/handlers -run 'Test(MCPTaskPriority|HandleCreateTaskPriority|HandleUpdateTaskPriority)' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/mcp/server ./internal/mcp/handlers -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service ./internal/task/repository/sqlite -run 'Test(ValidateTaskPriority|Service_.*Priority|UpdateTask_PriorityOnly|UpdateTaskPriorityOnly)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/handlers.go apps/backend/internal/mcp/server/handlers_test.go apps/backend/internal/mcp/server/task_priority_test.go apps/backend/internal/mcp/handlers/create_task_mode.go apps/backend/internal/mcp/handlers/handlers.go apps/backend/internal/mcp/handlers/task_priority_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/task_priority_test.go` (new)
- `apps/backend/internal/mcp/handlers/create_task_mode.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/task_priority_test.go` (new)

## Dependencies

None.

## Risks

- Every mapping layer must forward priority. Schema metadata alone is insufficient.
- Backend priority validation must precede deferred-prompt mutation.
- An update default or synthesized state would defeat field-scoped persistence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mcp-task-priority.md)
- [Design](../../specs/tasks/system-design/mcp-task-priority.md), especially
  Request mapping and Validation and failure behavior.
- Existing `create_task_workspace_mode_test.go`, `create_task_external_id_test.go`,
  `deferred_launch_prompt_test.go`, and service priority tests.
- Repository `/tdd` and backend test conventions.

## Results

- Initial focused regressions failed on the missing schema field, create/update
  persistence, and invalid priority mutation, as expected.
- Focused MCP schema, forwarding, SQLite, event, retry, omission, and validation
  regressions passed.
- Service priority tests passed in `internal/task/service` and
  `internal/task/repository/sqlite`.
- Changed-file Go lint and `git diff --check` passed.
- The full server/handler race command passed the server package. The handler
  package reached its 10-minute Go test timeout while an existing live
  clarification test was running. The focused priority test set passed under
  `-race` in both packages. This timeout produced no race report or priority
  regression failure.
