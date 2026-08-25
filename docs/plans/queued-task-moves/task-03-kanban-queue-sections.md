---
id: "03-kanban-queue-sections"
title: "Add frontend queue guidance"
status: completed
wave: 3
depends_on: ["02-deferred-destination-lifecycle"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 03: Add Frontend Queue Guidance

## Acceptance

- A pure frontend helper identifies and sorts destination-resident queued
  tasks with the backend promotion order and returns one-based positions.
- A limited Kanban column renders admitted cards first and a labeled queued
  area second. The queued label/count is absent when no task is queued.
- The column header continues to show admitted WIP as `admitted/limit`; queued
  cards do not consume the numerator.
- Drag/drop and other move UI treat the backend's queued result as success and
  show the card in the target queue without a capacity error.
- The shared column implementation preserves the focused mobile-column flow,
  one vertical scroll owner, readable queue state, and no horizontal overflow.
- The workflow step settings page identifies `Pull from` as optional automatic
  feeder intake. An info tooltip explains destination-queue priority, feeder
  intake, and queued direct or automatic transitions.
- The help also states that new tasks targeting a full step use the selected
  feeder. The full contract is available through hover or focus on desktop and
  tap or focus on mobile.
- All new copy is localized across English, pseudo, Portuguese, and Simplified
  Chinese catalogs.
- Unit/component tests cover partitioning, ordering, counts, empty queues, and
  desktop/mobile rendering. They also cover the `Pull from` help with and
  without a selected feeder.

## TDD Sequence

1. Add helper and column tests for admitted/queued partition and deterministic
   order. Run RED.
2. Implement the shared helper and column queue section.
3. Add `Pull from` guidance to the existing responsive step editor's info
   tooltip.
4. Update move-result handling only where current UI assumes a WIP conflict.
5. Add locale keys and run frontend gates GREEN.

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

- Added the shared `wip-queue` helper with backend-compatible ordering,
  destination queue positions, and admitted/queued partitioning.
- Kanban columns render admitted cards first, then a localized queued section;
  the header count remains admitted WIP over the configured limit.
- Added localized `Pull from` guidance for feeder and no-feeder configurations
  to the existing info tooltip across English, pseudo, Portuguese, and
  Simplified Chinese catalogs.
- Focused Vitest coverage passed with 5 files and 71 tests using the exact
  command above. Typecheck, lint,
  `i18n:check`, `i18n:ratchet`, and the E2E production build passed.

## Files Likely Touched

- `apps/web/lib/kanban/wip-queue.ts`
- `apps/web/lib/kanban/wip-queue.test.ts`
- `apps/web/lib/kanban/wip-limit.ts`
- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban-column.test.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-wip-controls.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-wip-controls.test.tsx`
- `apps/web/src/locales/en/kanban.json`
- `apps/web/src/locales/pseudo/kanban.json`
- `apps/web/src/locales/pt-pt/kanban.json`
- `apps/web/src/locales/zh-cn/kanban.json`
- `apps/web/src/locales/en/workflows.json`
- `apps/web/src/locales/pseudo/workflows.json`
- `apps/web/src/locales/pt-pt/workflows.json`
- `apps/web/src/locales/zh-cn/workflows.json`

## Dependencies

Task 02 fixes the authoritative backend promotion order and event semantics.

## Parallelism

`sequential`

## Output Contract

Record RED/GREEN evidence, queue helper contract, desktop/mobile rendering
notes, i18n and type/lint results, files changed, and exact command results.
Update this task and `plan.md` status in the same implementation conversation.
