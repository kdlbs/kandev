---
status: draft
system: system-page
created: 2026-08-27
owners:
  - kandev
---

# Backup Location and Action Guidance Requirements

## Overview

The Backups section shows SQLite snapshots and their maintenance actions.
Operators need the exact snapshot directory and clear meanings for icon-only actions.

The system-page system owns this behavior because it owns the backup maintenance surface.
The UI system supplies the shared tooltip and responsive primitives.

## Terminology

- **Backup directory:** The `backups` directory beside the configured SQLite database file.
- **Row action:** An icon-only control for one snapshot.

## Requirements

### REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001: Backup location and row action guidance

**Intent:** Show operators where Kandev stores snapshots and what each snapshot action does.

**User story:** As an operator, I want exact location and action guidance, so that I can manage snapshots without guessing.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.1:** When SQLite database information is available, the Backups section shall show the full resolved backup directory.
- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.2:** When a custom SQLite path is active, the shown directory shall be the `backups` sibling of that file.
- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.3:** When an authorized user hovers or focuses a row action, the tooltip shall identify only Download, Restore, or Delete. The control's accessible name shall continue to identify the snapshot.
- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.4:** When a user uses a coarse pointer, each visible row action shall remain directly usable through a target of at least 44 pixels.
- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.5:** When a long backup path or snapshot name is shown, the Backups section shall not cause horizontal page scrolling.
- **AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.6:** When database information is unavailable or the driver has no local backup directory, the interface shall not show a guessed filesystem path.

## Out of scope

- Changes to backup creation, retention, restore, download, or delete behavior.
- Changes to the configured database path or backup directory.
- Publication of per-snapshot filesystem paths.
- Changes to PostgreSQL backup support.
- Replacement of path placeholders outside the Backups section description.
