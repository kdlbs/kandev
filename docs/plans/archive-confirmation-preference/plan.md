---
spec: docs/specs/tasks/archive-confirmation.md
created: 2026-07-15
status: completed
---

# Implementation Plan: Archive Confirmation Preference

## Overview

Extend the existing per-user JSON settings contract with a default-true archive confirmation preference. Hydrate it into frontend state, expose an optimistic General settings toggle, and let the shared archive dialog execute immediately with cascade disabled when confirmation is off so every current archive surface inherits the behavior.

## Backend

- Add `ConfirmTaskArchive bool` to the user settings model and DTO plus `*bool` PATCH fields in `apps/backend/internal/user/dto/dto.go` and `apps/backend/internal/user/service/service.go`.
- Preserve missing-field compatibility in `apps/backend/internal/user/store/sqlite.go` by decoding the stored JSON field through a pointer and defaulting it to `true`.
- Include `confirm_task_archive` in stored JSON, user settings events, and the Go boot-payload mapping.
- Cover defaulting, explicit false persistence, patch application, and DTO mapping with focused Go tests.

## Frontend

- Add `confirm_task_archive` to HTTP/WS types and `confirmTaskArchive` to the settings slice, boot/SSR mapping, and WS update handler, defaulting missing values to `true`.
- Add a **Confirm before archiving tasks** switch to a dedicated Task Actions section under General settings, with optimistic persistence and rollback on failure.
- Update `TaskArchiveConfirmDialog` to read the preference. On an open transition while confirmation is disabled, invoke `onConfirm({ cascade: false })`, close the controlled state, and render no dialog content.

## Tests

- **Default true and explicit false:** `apps/backend/internal/user/store/sqlite_test.go`, `apps/backend/internal/user/service/service_test.go`, and `apps/backend/internal/user/dto/dto_test.go`.
- **Frontend mapping and live updates:** `apps/web/lib/ssr/user-settings.test.ts` and `apps/web/lib/ws/handlers/users.test.ts`.
- **Dialog behavior:** `apps/web/components/task/task-archive-confirm-dialog.test.tsx` verifies enabled confirmation and confirmation-free single execution with `cascade: false`.
- **Settings rollback:** `apps/web/components/settings/archive-confirmation-settings.test.tsx` verifies optimistic update, persistence payload, disabled state, and rollback.

## E2E Tests

- Desktop: disable confirmation, archive from the sidebar, assert no alert dialog and task removal.
- Mobile: disable confirmation, archive through the mobile task switcher, assert no alert dialog and task removal.
- Use the existing production-build E2E fixtures and user-settings reset path.

## Implementation Waves

1. [x] [task-01-backend-preference](task-01-backend-preference.md)
2. [x] [task-02-frontend-behavior](task-02-frontend-behavior.md)
3. [x] [task-03-e2e-and-verification](task-03-e2e-and-verification.md)

## Verification

```bash
make -C apps/backend fmt
rtk go test ./internal/user/... ./internal/backendapp/...
cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/task/task-archive-confirm-dialog.test.tsx components/settings/archive-confirmation-settings.test.tsx
cd apps/web && pnpm e2e:run tests/task/archive-confirmation-preference.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-confirmation-preference.spec.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Risks

- React Strict Mode can replay effects, so the confirmation bypass must guard one archive execution per open transition.
- Existing user rows omit the new boolean; both backend JSON decoding and frontend fallback must fail safely to confirmation enabled.
