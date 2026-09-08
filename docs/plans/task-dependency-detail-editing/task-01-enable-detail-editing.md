---
id: "01-enable-detail-editing"
title: "Add atomic dependency replacement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001
acceptance_criteria:
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.6
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.7
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.8
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.9
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.15
system_design:
  - ../../specs/tasks/system-design/task-dependency-detail-editing.md
---

# Task 01: Add atomic dependency replacement

## Summary

Add a task-scoped route that replaces one task's complete predecessor set as an
atomic dependency operation.

## In scope

- Add the full-set replacement handler and response contract.
- Authorize the edited task and every submitted predecessor.
- Validate empty, duplicate, self, cross-workspace, and cyclic inputs.
- Hold the dependency lock across validation and replacement.
- Apply the edge diff in one SQL transaction.
- Publish task updates for the edited task and every added or removed peer.
- Add repository, service, and handler tests.

## Out of scope

- Frontend changes.
- Multi-task dependency edits.
- Changes to the existing one-edge add and remove routes.
- Changes to `start_when_unblocked`.

## Acceptance

- A successful request replaces the exact predecessor set and returns its
  dependency projection.
- Validation and storage errors leave the prior set unchanged.
- Existing authorization, cycle prevention, deferred launch, and event behavior
  remain in force.

## TDD sequence

1. Add failing repository transaction tests.
2. Add failing service validation and publication tests.
3. Add failing HTTP contract tests.
4. Implement the repository, service, and handler changes.
5. Run the focused tests and refactor with all tests green.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/blockers.go`
- `apps/backend/internal/office/repository/sqlite/blockers_test.go`
- `apps/backend/internal/task/service/service_office.go`
- `apps/backend/internal/task/service/service_dependencies.go`
- `apps/backend/internal/task/service/service_dependencies_test.go`
- `apps/backend/internal/task/handlers/task_dependency_handlers.go`
- `apps/backend/internal/task/handlers/task_dependency_handlers_test.go`
- `apps/backend/internal/task/handlers/task_handlers.go`

## Dependencies

None. The core dependency service and persistence table already exist.

## Risks

- The replacement route must not publish before the storage transaction commits.
- Adding a method to `BlockerRepository` requires updates to its test doubles.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/task-dependency-detail-editing.md`
- `docs/specs/tasks/system-design/task-dependency-detail-editing.md`
- `apps/web/components/task/task-dependency-chip.tsx`
- `apps/backend/internal/task/handlers/task_dependency_handlers.go`
- `apps/backend/internal/task/service/service_dependencies.go`

## Results

Implemented the task-scoped replacement contract while preserving the existing
one-edge routes. The service validates the complete predecessor set while
holding the dependency lock, the repository applies the edge diff in one SQL
transaction, and the handler returns the existing dependency projection. Task
updates publish for the edited task and changed peers after a successful commit.

Validation passed:

- `go test ./internal/office/repository/sqlite ./internal/task/service`
- `go test ./internal/task/handlers`
- `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE make -C apps/backend test`
- `make -C apps/backend lint`
- `git diff --check`
