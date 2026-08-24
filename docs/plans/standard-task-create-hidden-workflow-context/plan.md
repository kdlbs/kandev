---
spec: docs/specs/workspaces/requirements/improve-kandev.md
created: 2026-07-29
status: implemented
---

# Implementation Plan: Standard Task Creation From Hidden Workflow Context

## Overview

The task-detail boot payload correctly exposes the current task's hidden
workflow so the task can render, but standard New Task entry points pass that
context straight into the shared task-create dialog. The dialog treats it as an
explicit selection, so a task created while viewing an Improve Kandev task can
silently inherit the hidden Improve workflow.

Normalize the workflow context at the shared task-create boundary: ordinary
task creation must discard a hidden contextual workflow and fall back to the
workspace's visible workflows, while a feature wrapper with a locked workflow
continues to preserve its explicit hidden workflow. Fetch the visible
workflow's steps when the normalized fallback differs from the caller-provided
context, then prove the behavior through unit and desktop/mobile E2E coverage.

## Frontend

### Shared workflow-context normalization

- Add a pure workflow-context resolver in
  `apps/web/components/task-create-dialog-defaults.ts`.
- When the caller-provided workflow is hidden and
  `lockedFields.workflow !== true`, clear the provided workflow and default
  step before initializing, resetting, or submitting the dialog. This lets the
  existing visible-only single-workflow fallback choose the workspace's Kanban
  workflow.
- Preserve visible workflows and preserve explicitly locked hidden workflows,
  including the Improve Kandev wrapper.
- Apply the normalized workflow/default-step context consistently in
  `apps/web/components/task-create-dialog.tsx`; do not change the task-detail
  boot state because it still needs the hidden workflow to render the task.

### Fallback step loading

- Extend the workflow-step effect in
  `apps/web/components/task-create-dialog-effects.ts` to load steps for the
  computed effective workflow when it differs from the normalized
  caller-provided workflow.
- Update the default-step resolver in
  `apps/web/components/task-create-dialog-defaults.ts` so fetched steps for the
  effective fallback supply its start/first step. Do not reuse the hidden
  workflow's step ID.
- Keep the existing explicit workflow-switch behavior unchanged.

### Mobile design contract

- Desktop entry point remains the app-sidebar **New Task** action.
- Mobile entry point remains the **New** action in
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`, the nearest
  shipped mobile exemplar.
- The existing task drawer, task-create dialog, hierarchy, scroll ownership,
  safe-area behavior, and touch targets do not change. Desktop and mobile share
  the same workflow normalization and step-loading logic.
- Mobile Playwright coverage creates a task from the existing task drawer while
  viewing a hidden-workflow task and verifies the visible workflow receives it.

## Tests

- **What:** an unlocked hidden contextual workflow is removed, while a locked
  hidden workflow and an ordinary visible workflow are preserved.
  **File:** `apps/web/components/task-create-dialog-defaults.test.ts`.
  **How:** table-driven pure-function tests.
- **What:** a visible fallback workflow that differs from the provided context
  fetches its steps and resolves the visible start step.
  **Files:** `apps/web/components/task-create-dialog-effects.test.ts` and
  `apps/web/components/task-create-dialog-defaults.test.ts`.
  **How:** focused hook and pure-function tests that fail against the current
  selected-workflow-only loading rule.

## E2E Tests

- **Scenario:** GIVEN a task in a hidden workflow, WHEN the user opens the
  desktop sidebar New Task dialog and creates a task, THEN the created task's
  workflow step belongs to the visible Kanban workflow.
  **File:** `apps/web/e2e/tests/task/create-task.spec.ts`.
  **What to verify:** seed a hidden workflow and task, navigate directly to the
  task, create without an agent through the sidebar, and query the created task
  to compare `workflow_step_id` with the visible workflow's start step.
- **Scenario:** GIVEN the same hidden task on a phone viewport, WHEN the user
  opens the mobile task drawer, taps **New**, and creates a task, THEN the task
  uses the visible workflow.
  **File:** `apps/web/e2e/tests/task/mobile-hidden-workflow-task-create.spec.ts`.
  **What to verify:** drive the existing drawer and dialog with touch-oriented
  interactions, then assert the created task's workflow step and absence of
  document horizontal overflow.

## Implementation Task

- [x] [task-01-normalize-task-create-workflow](task-01-normalize-task-create-workflow.md)
  — done

The task performs unit and desktop/mobile E2E RED runs before changing
production code, then implements and reruns the same focused checks GREEN.
After the targeted checks pass, run the workflow-requested repository gates in
order (`make fmt`, then `make typecheck test lint`) and commit with a
Conventional Commit message.

## Risks And Out Of Scope

- Hidden workflows remain stored in SQLite and present in task-detail boot
  state; they are not database corruption.
- This fix does not change workflow visibility APIs, migrations, workflow
  persistence, or the Improve Kandev bootstrap contract.
- The linked task ID is absent from the current database, so its original
  request cannot be reconstructed from persisted rows; the current task and
  task-detail boot payload reproduce the same failure path.
