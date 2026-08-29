---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
---

# Backup Location and Action Guidance System Design

## Purpose and boundaries

This design adds runtime location and action guidance to the Backups section.
It does not change snapshot storage or the authorization of backup operations.

The backup service remains the owner of snapshot files and operations.
The database service remains the source for the configured SQLite file path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001` | [Location contract](#location-contract), [Row action guidance](#row-action-guidance), [Responsive behavior](#responsive-behavior) |

## Location contract

`database.Service` already derives the backup directory from its configured database path.
For SQLite, `Stats` adds `backup_directory` to `GET /api/v1/system/database`.

The service computes the sibling directory with
`filepath.Join(filepath.Dir(databasePath), "backups")`, then resolves that value
with `filepath.Abs` before returning it. If the absolute-path resolution fails,
the database stats request returns the error. PostgreSQL and other drivers
return an empty value because they have no local SQLite snapshot directory.

The frontend adds `backup_directory` to `DatabaseStats`.
`DataStorageSettings` reads the shared database state after `DatabaseStatsCard` loads it.
The Backups description uses the resolved value and keeps `VACUUM INTO` as an interpolated command value.

The description stays absent until the database response supplies a directory.
The frontend does not reconstruct paths or show `<data-dir>/backups/` as a fallback.

## Row action guidance

`BackupRowActions` keeps the existing Download, Restore, and Delete buttons.
Each button becomes a `TooltipTrigger` with a `TooltipContent` sibling.

The tooltip uses a localized operation-only label from the existing task copy.
It intentionally omits the snapshot name. The button keeps its localized
accessible name, which identifies both the operation and the snapshot.

The tooltip opens on fine-pointer hover and keyboard focus.
The icon button and its accessible name remain the primary interaction contract.

## Responsive behavior

The existing Data and Storage route remains the mobile entry point.
The Backups table remains the visible action surface on mobile.

`queued-ghost-row-actions.tsx` is the nearest shipped coarse-pointer action pattern.
The row actions use pointer-specific minimum dimensions of 44 pixels without changing desktop density.

The tooltip is not required on touch devices.
Each icon button stays visible, named, and directly usable.

Long paths and tooltip labels wrap inside their surfaces.
The existing page and table containment rules prevent document-level horizontal scrolling.

## Failure and recovery

If database statistics fail, the database card shows its existing error.
The Backups heading omits the unverified path instead of showing a placeholder.

Tooltip rendering has no effect on the action handler.
Download, Restore, and Delete keep their existing errors and recovery behavior.

## Security

The database stats route uses the normal authenticated or synthetic install identity.
It already returns the exact SQLite database path to that audience.

The new directory field does not expose the path to a new audience.
The snapshot list and job results remain free of per-snapshot absolute paths.

Row actions and their tooltips remain admin-only.
Member users keep the read-only snapshot list without the Actions column.

## Persistence and observability

This change adds no stored state, event, log, or metric.
The backend derives the directory for each database stats response.

## Test boundaries

- Database package tests prove default, custom, and PostgreSQL directory values.
- Frontend tests prove dynamic description copy, tooltip labels, and accessible names.
- Desktop Playwright covers the resolved path and hover tooltips.
- Mobile Playwright covers 44-pixel actions and horizontal containment.

## Related specifications

- [System pages requirements](../requirements/system-page.md)
- [System pages system design](system-page-01.md)
- [SQLite backup path plan](../../../plans/sqlite-backups-database-path/plan.md)
