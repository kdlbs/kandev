---
id: "02-sidebar-filter-e2e"
title: "Prove sidebar filter behavior"
status: done
wave: 2
depends_on: ["01-retire-archived-filter"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-archived-filter.md"
---

# Task 02: Prove sidebar filter behavior

Add focused browser regressions showing that the shared desktop and mobile
sidebar filter editor no longer offers an option its data source cannot
satisfy.

## Acceptance

- Desktop Playwright coverage proves **Archived** is absent from the dimension
  selector and a supported option remains usable.
- Mobile Playwright coverage proves the same behavior from the task-switcher
  sheet and confirms the existing surface remains viewport-contained.
- Both regressions fail against the pre-fix registry and pass after Task 01.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "does not offer archived"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "does not offer archived"
```

If `apps/node_modules` is absent, first run:

```bash
cd apps && pnpm install --frozen-lockfile
```

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`

## Dependencies

Task 01.

## Parallelism

`sequential` — assertions depend on Task 01's registry contract.

## Inputs

- Spec desktop and mobile scenarios.
- Plan **E2E Tests** and **Mobile design contract**.
- `apps/web/e2e/pages/sidebar-filter-popover.ts` for desktop entry points.
- `seedAndOpenSheet` in
  `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts` for the mobile entry
  point.

## Output contract

Report the red and green Playwright commands, discovered test counts, artifact
paths on failure, cleanup/teardown evidence, files changed, blockers or risks,
and update this task plus `plan.md` status/results in the same primary
conversation.

## Results

- RED: both scenarios failed against the pre-fix registry because **Archived**
  was still present in the shared dimension selector.
- GREEN: `cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "does not offer archived"` — 1 Chromium test passed via the managed production runner, which built backend/Vite artifacts and cleaned its temporary E2E directory.
- GREEN: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "does not offer archived"` — 1 Pixel 5/mobile-chrome test passed via the managed production runner, which built backend/Vite artifacts and cleaned its temporary E2E directory.
- No failure artifacts or temporary test files remain.
