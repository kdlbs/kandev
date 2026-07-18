---
id: "01-backend-detachment"
title: "Backend detach contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/subtask-detachment.md"
---

# Task 01: Backend detach contract

## Acceptance

- `POST /api/v1/tasks/:id/detach` clears only the parent relationship and returns the updated task; root calls are idempotent.
- Detaching an `inherit_parent` task atomically stores `shared_group` mode without releasing workspace membership; other modes remain unchanged.
- The resulting `task.updated` payload explicitly clears `parent_id`, and the Office `No parent` mutation uses the same semantic operation.

## Verification

```bash
cd apps/backend && rtk go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard
```

## Files likely touched

- `apps/backend/internal/task/service/service_detachment.go`
- `apps/backend/internal/task/service/service_detachment_test.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/office/dashboard/handler.go`

## Dependencies

None.

## Inputs

- Spec sections: What, Data model, API surface, Failure modes, Scenarios.
- Existing update pattern: `Service.UpdateTask` and `httpUpdateTask`.
- Existing workspace policy: `WorkspacePolicy.MetadataBlock` and orchestrator `shared_group` inheritance.

## Output contract

Report the service/handler changes, tests run, files changed, blockers, residual risks, and update this task plus `plan.md` to `done` when acceptance passes.
