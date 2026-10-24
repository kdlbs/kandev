---
status: active
system: platform
created: 2026-10-12
owners:
  - kandev
---

# SQLite snapshot copy requirements

## Ownership

Platform owns consistent SQLite snapshots shared by startup, backup, reset,
maintenance, repair and legacy database adoption. Physical compaction is a
separate maintenance operation.

## Requirements

### REQ-PLATFORM-SQLITE-SNAPSHOT-001: Consistent backups without mandatory compaction

**Intent:** Protect large databases without rebuilding tables and indexes for every backup.

#### Acceptance criteria

- **AC-PLATFORM-SQLITE-SNAPSHOT-001.1:** Backup-only operations shall copy a transactionally consistent SQLite image, including committed WAL content, without requiring physical compaction. Supported file and in-memory sources shall preserve schema and data.
- **AC-PLATFORM-SQLITE-SNAPSHOT-001.2:** Copy work shall execute in bounded increments, observe cancellation between increments and contention retries, and release all acquired resources after success or failure. Online backup shall allow ordinary source writes between increments.

  For a WAL source, independent committed writes must not continually restart the
  copy. The copied view remains consistent while those writers proceed. Returning
  a borrowed source connection must not leave a copy-owned read transaction open
  or end a caller-owned transaction.
- **AC-PLATFORM-SQLITE-SNAPSHOT-001.3:** Backup copying shall complete before publication. Existing full integrity verification, private staging and atomic installation gates shall remain effective, including required upgrade and manual backups. Failure shall preserve existing destinations and the source, and remove only owned partial artifacts.
- **AC-PLATFORM-SQLITE-SNAPSHOT-001.4:** Required upgrade backups shall retain version eligibility, retention and pre-migration ordering. Failure shall block migrations as required by AC-PLATFORM-STARTUP-LIFECYCLE-001.7. Explicit physical compaction shall still reclaim free pages through its established maintenance workflow.

## Exclusions

Backup reuse, retention registry changes, secure erasure, PostgreSQL backup automation,
and automatic live physical compaction are excluded. Page-copy artifacts may include free pages.

## Design and delivery

- [Snapshot copy design](../system-design/sqlite-snapshot-copy.md)
- [Implementation plan](../../../plans/database-maintenance-throughput/plan.md)
