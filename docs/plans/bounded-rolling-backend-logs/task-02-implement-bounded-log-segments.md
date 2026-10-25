---
id: "02-implement-bounded-log-segments"
title: "Implement bounded log segments"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 02: Implement Bounded Log Segments

## Intent

Replace the stop-at-256-MiB writer with bounded segments that preserve the
newest backend evidence within one 256 MiB total budget.

## Acceptance

- The active file rotates before it exceeds 16 MiB.
- Writes continue after retained output exceeds 256 MiB. All recognized backend
  logs remain within the total budget, and later entries replace the oldest
  complete segments across days.
- UTC rollover, restart, cleanup, legacy input, and interrupted operations
  preserve the specified data and filename contracts.

## Files Likely Touched

- `apps/backend/internal/common/logger/daily_writer.go`
- `apps/backend/internal/common/logger/daily_writer_test.go`
- `apps/backend/internal/common/logger/backend_log_segments.go`
- `apps/backend/internal/common/logger/backend_log_segments_test.go`
- `apps/backend/internal/common/logger/retry_daily_writer.go`
- `apps/backend/internal/common/logger/retry_daily_writer_test.go`

The companion segment files are new if they keep the existing writer within
the backend complexity limits.

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01. This task owns only logger-package files. Task 01
owns the gateway files. Task 03 depends on this task's filename and ordering
contract.

## Inputs

- The storage requirements and scenarios in the diagnostic-logging
  specification.
- The accepted bounded-log ADR.
- The existing daily writer, retrying writer, rollover journal, and tests.

## Implementation

1. Parameterize limits in test fixtures and add failing cases for rotation,
   eviction, later-entry retention, restart, UTC rollover, cleanup, legacy
   conversion, and operation failure.
2. Add strict parsing and sorting for active, numbered, and legacy files.
3. Rotate the active file to an exclusive six-digit sequence name before its
   next entry exceeds the segment limit.
4. Enforce the total budget by removing the oldest complete segments across
   retained days before newer writes continue.
5. Reconstruct day, byte totals, and sequence state during startup without
   following symlinks or replacing existing files.
6. Convert an oversized old-format active file by keeping its bounded tail and
   recording recoverable progress.
7. Adapt the existing journal or add the smallest explicit transaction state
   needed for crash-safe size, day, and conversion operations.
8. Remove the permanent daily-limit error branch from the retrying writer.
   Route filesystem failures through the existing 30-second retry and loss
   reporting behavior.
9. Run focused tests, then refactor helpers to satisfy Go complexity limits.

## Verification

```bash
cd apps/backend && go test ./internal/common/logger -run 'TestDailyWriter|TestBackendLogSegment|TestRetryDailyWriter' -count=1
```

## Output Contract

Report the migration algorithm, crash-recovery invariant, changed files, exact
test result, blockers, and remaining risks. Update this task and `plan.md` in
the same conversation.

## Results

- Changed `daily_writer.go` and `backend_log_segments.go` to use 16 MiB active
  segments, a 256 MiB global budget, oldest-closed-segment eviction, UTC-day
  rollover, strict filename parsing, legacy-file retention, and bounded
  oversized-active-file conversion.
- Conversion renames the source to an owner-only backup and processes retained
  outputs from newest to oldest. Each output is copied through a temporary
  file, synced, and atomically renamed. The backup is then truncated and the
  journal records the next output. This keeps migration I/O linear and lets
  recovery resume after a stale temporary file, a truncated source, or an
  interrupted journal write. Recovery checks completed outputs before it
  adopts a fresh active file. Atomic rename is used for normal size and day
  rotation, so completed segments are never replaced.
- Retention accepts only dates from two UTC days before today through today.
  Future-dated segments are excluded from cleanup and bundle selection.
- Changed `backend_logger.go` so filesystem and rotation errors use the normal
  30-second retry path instead of a permanent daily-limit stop.
- Added writer, migration, restart, UTC rollover, global-eviction, legacy,
  and interrupted-conversion regression coverage.
- Verification: `cd apps/backend && go test ./internal/common/logger -run
  'TestDailyWriter|TestBackendLogSegment|TestRetryDailyWriter' -count=1` passed.
- Blockers: none.
- Remaining risk: filesystem failures on platforms with unusual rename or
  permission semantics need CI coverage in addition to local Unix coverage.
