---
id: "04-integration-e2e"
title: "Desktop and mobile integration coverage"
status: done
wave: 3
depends_on:
  - "01-device-view-and-workflow-fix"
  - "03-rich-list-row-ui"
plan: "plan.md"
spec: "../../specs/ui/requirements/task-listing-display-preferences.md"
role: test-engineer
model_tier: balanced
---

# Task 04: Desktop and mobile integration coverage

## Acceptance

- Playwright proves the one-workflow **All Workflows** regression and reload.
- Desktop Playwright proves device-local view restoration and portable rich-row
  persistence with repo, description, and PR metadata.
- Mobile Playwright proves drawer reachability, List restoration, Pipeline
  effective fallback without overwrite, rich-row navigation, and no document
  horizontal overflow.

## Verification

- `cd apps/web && pnpm e2e:run tests/kanban/workflow-filter.spec.ts tests/task/task-listing-view-preferences.spec.ts tests/task/mobile-task-listing-display.spec.ts`

## Files likely touched

- `apps/web/e2e/tests/kanban/workflow-filter.spec.ts`
- `apps/web/e2e/tests/task/task-listing-view-preferences.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`
- `apps/web/e2e/pages/kanban-page.ts` only for reusable stable selectors
- `apps/web/e2e/helpers/api-client.ts` for the typed settings field
- production files only for stable `data-testid` attributes required by these
  scenarios

## Inputs

- All spec scenarios.
- Plan: **E2E Tests**, **Mobile design contract**, and completed Task 01/03
  handoff capsules.
- E2E conventions in `.agents/skills/e2e/SKILL.md`.

## Output contract

Return a compact handoff with intent/acceptance, base/head SHA if committed,
changed files and entry points, spec sections, `user-flow + integration +
persistence` risk tags, exact command results and artifacts, uncertainties,
and the task-file status update. Do not edit `plan.md`.
