---
id: "01-virtualize-kanban-columns"
title: "Virtualize Kanban columns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/adaptive-kanban.md"
parallelism: sequential
---

# Task 01: Virtualize Kanban Columns

## Intent

Keep each Kanban column responsive when it contains hundreds of tasks. Mount only the visible task
rows plus a small overscan.

## Acceptance

- A 440-task desktop, tablet, or phone column mounts fewer than 50 card bodies during initial display.
- Scrolling changes the mounted task window and keeps reached card actions available.
- Counts, logical task order, shift selection, WIP queue placement, and drag behavior remain unchanged.

## TDD Sequence

1. Add the desktop, tablet, and phone large-column E2E specifications.
2. Run all three specifications and record the expected mounted-count failure.
3. Add `@tanstack/react-virtual` and implement the virtualized column task list.
4. Run the large-column specifications until all three pass.
5. Run the existing focused Kanban behavior specifications.

## Verification

If `apps/node_modules` is absent, first run:

```bash
cd apps && pnpm install --frozen-lockfile
```

Then run these final checks from `apps/web`:

```bash
pnpm e2e:run tests/kanban/large-column-virtualization.spec.ts tests/kanban/tablet-large-column-virtualization.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/wip-overflow-queue.spec.ts
pnpm e2e:run --project mobile-chrome tests/kanban/mobile-large-column-virtualization.spec.ts tests/kanban/mobile-kanban.spec.ts
pnpm exec eslint components/kanban/swimlane-container.tsx components/kanban/virtualized-column-task-list.tsx e2e/tests/kanban/large-column-virtualization-helpers.ts e2e/tests/kanban/large-column-virtualization.spec.ts e2e/tests/kanban/mobile-large-column-virtualization.spec.ts e2e/tests/kanban/tablet-large-column-virtualization.spec.ts
pnpm run typecheck
```

The first `pnpm e2e:run` builds current production assets. The mobile command can use `--no-build`
only when the assets did not change after the desktop command.

## Files Likely Touched

- `apps/web/package.json`
- `apps/pnpm-lock.yaml`
- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban/adaptive-desktop-kanban.tsx`
- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban/swimlane-section.tsx`
- `apps/web/components/kanban/swipeable-columns.tsx`
- `apps/web/components/kanban/virtualized-column-task-list.tsx`
- `apps/web/e2e/tests/kanban/large-column-virtualization-helpers.ts`
- `apps/web/e2e/tests/kanban/large-column-virtualization.spec.ts`
- `apps/web/e2e/tests/kanban/tablet-large-column-virtualization.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-large-column-virtualization.spec.ts`
- `docs/plans/kanban-large-column-virtualization/plan.md`
- `docs/plans/kanban-large-column-virtualization/task-01-virtualize-kanban-columns.md`

## Dependencies

None.

## Parallelism

Sequential. This task updates a package lock and one shared UI path.

## Inputs

- `docs/specs/ui/requirements/adaptive-kanban.md`, large-column requirements and scenarios.
- `docs/plans/kanban-large-column-virtualization/plan.md`, confirmed root cause and design.
- `apps/web/components/kanban-column.tsx`, current full-list mount and scroll owner.
- `apps/web/components/kanban/swimlane-kanban-content.tsx`, shared drag overlay and responsive layouts.
- `apps/web/components/kanban/swipeable-columns.tsx`, phone column composition.
- TanStack Virtual React documentation for dynamic measurement and stable item keys.

## Output Contract

Report the changed files, RED and GREEN results, exact commands, blockers, and remaining risks. Update
this task status, its Results section, and the plan verification results in the same conversation.

## Results

- Implemented a shared TanStack Virtual task list with stable task keys, dynamic row measurement,
  overscan, full ordered IDs for range selection, and queue-divider placement inside the measured row.
- Bounded Kanban lane flex geometry across desktop, tablet, and phone so the column remains the
  virtualizer's scroll owner without changing non-Kanban swimlane sizing.
- RED: both new large-column tests failed before the implementation with 440 mounted cards.
- GREEN: desktop/tablet large-column plus Kanban/WIP suite passed 9 tests; mobile large-column plus
  mobile Kanban suite passed 28 tests.
- Review remediation added the coarse-pointer tablet large-column regression and tightened the
  mounted-card bound to fewer than 50. It also preserved desktop/tablet workflow sorting and
  collapsed-swimlane sizing while keeping the mobile single-workflow behavior unchanged.
- Focused ESLint, `pnpm run typecheck`, and the pseudo-locale production E2E build passed.
- Fresh desktop and phone screenshots were captured from synthetic 440-task boards, inspected,
  compressed, and retained in the ignored `.pr-assets` directory for PR publication. Disposable
  capture specs were removed.

Remaining risk is the normal virtualization risk already recorded in the plan: dynamic card-height
changes and drag auto-scroll must continue to use measured rows and the existing drag overlay.
