---
status: active
system: ui
created: 2026-10-26
owners:
  - kandev
---

# Mobile List saved views and controls

## Overview

Phone List uses the user's existing workspace sidebar views. The user selects
views through tabs and edits their options through the top-left List control.
Search and temporary filters occupy space only while the user needs them.

UI owns this reusable presentation contract. Tasks retains task membership,
authorization, search matching, and runtime summaries. Existing contracts retain
portable view storage, automatic colors, and query bounds.

## Terms

- **Saved view:** An existing personal, workspace-scoped sidebar view.
- **View options:** The sheet opened through the top-left listing control.
- **Result filters:** Temporary filters that refine the displayed results.
- **Layout:** Kanban, Threads, or List. Layout and saved-view identity are distinct.

## Requirements

### REQ-UI-MOBILE-LIST-VIEWS-001: Shared saved views

**Intent:** Use the same task organization on desktop sidebar and phone List.

- **AC-UI-MOBILE-LIST-VIEWS-001.1:** Below 768 CSS pixels, List shall show tabs
  for the active workspace's saved sidebar views, using their saved names and order.
  The fixed Active, Archived, and All toolbar shall be absent.
- **AC-UI-MOBILE-LIST-VIEWS-001.2:** Selecting a tab shall use that view's
  filters, complete sort chain, grouping, collapse state, and task-row presentation.
  The same identity and persisted definition shall apply in the desktop sidebar.
- **AC-UI-MOBILE-LIST-VIEWS-001.3:** Running first, Red first, Orange first,
  then newest Last activity shall apply in that order before pagination.
  Effective colors, task-wide running rank, and complete-tree activity shall
  retain the sidebar's established meanings.
- **AC-UI-MOBILE-LIST-VIEWS-001.4:** An empty saved collection shall resolve to
  the canonical All tasks view. Invalid selection shall use the existing
  same-workspace fallback. No previous workspace shall supply fallback views.
- **AC-UI-MOBILE-LIST-VIEWS-001.5:** Phone List shall retain bounded browsing,
  shared accepted task records, stale-response guards, and live updates.
  A partial page shall not establish complete membership or global order.
- **AC-UI-MOBILE-LIST-VIEWS-001.6:** Long or numerous tabs shall scroll inside
  their row. The selected tab shall remain discoverable without document overflow.
  Task-row taps and visible secondary actions shall remain independently usable.
- **AC-UI-MOBILE-LIST-VIEWS-001.7:** Desktop standalone List and Threads shall
  retain their own preferences and view collections. Phone List shall not copy,
  migrate, or overwrite those definitions to select a sidebar view.

### REQ-UI-MOBILE-LIST-VIEWS-002: Temporary search and result filters

**Intent:** Keep everyday listing chrome compact without hiding active refinements.

- **AC-UI-MOBILE-LIST-VIEWS-002.1:** A search icon shall appear immediately left
  of the top-right hamburger. Ordinary List entry shall hide the search input
  and Filters button. Opening search shall reveal one row below the view tabs
  and focus the input.
- **AC-UI-MOBILE-LIST-VIEWS-002.2:** Search and result filters shall refine the
  selected view without changing its saved definition or shared editing draft.
  Ordinary result filters shall combine with saved filters rather than replace them.
- **AC-UI-MOBILE-LIST-VIEWS-002.3:** Search shall retain the existing workspace
  literal-matching contract and single trailing 300 ms admission window.
  Results and totals shall include matching tasks beyond the current page.
  Clearing search shall cancel queued text and immediately admit the empty query.
- **AC-UI-MOBILE-LIST-VIEWS-002.4:** Tapping the search icon again shall hide
  the row and clear search and temporary result filters. It shall cancel queued
  input and restore the opener's focus. Hidden refinements shall not remain active.
- **AC-UI-MOBILE-LIST-VIEWS-002.5:** Filters shall open a focused result-filter
  sheet with Apply and Clear. Closing without Apply shall discard its temporary
  edits. Applied refinements shall show a count and removable summary while active.
- **AC-UI-MOBILE-LIST-VIEWS-002.6:** View selection and manual workspace changes
  shall clear temporary refinements and start at page one. Workspace changes
  shall dismiss sheets bound to the previous workspace.
- **AC-UI-MOBILE-LIST-VIEWS-002.7:** Explicit List links and archive navigation
  shall retain their requested scope through reload and history without saving it.
  Such entry shall reveal active refinements. Clear shall restore the selected
  saved view. The archive command shall retain its empty-search and all-scope intent.
- **AC-UI-MOBILE-LIST-VIEWS-002.8:** Supported legacy sort/group links and
  plugin facets shall remain usable as explicitly identified result overrides.
  They shall not become sidebar-view fields or silently alter saved definitions.
  Missing plugin capabilities shall show the existing unavailable/reset behavior.

