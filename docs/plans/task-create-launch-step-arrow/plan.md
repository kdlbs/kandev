---
created: 2026-09-07
status: complete
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
legacy_specs: []
---

# Implementation Plan: Task Create Launch-Step Arrow

## Overview

Refine the existing task-create launch destination into a clearer visual flow.
Replace the generic information glyph and visible **Start step:** prefix with a
directional arrow help button followed by the destination step name. Preserve
the current explanation and prove that it remains available by pointer,
keyboard, and touch.

## Scope

### In scope

- Render `IconArrowBigRightLines` between the selected workflow and launch step.
- Show only the resolved step name as visible destination text.
- Preserve the localized explanation in a tooltip on hover and focus, and
  expose it in a drawer with a 44-pixel trigger on coarse pointers.
- Preserve the localized accessible name for the help button.
- Remove the now-unused visible launch-destination label from all locale
  catalogs and update the task-creation guide.
- Update focused component, desktop, and mobile coverage.

### Out of scope

- Changing launch-destination resolution or task submission behavior.
- Making the launch destination selectable.
- Changing the prompt-preview toggle or editor.
- Adding a new route or mobile composition.

## Technical approach

### Launch destination presentation

Update `WorkflowSelectorRow` so `LaunchDestinationInfo` renders
`IconArrowBigRightLines` in the existing ghost icon button. Keep the existing
localized accessible name, tooltip content, test ID, compact fine-pointer size,
and 44-pixel coarse-pointer hit area. Use `useTouchDrawer` to present the same
localized explanation in a drawer for coarse pointers. Render
`launchPreview.stepName` directly in `LaunchDestinationLabel`; workflow step
names are domain data and are not translated.

The arrow remains a help control rather than a navigation or submit action. Its
placement between the workflow trigger and step name communicates the flow
`workflow -> launch destination`, while the tooltip or touch drawer explains
the action-sensitive precedence.

### Localization and documentation

Remove `task:launchDestination` from the English, Portuguese, Simplified
Chinese, Traditional Chinese, and pseudo catalogs because the visible prefix is
removed. Retain `task:launchDestinationHelpLabel` and
`task:launchDestinationHelp`. Update the public task-creation procedure to name
the arrow help control instead of an information button.

### Responsive behavior

Desktop and mobile keep the current inline selector row and shared task-create
dialog. The nearest mobile exemplar remains the Kanban FAB to task-create dialog
flow documented by the launch-preview design. This content-only refinement adds
no scroll, safe-area, or navigation boundary. The existing mobile launch-preview
scenario will prove touch access, hit-area size, containment, and the absence of
horizontal overflow.

## Tests

- `AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1` maps to
  `apps/web/components/workflow-selector-row.test.tsx`, which verifies the arrow
  glyph, destination-only text, accessible name, tooltip disclosure, and
  coarse-pointer drawer.

## E2E tests

- Update `apps/web/e2e/tests/task/create-task.spec.ts` to verify the destination
  step names and the existing pointer-hover explanation.
- Update
  `apps/web/e2e/tests/task/mobile-create-task-launch-preview.spec.ts` to verify
  the destination step names, touch-activated explanation, 44-pixel target, and
  existing containment contract in `mobile-chrome`.

## Work orders

- [x] [Task 01: Refine the launch-step help control](task-01-refine-launch-step-help-control.md)

## Verification results

- Focused selector component tests passed (3 tests) after failing first on the
  old **Start step:** label and then on the missing coarse-pointer drawer.
- Focused ESLint, TypeScript typecheck, i18n completeness, and i18n new-code
  ratchet checks passed.
- Public documentation tests passed (61 tests), and all 46 published pages
  validated.
- Desktop Chromium passed the launch-preview scenario with pointer-hover help.
- Mobile Chrome passed the launch-preview scenario with the touch drawer,
  44-pixel target sizing, containment, and no horizontal overflow.
- Specification tests, full specification lint, and `git diff --check` passed.

## Risks

- A directional arrow can resemble a navigation action if it is separated from
  the destination name. Keeping the arrow inline, muted, and immediately before
  the name preserves the intended flow reading.
- Removing localized visible copy can leave stale catalog entries or public
  guidance. The i18n and documentation validators cover both boundaries.
- Tooltip-only help would be unavailable on touch. The coarse-pointer drawer
  and mobile interaction test preserve tap access.
