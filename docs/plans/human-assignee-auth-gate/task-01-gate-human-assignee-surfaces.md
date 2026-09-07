---
id: "01-gate-human-assignee-surfaces"
title: "Gate human-assignee surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-HUMAN-ASSIGNEE-001
acceptance_criteria:
  - AC-TASKS-HUMAN-ASSIGNEE-001.9
system_design:
  - ../../specs/tasks/system-design/human-assignee.md
---

# Task 01: Gate Human-Assignee Surfaces

## Summary

Use the auth mode to hide human-assignee controls and indicators outside
enabled authentication. Preserve the stored assignment and all enabled-mode
behavior.

## In scope

- Add failing component tests for the real synthetic disabled-mode state.
- Gate the task-topbar picker and Office property row.
- Gate the kanban card indicator without leaving an empty badge row.
- Add focused desktop Playwright coverage for task, board, and Office surfaces.

## Out of scope

- Backend, database, API, event, and migration changes.
- Permission-model changes.
- New mobile assignment interactions.
- Public documentation changes.

## Acceptance

- Disabled, setup, and anonymous auth states show no human-assignee controls or
  indicators.
- Enabled auth with a real user preserves existing controls, assignment
  actions, and card indicators.
- Hidden surfaces do not request user-directory or workspace-member data.
- Auth-mode changes never clear the stored human-assignee value.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test components/task/task-assignee-control.test.tsx components/task/simple/task-properties.test.tsx components/kanban-card-assignee.test.tsx components/kanban-card-content.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run tests/task/human-assignee-auth-gate.spec.ts)
(cd apps/web && pnpm e2e:run tests/office/property-pickers.spec.ts -- --grep "human assignee")
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-assignee-control.tsx`
- `apps/web/components/task/task-assignee-control.test.tsx`
- `apps/web/components/task/simple/task-properties.tsx`
- `apps/web/components/task/simple/task-properties.test.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/kanban-card-assignee.test.tsx`
- `apps/web/components/kanban-card-content.test.tsx`
- `apps/web/lib/auth/human-assignee.ts`
- `apps/web/e2e/tests/task/human-assignee-auth-gate.spec.ts`
- `apps/web/e2e/tests/office/property-pickers.spec.ts`

## Dependencies

None.

## Risks

- The kanban badge-row predicate can diverge from the badge render condition.
- Test fixtures can hide the bug if disabled mode uses `user: null` instead
  of the synthetic user.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/human-assignee.md`
- `docs/specs/tasks/system-design/human-assignee.md`
- `docs/decisions/2026-07-24-opt-in-authentication.md`
- `docs/public/team-access.md`
- Existing topbar, Office property, and kanban card assignee tests.

## Results

- Added `canShowHumanAssignee` as the shared enabled-auth and real-user gate.
- Gated the task topbar, Office property row, card indicator, and card badge-row
  predicate without changing persisted assignment data.
- Added the real synthetic disabled-mode state to component tests.
- Added focused task, board, and Office browser regressions.
- Verification passed: 27 component tests, two Playwright tests, TypeScript,
  frontend lint, specification validation, and whitespace checks.
- Addressed review feedback by making verification commands root-safe, covering
  the kanban content test, asserting complete Office-row absence, and using a
  strict task-topbar title locator.
