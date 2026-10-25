---
spec: docs/specs/tasks/requirements/sidebar-task-edit.md
created: 2026-08-03
status: complete
---

# Implementation Plan: Sidebar Task Editing

## Overview

Extend the shared sidebar `TaskSwitcher` single-task menu with an Edit callback, then route both the desktop sidebar and phone/tablet task switcher through one small edit controller and the existing `TaskCreateDialog` in edit mode. The implementation reuses current task snapshots and workflow-step data, leaves Rename intact, and relies on the existing `PATCH /api/v1/tasks/:taskId` plus `task.updated` reconciliation. No backend, schema, or public API changes are required.

## Frontend

### Shared single-task menu contract

- Update `apps/web/components/task/task-switcher.tsx` so `TaskSwitcher`, `TaskRow`, and `TaskItemWithContextMenu` can receive `onEditTask(task)` for the exact `TaskSwitcherItem` whose menu was opened.
- Update `apps/web/components/task/task-switcher-context-menu.tsx` to render a localized **Edit** item next to **Rename** for one live task. Keep it out of the existing multi-selection menu and omit it for archived or workflow-less synthetic rows; use the existing processing state to disable it while deletion is in progress.
- Add `common:edit` to `apps/web/src/locales/en/common.json` and resolve it at render time with `useTranslation()`.
- Regenerate `apps/web/src/locales/pseudo/common.json` as the checked-in locale synchronization artifact.

### Shared edit controller and dialog

- Add `apps/web/components/task/task-session-sidebar-edit.tsx` containing:
  - a pure target builder that combines the selected `TaskSwitcherItem`'s workflow identity with the authoritative task from `findTaskInSnapshots()`;
  - `useSidebarTaskEdit()`, which resolves the clicked task from `kanbanMulti.snapshots` with the active `kanban.tasks` fallback and owns the open edit target;
  - `SidebarTaskEditDialog`, which renders the existing `TaskCreateDialog` with `mode="edit"`, the target workflow and step list, and matching `editingTask` / `initialValues` fields.
- Do not navigate or change `tasks.activeTaskId` when opening or saving. Let the existing edit submit path and `task.updated` handler update both active Kanban state and multi-workflow snapshots.
- If the target cannot be resolved or lacks workflow/step metadata, do not open the dialog.

### Desktop sidebar wiring

- Update `apps/web/components/task/task-session-sidebar.tsx` to compose `useSidebarTaskEdit()` into `useSidebarActions()`.
- Update `apps/web/components/task/task-session-sidebar-switcher-props.ts` to pass `handleEditTask` into the shared switcher.
- Update `apps/web/components/task/task-session-sidebar-dialogs.tsx` to host `SidebarTaskEditDialog` with the active workspace and `stepsByWorkflowId`; closing it clears only the edit target.

### Phone and tablet wiring

- Update `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` to use the same edit controller, pass Edit through `MobileTaskList`, and render the shared edit dialog with `useSheetData().stepsByWorkflowId`.
- Wrap the edit callback with the existing `surfaceAction()` policy: the phone `Drawer` closes before the editor opens, while the tablet `Sheet` stays mounted behind it.

## Mobile design contract

- **Desktop outcome / mobile entry:** desktop uses right-click on a sidebar row; phone and tablet use the row's visible **Task actions** control.
- **Nearest shipped exemplar:** `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` supplies the existing inset bottom `Drawer`, wider `Sheet`, action handoff, safe-area padding, and single internal scroll owner. `apps/web/components/task/task-switcher-context-menu.tsx` supplies the shared responsive menu primitive.
- **Hierarchy and primary action:** selecting a task remains the row's primary action. Edit stays a secondary action in the explicit overflow menu and opens the established focused task editor.
- **Presentation rationale:** editing is a focused, potentially multi-field operation, so it uses the existing modal editor. The temporary phone task navigator closes first to avoid stacked bottom surfaces; the roomier tablet sheet retains context behind the modal.
- **Geometry:** keep the drawer's `88dvh` bound, safe-area padding, and internal task-list scroll. The existing mobile menu treatment owns viewport containment and at least 44px action rows; no document-level horizontal scrolling is added.
- **Shared logic:** target resolution, form values, validation, submission, and task state are shared. Only the phone/tablet surface-dismissal behavior remains presentation-specific.

