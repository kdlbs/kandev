---
status: active
system: platform
created: 2026-10-12
owners:
  - kandev
---

# SQLite File Hygiene Requirements

## Overview

A local SQLite install keeps its database, the write-ahead log (WAL), and the
pre-migration backups beside each other in the data directory. Two of those
files can grow or linger without any bound today: the WAL keeps its largest
size for the life of the database, and a pre-migration backup interrupted by a
crash leaves a whole database copy in the backups directory that nothing
removes.

Platform owns this contract because both files belong to the shared SQLite
persistence factory and its boot sequence, not to any feature store. The
backup list, restore, and storage pages that read these directories keep their
own contracts.

## Terminology

- **WAL reset:** The point at which SQLite starts writing the write-ahead log
  from its beginning again after a checkpoint has copied every frame into the
  database.
- **Boot backup staging directory:** The private directory, named with the
  `.kandev-backup-` prefix, in which a pre-migration backup is written before
  it is published under its final name.
- **Abandoned staging directory:** A boot backup staging directory whose
  backup is no longer being written, because the process that owned it exited
  before its cleanup ran.

## Requirements

### REQ-PLATFORM-SQLITE-FILE-HYGIENE-001: Bounded writer WAL file

**Intent:** Keep the WAL file from holding its peak size forever after a write
burst, without changing write durability or checkpoint timing.

**User story:** As an operator of a long-running install, I want the WAL file
to shrink back after a large write burst, so that disk usage reflects the live
data rather than the largest burst ever seen.

#### Acceptance criteria

- **AC-PLATFORM-SQLITE-FILE-HYGIENE-001.1:** When Kandev opens a SQLite writer
  connection, the connection shall report a `journal_size_limit` of 67108864
  bytes (64 MiB). This shall also hold for a connection that the writer pool
  opens to replace a discarded one.
- **AC-PLATFORM-SQLITE-FILE-HYGIENE-001.2:** When a WAL reset occurs, the WAL
  file left on disk shall be no larger than that limit. Between resets, the
  WAL can grow beyond the limit; the limit shall not reject or delay writes and
  shall not change the checkpoint schedule, journal mode, or synchronous mode.
- **AC-PLATFORM-SQLITE-FILE-HYGIENE-001.3:** Read-only SQLite connections and
  PostgreSQL connections shall keep their current settings. A build without
  cgo shall still compile.

### REQ-PLATFORM-SQLITE-FILE-HYGIENE-002: Reclaim abandoned boot backup staging

**Intent:** Remove the database copy that an interrupted pre-migration backup
leaves behind, without ever touching a backup that is still being written or
one that was published.

**User story:** As an operator whose backend was killed during an upgrade
backup, I want the leftover staging copy to be removed automatically, so that a
crash does not permanently cost one extra database's worth of disk.

#### Acceptance criteria

- **AC-PLATFORM-SQLITE-FILE-HYGIENE-002.1:** When the backend starts with a
  SQLite database, Kandev shall remove every boot backup staging directory in
  that database's backups directory whose newest modification time, taken over
  the directory and its direct entries, is more than six hours old. This shall
  happen whether or not the boot takes a new pre-migration backup.
- **AC-PLATFORM-SQLITE-FILE-HYGIENE-002.2:** When Kandev creates a system
  backup (a manual backup or a tool-payload retention backup), it shall apply
  the same removal rule to the backups directory before it creates the new
  backup.
- **AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3:** A boot backup staging directory
  with any entry modified within the last six hours shall remain intact.
  Published backups, `.snapshot-` staging directories, and any other file or
  directory without the `.kandev-backup-` prefix shall not be removed by this
  rule.
- **AC-PLATFORM-SQLITE-FILE-HYGIENE-002.4:** When a staging directory cannot be
  inspected or removed, or the backups directory cannot be listed for a reason
  other than absence, Kandev shall log a warning and shall continue the boot
  or the backup. A failure to reclaim debris shall not fail startup or backup
  creation.

## Out of scope

- Checkpoint scheduling, `wal_autocheckpoint` tuning, `VACUUM`, and database
  compaction.
- Pre-migration backup retention (the last two backups) and its failure
  policy, which stay as described in
  [ADR 0008](../../../decisions/0008-db-upgrade-safety.md).
- `.snapshot-` staging and `.tmp` sidecar sweeps owned by system backups.
- PostgreSQL storage, which has neither a WAL file nor these backups.

## Related documents

- [System design](../system-design/sqlite-file-hygiene.md)
- [Implementation plan](../../../plans/database-hygiene/plan.md)
