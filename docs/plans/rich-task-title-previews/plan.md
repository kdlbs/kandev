---
spec: docs/specs/tasks/requirements/rich-task-title-previews.md
created: 2026-08-18
status: complete
---

# Implementation plan: Rich task title previews

## Overview

Add one reusable title preview to the Kanban card, sidebar task row, and rich
task list. Reuse the existing task and contribution stores. Keep provider data
derivation separate from the shared change-request presentation.

## Backend

None. The feature uses existing task and contribution APIs.

## Frontend

### Preview and task hierarchy

`TaskTitleHoverCard` owns pointer and keyboard preview behavior.
`useTaskSubtasks` selects direct active subtasks. `TaskSubtaskRow` owns subtask
navigation and prevents its activation from reaching the parent row.

### Contribution status

`ChangeRequestTaskStatusSummary` owns the generic status-summary layout.
GitHub and GitLab components derive provider-specific rows and presentation data.

### GitLab state

`useWorkspaceMRs` shares one in-flight workspace request between mounted consumers.
It permits later refreshes and clears the workspace after failure.

## Tests

- **What:** pointer and keyboard preview behavior, subtask limits, and navigation isolation.
  **File:** `apps/web/components/task/task-title-hover-card.test.tsx`.
  **How:** component tests with a shared state provider.
- **What:** GitLab state derivation, request sharing, and failed-refresh cleanup.
  **File:** `apps/web/hooks/domains/gitlab/use-task-mr.test.ts` and GitLab summary tests.
  **How:** hook and pure-function tests with mocked API responses.
- **What:** provider-neutral status presentation.
  **File:** GitHub and GitLab status-summary render tests.
  **How:** render tests with provider-specific presentation data.

## E2E tests

- **Scenario:** A pointer opens a preview on each desktop task surface.
  **File:** `apps/web/e2e/tests/kanban/task-title-hover-subtasks.spec.ts` and
  `apps/web/e2e/tests/task/task-title-hover-surfaces.spec.ts`.
  **What to verify:** full title, direct subtasks, child navigation, and no overflow.
- **Scenario:** A coarse pointer opens the task directly.
  **File:** `apps/web/e2e/tests/kanban/mobile-task-title-hover-subtasks.spec.ts`.
  **What to verify:** no hover content and task-tree access through the mobile switcher.

## Verification results

- Seven focused Vitest files passed: 130 tests.
- TypeScript, ESLint, the locale audit, and the i18n ratchet passed.
- The desktop Chromium flow passed four tests.
- The mobile Chrome flow passed one test.
- `git diff --check` passed.

## Implementation order

- [x] [Task 01: Preview and hierarchy](task-01-preview-and-hierarchy.md)
- [x] [Task 02: Contribution status and state](task-02-contribution-status-and-state.md)
- [x] [Task 03: End-to-end verification](task-03-end-to-end-verification.md)
