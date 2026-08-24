---
id: "02-frontend-settings-contract"
title: "Frontend settings contract and SSR mapping"
status: done
wave: 2
depends_on: ["01-backend-last-seen-setting"]
plan: "plan.md"
spec: "../../specs/ui/requirements/relative-last-seen.md"
---

# Task 02: Frontend settings contract and SSR mapping

## Acceptance

- `UserSettings` / `UserSettingsUpdatePayload` carry `last_seen_display?: "absolute" | "relative"`;
  `LastSeenDisplay` is re-exported from the `apps/web/lib/types/http.ts` barrel (the established
  import surface for these types). `UserSettingsState` carries `lastSeenDisplay` (default
  `"absolute"`) and NO client-only metadata field: pending-selection protection lives in the
  component (pending-write gating), so no writer-id/lastWriterId state exists to be wiped by
  `mapUserSettingsResponse` callers that build from a fresh default state (use-layout-settings,
  layout-preset-selector, use-ensure-user-settings, hydration).
- `parseLastSeenDisplay` accepts only `"relative"` and coerces everything else (including
  `undefined`) to `"absolute"`; `mapUserSettingsData` maps the snake_case field into the store
  shape.
- The WS `user.settings.updated` handler (`registerUsersHandlers`) propagates the field through
  `mapUserSettingsData` with its existing revision gate and is otherwise UNCHANGED (own echoes are
  applied normally, so the store converges even if the component unmounts mid-write): valid value
  applies, unknown value normalizes to `"absolute"`, omitted field leaves the current value, stale
  revision ignored. Covered in `users.test.ts`, not just by SSR tests.
- `apps/web/hooks/use-ensure-user-settings.test.ts` `makeUnloadedSettings` (lines ~47-110) builds a
  complete typed `UserSettingsState` and gains `lastSeenDisplay: "absolute"`; audit remaining typed
  fixtures for the new required field.
- SSR unit tests cover default, round-trip, and unknown-value coercion.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts
```

## Files likely touched

- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/http.ts` (barrel re-export)
- `apps/web/lib/state/slices/settings/types.ts` (`lastSeenDisplay`)
- `apps/web/lib/ssr/user-settings.ts` (+ `user-settings.test.ts`)
- `apps/web/lib/ws/handlers/users.ts` (+ `users.test.ts`: `last_seen_display` assertions)
- `apps/web/hooks/use-ensure-user-settings.test.ts` (typed fixture gains `lastSeenDisplay`)

## Dependencies

Task 01 (backend field must exist so the payload type matches the wire contract).

## Inputs

- Spec "What" (per-user persisted setting, WS sync)
- Existing precedent: `changesPanelLayout` through
  `http-user-settings.ts` → `types.ts` → `ssr/user-settings.ts` → `ws/handlers/users.ts`

## Output contract

Return a compact handoff capsule with acceptance status, exact test command/results, risk tags,
uncertainties, and set this task to `done`.
