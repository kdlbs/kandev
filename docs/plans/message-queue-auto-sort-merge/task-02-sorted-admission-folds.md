---
id: "02-sorted-admission-folds"
title: "Sorted admission folds"
status: done
wave: 2
depends_on:
  - "01-persist-auto-sort-merge-policy"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.8
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.9
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 02: Sorted admission folds

## Summary

When effective Auto-sort-merge is ON, a newly admitted agent message with a
sender task folds into its latest same-sender entry, only if that entry is
compatible, crossing user and other-agent rows but no barrier, including at
full capacity. Otherwise the existing adjacent Auto-merge behaviour applies
unchanged.

## In scope

- `sortedFoldTarget` and the fold-specific `isFoldBarrier` predicate (not
  `IsReservedQueuedBy`, which includes `agent`) in a new
  `messagequeue/auto_sort_merge.go`, evaluated over the full persisted row
  stream including hidden reserved rows.
- A combined `AdmissionPolicy` snapshot and in-transaction validation of both
  policies, with retry on a policy change.
- Target selection in the two-transaction path, the client-queue-id
  single-transaction path, and both full-queue candidate paths, for SQL and
  memory. The edit-lease check applies to the chosen target in both the
  repository and the service.
- A failed sort-override read skips sorted folds but keeps adjacent
  Auto-merge.

## Out of scope

- The compaction pass, UI, and changes to `buildAutoMergedEntry`.

## Acceptance

- For A1, B1, A2 with the policy ON, a compatible B2 yields A1, B1+B2, A2.
  B1 keeps its ID and position, and its queued time becomes B2's.
- A B2 that is incompatible, edit-blocked, behind a barrier, user-sent, or
  sent with the policy OFF stays at the tail. With Auto-merge OFF, or with a
  failed sort read, the result equals today's behaviour.
- With a full queue, a compatible B2 without staged attachments folds into
  B1. Otherwise a compatible adjacent pair (for example a user row) still
  folds as today. Only when neither fold succeeds, effective Auto-merge is
  OFF, or a staged claim is needed does it return `queue_full` with no row
  change.
- Latest-only: A1 (model x), A2 (model y), B1, then A3 (model x) stays at the
  tail. A hidden reserved row between A1 and A2 blocks the fold.

## Verification

```bash
test -n "$KANDEV_TEST_POSTGRES_DSN" || { echo "KANDEV_TEST_POSTGRES_DSN unset: PostgreSQL cases will skip; record as unverified"; }
(cd apps/backend && go test ./internal/orchestrator/messagequeue/... -run 'SortedFold|AutoSortMerge|AutoMerge')
```

## Files likely touched

- New `apps/backend/internal/orchestrator/messagequeue/auto_sort_merge.go` and `auto_sort_merge_test.go`
- `messagequeue/{service,repository_sqlite,repository_admission,repository_memory}.go`
- New `messagequeue/service_auto_sort_merge_test.go` and `repository_auto_sort_merge_test.go` (SQLite and PostgreSQL variants, following `repository_auto_merge_test.go`)

## Dependencies

Task 01.

## Risks

- The full-queue CAS currently targets the tail. It must be applied to the
  sorted target's own position and payload.
- The existing tests that pin "immediate predecessor only" must keep running
  with the policy OFF, not be rewritten.

## Parallelism

`sequential`

## Inputs

- System design: Target selection, Admission flow.
- Patterns: `finalizeAutoMerge`, `autoMergeIntoAbove`,
  `mergeInsertedAdmissionCandidateTx`, `autoMergeCandidateIntoAbove`, and
  `service_auto_merge_test.go`.

## Results

- RED: `go test ./internal/orchestrator/messagequeue/... -run TestAutoSortMergeInterleavedAdmissions -count=1`
  failed on all eight memory/SQLite admission variants before sorted folds.
- RED: `go test ./internal/orchestrator/messagequeue/... -run TestAutoSortMergeLegacyAdmissionUsesChosenTarget -count=1`
  exposed the legacy SQLite fallback losing the combined policy.
- GREEN: both regressions pass, including full-capacity and receipt paths.
- `go test ./internal/orchestrator/messagequeue/... -run 'SortedFold|AutoSortMerge|AutoMerge'`: passed.
- `go test -trimpath -race ./internal/orchestrator/messagequeue/... -run 'SortedFold|AutoSortMerge|AutoMerge'`: passed.
- Coverage includes latest-only selection, every barrier class, hidden rows,
  local and durable target leases, OFF policies, failed sort reads retaining
  adjacent folds, override-race retries, FIFO position, and queued time.
- `KANDEV_TEST_POSTGRES_DSN` remains unset. PostgreSQL cases skip and are
  unverified; SQLite and memory variants passed.
