---
id: "01-persist-profile-recency"
title: "Persist bounded profile recency"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-003.1
  - AC-AGENTS-PROFILE-RECENT-USE-003.2
  - AC-AGENTS-PROFILE-RECENT-USE-003.4
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 01: Persist Bounded Profile Recency

## Summary

Implement the independent, bounded user recency record and expose it through
focused HTTP, boot, and WebSocket contracts. Preserve the existing user-settings
blob and event path unchanged.

## In scope

- Replay-safe SQLite/PostgreSQL schema and user-delete cascade.
- Repository and service move-to-front logic with ten-ID cap, revisions,
  255-byte ID validation, bounded conflict retry, and no-op suppression.
- DTO, controller, focused GET/PUT handlers, boot projection, event type,
  WebSocket action, and user-routed broadcaster registration.
- Backend unit and integration tests for persistence, contracts, and routing.

## Out of scope

- Frontend state, selector ordering, or launch integration.
- Agent-profile eligibility validation beyond bounded non-empty IDs.
- Changes to `users.settings` or `user.settings.updated`.

## Acceptance

- The backend stores no more than four rows and forty IDs per user and returns
  independent revisioned context records.
- Reusing the leading ID performs no write and publishes no event; concurrent
  changed writes converge through bounded revision retry.
- Boot and WebSocket projections are user-scoped, compact, and non-fatal to the
  primary app or launch path.

## Verification

```bash
cd apps/backend && go test ./internal/user/... ./internal/backendapp ./internal/gateway/websocket
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/store/store.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/agent_profile_recent_use_test.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/handlers/handlers.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/user_notifications.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`

## Dependencies

None.

## Risks

- SQLite and PostgreSQL expose different conflict details; tests must exercise
  replay and concurrent-update behavior without relying on dialect-specific
  error strings.
- Boot state has multiple route builders, so the shared authenticated projection
  must cover every selector entry point without duplicate queries.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-PROFILE-RECENT-USE-003`
- Persistence, synchronization, security, and failure sections in the system
  design.
- Existing user-settings CAS, boot-state, and user notification patterns.

## Results

Implemented the dedicated bounded user recency table, repository CAS writes,
service move-to-front logic, HTTP/boot/WebSocket contracts, and user-scoped
broadcast routing. Verified with:

```bash
cd apps/backend && go test ./internal/user/... ./internal/backendapp ./internal/gateway/websocket
```

Result: 1573 tests passed in 8 packages.
