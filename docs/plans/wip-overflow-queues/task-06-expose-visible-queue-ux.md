---
id: "06-expose-visible-queue-ux"
title: "Expose visible queue UX"
status: completed
wave: 6
depends_on:
  - "05-adapt-integration-watchers"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 06: Expose Visible Queue UX

## Acceptance

- Frontend task types, boot/event conversion, and store updates preserve WIP
  admission and queue metadata.
- Limited columns show `admitted/limit`, while all resident cards stay visible.
- Queued cards display an accessible destination badge on desktop and mobile.
- Creation success feedback distinguishes same-step and feeder overflow and
  names both the requested and actual visible step where useful.
- Workflow settings explain admitted WIP, in-column overflow, feeder
  precedence, and the full-feeder conflict.
- The mobile focused-column view uses the shared card without clipped text,
  hover-only meaning, nested interactive controls, or horizontal page overflow.

## TDD sequence

1. Add failing type/mapper/store tests for queue metadata.
2. Add failing count, badge, and create-feedback component tests.
3. Implement the shared card/count UX and workflow settings copy.
4. Exercise the mobile column exemplar and add focused component/accessibility
   coverage where logic is extractable.
5. Run frontend typecheck and lint for touched files.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  components/kanban-card-content.test.tsx \
  components/kanban-column-wip.test.tsx \
  components/task-create-dialog-submit.test.tsx \
  components/settings/workflow-pipeline-editor-wip-controls.test.tsx
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

If implementation extracts logic under different focused filenames, update the
command to those exact test files before marking the task done. Use the
resource-aware test conventions from ADR 0037 rather than adding broad worker
concurrency.

## Files likely touched

- `apps/web/lib/types/http.ts`
- frontend backend/state task types and mappers
- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-wip-controls.tsx`
- focused frontend tests

## Mobile parity contract

- Nearest exemplar: `apps/web/components/kanban/mobile-column-tabs.tsx`.
- Composition: existing focused-column tabs and shared `KanbanCard`.
- Scroll owner: existing column task list; no new page-level or nested vertical
  scroller.
- Safe area: no new fixed bottom surface.
- Primary action: unchanged existing task-card/create interactions.

## Dependencies

- Task 05 completes the backend contract consumed by UI types and feedback.

## Parallelism

`sequential`

## Output contract

Record screenshots or DOM evidence for desktop and narrow mobile layouts,
accessibility names, count semantics, files changed, and exact test/typecheck/
lint results.
