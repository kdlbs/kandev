# ADR-2026-09-03-separate-system-data-storage-pages: Separate System Data and Storage Pages

**Status:** accepted
**Date:** 2026-09-03
**Area:** frontend

## Context

The System `Data & Logs` route combines database status, backups, storage
maintenance, and diagnostic logs. These sections support different operator
tasks.

Storage maintenance is a large workflow. It includes disk capacity, cleanup
actions, policy settings, run history, and quarantine management. On a phone,
the combined route places diagnostic logs many screens below the page entry.

The settings restructure reduced four System rows to one row. Restoring every
old row would make the System navigation dense again.

## Decision

System settings will expose two direct page destinations:

- `Data & Logs` at `/settings/system/data-storage` will contain database
  status, backups, and diagnostic logs.
- `Storage` at `/settings/system/storage` will contain the complete storage
  maintenance workflow.

The existing `Data & Logs` path remains stable. The legacy Database, Backups,
and Logs paths will continue to redirect to that page.

Desktop and phone navigation will show both destinations directly. The split
will not add tabs, drawers, or another navigation level.

## Consequences

- Operators can open storage maintenance without moving through unrelated
  database and backup content.
- Diagnostic logs are no longer placed after the large storage workflow.
- The System navigation gains one row.
- Settings discovery must assign each storage target to the Storage page.
- The Storage route owns its save contributor and dirty-route protection.
- Public operations screenshots must show the new Storage destination.

## Alternatives Considered

- **Keep one combined page.** Rejected because the page combines separate
  tasks and produces excessive phone scrolling.
- **Restore four standalone pages.** Rejected because Database, Backups, and
  Logs remain small enough to share one operational data page.
- **Put Logs with Storage.** Rejected because the storage workflow would still
  place diagnostics after its longest content.
