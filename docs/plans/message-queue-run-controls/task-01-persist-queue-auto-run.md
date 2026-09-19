---
id: "01-persist-queue-auto-run"
title: "Persist queue Auto-run policy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-run.md"
---

# Task 01: Persist queue Auto-run policy

## Intent

Create the queue-owned, durable per-session policy and atomic repository
operations that later orchestration can trust. Do not add WebSocket or UI
surface in this task.

## Acceptance

1. Memory and SQL repositories read absent state as ON, persist explicit ON or
   OFF independently of queue contents, and project it through `QueueStatus`.
   Reconstructing the SQL repository preserves the value.
2. An automatic-reserve operation reads Auto-run and reserves the FIFO head in
   one per-session transaction. It distinguishes OFF from an enabled empty
   queue and preserves durable lifecycle reservation behavior.
3. `ClaimSendNow` sets ON atomically with an accepted exact claim. Rejected
   claims preserve the previous value, and restoring an accepted claim restores
   rows without reverting ON.
4. `TransferSession` applies pause-wins to source and destination, removes the
   source policy, and transfers entries/pending move unchanged. Queue
   snapshot/replace does not overwrite policy.
5. A real Postgres locking test proves OFF and automatic reserve cannot commit
   out of order around `queue_session_locks`.

## Verification

```bash
cd apps/backend
go test ./internal/orchestrator/messagequeue -run 'Test.*(AutoRun|SendNow|TransferSession|ReplaceSession)' -count=1
go test -race ./internal/orchestrator/messagequeue -run 'Test.*AutoRun' -count=1
go test ./internal/orchestrator/messagequeue -count=1
```

The Postgres-specific case may use the package's existing environment-gated
fixture. Record whether it ran or skipped and why; a skip is not evidence of
the cross-process assertion.

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_auto_run_test.go` (new)
- `apps/backend/internal/orchestrator/messagequeue/repository_send_now_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_auto_run_test.go` (new)

## Dependencies

None.

## Parallelism

Sequential. This task owns the repository interface and status shape consumed
by all later tasks.

## Inputs

- ADR: queue-owned state, missing means ON, queue-lock serialization,
  pause-wins transfer, and Send Now activation.
- Spec: `Data Model`, `State and Concurrency`, and persistence scenarios.
- Existing session-lock and Postgres race patterns in
  `repository_sqlite.go` and `repository_postgres_merge_test.go`.

## Risks

- Do not implement check-then-reserve as two transactions.
- Keep product policy out of the synchronization-only lock row.
- Do not make message rollback overwrite a policy choice made by another
  operation.
- Preserve durable lifecycle reserve/acknowledge semantics and existing Send
  Now restoration guarantees.

## Output contract

Report the repository contract, schema behavior, files changed, exact commands
and test counts, Postgres run/skip evidence, blockers, and residual risks. Set
this task to `done`, record results below, and synchronize `plan.md` before Task
02.

## Results

Implemented queue-owned Auto-run state in memory and SQL repositories. Missing
state reads ON; explicit values survive empty queues and SQLite repository
reconstruction. Policy-aware reservation checks state under the existing
per-session lock, accepted Send Now claims atomically persist ON, transfer uses
pause-wins and removes source state, and queue replacement leaves policy alone.

Changed repository types, memory/SQL implementations, status projection, and
new focused SQLite/Postgres contract tests.

Verification:

- `go test ./internal/orchestrator/messagequeue -run 'Test.*(AutoRun|SendNow|TransferSession|ReplaceSession)' -count=1` passed.
- `go test -race ./internal/orchestrator/messagequeue -run 'Test.*AutoRun' -count=1` passed.
- `go test ./internal/orchestrator/messagequeue -count=1` passed.
- Real Postgres lock test compiled but skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset. Cross-process behavior remains CI-gated
  until a Postgres DSN is available.
