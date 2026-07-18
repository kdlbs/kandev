---
id: "02-frontend-actions"
title: "Frontend detach actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/subtask-detachment.md"
---

# Task 02: Frontend detach actions

## Acceptance

- A single subtask exposes `Detach from parent` in the sidebar context menu and in both task-card menus; root and bulk menus do not.
- Confirmation states that hierarchy changes while shared workspace access remains, and successful submission cannot be duplicated.
- The Office `No parent` picker and mobile task actions invoke the canonical detach endpoint.

## Verification

```bash
cd apps/web && rtk pnpm test -- components/task/task-switcher.test.tsx components/kanban-card-menu-items.test.tsx components/task/simple/components/parent-picker.test.tsx lib/kanban/map-task.test.ts
cd apps/web && rtk pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/api/index.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/hooks/use-detach-task.ts`
- `apps/web/components/task/task-detach-confirm-dialog.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/kanban-card-menu-items.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/task/simple/components/parent-picker.tsx`
- Adjacent component and hook test files.

## Dependencies

None. Implement against the approved HTTP contract; integrated verification depends on Task 01.

## Inputs

- Spec sections: What, API surface, Scenarios.
- Existing menu patterns in `task-switcher-context-menu.tsx` and `kanban-card-menu-items.tsx`.
- Mobile parity rules in `.agents/skills/mobile-parity/SKILL.md`.

## Output contract

Report UI/API changes, tests run, files changed, blockers, mobile risks, and update this task plus `plan.md` to `done` when acceptance passes.
