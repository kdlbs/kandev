---
id: "01-backend-preference-contract"
title: "Backend preference contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/port-forwarding-discovery.md"
---

# Task 01: Backend preference contract

Implement the task-scoped `port_forwarding_enabled` metadata mutation and its HTTP boundary. Keep
the existing port discovery, tunnel, proxy, and executor transport behavior unchanged.

## Acceptance

- `PATCH /api/v1/tasks/:id/port-forwarding` accepts a JSON boolean and returns the updated task.
- Toggling merges only `port_forwarding_enabled`, preserves unrelated metadata, and publishes a
  normal `task.updated` event containing the merged metadata.
- Unauthorized/missing tasks and malformed bodies fail with the status/error behavior in the spec;
  no invalid request mutates the task.

## Verification

- `cd apps/backend && go test ./internal/task/service ./internal/task/handlers -run 'Test.*PortForward'`
- `make -C apps/backend test` (if the focused tests pass and the repository test target is available)

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_workflow.go` or a task-scoped service file
- `apps/backend/internal/task/service/service_port_forwarding_test.go`
- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_port_forwarding_test.go`

## Dependencies

None.

## Parallelism

Sequential. The task owns the backend contract consumed by every later task.

## Inputs

- Spec sections: Data model, API surface, Permissions, Failure modes.
- Existing `Service.UpdateTask` authorization/event conventions and `TaskDTO` conversion.
- Existing task HTTP route registration and handler test fixtures.

## Output contract

Report the implementation summary, actual files changed, exact test commands/results, any event or
authorization caveat, and synchronized task/plan status. Do not modify the frontend in this task.

## Results

- Added `MetaKeyPortForwardingEnabled` and a merge-preserving, authorized service mutation that
  publishes the normal `task.updated` event.
- Added `PATCH /api/v1/tasks/:id/port-forwarding` with strict boolean payload validation and
  updated-task response conversion; the handler also rejects trailing JSON values.
- Added service and HTTP coverage for enable, disable, metadata preservation, malformed payloads,
  and missing tasks.
- Changed files:
  - `apps/backend/internal/task/handlers/task_handlers.go`
  - `apps/backend/internal/task/handlers/task_http_handlers.go`
  - `apps/backend/internal/task/handlers/task_port_forwarding_test.go`
  - `apps/backend/internal/task/models/models.go`
  - `apps/backend/internal/task/service/service_port_forwarding_test.go`
  - `apps/backend/internal/task/service/service_workflow.go`
- Event/authorization caveat: no caveat; the mutation uses the existing task visibility
  authorization and publishes after a successful repository write.
- Verification: `rtk go test ./internal/task/service ./internal/task/handlers -run 'Test.*PortForward'`
  — passed (9 tests across 2 packages).
- Verification: `rtk make -C apps/backend test` — passed.
- Synchronized status: Task 01 and the implementation plan are marked completed.
