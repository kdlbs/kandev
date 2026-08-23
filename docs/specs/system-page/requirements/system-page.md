---
status: draft
system: system-page
created: 2026-05-18
owners:
  - tbd
---
# System pages Requirements

## Overview

Today, settings-style operational concerns (health, disk usage, current version, OSS attribution, log access, database maintenance) have no home in kandev's UI. The existing settings sidebar is product-configuration only (executors, agents, integrations); the only system-shaped entry — Changelog — sits awkwardly inside it. When something goes wrong, users have no path inside the app to see "where is my database, how big is it, what version am I on, is there an update, where are the logs." This forces them into the filesystem and into GitHub, and it makes recoverable problems (bloated SQLite, corrupt state) look unrecoverable.

Radarr and Sonarr solve this with a dedicated **System** area: a group of read-only diagnostic pages plus a small number of gated maintenance actions. This spec brings the same shape to kandev, scoped to the kandev-specific surface (database diagnostics, SQLite backups/maintenance, worktrees, GitHub releases, and daily backend log files).

## Requirements

### REQ-SYSTEM-PAGE-SYSTEM-PAGE-001: System pages

**Intent:** Today, settings-style operational concerns (health, disk usage, current version, OSS
attribution, log access, database maintenance) have no home in kandev's UI. The existing settings
sidebar is product-configuration only (executors, agents, integrations); the only system-shaped
entry — Changelog — sits awkwardly inside it. When something goes wrong, users have no path inside
the app to see "where is my database, how big is it, what version am I on, is there an update, where
are the logs." This forces them into the filesystem and into GitHub, and it makes recoverable
problems (bloated SQLite, corrupt state) look unrecoverable. Radarr and Sonarr solve this with a
dedicated **System** area: a group of read-only diagnostic pages plus a small number of gated
maintenance actions. This spec brings the same shape to kandev, scoped to the kandev-specific
surface (database diagnostics, SQLite backups/maintenance, worktrees, GitHub releases, and daily
backend log files).

#### Acceptance criteria

- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.1:** **Health issues** card: renders the existing `GET /api/v1/system/health` payload (warning/error/info issues with messages and links).
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.2:** **Disk usage** card: shows total kandev data footprint with a per-subdirectory breakdown (`data dir / worktrees / repos / sessions / tasks / quick-chat / backups`), an "as of HH:MM" timestamp, and a **Refresh** button. The disk walk is lazy and asynchronous: the first visit after a cold start (or after the 2h cache expires) returns `null` immediately and the page shows a loading spinner while the walk runs in the background; subsequent visits within 2h return the cached value instantly.
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.3:** **Version + update** card: short summary of current version and "update available" badge, with a CTA to the Updates page. 2. **Feature Toggles** — `/settings/system/feature-toggles`
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.4:** Install-wide runtime flags, risk metadata, environment locks, and restart handling.
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.5:** The full contract lives in [Feature Toggles](../../platform/requirements/feature-toggles.md). 3. **Database** — `/settings/system/database`
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.6:** Read-only: database driver, database size, and schema version. SQLite additionally shows the configured file path, WAL size, and last-backup timestamp from the sibling `backups/` directory. Postgres shows database-level size and does not render SQLite file/WAL/backup rows.
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.7:** SQLite-only **VACUUM** button — safe, reclaims space; shows progress and final size delta.
- **AC-SYSTEM-PAGE-SYSTEM-PAGE-001.8:** SQLite-only **Optimize** button — runs `PRAGMA optimize`; shows progress.

## System design

The migrated technical source is split into [part 1](../system-design/system-page-01.md), [part 2](../system-design/system-page-02.md).
