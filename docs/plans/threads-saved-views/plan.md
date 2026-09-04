---
created: 2026-08-31
status: completed
requirements:
  - ../../specs/ui/requirements/threads-saved-views.md
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
legacy_specs: []
---

# Implementation Plan: Threads Saved Views

## Overview

Add user-owned saved views to `/threads`. Each view selects exact tasks or a
live task set, filters bounded task metadata, sorts matching tasks, and limits
the number of admitted task columns. Users can save several views and switch
between them from the task-listing header.

The feature uses the existing backend user-settings document. It adds no task
query endpoint and no saved-view table. Threads and the task sidebar keep
separate saved definitions, but they share query and editor primitives.

## Scope

### In scope

- A canonical `All threads` view with the current attention order and a
  five-column default.
- One or many selected tasks, or all eligible tasks.
- Bounded task, workflow, agent, status, review, and Git filters.
- Deterministic sort and an optional limit from 1 through 30 columns.
- Create, switch, rename, save, Save as, discard, and delete actions.
- Backend user-settings persistence, revision ordering, and live client sync.
- Desktop and tablet top-bar controls before the task-listing view icons.
- A phone trigger and one inset bottom drawer with complete feature parity.
- Deep-link admission, stable live order, empty states, and write recovery.

### Out of scope

- Sharing saved definitions or active selection with sidebar task views.
- Grouping horizontal columns into visual sections.
- Transcript-content filters or server-side task queries.
- Shared team views or role-based ownership.
- Changes to the viewport-bounded session subscription contract.

## Technical approach

### Persist surface-owned views

Extend the existing portable user settings with `thread_views`,
`thread_active_view_id`, and `thread_view_draft`. Reuse the settings PATCH,
revision, boot hydration, and `user.settings.updated` path. Validate collection,
clause, task-ID, and column-limit bounds in the backend.

Follow
[ADR-2026-08-31-surface-owned-saved-task-views](../../decisions/2026-08-31-surface-owned-saved-task-views.md).
Keep Threads state independent from `sidebarViews`, but extract shared clause,
operator, sort-direction, wire-normalization, and editor contracts.

### Query bounded task candidates

Project task candidates from workflow snapshots and compact task summaries for
the active workspace. Do not read transcripts or task-session lists. Map the
bounded agent and executor identity needed by filters into snapshot state.

Apply workspace scope, Threads eligibility, task scope, filter clauses, sort,
deep-link reservation, and the column limit in that order. Pass admitted tasks
only to `ThreadsBoard`. Return admitted, matching, and hidden counts to the
view control.

### Preserve position during live work

Reset stable column order when the view query changes or the user selects
Reapply sort. Keep surviving columns in place for normal task and session
events. Remove tasks that stop matching and fill open slots from the current
sorted result.

### Use one responsive editor contract

Add optional task-view controls to `KanbanHeader`. Desktop uses compact
selector and settings controls before `ViewToggleGroup`. Tablet and phone use
one inset bottom drawer with internal saved-view, editor, and task-picker
pages. The drawer owns vertical scroll and has no nested drawer.

Hide `KanbanDisplayDropdown` on Threads. Its workflow and repository controls
must not become a second query owner. Other task-listing pages keep their
current header behavior.

## Tests

| Acceptance criteria                             | Evidence                                                                               |
| ----------------------------------------------- | -------------------------------------------------------------------------------------- |
| `AC-UI-THREADS-SAVED-VIEWS-001.1` through `.9`  | User-settings service, store, wire-mapping, header, and saved-view action tests.       |
| `AC-UI-THREADS-SAVED-VIEWS-002.1` through `.11` | Candidate projection and table-driven filter tests, including selected-task retention. |
| `AC-UI-THREADS-SAVED-VIEWS-003.1` through `.13` | Comparator, admission, stable-order, sort-description, limit, and deep-link tests.     |
| `AC-UI-THREADS-SAVED-VIEWS-004.1` through `.13` | Desktop component tests plus desktop and mobile Playwright coverage.                   |

## E2E tests

- Extend the desktop Threads test with two saved views. Verify task selection,
  live filters, deterministic sort, a three-column limit, hidden count, Save,
  reload, view switching, and cross-client settings refresh.
- Verify that a hidden deep-link task replaces the last normal admission,
  receives focus, and does not change the saved view.
- Extend the phone Threads test with the active-view trigger and one drawer.
  Verify view switching, task-picker navigation, filters, sort, limit, save,
  44-pixel targets, safe-area containment, and no document overflow.
- Preserve the existing session traffic assertions. A column limit must reduce
  task shells without increasing transcript or task-session subscriptions.

## Mobile design contract

- **Desktop outcome:** the active-view selector and settings button precede the
  four task-listing view icons.
- **Mobile entry and hierarchy:** a compact active-view button starts the
  header action strip and opens one inset bottom drawer.
- **Surface rationale:** one drawer gives clauses and task names full width and
  avoids nested modal or horizontal gesture conflicts.
- **Scroll ownership:** the drawer owns editor scrolling; the task picker
  replaces its body; the task deck keeps horizontal pager ownership.
- **Shared behavior:** view state, validation, task query, save actions, and
  counts are common. Only trigger and editor presentation differ.
- **Mobile proof:** mobile-chrome verifies complete action parity, touch
  geometry, safe-area clearance, focus return, and zero horizontal overflow.

## Work orders

- [x] [Task 01: Persist Threads saved views](task-01-persist-thread-views.md)
- [x] [Task 02: Query and admit Threads tasks](task-02-query-and-admit-thread-tasks.md)
- [x] [Task 03: Add desktop Threads view controls](task-03-add-desktop-thread-view-controls.md)
- [x] [Task 04: Add the mobile Threads view drawer](task-04-add-mobile-thread-view-drawer.md)
- [x] [Task 05: Prove saved-view behavior and resource bounds](task-05-prove-thread-view-behavior.md)
- [x] [Task 06: Improve the top-bar view editor](task-06-improve-top-bar-view-editor.md)

All work orders are sequential. They share user-settings types, query state,
header contracts, editor primitives, and E2E fixtures.

## Results

Implemented and verified. Threads now has persisted, workspace-scoped saved
views with bounded filtering, deterministic sorting, deep-link admission, live
stable ordering, desktop controls, and a native mobile drawer. The top-bar
editor also has sort descriptions, live task-picker metadata, a five-column
default, and stronger surface separation. All six work orders are complete.
Focused unit, backend, lint, localization, typecheck, and desktop/mobile
browser checks pass.

## Risks

- A saved task ID is a preference, not authorization. Query only candidates
  that the workspace snapshot already authorized.
- A query reset can move the column under the pointer. Reset only for explicit
  query changes or Reapply sort, not for live activity.
- A second header filter owner produces hidden constraints. Remove the
  old display dropdown from Threads when the new controls ship.
- Large saved definitions can inflate every user-settings update. Enforce 50
  views, 20 clauses per view, and 200 selected task IDs per view.
- Mobile clauses can become tall. Use one scroll owner and internal pages, not
  stacked drawers.
