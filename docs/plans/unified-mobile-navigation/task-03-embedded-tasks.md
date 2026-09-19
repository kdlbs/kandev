---
id: "03-embedded-tasks"
title: "Embed Tasks and move options to listing titles"
status: done
wave: 3
depends_on: ["01-shared-navigation", "02-pinned-shortcuts"]
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-004
  - REQ-UI-MOBILE-MENU-005
acceptance_criteria:
  - AC-UI-MOBILE-MENU-004.1
  - AC-UI-MOBILE-MENU-004.2
  - AC-UI-MOBILE-MENU-004.3
  - AC-UI-MOBILE-MENU-005.1
  - AC-UI-MOBILE-MENU-005.2
  - AC-UI-MOBILE-MENU-005.3
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 03: Embed Tasks and move options to listing titles

## Summary

Apply user feedback from the seeded first iteration: embed the actual collapsible
Tasks sidebar in the middle of the shared hamburger; remove dedicated navigation
pins and Task views. Open listing options only from the top-level mode dropdown.

## Scope

- Reuse sidebar task data, views, filters, actions, and dialog controllers through
  an inline presentation. Preserve the standalone title picker and navigation
  history. Default expanded, with disclosure state local to the mounted host.
- Remove the separate View options button; give Kanban/List/Threads the same
  listing-context entry and include Threads saved views in the options flow.
- Replace shortcut-specific tests with embedded-sidebar scenarios, migrate
  affected selectors, update translations and public instructions, and refresh
  the isolated test instance when implementation is validated.

## Exclusions

No backend/schema changes, new collapse persistence, global destination redesign,
desktop sidebar redesign, or change to existing pin preferences. Do not touch the
main instance on port 9998. Preserve completed Tasks 01/02 results as history.

## Acceptance

1. AC-UI-MOBILE-MENU-004.1-.3: the full sidebar appears inline in the shared
   menu, collapses without side effects, retains actions/recovery, and shares
   the outer scroller. No separate pinned shortcuts or Task views row remains.
2. AC-UI-MOBILE-MENU-005.1-.3: all three listing-title dropdowns open the
   appropriate options; the separate button is absent, and Threads retains all
   saved-view editing/recovery through the options surface.
3. Focus, browser history, task dialogs across closure/rotation, workspace
   isolation, long-title geometry, and desktop/tablet composition remain intact.

## ASCII UI preview

```text
Phone listing:
[Workspace / Kanban v]                   [Menu]
[Listing content, without an options toolbar row]

Kanban / Threads / List v -> View options
  Modes | applicable search/display | saved views

Shared hamburger:
  Menu                                    [X]
  Workspace v
  Global and page navigation
  Tasks v                                 [+]
    Saved view v / filters
    Task tree and existing row actions
  Workspace actions
  Plugins / integrations
  Utilities

Tasks > hides the inline body, leaving the heading and other sections.
Desktop/tablet: existing composition. Phone task title: existing task picker.
```

The menu owns one content scroller. Standalone controls have 44px phone targets.

## Implementation sequence

1. Add failing component/browser coverage for inline collapse, absence of the
   two removed sections, header options entry, and Threads saved-view access.
2. Extract the reusable sidebar controller/body boundary and add inline menu
   presentation. Keep action dialogs outside unmounting drawer content; keep
   requested controllers above responsive branch changes.
3. Rewire listing titles, embed Threads view controls, remove obsolete shortcut
   code/locales, migrate entry-point tests, and validate the rendered result.

## Verification

From `apps/web`, use the managed runner, one worker, no overlapping browser runs.
Create `mobile-navigation-tasks.spec.ts` and remove the superseded pins-only suite.

```bash
pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/threads/threads-view-controls.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/mobile/task-sheet-selection-context.test.tsx components/task/task-layout-repository.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --project mobile-chrome tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-navigation-tasks.spec.ts tests/layout/mobile-sidebar-read-recovery.spec.ts tests/github/mobile-task-view-access.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-task-list-search.spec.ts tests/kanban/mobile-display-settings-groups.spec.ts tests/settings/mobile-startup-page.spec.ts tests/chat/mobile-auto-scroll-toggle.spec.ts
```

Run targeted ESLint and any new extracted-controller unit suites. Inspect actual
393px and 767px screenshots, collapse/long-list scroll behavior, and 768px/820px
presentation. Run `compact-desktop-responsive.spec.ts` in Chromium using the
filename-anchored temporary config workaround documented in the parent plan if
the worktree path still matches the project's broad `mobile-` exclusion.
Update the affected-spec manifest for moved controls; preserve behavior assertions.
Run spec/public-doc validation and `git diff --check` from the repository root.

## Files and dependencies

