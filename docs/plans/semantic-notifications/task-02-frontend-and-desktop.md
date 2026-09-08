---
id: "02-frontend-and-desktop"
title: "Expose semantic notification settings and delivery"
status: done
wave: 2
depends_on: ["01-backend-events-and-persistence"]
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 02: Expose Semantic Notification Settings and Delivery

## Acceptance

- Settings show independent Agent turn finished and Agent needs an answer
  controls with clarification selected by default for a new provider.
- A provider fetched after cold mount hydrates the draft without a remount.
- A later provider refresh does not overwrite unsaved notification edits.
- Local browser and native desktop delivery preserve each semantic action and
  use its event-specific copy.
- Existing active-task suppression and sound behavior remain intact.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- --run \
  components/settings/notifications-settings-actions.test.tsx \
  lib/ws/handlers/notifications.test.ts
cd web && pnpm run typecheck
cd ../desktop/src-tauri && cargo test native_notifications
```

## Files likely touched

- `apps/web/components/settings/notifications-settings-actions.ts`
- `apps/web/components/settings/notifications-settings-actions.test.tsx`
- `apps/web/lib/notifications/events.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/notifications.ts`
- `apps/web/lib/ws/handlers/notifications.test.ts`
- `apps/desktop/src-tauri/src/native_notifications.rs`

## Dependencies

- Task 01 defines the canonical event/action strings and provider API catalog.

## Inputs

- Spec sections `What`, `Failure Modes`, and `Scenarios`.
- Existing settings-route save coordinator contract.
- Existing local notification and native desktop bridges.

## Output contract

Report the hydration guard, dirty-draft behavior, UI defaults, action/type
changes, desktop allowlist behavior, files changed, tests run, and residual
risks.
