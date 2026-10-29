---
status: draft
system: system-page
created: 2026-10-11
owners:
  - kandev
---

# DB History Retention Requirements

## Overview

A long-lived Kandev installation accumulates message metadata and history faster
than the existing per-target policies reclaim it. This capability makes the
operator's already-approved manual cleanup periodic and a product feature: it
reduces message metadata for messages whose task is finished or archived at a
14-day window, and reduces completed cleanup-job snapshots on the same periodic
pass. It also fixes reviewed design criteria for the remaining large categories
(session model metadata, repeated model-list copies, and superseded plan
revisions) so a later implementation has an approved contract.

System-page owns the contract because it owns install-wide storage maintenance
policy, its backup-gated preparation frame, and the Data & Logs settings surface.
The task system owns the persisted session, message, plan, and cleanup-job data
these targets reduce.

This capability is **implemented by extending the existing periodic retention
target in place**, not by a new policy. It reuses the `archived_data_retention`
settings record, its revision and approval state, its preparation and daily
schedule, and the existing `ArchivedDataRetentionCard`. It does not create a
second record, a second revision, or a second settings card. The message target
is broadened from archived-only to finished-or-archived and its default window
moves from 30 to 14 days; the completed cleanup-job snapshot target is reused
unchanged. This is deliberately a sibling of, not a replacement for, [Tool
Payload Retention](tool-payload-retention.md), which stays the inactivity-based
policy over active-task conversation history and keeps its own record, window,
and approved state.

The 14-day window is the window the owner approved for this cleanup. It is a
product contract for this capability, not a reversible default chosen to match a
byte-size target. The 14-day message window therefore becomes the initial value
of the existing record; a stored record an administrator already saved keeps its
saved value until the administrator changes it.

Recurrence is the point: the growth that motivated the operating cleanup must
not return. Periodic enforcement of the 14-day message window and the completed
cleanup-snapshot target (REQ-001, REQ-002) is that recurrence control. Delivery
inbox and effect retention stays with its own task, and restart-midturn repair
stays with Operations.

## Terminology

- **Finished task:** A task whose state is terminal (`COMPLETED`, `FAILED`, or
  `CANCELLED`) and that satisfies the existing inactivity contract
  (`PayloadTaskEligible`): no queued launch, every session terminal, and no
  pending input, permission, queued message, pending move, or unfinished run at
  or before the cutoff.
- **Archived task:** A task whose `archived_at` is set at or before the relevant
  cutoff. This is the eligibility contract the existing archived target uses.
- **Message metadata:** the `metadata` column of `task_session_messages`. Only
  tool progress detail inside it is removed; the row, its content, identity,
  ordering, type, and timestamps stay.
- **Message-metadata target:** the periodic reduction of message metadata in
  finished or archived tasks. It is the existing archived transcript target
  broadened to finished-or-archived eligibility and a 14-day window.
- **Cleanup-snapshot target:** the periodic reduction of the operational snapshot
  on a completed task-resource cleanup job.
- **Archive source evidence contract:** the `archive_source_manifest`,
  `workspace_id`, and `worktrees` fields a completed cleanup job still serves.
- **Shared catalog revision:** one `session_model_catalogs` row identified by
  `(catalog_key, revision)`.
- **Inline provider catalog:** a `task_sessions.metadata.acp_model_state` snapshot
  that carries `models` or `config_options` directly instead of a `catalog_ref`.
- **Session selection:** the session-owned part of `acp_model_state`:
  `current_model_id`, `current_mode_id`, `selected_config_values`, the settings
  attempt and source identity, the model and mode generations, and the settings
  policy. Reduction never removes the selection.
- **Obsolete plan revision:** a `task_plan_revisions` row that is not the task's
  current HEAD, is not referenced by another revision's `revert_of_revision_id`,
  is not user-authored, and is outside the configured recency window.
- **Stored-byte reduction:** a reduction in stored column bytes. It is different
  from a reduction in the database file size, which requires separate compaction.

## Requirements

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-001: Periodically reduce message metadata for finished or archived tasks

**Intent:** Make the operator's approved manual cleanup a periodic product
feature so the growth it addressed does not recur.

