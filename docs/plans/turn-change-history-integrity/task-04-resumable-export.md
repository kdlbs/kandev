---
id: "04-resumable-export"
title: "Preserve historical content in resumable bounded batches"
status: done
wave: 4
depends_on:
  - 02-history-comparison
  - 03-writer-attribution
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.4
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.5
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-004.13
  - AC-TASKS-TURN-CHANGES-004.14
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.2
  - AC-TASKS-TURN-CHANGES-007.6
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 04: Preserve historical content in resumable bounded batches

## Summary

Retain acknowledged content and resume from a durable cursor after interruption.
Keep export work separate from writer intervals and bounded cleanup.

## In scope

- Advertise export_batch_version and validate every raw/effective pair and inventory.
- Return bounded page prefixes with explicit continuation and failure reasons.
- Atomically store content links, byte accounting, next cursor, and claim token.
- Add replayable progress/lease migrations and a bounded task-owned processor.
- Enforce 25 entries per page, 20-second attempts, at most three attempts,
  two active installation attempts, and one 32 MiB cumulative turn budget.
- Resume original immutable objects after restart; never recapture or relaunch.
- Publish saving/retrying/complete/unavailable revisions.
- Integrate retention, ref cleanup, cancellation, shutdown, and task deletion.
  Reject late worker responses after cleanup or claim takeover.
- Preserve a capability-absent older agentctl's bounded one-shot behavior.

## Out of scope

Unlimited retries, longer Stop/Cancel waits, user retention controls,
new executor resource creation, and recovery of historical missing blobs without evidence.

## Acceptance

1. Timeout and lost-acknowledgement tests resume from durable progress without
   duplicate links/bytes or changed comparison identity.
2. Retry, page, cumulative-byte, concurrency, and retention bounds hold across
   repositories and claim takeovers; specific failure reasons remain visible.
3. Cleanup preserves saved files, settles the missing remainder, and rejects late
   responses within its existing budget. Backend shutdown adds no ACP cancellation.

## TDD and regression

Add proposed process/turn_checkpoint_export_batch_test.go,
task/changes/content_batches_test.go, and
task/repository/sqlite/turn_change_export_progress_test.go.
RED: TestTurnChangeExportBatchTimeoutResume forces the executor page deadline
after an acknowledged prefix, then asserts that the next attempt skips saved entries.

Require TestTurnChangeExportBatchLostReceipt,
TestTurnChangeExportClaimTakeover,
TestTurnChangeExportCleanupRejectsLatePage,
TestTurnChangeExportBudgetAcrossRepositories,
TestTurnChangeExportRetryExhaustion, and
TestTurnChangeExportLegacyCapability.
Use fake clocks/barriers for deterministic timeout and lease boundaries.
Destroy the fixture executor after successful preservation, then read saved content.
A generic unavailable-content test does not prove the incident's suspected timeout path.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'TestTurnCheckpointExportBatch|TestTurnChangeExport|TestPostgresTurnChangeExport' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/task/changes ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'TestTurnChangeExport.*(Concurrent|Claim|Receipt|Cleanup|Budget)|TestStopPreservesActiveTurnBeforeExecutorCleanup|TestTurnChangeCapturePreservesShutdownDeliveryOutcome' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/persistence/storeconformance -count=1)
```

Run PostgreSQL cases against the configured DSN, including duplicate page receipt,
lease takeover, and shared turn-byte admission.
Run scripts/lint-go-changed for actual changed Go files.

## Files likely touched

Existing: apps/backend/internal/common/turnchanges/contract.go;
apps/backend/internal/agentctl/server/process/turn_checkpoint_export.go;
apps/backend/internal/agentctl/server/api/git_turn_changes.go;
apps/backend/internal/agent/runtime/agentctl/client_turn_changes.go;
apps/backend/internal/task/changes/content.go;
apps/backend/internal/task/changes/coordinator_finish.go;
apps/backend/internal/task/changes/coordinator_cleanup.go;
apps/backend/internal/task/repository/turn_changes.go;
apps/backend/internal/task/repository/sqlite/turn_change_content.go;
apps/backend/internal/task/repository/sqlite/turn_change_retention.go;
apps/backend/internal/task/repository/sqlite/base_migrations.go;
apps/backend/internal/task/repository/sqlite/turn_changes_schema_ddl.go;
apps/backend/internal/task/service/archived_session_reconciliation.go;
apps/backend/internal/backendapp/orchestrator.go;
apps/backend/internal/backendapp/turn_changes_runtime.go;
apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go.

Proposed task/changes/export_processor.go and focused receipt/migration/test files
belong to existing owners. Do not introduce an Office or generic run scheduler.

## Dependencies

02-history-comparison supplies the accepted effective pair.
03-writer-attribution supplies the writer/boundary distinction and guard.

## Risks

The turn budget spans multiple repository jobs.
Content, progress, and lease receipts must commit together.
Retention/deletion/cleanup must invalidate workers before dropping the only source.

## Parallelism

sequential

## Inputs

- History integrity design: Resumable export; Runtime integration; Persistence; Bounds.
- Existing content receipts, retention leases, coordinator cleanup, and shutdown tests.
- Existing task-owned sweep and backend lifecycle construction patterns.

## Results

Implemented `export_batch_version = 1` with deterministic inventory fingerprints, bounded 25-entry pages, cursor continuation, and effective-pair validation. Added durable page receipts with claim tokens, takeover/late-response rejection, typed export reasons, per-page atomic content and cursor writes, cumulative turn byte admission, and a bound-capped processor with two concurrent attempts. Comparison summaries persist before content finishes; saved files remain readable and pending files use the existing loading state. Cleanup releases refs only after content settles.

### Fixup remediation (2026-10-28)

Expired exhausted claims now settle unavailable, and file-specific export failures stay on their entries so later files still save. The export processor has an idempotent stop that joins its in-flight pass. History columns, the dependent index, and the legacy backfill moved into `runMigrations` after their `ADD COLUMN` steps; the upgrade and replay tests exercise that boot path.
