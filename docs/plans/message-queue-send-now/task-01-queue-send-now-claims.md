---
id: "01-queue-send-now-claims"
title: "Add exact send-now queue claims"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 01: Add exact send-now queue claims

## Intent

Give the orchestrator an atomic, loss-aware way to claim one persisted queue
entry or an exact ordered snapshot, restore the original entries on failure,
and acknowledge every durable source after acceptance.

## Acceptance

1. An exact-ID claim is all-or-none, session-scoped, FIFO-ordered, and treats
   ordinary versus durable lifecycle rows according to the existing delivery
   guarantees.
2. Bulk aggregation preserves content, attachments, references, execution
   envelope, and source provenance exactly as specified, with pre-mutation
   overflow errors.
3. Restore and acknowledge operations handle mixed ordinary/durable claims
   without changing unrelated or newly queued rows.

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/send_now.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory_send_now_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_send_now_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_send_now_test.go`
- `apps/backend/internal/orchestrator/messagequeue/send_now_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task freezes the claim and restoration contract consumed by
Task 02.

## Inputs

- Spec: API-adjacent aggregation rules, State and Concurrency, Failure Modes.
- Plan: Backend / Exact queue dispatch claims.
- Existing patterns: `ReserveHead`, `TakeByID`, `Restore`,
  `AcknowledgeByID`, and `MergeIntoAbove`.

## Verification

```bash
cd apps/backend && go test -race ./internal/orchestrator/messagequeue -run 'SendNow' -count=1
```

If the PostgreSQL DSN is available, the same command must discover and pass the
env-gated PostgreSQL send-now cases; otherwise record the explicit skip.

## Output contract

Report files changed, the exact claim/restore/ack semantics, RED and GREEN test
commands/results, PostgreSQL execution or skip, blockers/risks, and synchronize
this task plus `plan.md` status/results.

## Results

- Added `SendNowClaim`, FIFO envelope aggregation, exact session-scoped claims, restoration, and durable-source acknowledgement to the memory and SQLite/PostgreSQL-compatible repositories.
- Claims are all-or-none, reject missing/duplicate/reserved or changed click-time snapshots without mutation, retain durable rows with an in-flight reservation, restore ordinary rows at their persisted positions, and acknowledge every durable source only after prompt acceptance.
- GREEN: `cd apps/backend && go test -race ./internal/orchestrator/messagequeue -count=1 -v` — 116 tests passed across memory and SQLite; env-gated PostgreSQL cases are included and skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- Aggregate envelope coverage proves FIFO content/attachments, oldest execution metadata, canonical reference de-duplication, provenance, attachment-only entries, and pre-mutation attachment/reference overflow errors.
