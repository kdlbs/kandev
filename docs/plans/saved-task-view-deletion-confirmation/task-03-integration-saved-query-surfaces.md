---
id: "03-integration-saved-query-surfaces"
title: "Integration saved-query surfaces"
status: done
wave: 2
depends_on:
  - "01-shared-confirmation-shell"
plan: "plan.md"
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
acceptance_criteria:
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.1
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.2
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.3
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.4
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.5
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.6
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.7
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.8
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.9
system_design:
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
---

# Task 03: Integration Saved-Query Surfaces

## Summary

Connect the shared confirmation shell to the shared integration scope bar,
GitHub and GitLab mobile sidebars, and Jira saved views. Preserve every
surface's existing callback, default-marker, built-in, and failure semantics.

## In scope

- Add captured target and nested-boundary handling to `SavedMenu` so GitHub,
  GitLab, and Azure DevOps inherit fine/coarse confirmation.
- Keep GitHub default actions and pending state isolated from delete; keep
  Cancel available if pending state begins after confirmation opens.
- Replace the target row in GitHub and GitLab mobile sheets with touch-density
  inline confirmation.
- Make GitLab and Jira coarse-pointer delete triggers visible, accessible, and
  at least 44px without changing row selection.
- Add Jira custom-view confirmation while leaving built-ins nondeletable.
- Add focused tests for every adapter, cancellation path, exact ID, pending
  guard, built-in guard, and event isolation.

## Out of scope

- Integration settings models, saved-query hooks, workspace/user persistence,
  provider requests, and rollback behavior.
- GitHub default-view semantics or the shared scope bar's selection behavior.
- External-plugin-only UI and browser-level verification owned by Task 04.

## Acceptance

- Every in-tree integration delete trigger opens a named confirmation and calls
  its original callback exactly once only after explicit confirmation.
- Delete actions never select a saved query, toggle GitHub's default marker, or
  become available for Jira built-ins or during an existing pending mutation.
- Fine-pointer parents remain mounted around the anchored confirmation; coarse
  rows expose visible 44px controls and inline content without overflow.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  components/integrations/presets-scope-bar-base.test.tsx \
  components/github/my-github/presets-sidebar.test.tsx \
  components/gitlab/my-gitlab/presets-sidebar.test.tsx \
  components/jira/my-jira/list-toolbar.test.tsx
pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/integrations/presets-scope-bar-base.tsx`
- `apps/web/components/integrations/presets-scope-bar-base.test.tsx`
- `apps/web/components/github/my-github/presets-sidebar.tsx`
- `apps/web/components/github/my-github/presets-sidebar.test.tsx`
- `apps/web/components/gitlab/my-gitlab/presets-sidebar.tsx`
- `apps/web/components/gitlab/my-gitlab/presets-sidebar.test.tsx`
- `apps/web/components/jira/my-jira/list-toolbar.tsx`
- `apps/web/components/jira/my-jira/list-toolbar.test.tsx`

## Dependencies

Task 01 (`SavedTaskViewDeleteConfirmation` and shared copy/boundary contract).

## Risks

- The shared scope bar runs inside Radix dropdown behavior that changes to a
  bottom sheet on coarse pointers; event prevention must work in both modes.
- GitHub rows have three independent actions. Replacing or anchoring delete
  must not alter saved-query selection or default mutation state.
- Jira uses one responsive popover rather than a dedicated mobile drawer, so
  inline replacement must fit its current scroll and width constraints.

## Parallelism

`parallel-safe` with Task 02 after Task 01 completes.

## Inputs

- Requirement acceptance criteria `.1` through `.9`.
- System-design sections Surface inventory, Surface adapters, Control flow, and
  Responsive and accessibility behavior.
- Existing shared scope-bar test, GitHub sidebar test, saved-layout nested
  confirmation pattern, and each integration's saved-query hook.

## Results

- Added captured confirmation to the shared GitHub, GitLab, and Azure DevOps
  scope menu while preserving menu selection and GitHub default mutation state.
- Added inline, touch-sized confirmation to GitHub and GitLab mobile sidebars.
- Added responsive Jira confirmation with built-in view protection and visible
  coarse-pointer deletion controls.
- Focused integration-surface tests passed 15/15; the combined confirmation
  suite passed 50/50 and web typecheck passed.
