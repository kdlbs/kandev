---
id: "04-reachability-api-and-events"
title: "Reachability API, change event, and immediate probe"
status: pending
wave: 3
depends_on: ["03-reachability-poller"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
  - REQ-EXECUTORS-SSH-REACHABILITY-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.14
  - AC-EXECUTORS-SSH-REACHABILITY-001.18
  - AC-EXECUTORS-SSH-REACHABILITY-001.19
  - AC-EXECUTORS-SSH-REACHABILITY-001.26
  - AC-EXECUTORS-SSH-REACHABILITY-001.28
  - AC-EXECUTORS-SSH-REACHABILITY-002.2
  - AC-EXECUTORS-SSH-REACHABILITY-002.3
  - AC-EXECUTORS-SSH-REACHABILITY-002.5
  - AC-EXECUTORS-SSH-REACHABILITY-002.6
  - AC-EXECUTORS-SSH-REACHABILITY-002.7
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 04: Reachability API, Change Event, and Immediate Probe

## Summary

Project the reachability record over HTTP as its own resource, publish a
change event only when the state or reason actually changes, and add an
immediate-probe action that coalesces concurrent callers into one connection.
Reset an executor's record when its connection configuration is saved.

## In scope

- Three routes on the existing `/api/v1/ssh` group:
  `GET /reachability` for every SSH executor's record,
  `GET /executors/:id/reachability` for one, and
  `POST /executors/:id/reachability/probe` to probe now, persist, and return
  the record. A `POST` for an unknown executor returns 404; for a non-SSH
  executor, 400 — matching the existing `resolveSSHTarget` mapping.
- `probing_enabled` on the response, `false` when the interval key is `0`, so
  a surface distinguishes "not probed yet" from "probing is off" without
  inferring it from an absent timestamp.
- Coalescing through a `golang.org/x/sync/singleflight` group keyed by executor
  id, so two overlapping `POST`s open one connection and share one result. Its
  write follows the same last-write-wins rule as a scheduled probe.
- The immediate probe runs and persists while the poller is disabled.
- `events.ExecutorReachabilityChanged = "executor.reachability.changed"`, a
  matching `ws.ActionExecutorReachabilityChanged`, and the bridge subscription
  in `task_notifications.go` beside `events.ExecutorUpdated`. Published only
  when `state` or `reason` differs from the stored record.
- Creating an SSH executor, or saving a change to its connection
  configuration, resets its record to `unknown` with a zero counter and
  triggers an out-of-band probe without waiting for the next scheduled pass.

## Out of scope

- Any frontend file. Task 06 owns the client types, API calls, and store.
- Changing `POST /api/v1/ssh/test`, `probe-agents`, or `probe-shells`. The
  immediate-probe action is a separate action against a saved executor and its
  pinned fingerprint; the test endpoint keeps dialing unpinned against a
  possibly-unsaved form configuration and keeps writing no record.
- Reachability fields on the executor DTO. A record that changes every interval
  must not invalidate the executor list payload.
- Any launch-path behavior.

## Acceptance

- Two overlapping `POST` probes for one executor produce exactly one dial and
  return one identical record to both callers.
- A probe whose result matches the stored state and reason publishes nothing;
  a probe that changes either publishes exactly one event carrying the record.
- Saving a changed host on an existing SSH executor leaves its record
  `unknown` with a zero counter and a probe already dispatched, rather than
  carrying the previous target's failure streak.

## Verification

Start with the coalescing test as a failing test — hold a probe open, issue a
second request, and assert one dial and two identical responses — and confirm
it fails against a handler that probes per request. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/ssh/...
go test -tags fts5 -race ./internal/executors/reachability/... -run 'Publish|Change'
go test -tags fts5 -race ./internal/gateway/websocket/ -run 'Reachability|Executor'
go test -tags fts5 -race ./internal/events/...
make lint
```

## Files likely touched

- `apps/backend/internal/ssh/reachability_handlers.go`
- `apps/backend/internal/ssh/reachability_handlers_test.go`
- `apps/backend/internal/ssh/handlers.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/executors/reachability/publish.go`
- `apps/backend/internal/executors/reachability/publish_test.go`

## Dependencies

Task 03. The poller's write path is the single place that decides whether a
result is a change, and the immediate probe must share it rather than
reimplementing hysteresis at the handler.

## Risks

- A `singleflight` group keyed by executor id shares the *result* as well as
  the work. A caller that mutates the returned record would corrupt the other
  caller's copy; return a value, not a pointer into shared state.
- The change-only publish rule is what keeps a steady host silent, and it is
  also what makes `checked_at` age in an open client. Task 06 must refetch on
  the interval; if that is dropped, this task's correctness becomes a
  user-visible staleness bug in a different work order.
- Registering a new WS action without the bridge subscription produces no
  compile error and no test failure unless the bridge is asserted directly.

## Parallelism

`parallel-safe` with task 05 — disjoint files, both depend only on task 03.

## Inputs

- System design, section *API and event contracts*.
- `internal/ssh/handlers.go` for the route group, the 404/400 mapping, and
  the WS dispatcher registration shape.
- `internal/gateway/websocket/task_notifications.go` around the
  `events.Executor*` subscriptions.

## Results

Pending.
