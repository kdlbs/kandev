---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-MAINTENANCE-001
---

# Database maintenance command System Design

## Purpose and boundaries

The system-page system owns operator maintenance surfaces. This design covers
the backup-gated retention execution and SQLite compaction command. Candidate
selection queries are owned by the task system's
[bounded session history](../../tasks/requirements/bounded-session-history.md)
design.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-DATABASE-MAINTENANCE-001` | Command surface; Backup gate; Execution; Compaction; Observability |

## Command surface

`kandev maintenance database` (dispatched by
`apps/backend/internal/launcher/maintenance_database.go`) supports `--dry-run`,
explicit retention options, `--backup <path>` or a verified freshly-created
snapshot, `--compact`, and prints per-table candidate/reclaim estimates and
rollback instructions. Dry runs are read-only.

## Backup gate

`apps/backend/internal/maintenance/backup.go` verifies the backup before
destructive work: attribute match and readability. Execution fails closed when
verification fails. The existing startup snapshot boundary is reused as the
fresh-snapshot reference.

## Execution

Selected redundant payloads, Git snapshots, and plan revisions are removed in
transactions; each removal is restricted to the retention-eligible candidate
set. Failure rolls back its transaction and leaves the database usable.

## Compaction

`apps/backend/internal/maintenance/compact.go` stages compaction with
`VACUUM INTO` to a temporary file, validates the result, and atomically
replaces the live database only after success. A failed stage keeps the
original database.

## Observability

The command reports storage gauges (database and WAL sizes, per-table
logical storage) without contents or credentials. Maintenance failures are
surfaced; nothing in the default invocation is destructive. The aggregate
storage and runtime gauges are exposed through the existing debug/status
surfaces with bounded labels.

## Failure and recovery

Rollback steps are printed after execution. A failed or rejected run leaves
the database and its backup untouched.

## Related decisions

- [Install-wide storage maintenance uses typed ownership providers and quarantine](../../../decisions/0045-install-wide-storage-maintenance.md)
