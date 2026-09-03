---
id: "01-persist-rehome-generations"
title: "Persist atomic rehome claims and loss evidence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
acceptance_criteria:
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.2
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.3
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.1
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.2
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
---

# Task 01: Persist rehome generations and loss evidence

## Summary

Create the durable environment-generation, loss-assessment, and rehome-claim
model. Prove the transaction elects one replacement while retaining the source
binding and all task/workflow/profile identity.

## In scope

- Replayable SQLite and PostgreSQL schema changes.
- Atomic claim/join repository API and task cleanup-barrier enforcement.
- Phase-current loss assessment based on Git status and remote reachability.
- Fresh and upgrade/replay tests, including concurrent claims.

## Out of scope

- Calling lifecycle launch or rendering errors.
- Deleting superseded environments or paths.

## Acceptance

- One active environment generation exists per task while historical bindings
  and sessions remain queryable.
- Concurrent equivalent claims persist one replacement environment and session.
- Unknown or unique repository evidence cannot win an automatic claim.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/task/service -run 'Test.*(Rehome|LossAssessment|TaskEnvironmentGeneration)'
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/service/service_turns.go`

## Dependencies

None.

## Risks

- PostgreSQL partial-index and table-rebuild behavior differs from SQLite.
- Shared-group and inherited workspace bindings must remain fail-closed.

## Parallelism

`sequential`

## Inputs

- Missing Workspace Rehome requirements and system design.
- Existing workspace-binding election and environment-owned Git snapshots.

## Results

Pending.
