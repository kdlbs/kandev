---
id: "02-add-activity-reconstruction"
title: "Add durable activity reconstruction"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-last-activity-sort.md"
---

# Task 02: Add durable activity reconstruction

Create the authoritative batched read that reconstructs task activity from
task, user-message, queued-prompt, and turn records.

## Acceptance

- One repository call returns the latest activity for every requested task ID.
- The result includes task creation and mutation, user prompts including
  non-reserved queued prompts, and turn start or completion. It excludes agent
  messages, reserved queue entries, and session metadata timestamps.
- SQLite and Postgres behavior is covered without an N+1 query.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TaskLastActivity'
cd apps/backend && go test ./internal/task/service -run 'StatusSummary.*Activity.*Rebuild'
```

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary_test.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary_postgres_test.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/backendapp/services.go`

## Dependencies

None.

## Risks

- The task-ID filter must apply inside every union branch.
- Null completion times must not erase a turn start.
- Add an index only after query-plan evidence shows the existing indexes are
  insufficient.

## Result

- RED: the repository test failed before `LoadTaskLastActivity` existed.
- GREEN: `go test ./internal/task/repository/sqlite -run 'TaskLastActivity'`
  passed (1 SQLite test). The Postgres counterpart is environment-gated.
- GREEN: `go test ./internal/task/service -run
  'StatusSummary.*Activity.*Rebuild'` passed (1 rebuild test).
- GREEN: queued user prompts are included in the same batched max query while
  agent-owned queued rows remain excluded; affected repository, projector, and
  handler packages passed 1,031 tests.
