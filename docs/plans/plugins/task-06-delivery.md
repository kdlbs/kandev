---
id: task-06
title: Event delivery subscriber (HMAC, retries, ring buffer)
status: done
wave: A
depends_on: [task-05]
plan: docs/plans/plugins/plan.md
---

# Event delivery subscriber

## Title
Subscribe plugins to the event bus and deliver signed event webhooks with
at-least-once semantics, sequential per plugin, retries, and an error-state buffer.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "Event webhook delivery" (headers,
  envelope, HMAC-SHA256(webhook_secret, raw_body), at-least-once, 10s timeout,
  3 retries backoff 5s/15s/45s, sequential per plugin, ring buffer 100 events /
  5min TTL while plugin is in error).
- Bus API: `internal/events/bus.EventBus.Subscribe(subject, func(ctx, *bus.Event) error)`.
  Model after `internal/gateway/websocket/task_notifications.go` `subscribe` helper.
  Wildcard subjects supported (`task.*`).
- Event envelope JSON: `{event_id, event_type, occurred_at, workspace_id, payload}`.
  `payload` = marshaled `event.Data`. `event_id` = generate (uuid). `occurred_at`
  = now RFC3339. `workspace_id` from event data if present (`data["workspace_id"]`),
  else omit.

## Acceptance
1. `internal/plugins/delivery/deliverer.go`: `Deliverer` subscribes each active
   plugin to the union of its `capabilities.events` subjects on the bus. On event,
   builds the envelope and signs `sha256=<hmac>` with the plugin's webhook secret.
   The webhook secret is recoverable via `store.RevealWebhookSecret(ctx, id)`
   (task-02 stores it in the encrypted secrets vault) — the Deliverer takes a
   small `SecretRevealer` interface `{ RevealWebhookSecret(ctx, id) (string, error) }`
   satisfied by the Service/store. This survives restarts (no in-memory-only cache).
2. Per-plugin worker goroutine + bounded queue → sequential delivery, no bus
   blocking. HTTP POST to `base_url+endpoints.events`, 10s timeout, retries
   5s/15s/45s on non-2xx/timeout.
3. When plugin status == error: buffer events in a ring buffer (cap 100, TTL 5min,
   drop+log oldest on overflow). On recovery (task-07 flips to active) flush in order.
4. `Refresh()` re-subscribes when a plugin is registered/enabled/disabled.

## Files
- `apps/backend/internal/plugins/delivery/deliverer.go` + `deliverer_test.go`
- `apps/backend/internal/plugins/delivery/ringbuffer.go` + `ringbuffer_test.go`
- `apps/backend/internal/plugins/delivery/envelope.go` + `envelope_test.go`

## Verification
- `go test ./internal/plugins/delivery/...` from `apps/backend` (use httptest server
  as the plugin; a fake bus or the real MemoryEventBus)
- `make -C apps/backend lint`

## Output contract
Report: how the webhook secret is held for signing, queue/worker model, buffer
behavior, and the Refresh trigger contract for task-05/09. Coordinate secret
handling with task-05 (may require a small addition there — note it in the report).

## Dependencies
task-05.
