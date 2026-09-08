---
id: "01-exclude-lifecycle-turns"
title: "Exclude lifecycle turns from activity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.10
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 01: Exclude Lifecycle Turns From Activity

## Summary

Make lifecycle-only turns invisible to `last_activity_at`. Keep live projection
and restart reconstruction on one marker contract.

## In scope

- Add failing live-projector and repository regression tests.
- Ignore lifecycle-only turn events in the task-status projector.
- Filter lifecycle-only rows from the activity reconstruction query.
- Cover SQLite and the environment-gated PostgreSQL path.

## Out of scope

- Change lifecycle-turn creation or message history.
- Change agent resume or task-state transitions.
- Change frontend code or saved-view behavior.

## Acceptance

- A newer lifecycle-only turn does not change live or rebuilt task activity.
- A newer conversational turn still advances task activity.
- SQLite and PostgreSQL use the existing shared marker predicate.

## Verification

```bash
cd apps/backend && go test ./internal/task/statussummary -run 'LastActivity.*Lifecycle|Lifecycle.*LastActivity'
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TaskLastActivity'
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary_test.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary_postgres_test.go`

## Dependencies

None.

## Risks

- A hand-written JSON expression can drift between database dialects.
- An over-broad filter can remove real conversational turns.

## Parallelism

`sequential`

## Inputs

- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.10`
- The activity derivation section in the bounded task-status design.
- `ADR-2026-08-17-separate-task-activity-from-summary-freshness`
- `models.TurnMetaKeyLifecycleOnly` and `turnLifecycleOnlyPredicate`.

## Results

- RED: `cd apps/backend && go test ./internal/task/statussummary -run
  TestProjectorLastActivityIgnoresLifecycleTurns -count=1` failed because the
  lifecycle-only completion timestamp advanced live activity.
- RED: `cd apps/backend && go test ./internal/task/repository/sqlite -run
  'TestTaskLastActivityBatch|TestPostgresTaskLastActivityBatch' -count=1`
  failed because the SQLite batch included the newer lifecycle turn.
- GREEN: `cd apps/backend && go test ./internal/task/statussummary -run
  'LastActivity.*Lifecycle|Lifecycle.*LastActivity'` passed.
- GREEN: `cd apps/backend && go test ./internal/task/repository/sqlite -run
  'TaskLastActivity'` passed.
- GREEN race checks passed for both targeted commands.
- Full affected packages passed: `statussummary` (80 tests) and `sqlite` (814
  tests).
- `gofmt -l` reported no changed Go files, and `git diff --check` passed.
- Changed files: `projector_events.go`, `projector_test.go`,
  `task_status_summary.go`, `task_status_summary_test.go`, and
  `task_status_summary_postgres_test.go`.
- The live projector and durable activity loader now share the existing
  `lifecycle_only` marker contract. No known implementation blockers remain.
