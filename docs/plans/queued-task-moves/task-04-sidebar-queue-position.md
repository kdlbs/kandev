---
id: "04-sidebar-queue-position"
title: "Show sidebar queue position"
status: completed
wave: 4
depends_on: ["03-kanban-queue-sections"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 04: Show Sidebar Queue Position

## Acceptance

- `useWorkspaceSidebarTasks` derives each destination-resident queued task's
  position, total queue size, and destination title from workspace workflow
  snapshots using the shared queue helper.
- `TaskSwitcherItem` carries WIP queue status in a field distinct from the
  existing `queuedCount`, which remains the pending-agent-prompt count.
- Desktop and mobile sidebar mappers pass the same queue object to shared
  `TaskItem` rendering for tasks and subtasks.
- All pointer presentations show one compact localized queue icon whose tooltip
  states `Position N of M in STEP queue`. Its focusable trigger supports hover,
  keyboard focus, and touch focus, so the position does not depend on hover.
- The icon is accessible status information, remains shrink-safe, and does not
  create horizontal overflow or a second scroll owner.
- Live task updates and queue promotions update or remove the chip without a
  reload.
- Unit/component tests prove mapping, ordering, render/hide behavior, icon and
  tooltip accessibility, subtasks, and separation from the queued-prompt badge.

## TDD Sequence

1. Add hook, mapper, and `TaskItem` tests for WIP queue status. Run RED.
2. Add the distinct item type and derive queue data once in the workspace hook.
3. Render the responsive localized chip in shared task rows.
4. Run focused tests and frontend gates GREEN.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  lib/kanban/wip-queue.test.ts \
  hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts \
  components/task/task-session-sidebar-item.test.ts \
  components/task/mobile/session-task-switcher-sheet-hooks.test.ts \
  components/task/task-item.test.tsx
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Implementation Result

- `useWorkspaceSidebarTasks` derives WIP queue positions from the shared
  helper and carries them separately from the existing queued-agent-prompt
  count.
- Desktop and mobile task switchers share the same queue status mapping and
  focusable queue icon. The icon tooltip states `Position N of M in STEP queue`.
- Focused mapper and component tests passed with the exact 5-file, 71-test
  Vitest run above. The queue icon disappears after promotion and preserves
  mobile width constraints in managed E2E coverage.
- The SSR Kanban boot payload includes WIP limits and task admission metadata,
  so the initial column count does not wait for a later snapshot refresh.

## Files Likely Touched

- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item-stats-row.tsx`
- related focused test files
- `apps/web/src/locales/en/sidebar.json`
- `apps/web/src/locales/pseudo/sidebar.json`
- `apps/web/src/locales/pt-pt/sidebar.json`
- `apps/web/src/locales/zh-cn/sidebar.json`

## Dependencies

Task 03 supplies the shared queue-order helper and Kanban terminology.

## Parallelism

`sequential`

## Output Contract

Record RED/GREEN evidence, the distinct queue type, fine/coarse-pointer
behavior, live-update behavior, i18n and type/lint results, files changed, and
exact command results. Update this task and `plan.md` status in the same
implementation conversation.
