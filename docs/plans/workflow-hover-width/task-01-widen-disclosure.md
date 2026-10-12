---
id: "01-widen-disclosure"
title: "Widen compact workflow disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001
acceptance_criteria:
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.3
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.6
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.9
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.11
system_design:
  - ../../specs/ui/system-design/compact-workflow-step-navigation.md
---

# Task 01: Widen compact workflow disclosure

## Summary

Use a 28rem desktop hover width with the existing viewport cap. Prove normal step names fit beside the actual controls.

## In scope

- Add the rendered regression described in the plan, run it to demonstrate failure, then change the Popover width.
- Check constrained viewport containment, desktop keyboard operation, tablet Drawer, and the existing phone move path.

## Out of scope

Backend, move semantics, full-stepper layout, translations, and new mobile surfaces.

## Acceptance

- Backlog and Implementation fit with capability icons and move/options controls in English and pt-pt at a desktop viewport.
- The desktop dialog stays viewport-contained; unusually long labels still truncate safely.
- Existing tablet touch and phone move paths pass their checks.

## ASCII UI preview

UI-01: Compact stepper hover, eligible step. AC-001.11 and AC-001.6 below refer to the full acceptance IDs in the work order.

```text
Before: [o I...  icons  options  Move here]
After:  [o Implementation  icons       options  Move here]
Phone:  [Task actions] -> [Move to] -> [step choices]
Tablet: [Current step v] -> [inset drawer with step choices]
```

Keep the row order and right-hand actions. Width is 28rem, capped to the viewport; ASCII spacing is illustrative.
The existing list owns vertical scrolling. Keep safe-area padding and touch targets in the existing drawer.
Phone navigation stays as shipped; the nearest exemplar is the existing compact disclosure Drawer and phone task actions.

See the [combined preview](plan.md#ascii-ui-preview).

## Verification

Run from the repository root. Install workspace dependencies if missing. The first E2E command builds current sources; the second reuses that build.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/workflow-stepper.test.tsx)
(cd apps/web && pnpm exec eslint components/task/workflow-step-disclosure.tsx e2e/tests/layout/task-topbar-workflow-stepper.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/layout/task-topbar-workflow-stepper.spec.ts --workers=1)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-task-actions.spec.ts --grep "moves a task to another step from the mobile task drawer" --workers=1)
git diff --check
```

## Files likely touched

- `apps/web/components/task/workflow-step-disclosure.tsx`
- `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts`
- This work order and `plan.md` for results.

## Dependencies

None. The original compact-navigation work order is already done.

## Risks

Check real computed label widths; visible text assertions alone do not detect ellipsis.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/compact-workflow-step-navigation.md).
- [System design](../../specs/ui/system-design/compact-workflow-step-navigation.md).
- Existing stepper E2E and component tests; screenshot attached to this task.

## Results

Implemented the desktop Popover width as `w-[28rem]` while retaining the existing `max-w-[calc(100vw-1rem)]` cap and the touch Drawer branch. Added the named desktop regression with a disposable workflow that renders Backlog and Implementation capability icons and move/options controls in English and pt-pt. The regression first failed against `w-72` because Implementation measured 82px of content in a 46px label box, then passed with the 28rem width.

Validation passed:

- `pnpm exec vitest run components/task/workflow-stepper.test.tsx` (20 tests).
- Targeted ESLint for the disclosure component and desktop E2E.
- Desktop workflow-stepper E2E (3 tests), including the new regression, keyboard hover flow, and tablet touch Drawer.
- Existing Pixel 5 phone task-drawer move E2E (1 test).
- `git diff --check`.

The host E2E run built the backend and pseudo-locale Vite bundle successfully. No new mobile surface or user-facing copy was added.
