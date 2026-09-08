---
id: "02-update-replay-and-wiring"
title: "Replay cached updates when a Local user subscribes"
status: done
wave: 2
depends_on: ["01-provider-event-and-delivery"]
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 02: Replay Cached Updates When a Local User Subscribes

## Acceptance

- Release detection invokes the canonical notifier and no longer owns a
  broadcaster, update-only policy store, or notified-version key.
- An immediate startup poll before any user subscriber leaves Local delivery
  pending; the first default-user subscription replays cached state without a
  GitHub request and delivers it once.
- The update notification settings endpoints and backend boot wiring are
  removed without changing the existing update-check/apply API.

## Verification

```bash
cd apps/backend
go test ./internal/system/updates ./internal/gateway/websocket ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/system/updates/service.go`
- `apps/backend/internal/system/updates/service_test.go`
- `apps/backend/internal/system/updates/notify_dispatch_test.go`
- `apps/backend/internal/system/updates/notify_settings*.go`
- `apps/backend/internal/system/updates/notify_store*.go`
- `apps/backend/internal/system/updates/handler.go`
- `apps/backend/internal/system/updates/handler_notify_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/persistence/meta.go`
- `apps/backend/internal/persistence/meta_test.go`
- `apps/backend/internal/persistence/postgres_meta_test.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/hub_user_subscription_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/gateway.go`
- Related backend application tests

## Dependencies

- Task 01 must expose the canonical update notifier behavior.

## Inputs

- Spec scenario `startup release poll completes before any Local client
  subscribes`.
- Plan sections `Release detection, cached replay, and startup wiring` and
  `Risks`.
- Current startup order in `apps/backend/internal/backendapp/main.go`.

## Output contract

Report exact notifier interface, subscription-callback ordering, removed API and
metadata surfaces, deterministic lifecycle tests, commands/results, risk tags,
and task status update.
