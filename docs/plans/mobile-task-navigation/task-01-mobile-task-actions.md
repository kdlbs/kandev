---
id: "01-mobile-task-actions"
title: "Mobile task actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 01: Mobile task actions

## Acceptance

- Task actions are visibly reachable with a 44px touch target below the app's 640px mobile breakpoint without covering diff stats.
- Same-workflow Move to works from the mobile Dockview task switcher even without an injected move callback.
- Shared context/dropdown menus use bounded bottom-sheet geometry on mobile while desktop remains unchanged.

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/app/globals.css`
- mobile task-action Playwright specs

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-external-link-menu.spec.ts tests/task/mobile-archive-confirmation-preference.spec.ts
```

## Inputs

- Spec `What` and first three scenarios.
- Existing desktop move path in `task-session-sidebar-switcher-props.ts`.
- Existing mobile task switcher in `session-task-switcher-sheet.tsx`.

## Output contract

Report behavior, files changed, tests run, blockers/risks, and set this task plus linked plan item to done.
