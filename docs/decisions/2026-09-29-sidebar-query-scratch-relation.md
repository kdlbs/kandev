# ADR-2026-09-29-sidebar-query-scratch-relation: Bound SQLite sidebar preparation

**Status:** proposed
**Date:** 2026-09-29
**Area:** backend

## Context

The sidebar evaluates complete views before returning bounded pages.
Last activity normalization expands through the recursive SQLite query during statement preparation.
An empty-database reproduction allocated 1.32 GB inside SQLite and retained about 1.6 GB RSS afterward.
The Go heap profile did not include these native allocations.

## Decision

Use a connection-local temporary relation between candidate filtering and recursive page evaluation on SQLite.
Keep all stages on one pinned reader connection and in one consistent read transaction.
The main database remains read-only. Only the SQLite temporary schema receives writes. Nonempty pin/manual-order lists share this lifetime in an indexed preference relation, populated with one JSON parameter; this prevents maximum legal saved preferences from expanding the page statement beyond SQLite parameter and preparation budgets.
Destroy the relation before returning the connection; discard connections whose cleanup cannot be established.
Keep PostgreSQL on its existing query path.

The sidebar remains server-authoritative and preserves complete-tree ranking before page selection.
Keep timestamp precision, saved views, response limits, and rich-row hydration unchanged.
Native-memory tests become required evidence for changes to this preparation boundary.

## Consequences

The database still evaluates all matching candidates, but recursive preparation references stored scalar columns.
An isolated schema-only prototype reduced native peak allocation to about 22 MB and preparation to 64 ms.
This prototype does not prove populated correctness, pooled cleanup, or concurrent behavior.
Implementation must establish those properties before delivery.

Temporary storage introduces cleanup ownership and possible disk cost for large workspaces.
No persistent schema, operator setting, custom SQLite driver, or browser cache is introduced.
The existing resource benchmark must measure execution as well as preparation.

## Alternatives considered

- A materialization hint alone: the measured `candidate_raw` experiment retained the allocation spike.
- A simpler timestamp expression: reduces cost but can lose strict validation, timezone handling, or nanosecond ordering.
- A custom SQLite scalar function: introduces registration on every connection and leaves the recursive expansion boundary implicit.
- Periodic allocator trimming: releases freed blocks but leaves transient peaks and expensive preparation.
- Caching prepared statements or page results: adds retention and invalidation without correcting cold-request cost.
- Client-side sorting: violates bounded transfer and complete-view evaluation contracts.

## References

- [Pagination requirements](../specs/ui/requirements/sidebar-task-pagination.md)
- [Browsing design](../specs/ui/system-design/sidebar-archived-filter.md#bounded-sqlite-query-preparation)
- [Fix package](../plans/sidebar-query-memory/plan.md)
