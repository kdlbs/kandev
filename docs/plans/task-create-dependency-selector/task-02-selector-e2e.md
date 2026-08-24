---
id: "02-selector-e2e"
title: "Verify desktop and mobile flows"
status: done
wave: 2
depends_on: ["01-selector-ui"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md"
---

# Task 02: Verify desktop and mobile flows

Add browser coverage for the final dependency selector through the real task
creation dialog and verify the mobile composition with the configured
`mobile-chrome` project.

## Acceptance

- Desktop E2E proves the default state, workflow/dependency placement, task
  icons, search, info help, multiple selection, clearing, and creation with
  the selected `blocked_by` IDs.
- Mobile E2E uses touch interaction to select a predecessor, verifies the help
  action is reachable, checks picker and control containment, and proves there
  is no document-level horizontal overflow.
- The exact managed desktop and mobile E2E commands pass against a fresh
  production build with no stale frontend artifacts.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
cd web
pnpm e2e:run --project chromium e2e/tests/task/create-task-dependency-selector.spec.ts
pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-create-task-dependency-selector.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`

## Dependencies

Task 01. The E2E selectors and responsive composition are outputs of the UI
implementation.

## Parallelism

`sequential`.

## Inputs

- Spec scenarios and Mobile design contract in
  `docs/specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md`
- Plan E2E Tests and Mobile parity contract in `plan.md`
- `apps/web/e2e/fixtures/test-base.ts`, `pages/kanban-page.ts`, and
  `pages/mobile-kanban-page.ts`
- Existing task creation coverage in `apps/web/e2e/tests/task/create-task.spec.ts`
  and mobile repository picker coverage

## Results

- Desktop managed E2E passed: `pnpm e2e:run --project chromium
  e2e/tests/task/create-task-dependency-selector.spec.ts` (1 test passed in
  4.6s). The run rebuilt the backend and pseudo-locale Vite assets, and the
  task creation API assertion confirmed both selected predecessor IDs.
- Mobile managed E2E passed: `pnpm e2e:run --project mobile-chrome
  e2e/tests/task/mobile-create-task-dependency-selector.spec.ts` (1 test
  passed in 3.8s). The run rebuilt the backend and pseudo-locale Vite assets,
  and verified the mobile selector, contained picker, touch selection, help
  copy, and horizontal overflow contract.
- The first mobile run exposed a 43.56px rem-based trigger in the configured
  viewport. Mobile controls now use the 48px spacing step, with desktop
  controls retaining their compact sizes. The rerun passed.
- The managed runner cleaned its test results and PR asset directories before
  each run. No failure artifacts remain from the passing reruns.

## Output contract

Report the E2E scenarios, commands, outcomes, artifacts, blockers, and
task/plan status updates in the same conversation.
