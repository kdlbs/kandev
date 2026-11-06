---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-001
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-002
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-003
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-004
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-005
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006
---

# DB History Retention System Design

## Purpose and boundaries

This capability belongs to system-page and owns the periodic retention of
finished-or-archived message metadata and completed cleanup-job snapshots, plus
the design criteria that bound the remaining growth sources. It is deliberately
a sibling of, not a replacement for, the
[Tool Payload](tool-payload-retention.md) and
[Archived Data](archived-data-retention.md) policies: the tool payload policy
stays the inactivity-based policy over active-task history, and this capability
does not read or write its record.

Unlike the earlier draft of this capability, the implementation does **not**
introduce a second policy record or a second settings card. It extends the
existing periodic retention policy **in place**. The persisted policy record,
revision, approval gate, preparation, scheduler, bounded passes, and the
`ArchivedDataRetentionCard` already exist in `internal/system/dataretention`;
the message target there is broadened from archived-only to finished-or-archived
and its initial window moves from 30 to 14 days. A second `db_history_retention`
record would require either rewriting the policy-hardwired frame or duplicating
it, which is not the smallest safe reuse, so this design reuses the existing
frame instead.

The task system owns the persisted messages, sessions, catalogs, plan revisions,
and cleanup jobs this capability reduces. The capability reuses the periodic
retention governance frame in `internal/system/dataretention` and the existing
offline maintenance command in `internal/maintenance` rather than building a
second framework.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-001` | Reused facilities, Policy, Message-metadata target |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-002` | Cleanup-snapshot target |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-003` | Deferred design: session model metadata |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-004` | Deferred design: plan revisions |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-005` | Policy, Settings surface |
| `REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006` | Bounds, Failure handling, Effect claims, Security |

## Why the existing target is extended, not duplicated

The original operating request named "messages older than 14 days in finished or
archived tasks" and "completed cleanup snapshots". The request is to make the
operator's manual cleanup periodic. The existing facilities already implement
almost all of it, and the remainder is a scope and window change to an existing
target, not a new policy.

The existing periodic frame is hardwired to one policy:

- `internal/system/dataretention/store.go:12` fixes the record to the single
  settings key `archived_data_retention`.
- `internal/system/dataretention/policy.go:38` fixes the record to two ages
  (`archived_age`, `cleanup_age`).
- `internal/system/dataretention/handler.go:13` fixes the HTTP base to
  `/database/archived-data-retention` and its request shape.
- `internal/system/dataretention/scan.go:139` walks archived tasks only.

Adding a second, differently shaped policy would require re-designing or
copying roughly ten reviewed files and a completed settings card, so this design
instead broadens the existing message target. Because reduction is idempotent by
removal marker, the tool payload policy remains a no-op on any row this
capability already reduced, and this capability is a no-op on any row the tool
payload policy already reduced.

## Reused facilities

- **Reducers and removal markers.** `models.ReduceToolPayload`
  (`internal/task/models/tool_payload_retention.go`) reduces tool progress detail
  in a message's `metadata` and writes the single `payload_retention` removal
  marker. `models.ReduceCleanupSnapshot`
  (`internal/task/models/cleanup_snapshot_retention.go`) reduces a cleanup
  snapshot to its archive source evidence and writes its own marker. Both are
  reused verbatim; this capability adds no second marker vocabulary.
- **Eligibility contracts.** `PayloadTaskEligible`
  (`internal/task/repository/sqlite/tool_payload_retention_scan.go:18`) is the
  finished-task contract: an allowed task state, no queued launch, every session
  terminal, and no pending input, permission, queued message, pending move, or
  unfinished run. `ArchivedTaskEligible` and `SucceededCleanupJobEligible`
  (`internal/task/repository/sqlite/archived_data_retention_scan.go:28`) are the
  archived-task and completed-job contracts.
- **Persistence helpers.** `WriteReducedMessageMetadata`,
  `WriteReducedCleanupSnapshot`, `NextArchivedTaskID`, `NextSessionID`,
  `NextArchivedMessageRows`, `NextSucceededCleanupJobID`, and the upper-cursor
  capture functions provide keyset paging and compare-and-swap writes.
- **Governance frame.** `internal/system/dataretention` already owns the policy
  record shape, the preparation and backup choice, the approved-revision gate,
  bounded progress with keyset cursors, the daily scheduler, and the shared
  `internal/system/maintenance.Guard` admission. The broadened target is the same
  pass that frame already runs.
- **Settings surface.** `ArchivedDataRetentionCard`
  (`apps/web/components/settings/system/archived-data-retention-card.tsx`) and
  its hooks, API client, and types already present both windows, the analysis
  estimate per target, preparation state, and the next scheduled check. This
  capability broadens its copy and its message-window initial value; it adds no
  card.
- **Offline maintenance.** `internal/maintenance` (the `kandev maintenance
  database` command) already owns dry-run versus execute, the exclusive-access
  ownership lock, a verified `VACUUM INTO` backup, transactional deletes, and
  staged compaction. The deferred design criteria reuse it for any row removal.

## Policy

The existing settings record under `archived_data_retention`, version 1, carries
`Enabled=false`, two independent whole-day ages, and a monotonic `Revision`. This
capability changes the message target's meaning, not the record shape:

- The message age keeps its persisted JSON field (`archived_age`), but its
  semantics broaden to the message-metadata window and its initial value becomes
  14 days. A record an administrator already saved keeps its saved value.
- The cleanup age (`cleanup_age`) keeps its 7-day initial value and its meaning.
- The revision, `ApprovedRevision` gate, preparation, choice, receipt, bounded
  progress, daily schedule, and the tool-payload isolation are unchanged.

`Revision` makes a stale concurrent save a conflict rather than an overwrite.
`ApprovedRevision` remains the deletion gate, re-checked inside the writer
transaction so a save that lands mid-pass stops that pass. The age is a whole-day
window in `1..3650`, inclusive at the boundary, so a row exactly at the cutoff is
in scope.

Enabling this capability never reads, writes, or changes the
`tool_payload_retention` record.

## Message-metadata target

Selection walks tasks under a captured upper task id, then each session, then
each message page. The selection and its upper cursor broaden from archived tasks
to the union of archived tasks (`archived_at IS NOT NULL`) and finished tasks
(task state `COMPLETED`, `FAILED`, or `CANCELLED`). A task is eligible when it is
archived under `ArchivedTaskEligible` **or** finished under `PayloadTaskEligible`
at the same cutoff; the finished path additionally requires the task's terminal
state, so a nonterminal task never qualifies through it. Both checks use the same
chronological SQLite date comparison the existing passes use. Equal timestamps
are retained.

For an eligible page, the target applies `models.ReduceToolPayload` to each
message's `metadata` and writes through `WriteReducedMessageMetadata` under a
compare-and-swap. The task state and cutoff are re-read inside the writer
transaction, so a task that becomes active again before commit keeps its
metadata. A row already carrying the marker is counted as already removed.

Messages, task identity, attachments, entity references, and every non-payload
metadata key are never touched, and no message row is ever deleted. A second
pass is a no-op. The tool payload policy keeps its own protections for rows it
reaches first, and the two policies remain idempotent by marker.

## Cleanup-snapshot target

The target walks `task_resource_cleanup_jobs` in `succeeded` state with a
readable `completed_at` at or before the cutoff, mirroring `scanJobBatch` in
`internal/system/dataretention/scan.go:296`. For each job it reads the resource
snapshot, applies `models.ReduceCleanupSnapshot`, and writes through
`WriteReducedCleanupSnapshot` under the succeeded-state compare-and-swap that
keeps a job that left the succeeded state. This target already exists in the
frame; this capability keeps it on the same bounded pass as the message target,
sharing one batch budget so neither starves the other.

Reduction keeps the archive source evidence contract (`archive_source_manifest`,
`workspace_id`, `worktrees`) and drops the rest deliberately, because the
retained set is the evidence contract rather than an inventory of field names. A
malformed snapshot is skipped and counted, never deleted. Ownership is pinned to
this one column of this one job kind; no other snapshot, session metadata,
transcript, or run record is in scope.

## Deferred design: session model metadata and repeated model-list copies

This is design criteria for REQ-003; it authorizes no implementation in this
package. The design extends, and does not contradict, the accepted
[session model catalog reference decision](../../../decisions/2026-10-05-session-model-catalog-reference.md),
which already shares the catalog for new sessions and explicitly defers pruning
already-written inline rows.

- **Eligibility.** A session of a finished or archived task that is not live and
  needs no recovery or continuation, with activity at or before the cutoff. The
  session selection is preserved exactly and never removed.
- **Reduction.** Compose the runtime-identity key with `SessionModelCatalogKey`
  from the session's `agent_profile_id`, `execution_profile_id`, and
  `executor_profile_id`; store the catalog once through `StoreSessionModelCatalog`;
  rewrite the session with `catalog_ref` plus the unchanged selection
  (`SplitSessionModelCatalog`) under a compare-and-swap. A non-composable key or a
  store failure leaves the inline catalog unchanged and counts a skip.
- **Revision pruning.** A `session_model_catalogs` revision is eligible only when
  no session `catalog_ref` names it and its `created_at` is at or before the
  cutoff. Because the reference check spans the whole installation and removal
  deletes rows, it runs only through the offline, backup-gated maintenance
  command; an indeterminate check retains the revision.
- **Safety.** Idempotent, compare-and-swap, no message or task-identity change,
  counts and bytes only.

## Deferred design: plan revisions

This is design criteria for REQ-004; it authorizes no implementation in this
package. The existing read-only selector `ListObsoletePlanRevisionCandidates`
(`internal/task/repository/sqlite/plan.go`) already protects the current HEAD
(`MAX(revision_number)`) and every revision named by another revision's
`revert_of_revision_id`. The design adds:

- **User edits preserved.** A revision with `author_kind = 'user'` is never a
  candidate, so user-authored history survives even when superseded.
- **Active tasks excluded.** Only revisions of finished or archived tasks are
  eligible.
- **Recency window.** The most recent non-HEAD revisions per task are preserved
  within a configurable window. The window is a reversible default, not a value
  chosen to match a byte target.
- **Offline only.** Removal runs only through the offline, backup-gated
  maintenance command with a verified backup and a transactional delete.
  Deletion is idempotent, and a revision inserted after candidate selection is
  never in the candidate set, so a task resumed before the delete commits keeps
  its new revision. The current plan document, HEAD, and revert ancestry stay
  readable.

## Data and contracts

State and time fields the eligibility uses, all already persisted:

| Source | Fields |
| --- | --- |
| `tasks` | `state`, `archived_at`, `queued_for_step_id`, `queued_at`, `created_at`, `updated_at` |
| `task_sessions` | `state`, `started_at`, `completed_at`, `updated_at`, `metadata` |
| `task_session_turns` | `started_at`, `completed_at`, `created_at`, `updated_at`, `metadata` |
| `task_session_messages` | `created_at`, `updated_at`, `metadata` |
| `task_resource_cleanup_jobs` | `state`, `completed_at`, `resource_snapshot` |
| `session_model_catalogs` | `catalog_key`, `revision`, `catalog_json`, `created_at` |
| `task_plan_revisions` | `task_id`, `revision_number`, `author_kind`, `revert_of_revision_id`, `created_at` |

No new table, index, or migration is required for the message and
cleanup-snapshot targets. The target broadens the selection query and the
eligibility check of the existing message target; the record shape is unchanged.

## Bounds and scheduling

A batch is bounded by row count and stored bytes, with a per-statement budget,
exactly as the existing passes. A row above the ceiling is skipped intact and
counted as oversize. A running pass continues in short bursts, then returns to an
idle tick; the next pass is due 24 hours after a complete pass. Progress is
bounded counts per target only. A schema change under a running pass ends the pass
as partial, because the captured cursors no longer describe the same database.

## Failure handling

- An eligibility decision that cannot complete within its statement budget skips
  that task, counts a budget skip, and ends the pass as partial.
- A reduction error on one row leaves its metadata unchanged and is counted; it
  does not fail the pass.
- Cancellation, a policy change, shutdown, or maintenance contention stops at the
  next batch boundary. A cancelled pass leaves committed reductions removed and
  resumes from persisted progress without double counts.
- The offline command fails closed when the ownership lock is held or the
  verified backup is absent, and rolls back the whole delete transaction on error.

## Effect claims

A metadata rewrite frees pages for reuse. It does not shrink the database file
and does not reclaim index space. No file-size or index-capacity recovery is
claimed. No `VACUUM`, `TRUNCATE`, or backup disposal runs in the periodic
capability; compaction stays the separate explicit operator action of the offline
maintenance command. The result and the settings copy state a stored-byte
reduction, not a file-size reduction, and report logical reduction distinctly
from the separate compaction outcome.

## Security and observability

Analysis and mutation require the existing install admin permission; read-only
users see status and cannot analyze, save, or start a pass. No message, task,
session, revision, or catalog identifier and no metadata content appears in a
metric label, log, API response, or report; only counts and byte estimates do.
The capability adds no permanent gate, fingerprint, hash, signature, audit,
monitor, or ledger.

## Related decisions

- [Session model catalog reference](../../../decisions/2026-10-05-session-model-catalog-reference.md)
- [SQLite writer transaction admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
- [Install-wide storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md)
- [Tool payload removal policy](../../../decisions/2026-09-14-tool-payload-removal-policy.md)
