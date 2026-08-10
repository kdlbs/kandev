---
id: "03-frontend-data-plumbing"
title: "Frontend interrupted-field plumbing"
status: done
wave: 3
depends_on: ["02-backend-dto-and-clearing"]
plan: "plan.md"
spec: "../../specs/tasks/interrupted-task-indicator.md"
---

# Task 03: Frontend interrupted-field plumbing

## Acceptance

- `Task.interrupted?: boolean` exists in `apps/web/lib/types/http.ts`.
- `toKanbanTask` maps `interrupted` from API payloads; the kanban task row type
  carries it.
- `mergeTaskUpdate` in the WS `task.updated` handler preserves an existing
  `interrupted` value when the payload omits the field (a lightweight update
  must not wipe a set marker).
- `TaskSwitcherItem` carries `interrupted`; `buildSidebarItem` and the mobile
  task-switcher sheet hooks map it from the kanban task.
- `TaskRow` passes `interrupted` into `TaskItem` (the icon itself is Task 04).

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test -- lib/kanban/map-task.test.ts lib/ws/handlers/tasks.test.ts components/task/task-session-sidebar-item.test.ts 2>/dev/null || pnpm --filter @kandev/web test -- lib/kanban lib/ws/handlers
```

(Adjust the file list to the actual test locations; run the focused tests for
the files you touched.)

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- Matching `*.test.ts`/`*.test.tsx` for each mapping and merge change.

## Dependencies

Task 02 (the DTO field must exist so payloads carry it).

## Inputs

- Spec: `API surface`, `Failure modes` (client merge wipe), scenario 7.
- Plan: `Frontend > Data plumbing`.
- Existing pattern: the `foreground_activity` preserve guard in
  `mergeTaskUpdate` and the `foregroundActivity` mapping in `toKanbanTask`.

## Risks

- Forgetting one of the two sidebar item builders (desktop
  `buildSidebarItem` vs the mobile sheet hook) leaves a surface without the
  field — both must map it.
- A partial `task.updated` merge that overwrites `interrupted` with
  `undefined` wipes the marker client-side — the preserve guard must mirror the
  `foreground_activity` semantics exactly (payload-omits-field → keep existing;
  payload `null`/`false` → clear).
- Do not translate or rename the field; it is a typed boolean used in logic.

## Output contract

Report every file changed, the merge-preserve test case, typecheck and focused
test results, then mark this task `done` and update `plan.md`.
