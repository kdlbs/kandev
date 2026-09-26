---
id: "03-task-surface-e2e"
title: "Task surface browser coverage"
status: pending
wave: 3
depends_on: ["01-foreground-recovery", "02-listing-pull-and-create"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-surface-refresh.md"
---

# Task 03: Task surface browser coverage

## Acceptance

- Mobile Playwright proves List creation, List/Kanban pull recovery, gesture
  rejection, safe-area containment, and no horizontal document overflow.
- Desktop Playwright proves Kanban/Home, List, and task-detail chat recover
  deliberately dropped live updates on a foreground event without reload or
  selection changes.
- Every dropped-frame test asserts the intended WebSocket notification was
  actually suppressed before asserting recovery.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/mobile-task-surface-refresh.spec.ts tests/task/task-surface-foreground-refresh.spec.ts
```

## Files likely touched

- `apps/web/e2e/helpers/ws-drop.ts`
- `apps/web/e2e/pages/mobile-kanban-page.ts`
- `apps/web/e2e/tests/task/mobile-task-surface-refresh.spec.ts`
- `apps/web/e2e/tests/task/task-surface-foreground-refresh.spec.ts`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. These scenarios validate the integrated result.

## Inputs

- Spec scenarios.
- E2E section and mobile design contract in `plan.md`.
- Existing `routeMainWebSocketWithPromptDrop`, task List fixtures, and
  `MobileKanbanPage`.

## Risks

- A live event that was not actually dropped can create a false-positive
  recovery test.
- Production E2E builds are required after frontend changes; do not use stale
  assets.

## Output contract

Report RED failures, production-build E2E result, screenshots or failure
artifacts inspected, residual risks, and update this task plus `plan.md` status
in the primary conversation.
