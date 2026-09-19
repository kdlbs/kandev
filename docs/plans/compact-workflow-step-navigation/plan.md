---
created: 2026-08-26
status: complete
requirements:
  - REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/compact-workflow-step-navigation.md
legacy_specs: []
---

# Implementation Plan: Compact Workflow Step Navigation

## Overview

Add an interactive disclosure to the compact task top-bar stepper. Reuse the existing movement policy and task-move callback.

Implement the component behavior and its browser proof in one vertical work order. This order keeps the responsive contract and movement result in one TDD cycle.

## Scope

### In scope

- Make the compact stepper a semantic and accessible trigger for active tasks.
- Show every workflow step in workflow order from the compact presentation.
- Permit moves only to targets that the full stepper already permits.
- Use an interactive Popover for fine pointers and an inset drawer for coarse pointers. The fine-pointer trigger and dialog expose matching ARIA semantics, and the move buttons remain keyboard tabbable.
- Give the coarse-pointer trigger a 44px hit area and visible disclosure cue.
- Preserve the existing phone movement path and full-stepper behavior.
- Cover the compact desktop and tablet paths with component and E2E tests.

### Out of scope

- Backend task-move changes.
- Cross-workflow movement from the compact stepper.
- A new phone top-bar control.
- New move notifications or error copy.
- Public documentation changes.

## Technical approach

### Stepper model and disclosure

Update `apps/web/components/task/workflow-stepper.tsx`. Keep `WorkflowStepper` as the owner of sorted steps, the current index, and move state.

Replace the static active-task branch in `MinimalWorkflowStepper` with a disclosure trigger. Keep the archived branch static.

Extract one disclosure body that renders each step from a shared row model. The model uses `canMoveToStep` for every target.

Reuse `StepCircleIndicator`, `task:currentStep`, `task:moveHere`, `task:moving`, and `task:stepOf`. Do not add untranslated copy.

Use `useTouchDrawer` to select the surface. Fine pointers use a controlled `Popover` with hover and focus behavior, dialog semantics, keyboard focus handling, and compact desktop-sized movement buttons. Coarse pointers use `Drawer` with internal overflow, safe-area padding, a 44px trigger hit area, 44px action hit areas, and a visible disclosure cue. Apply touch sizing only through coarse-pointer styles.

Keep the move request in `handleMove`. Return its success result so the temporary surface closes only after a successful request.

### Component tests

Extend `apps/web/components/task/workflow-stepper.test.tsx`. Mock width collapse and pointer mode independently.

Add tests for these results:

- fine-pointer hover or focus shows all ordered steps
- eligible targets show movement controls and the current step does not
- selection calls `moveTask` with the existing payload
- coarse-pointer activation opens the drawer with the same targets
- archived compact state stays static
- expanded mode does not mount the compact disclosure

### Browser tests

Add `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts`. Use a long task title and a 900px viewport to force the measured compact state.

The desktop scenario hovers and focuses the compact trigger, verifies the Popover dialog semantics, tabs to a step-specific move button, activates it with Enter, and polls the task API for the new step.

The tablet scenario uses `tabletTestPage`. It verifies the trigger hit area, opens the touch drawer, checks viewport containment, selects a target, and polls the same API result.

Keep `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` unchanged. Its current Pixel 5 move scenario remains the phone parity proof.

## Tests

| Acceptance criterion                           | Evidence                                                      |
| ---------------------------------------------- | ------------------------------------------------------------- |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.1` | Compact component test and desktop E2E trigger assertion      |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.2` | Fine-pointer component test and desktop Popover E2E           |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.3` | Touch-drawer component test and tablet trigger geometry E2E   |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.4` | Eligibility and current-step component tests                  |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.5` | `moveTask` payload component test and API-poll E2E assertions |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.6` | Keyboard desktop E2E with Tab and Enter, plus tablet geometry |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.7` | Archived compact-state component test                         |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.8` | Existing expanded-mode component test                         |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.9` | Existing Pixel 5 mobile task-drawer E2E scenario              |
| `AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.10` | Desktop compact button and coarse-pointer hit-area E2E     |

## E2E tests

- `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts`, Chromium project: compact hover disclosure and task movement.
- `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts`, Chromium project with `tabletTestPage`: touch drawer, geometry, and task movement.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`, mobile-chrome project: existing phone task movement through **Move to**.

## Work orders

- [x] [Task 01: Add compact workflow step disclosure](task-01-compact-step-disclosure.md)

## Verification results

Completed 2026-08-26.

- Component tests: 9 passed in `components/task/workflow-stepper.test.tsx`.
- Focused ESLint: passed with no warnings for the changed component, test, and E2E files.
- TypeScript typecheck: passed.
- i18n checks: passed with no new violations.
- Chromium E2E: compact Popover dialog semantics, keyboard Tab and Enter movement, Escape dismissal, focus return, and tablet touch-drawer scenarios passed.
- Mobile-chrome E2E: the existing phone **Move to** scenario passed.
- Desktop density: the fine-pointer move button measured below 40px, while the coarse-pointer move button and disclosure trigger remained at least 44px.
- Tablet geometry: the disclosure trigger and touch rows measured at least 44px, the touch drawer stayed inside the viewport, and the document did not gain horizontal overflow.

## Risks

- A nested disclosure can close its parent before a movement button receives the pointer. The design uses one Popover surface and no nested hover roots.
- The compact state depends on rendered width. The E2E fixture must prove that `workflow-stepper-minimal` is visible before it opens the disclosure.
- Tablet pointer detection depends on browser media queries. The E2E uses the existing `tabletTestPage` touch context.
- A failed move must not leave movement controls disabled or close the disclosure.
