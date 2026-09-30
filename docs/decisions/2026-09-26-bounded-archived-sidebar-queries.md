# ADR-2026-09-26-bounded-archived-sidebar-queries: Bound sidebar task queries

**Status:** proposed
**Date:** 2026-09-26
**Area:** backend, frontend, protocol

## Context

The sidebar loads every archived task through successive 100-task requests.
Its saved-view engine then filters, sorts, groups, and orders complete task trees.
Stopping that loop after one page gives incorrect global view results.
A descendant on a later page can change the position of its parent tree.

## Decision

The proposed repair uses an authorized, read-only sidebar task query.
It evaluates the complete view before returning one bounded display page.
UI owns these presentation semantics. Task repositories own authorized data reads.
Every sidebar view uses this query, regardless of archive status or list size.

Use Previous and Next pages with at most 100 task rows. Group headers do not consume task slots.
Hide the controls when at most 100 task rows remain after filters and collapse visibility.
Keep only the current page in the sidebar page cache. A small view uses the same query path.
The query returns ordered entries and boundary context, not a partial task collection
that the client must filter or regroup.

Use relational filtering, tree aggregation, and page selection before rich row hydration.
Do not transfer all sidebar candidates to Go or JavaScript for global sorting.
Reuse stored task summaries. Do not reconstruct transcripts during a sidebar read.

The query has no retained server snapshot. Each response uses one consistent database read.
Changes between page reads can move entries. Live invalidation and foreground refresh
converge the current page; exact historical enumeration is not this browsing contract.

## Consequences

Browser payloads, row construction, and sidebar page cache size remain bounded.
The backend must implement the existing view semantics with conformance fixtures.
Database ranking can still inspect many matching rows. Query plans and a large fixture
must demonstrate acceptable execution cost before delivery.

Page controls replace continuous scrolling in large sidebar views. A page can start inside a tree,
so the response must carry parent and group context without inventing a new root.
Saved preference shapes and active-task behavior remain compatible.
Sidebar title and text-group ordering use the existing task repository ordering policy.
This removes browser-locale variation across pages, but case and accented-title ties can move.
All sidebar views use the same collation on both sides of the pagination threshold. This is an explicit compatibility
tradeoff in the proposed package, not an unrecorded implementation fallback.

## Alternatives considered

- Stop the existing loop after 100 tasks: filters and tree ranking become incomplete.
- Load more and retain every page: browser cost still grows without a fixed bound.
- Virtualize the complete archive: DOM cost falls, but transfer and client derivation remain unbounded.
- Fetch lightweight keys for every task: global client work and transfer still grow with the archive.
- Persist query snapshots: adds retention and invalidation machinery for a browsing flow that permits live movement.

## References

- [Pagination requirements](../specs/ui/requirements/sidebar-task-pagination.md)
- [Archived navigation requirements](../specs/ui/requirements/sidebar-archived-filter.md)
- [System design](../specs/ui/system-design/sidebar-archived-filter.md)
- [Activity identity](2026-08-17-separate-task-activity-from-summary-freshness.md)

## Scope correction

The [2026-09-28 view reuse proposal](2026-09-28-sidebar-view-page-reuse.md)
revises only the single-page client-retention rule to a bounded first-page cache.
Server-side view evaluation and size-based paging remain unchanged.

The user explicitly required size-based pagination for all sidebar views.
Archive status determines membership, never whether pagination is available.
The archived conversation navigation repair remains a separate outcome in this package.
