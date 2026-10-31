---
status: draft
system: system-page
created: 2026-10-28
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
owners:
  - kandev
---

# Unified history compaction

## Boundary

System-page owns one administrative history-compaction policy and operation.
Tasks owns the [Git history contract](../../tasks/system-design/turn-change-preservation.md) and supplies typed cleanup operations.
This extends the existing [tool-payload design](tool-payload-retention.md) without introducing another policy.
The [accepted decision](../../../decisions/2026-10-28-git-turn-history-and-unified-compaction.md) records the selected scope.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001 | Policy and surface; Analysis |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002 | Cleanup; Backup and upgrade |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003 | Scheduling and recovery |

## Policy and surface

Expand the existing `tool_payload_retention` record with a versioned combined scope.
Keep the same enabled value, age controls, save/discard flow, manual action, and 24-hour schedule.
Use History compaction as the localized card/action terminology.
There is no additional checkbox, independently saved age, or second cleanup schedule for diffs.
The same controls and outcomes apply on desktop and phone.
Detailed results can distinguish categories without adding policy controls.

Retain SQLite support and existing explicit unsupported-engine behavior during migration.
Missing settings remain disabled. Existing period values are preserved.
Malformed or unapproved scope records block destructive work.
Physical database compaction remains the existing explicit file-maintenance operation.

## Analysis

Use one task-inactivity cutoff and the existing conservative activity predicate.
Enumerate the union of tasks with removable tool bodies or historical content.
Include a task that has old snapshots but no eligible tool-call payloads.
Count each task once and preserve per-category cursors and counts in the same bounded operation record.
Reread policy and activity at every mutation boundary.
Protect pending capture, object transfer, backup, and historical-read leases.

Show tool-body bytes, legacy SQL history bytes, checkpoint counts, and estimated reclaimable Git storage in Analysis details.
Do not add different byte definitions into a misleading physical-disk total.
Objects shared by a retained snapshot or ordinary branch are not uniquely reclaimable.
SQL deletion frees reusable pages. Ref deletion can leave Git objects until later maintenance.
Report actual reclaimed disk bytes only after measurement.

## Cleanup

Reuse `internal/system/toolretention` as the single policy/job owner.
Compose its existing message reducer with task-owned historical-content operations.
Use `internal/system/maintenance` for database maintenance admission and the existing activity coordinator for Git maintenance.
No SQL transaction spans Git I/O.

For each eligible task, preserve conversation prose, metadata, plans, attachments, and turn/file summaries.
Remove supported old tool details and expire eligible historical content under the same cutoff.
Delete expired legacy SQL links and orphan payloads through the existing content-addressed repository rules.
Persist exact source/ref/object cleanup intent before external mutation.
Remove owned refs with expected-OID checks, then acknowledge the receipt.
A crash leaves retryable intent rather than an untracked partially deleted source.

Reclaim unreachable objects only in backend-owned Git stores after rechecking leases and retained references.
Shared user repositories receive owned-ref removal only. Do not force broad GC or branch cleanup there.
Provably owned abandoned capture artifacts can be removed after confirming they have no live owner or lease.
Unknown ownership is a skipped result.

## Backup and upgrade

Reuse the existing single backup-or-skip preparation choice.
For the expanded scope, one verified backup artifact covers SQL state and required checkpoint Git objects.
Use a versioned bundle with the database image, a source manifest, and self-contained Git bundles for recorded checkpoint refs.
Pin source objects while producing and verifying the artifact.
Database verification still uses a read-only native-path connection.
Git verification checks that the recorded accepted objects can be restored without the live repository.

The existing restore flow restores the database and checkpoint sources as one maintenance operation.
Stage and verify source objects before publishing restored source bindings.
Keep old database-only backup formats readable for their original coverage.
A database-only receipt cannot claim protection for Git objects that this operation will delete.

Scope migration preserves the selected period and does not silently enable cleanup.
An existing tool-only approval requires the existing preparation choice once for the combined scope.
This reuses the current flow, with no additional policy or toggle.
Old estimates are stale after the scope version changes.

## Scheduling and recovery

Keep one manual/automatic job and one result, with tool and checkpoint phase details.
Manual work keeps its current priority. Batches remain bounded, cancellable, and contention-aware.
Disabling the policy blocks subsequent batches in every category.
Restart resumes receipts and cursors without double-counting committed work.
Do not let the former fixed-age history sweeper run alongside the combined policy.
Pending physical reclamation is visible and retryable through the same maintenance operation.
The page does not claim that existing backup files shrink after cleanup.

## Verification

Use real database fixtures for history-only tasks, mixed categories, stale estimates, policy changes, and partial failures.
Use real temporary Git stores for shared objects, leases, exact refs, expiry, and recovery after a crash.
Back up a checkpoint containing an uncommitted-only file version, remove the original source, then restore its historical diff.
Desktop and mobile E2E tests verify one set of controls, one analysis, and one compaction outcome covering both categories.
