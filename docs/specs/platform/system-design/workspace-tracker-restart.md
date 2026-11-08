---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-11-08
updated: 2026-11-08
owners:
  - kandev
---

# Workspace tracker continuity after agent restart

## Ownership and requirement mapping

Platform owns workspace observation, tracker lifetime, and publication.
This design supplements [workspace Git status](workspace-git-status.md).
Agents retains managed-runtime retry eligibility, attempt limits, and prompt delivery.
Tasks retains environment bindings and [live-source eligibility](../../tasks/system-design/environment-owned-git-status.md).

The active requirement already defines this outcome. No new requirement or acceptance criterion is necessary.

| Acceptance criteria | Design boundary |
| --- | --- |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2`, `.19`, `.43` | Live tracker admission and existing stream delivery |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5`, `.31` | Tracker retirement and shutdown cleanup |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21`, `.27` | Source identity and stale-result rejection |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20`, `.29` | Recovery without another mutation on desktop and phone |

## Existing lifecycle and correction

`WorkspaceTracker.Stop` closes its stop channel, cancels its lifetime context, and drains owned work.
That tracker is terminal. Its `Start` method cannot restore the closed channels or canceled context.
`process.Manager.stop` stops root, repository, and cached subpath trackers, including after an early agent exit.
The process manager can later start another agent on the retained instance.

Before that start, the process manager must replace every stopped tracker that its current graph still selects.
A live tracker with unchanged scope remains reusable.
The stopped-state predicate uses the tracker lifetime, independently of the agent status or repository path.
Matching repository identity alone cannot authorize reuse.

`WorkspaceTracker` remains terminal after Stop. The correction does not reset its `sync.Once`, channels, context, or revision counters.
Full Stop cleanup remains intact, including cleanup before the stopped-agent status guard.

## Components and control flow

Add a focused manager helper in `manager_workspace_restart.go`.
Call it from `startWithGeneration` after previous-process cleanup and before the replacement process starts.
Both persistent and one-shot adapters pass through this common boundary.
This includes startup before the first successful agent process and repeated Stop/Start cycles.

The helper performs the following operation under existing lifecycle admission and graph serialization:

1. Read the current workspace, allowed source roots, tracker graph, and cached subpath trackers.
2. If all selected trackers are live, retain the graph and its source identities.
3. Otherwise, reconcile the graph with the existing graph builder and configuration path.
4. Exclude stopped root and repository trackers from reconciliation reuse.
5. Remove stopped cached subpath trackers. Later authorized reads create fresh lazy trackers through `GetWorkspaceTrackerFor`.
6. Publish the replacement root and repository set through the existing graph locks and update the process runner's tracker reference.
7. Attach existing workspace subscribers to replacement trackers and start their normal polling.

The operation uses `rescanMu`, `repoTrackersMu`, and `workspaceTrackersMu` for their existing responsibilities.
It must preserve the established lifecycle lock order.
Do not perform tracker Stop, Git commands, subscriber callbacks, or goroutine waits while holding graph publication locks.
Repeated starts must not create duplicate trackers, polling loops, or subscriber attachments.
Permanent teardown admission still wins over restart or lazy creation.

Graph reconciliation must preserve its current classification rules:

- A real Git root retains the valid empty repository name.
- A plain task root retains Files ownership without becoming a successful Git repository.
- Attached repositories and initialized submodules retain their repository names and comparison anchors.
- A live named tracker remains reusable when another selected tracker is stopped.

Keep graph renewal local to the process manager. No new retry API, backend poller, or frontend recovery owner is necessary.

## Configuration and subscriber continuity

Use `configureTracker` and the existing manager-owned configuration sources for each replacement.
Preserve the effective Git environment, base branches, comparison targets, allowed source roots, recovery exclusions, and diagnostic identity.
Preserve the last pushed polling mode, including paused mode. Initial required observations retain existing startup behavior.
Comparison preparation remains asynchronous and subject to existing cancellation and credential rules.

Existing subscriber channels remain open and receive replacement-tracker events without a WebSocket reconnection.
Publication, attachment, and retirement form one subscriber handoff under the manager subscriber lock.
Subscribe and unsubscribe hold that same lock through attachment or channel closure.
Retired trackers clear their actual subscription sets before Stop, including subscribers already removed from the manager registry.
This prevents a retired tracker from retaining a channel that a concurrent unsubscribe is allowed to close.
Renewal must also update process-output routing through `ProcessRunner.setWorkspaceTracker`.

Lazy trackers remain synchronous read resources. Renewal does not start polling for every cached subpath.
All existing path validation and registered-source authorization remain mandatory.

## Ordering, cancellation, and failure

Each new tracker receives a new opaque `tracker_id` and fresh publication revisions.
Do not copy a stopped tracker's accepted cache, jobs, index snapshots, or ordering counters into the replacement.
Existing frontend display continuity can retain prior data with its original freshness.
New live observations establish replacement state through the existing ordered HTTP and stream projections.

Existing [workspace callback leases](workspace-stream-continuity.md) retain execution and client validation across startup attempts.
They cannot substitute for a live agentctl tracker.
Delayed observations or enrichment from a retired tracker cannot publish as replacement state.
Caller cancellation ends that caller's wait. It cannot poison the replacement tracker or another waiter.

An inaccessible or invalid repository remains unavailable. Renewal cannot synthesize a clean snapshot or authorize persisted fallback.
At least one failed current repository keeps aggregate freshness unavailable.
Healthy repositories remain readable while a different repository fails.
The operation preserves working-tree bytes, index contents, refs, and task/session identity.

Final manager or instance teardown still cancels observations and enrichment, removes temporary indexes, and drains owned goroutines.
Passthrough-only instances retain their existing cleanup, even when no ordinary agent process started.

## Desktop and phone outcomes

Desktop retains the task Changes panel. Phone retains bottom navigation to Changes and the full-height diff surface.
After a successful startup retry, both surfaces receive live file membership and settled patch content.
An active panel recovers without reload, focus cycling, manual retry, or another file mutation.
No rendered layout, copy, navigation, touch target, localization, or wire format changes are necessary.

## Verification and delivery

Process tests exercise real disposable repositories through Stop and Start.
They cover persistent and one-shot starts, stop before first start, lazy subpaths, named repositories, and initialized submodules.
An unchanged live graph and a mixed live/stopped graph require separate regressions.
Race tests cover concurrent graph reads, subscriber removal, old-worker completion, and final teardown.

API coverage calls both Git-status routes after restart and observes complete membership followed by ready details.
Browser coverage uses the existing deterministic managed-runtime failure fixture.
It crosses the real failed-start Stop and replacement Start, then checks streamed files and the rendered diff.
Synthetic status responses and direct store updates cannot prove this lifecycle correction.

The [repair plan](../../../plans/workspace-tracker-restart/plan.md) defines exact tests, commands, and work orders.
The existing [progressive publication decision](../../../decisions/2026-09-30-progressive-workspace-git-refresh.md) remains authoritative.
This repair restores terminal-tracker semantics within the existing ownership boundary. It does not require a new ADR.

## Exclusions

- Provider-specific recovery eligibility, runtime versions, npm cache repair, retry budgets, and prompt replay.
- Changes to Git admission, command deadlines, foreground refresh budgets, or recovery backoff.
- Database migrations, feature flags, new metrics, and complete diff persistence.
- Rebinding a task to another workspace or reclaiming executor resources.
