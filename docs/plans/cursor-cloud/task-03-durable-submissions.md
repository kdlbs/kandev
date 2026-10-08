---
id: "03-durable-submissions"
title: "Persist cloud bindings and submission operations"
status: complete
wave: 3
depends_on:
  - "02-profiles-and-capabilities"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-002.1
  - AC-EXECUTORS-CURSOR-CLOUD-002.2
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-003.2
  - AC-EXECUTORS-CURSOR-CLOUD-003.5
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 03: Persist cloud bindings and submission operations

## Summary

Concurrent reservations create one binding and one active operation; stale revisions cannot change ownership or dispatch another prompt.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Add binding, operation, stream checkpoint, event-receipt, and tool-grant tables through additive migrations in the shared repository layer.
- Implement unique session bindings, prompt-turn operation identity, immutable launch snapshots, revision checks, and expiring dispatch leases.
- Persist initial create IDs before network calls. Represent unknown submissions explicitly and prevent automatic redispatch after lease expiry.
- Provide transactional message/checkpoint hooks for later event integration, including deduplication by run, event ID, and event type.
- Cover migration replay and both supported database dialects using existing repository test conventions.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Concurrent reservations create one binding and one active operation; stale revisions cannot change ownership or dispatch another prompt.
- Reopening the repository retains create IDs, unknown outcomes, and checkpoints without plaintext credentials.
- Migration and transaction rollback tests preserve existing sessions and make replay harmless.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestManaged|TestSQLiteSchemaReinitializes' -count=1)
```

PostgreSQL verification requires a disposable test database in `KANDEV_TEST_POSTGRES_DSN`.
The guard below prevents a skipped suite from appearing successful. Use the existing PostgreSQL test fixture conventions.

```bash
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run 'TestPostgresManaged' -count=1)
```

### Evidence mapping

- 002.1, 002.2, 002.5: `internal/task/repository/sqlite/managed_agent_operations_test.go: TestManagedOperationSingleWriter, TestManagedOperationUnknown`.
- 003.2, 003.5: `internal/task/repository/sqlite/managed_agent_recovery_test.go: TestManagedBindingReopen, TestManagedCheckpointTransaction`.

- 002.2, 003.2: `internal/task/repository/sqlite/managed_agent_postgres_test.go: TestPostgresManagedSingleWriter, TestPostgresManagedRecovery`.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`.
- `apps/backend/internal/task/repository/sqlite/ (new managed_agent migration and repository files)`.
- `apps/backend/internal/task/models/ (managed binding and operation types)`.

## Dependencies

02-profiles-and-capabilities

## Risks

Provider success and local commit failure are separate outcomes. Tests must place crash boundaries before and after external submission.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Implemented additive managed-agent binding, operation, stream checkpoint/event receipt, and tool-grant persistence with CAS revisions, expiring dispatch leases, replay-safe reservations, and atomic message/checkpoint commits. Request snapshots exclude credentials; grants persist hashes only.

Passed: `go test ./internal/task/repository/sqlite -run 'TestManaged|TestSQLiteSchemaReinitializes' -count=1`, PostgreSQL `TestPostgresManaged` tests against a disposable local database, and `make lint`.
