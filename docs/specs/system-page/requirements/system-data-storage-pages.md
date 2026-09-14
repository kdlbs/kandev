---
status: active
system: system-page
created: 2026-09-03
owners:
  - kandev
---

# System Data and Storage Pages Requirements

## Overview

The system-page system owns the information architecture for operational data,
storage maintenance, and diagnostics. Operators need focused destinations for
storage work and system data work.

This requirement supersedes the route allocation for Database, Backups, Logs,
and Storage in `REQ-SYSTEM-PAGE-SYSTEM-PAGE-001`. The operational behavior of
those features remains unchanged.

## Terminology

- **Data & Logs:** The page for database status, backups, and diagnostic logs.
- **Storage:** The page for disk analysis, cleanup policy, run history, and
  quarantine management.

## Requirements

### REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001: Focused system data and storage destinations

**Intent:** Operators can open storage maintenance without moving through
unrelated database, backup, and log content.

**User story:** As an operator, I want separate data and storage destinations,
so that I can reach the required maintenance task quickly.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.1:** System settings shall show
  `Data & Logs` and `Storage` as separate page destinations.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.2:** `Data & Logs` shall use
  `/settings/system/data-storage` and show Database, Backups, and Logs.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.3:** `Data & Logs` shall not show
  storage analysis, cleanup, policy, history, or quarantine controls.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.4:** `Storage` shall use
  `/settings/system/storage` and show the complete storage maintenance flow.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.5:** Database, Backups, and Logs
  legacy routes shall continue to redirect to `Data & Logs`.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.6:** Settings discovery shall open
  each database, backup, log, or storage result on its owning page and target.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.7:** Desktop and phone settings
  navigation shall provide a direct entry to both pages with no new nested
  navigation.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.8:** The phone pages shall retain one
  content scroll owner and shall not create document horizontal overflow.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.9:** The route split shall preserve
  storage policy save protection, role gates, loading states, error states,
  and maintenance actions.

## Out of scope

- New backend APIs or storage-maintenance behavior.
- Separate pages for Database, Backups, and Logs.
- A new tab, drawer, or accordion navigation model.
- A new canonical path for `Data & Logs`.
