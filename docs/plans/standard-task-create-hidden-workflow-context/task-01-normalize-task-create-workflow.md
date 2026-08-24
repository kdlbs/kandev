---
id: "01-normalize-task-create-workflow"
title: "Normalize standard task-create workflow context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 01: Normalize standard task-create workflow context

## Acceptance

- Standard task creation ignores a hidden workflow inherited from task-detail
  state and resolves the workspace's sole visible workflow.
- Feature wrappers with `lockedFields.workflow` continue to create tasks in
  their explicitly supplied hidden workflow.
- When normalization selects a different workflow, its fetched start/first step
  becomes the submitted default step; no step from the hidden workflow leaks.
- Desktop sidebar and mobile task-drawer E2E flows both prove that a task
  created from a hidden-workflow task lands in the visible workflow.

## Verification

- `cd apps && pnpm install --frozen-lockfile` (required once in a fresh worktree)
- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-defaults.test.ts components/task-create-dialog-effects.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run tests/task/create-task.spec.ts -- --grep "hidden workflow task detail"`
- `cd apps/web && pnpm e2e:run tests/task/mobile-hidden-workflow-task-create.spec.ts -- --project=mobile-chrome`
- `make fmt`
- `make typecheck test lint`

## Files likely touched

- `apps/web/components/task-create-dialog-defaults.ts`
- `apps/web/components/task-create-dialog-defaults.test.ts`
- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/task-create-dialog-effects.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-hidden-workflow-task-create.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if the existing task response
  helper needs to expose `workflow_id` for a behavioral assertion

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Hidden-workflow requirements and task-detail regression scenario in
  `docs/specs/workspaces/requirements/improve-kandev.md`.
- Frontend approach in `plan.md`.
- Existing visible fallback in
  `apps/web/components/task-create-dialog-defaults.ts`.
- Explicit hidden workflow wrapper in
  `apps/web/components/improve-kandev-dialog-create.tsx`.
- Existing hidden workflow E2E seeding in
  `apps/web/e2e/helpers/api-client.ts`.
- Existing mobile task drawer in
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.

## Output contract

Report the unit and E2E RED/GREEN command results, changed files, failure
artifacts, remaining risks, and update this task to `done` plus the
corresponding plan checkbox in the primary conversation.

## Verification Results

- RED: focused dialog tests failed in five expected assertions before
  implementation (missing context resolver, missing visible-step fetch, and
  missing visible start-step selection).
- RED: desktop Chromium created the new task with the hidden workflow step
  instead of the visible Kanban start step.
- GREEN: 48 focused task-create dialog tests passed.
- GREEN: desktop Chromium and mobile Chrome hidden-workflow creation scenarios
  passed, including the persisted step assertion and mobile overflow check.
- GREEN: `make fmt`.
- GREEN: `make typecheck test lint`.
