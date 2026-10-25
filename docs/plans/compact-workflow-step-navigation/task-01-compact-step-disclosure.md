---
id: "01-compact-step-disclosure"
title: "Add compact workflow step disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001
acceptance_criteria:
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.1
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.2
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.3
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.4
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.5
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.6
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.7
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.8
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.9
  - AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.10
system_design:
  - ../../specs/ui/system-design/compact-workflow-step-navigation.md
---

# Task 01: Add Compact Workflow Step Disclosure

## Summary

Add the ordered workflow-step disclosure to the compact task top bar. Prove desktop hover, keyboard movement, tablet touch, and task movement through one TDD cycle.

## In scope

- Add the shared disclosure body and responsive surface selection.
- Reuse current-step state, target eligibility, plan-mode cleanup, and the task-move API.
- Add focused component tests for interaction, accessibility, movement, and archived behavior.
- Add desktop and tablet E2E coverage for the compact top bar, including keyboard activation and trigger geometry.
- Run the existing phone task-move E2E scenario as mobile parity proof.

## Out of scope

- Backend changes.
- Cross-workflow movement.
- A new phone task top-bar entry point.
- New user-facing copy.
- Changes to the full-stepper layout.

## Acceptance

- The compact trigger shows all ordered steps through hover, focus, or coarse-pointer activation. Fine-pointer movement controls are reachable by keyboard and retain compact desktop sizing; coarse-pointer activation has a 44px trigger and action hit area with a visible cue.
- The disclosure identifies the current step and moves the task only to eligible targets.
- The Popover and drawer stay usable, contained, and accessible. Archived and full-stepper states retain their existing behavior.

## Verification

Run these commands from `apps/web` after the workspace dependencies are installed:

```bash
pnpm exec vitest run components/task/workflow-stepper.test.tsx
pnpm run i18n:check
pnpm e2e:run --host --project chromium -- tests/layout/task-topbar-workflow-stepper.spec.ts --workers=1
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-task-actions.spec.ts --grep "moves a task to another step from the mobile task drawer" --workers=1
pnpm exec tsc --noEmit
```

## Files likely touched

- `apps/web/components/task/workflow-stepper.tsx`
- `apps/web/components/task/workflow-step-disclosure.tsx`
- `apps/web/components/task/workflow-stepper.test.tsx`
- `apps/web/e2e/helpers/animations.ts`
- `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts`
- `apps/web/e2e/tests/plugins/bitbucket-packaged-plugin.spec.ts`
- `apps/web/e2e/tests/review/submodule-review-helpers.ts`
- `docs/plans/compact-workflow-step-navigation/plan.md`
- `docs/plans/compact-workflow-step-navigation/task-01-compact-step-disclosure.md`

## Dependencies

None.

## Risks

- Hover content must stay open while the pointer moves from the trigger to a movement control.
- A move error must clear the target state without closing the disclosure.
- Width-based collapse must remain stable while the disclosure portal mounts.
- Coarse-pointer rows and action controls must meet the 44px target size without making the desktop Popover controls visually oversized.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001` and all related acceptance criteria.
- `docs/specs/ui/system-design/compact-workflow-step-navigation.md`.
- `apps/web/components/task/workflow-stepper.tsx` and its component tests.
- `apps/web/components/task/changes-panel-header.tsx` for the hover-card and touch-drawer precedent.
- `apps/web/e2e/fixtures/test-base.ts` for `tabletTestPage`.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` for phone movement proof.

## Results

Completed 2026-08-26.

- Added a semantic compact-step trigger for active tasks and preserved the static archived presentation.
- Reused the existing step eligibility policy and task-move payload, including plan-mode cleanup and move-state handling.
- Added a shared ordered disclosure body with current-step identification and 44px target rows.
- Selected a fine-pointer Popover or coarse-pointer drawer through the existing responsive pointer hook. The Popover exposes dialog semantics and keeps movement controls keyboard tabbable; the coarse trigger exposes a 44px hit area and disclosure cue.
- Added component coverage for expanded and compact states, ordered disclosure content, eligibility, movement payload, coarse-pointer behavior, archived state, and empty workflows.
- Added Chromium coverage for hover, Popover dialog semantics, keyboard Tab and Enter movement, Escape dismissal, focus return, tablet trigger geometry, and touch-drawer containment.
- Re-ran the existing Pixel 5 mobile task-drawer movement scenario successfully.
- Applied PR review remediation: defined the disclosure-owned step type, added failed-move recovery coverage, centralized finite-animation waiting for the affected E2E helpers, replaced the non-interactive hover surface with a keyboard-accessible Popover, and added coarse-pointer trigger geometry and cue coverage.
- Applied desktop density correction: fine-pointer move buttons use compact `h-7` sizing, while coarse-pointer action controls retain the 44px touch hit area.

Verification commands passed:

```text
pnpm exec vitest run components/task/workflow-stepper.test.tsx
pnpm run i18n:check
pnpm run i18n:ratchet -- --files components/task/workflow-stepper.tsx components/task/workflow-step-disclosure.tsx components/task/workflow-stepper.test.tsx e2e/tests/layout/task-topbar-workflow-stepper.spec.ts
pnpm exec eslint components/task/workflow-stepper.tsx components/task/workflow-step-disclosure.tsx components/task/workflow-stepper.test.tsx e2e/tests/layout/task-topbar-workflow-stepper.spec.ts
pnpm e2e:run --host --no-build --project chromium -- tests/layout/task-topbar-workflow-stepper.spec.ts --workers=1
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-task-actions.spec.ts --grep "moves a task to another step from the mobile task drawer" --workers=1
pnpm exec eslint components/task/workflow-stepper.tsx components/task/workflow-step-disclosure.tsx components/task/workflow-stepper.test.tsx e2e/helpers/animations.ts e2e/tests/layout/task-topbar-workflow-stepper.spec.ts e2e/tests/plugins/bitbucket-packaged-plugin.spec.ts e2e/tests/review/submodule-review-helpers.ts
pnpm e2e:run --host --no-build --project chromium -- tests/layout/task-topbar-workflow-stepper.spec.ts --workers=1
```
