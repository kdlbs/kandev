# ADR-2026-10-28-git-turn-history-and-unified-compaction: Git turn history and unified compaction

**Status:** accepted
**Date:** 2026-10-28
**Area:** backend, frontend

## Context

Turn history currently copies generated patches and old/new rendering content into SQL.
An export failure can leave valid Git endpoints with unavailable SQL content.
The user selected Git snapshots, SQL metadata, and on-demand diffs after reviewing t3code.
The user also selected one compaction policy for old tool details and historical diffs.
Separate history cleanup settings would add an unnecessary operator decision.

## Decision

Keep fresh immutable start/end checkpoints for each admitted checkout and turn.
Use SQL for identities, counts, paths, source ownership, and availability.
Generate historical patches and context from accepted Git objects when requested.
Local worktrees use their surviving common Git repository.
Remote or disposable sources transfer required objects to backend-owned Git storage before source destruction.
Historical reads resolve the object store directly, independently from a removed worktree path.
Existing SQL-backed history remains readable until migration or authorized expiry.

Expand the existing message-compaction policy into history compaction.
Use one enable switch, inactivity period, Analyze savings action, manual action, and automatic schedule.
The same eligible task cohort covers tool bodies, legacy SQL diff payloads, and retained checkpoint content.
Candidate discovery includes history-only tasks with no removable tool messages.
There is no separate history-retention policy or cleanup toggle.

Preserve conversation prose, message metadata, task/session records, plans, attachments, and turn/file summaries.
Expire removable history explicitly before deleting owned content and refs.
Protect active work, pending capture or preservation, backups, and historical-read leases at every mutation boundary.
Delete only owned checkpoint refs with their expected object IDs.
Reclaim Git objects only in Kandev-owned stores. Shared user repositories keep their normal Git maintenance.

Reuse existing maintenance coordination and bounded job patterns.
Remove the competing fixed-age turn-content sweeper when the unified policy owns that installation's history.
Capacity admission remains bounded when compaction is disabled. It does not silently expire protected history.
Existing database-engine capability limits remain explicit during migration.

The existing preparation flow covers the complete cleanup scope.
A selected backup includes SQL state and the Git objects required to restore affected history under one verified receipt.
Old tool-only approvals cannot silently authorize the expanded destructive scope.
Reuse the existing preparation choice once, without a second policy or toggle.
Database file compaction remains the existing physical-maintenance operation.

## Consequences

Temporary patch-read errors no longer permanently invalidate captured history.
SQL growth from new patch/rendering payloads stops after migration.
Git storage still requires bounded ownership, retention, and cleanup.
One policy keeps the operator workflow small while implementation reports each cleanup result accurately.
SQL reuse, removed Git refs, and measured disk reclamation remain different result fields.
The previous SQL-export retry package requires a complete Git-storage and shared-compaction revision before implementation.

## Alternatives Considered

- A separate historical-diff policy was rejected by the user because one compaction flow is simpler.
- Keeping SQL patches as the new default duplicates materialized rendering and retains the failing export dependency.
- Keeping snapshots forever would retain unreachable user work indefinitely through private refs.
- Running broad Git pruning in user repositories exceeds checkpoint ownership.
- A database-only backup cannot recover Git objects removed by the expanded operation.

## Related artifacts

- [Immutable interval decision](2026-10-07-immutable-turn-change-intervals.md)
- [Preservation decision replaced by this direction](2026-10-28-turn-change-preservation-retries.md)
- [Turn history requirements](../specs/tasks/requirements/turn-changed-files.md)
- [Turn history design](../specs/tasks/system-design/turn-change-preservation.md)
- [Compaction requirements](../specs/system-page/requirements/tool-payload-retention.md)
- [Compaction design](../specs/system-page/system-design/history-compaction.md)
- [Delivery package](../plans/turn-change-preservation-retry/plan.md)
- [Pinned t3code Git capture](https://github.com/pingdotgg/t3code/blob/9e09926d97824179d97311d1a810c6b5bb60d2d1/apps/server/src/vcs/GitVcsDriver.ts#L1115-L1163)
- [Pinned t3code diff query](https://github.com/pingdotgg/t3code/blob/9e09926d97824179d97311d1a810c6b5bb60d2d1/apps/server/src/checkpointing/CheckpointDiffQuery.ts#L205-L253)
