---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-11-05
owners:
  - kandev
---

# Git repository scope lifecycle

## Ownership and mapping

Platform owns Git observation and refresh health.
Tasks retains environment binding and source attachment.
This design extends [workspace Git status](workspace-git-status.md) for inventory transitions.
It preserves [environment ownership](../../tasks/system-design/environment-owned-git-status.md) and existing delayed recovery.

| Criteria | Design sections |
| --- | --- |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25, 001.26 | Tracker eligibility, Inventory reconciliation |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27 | Ordering and invalidation |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.39, 001.40, 001.41 | Recovery and presentation |

The [delivery package](../../../plans/live-repository-attachment/plan.md) covers issue #5071.
The related [Tasks design](../../tasks/system-design/chat-file-link-context.md) owns relative file links.

## Tracker eligibility

`Manager.RepositoryScopes()` defines the registered Git scopes.
The empty name is valid exactly when the root tracker owns a Git repository.
Named trackers can coexist with that valid root, including nested repositories and submodules.
The presence of any named tracker does not prove that the empty scope is obsolete.

The plain parent tracker continues to own Files events and poll-mode propagation.
`SetWorkspacePollMode` and `refreshTrackersDetached` must refresh Git only for Git-eligible trackers.
Use the same root eligibility as stream-attachment replay and `RepositoryScopes`.
Apply the rule to foreground mode changes, base-branch changes, rescans, and lazy tracker paths.
Do not globally convert real Git failures into ready empty snapshots.

`captureBasicGitStatus` retains its unavailable result for an invalid explicit Git query.
This package does not change repositoryless initial-state semantics or explicit invalid-repository requests.

## Inventory reconciliation

A single live status event describes one repository. It does not describe the complete inventory.
Use a current correlated `session.git.refresh` response for complete inventory evidence.
The backend multi-status read already enumerates every registered scope, including failed scopes.
Keep its chosen source's snapshots together. Do not concatenate unsuccessful sibling-source attempts with a later successful source.

A successful aggregate with a validated complete snapshot set can establish inventory even when one repository fails.
Every failed slot must retain its repository identity.
Malformed, truncated, mixed-source, mismatched, or unsuccessful responses cannot prove scope removal.
An empty response cannot clear prior repository failures.
No wire-format extension is necessary for this repair.

Maintain a runtime inventory record per environment with a local generation and nullable known scope list.
An attachment invalidates this record and advances its generation once.
It preserves healthy named snapshots while the next inventory read runs.
Use the existing store, refresh coordinator, and `useSessionGitRefresh` owner.
Do not introduce another poller or timer owner.

After an accepted inventory read, reconcile all associated maps in one store update:

- Keep snapshots and display companions for every current scope.
- Remove retired scopes from raw status, display status, and repository refresh maps.
- Remove the legacy mirror only when its referenced scope retires.
- Clear an obsolete environment refresh failure only after the current aggregate read succeeds.
- Apply current failed slots and retain their unavailable state.

`clearLegacyGitStatusEntry` must remove the empty scope's refresh state as well as its snapshots when retirement is confirmed.
Its callers must not retire a valid Git root after a root-preserving attachment.
Scope discovery alone does not authorize clearing every environment cache or invalidating unchanged checkout content.

## Ordering and invalidation

Materialization and workspace-source events invalidate inventory for their mapped environment.
An active Changes owner requests the replacement inventory without requiring a panel remount.
Sessions that share an environment share this generation and refresh work.

Capture inventory generation with every fresh, recover, and replay request.
Reject completion after generation, workspace binding, environment, session, or connection replacement.
Cancellation and timeout bookkeeping must obey the same fence.
Existing tracker identities and revisions still order each current repository's snapshots.

Once inventory excludes the empty scope, late unnamed status or unavailable events cannot recreate it.
An inventory invalidation permits a new authoritative read to establish a valid root again.
Named streaming success alone cannot clear an unrelated current failure.
Reconnect performs the same correlated inventory reconciliation as activation.

## Recovery and presentation

Unavailable health is existential: at least one failed current scope keeps the warning visible.
Healthy health requires all current scopes to recover.
Retired scopes do not participate in either predicate.
Include a mixed-state regression with one ready named repository and one failed named repository.
Also include a valid unnamed Git root with a named child.

The existing recovery coordinator keeps its bounded backoff and shared request ownership.
Accepted recovery cancels unnecessary retries. Same-state renders do not restart the schedule.
Retained healthy files and patches remain readable under the existing freshness rules.

`deriveChangesPanelGitStatus` derives failures from reconciled state.
The warning disappears after all current scopes recover.
A valid unnamed-root failure keeps the generic localized unavailable notice without an empty repository sentence.
Named failures keep their actual names.
Desktop toolbar and phone Changes use the same derivation and existing touch disclosure.

## Persistence and observability

Inventory reconciliation is runtime-only. It adds no database table or snapshot migration.
Existing accepted-observation persistence and historical snapshots retain their current ownership.
Reuse bounded debug logging for accepted and rejected scope changes.
Do not add repository paths, task IDs, or session IDs as metric labels.

## Verification

Agentctl tests exercise real plain-parent and Git-root tracker graphs through poll-mode and base-branch refresh paths.
Frontend tests cover cleanup, current inventory, mixed failure, late delivery, hydration, reconnect, and shared sessions.
The live browser scenario keeps Changes mounted during legacy attachment.
It observes real named snapshots and the warning state before and after reconnect.
Exact commands and test names belong to the linked work orders.