**User story:** As an administrator, I want tool detail older than 14 days
removed automatically for finished or archived tasks, so storage stops growing
without deleting conversations.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.1:** An enabled policy shall
  periodically reduce only `task_session_messages.metadata` of messages whose
  task is finished or archived and whose relevant timestamp is at or before the
  cutoff. The default window is 14 days; valid windows are 1-3650 days; the
  cutoff boundary is inclusive.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.2:** The reduction shall reuse the
  existing tool-payload removal rules and removal marker. It shall not introduce
  a second marker vocabulary or a divergent remover for the same field.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.3:** A task shall be eligible when it
  is archived (`ArchivedTaskEligible`: `archived_at` at or before the cutoff) or
  finished (`PayloadTaskEligible` at the same cutoff), compared chronologically
  through the same date comparison the existing passes use. A nonterminal task
  shall not be eligible through the finished path.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.4:** The reduction shall preserve
  every message row, its content, identity, ordering, type, and timestamps, and
  shall never delete a message.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.5:** A row that already carries the
  removal marker shall be left intact and counted as already removed, so a second
  pass is a no-op rather than a second removal.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.6:** Eligibility and the task state
  shall be re-checked inside the writer transaction, so a task that becomes
  active before commit keeps its metadata. The write shall compare-and-swap on
  the previous metadata value so a concurrent writer wins instead of being
  overwritten.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-001.7:** A pass shall be bounded by row
  count, a byte budget, and a per-statement budget. A row above the oversize
  ceiling shall be skipped intact and counted. A pass that cannot complete an
  eligibility decision shall fail closed: it retains the affected rows and
  reports the pass as partial, never complete.

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-002: Reduce completed cleanup-job snapshots on the same pass

**Intent:** Reclaim finished-job operational material on the same periodic frame
while keeping archive evidence readable.

**User story:** As an administrator, I want the snapshot of a completed cleanup
job reduced, so completed jobs stop consuming storage while the archive evidence
stays retrievable.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.1:** Only a
  `task_resource_cleanup_jobs` row in `succeeded` state with a readable
  `completed_at` at or before the cutoff shall be reduced. Every other state,
  including a missing or unreadable completion timestamp, shall be retained.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.2:** The reduction shall reuse the
  existing cleanup-snapshot reducer and its marker, and shall preserve the
  archive source evidence contract (`archive_source_manifest`, `workspace_id`,
  and `worktrees`) byte-for-byte.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.3:** Snapshot ownership is pinned to
  the resource-snapshot column of a completed task-resource cleanup job. No other
  task snapshot, session metadata, transcript, or run record shall be touched by
  this target.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.4:** A malformed snapshot shall be
  skipped and counted, never deleted. The job row, its trigger, state, attempt
  count, `next_attempt_at`, `last_error`, and timestamps shall remain.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.5:** `workspace_id` authorization and
  manifest matching shall return the same result after reduction, including after
  the task row has been deleted. A failure or an indeterminate read shall retain
  the snapshot.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.6:** The cleanup-snapshot target
  shall run in the same bounded periodic pass as the message-metadata target,
  under one shared batch budget, so one target cannot starve the other.

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-003: Session model metadata design criteria (design only)

**Intent:** Fix a safe, reviewed contract for the largest remaining per-session
growth source without authorizing a destructive implementation in this package.

**User story:** As an administrator, I want a safe plan for per-session model
metadata and repeated model-list copies, so a later change can reclaim them
without losing a session's selected model.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-003.1:** The design shall define
  eligibility for sharing an inline provider catalog as a session of a finished
  or archived task that is not live and needs no recovery or continuation, with
  activity at or before the cutoff. The session selection shall be preserved
  exactly and never removed.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-003.2:** The design shall store a catalog
  once under the session's composable runtime-identity key and replace the inline
  copy with a `catalog_ref`. When the key cannot be composed, or the store fails,
  the design shall leave the inline catalog unchanged rather than share a catalog
  it cannot prove is the same one.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-003.3:** The design shall make an
  unreferenced `session_model_catalogs` revision eligible only when no session's
  `catalog_ref` names it and its creation is at or before the cutoff, and shall
  remove it only through the existing offline, backup-gated maintenance path
  after a whole-installation reference check that retains the revision when the
  check is indeterminate.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-003.4:** This document shall designate
  REQ-003 as design criteria only. It authorizes no automatic session reduction
  and no offline catalog pruning in this package.

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-004: Plan-revision retention design criteria (design only)

**Intent:** Fix a safe contract for bounding superseded plan history without
losing history an operator or the product still needs.

**User story:** As an administrator, I want a safe plan for old superseded plan
revisions, so plan history stops growing without limit while the current plan and
user edits stay available.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-004.1:** The design shall make only
  obsolete plan revisions of finished or archived tasks eligible, with revision
  age at or before the cutoff.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-004.2:** The design shall always preserve
  the task's current HEAD (`MAX(revision_number)`), any revision named by another
  revision's `revert_of_revision_id`, and any user-authored revision
  (`author_kind = 'user'`).
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-004.3:** The design shall preserve the
  most recent non-HEAD revisions per task within a configurable recency window.
  When the window is not configured, only the HEAD, ancestry, and user-edit
  protections apply. A destructive count or age shall be an explicit, reversible
  default, not a value invented to match a byte-size target.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-004.4:** The design shall exempt a
  revision of an active task, and shall remove revisions only through the
  existing offline, backup-gated maintenance path with a verified backup and a
  transactional delete, so the current plan document stays readable.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-004.5:** This document shall designate
  REQ-004 as design criteria only. It authorizes no plan-revision removal in this
  package.

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-005: Governance and settings surface

