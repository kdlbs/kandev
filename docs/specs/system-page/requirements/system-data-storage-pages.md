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
  quarantine management, plus Office history retention.

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
  host storage analysis, cleanup, policy, maintenance history, or quarantine controls.
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

### REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002: Header tabs and destination compatibility

**Intent:** Operators can select related maintenance content from the page header.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.1:** Data & Logs shall offer Database and Logs tabs. Database shall contain database status/actions, message compaction, and backups in that order.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2:** Storage shall offer Host and Office retention tabs. Host shall retain the complete existing host-maintenance flow.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.3:** Office retention shall contain retention status followed by its policy. Data & Logs shall no longer contain Office retention.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.4:** Search, legacy links, and retention health links shall open the owning tab and target. Existing canonical page paths shall remain valid.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.5:** Both pages shall use the shared header-tab interaction. Role gates, save protection, loading/error states, and existing maintenance actions shall remain effective.

### REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003: Understandable maintenance controls

**Intent:** Operators can choose retention settings without database-schema knowledge.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.1:** Retention policy shall use Routine history and Agent run history labels. Minimum-kept fields shall identify their routine or agent-profile scope.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.2:** Field explanations shall use focusable hover information icons with tap-open help drawers on touch. Deletion consequences and active-work protection shall remain visible.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.3:** Inputs shall use 28px desktop height and at least 44px touch targets. Routine/agent retention windows and minimums shall remain in the main policy view.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.4:** Advanced settings shall contain cleanup interval, batch limit, and warning thresholds. The event threshold shall explicitly apply only to run events.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.5:** Temporary Kandev files shall identify diagnostic bundles and utility working folders. Copy shall explain inactivity, the 24-hour stale interval, quarantine, and shared-folder exclusions without repeated descriptions.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.6:** The message compaction card shall fill its content row on desktop and phone. Existing analysis, preparation, save, and compaction behavior shall remain available.

### REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004: Accurate retention summary

**Intent:** Operators can assess stored history and the last cleanup without reading database names.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.1:** Status shall show the saved automatic-cleanup state and last known cleanup outcome. An absent result shall say no cleanup is recorded since server startup.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.2:** Stored counts shall distinguish zero, unavailable, and stale measurements for routine history, agent run history, and events. Counts shall not imply deletion eligibility.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.3:** Status shall distinguish per-category preview, committed deletion, backlog, and error outcomes. Mixed preview/deletion outcomes shall not imply that no deletion occurred.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.4:** Warning thresholds shall use saved settings and retain disabled-threshold behavior. A common timestamp shall appear only when all displayed count timestamps match.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.5:** Expandable cleanup details shall retain provider attempts, run skill records, unknown statuses, routine attribution, and skip information. Errors and stale-data notices shall remain visible when details collapse.
- **AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.6:** Desktop shall align count summaries for comparison. Phone shall show a single-column summary with the same data and actions.

## Planning status

The added tab and presentation criteria describe the implemented 2026-09-15
settings package. The two-page split and its tab allocation are current.
The [shared header contract](../../ui/requirements/settings-header-tabs.md) owns reusable tab interactions.
Office retains ownership of [retention semantics](../../office/requirements/run-history-retention-operations.md).

## Out of scope

- New backend APIs or storage-maintenance behavior.
- Separate pages for Database, Backups, and Logs.
- New nested navigation outside the shared page-header tabs.
- A new canonical path for `Data & Logs`.
