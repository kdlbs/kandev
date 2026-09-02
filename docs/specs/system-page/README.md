---
status: draft
system: system-page
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# System page

## Purpose

The system-page system owns operational diagnostics and maintenance surfaces
for inspecting Kandev health, storage, logs, backups, and cleanup.

## Ownership

This system owns system-page information architecture, storage overview and
maintenance operations, diagnostic bundles, and operator-facing maintenance
feedback.

## Exclusions

- Backend diagnostic events belong to the [platform system](../platform/README.md).
- General settings presentation belongs to the [UI system](../ui/README.md).

## Specification map

### Requirements



- [Backup location and action guidance](requirements/backup-location-actions.md)
- [Storage Maintenance](requirements/storage-maintenance.md)
- [Storage maintenance scan, capacity, and dependency cleanup](requirements/storage-overview-parallel-scan.md)
- [System pages](requirements/system-page.md)

### System design



- [Backup location and action guidance](system-design/backup-location-actions.md)
- [Storage Maintenance System Design Part 1](system-design/storage-maintenance-01.md)
- [Storage Maintenance System Design Part 2](system-design/storage-maintenance-02.md)
- [Storage Maintenance System Design Part 3](system-design/storage-maintenance-03.md)
- [System pages System Design Part 1](system-design/system-page-01.md)
- [System pages System Design Part 2](system-design/system-page-02.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Platform](../platform/README.md): supplies health and diagnostics.
- [UI](../ui/README.md): renders the system page.
