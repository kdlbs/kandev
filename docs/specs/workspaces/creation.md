---
status: building
created: 2026-07-23
owner: Kandev
---

# Kanban workspace creation

## Why

A newly created Kanban workspace can currently contain no workflows, which leaves the task-create
flow without a destination workflow or start step. Users should be able to create a task immediately
after creating a Kanban workspace.

## What

- Creating a Kanban workspace also creates one visible workflow named `Kanban`.
- The workflow uses the built-in Kanban template and includes its configured steps and automation.
- The workspace response remains the existing workspace response; clients discover the created
  workflow through the existing workflow list and real-time update surfaces.
- Office onboarding remains a separate creation path and continues to materialize its Office
  workflows without receiving an implicit Kanban workflow from this behavior.

## Data model

The feature reuses the existing `workspaces`, `workflows`, and `workflow_steps` entities. The
created workflow belongs to the new workspace through `workflows.workspace_id`, references the
built-in `simple` workflow template, and owns the template-derived workflow steps.

## API surface

The existing Kanban workspace creation contracts retain their request and workspace response shapes:

- `POST /api/v1/workspaces`
- WebSocket `workspace.create`

Successful creation makes the new workflow available through the existing workflow list contracts.
Office onboarding uses its existing service-level workspace creation path and is excluded.

## Failure modes

- If the workspace row cannot be created, creation fails as it does today.
- The workspace, default Kanban workflow, and template-derived steps are committed in one
  transaction. If any insert fails or the request is canceled, the transaction rolls back and none
  of those rows become visible.

## Persistence guarantees

The default workflow and its steps are stored with the workspace and survive process restarts.
This is creation-time behavior only; existing empty workspaces are not backfilled.

## Scenarios

- **GIVEN** a user creates a Kanban workspace, **WHEN** creation succeeds, **THEN** listing workflows
  for that workspace returns one visible `Kanban` workflow created from the built-in Kanban template.
- **GIVEN** the newly created Kanban workflow, **WHEN** its steps are listed, **THEN** the
  template-derived start step and remaining Kanban steps are available so a task can be created
  immediately.
- **GIVEN** Office onboarding creates an Office workspace, **WHEN** onboarding materializes its
  workflows, **THEN** no extra implicit Kanban workflow is added by Kanban workspace creation.
- **GIVEN** Kanban bootstrap fails or the request is canceled before commit, **WHEN** persistence
  rolls back, **THEN** no workspace, workflow, or partial step from that attempt is visible.
- **GIVEN** an existing empty workspace from an earlier version, **WHEN** Kandev starts after this
  change, **THEN** the workspace remains unchanged.

## Out of scope

- Backfilling workflows into existing empty workspaces.
- Letting users choose a different default template during workspace creation.
- Changing Office onboarding or Office workflow defaults.
