---
id: "04-frontend-provider-delivery"
title: "Use provider settings and preserve the in-app update indication"
status: done
wave: 3
depends_on:
  - "01-provider-event-and-delivery"
  - "02-update-replay-and-wiring"
  - "03-native-permission-bridge"
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 04: Use Provider Settings and Preserve the In-App Update Indication

## Acceptance

- The Updates page has no update-only settings card/API/state; the existing
  provider matrix labels and persists `system.update_available`.
- A delivered Local update occurrence always shows an in-app toast and attempts
  native/browser delivery only without prompting; denied, unsupported, and
  command-failure paths retain the toast.
- The Notifications permission control uses the native permission bridge in
  Tauri and the Web Notification API elsewhere, exposing denied/unsupported
  state in the existing UI.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  lib/ws/handlers/notifications.test.ts \
  hooks/use-update-available-toast.test.ts \
  components/settings/notifications-settings-actions.test.tsx \
  lib/state/hydration/hydrator.test.ts \
  src/settings-routes.test.ts
cd web
pnpm run typecheck
```

## Files likely touched

- `apps/web/components/settings/system/update-notifications-card*.tsx`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/lib/types/system.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/notifications/events.ts`
- `apps/web/components/settings/notifications-settings.tsx`
- `apps/web/components/settings/notifications-settings-actions.ts`
- `apps/web/components/settings/notifications-settings-actions.test.tsx`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/lib/state/slices/system/*`
- `apps/web/lib/state/slices/ui/*`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/ws/handlers/notifications.ts`
- `apps/web/lib/ws/handlers/notifications.test.ts`
- `apps/web/lib/ws/handlers/system-events.ts`
- `apps/web/lib/ws/handlers/system-events.test.ts`
- `apps/web/hooks/use-update-available-toast.ts`
- `apps/web/hooks/use-update-available-toast.test.ts`
- `apps/web/components/update-available-toast-bridge.tsx`

## Dependencies

- Task 01 defines the event/payload contract.
- Task 02 removes the parallel backend policy API.
- Task 03 provides non-prompting native permission and delivery methods.

## Inputs

- Spec sections `API Surface`, `Failure Modes`, and mobile scenario.
- Plan `Frontend` and `Mobile Design Contract`.
- ADR-0046 settings save-coordinator invariant.

## Output contract

Report deleted orphan surfaces, event registration, fallback behavior,
permission UI behavior, tests/typecheck, risk tags, and task status update.
