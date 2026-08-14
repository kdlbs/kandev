---
id: "02-current-turn-backend-authority"
title: "Current-turn backend authority"
status: pending
wave: 2
depends_on: ["01-clarification-regression-red"]
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
---

# Task 02: Current-turn backend authority

## Acceptance

- One repository method returns only pending clarification rows in the latest message turn, with
  matching SQLite/PostgreSQL behavior and legacy missing-status support.
- Detach/expiry fallback and workflow guarding consume that method; repeated detach writes and
  publishes nothing, while repository errors keep the workflow barrier closed.
- A detached bundle in the current turn still supports late answer/rejection. A database-fallback
  response for an older-turn or terminal bundle returns conflict and emits no agent-resume event.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/clarification ./internal/orchestrator
```

The PostgreSQL behavior test skips locally unless `KANDEV_TEST_POSTGRES_DSN` is set and runs in the
repository's PostgreSQL CI job.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_test.go`
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

- Do not filter solely by maximum timestamp; tied timestamps need the existing dialect-specific stable
  order.
- Do not reject a detached current-turn bundle merely because the in-memory Store no longer owns it.
- Use `context.WithoutCancel` for terminal writes already promised durable by the handler.
- Do not make query failure look like no pending clarification in workflow guarding.

## Output contract

Use TDD: add focused failing repository/canceller/handler/guard cases, run RED, implement minimally,
then run the exact package command. Report files, test counts/results, PostgreSQL skip/run state,
blockers/risks, and update task/plan status.

## Results

Pending.
