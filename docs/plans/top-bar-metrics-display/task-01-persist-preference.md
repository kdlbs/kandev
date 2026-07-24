---
id: "01-persist-preference"
title: "Persist simplified metrics preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/app-status-bar.md"
---

# Task 01: Persist simplified metrics preference

## Acceptance

- `system_metrics_display.simplified` round-trips through the existing backend-owned user-settings JSON and PATCH/event contract.
- Missing stored values remain `false`, preserving the current detailed presentation.
- SPA boot state and frontend boot/live-update normalization expose `systemMetricsDisplay.simplified` consistently.

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- Narrow complete-shape frontend fixtures reported by type-check.

## Inputs

- Spec: **What**, **Data, API, and persistence**, and detailed/simplified scenarios.
- Plan: **Backend** and **User-settings transport and state**.
- ADR: `docs/decisions/0041-backend-owned-portable-user-settings.md`.

## TDD sequence

1. Add failing backend default/round-trip and boot-state tests.
2. Add failing frontend boot/live-update normalization tests.
3. Implement the minimum model, mapper, type, and default changes.
4. Run targeted tests and type-check; update only complete-shape fixtures required by the field.

## Verification

- `rtk go test ./internal/user/store ./internal/backendapp` from `apps/backend`
- `rtk pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts` from `apps`
- `rtk pnpm run typecheck` from `apps/web`

## Dependencies

None.

## Output contract

Return a compact handoff capsule containing intent/acceptance, base/head SHA, changed files and entry points, named spec/ADR sections, `localized` and `persistence` risk tags, exact commands/results, uncertainties, and this task file updated to `done`. Do not edit `plan.md`.
