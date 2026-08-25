---
id: "02-advanced-settings-e2e"
title: "Verify disclosure behavior on desktop and mobile"
status: completed
wave: 2
depends_on: ["01-advanced-settings-ui"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md"
---

# Task 02: Verify disclosure behavior on desktop and mobile

Extend the existing dependency-selector browser coverage to prove the new
collapsed default, expansion path, mobile touch behavior, and unchanged task
creation payload.

## Acceptance

- Desktop E2E proves the advanced row is collapsed on first render below the
  model, executor, and workflow controls, the dependency trigger is hidden
  until expansion, and the selector appears after expansion.
- Desktop E2E keeps the existing selector checks for the `No dependency`
  default, labeled setting help, task icons, search, info help, multiple
  selection, clearing, and `blocked_by` persistence after creation. It also
  verifies the two-column option grid keeps the label to the left of the selector
  on the same horizontal row and makes the option narrower than the full desktop grid.
- Mobile E2E expands the section with touch input, verifies the disclosure hit
  box and setting help control are at least 44 CSS pixels, verifies the label
  and selector remain on the same horizontal row with the label to the selector's
  left, and retains picker containment, touch
  selection, help readability, and no-horizontal-overflow checks.
- The managed desktop and mobile runs pass against a fresh production build
  with no stale frontend artifacts.

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

## Dependencies and risks

Task 01. The exact disclosure test ID and final placement are outputs of the UI
implementation. If the mobile hitbox is reduced because the visual label is
small, the E2E test should catch it before the change is considered complete.

## Parallelism

`sequential` after Task 01. The E2E files are independent of backend changes,
but they depend on the final UI contract.

## Inputs

- Scenarios and mobile contract in
  `docs/specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md`
- Existing desktop coverage in
  `apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts`
- Existing mobile coverage in
  `apps/web/e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`
- Layout helpers in `apps/web/e2e/helpers/layout-assertions.ts`

## Output contract

Report the exact managed commands, pass counts, viewport and overflow results,
artifacts, blockers, and the final Task 02 and plan status.

## Results

- `pnpm e2e:run --project chromium e2e/tests/task/create-task-dependency-selector.spec.ts`
  passed 1 test on a fresh production build.
- `pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`
  passed 1 test on a fresh production build. The test covered the 44 CSS pixel
  disclosure hitbox, picker containment, touch selection, help readability,
  and no horizontal overflow.
- No E2E artifacts or blockers remain. The runner cleaned its temporary fixture
  and test-result directories after each managed run.
