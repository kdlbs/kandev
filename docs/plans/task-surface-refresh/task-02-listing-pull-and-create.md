---
id: "02-listing-pull-and-create"
title: "Listing pull refresh and mobile create"
status: done
wave: 2
depends_on: ["01-foreground-recovery"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-surface-refresh.md"
---

# Task 02: Listing pull refresh and mobile create

## Acceptance

- Kanban/Home and List use authoritative guarded callbacks for foreground and
  manual refresh while retaining rendered data on failure.
- Mobile List and Kanban refresh only after an eligible downward pull from the
  active vertical scroll owner's top; horizontal, multi-touch, mid-scroll, and
  below-threshold gestures do not refresh.
- Mobile List renders the shipped safe-area-aware FAB, opens the existing
  create dialog in current context, and refreshes the current List after a
  matching task is created.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts lib/mobile/pull-to-refresh.test.ts
```

## Files likely touched

- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts`
- `apps/web/lib/mobile/pull-to-refresh.ts`
- `apps/web/lib/mobile/pull-to-refresh.test.ts`
- `apps/web/components/mobile/pull-to-refresh.tsx`
- `apps/web/components/kanban-board.tsx`
- `apps/web/components/kanban/mobile-fab.tsx`
- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/app/tasks/tasks-list-view.tsx`

## Dependencies

Task 01.

## Parallelism

Sequential. Kanban and List share the new refresh and pull contracts.

## Inputs

- Spec: mobile create, pull, Kanban/List foreground behavior, race/failure
  requirements.
- Mobile design contract in `plan.md`.
- Existing request-generation guards in List and multi-workflow snapshot
  fetches.
- Existing Kanban FAB and `TaskCreateDialog`.

## Risks

- Gesture arbitration with Embla and task DnD.
- Active `kanban` and `kanbanMulti` state diverging after a forced refresh.
- Explicit/manual error feedback accidentally appearing for silent foreground
  refresh.

## Output contract

Report RED failures, changed files, exact verification result, rendered mobile
check status, residual risks, and update this task plus `plan.md` status in the
primary conversation.
