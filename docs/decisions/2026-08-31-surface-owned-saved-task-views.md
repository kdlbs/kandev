# ADR-2026-08-31-surface-owned-saved-task-views: Keep Saved Task Views Surface Owned

**Status:** proposed
**Date:** 2026-08-31
**Area:** backend, frontend

## Context

The task sidebar already saves filters, sort, grouping, collapsed groups, and
task-row presentation. Threads needs filters and sort, but it also needs an
explicit task scope and a maximum column count.

One shared saved-view collection exposes irrelevant sidebar fields in Threads.
It also makes a view switch in one surface change the other surface. Separate
implementations without shared query primitives let filter operators and
task-field meanings drift.

Portable task views must follow the existing backend-owned user-settings
boundary.

## Decision

Each task surface owns its saved-view collection, active view, draft, and
presentation fields. The sidebar and Threads do not share saved view IDs or
active selection.

Task surfaces share typed filter clauses, operators, values, sort direction,
and reusable editor primitives. Each surface defines its own dimension and
sort-key registry.

Threads views persist in the existing backend user-settings JSON. Browser
storage is not a fallback source. The feature adds no saved-view table and no
task-query endpoint.

## Consequences

- Sidebar and Threads users can organize tasks independently.
- Threads can add task scope and column limits without changing sidebar view
  compatibility.
- Shared query primitives keep common filter and sort semantics consistent.
- User settings gain another bounded saved-view collection and draft.
- A future task surface must choose its own presentation contract. It can reuse
  query primitives without taking ownership of another surface's views.
- Cross-surface view sharing needs a separate product contract and migration.

## Alternatives Considered

### Reuse sidebar views directly

Rejected because grouping, collapsed groups, and task-row settings have no
Threads meaning. Shared active selection also couples unrelated surfaces.

### Copy the sidebar implementation into Threads

Rejected because filter operators, wire normalization, and editor behavior can
drift.

### Store Threads views in browser storage

Rejected because the views are portable user preferences. This option
conflicts with ADR 0041.

### Add a saved-view table and endpoint

Rejected because user settings already provide bounded persistence, revision
ordering, boot hydration, and live updates for private user preferences.
