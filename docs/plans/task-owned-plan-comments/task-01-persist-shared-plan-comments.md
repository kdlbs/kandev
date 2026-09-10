---
id: "01-persist-shared-plan-comments"
title: "Persist shared plan comments"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.1
  - AC-TASKS-PLAN-COMMENTS-001.2
  - AC-TASKS-PLAN-COMMENTS-001.3
  - AC-TASKS-PLAN-COMMENTS-001.4
  - AC-TASKS-PLAN-COMMENTS-001.6
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 01: Persist shared plan comments

## Summary

Create the backend source of truth for pending comments on the current task
plan. Expose authorized CRUD and complete versioned snapshots over WebSocket,
including lifecycle cascades and live task notifications.

## In scope

- `TaskPlanComment`, `task_plan_comments`, and
  `task_plans.comments_revision` with replayable SQLite/Postgres schema changes.
- Task/plan integrity constraints, stable ordering, optimistic row versions,
  and caller-UUID create idempotency.
- Repository, Plan service, DTO, WebSocket action/handler, and task-event
  broadcaster integration.
- Complete list/mutation/event snapshots and task authorization tests.

## Out of scope

- Message or queue delivery and comment consumption.
- Frontend state, rendering, or browser migration.

## Acceptance

- CRUD is authorized by `task_id`, independent of every session ID, and returns
  one complete `{task_id, plan_id, revision, comments}` snapshot.
- Every mutation is transactional, increments the collection revision once,
  rejects stale plan/version conflicts, and publishes only after commit.
- Plan/task deletion cascades comments, while session deletion and primary
  changes do not alter them; schema creation and replay pass on both dialects.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/gateway/websocket
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/plan_comment.go`
- `apps/backend/internal/task/repository/sqlite/plan_comment_test.go`
- `apps/backend/internal/task/service/plan_comment_service.go`
- `apps/backend/internal/task/service/plan_comment_service_test.go`
- `apps/backend/internal/task/handlers/task_plan_comment_handlers.go`
- `apps/backend/internal/task/handlers/task_plan_comment_handlers_test.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`

## Dependencies

None.

## Risks

- Adding a composite plan/task integrity constraint must remain replayable for
  existing SQLite and Postgres databases.
- Mutation events must not be published for rolled-back writes.

## Parallelism

`sequential`

## Inputs

- Requirements: `REQ-TASKS-PLAN-COMMENTS-001`.
- System design: persistence model, WebSocket contracts, plan lifecycle, and
  security boundary.
- Existing task-plan repository/service/handler patterns and Postgres schema
  replay tests.

## Results

- Added replayable SQLite/Postgres schema, revisioned CRUD, current-plan
  integrity, stable ordering, idempotent creates, and lifecycle cascades.
- Added authorized PlanService operations, complete DTO snapshots, WebSocket
  actions, conflict snapshots, and committed-change broadcasts.
- Focused repository, service, handler, and gateway suites pass.
