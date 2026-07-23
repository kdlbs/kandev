---
spec: docs/specs/gitlab-integration/spec.md
created: 2026-07-22
status: approved
---

# Implementation Plan: GitLab Task Link Menu

## Overview

Align GitLab task linking with the existing GitHub interaction: unlinked tasks
do not reserve top-bar space, and linking starts from the task's contextual
`Link` submenu. Reuse the existing GitLab MR dialog and task-menu plumbing, then
update desktop and mobile E2E coverage to prove the new entry points.

## Frontend

### Top-bar behavior

- Update `apps/web/components/gitlab/mr-topbar-button.tsx` so an unlinked task
  renders no generic link action.
- Preserve the linked-MR status/dropdown control, including `Link another merge
  request`, open, and unlink behavior.

### Shared task link menu

- Extend `apps/web/components/kanban-card-menu-items.tsx` and
  `apps/web/components/task/task-switcher-context-menu.tsx` with a
  `GitLab Merge Request` link action.
- Wire the existing `TaskMRLinkDialog` through kanban-card and sidebar/mobile
  task-switcher link state. Surface the action only while GitLab is configured
  for the active workspace.
- Reuse the task's repository associations and the existing workspace
  repository list; do not add a new API or duplicate link submission logic.

### Mobile design contract

- **Desktop outcome:** right-clicking a task opens `Link` > `GitLab Merge
  Request`; submitting the existing dialog links the MR and reveals its status
  control in the task top bar.
- **Mobile entry point:** the visible `Task actions` ellipsis in the existing
  task-switcher drawer opens the same contextual menu; no long press is needed.
- **Nearest exemplar:**
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` and
  `apps/web/components/task/task-item.tsx` provide the discoverable ellipsis,
  responsive menu treatment, and touch geometry.
- **Hierarchy and presentation:** task row > actions menu > `Link` submenu >
  `GitLab Merge Request` > existing link dialog. This is a short secondary
  action, so the responsive context-menu bottom sheet plus existing dialog is
  preferable to a new route or full-height surface.
- **Scrolling and geometry:** retain the menu primitive's safe-area-aware,
  internally scrolling mobile surface and 44px menu rows. The task-switcher
  drawer remains the outer list scroll owner while the portalled menu is open.
- **Shared logic:** desktop and mobile use the same availability hook, menu
  callbacks, target state, repository resolution, and `TaskMRLinkDialog`.

## Tests

- **What:** the shared link-menu model includes `GitLab Merge Request` only
  when its callback is present.
  **File:** `apps/web/components/kanban-card-menu-items.test.tsx`.
  **How:** extend the existing menu-entry unit tests and assert ordering and
  omission.
- **What:** a task-switcher link selection closes the menu and targets the
  selected task.
  **File:** `apps/web/components/task/task-switcher.test.tsx` or a focused
  colocated test if the existing helper coverage is sufficient.
  **How:** exercise the existing provider-neutral selection helper with the new
  callback plumbing.
- **What:** the GitLab top bar renders no generic action without linked MRs and
  keeps the linked-MR control behavior.
  **File:** `apps/web/components/gitlab/mr-topbar-button.test.ts` or a focused
  component test where practical.
  **How:** prefer testing extracted render-policy logic; do not add broad React
  component tests solely for markup.

## E2E Tests

- **Desktop scenario:** update
  `apps/web/e2e/tests/gitlab/gitlab-parity.spec.ts` so an unlinked task has no
  `Link MR` top-bar button, then right-click the task row, choose `Link` >
  `GitLab Merge Request`, submit the dialog, and verify the linked-MR status
  control appears and survives reload.
- **Mobile scenario:** extend
  `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts` or the focused mobile
  task-link menu spec so the visible `Task actions` ellipsis opens the menu,
  the nested GitLab action is touch reachable and viewport-contained, and the
  dialog opens without right-click or long press.

## Implementation Waves

Wave 1:

- [x] [task-01-frontend-link-menu](task-01-frontend-link-menu.md)

Wave 2:

- [x] [task-02-e2e-contextual-linking](task-02-e2e-contextual-linking.md)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/kanban-card-menu-items.test.tsx components/task/task-switcher.test.tsx components/gitlab/mr-topbar-button.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/gitlab/gitlab-parity.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts
make fmt
make typecheck test lint
```

## Verification Status

- Targeted desktop and mobile GitLab Playwright coverage passes (6/6), including
  validated screenshots, mobile touch targets, and viewport containment.
- Aggregate format, typecheck, unit, and lint checks pass. Aggregate tests pass
  with `TMPDIR=/tmp`.

## Risks

- Task menu props cross kanban, desktop sidebar, and mobile task-switcher
  surfaces; missing one caller would make the action viewport-dependent.
- GitLab availability is workspace-scoped. The action must disappear when the
  active workspace has no configured GitLab connection.
- Removing the generic top-bar entry invalidates the existing manual-link E2E
  steps; those must be changed rather than deleted.
