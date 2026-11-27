---
status: active
system: ui
created: 2026-10-19
owners:
  - kandev
---

# Repository Filter None Requirements

## Overview

Task-view repository filters need a selectable None option. Users can include
tasks without a repository alongside tasks from selected repositories.
UI owns this reusable presentation preference. Tasks and Workspaces retain
ownership of task membership and repository records.

## Terminology

- **None:** The existing missing-repository state in task-view filters.
- **Repository selection:** Values in one repository filter clause.

## Requirements

### REQ-UI-REPOSITORY-FILTER-NONE-001: Include tasks without a repository

**Intent:** Users can include tasks without a repository in a repository selection.

#### Selection acceptance criteria

- **AC-UI-REPOSITORY-FILTER-NONE-001.1:** Repository value pickers shall offer
  one translated None option, including when no named repositories are available.
  The option shall support the existing single-value and list operators.
- **AC-UI-REPOSITORY-FILTER-NONE-001.2:** Selecting only None with `is` or `in`
  shall match tasks in the existing missing-repository state.
  Other filter clauses shall continue to apply.
- **AC-UI-REPOSITORY-FILTER-NONE-001.3:** Selecting None and named repositories
  with `in` shall include the union of their matches without duplicate tasks.
  Removing None shall remove its matches unless another selected value matches them.
- **AC-UI-REPOSITORY-FILTER-NONE-001.4:** The `is_not` and `not_in` operators
  shall exclude the corresponding matches. Clearing all values shall retain
  the existing empty-clause behavior.

#### Restoration and interaction acceptance criteria

- **AC-UI-REPOSITORY-FILTER-NONE-001.5:** Saved views and drafts shall retain
  None through save, reload, and workspace switches. Existing workspace and
  personal-view isolation shall apply.
- **AC-UI-REPOSITORY-FILTER-NONE-001.6:** The option label and single-selection
  summary shall follow the active locale. Locale changes shall preserve selection
  membership and matching behavior.
- **AC-UI-REPOSITORY-FILTER-NONE-001.7:** Desktop and phone editors shall support
  selecting and removing None through their existing keyboard and touch controls.
  Existing search, selected-first ordering, dismissal, focus return, and internal
  option scrolling shall apply. Phone option targets shall measure at least 44px.

## Out of scope

Board display repository selectors, Threads filters, automatic-color rules,
repository attachment changes, and changes to named-repository matching or
missing-metadata resolution are separate contracts.

## Related specifications

- [Workspace task views](workspace-sidebar-task-views.md)
- [Selected filter options first](filter-selected-options-first.md)
- [System design](../system-design/repository-filter-none.md)

## Implementation plans

- [Repository filter None](../../../plans/repository-filter-none/plan.md)
