---
created: 2026-09-03
status: complete
requirements:
  - REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001
system_design:
  - ../../specs/tasks/system-design/task-dependency-detail-editing.md
legacy_specs: []
---

# Implementation Plan: Edit task dependencies

## Overview

Add dependency selection to the existing Edit task dialog. Keep the task-detail
dependency chip read-only. Add an atomic replacement route so Cancel and Update
retain their existing meanings.

The implementation has two sequential work orders. Both use TDD.

## Scope

### In scope

- Add an atomic task-scoped dependency replacement route.
- Search non-archived tasks in the edited task's workspace.
- Show current predecessors as selected in the Edit task dialog.
- Keep dependency selection as a draft until Update.
- Preserve dependencies when the user cancels.
- Add and remove direct predecessors through one replacement request.
- Keep the dialog open for cycle and request errors.
- Let existing WebSocket events update the Kanban board.
- Keep the task-detail dependency chip read-only.
- Provide desktop and touch layouts with the same user value.

### Out of scope

- Database migration, WebSocket, or MCP contract changes.
- Multi-task dependency edits.
- Cross-workspace edges.
- Changes to deferred launch intent.
- Changes to the Office task-properties picker.

## Technical approach

### Backend replacement contract

- Add `PUT /api/v1/tasks/:id/dependencies` with a complete predecessor list.
- Authorize and validate the complete desired set before mutation.
- Hold the existing dependency lock across validation and replacement.
- Apply the edge diff in one SQL transaction.
- Publish existing `task.updated` events for the edited task and changed peers.
- Keep the existing add and remove routes unchanged.

### Edit task dialog

- Add the dependency field only in edit mode and outside the create-only
  advanced-settings disclosure.
- Load the confirmed dependency projection when the dialog opens.
- Disable Update until the projection load finishes.
- Use `listTasksByWorkspace` for server-backed candidate search.
- Exclude the edited task and archived tasks.
- Keep selection changes in form state until Update.
- Add a typed replacement function and structured cycle parsing to the API
  client.
- Keep `TaskDependencyChip` unchanged in both task status rows.

### Desktop and mobile

- Reuse the current centered edit dialog on desktop.
- Reuse the current full-screen edit dialog on phones.
- Keep the task-switcher sheet behind the editor on tablets.
- Reuse task-create picker rows for search and draft selection.
- Use the form as the dialog's single vertical scroll owner.
- Keep touch controls at least 44 CSS pixels in the active dimension.
- Keep the document free of horizontal overflow.

### Localization

- Add editor labels, loading states, empty states, and errors to the task namespace.
- Update `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw` catalogs.
- Generate the pseudo locale and both Traditional Chinese catalogs with repository scripts.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.1` through `.3` | Dialog component tests for edit-mode visibility, initialization, and candidate filtering |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.4` and `.5` | Form-state tests and desktop cancel E2E |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.6` | Repository, service, and handler tests for atomic full-set replacement |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.7` | API client, dialog submit, WebSocket, and desktop E2E evidence |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.8` and `.9` | Backend rollback tests, dialog error tests, and desktop cycle E2E |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.10` and `.11` | Existing entry-point and dependency-chip tests plus focused regression tests |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.12` through `.14` | Desktop and mobile Playwright tests |
| `AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.15` | Backend authorization, cycle, and deferred-launch regression tests |

## E2E tests

- Extend `apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts` for
  desktop edit, update, cancel, and archived-task filtering.
- Extend `apps/web/e2e/tests/task/mobile-create-task-dependency-selector.spec.ts`
  for touch edit, update, dialog geometry, and overflow.
- Use the `chromium` project for desktop and `mobile-chrome` for mobile.

## Work orders

- [x] [Task 01: Add atomic dependency replacement](task-01-enable-detail-editing.md)
- [x] [Task 02: Add dependencies to Edit task](task-02-add-edit-dialog-dependencies.md)

## Verification results

Implemented and verified.

- Backend repository, service, and handler tests pass, including transaction
  rollback, authorization, cycle rejection, and update publication.
- Frontend focused tests pass with 68 tests across the edit dialog, submit
  flow, API client, dependency hook, and footer behavior.
- TypeScript typecheck, full frontend lint, i18n completeness, i18n ratchet,
  and the full backend test and lint suites pass.
- The desktop dependency-selector suite passes with 2 tests, and the mobile
  suite passes with 2 tests through the managed production E2E runner.
- Fresh desktop and mobile PR screenshots were captured, inspected, and
  compressed from the same E2E flows.

## Risks

- The task-field update and dependency replacement are separate HTTP requests.
  A dependency error must keep the dialog open and reload confirmed task fields.
- Candidate search can show stale results if a request from an old query wins a race.
- The cycle path contains task IDs. The UI must keep the error readable without changing server data.
