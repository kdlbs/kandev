---
id: "02-drain-restored-send-now"
title: "Drain restored Send Now entries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.12
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 02: Drain restored Send Now entries

## Summary

A restored Send Now claim triggers one FIFO drain, so a failed replacement prompt no longer leaves the entry waiting for an unrelated event.

## In scope

- Mark a pending drain after a successful restore in `executeSendNowClaimWithContext`.
- Cover a `superseded` failure with readiness published before the restore.

## Out of scope

- The cause of the `superseded` failure and the 30-second readiness wait.
- Restore paths for a replaced session, which the drain gates reject.

## Acceptance

- The restored entry runs once through FIFO, with Auto-run ON.
- A persistent FIFO failure does not start another Send Now or loop.
- A failed restore does not mark a drain, and a claim restored by startup recovery stays pending.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'SendNow' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_restore_drain_test.go`

## Dependencies

None

## Risks

- The drain can race a user sending the same entry. Existing claims and guards decide the owner, and the test asserts one delivery.

## Parallelism

`sequential`

## Inputs

- [Restored claims](../../specs/ui/system-design/message-queue-send-now.md#restored-claims)
- `queue_send_now_test.go`

## Results

Done.

- `executeSendNowClaimWithContext` marks a pending drain after a successful
  restore, and only after success. The existing deferred
  `drainQueuedDispatchIfPending` delivers through ordinary FIFO once the
  dispatch marker clears.
- `queue_send_now_restore_drain_test.go` covers one FIFO delivery, a persistent
  FIFO failure that does not loop, and a failed restore that does not mark a
  drain.
- `TestExecuteSendNowClaimRestorePreservesRecordedSources` now pauses Auto-run
  before the worker so its marker assertions still describe a paused queue; the
  claim itself resumes Auto-run first, which the test still pins.
