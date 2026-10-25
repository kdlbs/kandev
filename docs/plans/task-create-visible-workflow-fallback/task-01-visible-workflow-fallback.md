---
id: "01-visible-workflow-fallback"
title: "Select the sole visible workflow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 01: Select the sole visible workspace workflow

## Acceptance

- With no explicit workflow and exactly one visible workflow, that workflow
  becomes the effective task-create workflow even when hidden workflows are
  present.
- The standard New Task dialog omits the redundant workflow selector in that
  state.
- Explicit workflow selections, including the hidden Improve Kandev workflow,
  continue to win over implicit fallback.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-defaults.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run tests/task/create-task.spec.ts -- --grep "single visible workflow"`

## Files likely touched

- `apps/web/components/task-create-dialog-defaults.ts`
- `apps/web/components/task-create-dialog-defaults.test.ts`
- `apps/web/e2e/tests/task/create-task.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The unit, implementation, and E2E regression all exercise the same
fallback contract and should be completed in one TDD pass.

## Inputs

- `docs/specs/workspaces/requirements/improve-kandev.md` hidden-workflow and single-visible-workflow
  scenarios.
- `docs/plans/task-create-visible-workflow-fallback/plan.md`.
- `computeSingleWorkflowFallbackId` in
  `apps/web/components/task-create-dialog-defaults.ts`.
- The boot-store hidden workflow loading in `apps/web/app/page.tsx`.

## Output contract

Report the RED failure, changed files, GREEN unit/typecheck/E2E results,
remaining risks, and update this task plus `plan.md` to `done` in the same
primary conversation.

## Completion

- RED confirmed: the fallback returned `null` when its input contained one
  visible workflow and the hidden Improve Kandev workflow.
- GREEN: the fallback filters hidden workflows before deciding whether a sole
  workflow can be selected implicitly.
- Verification passed: focused Vitest regression, web typecheck, and the
  production-build Chromium E2E scenario.
