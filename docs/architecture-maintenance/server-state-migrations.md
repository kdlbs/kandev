# Server-state migrations

[Roadmap](README.md) · Inventory at main `359b5ffdbb6`, 2026-09-27.

The target is one owner for each server resource, not a complete replacement of Zustand.
TanStack Query owns finite snapshots after migration. Zustand retains UI state and unmigrated resources.
WebSocket remains the transport for live events and streams.

## Platform / System inventory

| ID       | Resource               | Current owner and evidence                                                                                                                                                                                                                                                                    | Status                                                          | Completion boundary                                                                                                                                    |
| -------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| QUERY-01 | About SystemInfo       | Query through [use-system-info.ts](../../apps/web/hooks/domains/system/use-system-info.ts)                                                                                                                                                                                                    | Done, [#3977](https://github.com/kdlbs/kandev/pull/3977)        | Snapshot and request lifecycle removed from Zustand. Separate process-control probes remain                                                            |
| QUERY-02 | Database statistics    | TanStack Query via [useDatabaseStats](../../apps/web/hooks/domains/system/use-database-stats.ts); used by DatabaseStatsCard and Backups description in [data-logs-settings.tsx](../../apps/web/components/settings/system/data-logs-settings.tsx); Zustand database state and setter removed. | In progress                                                     | Both consumers share one Query observer; preserve the resolved backup-directory description and wait for PR #4225 to merge before starting QUERY-03.   |
| QUERY-03 | Backup list            | TanStack Query via [use-backups.ts](../../apps/web/hooks/domains/system/use-backups.ts); backup-only Zustand state and actions removed.                                                                                                                                                       | In progress, [#4271](https://github.com/kdlbs/kandev/pull/4271) | One authoritative list with targeted create, delete, reset, and retention invalidation; preserve reload return/error behavior and existing permissions |
| QUERY-04 | Disk usage             | Zustand snapshot, job observation, and polling in [use-disk-usage.ts](../../apps/web/hooks/domains/system/use-disk-usage.ts)                                                                                                                                                                  | In progress, [#4291](https://github.com/kdlbs/kandev/pull/4291) | Query owns the snapshot. Refresh, terminal job events, and missed-event recovery remain correct                                                        |
| QUERY-05 | Other System resources | Outside this bounded inventory                                                                                                                                                                                                                                                                | Deferred                                                        | Inventory jobs, retention, and maintenance independently before selecting another resource                                                             |

### QUERY-02: database statistics

Assignee: [Carlos Florêncio](https://github.com/carlosflorencio). Work order: [Task 01](../plans/database-statistics-query-migration/task-01-database-statistics-query.md). PR: [#4225](https://github.com/kdlbs/kandev/pull/4225). Kandev task ID: `95ad66df-588d-48d1-afee-5c674c928013`.

This resource proves mutable data behavior. SystemInfo's infinite freshness is not a reusable default.
The reviewed design covers the two consumers: DatabaseStatsCard and the Backups description in DatabasePanel. One `useDatabaseStats` observer at that parent supplies both; the card must not start an independent observer.
The design defines measured-time freshness, refresh behavior, failure presentation, cancellation, and identity scope.
Backend URL, process generation, and auth boundaries are explicit. The installation-wide resource has no workspace scope.

Completion evidence includes both consumer paths, explicit reload, failure recovery, and identity transitions.
Tests must preserve unrelated form state. The implementation must remove the old state owner in the same PR.

### QUERY-03: backups

Assignee: [Carlos Florêncio](https://github.com/carlosflorencio). Design: [backup list Query cache](../specs/system-page/system-design/backup-list-query-cache.md). Work order: [Task 01](../plans/system-backup-query/task-01-backup-list-query.md). PR: [#4271](https://github.com/kdlbs/kandev/pull/4271). Kandev task ID: `24c8f330-1bbd-4cb7-84af-1d86c9b335ca`.

The current `reload()` returns `Promise<SnapshotInfo[]>` and rethrows the original error; create polling depends on its result and rejection. The Query owns the sole list value, while mutation feedback remains separate. Its mutable snapshot policy is finite: 30-second staleness, 5-minute inactive retention, mount-always refresh, stale-only focus/reconnect refresh, no automatic retry, and no polling interval.

The existing Query identity and targeted cleanup cover the full backend URL, boot ID, and auth identity without remounting the shell or clearing unrelated resources. Successful create/delete/reset and observed retention attempts refresh only the captured matching identity. Failed deletes preserve the list; reset reconciles terminal outcomes; restore retains its quit/relaunch process identity behavior. GET member access and admin-only mutations/downloads remain unchanged.

Completion evidence includes concurrent consumers, empty/loading/error/reload and last-good-data behavior, permissions, external-maintenance freshness, mutation and retention races, cancellation, backend/auth/process identity transitions, and preservation of unrelated shell/form state. The list remains separate from retention status and database-statistics snapshots.

### QUERY-04: disk usage

Assignee: [Carlos Florêncio](https://github.com/carlosflorencio). Design: [disk usage Query cache](../specs/system-page/system-design/disk-usage-query-cache.md). Work order: [Task 01](../plans/disk-usage-query-migration/task-01-disk-usage-query.md). PR: [#4291](https://github.com/kdlbs/kandev/pull/4291). Kandev task ID: `f6d34be3-bf57-4ca5-9596-dabd4e6372cc`.

The current hook observes terminal `disk-walk` jobs and reloads the snapshot.
It also polls every 1500 milliseconds while the snapshot reports `computing`.
This fallback covers a completion event missed before the WebSocket connection opens.

The design must identify one owner for event-to-cache invalidation.
Completion includes success, failure, duplicate events, missed events, reconnect, and leaving the page during a request.
Removing the fallback requires equivalent recovery evidence. Moving job streams into Query is outside this increment.

## Other systems

| System                    | Disposition                    | Prerequisite before migration                                            |
| ------------------------- | ------------------------------ | ------------------------------------------------------------------------ |
| Tasks / Office            | Deferred, not inventoried here | Agree projections and event ordering before changing cache ownership     |
| Sessions / transcripts    | Deferred, not inventoried here | Separate finite metadata from high-frequency streams and ordered history |
| Integrations / code hosts | Deferred, not inventoried here | Inventory provider capabilities, credentials, and repository identity    |
| Workspaces / repositories | Deferred, not inventoried here | Inventory boot snapshots, selected UI state, and resource identity       |

These rows are not claims that every resource belongs in Query.
Each selected resource gets its own row and owning system-design reference.

## Pilot constraints to preserve

The [SystemInfo design](../specs/platform/system-design/system-info-query-cache.md) owns the current technical contract.
The [restart design](../specs/platform/system-design/backend-restart-page-recovery.md) owns process-generation recovery.

- The authenticated app branch has a stable QueryClient. Identity changes must not remount forms or the shell.
- Query keys distinguish the full API base URL, page boot ID, and auth identity for SystemInfo.
- Native query cancellation reaches the fetch transport.
- Obsolete SystemInfo entries are removed in the same effect as cancellation, without a delayed removal callback.
- A rapid A-to-B-to-A identity transition must not remove the current A query.
- The boot payload contains no full SystemInfo snapshot. The pilot does not invent `initialData`.
- Restart, self-update, and generation probes remain independent no-store reads.
- SystemInfo uses `networkMode: "always"`. Its process-immutable freshness policy does not define mutable-resource policy.

The current cleanup targets SystemInfo only. The second resource needs deliberate identity cleanup coverage without erasing unrelated caches.

## Entry checklist

Before an entry becomes planned, record:

1. The owning system, source paths, consumers, and current state owner.
2. The approved identity, freshness, mutation, boot, and WebSocket contracts.
3. The state, effects, subscriptions, or actions that the PR will remove.
4. The applicable existing requirements and system design, or an explicit design gap.
5. The assignee, work order, task link, and focused verification commands.

Before an entry becomes done, record its merged PR and regression evidence.
Do not retain dual authoritative caches or hidden bridge setters as the completed migration.
