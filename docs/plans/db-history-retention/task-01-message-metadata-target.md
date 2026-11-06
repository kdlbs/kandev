---
id: "01-message-metadata-target"
title: "Broaden the message target to finished or archived at 14 days"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.1
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.2
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.3
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.4
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.5
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.6
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.7
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.3
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.4
system_design:
  - ../../specs/system-page/system-design/db-history-retention.md
---

# Task 01: Broaden the message target to finished or archived at 14 days

## Summary

Broaden the existing periodic message target in `internal/system/dataretention`
from archived-only to finished-or-archived, and move its initial window from 30
to 14 days. No new settings record, HTTP route, or settings card is added; the
existing `archived_data_retention` record and its frame are reused.

## Scope

- Candidate selection and eligibility in
  `internal/task/repository/sqlite/archived_data_retention_scan.go`:
  - Generalize the task cursor and its captured upper cursor from
    `archived_at IS NOT NULL` to the union of archived tasks and finished tasks
    (task state `COMPLETED`, `FAILED`, or `CANCELLED`).
  - Add a `HistoryTaskEligible` helper that returns true when
    `ArchivedTaskEligible(cutoff)` is true, or when the task state is terminal
    and `PayloadTaskEligible(cutoff)` is true, using the same SQLite date
    comparisons the existing checks use.
- `internal/system/dataretention/scan.go`: use the generalized task cursor,
  upper-cursor capture, and eligibility in the message pass, keeping the
  session/message keyset paging, the shared batch budget, and the compare-and-swap
  write through `WriteReducedMessageMetadata`.
- `internal/system/dataretention/policy.go`: change the message window's initial
  value from 30 to 14 days. Keep the persisted JSON field and the settings key,
  so a record already saved keeps its saved value and no migration is required.
- Re-read the task state and cutoff inside the writer transaction, count an
  already-marked row as already removed, and skip oversize rows intact.
- Bound the pass by row count, byte budget, and per-statement budget; fail closed
  on an incomplete eligibility decision and report the pass partial.
- Reuse the existing periodic worker, maintenance admission, and stop/quiesce
  chain; resume from persisted progress on restart without double counts.

## Exclusions

- No new settings record, HTTP route, revision stream, or settings card.
- No message deletion, content change, identity change, or non-payload metadata
  change.
- No change to the tool payload record, window, or approved state.
- No automatic session reduction, catalog pruning, or plan-revision removal.
- No `VACUUM` and no file compaction in the periodic path.
- No new table, index, or schema migration.

## Acceptance conditions

1. An in-window finished task and an in-window archived task are reduced with the
   existing removal marker; a live, pending, nonterminal, or out-of-window task
   is retained, and a second pass is a no-op.
2. A concurrent metadata write wins (compare-and-swap), and a task that becomes
   active before commit is skipped.
3. The pass shares the batch budget, respects the maintenance admission and the
   approved-revision gate, fails closed on an incomplete eligibility decision,
   and resumes after restart without double counts.

## Likely files

- `apps/backend/internal/system/dataretention/{policy.go,scan.go,service.go}`
- `apps/backend/internal/task/repository/sqlite/archived_data_retention_scan.go`
- `apps/backend/internal/task/repository/sqlite/tool_payload_retention_scan.go`
  (reused `PayloadTaskEligible`, unchanged)
- `apps/backend/internal/task/models/tool_payload_retention.go` (reused,
  unchanged)
- New `*_test.go` files next to the changed sources

## Verification

```bash
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... ./internal/task/models/... -count=1
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... -race -count=1
cd apps/backend && go build ./... && go vet ./...
cd apps/backend && gofmt -l internal/system/dataretention internal/task/repository/sqlite internal/task/models
```

## Results

Implemented. `HistoryTaskEligible`/`FinishedTaskEligible`/`terminalTaskState` and
`NextHistoryTaskID`/`HistoryUpperTaskID` added in
`internal/task/repository/sqlite/archived_data_retention_scan.go`; the message
pass in `internal/system/dataretention/scan.go` uses them; the message window
initial value is 14 days in `policy.go`. Tests:
`apps/backend/internal/task/repository/sqlite/archived_data_retention_scan_test.go`
(eligibility and cursor) and
`apps/backend/internal/system/dataretention/scan_test.go` (broadened pass reduces
finished and archived messages, leaves a live task untouched). `go test` for both
packages and `gofmt`/`go vet` pass.
