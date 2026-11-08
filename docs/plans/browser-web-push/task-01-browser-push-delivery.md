---
id: TASK-PLATFORM-BROWSER-WEB-PUSH-001
title: Add browser push delivery
status: done
wave: 1
plan: plan.md
depends_on: []
requirements:
  - REQ-PLATFORM-WEB-PUSH-001
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-WEB-PUSH-001.1
  - AC-PLATFORM-WEB-PUSH-001.2
  - AC-PLATFORM-WEB-PUSH-001.3
  - AC-PLATFORM-WEB-PUSH-001.4
  - AC-PLATFORM-WEB-PUSH-001.5
  - AC-PLATFORM-WEB-PUSH-001.6
  - AC-PLATFORM-WEB-PUSH-001.7
  - AC-PLATFORM-NOTIFICATIONS-001.1
  - AC-PLATFORM-NOTIFICATIONS-001.2
  - AC-PLATFORM-NOTIFICATIONS-001.3
  - AC-PLATFORM-NOTIFICATIONS-001.4
system_design:
  - ../../specs/platform/system-design/web-push.md
---

# Task 01: Browser push delivery

## Outcome

Deliver selected notification events to browser devices while keeping device
ownership, notification policy, and browser state with their existing owners.

## Scope

- Add Web Push subscription and diagnostic API operations and backend storage.
- Reuse notification provider event selection and occurrence claims.
- Route background deliveries through a bounded queue and restricted HTTPS sender.
- Add browser enrollment, logout cleanup, safe worker routing, and localized
  offline behavior.
- Keep Web Push controls in the Notifications settings page.

## Exclusions

- Offline task operations or cached task content.
- A second event-selection or recipient model.
- A new public plugin API.

## Acceptance conditions

1. The user can enroll and remove a browser device, and the backend keeps its
   user ownership separate from provider event choices.
2. Selected event delivery uses the existing notification service, while
   outbound destinations and queue lifecycle follow the Web Push design.
3. The worker shows safe push notifications, opens only same-origin targets,
   and uses the localized offline page without caching the app shell.

## UI preview

See [UI-01 in the plan](plan.md#ui-preview). The Notifications settings page
keeps one vertical scroll owner on phones.

## Verification

- `cd apps/backend && go test ./internal/notifications/webpush ./internal/common/config ./internal/persistence/requiredstores`
- `cd apps/backend && go test ./internal/backendapp -run 'TestPermissionNotificationOccurrence|TestFailedSessionOccurrence' -count=1`
- `cd apps/desktop/src-tauri && cargo test --lib native_notifications::tests::bridge_accepts_semantic_session_events`
- `cd apps/web && pnpm exec vitest run lib/ws/handlers/notifications.test.ts sw/sw-core.test.ts`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`

## Likely files and dependencies

The implementation changes `apps/backend/internal/notifications/webpush`,
`apps/backend/internal/common/config`, `apps/web/lib/pwa`, `apps/web/sw`, and
the Notifications settings UI. It uses the existing notification service,
provider store, API client, locale catalogs, and desktop notification command.

## Results

The focused Go, Rust, and frontend tests passed. Frontend ESLint, i18n, and
typecheck passed. The normal commit hooks passed. Hosted CI must pass on the
current pull request head before merge.
