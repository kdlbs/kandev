# Separate SQLite backups from compaction

**Status:** accepted
**Date:** 2026-10-12
**Area:** platform persistence

## Context

ADR [0008](0008-db-upgrade-safety.md) chose VACUUM INTO for pre-migration snapshots
when expected databases were small. A measured 9.41 GB upgrade snapshot took 818.4
seconds including validation. VACUUM rebuilds the database while making the copy.
Backup safety does not require rebuilding its tables and indexes every upgrade.

An isolated experiment on a different retained 6,869,843,968-byte snapshot used the
already installed go-sqlite3 Backup API: copy 32.677 seconds, full integrity_check
69.165 seconds, total 101.844 seconds, result ok. Source was read-only and temporary
output was removed. These different inputs do not establish a same-input speedup.

## Decision

Use incremental native page copy for backup-only consumers. Retain VACUUM INTO for
explicit physical compaction. Keep full integrity validation, private staging,
atomic installation, version-based backup eligibility and two-snapshot retention.
This supersedes only ADR 0008's backup mechanism.

Use the existing driver and shared persistence boundary. Reject raw live file copies,
which do not establish WAL consistency, and skipping integrity validation, which
would weaken upgrade safety. Backup reuse requires a separate attempt/retention design.

## Consequences

Backup avoids compulsory table/index rebuilding and supports bounded cancellation.
It still copies the database and may preserve free pages; full integrity validation
can dominate runtime. Disk space and write contention still affect duration. Explicit
compaction remains an independent, potentially expensive operation.

## References

- [Requirements](../specs/platform/requirements/sqlite-snapshot-copy.md)
- [Design](../specs/platform/system-design/sqlite-snapshot-copy.md)
- [Plan](../plans/database-maintenance-throughput/plan.md)
- [SQLite VACUUM documentation](https://www.sqlite.org/lang_vacuum.html)
- [SQLite online backup API](https://www.sqlite.org/backup.html)
