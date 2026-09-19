---
id: "01-authoritative-queue-cancellation"
title: "Make pending queue cancellation authoritative"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-management.md"
---

# Task 01: Make Pending Queue Cancellation Authoritative

## Acceptance

- Individual remove succeeds for visible user, agent, workflow, and server
  entries while preserving session scope and FIFO order.
- Clear-all removes every visible pending entry and returns the exact affected
  count.
- A hidden durable in-flight row survives individual and bulk cancellation.
- Reserve/remove and reserve/clear races have one complete winner; no row is
  both delivered and reported removed.
- Every queue WebSocket action authorizes its session before access. Add and
  append also reject an authorized session paired with a different task.
- Unauthorized calls expose no queue state, mutate nothing, and publish no
  status event.

## TDD Sequence

1. Reverse the existing agent/workflow removal expectations in service and
   handler tests; add server-origin, mixed clear-all, and in-flight cases.
2. Add deterministic SQLite/memory reservation race tests and an env-gated
   Postgres parity case. Run focused packages and record RED.
3. Add handler authorization tests for session-only and task/session-pair
   actions, including non-enumerating failure behavior. Run RED.
4. Change repository contracts and implementations with per-session locks,
   persisted compare guards, and affected-row accounting.
5. Add the task/session pair authorizer, inject it into queue handlers, and
   guard all entry points before queue access.
6. Rerun focused packages GREEN and inspect logs/error strings for obsolete
   origin-ownership wording.

## Verification

```bash
cd apps/backend
go test ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/task/service ./internal/backendapp
KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/orchestrator/messagequeue
```

The Postgres command skips when `KANDEV_TEST_POSTGRES_DSN` is unset.

## Files Likely Touched

- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_*_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/task/service/service_access.go`
- `apps/backend/internal/task/service/service_access_test.go`
- `apps/backend/internal/backendapp/gateway.go`

## Parallelism

Sequential. Repository race semantics, service expectations, and handler
authorization share one RED-GREEN cycle.

## Output Contract

Report RED/GREEN commands and counts, SQLite/Postgres status, authorization
coverage, changed files, and any residual race or compatibility risk. Update
this task and `plan.md` status in the same implementation conversation.

## Results

- RED: `go test ./internal/orchestrator/messagequeue -run 'Test(RemoveEntry|CancelAll)' -count=1`
  failed because agent/workflow/server rows remained.
- RED: `go test ./internal/task/service -run TestAuthorizeTaskSessionAccessRejectsMismatchedPair -count=1`
  failed because a same-owner mismatched task/session pair passed.
- RED: `go test ./internal/orchestrator/handlers -run 'TestQueueHandlersDenyUnauthorized' -count=1`
  failed because every handler reached queue dependencies.
- GREEN: `go test -race ./internal/orchestrator/messagequeue -count=1`.
- GREEN: full `internal/orchestrator/handlers` and `internal/backendapp`; focused
  task-service access tests.
- PostgreSQL parity test is present and skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is unset.
- The full task-service package was attempted; unrelated existing filesystem
  tests fail with `parent directory cannot be accessed` in this executor.