## Tests

- **What:** one eligible task exposes Edit and invokes the callback with the exact clicked task, while multi-selection and archived rows do not expose it.
  - **File:** `apps/web/components/task/task-switcher.test.tsx`
  - **How:** render the shared switcher, open the Radix context menu, invoke Edit, and assert callback/visibility rules.
- **What:** the sidebar edit target uses the clicked task's workflow and authoritative snapshot fields, refuses incomplete targets, and supplies the existing dialog with correct edit props and workflow-specific steps.
  - **File:** `apps/web/components/task/task-session-sidebar-edit.test.tsx`
  - **How:** unit-test the pure target builder and render the dialog with a mocked `TaskCreateDialog`.
- **What:** the touch task list forwards Edit through the same shared menu contract.
  - **File:** `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`
  - **How:** open the visible task-actions button, select Edit, and assert the target task is passed.
- Existing `TaskCreateDialog` unit tests remain the authority for edit validation and started-task field locks; this feature does not duplicate that form logic.

## E2E Tests

- **Scenario:** GIVEN a non-active desktop sidebar task, WHEN Edit is opened, saved, and the editor closes, THEN the route stays on the original task while the target's persisted title and sidebar row update.
  - **File:** `apps/web/e2e/tests/task/sidebar-layout.spec.ts`
  - **What to verify:** Edit is present alongside Rename; target values are prefilled; save persists; route and active task are unchanged.
- **Scenario:** GIVEN a started task in the phone task drawer, WHEN Edit is selected, THEN the drawer closes, the title stays editable, the prompt stays locked, and saving is visible after reopening the drawer.
  - **File:** `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
  - **What to verify:** visible task-actions entry, responsive menu row, no stacked drawer, lifecycle locks, persistence, and updated sidebar copy.
- **Scenario:** GIVEN the tablet task-switcher sheet, WHEN Edit is opened and canceled, THEN the sheet remains visible with the unchanged task row.
  - **File:** `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
  - **What to verify:** tablet sheet retention and cancel behavior.
- Extend the existing mobile action-menu geometry assertion in `mobile-sidebar-task-actions.spec.ts` to include **Edit**, retaining viewport containment, internal scrolling, and 44px row coverage after the menu grows.

## Verification Results

- Focused unit tests followed Red-Green-Refactor: the pre-implementation run reported 2 expected Edit-menu failures with 16 passing tests; the final run passed 24 tests across the three focused files, including unresolved-source, missing-workflow, and selection-clear guards added during PR review.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run i18n:check` and `pnpm run i18n:ratchet` passed after regenerating the pseudo locale.
- Targeted ESLint for all changed frontend files passed with no warnings; `git diff --check` passed.
- `cd apps/web && pnpm run build` passed.
- Managed Playwright passed 9 Chromium sidebar tests and 10 mobile-chrome sidebar/action tests. The final capture runs also passed 2 Chromium and 2 mobile-chrome tests and produced four validated, compressed screenshots under the ignored `.pr-assets/` directory.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Add the shared sidebar edit flow](task-01-shared-sidebar-edit-flow.md)

Wave 2:

- [x] [Task 02: Prove desktop and mobile edit parity](task-02-sidebar-edit-e2e.md) — depends on Task 01.

The tasks are sequential because Task 02 exercises the UI and selectors introduced by Task 01. No subagent use is implied by this plan.

## Risks

- Sidebar rows aggregate tasks across workflows; using the globally active workflow would initialize the wrong step list. The edit target must retain the clicked row's workflow identity.
- The synthetic archived row is not backed by a complete editable task snapshot. It must not expose Edit.
- Phone action selection must close the task drawer before opening the dialog, while tablet behavior intentionally differs; reusing `surfaceAction()` preserves that breakpoint contract.
- The menu already has substantial mobile height. Adding Edit must preserve bottom-sheet containment, internal scrolling, and access to destructive actions.
- Task updates arrive through the existing WebSocket reconciliation path; tests should assert eventual sidebar state and must not add a second competing cache-update implementation.
