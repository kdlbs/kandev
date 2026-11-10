---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Task list archive access and filters requirements

## Overview

Users can open the workspace archive from the command panel and refine its
results in List view. Archive membership, search, scope, and totals describe
one task set. Tasks owns this contract because it owns workspace task discovery
and archive membership. Desktop and phone outcomes belong to this same feature.

The [workspace search contract](workspace-search.md) remains the authority for
text matching. The UI system retains ownership of
[listing preferences](../../ui/requirements/task-listing-display-preferences.md),
including portable workflow defaults, sorting, grouping, and row details.

## Terms

- **Active:** Tasks without an archive timestamp. A completed task can remain active.
- **Archived:** Tasks with an archive timestamp.
- **All:** Active and archived tasks in the requested workspace and filter scope.
- **List filters:** Search text, one workflow or all workflows, and one repository
  or all repositories. Archive mode is a separate, visible selection.
- **Saved scope:** The user's existing portable workflow and repository choices.
  An explicit list link can override them for that route.

## Requirements

### REQ-TASKS-LIST-ARCHIVE-FILTERS-001: Archive destination

**Intent:** Open the current workspace archive without reconstructing filters.

#### Acceptance criteria

- **AC-TASKS-LIST-ARCHIVE-FILTERS-001.1:** The global command panel shall offer
  **Go to archived tasks** on task, listing, and settings routes. Searching for
  archive-related navigation terms shall find it on desktop and phone.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-001.2:** Selecting the command shall open List
  in the current workspace with Archived selected. The list shall start with
  an empty search, all workflows, all repositories, and the first result page.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-001.3:** Opening the archive destination shall
  retain the saved scope and the user's existing sort, grouping, and row-detail
  preferences. Opening or restoring its link shall not save filter changes.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-001.4:** Without a resolved current workspace,
  the command shall be disabled. It shall not choose another workspace to
  construct an archive destination.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-001.5:** Selecting an archived row shall open
  its existing archived detail. Navigation shall not unarchive the task or
  create, prepare, resume, or start a session.

### REQ-TASKS-LIST-ARCHIVE-FILTERS-002: Combined list filters

**Intent:** Make archive membership and the existing task filters explicit.

#### Acceptance criteria

- **AC-TASKS-LIST-ARCHIVE-FILTERS-002.1:** List shall offer exactly one selected
  archive mode: Active, Archived, or All. Active shall remain the default for
  an ordinary list entry. Each mode shall return its defined task membership.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-002.2:** Search, workflow, and repository
  selections shall combine with archive mode. Rows, totals, and pagination
  shall describe the same complete filtered set, including matches after page one.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-002.3:** Changing archive mode, search,
  workflow, repository, or page size shall start at page one. Sort changes
  shall retain their existing page-reset behavior.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-002.4:** **Clear filters** shall clear search
  and select all workflows and repositories. It shall retain archive mode,
  sort, grouping, and row details. Removing a workflow or repository indicator
  shall clear only that filter.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-002.5:** Ordinary list entries shall retain
  saved scope defaults. Explicit workflow and repository edits, including
  Clear filters, shall retain their existing portable-save behavior. Merely
  opening a link, changing archive mode, and restoring history shall not save scope.

### REQ-TASKS-LIST-ARCHIVE-FILTERS-003: Filter controls and feedback

**Intent:** Make the effective list scope visible and editable on both viewports.

#### Acceptance criteria

- **AC-TASKS-LIST-ARCHIVE-FILTERS-003.1:** Desktop List shall group archive mode,
  search, workflow, repository, and Clear filters above the rows. Sort and
  grouping shall remain identifiable display controls. A duplicate control
  shall not show a different effective workflow or repository.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-003.2:** Phone List shall show archive mode and
  search above the rows. A visible Filters action shall open an inset bottom
  drawer with workflow, repository, Clear filters, and Done. Selections shall
  take effect immediately. Sort and grouping shall remain in View options.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-003.3:** Selected workflow and repository
  filters shall have visible, removable indicators outside their editing
  surface. Search shall have a clear action. The phone Filters count shall
  count selected workflow and repository filters only.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-003.4:** Loading, a failed read, an unavailable
  filter, an empty archive, and an empty filtered result shall be distinguishable.
  Failed reads shall offer Retry. Empty filtered results shall offer Clear filters.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-003.5:** Controls and feedback shall use the
  active locale and semantic accessible names. Keyboard users shall select
  modes and filters and dismiss overlays. Dismissal shall return focus to the
  opener. Phone controls shall have at least 44px targets, contained scrolling,
  and no document horizontal overflow.

### REQ-TASKS-LIST-ARCHIVE-FILTERS-004: Route and request integrity

**Intent:** Keep results consistent through navigation, delayed reads, and task actions.

#### Acceptance criteria

- **AC-TASKS-LIST-ARCHIVE-FILTERS-004.1:** An explicit list link shall retain
  archive mode, search, workflow, and repository through reload and browser
  Back or Forward. A query-only navigation shall update an already open List.
  A link to another authorized workspace shall select that workspace before
  mounting List and retain the explicit filters.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-004.2:** Boot data or delayed responses from
  another workspace, archive mode, filter set, sort, or page shall not replace
  current results. Pending reads shall not present previous-scope rows or
  totals as current, or announce a successful empty result.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-004.3:** When a manual workspace change keeps List
  open, List shall retain archive mode, clear search, select all workflows and
  repositories, and start at page one. Previous-workspace rows and scope
  identifiers shall not enter that read. Home and Office navigation shall retain
  their existing destination behavior. Explicit links and browser history shall
  retain their scope as specified in `004.1`.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-004.4:** Archive, unarchive, delete, pull-to-refresh,
  and Retry shall use the current query. Removing the final row of the final
  page shall select the last available page, or page one when the result is empty.
- **AC-TASKS-LIST-ARCHIVE-FILTERS-004.5:** An unsupported archive-mode value
  shall resolve to Active. A workflow or repository unavailable in the resolved
  workspace shall show an unavailable-filter state until cleared or replaced.
  Filter validation shall wait for metadata reads to finish. A failed workspace
  lookup shall offer Retry. An unknown workspace shall offer Home.
  A failed metadata read shall block only queries that use that filter.
  It shall not silently broaden the scope. Existing authorization failures
  shall not display rows from another workspace.

## Out of scope

- New priority, label, workflow-step, assignee, or date filters and saved List views.
- Changes to search syntax, archive lifecycle, task retention, or agent execution.
- Changes to command-panel task-result ranking or archived-result presentation.
- Changes to board, Threads, sidebar filters, plugin contracts, or portable settings storage.
- New API endpoints, query capabilities, database changes, feature flags, or dependencies.

## Related artifacts

- [System design](../system-design/task-list-archive-filters.md)
- [Implementation package](../../../plans/task-list-archive-filters/plan.md)
- [Command-panel archived task results](command-panel-archived-task-results.md)
