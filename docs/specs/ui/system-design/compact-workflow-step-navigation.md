---
status: draft
system: ui
requirements:
  - REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001
---

# Compact Workflow Step Navigation System Design

## Purpose and boundaries

The UI system owns the responsive stepper and its temporary disclosure. The task system owns workflow data and the task-move API.

This design changes only the compact branch in `WorkflowStepper`. It does not change backend contracts or the full-stepper layout.

## Requirement mapping

| Requirement                                   | Design section                                                                                                                                                                                                   |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Responsive interaction](#responsive-interaction), [Accessibility and geometry](#accessibility-and-geometry) |

## Components and responsibilities

- `WorkflowStepper` sorts the steps, finds the current index, measures available width, and owns task-move state.
- `useToolbarCollapsed` continues to select the full or compact presentation from the center area's measured width.
- The compact presentation becomes an interactive trigger for active tasks. It keeps the current step marker, name, and position count.
- A shared disclosure body shows all sorted steps. It marks the current step and shows movement controls for eligible targets.
- `canMoveToStep` remains the single UI policy for target eligibility. Adjacent steps and steps with `allow_manual_move` remain eligible.
- `useTouchDrawer` selects the disclosure surface from pointer precision. This selection does not duplicate task state or move logic.
- The archived presentation remains a static badge and does not mount the disclosure.

## Data and contracts

The design uses the existing `WorkflowStepperStep` values. It does not add a new store field, API field, or persisted preference.

The disclosure receives these derived values:

- the steps sorted by `position`
- the current step index
- the target step that has a move in progress
- the result of `canMoveToStep` for each step
- the existing `handleMove` callback

The move callback continues to call `moveTask` with `workflow_id`, `workflow_step_id`, and position `0`. It also retains plan-mode cleanup.

## Control flow

1. `WorkflowStepper` measures the center area and selects the compact presentation when the full stepper overflows.
2. The compact trigger shows the current step and its position count.
3. The user opens the disclosure through hover, focus, or touch activation.
4. The disclosure shows all steps in workflow order.
5. The UI enables only eligible targets and leaves the current step unavailable.
6. The user selects a target. `handleMove` disables plan mode and sends the existing move request.
7. A successful request closes the temporary surface. Live task state then updates the current step.
8. A failed request restores the target control and keeps the surface available for another action.

## Responsive interaction

### Fine pointer

The compact trigger uses an interactive `HoverCard`. A hover or keyboard focus opens a vertical list below the trigger.

The hover delay and close delay match the full stepper. Users can move the pointer from the trigger into the content.

### Coarse pointer

The compact trigger opens an inset `Drawer` through `useTouchDrawer`. The drawer uses the same list and movement callback.

This drawer is the tablet fallback because the desktop top bar still renders at tablet widths. Phone task pages use the existing task drawer path.

### Mobile design contract

- **Desktop outcome:** The compact trigger shows all steps and supports eligible task moves.
- **Mobile entry point:** The phone task drawer keeps **Task actions** > **Move to**. The tablet compact trigger opens the new drawer.
- **Nearest shipped exemplar:** `BranchHoverCard` provides the hover-card and touch-drawer split. `mobile-sidebar-task-actions.spec.ts` provides phone movement proof.
- **Hierarchy:** The current step stays in the top bar. The temporary surface lists all steps and their movement actions.
- **Presentation rationale:** Step selection is a short, temporary choice. A hover card or drawer is more direct than a new route.
- **Scroll owner:** The disclosure list owns vertical overflow. The document does not gain horizontal overflow.
- **Shared logic:** Both surfaces use the same sorted steps, eligibility policy, move state, and callback.
- **Mobile proof:** The existing Pixel 5 move scenario remains part of the focused commands.

## Accessibility and geometry

The compact trigger is a semantic button. Its accessible name includes the current step, step number, and total step count.

The current row uses `aria-current="step"`. Eligible movement controls are keyboard accessible and use existing translated labels.

The hover card stays within the viewport through Radix collision handling. The drawer uses internal scrolling and bottom safe-area padding.

The disclosure supports Escape dismissal and focus return. Touch rows use a minimum active height of 44px.

## Failure and recovery

The existing move request remains the authority for transition errors. A request error clears the move-in-progress state and keeps the disclosure usable.

If the current step is absent, the compact trigger keeps the existing first-step fallback. It does not mark that fallback as the current step.

If live workflow data changes while the disclosure is open, React derives the list from the latest sorted steps. Removed steps disappear without persisted selection state.

## Test strategy

Component tests cover compact content, eligibility, fine-pointer disclosure, touch-drawer selection, archived behavior, and move callbacks.

Desktop E2E coverage forces the measured compact state at a narrow top-bar width. It opens all steps and moves the task through the UI.

Tablet E2E coverage uses `tabletTestPage`. It opens the touch drawer, checks containment, and selects an eligible step.

The existing Pixel 5 task-drawer scenario proves that phone users retain step movement without the desktop top bar.

## Related decisions

No architecture decision record applies. This design reuses the current responsive overlay and task-move contracts.
