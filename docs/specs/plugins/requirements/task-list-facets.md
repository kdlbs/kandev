---
status: active
system: plugins
created: 2026-09-22
owners:
  - kandev
---

# Plugin Task List Facets Requirements

## Overview

`/tasks` lists the tasks of the active workspace and offers Sort and Group
controls that shape that list. Some task dimensions are owned by a plugin rather
than by Kandev: a label, a tag, a provider state. A plugin publishes such a
dimension as a **facet**, and the host turns it into an additional Sort and Group
choice.

The plugin system owns this contract. A plugin supplies the facet identity, its
label, and the values per task; the host owns the contribution point, the
desktop and phone control surfaces, the page-local ordering and sectioning
rules, and the availability rules that apply when the owning plugin is not
active. The host never learns what a facet means, and no facet knowledge reaches
the task list API or the task store.

## Terminology

- **Facet:** A plugin-owned dimension of a task, identified by the owning plugin
  id plus a plugin-local facet id, and published through the host's task-list
  contribution point.
- **Facet value:** One `{ value, label, color? }` entry that a facet returns for
  one task. A task can return several values for the same facet.
- **Facet selection:** The facet currently chosen in the Sort or Group control of
  `/tasks`.
- **Built-in option:** A Sort or Group choice that ships with the host.
- **Active plugin:** A plugin whose UI bundle has registered its contributions in
  the current session.
- **Unassigned section:** The trailing section that holds the loaded tasks which
  return no value for the grouping facet.

## Requirements

### REQ-PLUGINS-TASKLIST-FACETS-001: Facets Are Selectable Task List Dimensions

**Intent:** A plugin can make its own task dimension selectable in the `/tasks`
Sort and Group controls, on desktop and on phones, without the host modelling
that dimension.

**User story:** As a plugin author, I want the host's task list to offer my
dimension as a sort and group choice, so that users can organize the list by the
data my plugin owns without me rebuilding the page.

#### Acceptance criteria

- **AC-PLUGINS-TASKLIST-FACETS-001.1:** When a plugin registers a facet whose id
  is a URL-safe slug and whose label is non-empty, the host shall offer one
  additional choice in the `/tasks` Sort control and one in the `/tasks` Group
  control, positioned after every built-in option; a registration whose id is not
  a URL-safe slug shall be rejected and shall not become selectable.
- **AC-PLUGINS-TASKLIST-FACETS-001.2:** The facet choice shall render the
  plugin-supplied label verbatim, and the host shall not substitute a host
  translation for it.
- **AC-PLUGINS-TASKLIST-FACETS-001.3:** While no plugin registers a facet, the
  host shall list only built-in options in both controls and shall render the
  task list as it renders without the facet capability.
- **AC-PLUGINS-TASKLIST-FACETS-001.4:** On a phone viewport, the Display Options
  sheet shall offer the same facet choices in its Sort and Group selectors, and
  the sheet's group help text shall name plugin-provided values as a grouping
  dimension in every shipped locale.
- **AC-PLUGINS-TASKLIST-FACETS-001.5:** When the owning plugin unregisters, is
  disabled, or is uninstalled, the facet choice shall disappear from both
  controls without a page reload, and the list shall continue to render.
- **AC-PLUGINS-TASKLIST-FACETS-001.6:** The published plugin API reference and
  the plugin authoring guide shall describe the facet contribution as a choice
  whose selection is stored in the host task-list preference, shall list the
  facet selector alongside the other registry selectors, shall not state that a
  facet selection is never persisted or never reaches the backend, and shall
  name the section for tasks without a value with the copy the host actually
  renders.
- **AC-PLUGINS-TASKLIST-FACETS-001.7:** A plugin that registers a facet with a
  fixed id and label, a synchronous `getValues` that reads only the task and
  workspace identifiers from the context, and a `subscribe` that returns an
  unsubscribe function shall be accepted and offered as a Sort and Group choice
  without any plugin-side change; the host shall not require a facet to declare
  a value vocabulary, a value type, or an asynchronous read.

### REQ-PLUGINS-TASKLIST-FACETS-002: Facet Grouping And Sorting Are Page-Local And Per-Value

**Intent:** Grouping and sorting by a facet reflect exactly what the plugin
returns for the rows already loaded, and every value a task carries stays
reachable.

#### Acceptance criteria

