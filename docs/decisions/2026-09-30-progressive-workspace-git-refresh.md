# ADR-2026-09-30-progressive-workspace-git-refresh: Tracker-owned progressive Git refresh

**Status:** accepted
**Date:** 2026-09-30
**Area:** protocol, backend, frontend

## Context

The caller deadline and tracker lifetime are separate. A fresh read can time out while its shared observation later succeeds.
The existing contract gives fresh reads no publication ownership. Slow diff enrichment also delays the entire file list.
An unchanged dirty worktree then has no guaranteed event that repairs a missed browser snapshot.

## Decision

The tracker owns ordered publication of accepted basic and enriched observations, independently of caller lifetime.
Basic membership is complete before diff work starts. Enrichment cannot alter that membership.
One bounded enrichment worker uses a latest-only pending slot and validates repository state before publication.

Foreground refresh has a correlated snapshot response and bounded recovery through the existing refresh action.
Notification streams remain bounded and non-blocking. Their cache provides replay instead of guaranteed delivery of every frame.
Environment and repository identity govern every snapshot. A failed live source cannot authorize a stale persisted replacement.

See the [workspace status design](../specs/platform/system-design/workspace-git-status.md) and [plan](../plans/changes-panel-git-refresh/plan.md).

## Consequences

- Useful work can repair the cache and connected subscribers after a caller timeout.
- File rows arrive independently of per-file diff latency.
- Publication needs an epoch, revision, and validated observation fingerprint.
- Consumers distinguish complete membership from pending details and compact summaries.
- The earlier fresh-read non-publication contract changes explicitly. Existing isolation and resource limits remain required.
- Background enrichment can be delayed under admission pressure. The UI must identify that state and retain bounded recovery after failure.
  The [follow-up recovery design](../specs/platform/system-design/changes-refresh-recovery.md) supplies delayed read retry without changing publication ownership.

## Alternatives Considered

1. Increase the original timeout. This postpones failure and still gives completed detached work no publication path.
2. Batch all diffs. This can reduce subprocess cost, but still couples file visibility to enrichment and leaves the recovery defect.
3. Cache every completed singleflight result. This permits out-of-order replacement and unrelated enrichment unless publication is fenced.
4. Fetch diffs only after selection. This adds a separate diff request/cache lifecycle and changes Review aggregation beyond the repair scope.
5. Poll every task more often. This increases background load and does not guarantee a snapshot survives delivery loss.
6. Restore a compact database row after live failure. Missing file details cannot establish clean membership or authoritative live status.
