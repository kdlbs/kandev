---
id: app-status-bar-03
title: Mount status surface in application shell
status: done
wave: 2
depends_on: [app-status-bar-01, app-status-bar-02]
plan: docs/plans/app-status-bar/plan.md
---

# Mount status surface in application shell

## Inputs

[Spec: responsive/layout](../../specs/ui/app-status-bar.md#responsive-and-layout-contract); tasks 01–02; `AppShell`; legacy layout; current route-height patterns.

## Files

- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/components/app-status-bar/app-status-bar.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.tsx`
- `apps/web/components/app-status-bar/app-status-bar.test.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.test.tsx`
- `apps/web/src/app-shell.tsx`, `apps/web/app/layout.tsx`, `apps/web/app/globals.css`
- `apps/web/src/task-detail-route.tsx`, `apps/web/app/stats/stats-page-client.tsx`, `apps/web/app/stats/loading.tsx`, `apps/web/components/plugin-page.tsx`
- `apps/web/components/kanban/kanban-board.tsx`, `apps/web/components/task/task-page-content.tsx`, `apps/web/components/task/task-page-inner.tsx`
- `apps/web/app/gitlab/gitlab-page-client.tsx`, `apps/web/app/github/github-page-client.tsx`, `apps/web/app/jira/jira-page-client.tsx`, `apps/web/app/tasks/[id]/kanban-task-shell.tsx`, `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/components/kanban/kanban-header.tsx`, `apps/web/components/kanban/kanban-header-mobile.tsx`, `apps/web/components/kanban/kanban-header-mobile.test.tsx`, `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/settings/system-metrics-settings-card.tsx`

## Acceptance

1. Shell is `h-dvh` column; sidebar/route row owns remaining height; desktop/tablet status bar is exactly 24 px and in flow.
2. Provider mounts bar or drawer, never both; built-ins and left/right plugin wrappers receive active route/workspace/task/session context.
3. Header metric mounts are removed, setting copy says Status bar, and shell-owned route roots use parent height/local overflow.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar components/kanban/kanban-header-mobile.test.tsx
cd apps && pnpm --filter @kandev/web typecheck
```

## Output contract

Report height/scroll ownership changes, removed metric mounts, tests, visual-check result, blockers, and task status.
