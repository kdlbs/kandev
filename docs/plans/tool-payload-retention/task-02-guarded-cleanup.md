---
id: "02-guarded-cleanup"
title: "Add backup preparation and guarded payload cleanup"
status: completed
wave: 2
depends_on:
  - "01-policy-and-analysis"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.1
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.6
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.4
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 02: Guarded Cleanup

## Summary

Implement explicit backup preparation, guarded payload changes, durable progress,
and replay protection. This is the backend mutation boundary.

## In scope

- Add the enablement/preparation state machine and backup/skip receipts. Reuse
  snapshot creation and add read-only integrity verification of the new snapshot.
  Thread cancellation through the snapshot helper. Failure keeps deletion blocked.
- Implement initial and manual cleanup using Task 01's reducer. Validate policy,
  approval, message revision, activity, and pending work inside each writer transaction.
- Commit counters and cursor with the batch. Preserve conversation timestamps.
  Handle disable, cancel, conflicts, disk errors, and interrupted operations.
- Preserve markers during all repository metadata replacements and delayed tool
  updates. Inspect direct SQL writers and recovery paths, not just the service.
- Return a typed removed-output response from lazy shell reads. Publish committed
  marker changes through existing message events. Preserve exports and identities.
- Coordinate snapshot, restore, and compaction work to avoid overlapping maintenance
  with payload batches. A conflicting operation defers cleanup without spinning.

## Out of scope

Rendered UI, daily timer wiring, compression, and automatic compaction.

## Acceptance

1. No cleanup occurs without explicit preparation. Backup failure, invalid
   receipts, disable, and stale preparation completions block later mutation.
2. Guarded batches preserve metadata, identity, and timestamps. Concurrent task
   admission or policy changes protect the next batch, with committed-only counts.
3. Delayed replacement writes and replay cannot restore removed fields. Repeated
   cleanup is a no-op, and restart neither loses progress nor counts it twice.

## Tests and verification

Use barriers to interleave cleanup with session admission, message updates,
policy disable, and backup completion. Test both orderings of each race.
Inject crashes before/after approval and before/after batch commit. Exercise
snapshot verification failure, missing backup, disk-full error, and busy timeout.
Use a temporary SQLite file and separate reader/writer connections.

Run independently from the repository root:

```bash
(cd apps/backend && go test ./internal/system/toolretention/... ./internal/system/backups/... ./internal/system/database/... ./internal/task/models/... ./internal/task/repository/sqlite/... ./internal/task/service/...)
(cd apps/backend && go test -race ./internal/system/toolretention/... ./internal/task/repository/sqlite/...)
(cd apps/backend && go test ./internal/persistence/... ./internal/task/handlers/...)
git diff --check
```

## Files likely touched

- `apps/backend/internal/system/toolretention/` preparation, runner, runtime store,
  handlers, cancellation, and tests.
- `apps/backend/internal/system/backups/store.go`, system database maintenance,
  `apps/backend/internal/persistence/snapshot.go`, system composition, and their
  focused tests.
- `apps/backend/internal/task/repository/sqlite/message.go`, direct message update
  paths, `message_payload_replay.go`, and tests.
- `apps/backend/internal/task/service/service_messages.go`,
  `apps/backend/internal/task/handlers/message_handlers.go`,
  model projections, message events, and tests.

## Dependencies and parallelism

Depends on Task 01. Integration follows its reducer contract. Repository mutation and replay handling share
one invariant and must not land as separate behavior changes.

## Inputs

Read the design's preparation, transaction safety, and removal rules. Reuse the
reducer without creating a separate SQL-only deletion path. Use `/tdd`.

## Risks

The in-memory job tracker cannot authorize cleanup after restart. Preparation
must rely on durable records. A service-only marker check leaves a stale-write race.

## Results

Implemented private snapshot staging, read-only snapshot integrity checks,
SHA-256 receipts, a shared maintenance lease, transactional metadata reduction,
and replay protection at repository and service boundaries.

- Preparation tests prove backup failure blocks deletion, missing/changed/corrupt
  receipts fail verification, interrupted preparation requires retry, and a late
  completion cannot override a disabled policy.
- Batches preserve message IDs, titles, tool IDs, timestamps, and exact committed
  counts. A 250-message fixture resumes across batches and restart without
  double counting. Empty batches do not rehash the selected backup.
- Replacement and replay tests preserve removal markers. Shell reads return 410;
  transcript updates evict cached output even when timestamps are unchanged.
- Compaction during analysis invalidates its rowid cursor with a partial result.
- Focused race tests passed for backups, database maintenance, persistence,
  repository replay, and toolretention. Store-conformance checks passed.
- Accepted manual backup/restore/reset/vacuum/optimize jobs detach from HTTP
  request cancellation; retention preparation retains explicit cancellation.

Cancellation regressions reproduce writer-held snapshots, pending preparation,
and operation-ID handoff races before the fix. Validation now precedes
interruption; the writer transaction repeats revision and identity checks.
Stale or invalid requests cannot interrupt newer work. Failed persistence keeps
cleanup disabled and cancellation retryable. The final package race run passed
in 4.477 seconds; scoped lint reports zero issues. Admission and compaction
fixture evidence is recorded in the plan.

Review remediation adds a real guarded SQLite update with retained shell exit
code, API projection, and live-event metadata projection. The exit code remains
visible after replay. Removed stdout/result fields stay absent, and unknown
large numbers retain exact precision. JSON-number conversion rejects fractional,
invalid, and overflowing exit codes. Focused race tests pass.
