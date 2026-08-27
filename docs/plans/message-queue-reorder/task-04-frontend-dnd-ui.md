---
id: "04-frontend-dnd-ui"
title: "Frontend drag-and-drop queue UI"
status: done
wave: 4
depends_on: ["03-frontend-api-hook-reorder"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-reorder.md"
---

# Task 04: Frontend drag-and-drop queue UI

## Acceptance

1. Queue panel rows are `@dnd-kit/sortable`: dragging the handle reorders
   rows live (transform/transition) and the drop calls
   `handleReorder(arrayMove(...))`; dragging from the row body never starts a
   drag; rows render at compact `#1..#N` positions after reorder.
2. The dotted grab handle is a floating overlay on the row's left edge
   (absolutely positioned, no layout shift): hidden until row hover or
   keyboard focus on fine pointers; always visible with a ≥44px hit area on
   coarse pointers; hidden while the row is editing; disabled while
   `isLoading`/`cancellationPending`; localized `aria-label` +
   `aria-roledescription`; keyboard sensor reorders via Space + arrows.
3. Component tests cover handle visibility transitions, disabled states,
   edit suppression, and drag-end → `reorderEntries` wiring; i18n keys added
   to `en/chat.json` + regenerated pseudo pass `i18n:check` and
   `i18n:ratchet`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm i18n:pseudo && pnpm i18n:check && pnpm i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/chat/queued-ghost-list.tsx` — `DndContext` + `SortableContext` + sensors (`PointerSensor` distance 4, `KeyboardSensor` + `sortableKeyboardCoordinates`), `activeId` state, `handleDragEnd` with `arrayMove`, thread `canDrag`/`isDragging`/`onReorder` props through `QueuePanelDisclosure`/`QueuePanel`.
- `apps/web/components/task/chat/queued-ghost-message.tsx` — `useSortable` on the row wrapper (`setNodeRef`, `CSS.Transform`, `group relative`, `z-10 opacity-40` while dragging), `QueueGrabHandle` (absolute left-edge overlay, 2×3 dot grid, `touch-none`, coarse-pointer always-visible variant, `t("chat:reorderQueuedMessage")` / `t("chat:sortable")`), `canDrag`/`isDragging` props.
- `apps/web/components/task/chat/queued-ghost-list.test.tsx` — drag wiring + visibility/disabled tests.
- `apps/web/components/task/chat/queued-ghost-message.test.tsx` — handle behavior tests.
- `apps/web/src/locales/en/chat.json` + `apps/web/src/locales/pseudo/chat.json` — keys `reorderQueuedMessage`, `sortable`, `failedToReorderQueuedMessages`.

## Dependencies

Task 03 (`reorderEntries`/`handleReorder` in the hook).

## Parallelism

Sequential.

## Inputs

- Spec: `## What`, `## Responsive and Mobile Behavior`, `## Scenarios` (handle visibility, body-drag no-op, edit suppression, keyboard).
- Plan: `### Queue panel`, `### Row`, `### i18n`, `### Tests`.
- Mobile-parity contract (from skill): entry point = existing queue panel; nearest shipped drag exemplar = kanban `swimlane-container.tsx` (`useSortable` + handle listeners + `touch-none`); presentation = inline panel (unchanged); scroll owner = `queue-scroll-region`; touch targets ≥44px on coarse pointers; shared hook/state across viewports.
- Existing patterns: `swimlane-container.tsx`, `kanban-card-content.tsx` (`DraggableAttributes`/`listeners` typing), coarse-pointer classes like `[@media(pointer:coarse)]:h-11`.

## Output contract

Summary, files changed, exact test/lint commands and outcomes, blockers, risks; update task + plan statuses in the same conversation.

## Results

- `cd apps && pnpm --filter @kandev/web test -- components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-message.test.tsx hooks/domains/session/use-queue.test.ts lib/api/domains/queue-api.test.ts` → 114 passed.
- `cd apps/web && pnpm exec eslint --max-warnings 0 <changed files>` → 0 warnings.
- `cd apps/web && pnpm i18n:pseudo && pnpm i18n:check && pnpm i18n:ratchet` → all pass.
- `cd apps/web && pnpm run typecheck` → exit 0 (run with `NODE_OPTIONS=--max-old-space-size=12288`; default heap OOMs on this box, environmental).
- Files: `components/task/chat/queued-ghost-list.tsx` (`useQueueReorder`, DndContext/SortableContext wiring), `components/task/chat/queued-ghost-message.tsx` (`SortableRowShell`, `QueueGrabHandle`), `src/locales/en/chat.json` + regenerated pseudo (`reorderQueuedMessage`, `sortable`, `failedToReorderQueuedMessages`), both component test files. Drag simulation in happy-dom requires `isPrimary` patching (happy-dom omits the primary-pointer computation) and a second pointermove to dispatch DragMove — documented in `simulateReorderDrag`.
