---
id: "03-add-desktop-thread-view-controls"
title: "Add desktop Threads view controls"
status: done
wave: 3
depends_on:
  - "02-query-and-admit-thread-tasks"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-001
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-001.1
  - AC-UI-THREADS-SAVED-VIEWS-001.3
  - AC-UI-THREADS-SAVED-VIEWS-001.4
  - AC-UI-THREADS-SAVED-VIEWS-002.1
  - AC-UI-THREADS-SAVED-VIEWS-002.2
  - AC-UI-THREADS-SAVED-VIEWS-002.12
  - AC-UI-THREADS-SAVED-VIEWS-003.6
  - AC-UI-THREADS-SAVED-VIEWS-003.9
  - AC-UI-THREADS-SAVED-VIEWS-004.1
  - AC-UI-THREADS-SAVED-VIEWS-004.7
  - AC-UI-THREADS-SAVED-VIEWS-004.8
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 03: Add Desktop Threads View Controls

## Summary

Add the active-view selector and editor before the desktop task-listing view
icons. Reuse generalized sidebar editor primitives with Threads-owned state and
registries.

## In scope

- Add optional task-view control slots to `KanbanHeader`.
- Render the active-view selector and settings button on Threads.
- Add create, switch, rename, Save, Save as, discard, and delete actions.
- Add task-scope selection with search, checkboxes, Select all, and Clear all.
- Add Threads filter, sort, limit, hidden-count, and Reapply sort controls.
- Disable Save for an empty selected scope and expose the validation reason.
- Generalize sidebar clause and editor primitives without changing sidebar
  behavior.
- Hide `KanbanDisplayDropdown` on Threads.
- Add all user-facing text to the five locale catalogs.

## Out of scope

- Phone drawer presentation and browser-level E2E.

## Acceptance

- Controls appear directly before the task-listing view icons.
- The editor remains inside the viewport and owns one vertical scroll area.
- View actions update the query and persist through Task 01 state actions.
- Empty and capped results keep the controls available and explain the result.
- Keyboard users can complete every view and clause action and regain focus.

## Verification

Write failing header, selector, editor, task-picker, and sidebar-regression tests
first. Then run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/threads/thread-view-controls.test.tsx components/threads/thread-view-editor.test.tsx components/task/sidebar-filter/sidebar-filter-editor.test.tsx components/kanban/kanban-header.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/components/threads/thread-view-controls.tsx`
- `apps/web/components/threads/thread-view-editor.tsx`
- `apps/web/components/threads/thread-task-picker.tsx`
- `apps/web/components/task/sidebar-filter/`
- `apps/web/app/threads/threads-page-client.tsx`
- `apps/web/src/locales/*/threads.json`

## Dependencies

- Task 02 supplies the query result, counts, and reset action.

## Risks

- Preserve the sidebar editor behavior while extracting shared primitives.
- Keep long view names and filter labels inside the header and popover.

## Parallelism

`sequential`
