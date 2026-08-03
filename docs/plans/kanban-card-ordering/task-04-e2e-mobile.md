---
id: "04-e2e-mobile"
title: "E2E and mobile parity"
status: done
wave: 4
depends_on: ["03-column-card-ui"]
plan: "plan.md"
spec: "../../specs/ui/kanban-card-ordering.md"
---

# Task 04: E2E and mobile parity

## Acceptance

- Playwright covers same-step admitted reorder persistence (reload).
- Coverage asserts cross-step move still works.
- Phone layout continues to use the shared DnD path (TouchSensor); no separate native app.
- Spec/plan task statuses updated when done.

## Files likely touched

- `apps/web/e2e/tests/kanban/card-reorder.spec.ts`
- `apps/web/e2e/pages/kanban-page.ts` (helpers if needed)
- `docs/plans/kanban-card-ordering/plan.md`
- `docs/specs/ui/kanban-card-ordering.md` (status)

## Dependencies

Task 03.

## Parallelism

sequential

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/kanban/card-reorder.spec.ts
```

## Risks

- Playwright HTML5 DnD vs dnd-kit pointer sensors may need pointer-based drag helpers rather than
  `dragTo` alone.

## Results

```bash
cd apps && pnpm --filter @kandev/web build
cd apps/web && e2e/scripts/run-e2e.sh --no-build --host -- tests/kanban/card-reorder.spec.ts
```

2 passed (reorder persistence + queued-after-admitted / cross-step). Phone uses the same DnD path (`TouchSensor`); no separate native app.
