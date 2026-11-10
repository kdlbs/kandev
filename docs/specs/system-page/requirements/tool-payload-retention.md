---
status: active
system: system-page
created: 2026-09-14
updated: 2026-10-12
owners:
  - kandev
---

# History compaction requirements

## Overview

Administrators can remove old tool payloads and historical diff content through one compaction policy.
Conversation prose, metadata, and turn/file summaries remain available.
System-page owns the shared maintenance policy and its Data & Logs settings surface.

The policy uses task inactivity and defaults to three calendar months.
Cleanup remains disabled until an administrator completes the preparation choice.

## Terminology

- **Payload:** Tool input or result bodies covered by the supported removal rules.
- **Historical content:** Retained turn checkpoints, derived rendering content, and legacy SQL diff payloads.
- **History compaction:** One cleanup operation covering both eligible tool payloads and historical content.
- **Retained metadata:** Message identity, order, title, tool identity, status,
  timestamps, relationships, and supported result summaries.
- **Inactive task:** A task with no conversation activity within the selected
  period and no active, waiting, idle, starting, or queued work.
- **Payload savings:** The estimated reduction in stored message bytes. This is
  different from a reduction in the database file size.

## Requirements

### REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001: Policy and savings analysis

**Intent:** Let administrators understand and configure cleanup before data removal.

**User story:** As an administrator, I want a savings estimate before I enable
cleanup, so that I can compare storage savings with lost tool details.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.1:** The Data & Logs page at
  `/settings/system/data-storage` shall expose one history-compaction policy for old tool details and historical diff content.
  Missing settings shall remain disabled. Existing age settings shall remain unchanged.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.2:** Administrators shall select
  whole weeks or calendar months. The initial value is three months.
  Valid values are 1–520 weeks or 1–120 months. Invalid input shall save nothing.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.3:** Eligibility shall use the latest
  task conversation activity, not task creation. Activity exactly at the cutoff
  shall remain protected. Active or queued work shall remain protected at any age.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.4:** An administrator shall be able
  to analyze an unsaved policy while cleanup remains disabled. Opening the page
  shall not start an analysis, backup, or cleanup.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.5:** Analysis shall show its period,
  cutoff, timestamp, eligible task and message counts, estimated payload savings,
  and skipped-data counts. Partial or failed analysis shall never appear as a
  complete estimate. Policy edits shall mark previous results as stale.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.6:** The page shall distinguish
  payload savings, reusable database space, and actual file-size reduction.
  It shall explain the separate compaction action and unchanged backup sizes.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.7:** Settings shall survive restart
  and use the route save/discard flow. Read-only users shall see status but cannot
  analyze, change the policy, or start cleanup. The policy covers the installation.

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.8:** One inactivity period, enable switch, analysis action, manual action, and automatic schedule shall cover both content categories.
  Analysis shall include eligible history-only tasks and show a breakdown without separate cleanup controls.

### REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002: Backup and controlled cleanup

**Intent:** Make payload removal deliberate and preserve usable conversations.

