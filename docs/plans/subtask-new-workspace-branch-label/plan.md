---
spec: docs/specs/office/requirements/tasks.md
created: 2026-08-11
status: done
---

# Implementation Plan: Correct Isolated Subtask Branch Messaging

## Overview

The subtask dialog currently carries a legacy parent-worktree badge into the
`new_workspace` rendering path whenever the selected repository matches the
parent repository. The backend payload and materialization behavior are already
correct. This repair removes the misleading badge from the isolated path,
preserves it for `inherit_parent`, and adds focused desktop and mobile
regressions.

---

## Backend

No backend, API, persistence, or worktree behavior changes. The existing
`workspace_mode` and repository payload remain authoritative.

---

## Frontend

### Subtask workspace section

- Update `apps/web/components/task/new-subtask-form-parts.tsx` so
  `WorkspaceSection` renders `WorktreeBadge` only for `inherit_parent`.
- Remove `showWorktreeBadge`, `shouldShowWorktreeBadge`, and the
  `parentRepositoryId` plumbing that exists only to display the parent branch in
  the `new_workspace` path.
- Update `apps/web/components/task/new-subtask-dialog.tsx` only where needed to
  stop passing the obsolete presentation prop. Keep parent repository seeding,
  selected base branches, submission data, workspace-mode defaults, and labels
  unchanged.

### Mobile parity

This is a visibility correction inside the existing responsive subtask dialog;
it does not change composition, navigation, scrolling, safe-area behavior, or
touch targets. The nearest mobile exemplar is the existing flow in
`apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`: mobile session
menu, Tasks sheet, visible task-actions menu, then the shared New Subtask
dialog. That test will prove that switching to an isolated workspace removes
the parent branch indicator while leaving repository selection reachable.

### Public documentation

No public-doc edit is required. `docs/public/coordination.md` already states
that inherited children reuse the parent branch and that a new workspace is
isolated with its own selected branch. The code will be aligned with that
published contract.

---

## Tests

- **What:** `inherit_parent` shows the active parent branch indicator, while
  `new_workspace` does not show it even when the same repository remains
  selected.
- **File:** `apps/web/components/task/new-subtask-form-parts.test.tsx`.
- **How:** render the shared form body for both workspace modes with an active
  parent worktree branch. The `new_workspace` assertion must fail against the
  current code before the component change, then pass afterward. Also assert
  that the repository selector remains visible in isolated mode.

No service integration test is required because the request payload and backend
materialization contract do not change.

---

## E2E Tests

- **Desktop scenario:** Given a parent with an active worktree, when the user
  opens New Subtask and selects **Create new workspace**, the parent branch
  indicator disappears and the repository/base-branch controls remain visible.
  Extend `apps/web/e2e/tests/task/subtask.spec.ts`.
- **Mobile scenario:** Given the same parent reached from the mobile Tasks
  sheet, when the user selects **Create new workspace**, the same messaging and
  repository-control behavior is visible in the shared dialog. Extend
  `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`.

---

## Verification Results

Passed. Task 01's focused component test, typecheck, and i18n ratchet passed.
Task 02's rebuilt production web bundle and focused desktop and Pixel 5 mobile
Playwright scenarios passed. The E2E runs confirmed that inherited workspaces
show the parent branch indicator, while isolated workspaces hide it and keep
the repository and base-branch controls visible.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Correct branch messaging](task-01-correct-branch-messaging.md)

Wave 2:

- [x] [Task 02: Prove responsive behavior](task-02-prove-responsive-behavior.md)

Both tasks are sequential. Task 02 depends on the corrected UI contract from
Task 01. The plan does not authorize subagent execution.

## Risks

- Removing the new-workspace-only parent branch badge must not remove
  `parentRepositoryId` from repository seeding in `NewSubtaskForm`.
- Assertions must target the branch indicator text or its scoped element, not a
  raw branch string that can also appear in the repository/base-branch picker.

## Out of scope

- Changing which workspace mode is selected by default.
- Changing the base branch sent during subtask creation.
- Copying uncommitted parent work into an isolated workspace.
- Changing worktree branch generation or executor behavior.
