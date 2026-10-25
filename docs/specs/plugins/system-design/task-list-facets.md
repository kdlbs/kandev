---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-001
  - REQ-PLUGINS-TASKLIST-FACETS-002
  - REQ-PLUGINS-TASKLIST-FACETS-003
---

# Plugin Task List Facets System Design

## Purpose and boundaries

This design covers the host half of plugin task-list facets: the contribution
point a plugin bundle registers, the `/tasks` controls that expose it, the
page-local sectioning and ordering the host applies, and the availability rules
that keep a saved selection usable while its plugin is not active.

The plugin system owns this contract because the contribution point, the
registration lifecycle, and the availability rules are all plugin-system
concerns. The host does not own facet semantics: it never learns what a value
means, never interprets a value, and never sends one to the task list API.

Adjacent contracts used but not owned here:

- The task list request and its order-by behavior are owned by the task system.
  This design only fixes which sort value a list request may carry.
- The persisted sort and group preference is owned by the user-settings
  contract. This design adds one accepted shape to it.
- Per-user plugin storage (`host.storage`) is owned by the plugin system's
  per-user storage contract and is reused, not extended, by facet consumers.
- Task row presentation of a facet value is a separate contribution contract
  and stays out of scope.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-TASKLIST-FACETS-001` | [Contribution contract](#contribution-contract), [Host registry](#host-registry), [Control surfaces](#control-surfaces) |
| `REQ-PLUGINS-TASKLIST-FACETS-002` | [Selection model](#selection-model), [Section building](#section-building), [Facet ordering](#facet-ordering), [Failure and recovery](#failure-and-recovery) |
| `REQ-PLUGINS-TASKLIST-FACETS-003` | [Preference encoding](#preference-encoding), [Selection model](#selection-model), [Persistence](#persistence), [Failure and recovery](#failure-and-recovery) |

## Contribution contract

`apps/packages/plugin-sdk/src/index.ts` defines the public shapes:

- `TaskListFacetValue` - `{ value, label, color? }`.
- `TaskListFacetContext` - `{ taskId, workspaceId? }`, named so the
  registration signature stays stable as the context grows.
- `TaskListFacetRegistration` - `{ id, label, getValues(context), subscribe? }`.

`getValues` is synchronous and returns every value the task carries. `subscribe`
is optional; when present it returns an unsubscribe function and fires whenever
the plugin's values change. The host treats a throwing callback as a plugin
failure and isolates it.

`apps/web/lib/plugins/types.ts` re-exports the shapes for host code, and
`docs/plans/plugins/PLUGIN-API.md` plus `docs/public/plugins-authoring.md`
document them, including the multi-value, unassigned, ordering, and persistence
outcomes.

## Host registry

`apps/web/lib/plugins/registry.ts` stores registrations as
`Owned<TaskListFacetRegistration>[]` alongside the other contribution lists.
The plugin-scoped view built by `forPlugin` exposes `registerTaskListFacet`, and
`getTaskListFacets()` returns each entry paired with its owning `pluginId`.

`registerTaskListFacet` rejects a duplicate plugin-local id and an id that is not
a URL-safe slug (`^[a-z0-9]+(?:-[a-z0-9]+)*$`), matching the encoding the
preference uses. `unregisterPlugin` drops the plugin's facets with its other
registrations, and the registry's `totalCount` includes them so lifecycle tests
detect a leak.

`apps/web/lib/plugins/registry-registration-types.ts` owns
`PluginTaskListFacetRegistration` and
`pluginTaskListFacetRegistrationKey(registration)` - the `pluginId:facetId`
pairing that both the control value and the registry lookup derive from.

## Selection model

`apps/web/hooks/use-task-list-facets.ts` resolves the active facets against the
loaded page:

- `useTaskListFacets(tasks, workspaceId)` returns `facets` (each carrying its
  `key`, the encoded preference value) and `values`, a map from
  `<facetKey>:<taskId>` to the values that the facet returned for that task.
- `resolveTaskFacetValues` calls `getValues` per loaded task, drops malformed
  entries, and catches a throwing `getValues` by recording an empty value list
  for that task. It also drops an entry whose label is empty, per entry: the
  audit is entry by entry, so a task returning
  `[{ value: "z", label: "" }, { value: "a", label: "Alpha" }]` keeps Alpha, and
  the empty-label entry changes nothing. A task whose entries are all
  empty-label ends up with no values, so it lands in the unassigned bucket and
  sorts last, matching `AC-PLUGINS-TASKLIST-FACETS-002.8`.
- The section builder applies the same empty-label rule on the values map it
  receives, so a caller that renders the list from raw facet values (the view
  takes `facetValues` as a prop) cannot produce a section titled with the empty
  string, and `firstFacetLabel` skips empty labels so the sorter agrees on its
  own input. All three read the same rule from the same definition of "not a
  value" (`AC-PLUGINS-TASKLIST-FACETS-002.8`); the resolver's discard is what
  makes the hook's exposed map already filtered.
- Subscriptions registered by `subscribe` bump a revision counter, which
  recomputes `values` without a reload.

`apps/web/hooks/use-task-list-facet-selection.ts` owns the two-level selection:

- **Requested** - the value read from the query or the stored preference. It may
  be a built-in option or a facet option, and the host never rewrites it while a
  facet is requested.
- **Effective** - a facet request is effective only when its key is present in
  the resolved facet list. Otherwise the effective value is the built-in default
  (`DEFAULT_TASKS_LIST_SORT`, `DEFAULT_TASKS_LIST_GROUP`).

The hook returns the effective values for the controls and the list, the
resolved facet keys used for sectioning and ordering, and the change handlers.
Built-in selections keep the existing path: they update the stored preference and
the query, and a selectable facet does the same with its facet option, so
`REQ-PLUGINS-TASKLIST-FACETS-003` needs no separate persistence trigger.

## Preference encoding

`apps/web/lib/tasks/tasks-list-options.ts` owns encoding and decoding:

- `TASK_LIST_FACET_PREFIX` (`facet:`) and `isTaskListFacetOption`.
- `encodeTaskListFacetOption({ pluginId, id })` builds
  `facet:<pluginId>:<facetId>`; `parseTaskListFacetOption(value)` returns the
  pair or `null` for anything else.
- `parseTasksListSort` and `parseTasksListGroup` return either a built-in option
  or a facet option, and they return the **trimmed** token rather than the raw
  input, matching the server's existing trim in `NormalizeTasksListSort` and
  `NormalizeTasksListGroup`; the client-side trim and its trimmed return are
  required by `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3` and
  `AC-PLUGINS-TASKLIST-FACETS-003.4`, which forbid the current client/server
  asymmetry. The shipped parsers compare exactly, so a padded built-in token
  fails the match and returns the built-in default, and the query-param sync then
  persists that default over the stored value: `?sort=%20title_asc%20` renders as
  the default on the client while the boot payload resolves `title_asc`, and the
  user's stored choice is overwritten. Trimming on match alone does not fix it;
  the returned token must be the trimmed one.
- `resolveTasksListApiSort(requested)` returns the built-in sort to use for a
  task-list query: the requested built-in value, or `TasksListSortDefault` for a
  facet. No analogous resolver is needed for grouping because grouping never
  reaches a query. It applies at all three live query sites: the client refetch
  in `useTaskOperations`, the Go boot handler that serves the first page, and the
  task-list HTTP endpoint's `sort` gate in
  `internal/task/handlers/task_http_handlers.go`. That third site is load-bearing:
  it currently calls `NormalizeTasksListSort`, which delegates to the widened
  `IsValidTasksListSort`, so without routing it through the resolver
  `GET /api/v1/workspaces/:id/tasks?sort=facet:<pluginId>:<facetId>` forwards the
  facet identifier into `ListTasksByWorkspaceWithArchiveMode` and on to
  `taskListOrderBy`, absorbed only by the default branch. The boot handler's own
  site has a second obligation: it uses one resolved value twice, so the
  first-page query takes the built-in value while the payload's `tasksListSort`
  field keeps the requested value for the client. The backend resolver is
  `TasksListApiSort` in the same model package, and it tests the facet-shaped
  predicate `IsTaskListFacetValue` rather than `IsValidTasksListSort`, which
  accepts facet values.
- `apps/web/app/tasks/page.tsx` is a non-executing Next-era twin: nothing
  imports it, the package has no `next` dependency, and the live `/tasks` route
  is built by `apps/web/src/spa-routes.tsx` from `TasksPageClient` plus the Go
  boot payload. It is maintained for type compatibility only, and it is not a
  query site whose behavior this design constrains.

`IsTaskListFacetValue` is deliberately spelled after the facet family
(`TaskListFacetRegistration`, `TaskListFacetValue` in the SDK,
`isTaskListFacetOption` in the web host) rather than after the `TasksList*`
sort-and-group family of the file it lives in, so the one predicate that
recognizes the encoded form reads the same in Go and TypeScript. The backend
mirrors the same shape rule segment by segment in
`apps/backend/internal/user/models/tasks_list_preferences.go`. The plugin-id
segment follows the plugin identity grammar the manifest already enforces
(`^[a-z0-9][a-z0-9._-]*$` in `internal/plugins/manifest/validate.go`), which
permits dots and underscores but never a colon, so the two colons of the encoded
form stay unambiguous. The facet-id segment follows the registration rule the
host registry enforces (`^[a-z0-9]+(?:-[a-z0-9]+)*$`). `IsValidTasksListSort` and
`IsValidTasksListGroup` accept a built-in value or a well-formed facet value,
`NormalizeTasksListSort` and `NormalizeTasksListGroup` keep an accepted facet
value and trim it, and anything else still normalizes to the built-in default.
A facet id narrower than the manifest grammar is deliberate: a plugin id this
host accepts must not be rejected by a preference write. `TasksListFacetValues()`
-style enumeration is deliberately not added: the backend cannot enumerate a set
defined by installed plugins.

## Control surfaces

- Desktop: `apps/web/app/tasks/tasks-list-controls.tsx` lists the built-in Sort
  and Group options first and appends one entry per active facet, labelled with
  the plugin's label and never resolved through `t()`.
- Phone: `apps/web/components/kanban/mobile-menu-task-list-options.tsx` offers
  the same two option lists in the Display Options sheet, with the same
  ordering and labels. Its group help copy
  (`kanban:groupTasksIntoSectionsByState`) names plugin-provided values as
  a grouping dimension in every shipped locale.

Both surfaces read the option list from `useTaskListFacets` and bind the
effective selection, so a fallback is visible in the control without changing
the stored preference.

## Section building

`apps/web/lib/tasks/tasks-list-sections.ts` owns section construction, extracted
from `apps/web/app/tasks/tasks-list-view.tsx` so it is unit-testable without
rendering. `buildTaskSections(tasks, options)` takes the grouping value, the
workflow and repository label maps, the resolved facet values, and a translate
function for host copy.

For a facet grouping value it:

1. collects one bucket per value returned by at least one loaded task, keyed by
   `facet:value:<value>`, keeping the value's label and color. A task pushed into
   the same bucket twice by a repeated value still yields one row and a count of
   one, because the per-bucket tree rebuild drops an already-visited task id; no
   separate dedupe layer is needed;
2. appends a single bucket for the tasks that returned no value, keyed
   `facet:host:ungrouped` and titled with `sidebar:groupUnassigned`;
3. sorts the value buckets by label with a case-insensitive comparison and
   leaves the unassigned bucket last, so a value label can never displace it;
4. rebuilds the parent/child tree inside each bucket from that bucket's tasks
   alone, so a child whose parent is not in the bucket renders at the bucket
   root;
5. renders the value's color as a swatch in the section header.

Distinct keys keep a plugin value literally named `untagged` or
`__host_ungrouped__` separate from the host's unassigned bucket.

## Facet ordering

`sortTasksByFacet(tasks, facetKey, values)` orders the loaded rows by
`firstFacetLabel`, the alphabetically first value label compared
case-insensitively. Tasks with no value sort last, and equal labels keep the
incoming order through the recorded index. The ordering never leaves the page: a
list request carries `resolveTasksListApiSort(requestedSort)`, and
`taskListOrderBy` in `apps/backend/internal/task/repository/sqlite/task.go`
keeps its default branch as the backstop for any unrecognized sort string.

A facet selection updates the requested value, the query, and the stored
preference, and it must **not** run the built-in local re-sort: `sortTasksForList`
/ `compareTasksForList` switch on the built-in values and fall to `updated_desc`,
so re-sorting the loaded rows with a facet value would rewrite the very order the
facet sort preserves for equal labels. With the re-sort skipped, the order the
facet sort preserves is the order the rows were loaded in, which is the order the
API resolved for the request — for a facet that is `TasksListSortDefault`, because
`resolveTasksListApiSort` maps a facet to it. The boot-row projection in
`apps/web/src/spa-routes.tsx` must copy the incoming rows without a local re-sort
when the requested sort is a facet. SQLite `COLLATE NOCASE` and JavaScript
`localeCompare` can order tied titles differently, so applying the built-in
default again would change the incoming order even for rows already sorted by
the API. The query-param sync and selection handlers in
`apps/web/app/tasks/tasks-page-client.tsx` likewise skip the built-in sorter for
facets. Built-in requests retain their existing local sort. Facet ordering is
owned solely by `sortTasksByFacet` at render.

## Control flow

Boot and restore:

1. The boot payload supplies the stored sort and group preference
   (`tasksListSort`, `tasksListGroup` from
   `apps/backend/internal/backendapp/boot_state_routes.go`).
2. A `sort` or `group` query value wins over the stored preference, through
   `tasksListSortForRoute` / `tasksListGroupForRoute`, both of which accept the
   facet shape.
3. The boot handler resolves the requested sort twice from one source: the
   task-list query for the first page gets a built-in sort, and the boot
   payload's `tasksListSort` keeps the requested facet value so the client can
   bind the control. Together with the client refetch, no task-list query
   carries a facet identifier.
4. `useTasksListPreferenceSync` treats a query-supplied value as the requested
   selection and persists it, so a shared
   `?group=facet:<pluginId>:<facetId>` link restores and re-saves the facet.
5. Facet bundles load asynchronously after boot. Until the owning plugin
   registers, the requested facet is not resolvable, so the effective value is
   the built-in default; the registry version change re-resolves it into the
   facet selection without a reload.

Selection change: a control change sets the requested value, updates the stored
preference through the user-settings API, and rewrites the query. A facet request
also resolves the API sort back to the built-in default for the next list
request, and it does not run the built-in local re-sort, so the order it preserves
for equal labels is the order the rows were loaded in (see
[Facet ordering](#facet-ordering)). A built-in selection keeps its existing
behavior, including the local re-sort that keeps the visible page consistent
before the next fetch.

Value change: a facet `subscribe` notification bumps the resolution revision.
Sections and ordering recompute from the new values; the loaded rows and the
query are untouched.

Plugin lifecycle change: `unregisterPlugin` removes the facet, the resolved
facet list loses the key, the requested facet becomes unresolvable, and the
effective value falls back to the built-in default.

## Failure and recovery

- A throwing `getValues` records an empty value list for that task, so the task
  lands in the unassigned bucket instead of breaking the list.
- A throwing `subscribe` registration is logged and skipped; the facet stays
  selectable and simply stops receiving live updates.
- A throwing unsubscribe is logged and does not prevent the remaining
  subscribers from being released.
- A requested facet whose plugin is inactive, disabled, uninstalled, or failed
  resolves to the built-in default and never rewrites the stored preference.
- A stored value matching neither a built-in option nor the facet shape is
  normalized to the built-in default on read, in the store and in the boot-state
  route; a write carrying such a value is rejected by the user service with an
  error naming both accepted forms. Neither side silently rewrites a stored value
  that is already valid.
- The order-by builder keeps its default branch as a defensive backstop, so an
  unrecognized sort string can never produce an unspecified order. It is not a
  sanctioned path: every host request site resolves a facet to a built-in sort
  before issuing the request.

## Persistence

The selection is stored in the existing `tasks_list_sort` and
`tasks_list_group` user settings rows through the user-settings API; no new
column, table, or endpoint is introduced. The facet form is accepted by shape, so
installing or removing a plugin never changes the stored schema and never
invalidates an existing row. A row may hold a facet whose plugin is not
installed; that is a supported state and preserves the choice across plugin
state changes.

Facet values themselves are never persisted by the host. A facet consumer that
needs durable values owns them in its own plugin storage scope.

## Security

Facet labels and value labels are plugin-supplied strings rendered as text
through React, so they cannot inject markup. A value's `color` is applied only
to the section swatch's `backgroundColor`, where the browser discards anything
that is not a color. Facet callbacks run in the plugin's own bundle context and
therefore carry the plugin's existing trust; the host adds no privilege by
calling them. A facet cannot affect the task list request, the task store, or
another plugin's registrations.

## Observability

The existing host error paths log through `console.error` with the
`[plugins] task-list facet "<pluginId>:<facetId>"` prefix for the `getValues`
and `subscribe` failures, so those faults are attributable to one plugin and one
facet. The unsubscribe failure path logs `[plugins] task-list facet
unsubscribe() threw` without the facet id; that is existing behavior and stays
outside this change's scope. The effective preference remains visible through the
user-settings API and the `/tasks` query, which distinguishes a fallback (query
keeps the facet, control shows the default) from a stored default. No new metric
is warranted: the feature adds no background work and no autonomous transition.

## Related decisions

- [ADR-2026-09-22-plugin-facet-selection-persistence](../../../decisions/2026-09-22-plugin-facet-selection-persistence.md)
- [ADR-2026-08-04-plugin-contribution-lifecycle-authority](../../../decisions/2026-08-04-plugin-contribution-lifecycle-authority.md)
- [ADR-2026-08-01-per-user-plugin-storage](../../../decisions/2026-08-01-per-user-plugin-storage.md)
