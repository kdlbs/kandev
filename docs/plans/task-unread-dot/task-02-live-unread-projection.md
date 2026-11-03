---
id: "02-live-unread-projection"
title: "Project live unread updates"
status: done
wave: 2
depends_on: ["01-unread-read-model"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.7
  - AC-TASKS-TASK-UNREAD-DOT-001.8
  - AC-TASKS-TASK-UNREAD-DOT-001.11
  - AC-TASKS-TASK-UNREAD-DOT-001.12
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 02: Project live unread updates

## Summary

The projector follows message, read-cursor, message-deletion, and
session-removal events, so the aggregate changes live and converges regardless
of event order. The e2e cursor route reaches a warm projector.

## In scope

- `TaskSessionReadCursorChanged`, published by `MarkSessionRead` after every
  successful write with the persisted cursor and its `created_at`.
- Projector handling and subscriptions for `MessageAdded`, the cursor event
  (raise, or replace and reload on `reset: true`), `MessageDeleted` of the
  tracked newest message, and `SessionRemoved`.
- When unread observation is false, qualifying `MessageAdded`, cursor events,
  deletion of the tracked newest message, and `SessionRemoved` trigger
  task-wide recovery reads; only success replaces all session ranks and marks
  state observed.

## Out of scope

- Restore and read model (task 01), list-load repair (task 03), frontend.
- Changes to the cursor write guard or the `mark-read` response.

## Acceptance

- Delivering the cursor event before or after its message's `MessageAdded`
  gives the same read state; a newer message re-arms the aggregate.
- `reset: true` with a lower cursor reloads the session's ranks and re-arms the
  aggregate on a warm projector, and matches a database reload; a production
  write never sets `reset`.
- A failed unread reload skips only the unread change; a `MessageDeleted`
  pending-request clear in the same event still applies. If task-wide
  observation failed, the next qualifying background `MessageAdded` and any
  cursor event each trigger an all-session reload before marking state observed.
  Deterministic two-session tests prove that successful recovery includes an
  unread sibling; failed recovery preserves the prior aggregate baseline.
- After `SessionRemoved`, a successful task-wide reload excludes the deleted
  session. If that reload fails, the prior aggregate remains; a later successful
  reload removes the dot when the deleted session was its only unread
  contributor. A deterministic one-session test covers failed reload and
  successful recovery.

## Verification

```bash
(cd apps/backend && go test ./internal/task/statussummary/... ./internal/task/service/... ./internal/task/repository/sqlite/... ./internal/backendapp/...)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/events/types.go`
- `apps/backend/internal/task/service/service_sessions.go` and its test
- `apps/backend/internal/task/statussummary/{projector,projector_events,metrics_vars}.go`
- `apps/backend/internal/task/statussummary/projector_unread_test.go`
- `apps/backend/internal/task/repository/sqlite/task_status_summary_unread_postgres_test.go`
- `apps/backend/internal/backendapp/e2e_reset.go` (pass the existing `eventBus` to the route; read the session and message for `task_id` and `created_at`)

## Dependencies

`01-unread-read-model` provides rank state and the loader.

## Risks

- Streaming messages for a viewed task double summary writes; record the flip
  counter and CAS retries in Results.
- A CAS replay must stay idempotent: `MessageAdded` and the cursor event only
  raise ranks (except the explicit `reset`, which reloads ranks).
- `statussummary` tests cannot import `repository/sqlite` (import cycle); the
  Postgres-gated ordering test lives in package `sqlite`.

## Parallelism

`sequential`

## Inputs

- System design: Projection, Failure and recovery, Cost.
- `applyMessageEventLocked`, `persistPendingRefreshLocked`, `MarkSessionRead`.

## Results

Done. `events.TaskSessionReadCursorChanged` is published by `MarkSessionRead`
with the persisted cursor (a stale request republishes the newer cursor) and by
the e2e read-cursor route with `reset: true`; a publish failure there returns
500. The projector subscribes to it and `SessionRemoved`;
`refreshUnreadBeforeEventLocked` performs task-wide recovery reads while
unobserved and per-session reads otherwise, never returning an error, and the
synchronous fold (`applyUnreadEventLocked`) only raises ranks except on reset.
Any handled event persists when the derived aggregate differs from the stored
one; without this, a cold projector whose restore read already contained the
triggering message never wrote the corrected value (found by
`TestTaskStatusSummaryUnreadLiveOrdering`). `SessionRemoved` drops only the
removed session's unread contribution and excludes it from the reload; its
other observation fields are unchanged. Flips are counted in
`task_status_summary_unread_flips_total` and logged at debug level. CAS retries
under streaming were not load-tested; the flip counter is the measurement point
named in the design. Verified by `projector_unread_test.go`,
`projector_unread_restore_test.go`, `TestTaskStatusSummaryUnreadLiveOrdering`,
and `TestMarkSessionReadPublishesPersistedCursor`.