### REQ-UI-MOBILE-LIST-VIEWS-003: One saved-view editing entry

**Intent:** Keep saved changes separate from temporary result filtering.

- **AC-UI-MOBILE-LIST-VIEWS-003.1:** The top-left List control shall open View
  options. The sheet shall contain layout choices and the selected saved view's
  name, filters, sort, grouping, task-row presentation, and automatic-color access.
- **AC-UI-MOBILE-LIST-VIEWS-003.2:** The hamburger menu, saved-view tabs, and
  result Filters control shall not enter saved-view editing from phone List.
  The menu shall contain no Edit view (List) action or duplicate editor shortcut.
- **AC-UI-MOBILE-LIST-VIEWS-003.3:** View options shall retain the existing
  create, rename, delete, save, Save as new, and Discard behavior and view limit.
  Save controls shall appear only for a view draft and remain reachable while
  the editor body scrolls. Closing shall preserve the existing draft behavior.
- **AC-UI-MOBILE-LIST-VIEWS-003.4:** Sort rules shall use stacked phone cards.
  Fields, directions, colors, and rule order shall remain readable. Existing
  handles and More actions shall retain keyboard and explicit move alternatives.
- **AC-UI-MOBILE-LIST-VIEWS-003.5:** Automatic colors shall retain their
  existing immediate personal-settings behavior. View Save and Discard shall
  neither claim ownership of those writes nor roll them back.
- **AC-UI-MOBILE-LIST-VIEWS-003.6:** Layout selection shall preserve existing
  navigation and draft retention. The phone Pipeline fallback shall not overwrite
  the saved desktop layout. Threads shall retain its separate view editor.
- **AC-UI-MOBILE-LIST-VIEWS-003.7:** Temporary search, result filters, and link
  overrides shall remain distinguishable from the saved definition inside View
  options. Saving a view shall not silently incorporate those refinements.

### REQ-UI-MOBILE-LIST-VIEWS-004: Compact and legible phone composition

**Intent:** Match the agreed previews while preserving the screenshot's control density.

- **AC-UI-MOBILE-LIST-VIEWS-004.1:** The workspace name, including Default,
  shall remain sharp at the top edge on iPhone Safari and installed PWA.
  Cold load, scrolling, and overlay dismissal shall not leave blur or a faded label.
- **AC-UI-MOBILE-LIST-VIEWS-004.2:** Existing mobile control heights shall
  remain unchanged. This includes the current 40px layout choices and 28px
  select options. This request does not introduce a universal 44px enlargement.
  Padding corrections shall not inflate control heights or create empty rows.
- **AC-UI-MOBILE-LIST-VIEWS-004.3:** Short choice sheets shall fit their content.
  View options and app navigation shall use one scrolling body, fixed chrome,
  dynamic viewport bounds, and safe-area clearance for their final actions.
- **AC-UI-MOBILE-LIST-VIEWS-004.4:** The app menu shall separate its header and
  workspace picker with visible padding. It shall group creation, Home/quick
  actions, eligible tools, contextual Tasks, and utilities consistently.
  Saved visibility and relative tool order shall remain effective.
- **AC-UI-MOBILE-LIST-VIEWS-004.5:** From phone List, contextual Tasks shall
  start collapsed and retain task selection when expanded. Its menu presentation
  shall omit saved-view editing actions. Task-picker editing on other surfaces
  shall retain its existing behavior.
- **AC-UI-MOBILE-LIST-VIEWS-004.6:** Phone chrome, drawers, and task controls
  shall remain contained and operable with long labels, empty results, loading,
  and read errors. Keyboard dismissal and focus return shall remain predictable.
  All new product copy shall use the active locale.

## Precedence and related contracts

These criteria define the current phone List behavior. The plan identifies the
acceptance criteria that this phone-specific contract refines.

- [Workspace sidebar views](workspace-sidebar-task-views.md) owns saved identity and persistence.
- [Sidebar pagination](sidebar-task-pagination.md) owns bounded global ordering and shared records.
- [Workspace search](../../tasks/requirements/workspace-search.md) owns text matching.
- [List archive access](../../tasks/requirements/task-list-archive-filters.md) owns archive destination semantics.
- [Listing preferences](task-listing-display-preferences.md) retains desktop/tablet preferences and startup behavior.

## Out of scope

- Sharing Threads or integration view collections with sidebar views.
- Rewriting saved definitions, introducing a new settings table, or changing task lifecycle.
- Enlarging controls across the application or redesigning desktop List.
- Changing automatic-color ownership or the established sort comparators.

## Implementation plan

- [Mobile List layout](../../../plans/mobile-list-layout/plan.md)

The phone saved-view surface and request-only result refinements are implemented.
Focused behavior and build checks passed. Screenshot comparisons are recorded
in the plan evidence. Real-iPhone Safari and installed PWA evidence for the
reported workspace-label blur remains open.