- `apps/web/components/navigation/app-nav-sheet.tsx`, `app-nav-sections.tsx`,
  `use-task-view-navigation.tsx`, and obsolete `mobile-pinned-*`/shortcut adapters.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`, its
  controller/dialog helpers, `responsive-task-picker.tsx`, and `task-layout.tsx`.
- `apps/web/components/kanban/kanban-header-mobile.tsx`, `mobile-menu-sheet.tsx`;
  `apps/web/components/threads/threads-view-controls.tsx` and view-list/editor.
- Related unit/E2E tests, locales, web AGENTS, public navigation instructions.

## Risks

A body rendered inside the drawer must not own persistent child dialogs.
Nested task scrolling can strand utilities; sidebar reads cannot show old
workspace titles. Threads currently owns a separate saved-view picker, so merely
changing its label would lose mode/display access. Preserve that functionality
through the shared options surface instead.

## Parallelism

Sequential in the primary session; no delegation authorized.

## Results

Implemented on 2026-09-18 following the user's explicit implementation request.

- Embedded the full sidebar through the persistent app-shell controller and a
  menu portal. Collapse survives menu closure; task dialogs survive rotation.
- Removed dedicated navigation pins and Task views. Listing dropdowns open
  options; Threads saved views share that surface. Board pagination remains
  beside the dropdown, with sync status and recovery preserved.
- Red: 2 header unit cases and 4 browser cases failed before implementation.
  Green: 100 distinct focused unit tests (95 in the main run, 5 additional
  Threads-page cases), typecheck, targeted ESLint, production build, i18n checks
  and new-code ratchet passed. No new public copy was required.
- Browser validation initially found missing Threads pagination and stale
  saved-view trigger assertions. Restored the separate pagination slot and
  moved view-name assertions into the options surface without dropping behavior
  checks. Final browser results follow below.
- Inspected 393px and 767px screenshots, full-list scrolling, and the live seeded
  workbench. Public-doc tests/validator and specification validators passed.
- Refreshed the existing isolated preview at http://100.105.155.17:48439,
  preserving its seeded workspaces/tasks and separate database. Direct Tailscale
  browser smoke passed for menu collapse, header options, and task workbench.
  The main instance on port 9998 was not modified. Shutdown:
  `python3 /tmp/kandev-mobile-menu-test-hw9njt_n/stop.py`.

Revision logs: `/tmp/mobile-menu-revision-*`, `/tmp/mobile-menu-pagination-*`,
`/tmp/mobile-menu-final-*`, and `/tmp/mobile-menu-preview-smoke.log`.

Final browser result: 64 distinct mobile scenarios passed across the main run
and the final 14-case Threads/header rerun (60/64 initially, with all four
failures corrected). Both compact-desktop Chromium cases passed. The temporary
filename-anchored browser config was removed. Total: 66 distinct browser cases.


## PR review follow-up

- Reconciled current-main task move callbacks and workspace-scoped saved-view
  settings with the embedded task menu.
- Bridged archived-task and port-forwarding page context through the app-menu
  outlet, preserving the current archived task in views that exclude archives.
- Retained the task-picker key during transient null workspace metadata; a
  different resolved workspace still resets its drafts.
- Added explicit pointer styling to the new interactive section controls and
  reconciled the older task-view/chrome documents with the embedded menu.
- Regression unit tests failed first for lost context and picker draft loss,
  then passed after the fixes. The focused task picker/menu run passed 30 tests,
  and the layout regression suite passed 4 tests. Merge validation passed 57
  unit tests and 20 phone browser scenarios covering embedded navigation,
  sidebar saved views, and task moves. Typecheck, focused ESLint, specification,
  documentation, and merged harness checks passed.
- Final managed phone regressions passed (2 tests): the embedded current archived
  task remains visible, and deleting a Threads saved view then reopening options
  starts on the view list. The latter disproves the reported stale editor:
  closing the outer drawer unmounts its controls, resetting local editor state.
- Removed the duplicate integration spec entry and completed requirement mapping.

Review verification logs: `/tmp/pr3830-merge-unit.log`,
`/tmp/pr3830-merge-e2e.log`, `/tmp/pr3830-context-red.log`,
`/tmp/pr3830-final-unit.log`, `/tmp/pr3830-picker-red.log`,
`/tmp/pr3830-picker-green.log`, and `/tmp/pr3830-review-e2e.log`.

## CI follow-up

- Updated phone tests that still targeted the retired hamburger task picker,
  separate Task views action, or home-only menu container.
- Matched bottom navigation independently of the task title. Changes selectors
  retain support for the numeric badge in their accessible name.
- Waited for drawer animations before checking touch geometry, used the existing
  virtual-tree reveal helper for offscreen files, and disabled queue auto-run in
  the oversized-message preview test so completion cannot remove its subject.
- Browser verification exposed a repository-source menu touch race: opening on
  pointer-down or pointer-up could select the first item with the same touch's
  synthetic click. The menu now opens on that click; desktop pointer and keyboard
  activation retain Radix behavior. Extracted the existing menu into its own
  component to keep the dialog within the file-size limit. No public contract or
  copy changes are needed: choosing a source still requires selecting an item.
- Final focused navigation run: 15 phone tests passed, including every reported
  obsolete-picker failure, Changes panel cases, workspace views, archive recovery,
  Office nesting, and workflow session focus/queue ownership.
- Additional final browser cases passed: oversized-message previews, source
  attachment and persistence, no-cursor file opening, saved-view geometry, and
  terminal bottom spacing. The new touch regression failed before both event
  ordering corrections; the final component suite passed all 15 tests.
- TypeScript, focused ESLint, and the frontend E2E build passed.

Evidence: `/tmp/pr3830-ci-mobile-final.log`, `/tmp/pr3830-ci-extra.log` (the final
three cases), `/tmp/pr3830-touch-browser.log` (oversized message),
`/tmp/pr3830-touch-click-browser.log`, `/tmp/pr3830-touch-click-red.log`, and
`/tmp/pr3830-touch-extract-unit.log`. Earlier failures remain in these logs;
final cases are identified above rather than treating those runs as wholly green.
