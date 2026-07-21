---
id: "03-sidebar-view-e2e"
title: "Sidebar view creation E2E"
status: done
wave: 3
depends_on: ["02-sidebar-new-view-ui"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-view-creation.md"
---

# Task 03: Sidebar View Creation E2E

Prove the integrated direct-create journey against the production Vite build on desktop and mobile, then run repo verification.

## Acceptance

- Desktop Playwright proves immediate canonical creation, focused optional rename, rename/cancel persistence, automatic naming, dirty/limit guards, and reload survival.
- Mobile Playwright proves the fixed touch action creates and persists a view without hover, overlapping hit areas, broken chip scrolling, or horizontal page overflow.
- Focused E2E plus `make fmt`, repo typecheck, and lint complete successfully; feature-relevant tests pass, any unrelated baseline failure is recorded, and stale sidebar-view test comments are corrected.

## Verification

```bash
cd apps/web
pnpm build
pnpm e2e:run --host --no-build --project chromium -- tests/task/sidebar-filter.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-views.spec.ts

cd ../..
make fmt
make typecheck test lint
```

## Verification result

- Production build passed.
- Focused desktop Playwright passed 18/18; focused mobile Playwright passed 5/5.
- Web unit suite passed 5,113 tests across 632 files; backend retry, script tests, typecheck, and all linters passed.
- The repository-wide CLI suite retains an unrelated baseline failure: `.github/workflows/notify-docs.yml` pins pnpm `10.29.3`, while `apps/package.json` pins `9.15.9`. Neither file is changed by this plan.

## Files likely touched

- `apps/web/e2e/pages/sidebar-filter-popover.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `docs/specs/ui/sidebar-view-creation.md`
- `docs/plans/sidebar-view-creation/plan.md`
- `docs/plans/sidebar-view-creation/task-03-sidebar-view-e2e.md`

## Dependencies

Task 02 and its completed desktop/mobile UI.

## Inputs

- Spec: all scenarios, especially canonical defaults, optional rename, blocked states, failure-safe persistence, and mobile parity.
- Plan: E2E Tests; Verification; Risks and considerations.
- Existing page object: `SidebarFilterPopoverPage` in `apps/web/e2e/pages/sidebar-filter-popover.ts`.
- Existing journeys: saved-view CRUD in `sidebar-filter.spec.ts` and mobile sheet helpers in `mobile-sidebar-views.spec.ts`.
- Scope mobile selectors to the open sheet because a hidden desktop AppSidebar is mounted concurrently.

## Output contract

Report desktop/mobile journeys exercised, exact commands/results, artifacts for failures, files changed, blockers, and residual risks. Mark this task and plan entry `done`; move spec/plan status to `shipped`/`completed` only after integrated verification passes.
