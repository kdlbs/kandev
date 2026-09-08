---
spec: docs/specs/platform/requirements/notifications.md
created: 2026-07-24
status: implemented
---

# Implementation Plan: Semantic Notifications

## Overview

Replace the overloaded `session.waiting_for_input` trigger with two
independently selectable semantic notifications:
`session.turn_finished` and `session.clarification_requested`. Source them from
durable turn/message events, give each occurrence durable idempotency, migrate
legacy subscriptions to clarification only, and carry the distinction through
Apprise, local browser, and native desktop delivery.

In the same settings surface, fix cold-load hydration so an asynchronously
loaded provider cannot be mistaken for missing configuration. Preserve unsaved
manual-save drafts once hydration has completed.

The domain-event boundary is recorded in
[the semantic notification ADR](../../decisions/2026-07-24-semantic-notification-events.md).

## Backend

### Semantic event sources and messages

Likely files:

- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/notifications/service/service.go`
- `apps/backend/internal/notifications/service/service_test.go`
- `apps/backend/internal/notifications/models/models.go`
- `apps/backend/pkg/websocket/actions.go`

Changes:

- Stop subscribing the notification service to generic
  `task.session_state_changed` transitions.
- Subscribe to canonical completed-turn and message-added events.
- Translate one durable completed agent turn into
  `session.turn_finished`, keyed by turn ID.
- Translate only a structured `clarification_request` message with
  `author_type=agent`, `requests_input=true`, and non-empty task/session/request
  identity into `session.clarification_requested`, keyed by the request's shared
  `pending_id`. This produces one notification for a multi-question
  clarification bundle.
- Produce the exact event-specific title and body from the spec, while keeping
  existing task/session/provider routing.
- Do not emit either notification on startup, readiness, user answers, or
  generic waiting-state transitions.

### Occurrence-scoped persistence and migration

Likely files:

- `apps/backend/internal/notifications/store/sqlite.go`
- `apps/backend/internal/notifications/store/sqlite_test.go`
- Notification models and repository interfaces used by the store

Changes:

- Add a non-empty occurrence ID to delivery records.
- Replace session-scoped uniqueness with
  `(provider_id, event_type, occurrence_id)` uniqueness on both SQLite and
  Postgres paths. Preserve the task-session ID as data.
- Make the schema upgrade replayable and safe for legacy databases whose
  delivery table contains the old inline unique constraint.
- Migrate every legacy `session.waiting_for_input` subscription to
  `session.clarification_requested`, merging enabled state idempotently when a
  semantic row already exists. Never create a turn-finished subscription from
  the legacy key.
- Update the available-event catalog and default subscriptions so
  clarification is on by default and turn completion is opt-in.
- Test fresh schema creation, legacy migration, repeated migration, replay of
  one occurrence, and multiple occurrences in one session.

## Frontend and Desktop

### Settings hydration and event choices

Likely files:

- `apps/web/components/settings/notifications-settings-actions.ts`
- `apps/web/components/settings/notifications-settings-actions.test.tsx`
- `apps/web/hooks/domains/settings/use-notification-providers.ts`
- `apps/web/lib/notifications/events.ts`

Changes:

- Add the two event rows with the labels and descriptions from the spec.
- Initialize the route-local notification draft when the asynchronous provider
  load first completes, rather than snapshotting the initial empty store.
- Hydrate only while the draft is clean/uninitialized so later store refreshes
  cannot erase unsaved edits.
- Make new-provider defaults clarification-only. Keep the existing shared
  floating Save behavior and provider-specific event checkboxes.
- Add a regression test that starts with an empty store, resolves a provider,
  and proves the provider and its selections appear without remounting. Add a
  second test proving a refresh does not replace a dirty draft.

### Local WebSocket and native notification parity

Likely files:

- `apps/backend/internal/notifications/providers/local.go`
- `apps/backend/internal/notifications/providers/local_test.go`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/notifications.ts`
- `apps/web/lib/ws/handlers/notifications.test.ts`
- `apps/desktop/src-tauri/src/native_notifications.rs`

Changes:

- Preserve the semantic event as the local WebSocket action instead of always
  broadcasting the legacy waiting action.
- Register both semantic actions in the web client and use event-specific
  fallback copy, while retaining existing active-task suppression, sound, and
  browser/native delivery behavior.
- Permit both semantic event prefixes at the desktop native boundary and keep
  legacy-prefix acceptance only where compatibility requires it.
- Carry the backend occurrence ID through Local WebSocket delivery and use it
  as the native de-duplication identity.
- Add frontend and Rust tests for both event types and for rejection of
  unrelated native notification events.

## E2E, Mobile, and Documentation

Likely files:

- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- A focused `apps/web/e2e/tests/settings/mobile-*.spec.ts`
- `docs/public/websocket-api.md`
- `docs/public/desktop-app.md`
- `docs/specs/desktop/requirements/desktop-tauri-app.md`
- `docs/specs/office/requirements/inbox.md`
- `docs/decisions/0039-native-desktop-integration-boundary.md`

Changes:

- Verify a cold settings load displays a seeded provider and both independent
  event controls.
- Verify one event selection can be changed and saved on desktop.
- Render the event/provider matrix as stacked event cards on narrow viewports,
  then verify both event rows remain readable and touch-operable at the
  repository's canonical mobile viewport without horizontal overflow.
- Update public WebSocket and desktop notification terminology and reconcile
  older specs/ADRs that name the retired waiting-state action.

## Waves

Wave 1:

- [x] [task-01-backend-events-and-persistence](task-01-backend-events-and-persistence.md)

Wave 2:

- [x] [task-02-frontend-and-desktop](task-02-frontend-and-desktop.md)

Wave 3:

- [x] [task-03-e2e-docs-and-verification](task-03-e2e-docs-and-verification.md)

## Verification

Targeted backend:

```bash
cd apps/backend
go test ./internal/notifications/... ./internal/backendapp/...
```

Targeted frontend and desktop:

```bash
cd apps
pnpm --filter @kandev/web test -- --run \
  components/settings/notifications-settings-actions.test.tsx \
  lib/ws/handlers/notifications.test.ts
cd web && pnpm run typecheck
cd ../desktop/src-tauri && cargo test native_notifications
```

Focused E2E:

```bash
cd apps/web
pnpm e2e:run --project chromium \
  tests/settings/settings-manual-save.spec.ts
pnpm e2e:run --project mobile-chrome \
  tests/settings/mobile-notification-events.spec.ts
```

Final repository verification:

```bash
make fmt
make typecheck
make test
make lint
```

## Risks

- The current delivery table embeds its legacy unique constraint in schema
  creation, so SQLite migration may require a transactional table rebuild while
  Postgres requires a dialect-specific constraint/index migration.
- Clarification bundles contain several messages; filtering on
  `requests_input=true` and using their shared request identity is essential to
  avoid one alert per question.
- A completed-turn domain event may also participate in recovery paths. Tests
  must prove occurrence IDs make replay idempotent without suppressing later
  turns.
- Frontend hydration must distinguish initial data arrival from a later server
  refresh while the user has unsaved changes.
