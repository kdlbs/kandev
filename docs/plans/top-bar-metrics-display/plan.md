---
spec: docs/specs/ui/app-status-bar.md
created: 2026-07-23
status: complete
---

# Implementation Plan: Simplified Resource Metrics Display

## Overview

Extend the existing backend-owned `system_metrics_display` user preference with a backward-compatible `simplified` boolean, carry it through boot hydration and live settings updates, and expose it in Appearance settings. The metrics renderer will use the same data, ordering, thresholds, limits, subscriptions, and tooltips in both styles; simplified mode removes only the host marker and percentage meter bars. Desktop and phone E2E coverage will prove the saved choice and responsive presentation.

## Backend

### Portable user setting

- `apps/backend/internal/user/models/models.go`: add `Simplified bool` to `SystemMetricsDisplaySettings` with wire name `simplified`; the Go zero value preserves detailed mode for existing rows.
- `apps/backend/internal/backendapp/boot_state_routes.go`: include `simplified` in the camel-cased `systemMetricsDisplay` boot payload.
- `apps/backend/internal/user/store/sqlite_test.go`: extend default and round-trip coverage to prove missing values remain detailed and `true` persists in the existing settings JSON.
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`: prove the SPA boot state includes the simplified preference.

No schema migration, endpoint, or WebSocket action is required. Existing user-settings PATCH, DTO, service, SQLite JSON, and `user.settings.updated` paths already replace and broadcast `SystemMetricsDisplaySettings`.

## Frontend

### User-settings transport and state

- `apps/web/lib/types/http-user-settings.ts` and `apps/web/lib/types/backend.ts`: add the optional snake-case `simplified` wire field.
- `apps/web/lib/state/slices/settings/types.ts` and `apps/web/lib/state/slices/settings/settings-slice.ts`: add the required camel-case `simplified` state field with a detailed-mode default.
- `apps/web/lib/ssr/user-settings.ts`: map missing wire values to `false` for boot hydration and WebSocket updates.
- `apps/web/hooks/use-user-display-settings.ts`: preserve the complete default metrics display shape.
- `apps/web/lib/ssr/user-settings.test.ts` and `apps/web/lib/ws/handlers/users.test.ts`: cover saved and missing preference normalization across boot and live updates.
- Update narrow existing fixtures whose complete `systemMetricsDisplay` state objects require the new field.

### Appearance setting

- `apps/web/components/settings/general-settings.tsx`: include the simplified choice in the Appearance saved/draft state, user-settings PATCH, in-memory commit, dirty tracking, and discard behavior.
- `apps/web/components/settings/system-metrics-settings-card.tsx`: add a self-documenting **Simplified metrics** switch below the display toggle. Its copy states that the choice removes the Host marker and progress bars while retaining metric icons and values; it participates in the shared floating **Save changes** flow.
- `apps/web/components/settings/system-metrics-settings-card.test.tsx`: prove the control is accessible and reports dirty state without introducing a card-local save.

### Metrics rendering

- `apps/web/components/system-metrics/status-surface-metrics.tsx`: read the preference once and pass it through the bar and drawer render paths. In simplified mode, omit `SourceBadge` and `MetricMeter`; retain metric icons, values, colors, limits, accessible names, and tooltip details.
- `apps/web/components/system-metrics/status-surface-metrics.test.tsx`: prove detailed mode remains the default and simplified mode removes only the host marker/meters on desktop and phone.

## Mobile design contract

- **Desktop outcome:** Appearance settings can save simplified metrics; the global or fallback bar renders icon/value pairs without Host or meters.
- **Mobile entry point:** the existing Settings mobile menu opens the existing global Status inset bottom drawer.
- **Nearest exemplar:** `apps/web/components/app-status-bar/app-status-drawer.tsx` supplies the existing inset drawer, fixed header, single internal scroll owner, safe-area clearance, and 44 px status rows. `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts` supplies the responsive Appearance/save pattern.
- **Hierarchy and action:** the new setting remains an inline switch inside Resource Metrics; the shared floating Save action remains primary. Status remains an existing drawer row, not a new navigation surface.
- **Shared versus responsive behavior:** the persisted preference and metric view-model are shared. Only existing bar-versus-drawer composition differs by viewport.
- **Scrolling and touch:** the settings page keeps document scrolling and the shared floating Save control; the Status drawer keeps its one `overflow-y-auto` body and existing safe-area handling. The labeled switch row and existing drawer rows remain touch reachable.

## Tests

- **What:** missing and persisted `simplified` values survive SQLite and boot mapping.
  - **Files:** `apps/backend/internal/user/store/sqlite_test.go`, `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
  - **How:** focused Go unit tests over the real SQLite settings JSON and boot-state mapper.
- **What:** boot and live-update payloads normalize missing preferences to detailed and retain explicit simplified mode.
  - **Files:** `apps/web/lib/ssr/user-settings.test.ts`, `apps/web/lib/ws/handlers/users.test.ts`
  - **How:** Vitest pure state-mapping tests.
- **What:** settings dirty-state integration and renderer output for detailed/simplified desktop and phone presentations.
  - **Files:** `apps/web/components/settings/system-metrics-settings-card.test.tsx`, `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
  - **How:** focused Testing Library/Vitest tests using the existing settings coordinator and state provider.

## E2E Tests

- **Scenario:** selecting and saving simplified metrics persists across reload and the desktop app status bar shows metric icon/value pairs without Host or meter bars.
  - **File:** `apps/web/e2e/tests/settings/resource-metrics-display.spec.ts`
  - **What to verify:** control dirty state, shared Save, reload persistence, metrics visibility, and absence of simplified-away elements.
- **Scenario:** the saved simplified preference produces the same icon/value presentation in the phone Status drawer.
  - **File:** `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`
  - **What to verify:** mobile setting/save path, existing Status drawer entry point, 44 px row containment, no Host marker/meters, and no document horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [Task 01: Persist simplified metrics preference](task-01-persist-preference.md)

Wave 2:

- [x] [Task 02: Add simplified settings and rendering](task-02-settings-and-rendering.md)

Wave 3:

- [x] [Task 03: Prove desktop and mobile flows](task-03-responsive-e2e.md)

## Verification

- `rtk go test ./internal/user/store ./internal/backendapp` from `apps/backend`
- `rtk pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/settings/system-metrics-settings-card.test.tsx components/system-metrics/status-surface-metrics.test.tsx` from `apps`
- `rtk pnpm run typecheck` from `apps/web`
- `rtk pnpm e2e:run tests/settings/resource-metrics-display.spec.ts` from `apps/web`
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-resource-metrics-display.spec.ts` from `apps/web`

## Risks

- Full-shape frontend fixtures will fail type-check until each gains the backward-compatible `simplified: false` default.
- E2E must wait for a real sampled metrics snapshot before asserting rendered values; it must not assert mock-only elements.
