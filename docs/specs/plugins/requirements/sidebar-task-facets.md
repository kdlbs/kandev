---
status: active
system: plugins
created: 2026-10-24
owners:
  - kandev
---

# Plugin Sidebar Task Facets Requirements

## Overview

Workspace-scoped saved task views in the left sidebar offer plugin-owned task
facets in **Filters and sort → Group by**. Users can organize the sidebar by
plugin data, such as tags, without a separate plugin sidebar or host-owned tag
model. This extends the existing [task-list facet contract](task-list-facets.md)
without changing a plugin's synchronous registration API.

The plugin system owns facet identity, labels, values, and availability. The host
owns the sidebar controls, saved selection, page-local sections, and task
interactions on desktop and phones. Sidebar selections remain independent of
`/tasks` preferences.

## Terminology

- **Facet:** A plugin-owned task dimension registered with an identity, label,
  synchronous `getValues`, and optional change subscription.
- **Accepted page:** The bounded sidebar task page after the current view's
  filters, sort chain, pinning, and sibling ordering have been applied.
- **Value section:** A group containing accepted-page tasks that return one
  particular facet value identifier.
- **Row occurrence:** One rendered copy of a task in a particular value section.
  Several occurrences can refer to the same canonical task.
- **Unassigned section:** The trailing group for tasks that return no valid value.

## Requirements

### REQ-PLUGINS-SIDEBAR-TASK-FACETS-001: Saved Sidebar Facet Grouping

**Intent:** Plugin-owned dimensions behave as saved sidebar grouping choices,
while every loaded task and every value remains reachable.

**User story:** As a workspace user, I want to group a saved sidebar task view by
my plugin's tags, so that I can navigate the same task through each relevant tag.

#### Acceptance criteria

- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.1:** On desktop and phones, the Group by
  control shall retain built-in choices in their existing order and append active
  facets using their registered labels verbatim. A compatible existing facet
  registration shall require no plugin-side changes.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.2:** Selecting a facet shall record
  `facet:<pluginId>:<facetId>` in the saved view's `group` field and in its unsaved
  draft. Saving, reloading, switching views, and switching workspaces shall retain
  each workspace's own selection without changing `/tasks` preferences.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.3:** Facet grouping shall apply only to the
  accepted page. Filters, sort priority, pinning, sibling order, and pagination
  shall continue to choose source rows before grouping; section counts shall
  describe loaded members, not workspace-wide totals. The host shall not request
  a workspace-wide fetch or send a facet identifier as a server grouping query.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.4:** Value reads shall receive the explicit
  sidebar workspace identifier and each accepted-page task identifier. Switching
  workspaces shall not reuse another workspace's facet values.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.5:** The host shall render only nonempty
  value sections, order them by label case-insensitively, and keep distinct value
  identifiers separate even when their labels are equal.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.6:** On desktop and phones, a valid optional
  value color shall appear as a decorative section-heading swatch under the
  existing host color-validation rules. Missing or unsafe colors and Unassigned
  shall have no swatch; built-in headings shall remain unchanged.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.7:** A task returning several valid values
  shall appear once in each matching section. Repeated value identifiers for one
  task shall not duplicate its row within a section.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.8:** A task with no valid values shall
  appear in one trailing section using the shared `sidebar:groupUnassigned` copy.
  Blank identifiers or labels shall create no value section. A plugin value
  labelled Unassigned shall remain distinct from the host fallback.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.9:** Each section shall build its hierarchy
  only from its own matching tasks. A child whose parent is absent shall appear
  at that section's root; a parent's membership shall not implicitly include its
  descendants. An absent child shall not inflate the count or expose an empty
  subtask toggle.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.10:** Group and parent/subtask collapse
  shall remain independent across values and facets, including after saving and
  reloading the view.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.11:** Actions and selection shall retain
  canonical task identity. Shift-click range endpoints shall refer to the clicked
  section-qualified occurrences rather than the first copy in another section;
  bulk operations shall deduplicate canonical task identifiers.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.12:** A plugin change notification shall
  update memberships and labels without refetching tasks. Facet registration and
  unregistration shall update the sidebar and its Group by control reactively.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.13:** While a requested facet is unavailable,
  the list and control shall use None without overwriting the saved selection or
  draft, and shall restore the requested grouping when the facet registers again.
  A failing value callback shall remain contained and treat that task as Unassigned.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.14:** An unrecognized saved grouping value
  shall not prevent opening the view editor. Its summary and Group by control
  shall display None without writing a fallback; the user shall remain able to
  explicitly choose an available grouping.
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.15:** Root drag reordering shall be
  disabled in facet sections containing children promoted from absent parents;
  those children shall retain the accepted page's real-parent sibling order.
  Sections containing only real roots and ordinary nested sibling reordering
  shall retain their existing behavior.

## Out of scope

- Sidebar facet sorting or filtering, nested grouping dimensions, and changing
  `/tasks` preferences or behavior.
- Server-side facet queries, ordering, counts, or loading every workspace task.
- A host-owned tag vocabulary, asynchronous per-task reads, a second registration
  API, plugin SDK changes, or backend schema changes.
- Changing built-in grouping behavior or unrelated task interaction contracts.
