---
id: "01-device-view-and-workflow-fix"
title: "Device-local view and workflow fix"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-listing-display-preferences.md"
role: implementer
model_tier: balanced
---

# Task 01: Device-local view and workflow fix

## Acceptance

- **All Workflows** remains `null` with exactly one visible workflow and has a
  failing-then-passing unit regression.
- Kanban/Pipeline/List changes write the versioned device-local preference and
  no longer PATCH the backend view mode.
- Home restores List or the desktop Kanban/Pipeline preference; phone falls
  back from Pipeline to Kanban without changing stored Pipeline.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run lib/kanban/resolve-workflow.test.ts lib/task-listing/view-preference.test.ts`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/lib/kanban/resolve-workflow.ts`
- `apps/web/lib/kanban/resolve-workflow.test.ts`
- `apps/web/lib/task-listing/view-preference.ts`
- `apps/web/lib/task-listing/view-preference.test.ts`
- `apps/web/hooks/use-task-listing-view.ts`
- `apps/web/hooks/use-kanban-display-settings.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/app/page-client.tsx`
- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/app/page.tsx`
- `apps/web/app/tasks/page.tsx`
- `apps/web/src/spa-routes.tsx`

## Inputs

- Spec: **What**, **Device-local view preference**, and device-view scenarios.
- Plan: **Root cause**, **Workflow-filter correction**, **Device-local
  task-listing view**, and **Mobile design contract**.
- Existing pattern: `apps/web/hooks/use-global-view-mode.ts`.

## Output contract

Return a compact handoff with intent/acceptance, base/head SHA if committed,
changed files and entry points, spec sections, `localized + user-flow +
persistence` risk tags, exact command results, uncertainties, and the task-file
status update. Do not edit `plan.md`.
