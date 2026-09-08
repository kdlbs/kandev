---
spec: docs/specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md
created: 2026-08-13
status: implemented
---

# Implementation Plan: Task-create dependency selector refinement

## Overview

Replace the task-create dialog's separate dependency action and chips with one
searchable, multi-select control that matches the agent and executor selector
visual language. Pair it with the workflow selector on desktop and keep the
same data derivation and `blocked_by` submission path. Add focused component
tests plus desktop and mobile browser coverage for selection, teaching copy,
placement, and viewport containment.

This is a frontend-only refinement. It does not change dependency persistence,
the task API, or the existing `blockedBy` state shape.

## Frontend

### Dependency selector

- `apps/web/components/task-create-dialog-dependencies.tsx`
  - Replace the separate label, Add dependency button, and removable chips with
    one full-width selector trigger.
  - Preserve `useBoardTasks`, archived-task filtering, and both board-slice
    sources.
  - Keep multi-predecessor toggling in `blockedBy`, add the `No dependency`
    clearing entry, task icons, selected-state indicators, and localized
    trigger summaries.
  - Add the search-row info action with an accessible tooltip/help message and
    stable test IDs for the trigger, dropdown, task entries, and help control.

### Workflow/dependency layout

- `apps/web/components/task-create-dialog.tsx`
  - Stop rendering the dependency control as an independent block below the
    agent and executor row.
- `apps/web/components/task-create-dialog-form-body.tsx`
  - Render the workflow and dependency controls in a shared responsive row.
  - Keep the dependency selector visible when the workflow selector is hidden
    for a single enforced workflow or a locked workflow.
  - Stack workflow first and dependency second below the mobile breakpoint.

### Localization

- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json` (regenerated with the repository
  pseudo-locale script)

Add the default label, search placeholder, empty result, dependency count, and
help text. Keep all copy in the task namespace and remove or leave legacy keys
only after checking for other consumers.

## State and API

No state shape, API client, backend, persistence, or WebSocket changes. The
selector continues to call `onChange` with the complete predecessor ID array,
and `buildCreateTaskPayload` continues to emit `blocked_by` only when the array
is non-empty.

## Tests

- **What:** The dependency selector shows its no-dependency default, toggles
  several task entries, clears all entries, excludes archived tasks, renders
  task icons, and exposes the info help control.
  **File:** `apps/web/components/task-create-dialog-dependencies.test.tsx`
  **How:** Render the component with mocked Kanban board and snapshot state;
  interact with the command menu and assert `onChange` values and visible
  labels/test IDs.
- **What:** Workflow and dependency controls share the desktop row while the
  dependency control remains visible without a workflow trigger.
  **File:** `apps/web/components/task-create-dialog-form-body.test.tsx`
  **How:** Render `WorkflowSection` or the extracted paired row with both a
  visible workflow and a single enforced workflow; assert the dependency slot
  remains present and ordering is stable.
- **What:** Existing create payload behavior remains intact for multiple
  predecessors.
  **File:** `apps/web/components/task-create-dialog-helpers.test.ts`
  **How:** Retain and, if needed, extend the pure payload test for two
  `blockedBy` IDs and the omitted empty-array case.

## E2E Tests

- **Scenario:** GIVEN two non-archived predecessor tasks, WHEN the user opens
  the desktop create dialog, searches for both, and toggles both entries, THEN
  the single dependency trigger shows the selected count and the created task
  reports both predecessor IDs.
  **File:** `apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts`
  **What to verify:** Default `No dependency` state, workflow/dependency row,
  task icons, search, info help, multi-selection, and `blocked_by` persistence
  after UI submission.
- **Scenario:** GIVEN a Pixel 5 viewport, WHEN the user opens the task-create
  dialog and dependency picker, THEN the selector and help action are
  touch-usable, the picker is contained, and the document has no horizontal
  overflow.
  **File:** `apps/web/e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`
  **What to verify:** Full-width mobile trigger, `.tap()` selection, readable
  help content, picker containment, 44px active controls, and overflow safety.

## Mobile parity contract

- Desktop outcome: choose zero or more predecessor tasks while creating a task.
- Mobile entry point: the dependency selector in the create dialog's form,
  immediately after the workflow context.
- Closest exemplar: mobile task-create repository picker / `Pill` command
  surface for contained search, touch targets, and overflow checks.
- Mobile primary action: open the dependency selector and toggle task rows.
- Presentation: one full-width trigger and a contained searchable popover; no
  desktop-only hover path is required for selecting a dependency.
- Scroll ownership: the command list owns long candidate scrolling; the dialog
  body owns form scrolling when the popover is closed.
- Shared logic: task candidates, archived filtering, selected IDs, and payload
  state are shared between desktop and mobile.

## Verification Results

- Focused frontend unit tests passed: `pnpm --filter @kandev/web test --
  task-create-dialog-dependencies task-create-dialog-form-body
  task-create-dialog-helpers` (4 files, 62 tests).
- Frontend typecheck passed: `pnpm run typecheck` from `apps/web`.
- Focused frontend and E2E-file ESLint passed with `--max-warnings 0`.
- `pnpm run i18n:check` passed with the repository's existing real-locale
  parity advisories; `pnpm run i18n:ratchet` passed with zero new violations.
- `git diff --check` passed.
- Desktop managed E2E passed: `pnpm e2e:run --project chromium
  e2e/tests/task/create-task-dependency-selector.spec.ts` (1 test).
- Mobile managed E2E passed: `pnpm e2e:run --project mobile-chrome
  e2e/tests/task/mobile-create-task-dependency-selector.spec.ts` (1 test).
- The first mobile run found a 43.56px rem-based trigger; the final 48px
  mobile control sizing and rerun passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [Task 01: Implement the selector and layout](task-01-selector-ui.md)

Wave 2 (sequential, depends on Task 01):

- [x] [Task 02: Verify desktop and mobile flows](task-02-selector-e2e.md)

No task is parallel-safe because the browser coverage depends on the final
selector test IDs and responsive composition.

## Open Questions

None. The selector preserves multiple predecessor selection after user
confirmation.
