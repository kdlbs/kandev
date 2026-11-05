---
id: "02-prepare-session-once"
title: "Persist prepared sessions once"
status: completed
wave: 2
depends_on:
  - "01-measure-creation"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RESPONSIVENESS-001
acceptance_criteria:
  - AC-TASKS-CREATE-RESPONSIVENESS-001.3
  - AC-TASKS-CREATE-RESPONSIVENESS-001.4
system_design:
  - ../../specs/tasks/system-design/task-create-responsiveness.md
---

# Task 02: Persist prepared sessions once

## Summary

Persist initial prompt state and primary election in the existing session
transaction. Remove the three subsequent writer acquisitions for new preparation.

## In scope

- Add a typed seed under task models and carry it before session construction.
- Include preview and submission metadata in the original session insert.
- Elect a primary only when missing under the owning task lock.
- Return actual committed primary ownership and invoke its callback once.
- Cover ordinary, runtime-seed, workflow-route, bound, shared-group, and retry paths.
- Preserve reused Office sessions and explicit primary promotion.

## Out of scope

Task aggregate creation, task state transitions, preference writes, UI changes,
provider launch, and shared SQL writer policy.

## Acceptance

1. TDD proves one preparation transaction contains session, initial metadata,
   and primary election. Injected failure and pre-admission cancellation persist
   none of that state. No seed adds post-insert metadata writes.
2. Concurrent real creators preserve one primary and the original-session marker.
   Existing primary ownership, explicit promotion, binding retry, and Office reuse
   preserve their outcomes. Callbacks observe committed ownership exactly once.
3. Initial attachments, plan mode, external-ID retries, fresh recovery, queued
   creation, and deferred passthrough startup retain their current behavior.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'TestPreparedSession(SeedAtomicity|PrimaryElection|ConcurrentPrimaryElection)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository -run 'TestSetSessionPrimary_DemotesOthersAndPromotesExactlyOne' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor -run 'TestPreparedSession|TestPrepareSession' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestPreparedSession|TestFreshStartSubmission|TestPrepareTaskSession' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run 'TestHTTPCreateTask' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
```

Run the first command again with `KANDEV_TEST_POSTGRES_DSN` set to an isolated
test database. Real multi-connection PostgreSQL coverage is required.
An unavailable DSN leaves that result pending.
Use `bash scripts/lint-go-changed` with the exact changed Go paths from the repo root.

## Files likely touched

- `apps/backend/internal/task/models/initial_prompt_preview.go`
- `apps/backend/internal/task/models/initial_prompt_submission.go`
- `apps/backend/internal/task/models/prepared_session_seed.go` (new typed carrier)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_office.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- Focused sibling tests for preparation, rollback, primary concurrency, and dialect parity.

Do not add another persistence owner or special writer pool.

## Dependencies

Task 01 provides the full-path fixture and retained original binary.

## Risks

Session reuse must not overwrite a previous submission.
Election must use current state under the task lock, not an earlier session list.
Workspace retries can change which creator owns primary publication.
No external operation belongs inside the transaction.

## Parallelism

`sequential`

## Inputs

The Prepared session seed and Primary election design sections.
Existing session creation variants, retry ownership tests, and recovery tests.

## Results

Completed. `models.PreparedSessionSeed` carries the preview and validated
submission from the orchestrator context into executor preparation. Ordinary,
initial-runtime-seed, workflow-route, bound, shared-group, and Office creation
paths persist the seed in the original session insert. The old post-insert
`SetSessionMetadataKey` writes were removed.

`createTaskSession` now elects an initial primary inside the same guarded
transaction and records committed ownership on `TaskSession.IsPrimary`.
`SetSessionPrimary` remains the explicit promotion path. The executor publishes
the primary callback only when the committed insert reported ownership.

The new repository, executor, orchestrator, handler, and SQL guard commands in
`plan.md` passed. The PostgreSQL-gated repository tests were not run because
`KANDEV_TEST_POSTGRES_DSN` is unavailable; their shared SQL builders and task-row
lock ordering are unchanged.

Retained behavior evidence includes the existing workspace binding retry,
Office reuse, runner retry, seed rollback, pre-admission cancellation, fresh
recovery, and explicit primary promotion tests.
