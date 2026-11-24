---
status: active
system: ui
created: 2026-11-19
owners:
  - kandev
---

# Workflow Settings Compact List Requirements

## Overview

The workspace Workflows settings page renders every workflow as a full editor
card: name, agent profile, description, prompt, step pipeline, and actions.
A workspace with many workflows makes the page very long, and people cannot
scan it at a glance. A compact view shows each workflow as one short,
information-dense row. People expand a row to edit that workflow in place.
UI owns this presentation. Tasks keeps workflow definitions, step data, save
semantics, ordering, export, duplication, and deletion.

## Terms

- **Compact row:** the collapsed presentation of one workflow in the compact
  view.
- **Expanded card:** the existing full workflow editor card.
- **View preference:** the per-browser choice between Compact and Detailed.

## Requirements

### REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001: Compact expandable workflow rows

**Intent:** Let people scan many workflows quickly and open only the one they
want to edit.

#### Acceptance criteria

- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.1:** In the compact view, each
  saved workflow shall render as a collapsed compact row that shows the
  workflow name, its ordered steps (color dot and name), the step count, and
  the workflow agent profile when one is set. On viewports at least 1024 px
  wide, the row shall also show the description, truncated to the available
  width. Text that does not fit shall truncate rather than wrap.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.2:** A compact row shall show a
  Synced marker for a workflow synced from a repository, a read-only marker for
  an Improve Kandev workspace workflow, and an Unsaved marker while the
  workflow has unsaved draft changes.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.3:** When a person activates a
  compact row by pointer, touch, Enter, or Space, the system shall expand that
  workflow in place to the expanded card with every existing field and action.
  Activating the row header again shall collapse it. Other rows shall keep
  their state.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.4:** Collapsing an expanded card
  shall not discard, save, or reset its draft. Unsaved edits shall remain in
  the shared settings save surface and shall reappear unchanged when the card
  is expanded again.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.5:** A workflow added or
  duplicated during the current page visit shall start expanded in the
  compact view and shall stay expanded after it is saved. Import reloads the
  page, so imported workflows follow the default for a new visit.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.6:** Drag reordering shall remain
  available from collapsed rows and expanded cards, with the same order and
  save behavior as before.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.7:** The row header shall expose
  its expanded state to assistive technology and have a localized accessible
  name that includes the workflow name. On phones (including narrow
  fine-pointer windows) and coarse pointers, the row header, drag handle, and
  view control options shall each be at least 44 px tall. Below 768 px the
  row shall use two lines (identity on the first, steps on the second) without
  document horizontal overflow.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.8:** When settings navigation
  targets a workflow card, through a `#workflow-card-<id>` link (for example,
  the WIP queue settings link) or a settings search result, the system shall
  expand that card in the compact view before revealing it.

### REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002: Workflow list view preference

**Intent:** Let people choose between the compact list and the full detailed
list and keep that choice.

#### Acceptance criteria

- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.1:** The Workflows list shall
  show a localized two-option view control, Compact and Detailed, above the
  list. It shall be available in every workspace, including the read-only
  Improve Kandev workspace.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.2:** When no valid stored
  preference exists, the list shall use the compact view.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.3:** The Detailed view shall
  render every workflow as an expanded card with no collapse control, matching
  the current page.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.4:** The selected view shall
  persist per browser across reloads and workspaces. When browser storage is
  unavailable, the control shall still switch the view for the current page
  visit and shall show no error.
- **AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002.5:** Changing the view shall not
  change drafts, save state, workflow data, or workflow order. The view
  preference shall never mark the page dirty.

## Exclusions

- Backend, API, database, or workflow data changes.
- Server-side or per-account storage of the view preference or row expansion.
- Persisting which rows are expanded across reloads.
- Expand all or collapse all controls, search, filtering, or sorting.
- Compact presentation for other settings lists, the kanban board, or the
  workflow selector.
- New actions in the compact row; Export, Duplicate, and Delete remain in the
  expanded card.

[Settings composition](settings-composition.md) keeps settings sections and
groups expanded. That rule does not cover a list of resource records, which
this document governs, consistent with
[collapsible agent blocks](../../agents/requirements/collapsible-agent-blocks.md).

## Implementation Plans

- [Workflow settings compact list](../../../plans/workflow-settings-compact-list/plan.md)
