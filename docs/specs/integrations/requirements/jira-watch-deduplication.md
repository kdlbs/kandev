---
status: active
system: integrations
created: 2026-11-10
owners:
  - kandev
---

# Jira watch deduplication

## Overview

Jira watches create tasks from matching issues. Deduplication must protect existing tasks and permit recovery after permanent task deletion.

Integrations owns this contract because each watch owns its issue mapping. Tasks owns task existence, archival, and deletion.

## Terminology

- **Mapping:** A watch's association between an issue and a created task.
- **Unfinished reservation:** A claim for an issue whose task creation has not completed with an assigned task identity.
- **Eligible result:** An issue returned by an enabled watch's due search, subject to existing routing and capacity rules.

## Requirements

### REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001: Task-aware duplicate prevention

**Intent:** A watch must recover matching work after permanent deletion without creating duplicates of existing tasks.

#### Acceptance criteria

- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.1:** When a mapped task is permanently deleted, its issue shall become eligible for creation again upon its next eligible result.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.2:** While a mapped task exists, its watch shall prevent duplicate creation for that issue. This includes archived tasks and tasks on completed steps.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.3:** An unfinished reservation shall prevent duplicate creation until an existing release, reset, or watch deletion flow removes it.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.4:** Concurrent deliveries for the same watch and issue shall permit at most one successful claim, including after a mapped task disappears.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.5:** Each watch shall evaluate its own mapping independently. A missing task in one watch shall not invalidate another watch's existing task or unfinished reservation.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.6:** Existing mappings to missing tasks shall recover through ordinary eligible polling without a watch reset or manual database changes.
- **AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.7:** If deduplication state cannot be read or reclaimed reliably, that delivery shall create no task. A later eligible poll shall remain able to retry.

## Compatibility and exclusions

Watch queries, result windows, polling intervals, routing, capacity, and automatic start retain their current contracts. Each watch retains its separate issue identity.

This capability adds no global duplicate rule across watches. It does not recover unfinished reservations through age or inferred task metadata.

Provider writes, live data repair, watch resets, deployment, and other integrations are outside this capability. No rendered UI or new user-facing copy is required.

## Related artifacts

- [System design](../system-design/jira-watch-deduplication.md)
- [Implementation plan](../../../plans/jira-watch-deleted-task-recovery/plan.md)
