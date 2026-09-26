---
id: "01-disable-default"
title: "Centralize portable user-setting defaults"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/unread-divider.md"
related_specs:
  - "../../specs/ui/requirements/transcript-navigation-settings.md"
---

# Task 01: Centralize portable user-setting defaults

## Acceptance

- Missing `unread_divider` and `show_transcript_auto_scroll_control` values
  resolve to `false`; explicit stored values remain unchanged.
- The backend has one default constructor and the frontend has one default-state
  factory plus one wire-to-store mapper.
- Boot/HTTP, WebSocket, and settings-save responses use the common mapper.
- Shared E2E setup remains a deliberate test baseline, and behavior specs opt
  into the preferences they exercise.
- Backend/frontend contract tests prove missing-value defaults; desktop/mobile
  settings tests continue to prove explicit-value persistence.

## Files likely touched

- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/components/settings/editors-settings-state.tsx`
- `apps/web/e2e/tests/task/unread-divider-preference.spec.ts`
- `apps/web/e2e/tests/task/mobile-unread-divider-preference.spec.ts`
- `apps/web/e2e/tests/chat/unread-divider.spec.ts`
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts`

## TDD sequence

1. Add a live-update regression proving omitted fields preserve the current
   normalized value and run it red.
2. Consolidate backend defaults and frontend mapping while keeping focused
   default and explicit-value tests green.
3. Remove collateral fixture/spec changes and keep feature preconditions
   explicit in behavior tests.

## Verification

- `cd apps/backend && go test ./internal/user/store -run 'TestScanUserSettingsUnreadDividerDefault'`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/users.test.ts`
- `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-user-display-settings.test.ts`
- `cd apps/web && pnpm e2e:run tests/task/unread-divider-preference.spec.ts -- --project=chromium --project=mobile-chrome`
- `cd apps/web && pnpm e2e:run tests/chat/unread-divider.spec.ts tests/chat/mobile-unread-divider.spec.ts -- --project=chromium --project=mobile-chrome`

## Results

- Backend focused defaults: 7 tests passed.
- Frontend mapper, live-update, carry-forward, and settings components: 48 tests passed.
- Frontend typecheck and targeted ESLint: passed.
- Managed production-build Playwright run: 7 desktop/mobile tests passed.
- Public docs validation: 58 validator tests passed; 41 published pages validated.

## Parallelism

Sequential. The backend and frontend defaults share one persisted contract and
the E2E tests depend on both layers.
