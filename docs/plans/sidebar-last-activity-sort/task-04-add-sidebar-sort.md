---
id: "04-add-sidebar-sort"
title: "Add the sidebar activity sort"
status: completed
wave: 3
depends_on: ["03-project-live-task-activity"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-last-activity-sort.md"
---

# Task 04: Add the sidebar activity sort

Expose `lastActivityAt` through the shared saved-view picker, comparator, row
time, portable settings, and required locales.

## Acceptance

- Desktop and mobile view editors offer localized **Last activity** with both
  directions. The option updates the live order and row time.
- Saved views and drafts retain `lastActivityAt` through wire conversion,
  migration, hydration, settings events, sync failure rollback, and reload.
- Existing **Updated** views and new-view defaults remain unchanged. Missing
  activity uses task update, then creation, as fallback.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/apply-view.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts components/task/task-session-sidebar-item.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/components/task/sidebar-filter/sort-picker.tsx`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar-item.test.ts`
- `apps/web/components/task/task-item-stats-row.tsx`
- `apps/web/lib/state/slices/ui/ui-slice-migration.test.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-wire.test.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 03.

## Risks

- Do not translate or rename the persisted key.
- Do not call `t()` at module scope in the option registry.
- The effective draft controls both sorting and displayed row time.

## Result

- Added the `lastActivityAt` sort key, migration allowlist entry, shared
  comparator with update/creation fallbacks, and wire round-trip coverage.
- Threaded `last_activity_at` from summary payloads into desktop and mobile
  task rows. Relative time uses activity only when the effective view selects
  the new sort; existing Updated behavior and defaults remain unchanged.
- Added the localized label to English, Portuguese, Simplified Chinese,
  Traditional Chinese, Hong Kong Chinese, and pseudo catalogs.
- Verification passed:
  - focused Vitest suite: 119 tests
  - `pnpm run typecheck`
  - `pnpm --filter @kandev/web lint`
  - `pnpm run i18n:zh-hant`, `i18n:check`, and `i18n:ratchet`
