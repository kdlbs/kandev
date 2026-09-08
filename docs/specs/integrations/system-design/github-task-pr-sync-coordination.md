---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001
created: 2026-08-27
owners:
  - kandev
---

# GitHub task pull request sync coordination System Design

## Purpose and boundaries

This design gives `github.task_pr.sync` one frontend lifecycle owner for each
application store, workspace context, and task. It removes duplicate requests
from concurrently mounted `useTaskPR` consumers without changing the WebSocket
contract, pull request store, retry policy, unlink operation, or rendered UI.

The integration system owns this design because it coordinates provider
synchronization. The UI remains a consumer of the existing hook result.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001` | [Resource identity and ownership](#resource-identity-and-ownership), [Request and retry lifecycle](#request-and-retry-lifecycle), [Hook integration](#hook-integration), [Failure and recovery](#failure-and-recovery) |

## Current failure mode

`useTaskPR` is mounted by the PR top bar, status chip, detail panel, Dockview
headers and watermark, review-panel synchronization, task shortcut, and
external-file-link detection. Every hook instance currently owns independent
request, retry, permanent, reconnect, and loaded state.

Mounting several of these consumers for one task therefore sends several
`github.task_pr.sync` freshness requests. Empty results create several
five-second retry intervals, and reconnect transitions can restart all of them.
The requests are semantically identical.

An in-flight promise map alone is insufficient. It can collapse calls that
overlap in time, but independent intervals can issue new requests immediately
after the shared promise settles. The shared abstraction must own the complete
lifecycle.

## Resource identity and ownership

Add `apps/web/hooks/domains/github/task-pr-sync-resource.ts`. The module exposes
a testable resource factory and an application resource registry.

The registry is a `WeakMap<StoreApi<AppState>, ...>` so separate state providers
cannot share requests. Each store registry keys entries by:

```text
workspaceId + workspaceContextGeneration + taskId
```

The workspace-context generation prevents a response from an earlier load of
the same workspace ID from joining or updating the current context.

Each entry owns:

- the immutable task and workspace scope;
- the current `loaded` snapshot and snapshot listeners;
- the active request promise;
- retry count, permanent status, and one retry timer;
- the previous connection status and one store connection observer;
- a consumer lease count and a generation used to reject stale completion;
- cleanup state for an orphaned in-flight request.

The task pull request Zustand slice remains the source of truth for associated
pull requests. The resource stores only synchronization lifecycle state.

## Request and retry lifecycle

The first consumer lease for an entry starts one freshness request. Additional
leases subscribe to the same entry and do not start another request.

`refresh` returns the active promise when a request is already running. When no
request is active, it sends `github.task_pr.sync` with the scoped task ID. This
rule applies to automatic and manual refreshes.

Response handling preserves the current hook policy:

- A successful response marks the scoped task as loaded.
- A non-empty response writes each association through `setTaskPR` with the
  entry's workspace scope, resets the retry count, and stops retry scheduling.
- An empty response leaves pull request detection eligible for the bounded
  retry schedule.
- A permanent response stops the retry schedule.
- A transient request failure keeps loading pending while retry attempts
  remain. The final failed retry marks the task as loaded.
- One retry is attempted every five seconds, up to six attempts after the
  initial freshness request.

The entry observes scoped task pull request state in the application store. A
pull request added through any path cancels the retry timer. If the scoped
association later becomes empty while a consumer remains, the entry can resume
the remaining retry lifecycle as the current hook does.

The last consumer release cancels the retry timer, removes the connection and
task-state observer, and invalidates the entry generation. An unresolved
request can settle for its callers, but it cannot publish lifecycle state or
write pull requests after the entry becomes stale. The entry is removed after
settlement so a later mount performs a new freshness check.

## Connection recovery

The entry, rather than each hook, detects the transition edge into
`connected`. That edge clears the retry and permanent state, starts one
immediate refresh, and starts one fresh bounded retry window if the result is
empty.

Repeated writes of `connected` do nothing. Different scoped entries recover
independently.

## Hook integration

Refactor `apps/web/hooks/domains/github/use-task-pr.ts` to use
`useAppStoreApi` and the shared task PR sync resource.

`useSyncExternalStore` binds each hook instance to the resource's stable
`loaded` snapshot. Its subscription is also the consumer lease: the first
subscriber activates the entry and the last subscriber releases it. Strict
Mode unsubscribe and resubscribe cycles must rejoin an active request instead
of starting a duplicate request.

The hook continues to derive `pr` and `prs` from the scoped Zustand slice. It
continues to expose:

- `refresh`, delegated to the scoped resource;
- `unlink`, retaining the current workspace and generation guards;
- `loaded`, true when either the resource has settled the scoped load or the
  scoped store already contains an association.

After a successful unlink, the hook invalidates the scoped resource generation
after removing the local association. A response already in flight can settle
for its caller, but it cannot restore the removed association.

Call sites do not change. All existing surfaces benefit from coordination
through the hook boundary.

## Failure and recovery

- If the WebSocket client is unavailable, the resource does not invent a
  successful result. Existing reconnect recovery remains the path to a fresh
  window.
- A request rejection is absorbed by `refresh`, matching the existing
  non-rejecting hook contract.
- Before writing a response, the resource verifies both its entry generation
  and `isCurrentWorkspaceContext` against the current store state.
- A task or workspace switch releases the old entry and acquires a distinct
  entry. The old response cannot alter the new entry's loaded or permanent
  state.
- The application registry is store-scoped and entries are released after the
  last consumer, preventing cross-provider leaks and unbounded settled state.

## Test design

Add resource-level tests for request joining, retry ownership, manual refresh,
consumer leases, reconnect edges, permanent responses, terminal failures, and
stale-context rejection. Use injected request and timer options so tests remain
deterministic.

Extend `use-task-pr.test.tsx` with a multi-consumer regression that renders two
`useTaskPR` instances under one `StateProvider`. Assert one initial request, one
request per retry tick, shared loaded state, continued retries after one
consumer leaves, and cleanup after the last consumer leaves. Keep existing
unlink and reconnect tests green.

Separate task and workspace-context tests prove that keys do not over-deduplicate.

No new desktop or mobile E2E test is required because the hook return contract
and all rendered interactions remain unchanged.

## Related designs

- [GitHub task pull request sync coordination requirements](../requirements/github-task-pr-sync-coordination.md)
- [PR task status summary requirements](../../ui/requirements/pr-task-status-summary.md)
- `apps/web/hooks/domains/github/pr-commits-resource.ts`, the existing shared
  external-resource pattern for GitHub PR commit requests
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.ts`, the existing
  store-scoped, workspace-generation-aware request registry pattern
