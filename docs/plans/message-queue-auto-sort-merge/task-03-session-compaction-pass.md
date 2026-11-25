---
id: "03-session-compaction-pass"
title: "Session compaction pass"
status: done
wave: 3
depends_on:
  - "02-sorted-admission-folds"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.11
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.12
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 03: Session compaction pass

## Summary

When a session policy write turns effective Auto-sort-merge from OFF to ON,
compact the pending queue in the same transaction as the write. Except that
edit-leased rows are never folded into another row or receive a fold, the
result must equal in-order admission with the policy ON. Both the
Auto-sort-merge and Auto-merge session writes use this path.

## In scope

- `compactSessionQueueTx` for SQL and memory, reusing `sortedFoldTarget` and
  `buildAutoMergedEntry`, with CAS target writes and source deletes.
- The `SetSessionPolicyAndCompact(ctx, identity, change, global)` repository
  operation (lock, validate, read both overrides, write, compact on an
  effective OFF-to-ON change, commit). The service loads `GlobalQueuePolicy`
  before opening the transaction and passes it in; the persisted settings
  loader is never called inside the transaction (SQLite's writer pool has one
  connection, so a nested read would wait on itself).
- `SessionPolicyResult.CompactedCount` populated by both session writes and
  returned as `compacted_count`. Handlers publish one final status.
- A per-segment latest-by-sender index over the full persisted row stream;
  only fold barriers (including hidden reserved rows) reset the index.
  Edit-leased rows are kept untouched, are never a fold source or target, but
  stay in the index and become `prev`, matching admission.

## Out of scope

- Compaction on global changes, splitting rows on OFF, and UI.

## Acceptance

- For the queue A1, B1, A2, B2, U1, U2, A3, turning the session switch ON
  (Auto-merge ON) yields A1+A2+A3, B1+B2, U1+U2 when all entries are
  compatible. Reserved and edit-leased rows are untouched, and Auto-run is
  unchanged. A1, leased U1, A2 compacts to A1+A2, U1, identical to admission.
- `TestCompactionPreservesLeasedSourceAgainstCompatiblePredecessor`: U1,
  leased U2 (compatible) stays U1, U2 with both IDs and contents unchanged and
  the lease still valid, on memory, SQLite, and PostgreSQL.
- Both session switches commit and compact through the real settings loader
  and queue repository sharing one single-connection SQLite writer pool.
- Turning session Auto-merge ON while the sort preference is ON compacts.
  ON-to-ON writes, OFF writes, and global saves do not.
- An injected write failure rolls back both the policy write and every fold.

## Verification

```bash
test -n "$KANDEV_TEST_POSTGRES_DSN" || { echo "KANDEV_TEST_POSTGRES_DSN unset: PostgreSQL cases will skip; record as unverified"; }
(cd apps/backend && go test ./internal/orchestrator/messagequeue/... -run 'Compaction|AutoSortMerge|SetSessionAutoMerge')
(cd apps/backend && go test ./internal/orchestrator/handlers/... -run 'AutoSortMerge|AutoMerge')
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/{auto_sort_merge,repository,repository_sqlite,repository_memory,service}.go`
- New `messagequeue/repository_compaction_test.go` and `service_auto_sort_merge_compaction_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_operations_handlers.go` and tests

## Dependencies

Task 02.

## Risks

- The in-process `Service.editLeases` must be passed into the pass, as reorder
  does. Otherwise an entry being edited can be folded.
- A large queue makes one long transaction. Target lookup is O(1) per row, but
  N is the persisted backlog, which capacity does not bound (unlimited or
  lowered caps). Test an over-cap backlog; add no new cap.

## Parallelism

`sequential`

## Inputs

- System design: Compaction pass, Policy mutations.
- Patterns: `reorderEntries` (lease checks and CAS), `applyMergeWrites`, and
  `TestService_SetSessionAutoMergeWaitsForAdmissionGate`.

## Results

- RED: `go test ./internal/orchestrator/messagequeue/... -run TestAutoSortMergeSessionSwitchCompaction -count=1`:
  failed before compaction (expected four folded rows, got zero).
- `go test ./internal/orchestrator/messagequeue/... -run 'Compaction|AutoSortMerge|SetSessionAutoMerge'`: passed.
- `go test ./internal/orchestrator/handlers/... -run 'AutoSortMerge|AutoMerge'`: passed.
- `go test ./internal/backendapp/... -run 'AutoSortMerge'`: passed with both
  session switches compacting through the real single-connection settings
  loader.
- `go test -trimpath -race ./internal/orchestrator/messagequeue/... -run 'Compaction|AutoSortMerge|SetSessionAutoMerge'`: passed.
- Final targeted rerun after logging changes included `|SortedFold`: passed.
- Lease-source positive/negative controls, cross-lease folds, hidden barriers,
  effective transitions, over-cap backlog, Auto-run preservation, and SQL
  rollback passed. PostgreSQL variants skipped because the DSN is unset.
- `go run ./cmd/sqlguard ./internal`: passed.
- The mandatory store-conformance check exposed an external admission
  interface still using `AutoMergePolicy`; migrated it to `AdmissionPolicy`.
  Its rerun is recorded in the plan's final verification results.
