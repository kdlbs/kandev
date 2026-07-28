---
id: "02-frontend-settings-stability"
title: "Frontend settings stability"
status: done
wave: 2
depends_on: ["01-backend-cached-overview"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 02: Frontend settings stability

## Acceptance

- Storage analysis shows the relative age of the exact snapshot returned by the backend.
- The external Go-cache input hydrates from and follows the persisted adopted path without erasing
  an in-progress edit.
- Analysis and cleanup do not disable policy editing; only conflicting settings mutations block
  save, with a specific reason.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/settings/system/storage/storage-overview-card.test.tsx \
  components/settings/system/storage/storage-policy-card.test.tsx \
  components/settings/system/storage/storage-maintenance-settings.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- Storage fixtures in `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-policy-card.tsx`
- `apps/web/components/settings/system/storage/storage-policy-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.test.tsx`

## Dependencies

Task 01 defines the final `analyzed_at` API contract.

## Inputs

- Spec cache, relative timestamp, external Go-cache, and policy-editing scenarios
- Shared `formatRelativeTime` in `apps/web/lib/utils.ts`
- Existing coordinated-save contract in `settings-save-provider.tsx`

## Output contract

Return a compact handoff capsule with acceptance status, changed entry points, focused test results,
risk tags, uncertainties, and set this task to `done`.