**Intent:** Let administrators understand and enable the periodic cleanup by
extending the existing retention surface, never by creating a second one.

**User story:** As an administrator, I want to see and control the periodic
cleanup in the existing retention card, so I can choose when it runs and what it
costs.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.1:** The capability shall reuse the
  existing `archived_data_retention` settings record, its revision and approval
  state, and the existing `ArchivedDataRetentionCard`. It shall not add a second
  settings record, a second revision stream, or a second settings card. The
  message target's window and eligibility are broadened in place, and enabling
  the capability shall not read, write, or change the `tool_payload_retention`
  record.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.2:** Administrators shall select a
  whole-day window for each target. Valid values are 1-3650 days; the initial
  message-window value is 14 days and the initial cleanup-window value is 7 days.
  Invalid input shall save nothing.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.3:** Before the first mutation the
  administrator shall explicitly choose backup or continue without backup, and a
  pending choice shall arm no deletion. Preparation, the approved-revision gate,
  bounded batches, the daily schedule, and restart behavior shall follow the
  existing retention governance frame.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.4:** An administrator shall be able
  to analyze an unsaved policy while the policy stays disabled, and opening the
  page shall not start an analysis, backup, or reduction. Analysis shall report
  each target separately with rows scanned, rows eligible, rows reduced, stored
  bytes before the pass, bytes removed, oversize rows, and skipped-data counts. A
  partial or failed analysis shall never appear as a complete estimate, and a
  policy edit shall mark a previous result stale.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.5:** Only SQLite shall be supported.
  Other engines shall show an unavailable state and reject analysis and mutation
  requests without side effects. Read-only users shall see status but cannot
  analyze, save, or start a pass.

### REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006: Protection, reporting, and effect claims

**Intent:** Keep the reduction safe for live work and honest about what it
achieves.

**User story:** As an administrator, I want required data preserved and an
accurate result, so I can trust the cleanup.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.1:** Active tasks, sessions that are
  live or need recovery or continuation, messages, task identity, attachments,
  entity references, the session selection, and the current plan shall remain
  usable and unchanged.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.2:** The capability shall report a
  stored-byte reduction distinctly from a file-size reduction, shall point at the
  separate compaction action, shall not run `VACUUM`, and shall not compete with
  writes for the writer during compaction.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.3:** No batch shall run while backup,
  restore, reset, or compaction owns the shared maintenance admission, and reset
  or restore shall stop this worker before it proceeds.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.4:** An unreadable policy or
  preparation record, a schema change under a running pass, or an eligibility
  decision that cannot complete within its statement budget shall fail closed:
  deletion stops, the affected rows are retained, and the pass is reported as
  partial rather than complete.
- **AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.5:** Progress and results shall
  report bounded counts and byte estimates only. No message, task, session,
  revision, or catalog identifier and no metadata content shall appear in a
  metric label, log, or API response. The capability shall add no permanent gate,
  fingerprint, hash, signature, audit, monitor, or ledger.

## Out of scope

- Any change to the Tool Payload Retention policy, its record, window, or
  approved state.
- Message deletion, message content changes, database compaction, index removal,
  and file-size reduction. Compaction remains the separate explicit operator
  action.
- Automatic session-metadata reduction, automatic or offline catalog-revision
  pruning, and plan-revision removal; REQ-003 and REQ-004 are design criteria
  only in this package.
- Delivery inbox and effect retention, restart-midturn repair, Office run
  history, filesystem caches, worktrees, task environments, attachments, and
  provider-side history.
- A new permanent gate, fingerprint, hash, signature, audit, monitor, or ledger,
  and any automatic `VACUUM` competing with writes.
- PostgreSQL support; a PostgreSQL deployment manages retention with its own
  operator tooling.

## Related documents

- [DB History Retention system design](../system-design/db-history-retention.md)
- [Archived Data Retention requirements](archived-data-retention.md)
- [Tool Payload Retention requirements](tool-payload-retention.md)
- [Session model catalog reference decision](../../../decisions/2026-10-05-session-model-catalog-reference.md)
- [Platform bounded storage and database maintenance](../../platform/requirements/pr-watch-and-bounded-storage.md)
