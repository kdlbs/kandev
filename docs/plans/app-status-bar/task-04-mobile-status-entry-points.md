---
id: app-status-bar-04
title: Add mobile Status entry points
status: done
wave: 3
depends_on: [app-status-bar-03]
plan: docs/plans/app-status-bar/plan.md
---

# Add mobile Status entry points

## Inputs

[Spec: responsive/layout](../../specs/ui/app-status-bar.md#responsive-and-layout-contract); mobile UI language; task 03 provider trigger.

## Files

- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/page-topbar.tsx`
- `apps/web/app/office/components/office-topbar.tsx`
- Focused tests beside each changed surface.

## Acceptance

1. Home, task, Settings, standard PageTopbar, and Office expose Status through native mobile controls; existing task navigation height and terminal offsets stay intact.
2. Menus close before drawer opens; triggers have accessible labels and at least 44 px active target dimension.
3. Normal routes have one Status drawer owner; full-bleed plugin-page guidance remains explicit rather than forcing host chrome.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/kanban components/task/mobile components/settings/settings-layout-client.test.tsx components/page-topbar.test.tsx
```

## Output contract

Report changed entry points, close/open and focus behavior, rendered mobile check, tests, blockers, and task status.
