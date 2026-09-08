---
id: "02-isolate-replaceable-session-delivery"
title: "Isolate replaceable session delivery"
status: done
wave: 2
depends_on: ["01-coalesce-agent-stream-ingress"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 02: Isolate Replaceable Session Delivery

## Acceptance

- `session.message.updated` uses one pending slot per client/session/message and
  a newer full-state payload replaces the older payload in place.
- Replaceable capacity is bounded per session and globally; one session's
  overflow cannot consume or evict control, semantic, or another session's
  allowance.
- Active session queues make round-robin progress while control and semantic
  traffic receive bounded priority.
- Message creation/deletion, turn completion, and state changes retain their
  order relative to the newest queued replacement for that session.
- Replacement/eviction/depth telemetry carries IDs and action but no content.

## TDD sequence

1. RED: use tiny injected capacities to prove replacement-in-place, stable
   ordering, per-session/global bounds, and noisy-session-only eviction.
2. RED: prove two quiet sessions, semantic notifications, and a correlated
   response progress while one session continuously replaces updates.
3. RED: cover close/drain races, zero-value test clients, and existing control
   overflow behavior.
4. GREEN: introduce a typed outbound notification scheduler and pass action,
   session, and message routing identity from hub broadcasts.
5. GREEN: update `WritePump` to drain bounded priority bursts and round-robin
   replaceable session queues.
6. REFACTOR: centralize classification/key extraction and add content-free
   queue metrics/logs.

## Files likely touched

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- new gateway notification scheduler/classifier files
- `apps/backend/internal/gateway/websocket/client_send_queue_test.go`
- `apps/backend/internal/gateway/websocket/session_notifications_test.go`
- related hub broadcast and lifecycle/leak tests

## Verification

```bash
cd apps/backend && go test ./internal/gateway/websocket/...
cd apps/backend && go test -race ./internal/gateway/websocket/...
```

Result: `go test ./internal/gateway/websocket/... -count=1` passed (200 tests).
Race result: `go test -race ./internal/gateway/websocket/... -count=1` passed
(200 tests).

Production capacities: 256 semantic FIFO frames, 32 distinct replaceable
message updates per session, and 256 replaceable updates per client. Control
responses/errors retain their independent 256-frame queue. Replacement keeps
its original per-session position; per-session pressure evicts only that
session's oldest replaceable entry, while a full global queue rejects an
incoming replacement whenever its session has no pending entry rather than
evicting another session. Queue logs include action,
client, session, message, reason, and depth only; content is never logged.

Files changed:

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/notification_scheduler.go`
- `apps/backend/internal/gateway/websocket/client_send_queue_test.go`

## Dependencies

Task 01 establishes lossless coalescing and boundary ordering before gateway
replacement is introduced.

## Parallelism

Sequential. Task 03 consumes the final replacement/barrier semantics.

## Inputs

- ADR **Isolate Replaceable Session Stream Traffic**.
- Existing reserved control queue and `client_send_queue_test.go`.
- Full-state `session.message.updated` payload from task service.

## Risks

- Moving a replacement to the back of a queue can overtake a terminal event;
  update bytes in the existing slot.
- Absolute priority can starve stream rendering; use bounded priority bursts.
- Parsing identity from raw JSON in multiple places can drift; classify once
  from the typed WebSocket message before enqueue.

## Output contract

Report production/test capacities, scheduling order, saturation measurements,
replacement and eviction behavior, metrics added, test results, and files
changed.
