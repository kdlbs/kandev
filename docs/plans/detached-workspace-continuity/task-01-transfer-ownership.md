---
id: "01-transfer-ownership"
title: "Transfer workspace ownership"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
acceptance_criteria:
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.1
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.2
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.3
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.4
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.5
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.1
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.2
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.3
system_design:
  - ../../specs/tasks/system-design/detached-workspace-continuity.md
---

# Task 01: Transfer Workspace Ownership

## Summary

Implement generation-fenced workspace stewardship from schema through task
creation, detachment, cleanup, and restart recovery. The result keeps the
detached child's canonical environment alive after former-parent lifecycle
operations and makes every child-creation route establish the same membership
before launch.

## In scope

- Add replayable workspace-group and task-environment ownership generations.
- Implement atomic detachment and guarded owner-transfer repository operations.
- Fence task-resource and workspace-group cleanup by current owner, generation,
  membership, and cleanup barrier.
- Centralize child workspace-policy resolution and attachment in `CreateTask`.
- Add focused SQLite, PostgreSQL, service, concurrency, restart, and entry-point
  regression tests.

## Out of scope

- Workspace copying, session restart, or frontend behavior changes.
- Cleanup changes unrelated to shared ownership.
- New task hierarchy or blocker behavior.

## Acceptance

- Detachment either commits hierarchy, group steward, membership roles,
  environment owner, and incremented generations together or commits none of
  them.
- Parent archive/delete and delayed cleanup cannot stop, delete, reuse, or
  change a detached child's current-generation workspace.
- Every supported child creation route attaches the canonical workspace policy
  before publishing or returning a launchable task, with rollback on failure.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/office/repository/sqlite -run 'Test.*(DetachedWorkspaceContinuity|OwnershipGeneration|WorkspaceGroupGeneration|SchemaReplay)' -count=1
cd apps/backend && go test ./internal/task/service -run 'Test.*(Detach|DetachedWorkspace|Stale.*Cleanup|CreateChildTask.*Workspace)' -count=1
cd apps/backend && go test ./internal/task/handlers ./internal/mcp/handlers ./internal/plugins ./internal/backendapp -run 'Test.*CreateTask.*Workspace' -count=1
KANDEV_TEST_POSTGRES_DSN='<isolated test DSN>' go test ./internal/task/repository/sqlite ./internal/office/repository/sqlite -run 'Test.*(DetachedWorkspaceContinuity|OwnershipGeneration|WorkspaceGroupGeneration|SchemaReplay).*Postgres' -count=1
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/office/models/workspace_groups.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/workspace_group.go`
- `apps/backend/internal/office/repository/sqlite/base_migrations.go`
- `apps/backend/internal/office/repository/sqlite/workspace_groups.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_detachment.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_cleaner.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/backendapp/helpers.go`
- Focused adjacent `*_test.go` files.

## Dependencies

None.

## Risks

- Cross-table lock order must remain identical in detach, lifecycle cleanup,
  and ownership-transfer paths to avoid PostgreSQL deadlocks.
- A cleanup claim that is released too early reopens the external-operation
  race; failure paths must retain or explicitly release the same generation.
- Existing tests and fakes implement narrow repository interfaces and will need
  compatible generation-aware methods.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/detached-workspace-continuity.md`
- `docs/specs/tasks/system-design/detached-workspace-continuity.md`
- `docs/decisions/2026-09-04-generation-fenced-task-environment-ownership.md`
- Existing detachment, handoff, task-resource cleanup, and task-create tests.

## Results

Implemented generation-fenced workspace ownership end to end:

- Added replayable ownership generations to workspace groups and task
  environments, including fresh schema and worktree cutover paths.
- Made detachment transactionally update hierarchy, workspace mode, group
  steward, membership roles, environment owner, and both generations.
- Added cleanup-barrier-aware, expected-owner and expected-generation
  environment transfers; removed the unguarded transfer API.
- Added generation claims for workspace-group cleanup and stale-snapshot guards
  for the complete environment and worktree cleanup pipeline.
- Moved workspace-policy attachment into `Service.CreateTask`, wired all
  production entry points through it, and retained rollback on attachment
  failure.
- Added focused tests for atomic rollback, idempotency, ownership roles,
  generation advancement, stale retry rejection, cleanup fencing, and internal
  child creation.

Verification results:

- Repository focus: 27 passed.
- Service focus: 19 passed.
- HTTP/MCP/plugin/backend entry-point focus: 15 passed.
- Final combined ownership and creation focus: 76 passed across seven packages.
- Race-enabled ownership focus: 37 passed across three packages.
- Backend lint: zero issues.
- Spec linter: 30 tests passed and all spec files passed.
- PostgreSQL-only run: skipped because no isolated DSN was available.
- Full backend run: 9,907 passed, 16 skipped, seven environment-sensitive
  config tests failed because this VM has a real `/root/.kandev/config.yaml`.
