---
created: 2026-09-06
status: complete
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
system_design:
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
legacy_specs: []
---

# Implementation Plan: Saved Task View Deletion Confirmation

## Overview

Add the shared localized confirmation shell first, connect the independent task
and integration surface adapters in parallel, then exercise their integrated
desktop and mobile behavior with production-build Playwright. This order gives
each surface one stable confirmation contract while leaving every existing
delete and persistence callback in place.

## Scope

### In scope

- Confirm deletion of task-sidebar views and Threads views.
- Confirm deletion of Jira saved views and GitHub, GitLab, and Azure DevOps
  saved queries in every Kandev-owned desktop and mobile entry point.
- Name the target and explain that tasks and connected-service data are safe.
- Use an anchored fine-pointer confirmation and in-place coarse-pointer
  confirmation with keyboard, focus, touch-target, safe-area, and overflow
  coverage.
- Preserve existing eligibility, default-marker, fallback, persistence,
  rollback, error, and parent-surface behavior.
- Add localized copy in all supported catalogs and pseudo-locale coverage.

### Out of scope

- Backend, API, user-settings schema, provider, or saved-view model changes.
- Confirmation for saved layouts, action presets, review watches, run filters,
  tasks, or unrelated saved artifacts.
- Bespoke UI maintained only in an external plugin repository.
- A preference to skip confirmation or a redesign of the owning surfaces.

## Technical approach

### Shared confirmation contract

- Add
  `apps/web/components/confirmation/saved-task-view-delete-confirmation.tsx`
  as a presentation-only adapter around `ActionConfirmPopover` and
  `InlineConfirmActions`.
- Pass a captured `{ id, label }` target, explicit `popover` or `inline`
  presentation, anchor/focus-boundary refs, pending state, and the existing
  delete callback. Keep mutation and parent-close behavior in callers.
- Reuse the existing `data-confirmation-boundary` contract and expose one
  predicate for parent Radix menus/popovers to ignore interaction originating
  inside the fine-pointer confirmation.
- Add shared `common` localization keys for the named title, consequence and
  reassurance, and Delete action. Update `en`, `pt-pt`, and `zh-cn`, then
  generate `zh-hk`, `zh-tw`, and `pseudo` with repository scripts.

### Task-owned surfaces

- In `SidebarFilterPopover` and `ViewHeaderRow`, replace immediate active-view
  deletion with a captured candidate. Render an anchored confirmation from the
  desktop popover and replace the drawer header action region on coarse
  pointers. Continue calling `deleteSidebarView` only after confirmation.
- In `ThreadsViewControls`, carry the active candidate through
  `ThreadsViewEditor` and `EditorActions`. Keep the current behavior that closes
  the desktop settings popover or mobile drawer after confirmed deletion.
- Use `sidebarViewName` and `threadViewName` for confirmation labels so built-in
  labels match the text shown to the user.
- Do not modify `sidebar-view-actions.ts`, `thread-view-actions.ts`, their
  persistence queues, last-view guards, or optimistic rollback.

### Integration-owned surfaces

- In `presets-scope-bar-base.tsx`, add one candidate per `SavedMenu`; anchor a
  fine-pointer confirmation to the selected delete item and replace that row
  inline when the shared dropdown uses coarse-pointer presentation. Preserve
  GitHub default-mutation disabling and prevent delete controls from selecting
  the query or toggling its default marker.
- In the GitHub and GitLab mobile `PresetsSidebar` components, replace the
  affected saved-query row with touch-density confirmation. Keep the GitHub
  default action separate; make GitLab's delete action visible and at least
  44px without hover.
- In Jira `ListToolbar`, add candidate state to `ViewsDropdown`; protect the
  parent popover around fine-pointer confirmation and replace the custom saved
  row inline for coarse pointers. Built-in rows remain nondeletable, and the
  coarse delete trigger becomes visible and touch reachable.