**User story:** As an administrator, I want a backup choice and clear removal
rules, so that I understand the first cleanup and its recovery limits.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.1:** Before the first cleanup, the
  page shall explain permanent removal and recommend a new database backup.
  The administrator shall explicitly choose backup or continue without backup.
  Analysis remains optional. No backup choice shall be inferred from silence.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.2:** If backup is selected, cleanup
  shall wait for a completed, verified backup. Failure, cancellation, or an
  interrupted preparation shall block cleanup and expose retry or cancel.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3:** Cleanup shall preserve turn/file summaries and message
  rows, titles, ordering, timestamps, tool identity and status, relationships,
  and supported summaries. It shall preserve human and assistant prose,
  permissions, plans, todos, attachments, and task/session records.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.4:** Conversations shall show an
  explicit removed-payload state instead of empty or loading output. Removed
  details shall not reappear after delayed updates, replay, or task resumption.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.5:** Cleanup shall recheck activity
  and policy before each committed batch. Concurrent activity shall protect a
  task before a later batch starts. Unknown states or unsupported data shall
  remain intact and appear in skipped counts.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.6:** The first cleanup shall run
  after activation preparation. Administrators shall also be able to request
  cleanup now under the saved, enabled policy. Results shall report committed
  removals, skipped rows, remaining work, and failures.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.7:** Backup recovery shall use the
  existing restore process, including recorded checkpoint sources. The page shall explain that restore affects
  the whole database and can replace newer data. No per-message undo is promised.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.8:** Backup verification shall run
  its integrity check on the backup file at its native absolute path through a
  read-only connection. This includes a Windows drive-letter path and a path
  containing a space, `#`, `%`, or, where file names allow it, `?`. Verification
  shall not create or check any other file.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9:** When backup preparation
  fails, the backend shall keep the underlying error message as
  valid UTF-8, cut to at most 1,024 bytes after replacing invalid byte sequences,
  in the stored retention record. The backend shall log the original error
  only after committing the failure for the matching policy revision and
  operation. Cancellation, stale completions, and failed state writes shall
  not produce a current-preparation failure log. The status response
  shall still report only the failure code. A later successful preparation, a
  disable, a new pending preparation, or an interrupted preparation shall
  clear the stored message.

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.10:** For the same eligible task cohort, cleanup shall remove supported tool details and expire historical diff content.
  Retained turn/file summaries shall explain expiry. User branches, unrelated refs, plans, and attachments shall remain intact.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.11:** Pending captures, preservation jobs, and historical-read or backup leases shall prevent conflicting content removal.
  Interrupted cross-store cleanup shall resume without losing ownership or repeating committed removal counts.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.12:** A selected pre-compaction backup shall cover all historical content the operation can remove, including required Git objects.
  Old tool-only approvals shall require the existing preparation choice for the expanded scope. No additional policy or enable switch shall appear.

### REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003: Background operation and access

**Intent:** Keep the policy effective without creating another startup bottleneck.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.1:** An enabled policy shall check
  for eligible payloads daily. Preparation and scheduled cleanup shall not block
  backend readiness. Only one tool-payload operation shall execute at a time.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.2:** Analysis and cleanup shall use
  bounded work with progress and cancellation. They shall release database
  resources between batches and stop on sustained database contention.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.3:** Disabling the policy shall
  prevent subsequent cleanup batches after the save commits. Completed removals
  remain removed. Restart shall preserve committed progress without double counts.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.4:** The page shall show preparation,
  running, completed, partial, failed, and never-run states, plus the next scheduled
  check. Unreadable policy or preparation state shall block deletion.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.5:** Desktop and phone users shall
  have the same actions and outcomes. Phone controls shall have 44px touch targets,
  one page scroll owner, and no horizontal overflow. All copy shall be localized.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.6:** The first version shall support
  SQLite. Other database engines shall show an unavailable state and reject
  analysis and mutation requests without side effects.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.7:** After a status request fails,
  the next successful background status request shall clear that request error
  without a manual refresh. Saved policy and unsaved edits shall remain intact.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.8:** A successful status request
  shall not dismiss an unresolved user-action error or a recorded failed operation.
  Recovery shall not retry mutations or enable compaction automatically.
- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.9:** A failed status read shall say
  that current status is unavailable while retaining the last known analysis
  and its timestamp. It shall not describe a still-running analysis as a failed
  operation. A persisted failed operation and a failed user action shall retain
  their distinct failure feedback.

- **AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.10:** The shared operation shall report tool removals, history expiry, pending reclamation, and failures together.
  No independent fixed-age turn-history cleanup schedule shall compete with the selected compaction policy.

## Out of scope

- Compression, cold storage, per-message restore, and removal from external
  provider histories or existing backups.
- Automatic database-file compaction, tool-message deletion, index removal, and attachment cleanup.
- Changes to the independent Office run-history or filesystem retention policies.

## Related documents

- [Tool-payload implementation design](../system-design/tool-payload-retention.md)
- [Unified history compaction design](../system-design/history-compaction.md)
- [Git history delivery package](../../../plans/turn-change-preservation-retry/plan.md)
- [Implementation plan](../../../plans/tool-payload-retention/plan.md)
- [Preparation failure diagnostics plan](../../../plans/database-hygiene/plan.md)
- [Vacuum and status recovery fix](../../../plans/vacuum-compaction-status/plan.md)
- [Database statistics resilience plan](../../../plans/database-statistics-resilience/plan.md)
