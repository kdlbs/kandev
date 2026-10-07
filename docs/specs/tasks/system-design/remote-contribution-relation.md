---
status: current
system: tasks
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
---

# Remote Contribution Relation Evidence

## Boundary and requirement mapping

Tasks owns comparison of the selected checkout with its published contribution.
This refinement implements existing criteria 001.4 and 001.7 in
[remote contribution requirements](../requirements/remote-contribution-tasks.md).
The broader [contribution design](remote-contribution-tasks.md) retains branch
selection, source routing, confirmation, and backend lease ownership.
The [bounded timeline design](../../ui/system-design/bounded-changes-rendering.md)
owns row geometry independently of the comparison result.

The classifier implements this evidence gate for both desktop and phone.

## Evidence and classification

`useRemoteContributionRelation` selects repository/branch-scoped provider
evidence and ready Git status. `classifyRemoteContribution` consumes the
current provider head, complete authoritative provider commits, local head,
upstream head, and upstream-relative counts. Display-only retained provider
commits remain excluded from authorization.

The provider-history hook waits for a registered WebSocket client and a connected
transport before it starts a request. Connection startup does not consume the
provider retry budget. After reconnection, the hook loads unresolved history for
the selected source. Successful cached history keeps its existing source identity.


The provider API and local remote-tracking ref are independent snapshots.
Counts against one upstream head cannot establish ancestry against a different
provider head. Base-relative counts never resolve that mismatch.

Evaluate evidence in this order:

| Evidence | Relation and presentation |
| --- | --- |
| No selected contribution | Existing ordinary repository behavior |
| Required current provider/Git evidence unavailable | Unknown, unified |
| Local head equals current provider head | Aligned, unified |
| Complete current provider history contains the unequal local head | Provider ahead, unified; existing configured-upstream Pull policy |
| Upstream head equals current provider head; ahead > 0 and behind = 0 | Local ahead, unified |
| Upstream head equals current provider head; ahead > 0 and behind > 0 | Confirmed divergence, separate |
| All other combinations | Unknown, unified |

The final case includes stale/unrelated upstream heads, absent upstream heads,
and inconsistent zero or one-sided counts that do not independently prove the
provider/local relationship. Missing a local SHA from a PR commit list alone
does not prove both histories contain unique commits.

Unknown must use the existing `unavailable_evidence` action policy. It must not
enable replacement/restoration or ordinary remote mutation from fallback counts.
Keep local files, commits, and retained published provenance usable. Genuine
divergence keeps local-first version groups and existing explicit confirmations.
Do not compare patch content, change Git refs, fetch automatically, or broaden
the supported provider surface.

## Desktop and phone

Both compositions share the same classifier and action policy. Uncertain
evidence uses the existing unified Commits section; confirmed divergence uses
Local checkout commits and PR version sections. The existing phone Changes
navigation and touch-accessible Git action surface remain the entry points.

## Validation and decisions

Pure classifier tests cover the evidence table, including stale upstream with
positive counts and zero-count inconsistency. Hook tests cover ready/pending
status and selected repository scope. Desktop and phone browser regressions
prove the rendered groups and absence of destructive choices for uncertainty,
then prove that matching two-sided evidence still exposes comparison.

This correction follows the ancestry rule in
[head drift ADR](../../../decisions/2026-08-10-remote-contribution-head-drift.md)
and the unavailable-evidence rule in
[provider enrichment ADR](../../../decisions/2026-08-13-provider-history-changes-enrichment.md).
It introduces no schema, persistence, or new architectural boundary.

## Implementation plan

- [Changes sidebar history and spacing repair](../../../plans/changes-sidebar-history-spacing/plan.md)
