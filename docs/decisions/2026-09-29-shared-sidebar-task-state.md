# ADR-2026-09-29-shared-sidebar-task-state: Reuse normalized task overviews

**Status:** proposed
**Date:** 2026-09-29
**Area:** frontend, protocol

## Context

Homepage snapshots already contain active task data, but sidebar mounting requires a separate server query.
An invalidation during that query currently discards its response, which can prolong initial blank rendering.
The user requested shared normalized Zustand state and on-demand fetching for tasks outside resident coverage, including archives.

## Decision

Use one canonical overview entity per task, with separate homepage membership and sidebar page metadata.
Complete, current, semantically compatible resident data supplies sidebar pages immediately.
Uncovered views use bounded server queries and merge their records into the same store.
Keep Zustand. Do not introduce Redux or a second independently mutable task cache.

Add explicit snapshot coverage and ordering-profile metadata. Cached IDs alone do not prove completeness.
SQLite local ordering must match the server. Other database profiles remain server-evaluated until parity is established.
Keep hard identity/access barriers, but treat ordinary same-view invalidation as a background refresh request.
Reconcile newer entities and removals before displaying a provisional successful page.
Do not wait indefinitely for an event-free request window.

Preserve bounded page retention and count attributable entity bytes after normalization.
Keep optimized server evaluation for cold archives, partial datasets, and unsupported ordering profiles.

## Consequences

Previously loaded complete homepage data can render in the sidebar without duplicate fetches.
Normalization requires routing overview writes through shared actions and converting consumers to selectors.
Coverage and freshness require explicit state; reconnects and unknown mutations invalidate completeness.
The correction does not establish the cause of the reported browser renderer crash.

## Alternatives considered

- Retain unconditional server reads: duplicates available work and exposes initial loading to server latency.
- Restore unqualified concatenation of snapshots: can treat truncated or missing workflow data as complete.
- Fetch every missing archive page: recreates unbounded transfer and retention.
- Keep separate entity caches: permits homepage/sidebar disagreement and duplicated memory.
- Accept every late response: can restore deleted tasks or data from a previous account.

## Superseded rules

This proposal revises the unconditional server-query rule in
[bounded sidebar queries](2026-09-26-bounded-archived-sidebar-queries.md).
It also revises blanket revision-mismatch rejection in the implementation of
[bounded page reuse](2026-09-28-sidebar-view-page-reuse.md).
The 100-row display bound, complete-view semantics, hard context barriers, and finite reuse budgets remain.

## References

- [Requirements](../specs/ui/requirements/sidebar-task-pagination.md)
- [Shared-state design](../specs/ui/system-design/sidebar-shared-task-state.md)
- [Package](../plans/sidebar-query-memory/plan.md)
