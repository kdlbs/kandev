---
id: "04-frontend-ui-wiring"
title: "Frontend: StatusPill options + per-project status fetch/cache wiring"
status: done
wave: 3
depends_on: ["02-frontend-types-api", "03-frontend-filter-model"]
plan: "plan.md"
spec: "../../specs/jira-status-filter/spec.md"
---

# Task 04: Status pill + page wiring

Populate the status filter from the selected project(s) and reconcile selection.

## Acceptance
- `StatusPill` accepts `options: JiraStatus[]`, `value: string[]`, `disabled`, and a hint; multi-selects by status name; when `options` is empty (no project selected) it renders disabled with a hint to pick a project.
- Page hook fetches statuses for each selected project key via `listJiraProjectStatuses`, unions + de-dupes by name, and caches per project key for the session (no refetch on re-select). Status fetch failure is non-fatal (empty options, list still loads).
- Changing the project selection drops any selected statuses no longer present in the union (reconciliation helper is unit-testable).
- `FilterBar` passes options/disabled through to `StatusPill`.

## Verification
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `cd apps && pnpm --filter @kandev/web test -- components/jira/my-jira` (reconciliation helper test)

## Files likely touched
- `apps/web/components/jira/my-jira/filter-pills.tsx`
- `apps/web/components/jira/my-jira/filter-bar.tsx`
- `apps/web/app/jira/jira-page-client.tsx`
- new small hook/helper module under `apps/web/components/jira/my-jira/` (+ test)

## Inputs
- Spec: What (union/dedupe, disabled-when-no-project, cache, drop stale), Failure modes, Decisions, Scenarios 1/4/5/7.
- Plan: Frontend > Status pill, Filter bar + page wiring.
- Patterns: existing `StatusPill`/`ProjectPill` in `filter-pills.tsx`; `useJiraPageData`/`loadUserProjects` in `jira-page-client.tsx`.
- Follow `/mobile-parity`: keep the pill responsive.

## Dependencies
Tasks 02 and 03.

## Output contract
Report: summary, files changed, tests run + results, blockers, risks; set `status: done` + tick plan checkbox.
