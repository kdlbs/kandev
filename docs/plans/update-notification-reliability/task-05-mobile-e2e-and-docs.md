---
id: "05-mobile-e2e-and-docs"
title: "Prove mobile provider settings and update the public contract"
status: done
wave: 4
depends_on: ["04-frontend-provider-delivery"]
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 05: Prove Mobile Provider Settings and Update the Public Contract

## Acceptance

- Mobile Playwright toggles the update event through the existing provider
  matrix, saves, reloads, and proves persistence, 44px touch targets,
  containment, and no document horizontal overflow.
- The obsolete update-only settings E2E is deleted.
- Public operations and WebSocket docs describe provider-based update
  configuration and the version occurrence payload.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/settings/mobile-notification-events.spec.ts
cd ../../
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/settings/mobile-notification-events.spec.ts`
- `apps/web/e2e/tests/system/update-notifications-settings.spec.ts`
- `docs/public/operations.md`
- `docs/public/websocket-api.md`

## Dependencies

- Task 04 must land the final provider settings and Local delivery surface.

## Inputs

- Spec mobile and denied-permission scenarios.
- Plan `Mobile Design Contract`, `E2E Tests`, and `Public and Internal
  Documentation`.
- `/mobile-parity` and `/e2e` guidance.

## Output contract

Report RED/GREEN E2E evidence, rendered mobile geometry, removed obsolete test,
public-doc validation, files changed, risk tags, and task status update.
