---
id: "03-storage-temp-artifact-ui"
title: "Storage temporary-artifact action"
status: done
wave: 3
depends_on: ["02-temp-artifact-provider-api"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 03: Storage temporary-artifact action

Add the new analysis row and explicit cleanup action to the existing Storage page without adding a
second settings surface or a scheduled policy toggle.

## Acceptance

- Desktop Storage analysis renders the localized temporary-artifact row with bytes, stale/active
  counts, warnings, unavailable state, and a clearly scoped cleanup action.
- The action uses the existing Storage controller and a reversible confirmation, submits exactly
  `resources: ["temporary_artifacts"]`, preserves busy/Run-anyway behavior, and leaves Go-cache and
  global actions unchanged.
- The same composition is usable on mobile with wrapped content, at least 44px touch targets, and
  no new horizontal overflow; all new copy exists in the four system locale catalogs.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web test -- storage-overview-card storage-confirmation-dialogs use-storage-maintenance system-api
pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-confirmation-dialogs.tsx`
- `apps/web/components/settings/system/storage/storage-confirmation-dialogs.test.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-totals.ts`
- `apps/web/components/settings/system/storage/storage-totals.test.ts`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pseudo/system.json`
- `apps/web/src/locales/pt-pt/system.json`
- `apps/web/src/locales/zh-cn/system.json`

## Dependencies

Task 02.

## Parallelism

`sequential`; the overview card, controller, and locale catalogs are shared frontend surfaces.

## Inputs

- Plan Frontend sections and the mobile-parity contract.
- Existing Go-cache resource-specific action and confirmation/dialog patterns.
- `apps/web/AGENTS.md` i18n and settings-save rules.

## Output contract

Report the rendered states, confirmation/resource payload, locale files, focused typecheck/test/lint
results, mobile layout evidence, and synchronized task/plan status.

## Results

- Added localized desktop and mobile Storage rows for Kandev-owned temporary artifacts, including
  measured/stale/protected counts, warnings, unavailable state, and explicit cleanup action.
- The confirmation uses the existing controller with exactly
  `resources: ["temporary_artifacts"]`; scheduled and unscoped cleanup behavior remains unchanged.
- Added English, pseudo, Portuguese, and Simplified Chinese catalog entries and 44px-compatible
  action styling.
- Focused frontend verification passed: 71 Vitest tests across six files, web typecheck, web lint,
  `i18n:check`, and `i18n:ratchet`.
