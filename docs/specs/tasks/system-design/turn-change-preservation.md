---
status: draft
system: tasks
created: 2026-10-28
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
owners:
  - kandev
---

# Git turn history preservation

## Boundary and decision

Tasks owns accepted intervals, historical sources, and availability.
[Git turn history and unified compaction](../../../decisions/2026-10-28-git-turn-history-and-unified-compaction.md) replaces eager SQL rendering export.
System-page owns the [single compaction policy](../../system-page/system-design/history-compaction.md).
The [delivery package](../../../plans/turn-change-preservation-retry/plan.md) covers both independent contracts.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-TURN-CHANGES-002 | Capture and identity |
| REQ-TASKS-TURN-CHANGES-003 | Capture and identity; Durability and recovery |
| REQ-TASKS-TURN-CHANGES-004 | Content sources; On-demand reads; Compaction |
| REQ-TASKS-TURN-CHANGES-005 | Projection |
| REQ-TASKS-TURN-CHANGES-006 | On-demand reads; Projection |
| REQ-TASKS-TURN-CHANGES-007 | Bounds and evidence |

## Capture and identity

Retain fresh start/end commits for every admitted checkout and turn.
Keep write-once acceptance, checkout identity, policy attribution, and canonical raw counts.
Do not adopt upstream baseline reuse or authored-path filtering.
`CaptureTerminalEndpoints` accepts the original end before releasing the successor fence.
Git and SQL I/O remain outside lifecycle mutexes.
Close execution-overlap attribution at the accepted terminal boundary, independently from preservation progress.

## Content sources

Add an internal typed source locator to repository-change metadata.
Source kinds are legacy SQL, a verified common Git repository, and a backend-owned Git archive.
Locators carry installation ownership and immutable source identity. They are never browser-supplied paths.
Existing rows with no new source locator remain legacy SQL and retain their content links.
New captures store SQL identities, paths, counts, status, and source metadata without patch/rendering payloads.

For local worktrees, address the surviving common Git directory directly.
Do not make reads depend on the original worktree cwd or recreate a removed worktree.
Before deleting the repository holding the common directory, preserve required checkpoint objects in backend-owned Git storage.
Remote or disposable sources import the accepted commit/tree/blob closure into that storage before destruction.
Use one archive per durable repository identity, with distinct refs for checkout and turn identity.
Import missing objects and verify hashes, closure, refs, and ownership before acknowledging preservation.
Never use object alternates whose deletion would remove the archive's only source.

## On-demand reads

Resolve a server-owned file entry to its accepted source and endpoint pair after normal task authorization.
Generate canonical or whitespace-filtered patches from those exact objects.
Read old/new blobs from Git for historical context expansion.
Reuse the isolated attribute/config treatment from `turn_checkpoint_diff_view.go`.
Canonical counts remain independent from the whitespace selection.
Existing historical response DTOs can remain compatible through a source-aware content reader.
Legacy SQL reads continue through their existing integrity checks.

A temporary query timeout returns a retryable read error. It does not rewrite a verified snapshot as permanently unavailable.
Only missing or corrupt source evidence changes durable availability.
The read API holds a source lease that blocks conflicting expiry and object reclamation.
No new persistent rendered-patch cache is required.

## Durability and recovery

Comparison and object preservation run independently from the prompt fence.
Preservation stays pending during bounded transient retries against the accepted endpoint pair.
Successful repositories survive another repository's retry. Repeated imports do not double-count stored content.
Readiness requires a verified retained source and truthful summary completeness, not a materialized SQL patch.
A later rendering request has its own failure lifecycle.

Before ordinary source destruction, preserve the required objects or settle an explicit final failure within the remaining deadline.
Stop/Cancel retains its short settlement bound. Backend shutdown does not synthesize provider cancellation.
Restart can resume only accepted pairs through verified original sources or verified backend archives.
It never captures a replacement interval or provisions replacement compute to manufacture history.
Publication retries preserve source receipts and stable turn/file IDs.

## Compaction

The shared System-page policy determines history eligibility and expiry.
Tasks provides typed inventory, leases, source validation, and idempotent removal operations.
Preserve turn/file summaries, canonical counts, terminal anchors, and explicit expired state.
Expire SQL metadata before destructive source cleanup, with a durable receipt recording expected ref/object identity.
Delete only owned refs through compare-and-set. Interrupted cleanup retries from its receipt.
Shared blobs remain until every retained reference releases them.
User branches and unrelated refs are outside ownership.
Only backend-owned stores receive explicit object reclamation. Shared user repositories retain their normal maintenance.

The former fixed 30-day turn-content sweep must not compete with the shared policy after migration.
Existing expired history remains expired. Unexpired legacy content follows the shared policy instead of an obsolete fixed deadline.
Capacity limits bound new admission when cleanup is disabled. They do not silently delete protected history.
Task deletion retains its explicit lifecycle-owned cleanup semantics.

## Projection

Share terminal projection between REST summaries and `session.turn.changes.updated`.
During preservation, derive terminal identity from the stored claim without satisfying the internal finalization guard.
The existing Preparing changes state is independent from the agent-running indicator.
Ready and expired updates keep the same turn, file selection, anchor, and revision ordering.
Desktop keeps its docked viewer; phone keeps its full-height diff drawer and existing touch behavior.
Existing Retry repeats the historical read. It never substitutes live files.

## Bounds and evidence

Keep endpoint capture bounded by the parent design.
Proposed preservation bounds remain 90 seconds per attempt and three minutes from the stored terminal claim.
Use capped backoff and shared Git admission. Measure object-transfer and query costs before finalizing numeric defaults.
Keep bounded output, rendering response, file inventory, and content-admission limits.
The former SQL compressed-byte limits are not proof of equivalent Git physical storage costs.
Report actual store growth and deduplicated object accounting without claiming unmeasured disk savings.
Log stage, safe error class, duration, and retry status. IDs are structured fields, never metric labels.

## Migration and compatibility

Retain legacy SQL rendering until verified migration or shared-policy expiry.
Do not force-delete old payloads or enable history compaction during storage migration.
Unsupported compaction database engines retain their shipped compatible history path until the shared policy is supported there.
Do not advertise Git-history cleanup on an unsupported engine or remove its existing fallback.
Backups/restores preserve source bindings through the compaction backup contract.
The ignored upstream research report is not a tracked implementation dependency.
