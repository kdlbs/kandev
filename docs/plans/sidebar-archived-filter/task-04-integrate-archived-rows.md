---
id: "04-integrate-archived-rows"
title: "Integrate archived sidebar rows"
status: pending
wave: 3
depends_on: ["03-archived-sidebar-projection"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-archived-filter.md"
---

# Task 04: Integrate archived sidebar rows

Consume the archived projection in the shared row model and make archived
navigation safe on desktop and mobile.

## Acceptance

- Desktop and mobile rows carry full labels plus `isArchived: true`, display
  the archived badge, and deduplicate the existing synthetic current-task row.
- Selecting an archived row navigates directly to archived task detail, closes
  the mobile drawer, and never prepares or launches a session.
- Archived rows cannot enter multi-selection or expose active-only actions;
  delete may remain available. Archived-load failure uses localized copy and a
  reachable Retry action on both surfaces.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-session-sidebar-item.test.ts components/task/task-session-sidebar-selection.test.ts components/task/task-select-helpers.test.ts components/task/task-switcher.test.tsx components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/mobile/session-task-switcher-sheet.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint components/task/task-session-sidebar.tsx components/task/task-session-sidebar-item.ts components/task/task-switcher.tsx components/task/task-switcher-context-menu.tsx components/task/mobile/session-task-switcher-sheet-hooks.ts components/task/mobile/session-task-switcher-sheet.tsx
```

## Files likely touched

- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar-item.test.ts`
- `apps/web/components/task/task-session-sidebar-selection.tsx`
- `apps/web/components/task/task-session-sidebar-selection.test.ts`
- `apps/web/components/task/task-select-helpers.ts`
- `apps/web/components/task/task-select-helpers.test.ts`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher.test.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`
- `apps/web/src/locales/en/sidebar.json`

## Dependencies

Task 03.

## Parallelism

`sequential` — owns both responsive consumers of the projection and their
shared task-row behavior.

## Inputs

- Spec row navigation, synthetic-row dedupe, failure, and mobile scenarios.
- Plan **Desktop and mobile consumption** and **Mobile design contract**.
- Existing `SessionTaskSwitcherSheet`, `TaskSwitcherItem.isArchived`,
  `TaskUnarchiveButton`, and archived detail-route behavior.

## Risks

- The current selection fallback prepares tasks with no loaded session; the
  archived guard must run before that branch.
- Action guards must cover touch/context-menu and modifier-selection paths,
  not only visual edit controls.
- New failure/retry copy must use `sidebar` translations and pass the i18n
  ratchet.

## Output contract

Report the desktop/mobile row mapping, navigation and action guards, localized
failure behavior, rendered verification performed, exact files/commands/results,
blockers, and update this task plus `plan.md` status/results.

## Results

Pending.
