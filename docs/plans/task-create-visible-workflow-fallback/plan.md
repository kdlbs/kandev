---
spec: docs/specs/workspaces/requirements/improve-kandev.md
created: 2026-07-28
status: implemented
---

# Implementation Plan: Task-create visible workflow fallback

## Overview

Fix the standard task-create dialog so its implicit single-workflow selection
uses visible workflows rather than every workflow in the global boot store. The
confirmed regression occurs when the store contains one visible Kanban workflow
plus the hidden Improve Kandev workflow: fallback selection sees two rows and
returns no workflow, while the picker later hides Improve Kandev and renders a
redundant one-option selector.

No backend, persistence, or API changes are required.

## Frontend

### Workflow fallback

- Extend the workflow candidate shape used by
  `apps/web/components/task-create-dialog-defaults.ts` with the existing
  visibility field.
- Update `computeSingleWorkflowFallbackId` to derive fallback candidates from
  non-hidden workflows.
- Preserve explicit `selectedWorkflowId` and `workflowId` values, including the
  hidden workflow explicitly supplied by the dedicated Improve Kandev dialog.
- Leave picker rendering and hidden-workflow storage unchanged.

### Mobile design contract

- Desktop and mobile keep the existing New Task dialog entry points and
  composition.
- The nearest mobile path is the Kanban New Task FAB flow in
  `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`; both viewports share the
  same fallback helper and form state.
- This is state/data normalization only: it changes neither navigation,
  overlays, scrolling, touch targets, nor responsive composition. No
  mobile-specific state or persisted fallback is introduced.

## Tests

- **What:** one visible workflow plus a hidden workflow resolves to the visible
  workflow when no explicit selection exists.
  **File:** `apps/web/components/task-create-dialog-defaults.test.ts`.
  **How:** focused pure-function unit test that fails against the current
  all-workflow count and passes after visibility filtering.
- Existing explicit-selection behavior remains covered by the helper's
  short-circuit contract and targeted type checking.

## E2E Tests

- **Scenario:** GIVEN a workspace with one visible workflow and a hidden
  workflow, WHEN the user opens the standard New Task dialog, THEN the
  one-option workflow selector is absent.
- **File:** `apps/web/e2e/tests/task/create-task.spec.ts`.
- **What to verify:** seed the hidden workflow through the existing E2E API,
  reload so the boot payload contains both workflows, open New Task, and assert
  that the workflow selector trigger is not rendered.
- No separate mobile E2E is added because the change is pure shared-state
  normalization with no viewport-specific behavior; the focused unit test
  exercises the shared decision used by both layouts.

## Implementation Tasks

- [x] [task-01-visible-workflow-fallback](task-01-visible-workflow-fallback.md)
  — done

## Validation

- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-defaults.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run tests/task/create-task.spec.ts -- --grep "single visible workflow"`

## Risks And Out Of Scope

- Hidden workflows remain intentionally absent from Settings and the standard
  picker.
- Explicit hidden workflows supplied by dedicated feature entry points remain
  valid.
- The fix does not change workflow persistence, boot payload contents, task
  creation APIs, or responsive layout.
