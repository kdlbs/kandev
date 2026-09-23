---
id: "01-durable-layout"
title: "Persist layout and repository placement"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 01: Persist layout and repository placement

## Summary

Add the durable layout and slot-path contract before exposing new controls.
Initial launch and all reuse paths must retain the selected root without rematerializing valid checkouts.

## In scope

- Typed task initial layout, environment layout, and per-slot relative path fields with SQLite/PostgreSQL schema parity.
- Create request, DTO, repository transaction, launch request, preparer, and effective-workspace propagation.
- Legacy classification from validated existing paths. No cardinality-based move, source-path fallback, or destructive repair.
- Parent-root single-repo launch and reuse. Preserve repository setup CWD and exactly-once setup scripts.
- Read/write/update/reset paths, queued launches, workspace-only promotion, additional sessions, and restart reconstruction.

## Out of scope

UI controls, nested materialization, workspace rebind, and new recovery mechanisms.

## Acceptance

1. Persisted layout survives create/read/update and restart in both supported database dialects. Invalid explicit values fail before creation.
2. One-repo parent-root launch and all reuse paths retain the task root. Legacy valid paths remain unchanged, including mixed live and failed/deleted inventory.
3. Setup scripts run at their correct scope once. Additional sessions reuse physical inventory and cannot regenerate paths from source count.

## Tests and TDD

Add proposed `workspace_placement_test.go` to task service and SQLite repository packages.
Add proposed `env_preparer_worktree_placement_test.go` with `TestWorktreePreparer_InitialWorkspaceLayout`.
Add `TestWorkspacePlacementReuse` cases to executor environment reuse tests and service workspace-info tests.
Run new tests red before production changes. Include explicit empty/null request values and unknown enums.
Add upgrade/store-conformance coverage for new columns and legacy defaults.

## Verification

Run from repository root. PostgreSQL cases require the repository's isolated PostgreSQL test fixture.
Record unavailable infrastructure as incomplete evidence, not a successful skip.

```bash
(cd apps/backend && go test ./internal/task/models ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'WorkspacePlacement|InitialWorkspaceLayout|WorkspaceInfo|WorktreePreparer|EnvironmentReuse' -count=1)
(cd apps/backend && go test ./internal/persistence/storeconformance -count=1)
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_requests.go`, task create service and `service_task_environments.go`
- `apps/backend/internal/task/handlers/task_handlers.go`, task creation handlers, and `internal/task/dto/`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`, `task.go`, `task_environment.go`
- `apps/backend/internal/persistence/storeconformance/` and required-store fixtures
- `apps/backend/internal/orchestrator/executor/task_environment.go`, `task_environment_reuse.go`, launch request construction
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`, `env_preparer.go`, environment/reuse validators
- `apps/web/lib/types/http.ts` and task-create request types where the API contract is mirrored

## Dependencies

None. Source baseline inspected: `8acb32c889`. Refresh the base and scoped guidance before implementation.

## Inputs

Design: Data and migration; Launch and reuse. Existing reuse and setup-script tests are the primary patterns.
The additional-session workspace-reuse design is updated to reference the new path authority.

## Risks

Database rows and live executions can disagree after legacy promotion. Migration must preserve that fact instead of falsely marking adoption complete.


## Parallelism

`sequential`

## Results

Implemented durable initial-layout, environment-layout, and per-repository relative-path fields through SQLite schema, migrations, repositories, DTOs, launch/resume projections, worktree preparation, and reuse paths. Existing tasks retain their recorded physical roots; nested placement identity is preserved across lifecycle reconstruction.

Validation: changed backend packages passed with `go test ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/orchestrator/executor ./internal/task/service ./internal/task/repository/sqlite ./internal/worktree -count=1`; store conformance passed with `go test -race ./internal/persistence/storeconformance -count=1`; `make lint`, `make sqlguard`, and `make -C apps/backend build` passed. Live native-harness and cross-platform evidence remain unavailable.
