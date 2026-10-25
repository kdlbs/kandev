---
id: "01-provider-event-and-delivery"
title: "Route update occurrences through notification providers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 01: Route Update Occurrences Through Notification Providers

## Acceptance

- `system.update_available` is selectable for every provider and uses the
  release version as its provider-scoped delivery occurrence ID.
- A successful provider delivery deduplicates replay, while a Local send with
  no eligible user subscriber releases only its own claim.
- Fresh and upgraded Local/System defaults include the event exactly once;
  existing Apprise providers are unchanged and a later opt-out survives restart.

## Verification

```bash
cd apps/backend
go test ./internal/notifications/...
```

## Files likely touched

- `apps/backend/internal/notifications/service/service.go`
- `apps/backend/internal/notifications/service/service_test.go`
- `apps/backend/internal/notifications/providers/provider.go`
- `apps/backend/internal/notifications/providers/local.go`
- `apps/backend/internal/notifications/providers/local_test.go`
- `apps/backend/internal/notifications/store/sqlite.go`
- `apps/backend/internal/notifications/store/sqlite_test.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/hub_broadcast_test.go`

## Dependencies

None.

## Inputs

- Spec sections `What`, `Data Model`, `Failure Modes`, and
  `Persistence Guarantees`.
- ADR `2026-07-24-semantic-notification-events`.
- Existing semantic-occurrence flow in
  `apps/backend/internal/notifications/service/service.go`.

## Output contract

Report event copy/payload, occurrence and migration identities, provider failure
semantics, files changed, exact test results, risk tags, and task status update.
