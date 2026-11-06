# ADR-2026-09-22-plugin-facet-selection-persistence: Persist Plugin Facet Selections In The Host Sort And Group Preference

**Status:** accepted
**Date:** 2026-09-22
**Area:** frontend

## Context

`/tasks` accepts plugin-provided sort and group facets through
`registry.registerTaskListFacet`. The shipped contribution point is documented as
strictly page-local: the selection is held in component state and, per
`docs/plans/plugins/PLUGIN-API.md`, "no facet selection is persisted or sent to
the backend".

A released tags plugin needs more than that. A user who groups `/tasks` by tag
expects the choice to survive a reload, to be shareable as a link, and to return
after the plugin is disabled and enabled again. That requires the selection to
live in the host preference that already stores the `/tasks` sort and group
choice.

The obstacle is that `tasks_list_sort` and `tasks_list_group` are closed enums
owned by `internal/user/models`. A facet identifier is defined by whichever
plugins are installed at read time. The backend cannot validate a facet against a
list of known values, and it must not acquire knowledge of plugin state to render
a task-list preference.

## Decision

The saved `/tasks` sort and group preference may hold
`facet:<pluginId>:<facetId>` in addition to the built-in values.

- The backend accepts that form for both enums by shape, segment by segment: the
  plugin-id segment follows the plugin identity grammar the manifest already
  enforces (`^[a-z0-9][a-z0-9._-]*$`), which admits dots and underscores and
  never a colon, and the facet-id segment follows the host registry's URL-safe
  slug rule. It does not consult installed plugins, does not resolve the
  identifier, and does not enumerate facets. A value matching neither a built-in
  nor the facet shape normalizes to the built-in default, and the built-in enum
  lists remain the source of truth for built-in values.
- The frontend owns availability resolution. A saved facet selection whose plugin
  is not currently active - disabled, uninstalled, or still loading - renders as
  the built-in default in both the list and its controls, without rewriting the
  saved value. The facet selection resumes when the plugin becomes active.
- A facet identifier is never sent to a task-list query. All three live
  task-list query sites resolve the requested sort to a built-in sort first: the
  browser refetch, the Go boot handler, which serves the first page and must
  therefore resolve the query while keeping the requested facet in the boot
  payload it returns to the client, and the task-list HTTP endpoint's `sort`
  gate, which normalizes through the same predicate this decision widens and must
  therefore route through the resolver instead. Facet ordering is applied to the
  loaded page at render time. The order-by builder's default branch remains a
  defensive backstop, not a sanctioned path.
- Plugin-authored facet labels and value labels are rendered verbatim. They
  never pass through the host translation layer.

## Consequences

A preference can hold a value that no installed plugin can serve. That is
deliberate: the value must outlive a plugin disable, uninstall, or install, and
must be restored when the plugin returns. Shape validation cannot detect a
mistyped facet identifier, so the frontend fallback is the only correction path,
and it must not rewrite the stored preference.

The two segments deliberately use different rules. The plugin-id segment must
accept every id this host can install, or a legally named plugin's preference
write would be rejected and silently discarded; the facet-id segment only has to
match the host registry's own registration rule. A narrower plugin-id rule would
turn a valid plugin into the silent-destruction state this decision forbids.

The host must distinguish the requested selection from the effective selection.
Persisting or displaying the fallback would silently destroy the user's choice.

Deep links stay stable across plugin state, and the backend stays free of plugin
knowledge for a preference that is purely presentational. The invariant that a
facet never reaches a task-list query keeps facet ordering page-local, so paging
behavior is unchanged.

## Alternatives Considered

Persisting the selection in plugin-owned storage through `host.storage` was
rejected: the host could not render a fallback without an active plugin, and the
deep link would still need a host-side identifier.

Extending the backend enum with the currently installed facets was rejected: the
preference contract would change whenever a plugin is installed or removed, deep
links would break across plugin state, and the backend would need plugin
knowledge for a UI preference.

Rejecting an unrecognized facet value and normalizing it to the default was
rejected: the selection must return when the plugin returns, which requires
keeping the value.

Persisting an opaque per-plugin index instead of the identifier was rejected: it
is not stable across plugin updates, not human-readable in a URL, and not
resolvable without the plugin.
