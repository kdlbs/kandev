---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001
created: 2026-08-09
updated: 2026-08-09
owners:
  - nova28
---
# Automations — "Pull request merged" trigger System Design Part 6

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Persistence guarantees

- The trigger definition and its config are ordinary `automation_triggers` rows and
  survive restart.
- Dedup memory lives in `automation_runs.dedup_key` and survives restart. There is no
  in-memory dedup cache.
- The subscription itself is process-local and is re-established on start-up.
- A merge that happens while Kandev is **down** is detected on the first poll after
  start-up: the row still reads `open`, the sync writes `merged`, the change publishes,
  and the trigger fires. **This guarantee holds only because the orchestrator event
  subscription and then the automation components start before the GitHub poller**, which
  [Bus subscription](#start-up-ordering-is-part-of-this-contract-not-an-implementation-detail)
  requires. It is stated as a dependency rather than left implicit because the dependency is
  not obvious and the failure is silent: `prMonitorLoop` runs its first `checkPRWatches`
  immediately on start, so under the opposite ordering that sweep publishes the merged-state
  event into a bus with no automation subscriber attached, the event is dropped, and — since
  the row is now persisted as merged — the PR may never publish a qualifying event again.
  The overnight-merge case is the single most likely real-world path into this feature, so
  this ordering is load-bearing rather than incidental. The end-to-end down-time-recovery
  scenario is the required observable; an extracted start helper may support that test, but
  a source-line ordering assertion may not substitute for the behavior.
- A merge that was already **persisted** as merged before the outage is a different case,
  and the honest answer is that it **may or may not** fire. It will not be republished by
  `reconcileTaskPRLifecycle` — that path publishes only on `state` / `merged_at` /
  `closed_at` changes, and in any case
  [never runs on a merged row](#which-publish-paths-touch-a-merged-row).
  It **will** republish from the watch-driven `SyncTaskPR` whenever any of that path's
  sixteen fields next changes, which for a PR still attracting review or check activity is
  likely; and the association and restore paths publish unconditionally, so re-linking or
  restoring that PR fires the trigger once. For a merged PR that nobody touches again and
  whose watch has been removed, nothing further publishes and it never fires. See
  [Retroactivity](#retroactivity-and-first-observation-semantics).
- Run tasks and their worktrees follow the standard automation-run retention: the ten
  most recent terminal runs per automation keep their checkout, older ones are reclaimed.
