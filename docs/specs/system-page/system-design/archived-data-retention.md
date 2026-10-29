---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001
  - REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002
  - REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003
  - REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004
---

# Archived Data Retention System Design

## Purpose and boundaries

This capability belongs to system-page. The
[requirements](../requirements/archived-data-retention.md) define the outcomes.
It is deliberately a sibling of, not an extension of, the
[tool payload retention design](tool-payload-retention.md): that policy is
inactivity-based over live task history, this one is age-based over data that
already belongs to finished work.

The [DB History Retention design](db-history-retention.md) broadens this
capability's transcript target from archived-only to finished-or-archived and
moves its initial window from 30 to 14 days. It does not add a record or a card:
it extends this policy's `archived_data_retention` record and its settings card in
place. This document retains the record, cleanup-snapshot, and governance design;
the broadened message-eligibility contract is stated in the DB History Retention
design.

The Database tab of `DataLogsSettings` mounts `ArchivedDataRetentionCard` beside
`ToolPayloadRetentionCard`. The card owns its own policy and save contributor
`system:archived-data-retention` and reuses the existing status/draft
error-channel split.

## Relationship to the tool payload retention policy

The two policies reduce the same message field through the same reducer, so the
design keeps them from diverging:

- Both call `models.ReduceToolPayload` unchanged, so both write the identical
  `metadata.payload_retention` marker. The reducer's `already_removed` reason
  makes the second trigger a no-op on a row the first already handled.
- This policy never reads or writes the `tool_payload_retention` record, its
  revision, or its approved state. It stores under its own settings key
  `archived_data_retention`.
- The archived target is a safety net for archived tasks the inactivity policy
  declines: `PayloadTaskEligible` additionally requires terminal sessions and no
  pending work, and those gates decline some archived tasks. The archived target
  gates on `archived_at` only, which is what its requirement states explicitly.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001 | Policy, Eligibility, Analysis |
| REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002 | Message history target |
| REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003 | Cleanup-job snapshot target |
| REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004 | Governance, Scheduling, Settings surface |

## Components and responsibilities

`internal/system/dataretention` owns the policy record, preparation, scheduling,
both scans, bounded progress persistence, and the HTTP surface. It is composed in
`internal/system/system.go` and shares the backup helpers and the maintenance
quiesce chain with `internal/system/toolretention`.

`internal/task/models` owns both reducers. `ReduceToolPayload` is reused
unchanged; `ReduceCleanupSnapshot` is new and lives beside it.

`internal/task/repository/sqlite` owns candidate queries, eligibility
re-checks, and guarded writes for both targets.

`internal/system/maintenance.Guard` is shared, so a batch defers while backup,
restore, reset, or compaction owns the writer.

## Policy

`Policy` carries `Enabled`, `ArchivedAge`, `CleanupAge`, and `Revision`. Both
ages are whole days in `1..3650`. Defaults are 14 days (the message-metadata
window, broadened from 30 by DB History Retention) and 7 days with `Enabled`
false. `Cutoff` returns `now - days` in UTC and is inclusive, so a row exactly at
the boundary is in scope for the pass.

`Revision` makes a stale concurrent save a conflict rather than an overwrite.
`ApprovedRevision` is the gate: a deletion batch re-checks
`ApprovedRevision == Revision` inside the writer transaction, so a save that
lands mid-pass stops that pass.

## Stored record

One settings row under `archived_data_retention` holds the policy, the
preparation state and choice, the approval, the backup receipt, a
first-mutation flag, the resumable `progress`, and the last analysis and last
run. `progress` carries both targets' keyset cursors, the captured upper
cursors, the captured `PRAGMA schema_version`, and the current phase, so
finishing one target never loses the other's position.

A record that fails validation is an error, not a default: an unreadable policy
blocks deletion rather than proceeding on defaults.

## Governance frame

Preparation, approval, batching, and the daily schedule are the tool-retention
frame, restated only where the second target required it:

- Enabling, and any change that brings a window forward, requires an explicit
  backup-or-skip choice and leaves `Preparation` pending. A pending choice arms
  no deletion.
- A selected backup is taken through `backups.Service.CreateForRetention` and
  verified through `VerifyRetentionBackupUnderLease` before the first mutation,
  and the receipt is re-verified only when a row is actually about to change so
  an empty batch does not re-hash a multi-gigabyte file.
- An interrupted preparation fails closed on restart, so a restart cannot arm a
  deletion silently.
- `Cancel` stops a pass at its next batch boundary. Cancelling a backup
  invalidates the approval so a late backup cannot arm cleanup.

## Message history target

