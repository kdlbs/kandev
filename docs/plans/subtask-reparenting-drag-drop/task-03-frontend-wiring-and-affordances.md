---
id: "03-frontend-wiring-and-affordances"
title: "Frontend wiring and affordances"
status: done
wave: 3
depends_on: ["02-frontend-nest-drop-infrastructure"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 03: Frontend wiring and affordances

## Acceptance

- Desktop sidebar and mobile task switcher sheet wire `onReparentTask` through the existing `useNestTask` hook: a drop on a nest zone calls `updateTask(taskId, { parent_id })` with the task's `workflowId`, applies the optimistic snapshot patch, rolls back + toasts on error, and reconciles via the `task.updated` WS event.
- The nest-zone affordance shows `Nest under <title>` (localized via `t()`), and the drag exposes no nest zone on invalid targets.
- All new user-facing copy is i18n'd; `pnpm run i18n:ratchet` is clean for touched lines.

## Verification

```bash
cd apps/web && pnpm test -- components/task/task-session-sidebar.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm run i18n:ratchet
```

(Adjust test file list to the files that actually exist for these components; if a surface has no test file, add focused tests for the new handler.)

## Files likely touched

- `apps/web/components/task/task-session-sidebar.tsx` — `handleReparentTask(taskId, parentTaskId)` resolving `workflowId` from `displayTasks`, wired to `onReparentTask` via `buildTaskSwitcherProps`.
- `apps/web/components/task/task-session-sidebar-switcher-props.ts` — thread the new prop.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` — same wiring (shared `TaskSwitcher`; touch drag already active).
- `apps/web/src/locales/en/<sidebar-namespace>.json` — new keys (`nestUnder`, aria label); use the namespace already used by the sidebar components.
- `apps/web/components/task/task-switcher-subtask-dnd.tsx` — affordance label rendering (if not already in task 02).

## Dependencies

Task 02 (props/context plumbing) and Task 01 (composite semantics behind `updateTask`).

## Inputs

- Spec sections: What (single API call, optimistic update, no confirmation), Scenarios (invalid target toast, mobile).
- Existing code: `useNestTask` in `apps/web/hooks/use-nest-task.ts` (non-null path already calls `updateTask`), `buildTaskSwitcherProps`, mobile sheet's existing `TaskSwitcher` usage.

## Output contract

Report wiring changes, exact commands and results, files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.

## Results

- `pnpm run typecheck` clean; `pnpm run lint` clean; `pnpm run i18n:ratchet` clean; task-component suites passed (249 files / 2140 tests in the `components/task` run).
- Desktop sidebar and mobile sheet wire `onNestTask` through the existing `useNestTask` hook; workflow resolved from snapshot keys via the new shared helper `taskWorkflowIdFromSnapshots` in `hooks/use-nest-task.ts`; `MobileTaskList` gained `onNestTask` (sheet stays open on drop); new `sidebar:nestUnder` key.
- Files changed: `components/task/task-session-sidebar.tsx`, `task-session-sidebar-switcher-props.ts`, `components/task/mobile/session-task-switcher-sheet.tsx`, `session-task-switcher-sheet-hooks.ts`, `hooks/use-nest-task.ts`.
