# ADR-2026-09-28-sidebar-view-page-reuse: Reuse bounded sidebar first pages

**Status:** accepted
**Date:** 2026-09-28
**Area:** frontend, protocol

## Context

Server-side sidebar pagination preserves complete filter and tree-order semantics
without downloading the workspace. Retaining only the current response makes
every view switch wait for another server read, including a return to a view
already visited. The user requested responsive switching and background refresh
while preserving bounded browsing.

## Decision

Revise only the client-retention portion of
[bounded sidebar queries](2026-09-26-bounded-archived-sidebar-queries.md).
Keep server-authoritative filtering/order, 100-row pages, and bounded hydration.
Reuse at most five recently successful first pages in the active store/workspace,
subject to a 2 MiB serialized-response budget and five-minute fetch-age expiry.
Later pages are display-only. A cache hit is visible before background refresh;
an unvisited, expired, or invalidated view loads normally.

Complete query identity, context generation, and query revision determine reuse.
Clear reusable results on membership/order invalidation, workspace/account change,
access denial, or store disposal. Do not persist or prefetch. The current page
continues its existing event reconciliation and bounded refresh behavior.

## Consequences

Return switching no longer depends on network latency when a snapshot is eligible.
Client retention has a fixed ceiling and does not grow with archive size or paging.
Initial visits and event-invalidated views still require server reads. Heavy event
churn can reduce cache hits. Query latency must be measured separately from cache
hit latency; neither a warm cache nor the old fixture benchmark proves live speed.

The previous single-current-page cache statement is superseded by this bounded
reuse rule when implemented. Other portions of the earlier decision remain intact.
No task/session lifecycle, settings schema, or database migration changes.

## Alternatives considered

- Keep only the current page: smallest retention, but repeats the reported delay.
- Restore all-task client filtering: fast local switching at unbounded transfer and memory cost.
- Prefetch every saved view: extra database load and requests without evidence of use.
- Retain every visited page: grows with browsing history and archive size.
- Reuse snapshots across unknown membership changes: faster during churn, but can
  restore tasks known to be removed or ineligible without a richer invalidation model.

## References

- [Pagination requirements](../specs/ui/requirements/sidebar-task-pagination.md)
- [Browsing design](../specs/ui/system-design/sidebar-archived-filter.md)
- [Repair plan](../plans/sidebar-view-loading-repair/plan.md)
