---
id: "05-live-event-transport"
title: "Live event transport"
status: completed
wave: 5
depends_on:
  - "04-browser-data-state"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-006
  - REQ-PLUGINS-ISOLATED-WEB-APPS-009
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-006.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.3
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 05: Live event transport

## Summary

Add bounded Server-Sent Events for authorized public Kandev events. Define
connection ownership, replay, heartbeat, cancellation, and resync behavior.

## In scope

- Add the capability-path SSE endpoint and event-scope filters.
- Add process generation and monotonic event identifiers.
- Add 15-second heartbeats and immediate response flushing.
- Add per-user and per-instance connection admission.
- Add bounded time and event-count replay.
- Add gap and restart resync signals.
- Add disconnect cleanup, leak tests, metrics, and safe logs.

## Out of scope

- Canvas metadata events, source publishing, and host user interface.

## Acceptance

- Reconnect replays one complete range or sends `runtime.resync_required`.
- Current scope and grants filter every event, including replayed events.
- Disconnect releases subscriptions and counters without a timeout sleep.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/webapp/... ./internal/events/... ./internal/plugins/...
```

## Files likely touched

- `apps/backend/internal/plugins/webapp/events.go`
- `apps/backend/internal/plugins/webapp/events_test.go`
- `apps/backend/internal/events/**`
- `apps/backend/internal/backendapp/**`

## Dependencies

- Task 04 provides authorized data shapes and runtime context.

## Risks

- Replay can expose data after an instance loses access.
- Missing request cancellation can leak subscriptions and counters.
- Proxy buffering can hide heartbeats and delay disconnect detection.

## Parallelism

`sequential`

## Inputs

- Live event, request authorization, observability, and recovery sections.
- Current event bus lifecycle and goroutine ownership patterns.

## Results

Completed on 2026-08-28. Added the bounded per-instance SSE hub with process
generations, monotonic cursors, replay and resync signals, heartbeat flushing,
per-user and per-instance stream limits, slow-consumer handling, and context
cancellation cleanup. The plugin service projects scoped Kandev bus events
into the hub and rechecks capability authority while filtering delivery.
Focused web-app and plugin tests pass.
