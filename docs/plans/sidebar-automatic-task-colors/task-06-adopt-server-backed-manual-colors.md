---
id: "06-adopt-server-backed-manual-colors"
title: "Adopt server-backed manual colors"
status: done
wave: 6
depends_on:
  - "05-persist-personal-manual-colors"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.7
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.8
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.9
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-005.10
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
---

# Task 06: Adopt Server-Backed Manual Colors

## Summary

Make backend settings the only display source for manual colors. Import valid legacy browser colors with missing-only patches, then remove the legacy value.

## In scope

- A settings-backed `useTaskColor` selector and optimistic `useSetTaskColor` mutation.
- Safe rollback for failed writes and revision guards for delayed responses.
- A startup migration after user settings load.
- Bounded legacy parsing and import batches.
- Legacy-key removal only after complete success.
- Localized write errors.
- Desktop and mobile browser coverage without layout changes.

## Out of scope

- Real-time cross-browser delivery beyond existing settings events.
- Changes to color menus, marker geometry, touch targets, or drawer composition.
- Shared task colors.

## Acceptance

- Fresh clients never persist manual colors in browser storage.
- Legacy colors import only into missing task IDs. Stored colors and tombstones win conflicts.
- Desktop and mobile show the backend value after reload. Failed writes restore the latest confirmed value.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/task-colors.test.ts hooks/use-task-color.test.tsx hooks/use-task-color-migration.test.tsx components/task/task-item.test.tsx components/task/task-switcher-context-menu.test.tsx)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/sidebar-task-color-sync.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-color-sync.spec.ts)
```

## Files likely touched

- `apps/web/lib/task-colors.ts`
- `apps/web/hooks/use-task-color.ts`
- `apps/web/hooks/use-task-color.test.tsx`
- `apps/web/hooks/use-task-color-migration.ts`
- `apps/web/hooks/use-task-color-migration.test.tsx`
- `apps/web/hooks/use-ensure-user-settings.ts`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-switcher-color-menu.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/task/sidebar-task-color-sync.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-color-sync.spec.ts`

## Dependencies

Task 05 supplies the stored map, patch semantics, wire types, and common settings mapping.

## Risks

- A migration that starts before settings load can treat an unknown server value as missing.
- Removing the browser key before all batches succeed can lose colors that were not imported.
- A delayed HTTP response can replace a newer settings event without revision guards.

## Parallelism

`sequential`

## Inputs

- Requirement sections for portable manual colors and migration behavior.
- System-design sections for legacy migration, failure behavior, and responsive behavior.
- Existing desktop and phone task-color menus.

## Results

Implemented the settings-backed manual-color selector and optimistic patch mutation with serialized writes, revision guards, latest-confirmed rollback, and localized errors. Added load-gated legacy import in bounded missing-only batches, safe parsing, success-only legacy-key removal, and desktop/mobile coverage without changing the existing menu or drawer layout.

Verification: the focused Task 06 suite passed 76 tests; `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:zh-hant`, and `pnpm run i18n:check` passed; desktop and mobile Playwright sync specs each passed one test.
