---
status: shipped
created: 2026-07-14
owner: kandev
---

# Workflow Settings Autosave

## Why

Workflow settings currently mix manual saving for workflow details with immediate saving for workflow steps. The mixed model makes the Save button look authoritative even when it does not control step persistence, and responsive clipping can make that required action unreachable.

## What

- Confirming the Add Workflow dialog creates the workflow and its initial template or custom steps; users do not need a second Save action.
- Changes to workflow name, default agent profile, step fields, step ordering, and step membership save automatically.
- Each workflow card exposes one status for all automatic persistence: `Saving`, `Saved`, or `Couldn't save`, with a Retry action when the last operation failed.
- Workflow name edits are debounced. Discrete controls and step mutations begin saving immediately.
- A workflow card never presents a manual Save button.
- Desktop and mobile layouts keep every required action inside the viewport. The pipeline may scroll horizontally inside its own region, but the page and card do not horizontally overflow.
- On narrow screens, workflow details and step configuration fields stack, section actions wrap, and destructive/secondary actions remain reachable by touch.

## Failure Modes

- Automatic saves run in order within the metadata and step mutation streams. A failed save pauses later queued writes in the affected stream, leaves the user's current value visible, shows `Couldn't save`, and offers Retry for the exact failed operation.
- A failed workflow creation keeps the Add Workflow dialog open, preserves its inputs, and shows an error notification.
- If both workflow creation and rollback fail, the partial workflow remains visible so the user can retry setup or delete it.
- Retrying does not create duplicate workflows or replay an operation that already succeeded; queued writes in the affected stream resume only after the failed write succeeds.

## Persistence Guarantees

Only successful backend writes survive reload or restart. Save-status UI state is transient and is reconstructed as the neutral autosave state after navigation.

## Scenarios

- **GIVEN** an existing workflow, **WHEN** the user changes its name and pauses, **THEN** the card shows saving feedback and the new name remains after reload without a Save click.
- **GIVEN** an existing workflow, **WHEN** the user changes a step setting, **THEN** the same card-level status reports the operation and the setting remains after reload.
- **GIVEN** a selected template or Custom, **WHEN** the user confirms Add Workflow, **THEN** the workflow and initial steps are persisted and the new card has no Save button.
- **GIVEN** an automatic save failure, **WHEN** the user selects Retry, **THEN** the last failed change is attempted again and the status updates from Saving to Saved on success.
- **GIVEN** a 390px-wide viewport, **WHEN** the workflow settings page and a step editor are open, **THEN** all required controls are fully inside the viewport and the document has no horizontal overflow.

## Out of Scope

- Atomic multi-step transactions or version-conflict resolution across browser tabs.
- Undo history for workflow changes.
- Changes to workflow or workflow-step HTTP contracts.

## Implementation Plan

See [the completed implementation plan](../../plans/workflow-settings-autosave/plan.md).