- Continue invoking each wrapper's existing `onDeleteSaved` or `onDeleteView`
  callback. Azure DevOps inherits the behavior through `IntegrationScopeBar`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-SAVED-TASK-VIEW-DELETION-001.2`, `.3`, `.4`, `.6`, `.7`, `.8`, `.10` | New `saved-task-view-delete-confirmation.test.tsx` covers named localized content, fine/inline routing, cancel, Escape, disconnected anchors, focus, destructive confirmation, and a single captured-ID callback. Existing `action-confirm-popover.test.tsx` retains nested-boundary and anchor-lifecycle coverage. |
| `AC-UI-SAVED-TASK-VIEW-DELETION-001.1`, `.3`, `.4`, `.5`, `.8` for task surfaces | `view-manager.test.tsx`, `sidebar-filter-popover.test.tsx`, new `threads-view-editor-actions.test.tsx`, and `threads-view-controls.test.tsx` cover no mutation before confirmation, cancel, exact confirmation, last-view guards, active labels, and current parent-close behavior. |
| `AC-UI-SAVED-TASK-VIEW-DELETION-001.1`, `.3`, `.4`, `.5`, `.8` for integration surfaces | `presets-scope-bar-base.test.tsx`, GitHub `presets-sidebar.test.tsx`, new GitLab `presets-sidebar.test.tsx`, and new Jira `list-toolbar.test.tsx` cover each adapter, built-in/pending guards, target IDs, and isolation from select/default actions. |
| `AC-UI-SAVED-TASK-VIEW-DELETION-001.7`, `.9` | Responsive component assertions and mobile Playwright inspect visible 44px controls, parent surface retention, safe-area reachability, long-copy containment, and zero document overflow. |

## E2E tests

| Flow | Acceptance criteria | Playwright evidence |
| --- | --- | --- |
| Open task-sidebar and Threads delete confirmations, cancel without changing settings, then confirm and observe the existing fallback/reload behavior. | `.1`, `.2`, `.3`, `.4`, `.5`, `.6`, `.8` | `tests/task/sidebar-filter.spec.ts` and `tests/task/threads-view.spec.ts` on `chromium` |
| Confirm saved-query deletion without selecting the row or changing a GitHub default; exercise the shared GitLab and Azure adapters and Jira custom-view guard. | `.1`, `.2`, `.3`, `.4`, `.5`, `.6`, `.8` | `tests/github/github-scope-bar.spec.ts`, `tests/gitlab/gitlab-issue-milestone-filter.spec.ts`, `tests/integrations/azure-devops.spec.ts`, and `tests/integrations/jira-status-filter.spec.ts` on `chromium` |
| Repeat every distinct coarse-pointer composition inside its existing drawer, sheet, dropdown, or editor. Check touch targets, one overlay/scroll owner, safe area, long labels, and overflow before confirming. | `.1`, `.2`, `.3`, `.4`, `.7`, `.8`, `.9`, `.10` | `tests/task/mobile-sidebar-views.spec.ts`, `tests/task/mobile-threads-view.spec.ts`, `tests/github/mobile-github-sidebar.spec.ts`, `tests/gitlab/mobile-gitlab-issue-milestone-filter.spec.ts`, `tests/integrations/mobile-azure-devops.spec.ts`, and new `tests/integrations/mobile-jira-saved-view.spec.ts` on `mobile-chrome` |

## Work orders

Wave 1:

- [x] [Task 01: Shared confirmation shell](task-01-shared-confirmation-shell.md)

Wave 2:

- [x] [Task 02: Task saved-view surfaces](task-02-task-saved-view-surfaces.md)
- [x] [Task 03: Integration saved-query surfaces](task-03-integration-saved-query-surfaces.md)

Wave 3:

- [x] [Task 04: Cross-surface browser verification](task-04-cross-surface-browser-verification.md)

Tasks 02 and 03 are parallel-safe after Task 01 because their production and
unit-test files are disjoint. The wave is a dependency description only and
does not authorize subagent execution.

## Verification results

- Task 01: focused confirmation tests passed 15/15; locale generation,
  `i18n:check`, and web typecheck passed.
- Task 02: task sidebar and Threads component tests passed 18/18; web typecheck
  passed.
- Task 03: shared integration, GitHub, GitLab, and Jira component tests passed
  15/15; the final combined focused suite passed 50/50 and web typecheck passed.
- Task 04: the desktop matrix passed 44/44 on `chromium`, and the mobile
  matrix passed 24/24 on `mobile-chrome`. Fresh desktop and phone screenshots
  were captured from those flows, inspected, and compressed for the PR.
- Final checks passed: changed-file ESLint, web typecheck, `i18n:check`,
  `i18n:ratchet`, specification lint and lint tests, and `git diff --check`.

## Risks

- A nested Radix confirmation can dismiss its parent or misdirect focus unless
  every fine-pointer owner applies the shared confirmation-boundary predicate.
- Optimistic deletion removes the anchor immediately after confirmation; the
  shell must close first and invoke the callback with the captured ID.
- GitHub default mutation and delete state share one row. Confirm must remain
  disabled while pending without trapping the user from cancelling.
- Hidden desktop controls can remain mounted during mobile tests, so locators
  must stay scoped to the visible drawer or sheet.
- GitLab and Jira currently depend on hover disclosure. Enlarging their coarse
  controls must not activate the row or create horizontal overflow.
- Longer localized names and confirmation copy can expand a row, especially in
  the shared dropdown bottom-sheet presentation; the parent must remain the
  only scroll owner.
