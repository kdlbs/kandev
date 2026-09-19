---
id: "01-backend-events-and-persistence"
title: "Emit and persist semantic notification occurrences"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 01: Emit and Persist Semantic Notification Occurrences

## Acceptance

- Startup and generic session waiting transitions send no notification.
- Each completed agent turn emits `session.turn_finished` with the specified
  copy and turn occurrence ID.
- Each structured clarification bundle emits one
  `session.clarification_requested` with the specified copy and request
  occurrence ID.
- Replaying an occurrence is idempotent, while later occurrences in the same
  session can notify.
- Legacy waiting subscriptions migrate idempotently to clarification only
  without losing providers, credentials, or delivery history.

## Verification

```bash
cd apps/backend
go test ./internal/notifications/... ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/notifications/models/models.go`
- `apps/backend/internal/notifications/service/service.go`
- `apps/backend/internal/notifications/store/sqlite.go`
- `apps/backend/internal/notifications/providers/local.go`
- `apps/backend/pkg/websocket/actions.go`
- Related backend tests

## Dependencies

None.

## Inputs

- Spec sections `Data Model`, `State and Event Semantics`, and
  `Persistence Guarantees`.
- ADR `2026-07-24-semantic-notification-events`.
- Existing task `turn.completed` and `message.added` event payloads.

## Output contract

Report the exact event filters, occurrence-ID sources, dialect migration
strategy, legacy-subscription merge behavior, files changed, tests run, and
residual risks.
