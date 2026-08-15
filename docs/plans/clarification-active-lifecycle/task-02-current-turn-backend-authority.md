---
id: "02-current-turn-backend-authority"
title: "Current-turn backend authority"
status: completed
wave: 2
depends_on: ["01-clarification-regression-red"]
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
---

# Task 02: Current-turn backend authority

## Acceptance

- One repository method returns only pending clarification rows in the newest durable turn, with
  matching SQLite/PostgreSQL behavior and legacy missing-status support after ownership matches.
- Deleting every message from a newer turn cannot reactivate a pending clarification from an older
  turn.
- Detach/expiry fallback and workflow guarding consume that method; repeated detach writes and
  publishes nothing, while repository errors keep the workflow barrier closed.
- A detached bundle in the current turn accepts a late answer with one resume event or a rejection
  with no resume event. Any database-fallback answer or rejection for an older-turn or terminal bundle
  returns conflict, performs no write, and emits no agent-resume event.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/clarification ./internal/orchestrator
```

The PostgreSQL behavior test skips locally unless `KANDEV_TEST_POSTGRES_DSN` is set and runs in the
repository's PostgreSQL CI job.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_active_clarification_test.go`
- `apps/backend/internal/task/repository/sqlite/message_crud_coverage_test.go`
- `apps/backend/internal/task/repository/sqlite/message_pending_postgres_test.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/clarification/canceller.go`
- `apps/backend/internal/clarification/canceller_test.go`
- `apps/backend/internal/clarification/handlers.go`
- `apps/backend/internal/clarification/handlers_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/clarification_guard.go`
- `apps/backend/internal/orchestrator/clarification_guard_test.go`
- Focused repository-interface mocks that compile against the renamed method

## Dependencies

Task 01.

## Parallelism

Sequential. Task 03 consumes this repository authority and its exact failure semantics.

## Inputs

- Spec `What`, `API surface`, `State machine`, and stale-client scenarios.
- ADR current-turn ownership decision.
- Existing `pendingActionsBySessionQuery` and `pendingActionMessageOrder` dialect patterns.
- Existing clarification handler primary, detached fallback, and stale-dismissed paths.

## Risks

- Do not filter solely by maximum timestamp; use the shared `started_at`, `created_at`, and `id`
  ordering for tied values.
- Derive the boundary from `task_session_turns`, never the latest surviving message.
- Do not reject a detached current-turn bundle merely because the in-memory Store no longer owns it.
- Use `context.WithoutCancel` for terminal writes already promised durable by the handler.
- Do not make query failure look like no pending clarification in workflow guarding.

## Output contract

Use TDD: add focused failing repository/canceller/handler/guard cases, run RED, implement minimally,
then run the exact package command. Report files, test counts/results, PostgreSQL skip/run state,
blockers/risks, and update task/plan status.

## Results

- Added newest-durable-turn clarification authority for SQLite and PostgreSQL, including stable turn
  ordering, missing-status compatibility, and deletion-proof ownership.
- Routed canceller, workflow guard, and detached-response fallback through that authority. Repeated
  detachment is a no-op; superseded/terminal responses return conflict without writes or resume
  events; repository errors fail closed.
- `cd apps/backend && go test ./internal/task/repository/sqlite ./internal/clarification ./internal/orchestrator`
  passed. The environment-gated PostgreSQL case skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` was unset; it remains enabled for PostgreSQL CI.
