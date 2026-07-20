---
id: "02-mobile-home-task-interaction"
title: "Mobile Home task interaction"
status: completed
wave: 2
depends_on: ["01-mobile-kanban-hierarchy"]
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 02: Mobile Home task interaction

## Acceptance

- Mobile card-body tap navigates directly to `/t/:id`; no intermediate task sheet remains.
- More options retains full task actions.
- Edit on a started task exposes and saves title while prompt stays locked.

## Files likely touched

- `apps/web/components/kanban-board.tsx`
- `apps/web/components/kanban/mobile-task-sheet.tsx` (delete)
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-helpers.test.ts`
- `apps/web/e2e/pages/mobile-kanban-page.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`

## Inputs

- Spec direct-navigation and started-title scenarios.
- `useKanbanNavigation` already treats mobile card clicks as full navigation.
- `computeIsTaskStarted` remains prompt-lock source of truth.

## Verification

```bash
cd apps && NODENV_VERSION=24.12.0 pnpm --filter @kandev/web test -- --run components/task-create-dialog-helpers.test.ts components/kanban-card-click.test.ts
NODENV_VERSION=24.12.0 make build-web
cd apps/web && NODENV_VERSION=24.12.0 pnpm e2e:run tests/kanban/mobile-kanban.spec.ts
```

## Output contract

Report behavior, tests, files, blockers/risks; mark task and plan item done.
