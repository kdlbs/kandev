---
id: "02-adaptive-desktop-board"
title: "Adaptive desktop board"
status: done
wave: 2
depends_on: ["01-responsive-browser-contract"]
plan: "plan.md"
spec: "../../specs/ui/requirements/adaptive-kanban.md"
---

# Task 02: Adaptive desktop board

## Acceptance

- Full and compact desktop columns retain the shared readable minimum; when all steps cannot fit,
  the board keeps complete lanes in an internally scrollable, snap-aligned lane window.
- The new desktop wrapper preserves column DnD, task actions, multi-select, orphan display, preview
  response, and internal scroll ownership without changing phone or tablet composition.
- Long parent relationships render as contained, truncating hierarchy metadata while session and
  review statuses continue to wrap independently.

## Files likely touched

- `apps/web/components/kanban/kanban-grid-template.test.ts`
- `apps/web/components/kanban/kanban-grid-template.ts`
- `apps/web/components/kanban/adaptive-desktop-kanban.tsx` (new)
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban-board-grid.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/e2e/tests/layout/compact-desktop-responsive.spec.ts` only if the RED contract itself is
  proven incorrect during GREEN work
- `apps/web/e2e/tests/kanban/kanban-board.spec.ts` only if the RED contract itself is proven incorrect
  during GREEN work

## Inputs

- Completed Task 01 RED browser contract.
- Spec `What`, `Failure modes`, `Persistence guarantees`, and all responsive scenarios.
- Plan `Container fit model`, `Windowed desktop composition`, `Card hierarchy`, and `Mobile design
  contract`.
- Existing `MobileColumnTabs`, `TabletKanbanLayout`, `useKanbanLayout`, and `KanbanHeader`
  measurement patterns.

## Dependencies

Task 01.

## Parallelism

Sequential. It owns the shared Kanban composition files and completes Task 01's RED contract.

## Verification

Follow RED-GREEN-REFACTOR: first expand and run the pure helper tests to observe their expected
assertion failures, implement the minimum behavior, then run these final targeted checks:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/kanban/kanban-grid-template.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/layout/compact-desktop-responsive.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/mobile-kanban.spec.ts
```

## Risks

- Keep `adaptive-desktop-kanban.tsx` below the frontend component size and complexity limits.
- Keep desktop DnD IDs owned only by real lanes; duplicate IDs would make destinations ambiguous.
- Do not persist responsive mode or horizontal scroll position.

## Output contract

Report RED and GREEN evidence, files changed, final command results, blockers, and remaining risks;
mark this task and both plan/spec implementation statuses accurately in the same conversation.