- **AC-PLUGINS-TASKLIST-FACETS-002.1:** When the list groups by a facet, the host
  shall create exactly one section per value returned by at least one loaded
  task, order the value sections by label using a case-insensitive comparison,
  and show the value's color as a swatch in the section header when the value
  supplies one.
- **AC-PLUGINS-TASKLIST-FACETS-002.2:** A task that returns several values for
  the grouping facet shall appear once in each matching value section, and no
  more than once in any single section.
- **AC-PLUGINS-TASKLIST-FACETS-002.3:** The host shall render the loaded tasks
  that return no value for the grouping facet in one section placed after every
  value section and labelled with the shared `sidebar:groupUnassigned` copy, and
  shall not create a value section for a value that no loaded task returns.
- **AC-PLUGINS-TASKLIST-FACETS-002.4:** Parent and child indentation inside a
  value section shall be rebuilt from the tasks of that section, so a child whose
  parent is not in the section renders at that section's root.
- **AC-PLUGINS-TASKLIST-FACETS-002.5:** When the grouping facet notifies the host
  that its values changed, the host shall rebuild the sections from the new
  values without reloading the page.
- **AC-PLUGINS-TASKLIST-FACETS-002.6:** When the list sorts by a facet, the host
  shall order the loaded rows by the alphabetically first value label of each
  task using a case-insensitive comparison, place the tasks that return no value
  last, and keep the incoming order for rows whose labels compare equal. The
  incoming order is the order of the rows as loaded by the list request, and
  selecting the facet shall not locally re-order those rows, so the same page
  order yields the same facet ordering.
- **AC-PLUGINS-TASKLIST-FACETS-002.7:** The host shall not include a facet
  identifier in a task list query, on the boot-served first load or on a client
  refetch; a list sorted by a facet shall query with a built-in sort at every
  task-list query site and reorder the fetched rows at render time.
- **AC-PLUGINS-TASKLIST-FACETS-002.8:** A returned value whose label is empty
  shall not count as a value: the host shall create no section for it, shall not
  let it change the task's ordering, and shall treat a task whose values are all
  empty-label as returning no value. A returned value with a non-empty label is
  unaffected by an empty-label sibling.

### REQ-PLUGINS-TASKLIST-FACETS-003: A Facet Selection Survives Reload, Links, And Plugin State Changes

**Intent:** A facet selection behaves like a built-in selection for the user, and
a plugin that is temporarily unavailable does not destroy it.

#### Acceptance criteria

- **AC-PLUGINS-TASKLIST-FACETS-003.1:** When a user selects a facet in Sort or
  Group, the host shall record `facet:<pluginId>:<facetId>` in the existing
  task-list sort or group preference and in the `/tasks` query, and a later load
  of `/tasks` with that query shall present the facet as the selected choice and
  apply its grouping or ordering.
- **AC-PLUGINS-TASKLIST-FACETS-003.2:** The host shall accept the
  `facet:<pluginId>:<facetId>` form when reading the stored sort or group
  preference through the user-settings API, and shall normalize a stored value
  that matches neither a built-in option nor that form to the built-in default.
- **AC-PLUGINS-TASKLIST-FACETS-003.3:** While the plugin that owns a stored facet
  selection is not active, the host shall present the built-in default in the
  list and in both controls, shall not overwrite the stored value, and shall
  restore the facet selection without user action once that plugin becomes
  active.
- **AC-PLUGINS-TASKLIST-FACETS-003.4:** When a stored or query-supplied sort or
  group value carries surrounding whitespace, the server and the client shall
  resolve it to the same value, trimming before matching and keeping the trimmed
  value on both sides, so neither a padded facet value nor a padded built-in
  value yields the facet or the built-in choice on one side and the opposite on
  the other.

## Out of scope

- Facet values on the kanban board or a task's own page. Task row metadata is a
  separate contribution contract. Sidebar saved-view grouping is covered by
  [Plugin Sidebar Task Facets](sidebar-task-facets.md); sidebar facet sorting
  and filtering remain out of scope.
- Server-side filtering, ordering, or counting by a facet value.
- Facet-defined grouping nested inside the `state`, `workflow`, or `repository`
  dimensions.
- A host-owned vocabulary of facet values. The host never interprets a value.
- Facet contributions that need an asynchronous read for a single task; facet
  values are supplied synchronously from data the plugin already holds.
