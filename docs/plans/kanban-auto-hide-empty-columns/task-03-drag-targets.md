---
id: "03-drag-targets"
title: "Restore auto-hidden steps as move targets"
status: done
wave: 3
depends_on: ["02-derived-visibility"]
plan: "plan.md"
spec: "../../specs/ui/requirements/kanban-auto-hide-empty-columns.md"
---

# Task 03: Restore auto-hidden steps as move targets

## Acceptance

- Starting a pointer or touch drag reveals every auto-hidden, non-manually-hidden live step as an
  accessible droppable target with a unique real step id.
- Cancelling or failing the drag removes temporary targets; a successful authoritative move keeps
  the occupied destination visible.
- Bulk Move, Pipeline move controls, and phone move targets include auto-hidden steps and continue
  excluding manually hidden steps.
- A Pipeline arrow shows the destination name in a tooltip only when that adjacent destination is
  auto-hidden between visible stages.

## Likely files

- `apps/web/components/kanban/swimlane-kanban-content.tsx` and focused tests
- `apps/web/components/kanban/swimlane-graph2-content.tsx` and focused tests
- `apps/web/components/kanban/mobile-drop-targets.tsx` and tests
- `apps/web/components/kanban-board.tsx` and tests
- shared Kanban presentation helper extracted during Task 02

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/kanban-board.test.ts components/kanban/mobile-drop-targets.test.tsx components/kanban/swimlane-kanban-content.test.tsx components/kanban/swimlane-graph-content.test.tsx components/kanban/swimlane-graph2-orphan-display.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:ratchet)
```

## Risks

- Ghost targets and real columns must never mount the same droppable id simultaneously.
- DnD completion is asynchronous; local drag state cannot be treated as authoritative task state.
- Pipeline uses explicit move controls rather than DnD and must stay compact while retaining hidden
  destinations in its movement model.
