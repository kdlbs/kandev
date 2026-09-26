---
id: "02-frontend-busy-override"
title: "Storage busy feedback and override UI"
status: done
wave: 2
depends_on: ["01-backend-activity-api"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 02: Storage Busy Feedback and Override UI

## Acceptance

- The Storage controller recognizes the typed 409 response, retains the original cleanup resource
  selection, and sends `force: true` only after the user selects **Run anyway**.
- The settings page names every reported active category, warns about disruption, and renders the
  direct override only when the backend says it is available.
- The existing phone one-column action composition presents the warning and an accessible, full-width
  override button without a dialog, hover-only behavior, or horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  hooks/domains/system/use-storage-maintenance.test.tsx \
  components/settings/system/storage/storage-maintenance-settings.test.tsx \
  lib/api/domains/system-api.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.test.tsx`

## Dependencies

Task 01 defines the typed 409 response and the `force` request field.

## Inputs

- Spec: API surface and busy-override scenarios
- Plan: Frontend and Storage page/mobile contract
- Existing `ApiError.body` handling in `apps/web/lib/api/client.ts`

## Output contract

Report acceptance status, UI/API contract changes, focused test results, mobile evidence or blocker,
risks, and update this task plus `plan.md` to `done` in the same conversation.

## Validation Results

- Focused web tests for the hook, Storage settings, and system API: passed (57 tests).
- `cd apps/web && pnpm run typecheck`: passed.
