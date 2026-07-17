---
id: "01-mobile-kanban-hierarchy"
title: "Mobile Kanban hierarchy"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 01: Mobile Kanban hierarchy

## Acceptance

- One always-visible control names current workflow and step; its drawer reaches workflow and step choices with 44px targets.
- Choosing a workflow updates the existing active workflow selection so board actions and task creation match the visible workflow.
- Mobile renders Kanban and offers only Kanban/List even when saved mode is Pipeline; saved mode is not overwritten.
- Home menu is an inset, safe-area-aware bottom card; desktop workflow filters and Pipeline are unchanged.

## Files likely touched

- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/lib/kanban/view-registry.ts`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban/mobile-column-tabs.tsx`
- `apps/web/components/kanban/mobile-workflow-picker.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/homepage-commands.tsx`
- `apps/web/e2e/pages/mobile-kanban-page.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`

## Inputs

- Spec `What`: workflow hierarchy, Pipeline exclusion, Home menu surface.
- Existing `SwimlaneContainer` focus fallback and `MobileColumnTabs` step/Embla synchronization.

## Verification

```bash
cd apps/web && NODENV_VERSION=24.12.0 pnpm run typecheck
NODENV_VERSION=24.12.0 make build-web
cd apps/web && NODENV_VERSION=24.12.0 pnpm e2e:run tests/kanban/mobile-kanban.spec.ts
```

## Output contract

Report behavior, tests, files, blockers/risks; mark task and plan item done.
