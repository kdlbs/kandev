---
id: "02-storage-resource-rows"
title: "Database storage resource rows"
status: completed
wave: 2
depends_on: ["01-database-measurements"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.9
system_design:
  - ../../specs/system-page/system-design/storage-database-footprint.md
---

# Task 02: Database storage resource rows

## Summary

Render both resources and incorporate their measured bytes into the existing total, including localized state and scope explanations.

## In scope

- Both expandable rows display sizes, locations, and measurement explanations through existing shared desktop/mobile components.
- Totals add each included measurement once and correctly handle unknown, unavailable, not-applicable, and overlap states.
- All five language catalogs and targeted component fixtures include the new resources.

## Out of scope

New cleanup behavior, retention changes, remote database measurement, and unrelated storage categories.

## Acceptance

- Both expandable rows display sizes, locations, and measurement explanations through existing shared desktop/mobile components.
- Totals add each included measurement once and correctly handle unknown, unavailable, not-applicable, and overlap states.
- All five language catalogs and targeted component fixtures include the new resources.

## Verification

Run from `apps/web`:

For a fresh worktree, first run `rtk pnpm install --frozen-lockfile` from `apps/`.

```bash
rtk pnpm exec vitest run components/settings/system/storage/storage-totals.test.ts components/settings/system/storage/storage-overview-card.test.tsx hooks/domains/system/use-storage-maintenance.test.tsx
rtk pnpm run typecheck
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/storage/storage-overview-resources.ts`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-totals.ts`
- `apps/web/components/settings/system/storage/storage-totals.test.ts`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/hooks/domains/system/use-storage-maintenance.test-fixtures.tsx`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pt-pt/system.json`
- `apps/web/src/locales/zh-cn/system.json`
- `apps/web/src/locales/zh-hk/system.json`
- `apps/web/src/locales/zh-tw/system.json`

## Dependencies

Task 01.

## Risks

Adding required fields can break existing fixtures; optional compatibility fields must still mark unknown measurements as partial.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), active requirement `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002`.
- [System design](../../specs/system-page/system-design/storage-database-footprint.md).
- Existing storage provider, overview cache, resource rows, and mobile storage tests.

## Results

Implemented localized database and backup rows, exact-once totals, partial and
status handling, scope explanation, and compatibility fixtures. The focused web
tests passed 45 tests, followed by typecheck, lint, and all i18n gates.
