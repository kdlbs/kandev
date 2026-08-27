---
id: "09-native-link-review-surfaces"
title: "Native Link and review surfaces"
status: completed
wave: 2
depends_on: ["01-design-package", "04-frontend-plugin-registry"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 09: Native Link and review surfaces

## Intent

Dispatch Link actions and review rendering through plugin registry providers while
keeping GitHub/GitLab compatibility and desktop/mobile feature parity.

## Owned paths

- `apps/web/components/kanban-card-menu-items.tsx`
- `apps/web/components/kanban-card-menu-items.test.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-session-sidebar-link-actions.ts`
- `apps/web/components/task/review-panel-provider.ts`
- `apps/web/components/task/review-panel-provider.test.ts`
- `apps/web/components/task/review-detail-panel.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`
- `apps/web/lib/state/dockview-panel-actions.ts`
- `apps/web/lib/state/dockview-panel-actions.test.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-store.test.ts`
- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`

## Dependencies

Tasks 01 and 04.

## Acceptance

1. `registerTaskAction` renders Link-group actions—including **Link → Bitbucket Pull
   Request**—in existing desktop and visible mobile menus, closes before invocation,
   and passes read-only current context.
2. Registered reviews use normalized items and external-store refresh in selectors,
   dock/detail panels, task center, auto-open/close, task switching, and mobile
   navigation; plugin `ReviewPanel` renders in those native locations.
3. Provider-neutral panel IDs/params preserve saved-layout aliases `pr-detail`,
   `mr-detail`, `prKey`, and `mrKey`; unload removes plugin UI safely.

## Verification

```sh
cd apps/web && pnpm test -- components/kanban-card-menu-items.test.tsx components/task/review-panel-provider.test.ts components/task/dockview-session-tabs.test.ts lib/state/dockview-panel-actions.test.ts lib/state/dockview-store.test.ts components/task/mobile/session-mobile-layout.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Risks

Link/menu placement and mobile discoverability are product contracts. Do not replace
native panels with a standalone route or remove legacy layout restoration.
