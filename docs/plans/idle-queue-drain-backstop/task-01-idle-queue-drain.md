---
id: "01-idle-queue-drain"
title: "Drain queued messages left on idle sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-IDLE-QUEUE-DRAIN-001
acceptance_criteria:
  - AC-TASKS-IDLE-QUEUE-DRAIN-001.1
  - AC-TASKS-IDLE-QUEUE-DRAIN-001.2
  - AC-TASKS-IDLE-QUEUE-DRAIN-001.3
  - AC-TASKS-IDLE-QUEUE-DRAIN-001.4
system_design:
  - ../../specs/tasks/system-design/idle-queue-drain.md
---

# Task 01: Drain queued messages left on idle sessions

## Summary

Offer each session that still holds queued messages to the existing Auto-run
drain from the ceiling sweep, with a per-session backoff, so a message whose
dispatch was returned to the queue reaches an idle session without a new event.

## In scope

- `messagequeue.Service.ListSessionsWithPending` with memory and SQLite
  implementations; the SQLite query joins live sessions and skips in-flight rows.
- `orchestrator.Service.drainIdleQueues`, called at the end of
  `ceilingSweepTick`, running each due session through the body of
  `DrainQueuedMessageIfAutoRun` under a `TryLock` of its cancel-in-flight guard;
  a busy session is skipped for that pass.
- `idleQueueDrainBackoff`: immediate first attempt, then 1 to 15 minutes,
  reset when the session leaves the listing with no queued dispatch in flight.

## Out of scope

Dead-letter storage, notifications, settings, Auto-run changes, and moving
queued messages to another session.

## Verification

- `go test ./internal/orchestrator/ -run TestIdleQueueDrain`
- `go test ./internal/orchestrator/messagequeue/ -run TestListSessionsWithPending`
