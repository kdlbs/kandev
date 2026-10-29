---
status: active
system: system-page
created: 2026-10-05
owners:
  - kandev
---

# Archived Data Retention Requirements

## Overview

Administrators can reduce installed storage by shrinking data that belongs to
finished work: the tool progress detail inside transcripts of finished or
archived tasks, and the operational snapshot stored on completed resource-cleanup
jobs. System-page owns this maintenance policy and its Data & Logs settings
surface.

This policy governs two targets with independent windows. It does not replace the
existing tool-payload policy, which remains the inactivity-based policy over
active-task conversation history and keeps its own setting, revision, and approved
state. See [Tool Payload Retention](tool-payload-retention.md).

The transcript target is broadened by the [DB History Retention
capability](db-history-retention.md): eligibility moves from archived tasks to
finished or archived tasks, and the initial message window becomes 14 days.
DB History Retention owns the broadened target contract; this document retains
the removal, preservation, and governance invariants that both documents share.

Both targets ship **disabled**. Cleanup remains disabled until an administrator
completes the preparation choice.

## Terminology

- **Archived task:** A task whose `archived_at` is set at or before the archived
  cutoff. A task unarchived before a batch commits is not an archived task.
- **Completed cleanup job:** A `task_resource_cleanup_jobs` row whose state is
  `succeeded` and whose `completed_at` is at or before the cleanup cutoff. Every
  other state, including a missing or unreadable completion timestamp, is
  retained.
- **Archive source evidence contract:** The three snapshot fields
  `archive_source_manifest`, `workspace_id`, and `worktrees`. Together they are
  what the archive-source-manifest read endpoint still consumes from a completed
  job.
- **Stored-byte reduction:** The reduction in stored column bytes. This is
  different from a reduction in the database file size.

## Requirements

### REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001: Policy and analysis

**Intent:** Let administrators understand both targets before data is removed.

**User story:** As an administrator, I want a savings estimate for archived
transcripts and completed cleanup jobs, so that I can compare the storage effect
with the history I give up.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.1:** The Data & Logs page at
  `/settings/system/data-storage` shall expose an independent archived-data
  policy, disabled by default, including on upgrades.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.2:** Administrators shall select a
  whole-day window for each target independently. The initial values are 14 days
  of message history and 7 days of finished cleanup. Valid values are 1-3650
  days. Invalid input shall save nothing.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.3:** Eligibility shall use the
  archived timestamp for transcripts and the completion timestamp for cleanup
  jobs. The window includes its boundary, so a row exactly at the cutoff is in
  scope for the pass.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.4:** An administrator shall be able
  to analyze an unsaved policy while cleanup remains disabled. Opening the page
  shall not start an analysis, backup, or cleanup.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.5:** Analysis shall report each
  target separately: rows scanned, rows eligible, rows reduced, stored bytes
  before the pass, bytes removed, oversize rows, and skipped-data counts. A
  partial or failed analysis shall never appear as a complete estimate. Policy
  edits shall mark previous results as stale.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-001.6:** The page shall state that the
  result is a stored-byte reduction and not a file-size reduction, and shall
  point at the separate compaction action.

### REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002: Message history detail

**Intent:** Reclaim finished conversation volume without damaging history. The
eligibility scope of this target is the finished-or-archived message-metadata
target of [DB History Retention REQ-001](db-history-retention.md); this
requirement states the removal and preservation invariants that scope shares.

**User story:** As an administrator, I want tool progress detail removed from
finished or archived transcripts, so that old conversations stop dominating
storage while the conversation itself stays readable.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.1:** Cleanup shall use the existing
  removal rules and the existing removal marker. It shall not introduce a second
  marker vocabulary or a divergent remover for the same field.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.2:** Cleanup shall preserve message
  rows, content, identity, ordering, type, and timestamps. Only `metadata` shall
  shrink.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.3:** A task that becomes active
  between selection and commit shall not be modified. A task inside the window
  shall not be selected.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.4:** An archived task is gated on
  `archived_at` alone, so a nonterminal session, a pending question, or a queued
  workflow admission on an archived task shall not by itself block this target. A
  finished (non-archived) task is additionally gated on the inactivity contract.
  The existing inactivity tool-payload policy keeps its own protections
  unchanged.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.5:** Removed details shall not
  reappear after delayed updates, replay, or task resumption. Conversations shall
  show an explicit removed-payload state.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-002.6:** A row already carrying the
  removal marker shall be left intact and counted as already removed, so a second
  trigger is a no-op rather than a second removal.

### REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003: Completed cleanup-job snapshots

**Intent:** Reclaim finished-job operational material while keeping the archive
source evidence readable.

**User story:** As an administrator, I want the operational snapshot of a
finished cleanup job reduced, so that completed jobs stop consuming storage while
archive evidence stays retrievable after task deletion.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.1:** Only a `succeeded` job with a
  readable completion timestamp at or before the cutoff shall be reduced. The
  `pending`, `retry_wait`, `waiting_for_clean`, `running`, `prepared`, `failed`,
  and `cancelled` states shall never be reduced.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.2:** The archive source evidence
  contract shall survive reduction. `archive_source_manifest`, `workspace_id`, and
  `worktrees` shall be preserved byte-for-byte, because the manifest read path
  authorizes `workspace_id` and matches each manifest against `worktrees`.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.3:** Reducing a generation that
  carries a manifest shall return the same manifests, apply the same workspace
  authorization, and reject a foreign workspace exactly as before reduction,
  including after the task row has been deleted.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.4:** A snapshot shall record a
  reduction marker carrying the marker version, the reduction time, the removed
  field paths, and the net byte reduction. An unknown top-level key shall be
  dropped deliberately, because the retained set is the evidence contract rather
  than an inventory of known field names.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.5:** A malformed snapshot shall be
  skipped and counted, never deleted.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-003.6:** The job row, its trigger, its
  state, attempt count, `next_attempt_at`, `last_error`, `created_at`,
  `updated_at`, and `completed_at` shall remain, so cleanup history and its result
  summary stay available.

### REQ-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004: Backup, scheduling, and access

**Intent:** Keep removal deliberate, bounded, and reversible at the database level.

**User story:** As an administrator, I want a backup choice and a bounded daily
pass, so that the policy cannot quietly consume the database or the writer.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.1:** The preparation, approved
  revision gate, bounded batches, and daily schedule shall follow the existing
  tool-payload governance frame. Enabling this policy shall not read, write, or
  change the `tool_payload_retention` record, its revision, or its approved state.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.2:** Before the first mutation the
  administrator shall explicitly choose backup or continue without backup. No
  choice shall be inferred from silence. A pending choice arms no deletion.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.3:** If backup is selected, cleanup
  shall wait for a completed, verified backup. Failure, cancellation, or an
  interrupted preparation shall block cleanup.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.4:** Each batch shall be bounded by
  row count and byte budget, and a single row above the oversize ceiling shall be
  skipped intact. A pass shall recheck eligibility inside the writer transaction
  and end the pass when the database schema changed under it.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.5:** No batch shall run while
  backup, restore, reset, or compaction owns the shared maintenance admission.
  Reset and restore shall stop this worker before they proceed.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.6:** An unreadable policy or
  preparation state shall block deletion rather than proceeding.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.7:** An enabled policy shall check
  daily for eligible data, and administrators shall also be able to request a pass
  now under the saved, enabled policy. Only one archived-data operation shall
  execute at a time, and it shall be cancellable at a batch boundary.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.8:** Only SQLite is supported. Other
  engines shall show an unavailable state and reject analysis and mutation
  requests without side effects.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.9:** Read-only users shall see
  status but cannot analyze, change the policy, or start a pass. All copy shall be
  localized, and phone users shall have the same actions with 44px touch targets,
  one page scroll owner, and no horizontal overflow.
- **AC-SYSTEM-PAGE-ARCHIVED-DATA-RETENTION-004.10:** No pass shall compact, vacuum,
  truncate, or discard a manual backup. Compaction stays the separate explicit
  operator action.

## Out of scope

- Message deletion, database compaction, index removal, and file-size reduction.
- Any change to the existing inactivity-based tool-payload policy, or to Office
  run-history and filesystem retention.
- Fixing the source duplication of the large per-session metadata blobs inside a
  cleanup-job snapshot; this policy reduces the copy, it does not stop its growth.
- Retention for worktrees, task environments, or provider-side history.

## Related documents

- [System design](../system-design/archived-data-retention.md)
- [DB History Retention requirements](db-history-retention.md)
- [Tool Payload Retention requirements](tool-payload-retention.md)
- [Tool Payload Retention system design](../system-design/tool-payload-retention.md)