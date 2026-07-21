---
id: "03-frontend-filter-model"
title: "Frontend: filter model status -> status names + saved-view back-compat"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/jira-status-filter/spec.md"
---

# Task 03: Filter model + saved-view back-compat

Switch the filter model from status categories to real status names.

## Acceptance
- `FilterState.statusCategories: JiraStatusCategory[]` becomes `statuses: string[]`; `DEFAULT_FILTERS` and `filtersEqual` updated; `STATUS_CATEGORY_OPTIONS` removed.
- `statusClause` emits `status in ("A", "B")` using the existing `quote`; empty selection emits no clause.
- Legacy saved views persisted with `statusCategories` hydrate to `statuses: []` without throwing (`use-saved-views.ts`).
- Unit tests cover the new JQL clause and the legacy-view hydration.

## Verification
- `cd apps && pnpm --filter @kandev/web test -- components/jira/my-jira/filter-model.test.ts`
- `cd apps && pnpm --filter @kandev/web test -- components/jira/my-jira/use-saved-views`
- `cd apps/web && pnpm run typecheck`

## Files likely touched
- `apps/web/components/jira/my-jira/filter-model.ts`
- `apps/web/components/jira/my-jira/filter-model.test.ts`
- `apps/web/components/jira/my-jira/use-saved-views.ts` (+ test)

## Inputs
- Spec: What, Data model (FilterState change + saved-view rule), Scenarios (JQL, saved views).
- Plan: Frontend > Filter model, Saved views back-compat; Tests.
- Note: this changes a shared type; expect TypeScript errors in `filter-pills.tsx`/`filter-bar.tsx`/`jira-page-client.tsx` resolved by task 04.

## Dependencies
None (type-only; consumers fixed in task 04).

## Output contract
Report: summary, files changed, tests run + results, blockers, risks; set `status: done` + tick plan checkbox.
