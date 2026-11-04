---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-SIDEBAR-TASK-FACETS-001
---

# Plugin Sidebar Task Facets Design

## Purpose and boundaries

Extend the existing task-list facet contribution into workspace-scoped sidebar
saved views. Reuse the registry, synchronous resolver, validation, subscriptions,
and namespaced option encoding; introduce no SDK API or backend schema.

[Requirements](../requirements/sidebar-task-facets.md) own observable behavior.
The [delivery plan](../../../plans/sidebar-task-facets/plan.md) records scope and
verification. `/tasks` retains its separate preferences and rendering path.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-SIDEBAR-TASK-FACETS-001` | Selection and availability, Accepted page projection, Section and task identity, Rendering and interaction |

## Selection and availability

`GroupKey` combines the existing built-in union with `TaskListFacetOption`.
The existing sidebar wire format stores the namespaced string in both saved views
and drafts. Workspace-scoped sidebar state remains the persistence owner.

`GroupPicker` appends live registry facets after the unchanged built-in options.
`useSidebarGroupLabel` resolves the editor disclosure summary through that same
registry. A missing facet uses None in the control and summary without emitting a
change; registration restores the original selection. Unknown non-facet strings
also use None rather than dereferencing a missing built-in option. Only explicit
user selection changes the draft. Neither component repairs persisted values as
a render side effect.

## Accepted page projection

The sidebar task-page query substitutes built-in `none` for a facet request and
omits server disclosure for that request: collapse in one projected section must
not hide another section's copy at the source. Existing filters, sort chain,
pinning, sibling ordering, and pagination still select the bounded source rows.
No facet-specific backend query or workspace-wide fetch is introduced.

`useSidebarFacetGrouping` obtains accepted tasks with `sidebarPageTasks`, passes
them and the explicit sidebar workspace ID to `useTaskListFacets`, and projects
only while the selected facet is registered. Built-in grouping returns the
existing page and shared child map unchanged. Resolver subscriptions, registry
updates, workspace changes, and locale changes invalidate the local projection.
Value callback exceptions and invalid values remain handled by the existing
facet resolver.

`groupSidebarTasksByFacet` creates one bucket per value ID, deduplicating values
per task. Sections sort by case-insensitive label, with a key tie-breaker. A
nonempty Unassigned section follows all value sections. Group keys encode the
facet and value separately from the fallback, so equal labels and a plugin value
named Unassigned cannot collide.

## Section and task identity

Each facet bucket rebuilds its own tree with the existing ungrouped tree builder.
Only section members appear in that tree. Children whose parent is absent become
roots; membership never pulls in descendants. The facet result has no global
child map: consumers use each section's map for rendering, counts, collapse,
memoization, and traversal. Built-in groups keep the shared map.

Collapse identity encodes section and parent task ID. React row identity and
shift-range endpoint identity are occurrence-local; task actions still receive
canonical IDs. Range traversal uses the clicked occurrence rather than searching
for the first canonical ID, and bulk task operations deduplicate IDs afterward.

## Rendering and interaction

Desktop and mobile headings render the optional decorative swatch through the
existing bounded CSS color validator. Unassigned and built-in headings do not
acquire plugin swatches. Existing sidebar Group by surfaces and translations
remain the control owners.

A facet section containing a promoted child root has no root reorder callback.
This disables both root drag activators and the group-spanning root reorder
handler, avoiding persistence of promoted children as real roots. The accepted
page's real-parent sibling order is retained. Sections containing only real
roots and ordinary nested sibling lists keep their existing reorder behavior.
This does not alter task navigation, selection, or canonical action targets.

## Compatibility and failure behavior

No plugin implementation change is required for an existing compatible facet.
Unregistering the plugin returns to the accepted ungrouped page and None in the
editor without changing the requested saved grouping. Re-registering restores
projection. The existing resolver contains callback errors per task. Unknown
saved built-in values cannot crash the editor summary or leave its selector
without a usable fallback.
