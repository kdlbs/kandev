---
id: "01-persist-task-status-summaries"
title: "Persist task status summaries"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/bounded-task-status-delivery.md"
---

# Task 01: Persist Task Status Summaries

## Acceptance

- A typed, bounded `TaskStatusSummary` can be batch-read and atomically
  compared/updated with a monotonic per-task revision; semantic no-ops do not
  write or publish a new revision.
- SQLite and Postgres schema replay create the portable summary store, enforce
  one row per task, and delete the row with its task.
- Boot state, task-list responses, and workflow snapshots carry summaries via
  batched hydration with no per-task query; an absent summary remains a safe
  coarse fallback.

## TDD sequence

1. RED: add repository contract tests for schema replay, batch reads,
   compare/update, no-op behavior, concurrent updates, and cascade deletion.
2. RED: add DTO/boot/list tests for bounded serialization, batch hydration,
   missing rows, and omitted-field compatibility.
3. GREEN: implement the typed model, portable repository, keyed update path,
   DTO field, and batched loaders.
4. REFACTOR: keep timestamps/revisions outside semantic equality, centralize
   JSON validation, and expose narrow provider/repository interfaces.

## Verification

```bash
cd apps/backend && go test ./internal/task/statussummary ./internal/task/repository/... ./internal/backendapp/... ./internal/task/dto
```

## Files likely touched

- `apps/backend/internal/task/statussummary/**`
- `apps/backend/internal/task/models/**`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- task/workflow list handlers and focused tests

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the persisted contract and projector interface.

## Inputs

- Spec sections **Task status summary**, **Persistence guarantees**, and
  **Failure modes**.
- ADR `2026-08-01-separate-task-summary-session-stream-traffic.md`.
- Existing portable/replayable migration conventions in
  `apps/backend/internal/task/repository/sqlite`.
- Existing batched task enrichment in `apps/backend/internal/backendapp/boot_state.go`.

## Risks

- Do not put authoritative message, Git, or PR records into the summary table.
- Do not make task list latency proportional to task count.
- A malformed stored JSON value must be observable and repairable without
  preventing the workspace from loading.

## Verification results

- RED schema replay test initially failed because the new table was absent.
- `cd apps/backend && go test ./internal/task/statussummary ./internal/task/repository/... ./internal/backendapp/... ./internal/task/dto` — passed (539 tests across 6 packages).
- The focused boot snapshot test confirms summaries are hydrated through the batch path and serialized as `statusSummary`.
- The Postgres replay test is environment-gated and skips when no isolated Postgres DSN is configured.

## Output contract

Report RED failures, final schema/model shape, batch-query evidence, exact files
changed, focused test results, migration replay results, and blockers or risks.
