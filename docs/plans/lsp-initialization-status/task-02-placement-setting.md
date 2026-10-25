---
id: "02-placement-setting"
title: "Portable placement setting"
status: completed
wave: 2
depends_on: ["01-initialization-stage"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 02: Portable Placement Setting

## Acceptance

- `lsp_status_location` round-trips through the existing backend user-settings JSON, DTO, PATCH, event, and boot-state paths; missing values default to `toolbar` and invalid PATCH values fail.
- Settings > Editors exposes `toolbar` and `status_bar`, participates in the route save coordinator, and explains the application-status-bar/fine-pointer fallback.
- One pure helper resolves effective toolbar versus status-bar placement without mutating the saved preference.

## TDD sequence

1. Add failing backend tests for default, round-trip, validation, DTO, event, and boot mapping.
2. Implement the backend contract and run its focused Go packages.
3. Add failing frontend hydration, WS, dirty-state, and placement-resolution tests.
4. Implement store/API/settings wiring and the placement choice.

## Verification

```bash
cd apps/backend && go test -run 'Test.*LspStatusLocation' ./internal/user/dto ./internal/user/service ./internal/user/store ./internal/backendapp
cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/settings/settings-dirty.test.ts lib/lsp/lsp-status-placement.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/settings lib/ssr/user-settings.ts lib/ws/handlers/users.ts lib/lsp/lsp-status-placement.ts hooks/use-user-display-settings.ts
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/components/settings/editors-settings-state.tsx`
- `apps/web/components/settings/editors-settings.tsx`
- `apps/web/components/settings/settings-dirty.ts`
- `apps/web/components/settings/settings-dirty.test.ts`
- `apps/web/lib/lsp/lsp-status-placement.ts`
- `apps/web/lib/lsp/lsp-status-placement.test.ts`

## Dependencies

Task 01 supplies the shared status language used in the placement UI.

## Parallelism

Sequential. Backend and frontend share one portable wire contract.

## Inputs

- Spec User settings and placement scenarios.
- ADR 0041 for backend-owned portable settings.
- ADR 0046 and `EditorsSettings` for coordinated save behavior.

## Output contract

Record RED/GREEN evidence, wire/default values, files changed, exact commands, and update this task plus `plan.md`.

## Result

- RED backend: the focused Go command failed because the model, DTO, validation, persistence, event, and boot-state contracts did not expose `lsp_status_location`.
- GREEN backend: focused tests pass across DTO, service, store, controller, and boot-state packages. The backend-owned JSON value accepts `toolbar` and `status_bar`, defaults missing or unknown stored values to `toolbar`, and rejects invalid PATCH values.
- RED frontend: the four focused Vitest files failed on missing hydration, live-update, dirty-state, and placement behavior.
- GREEN frontend: 47 focused tests, the full web typecheck, and focused ESLint pass without warnings.
- Settings > Editors now saves the portable preference through the route save coordinator. A pure resolver keeps `status_bar` only when the application status bar and a fine pointer are available, falls back to `toolbar` without rewriting the preference, and excludes the phone viewer.
