---
id: "02-restore-archived-filter"
title: "Restore archived filter contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-archived-filter.md"
---

# Task 02: Restore archived filter contract

Replace the branch's superseded retirement behavior with the original
saved-view contract and positive-clause loading predicate.

## Acceptance

- `archived` is again a supported boolean dimension in the type union,
  registry, migration allowlist, and pure view evaluator; Show keeps archived
  rows and Hide keeps active rows.
- Boot, hydration, and live user-settings normalization preserve valid
  archived clauses in saved views and drafts.
- A tested pure predicate requests archived candidates only for
  `archived is true`; no clause and `archived is_not true` keep the active-only
  data path.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/filter-dimension-registry.test.ts lib/sidebar/apply-view.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/hydration/hydrator.test.ts lib/ws/handlers/users.test.ts hooks/domains/sidebar/use-effective-sidebar-view.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/sidebar-filter/filter-dimension-registry.ts`
- `apps/web/components/task/sidebar-filter/filter-dimension-registry.test.ts`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/slices/ui/ui-slice-migration.test.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/domains/sidebar/use-effective-sidebar-view.ts`
- `apps/web/hooks/domains/sidebar/use-effective-sidebar-view.test.ts`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01: this task owns frontend filter/view files; Task 01
owns backend query files.

## Inputs

- Spec **What**, persistence guarantee, and saved-view scenarios.
- Plan **Restore the saved-view filter contract**.
- Pre-retirement behavior in commit `f6f45b577` and current generic
  `migrateSidebarViewDraft` boundaries.

## Risks

- Existing tests on this branch assert that archived clauses are removed; they
  must be replaced with preservation tests, not left as contradictory coverage.
- The loading predicate is an upstream candidate-set rule and must not change
  the boolean semantics inside `applyView`.

## Output contract

Report restored types/registry/evaluator behavior, migration behavior, the
positive-clause predicate, exact files changed, commands/results, blockers,
and update this task plus `plan.md` status/results.

## Results

- Restored `archived` in the filter type union, registry, evaluator, and
  migration allowlist; `Show` evaluates archived rows and `Hide` evaluates
  active rows through the existing boolean operators.
- Added `viewRequiresArchivedTasks`, which gates archived candidate loading only
  for `archived is true`; default views and `is_not` remain active-only.
- Replaced removal regressions with preservation coverage across boot draft,
  hydration, live settings updates, and saved-view migration.
- `cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/filter-dimension-registry.test.ts lib/sidebar/apply-view.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/hydration/hydrator.test.ts lib/ws/handlers/users.test.ts` — passed (5 files, 95 tests).
- `cd apps/web && pnpm run typecheck` — passed.