The target's eligibility is defined by the [DB History Retention
design](db-history-retention.md): the union of archived tasks (`ArchivedTaskEligible`)
and finished tasks (`PayloadTaskEligible` at the same cutoff). Selection walks
`tasks` under a captured upper task id, then that task's sessions, then each
session's messages by `rowid` under a captured upper rowid. Both upper cursors
are captured when the pass starts, so rows created mid-pass are not walked.

`ArchivedTaskEligible` re-reads `archived_at` for the current task; a task
unarchived since selection fails the check and is skipped with
`protected_tasks`. The finished path re-reads the inactivity contract inside the
writer transaction. Timestamps are compared through SQLite date functions because
legacy rows can carry textual UTC offsets that break lexical ordering, and the
function refuses any non-SQLite driver before that comparison runs.

`ReduceArchivedMessageMetadata` re-reads eligibility inside the writer
transaction and writes with a compare-and-swap on the previous `metadata`
value, so a concurrent write wins instead of being overwritten.

## Cleanup-job snapshot target

Selection walks `task_resource_cleanup_jobs` by `state = succeeded` under a
captured upper job id. `SucceededCleanupJobEligible` re-checks both the state
and the completion cutoff inside the writer transaction, and the update is
guarded on `state = succeeded` plus the previous snapshot value, so a job that
left the terminal state keeps its snapshot.

`ReduceCleanupSnapshot` retains the **archive source evidence contract**, not the
manifest alone. `Service.GetTaskSourceManifest` decodes the snapshot, skips a
generation with no manifest, requires `workspace_id` to be non-empty, authorizes
that workspace, and matches every manifest against `worktrees`. Retaining
`archive_source_manifest` while dropping either of the other two would leave a
readable manifest failing the workspace-identity or worktree-match check, so the
reduced generation would return an error instead of the evidence. The reducer
therefore keeps all three and drops everything else, including `sessions`,
`agent_profile_snapshot`, `worktree_branch_metadata`, `task_environment`,
`inventory_repair`, `orphan_reap_*`, `stop_targets`, `attachments`,
`ssh_task_dirs`, and any unknown top-level key.

A malformed snapshot fails closed with the `malformed` reason and is skipped,
never deleted. A snapshot that is empty or already reduced is counted and left
intact.

## Bounds and observability

A batch is bounded at 100 rows and 8 MiB, with a 200 ms per-statement budget. A
single row above the 32 MiB `models.ToolPayloadMaxBytes` ceiling is skipped
intact and counted as oversize. Both targets share one batch budget so neither
can consume the whole pass and starve the other. A running pass continues in
short bursts for at most 30 s of wall clock, then returns to a one-minute idle
tick; the next pass is due 24 h after a complete pass.

Progress is reported through `internal/system/jobs` as bounded counters only:
rows, eligible, reduced, bytes, total bytes, oversize, and skipped reasons per
target. No task, session, message, or job identifier is ever a metric label, and
no new table is introduced.

## Failure handling

- A schema change under a running pass ends the pass as `partial` with
  `database_changed`, because the captured cursors no longer describe the same
  database.
- An eligibility decision that cannot be made inside the statement budget skips
  that task, counts `eligibility_budget`, and ends the pass as `partial`, so an
  unbounded decision is never presented as a complete removal.
- A pass that already reduced something fails as `partial`, not `failed`, so the
  effect stays visible.
- Disabling the policy prevents subsequent batches after the save commits.
  Completed removals remain removed.

## Effect claims

A `metadata` or `resource_snapshot` update frees pages for reuse. It does not
shrink the database file and does not reclaim index space for the message
metadata expression indexes, which hold their own copy of the removal marker. No
file-size or index-capacity recovery is claimed, and no VACUUM, TRUNCATE, or
backup disposal runs in this capability.

## Settings surface

`ArchivedDataRetentionCard` presents the two windows, the analysis estimate per
target, the preparation and pass state, and the next scheduled check. Its save
contributor is `system:archived-data-retention`. All copy goes through `t()` in
every catalog, including the Traditional Chinese pair through the existing
hant conversion script. Copy names the message window as covering finished or
archived tasks, states the stored-byte reduction rather than a file-size
reduction, and points at the separate compaction action.

Read-only users see status and cannot analyze, save, or start a pass. A failed
status read reports that current status is unavailable while retaining the last
known analysis, matching the existing card.

## Related documents

- [Requirements](../requirements/archived-data-retention.md)
- [DB History Retention design](db-history-retention.md)
- [Tool Payload Retention system design](tool-payload-retention.md)
- [Settings storage pages design](system-data-storage-pages.md)