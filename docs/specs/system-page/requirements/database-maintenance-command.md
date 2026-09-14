---
status: active
system: system-page
created: 2026-09-14
owners:
  - kandev
---

# Database maintenance command Requirements

## Overview

Self-hosted operators need a safe way to reclaim bounded-storage candidates
and compact SQLite after the retention data model lands. Destructive work
must never run without a verified backup, and the operator must always see
what would be reclaimed before anything is removed.

The system-page system owns operator-facing maintenance surfaces. The
retention candidate model it consumes is owned by the
[task system](../tasks/requirements/bounded-session-history.md).

## Requirements

### REQ-SYSTEM-PAGE-DATABASE-MAINTENANCE-001: Backup-gated database maintenance command

**Intent:** The native maintenance database command shall report, execute,
and roll back bounded-storage cleanup only behind a verified backup, never
silently deleting conversation history.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATABASE-MAINTENANCE-001.1:** The command shall support a
  dry run and explicit retention settings, and report per-table candidate and
  reclaim estimates without contents or credentials.
- **AC-SYSTEM-PAGE-DATABASE-MAINTENANCE-001.2:** Destructive execution shall
  fail closed when a verified backup is absent; the verified backup may be an
  operator-supplied path or a freshly-created snapshot.
- **AC-SYSTEM-PAGE-DATABASE-MAINTENANCE-001.3:** Execution shall remove only
  selected redundant payloads, snapshots, and plan revisions inside
  transactions; compaction shall stage and validate its result
  (`VACUUM INTO` or equivalent) and atomically replace only after success.
- **AC-SYSTEM-PAGE-DATABASE-MAINTENANCE-001.4:** The command shall print
  SQLite/WAL and table-storage measurements and rollback steps, and shall
  not include database contents, credentials, or destructive default
  behavior.
- **AC-SYSTEM-PAGE-DATABASE-MAINTENANCE-001.5:** Upgrade shall not silently
  delete task history; the command's retention is operator-selected and
  backup-gated, and public operations documentation shall describe backup of
  `kandev.db` and `master.key`, image upgrade, migration and maintenance dry
  run, execute and compact, rollback, and post-release health checks.

## Out of scope

- Retention candidate selection semantics, owned by the
  [task system](../tasks/requirements/bounded-session-history.md).
- Scheduled storage maintenance, covered by
  [storage maintenance](storage-maintenance.md).
