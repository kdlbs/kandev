# ADR-2026-08-04-navigation-manifest-boundaries: Centralize Navigation and Namespace Plugin Destinations

**Status:** accepted
**Date:** 2026-08-04
**Area:** frontend

## Context

The sidebar, mobile menu, and command palette previously declared overlapping
destination lists independently. That made navigation coverage difficult to
review and allowed `/stats` to remain unreachable on phones. The first shared
manifest fixed that gap, but its catalog, types, plugin mapping, surface policy,
and resolution rules initially lived in one growing module.

Plugin navigation introduced a second boundary problem. `NavItem.id` is local to
the plugin that registered it, while resolved destination IDs are React keys.
Using only the item ID allowed two plugins, or a plugin and a first-party
destination, to collide.

## Decision

- Keep one compact first-party catalog in `core-destinations.ts`. Destinations
  explicitly declare the surfaces on which they may appear.
- Keep shared vocabulary in `types.ts`, plugin conversion and identity in
  `plugin-destinations.ts`, filtering and copy/href resolution in
  `resolve-destinations.ts`, and cross-surface policy in `surface-policy.ts`.
- Preserve plugin ownership through `PluginNavRegistration`. Build internal
  plugin destination IDs as
  `plugin:<encodeURIComponent(pluginId)>:<encodeURIComponent(itemId)>` and keep
  the raw item ID separately only for compatibility selectors.
- Keep availability-gated and static consumers distinct. Static consumers must
  not mount integration polling merely to render ungated links.

The following concerns remain explicit follow-up decisions rather than fields or
validation added before a concrete requirement exists:

- Catalog order is shared across surfaces. Add per-surface order or priority
  only when two surfaces need different ordering.
- Plugin nav paths remain links supplied by plugins. Static and nested routes
  resolve before plugin routes, so plugins cannot serve or shadow first-party
  pages, but link ownership is not yet validated.
- Route coverage continues to inspect the current route switch. Typed route IDs
  shared by routing and navigation should replace that source scan before path
  ownership validation is introduced.
- Jira, Linear, and Azure DevOps availability polling remains per mounted gated
  consumer. Deduplication belongs in shared integration state, not navigation.

## Consequences

- Adding a first-party destination has one catalog entry and testable mobile
  coverage rules, while mechanics can evolve without growing the catalog file.
- Plugin destination React keys are globally stable across owners without
  changing existing plugin test IDs.
- New navigation consumers must choose the gated or static hook deliberately.
- Future ordering, path ownership, and typed-route work has a discoverable
  decision boundary instead of relying on comments in one implementation file.

## Alternatives Considered

- **Keep one navigation module until it reaches the line limit.** Rejected
  because the catalog and its mechanics change for different reasons and were
  already approaching the frontend file-size limit.
- **Use only `NavItem.id` for plugin destinations.** Rejected because plugin IDs
  are not globally coordinated and resolved IDs are React keys.
- **Add priorities and path validation immediately.** Rejected because all
  surfaces currently agree on order, while reliable path ownership depends on a
  typed route catalog that does not yet exist.
- **Centralize integration polling in this change.** Rejected because polling is
  owned by integration domain hooks and is independent of manifest structure.
