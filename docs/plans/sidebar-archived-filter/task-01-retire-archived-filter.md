---
id: "01-retire-archived-filter"
title: "Retire archived filter contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/sidebar-archived-filter.md"
---

# Task 01: Retire archived filter contract

Remove `archived` from the supported sidebar filter contract and migrate legacy
saved clauses through the existing frontend normalization boundary.

## Acceptance

- The shared dimension registry and `FilterDimension` contract no longer
  expose `archived`.
- Hydrating a legacy saved view removes only its `archived` clause and
  preserves all other view state.
- Archived task rendering and the synthetic current-archived row remain
  supported outside the filter engine.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/filter-dimension-registry.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/ui-slice.test.ts lib/sidebar/apply-view.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/task/sidebar-filter/filter-dimension-registry.ts components/task/sidebar-filter/filter-dimension-registry.test.ts lib/sidebar/apply-view.ts lib/sidebar/apply-view.test.ts lib/state/slices/ui/sidebar-view-types.ts lib/state/slices/ui/ui-slice.ts lib/state/slices/ui/ui-slice.test.ts lib/state/slices/ui/ui-slice-migration.test.ts
```

If `apps/node_modules` is absent, first run:

```bash
cd apps && pnpm install --frozen-lockfile
```

## Files likely touched

- `apps/web/components/task/sidebar-filter/filter-dimension-registry.ts`
- `apps/web/components/task/sidebar-filter/filter-dimension-registry.test.ts`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/slices/ui/ui-slice-migration.test.ts`

## Dependencies

None.

## Parallelism

`sequential` — Task 02 consumes the supported-dimension behavior established
here.

## Inputs

- Spec sections: **What** and **Scenarios**.
- Plan sections: **Root cause**, **Frontend**, and **Tests**.
- Existing `migrateView` removed-dimension behavior in
  `apps/web/lib/state/slices/ui/ui-slice.ts`.
- Existing direct archived-task placeholder in
  `apps/web/components/task/task-session-sidebar-archived-item.ts`.

## Output contract

Report the legacy migration behavior, exact supported-dimension changes, files
changed, commands and results, blockers or risks, and update this task plus
`plan.md` status/results in the same primary conversation.

## Results

- RED: `cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/filter-dimension-registry.test.ts lib/state/slices/ui/ui-slice.test.ts` — failed as expected with 3 assertions because the registry and migration still accepted `archived`.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/filter-dimension-registry.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/ui-slice.test.ts lib/sidebar/apply-view.test.ts` — passed, 4 files / 98 tests.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec eslint components/task/sidebar-filter/filter-dimension-registry.ts components/task/sidebar-filter/filter-dimension-registry.test.ts lib/sidebar/apply-view.ts lib/sidebar/apply-view.test.ts lib/state/slices/ui/sidebar-view-types.ts lib/state/slices/ui/ui-slice.ts lib/state/slices/ui/ui-slice.test.ts lib/state/slices/ui/ui-slice-migration.test.ts` — passed with no warnings or errors.
- `git diff --check` — passed.
- No temporary instances, captures, or generated artifacts were left behind.
