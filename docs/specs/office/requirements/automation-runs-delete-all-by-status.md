---
status: draft
system: office
created: 2026-08-12
owners:
  - clem
---
# Automation runs — status-scoped delete all Requirements

## Overview

The Recent Runs table on an automation's settings page
(`/settings/workspaces/<id>/automations/<id>`) can filter by status (Skipped,
Archived, Cancelled, ...), but the "delete all runs" action always clears
every run for the automation regardless of the active filter. A user who
filters to one status to tidy just those runs cannot delete them as a group:
they must click the per-row delete button once per run. The delete-all
control also sits in the section header above the table, away from the
per-row delete buttons, so its scope is easy to misread.

## Requirements

### REQ-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001: Automation runs — status-scoped delete all

**Intent:** The Recent Runs table on an automation's settings page
(`/settings/workspaces/<id>/automations/<id>`) can filter by status (Skipped, Archived, Cancelled,
...), but the "delete all runs" action always clears every run for the automation regardless of the
active filter. A user who filters to one status to tidy just those runs cannot delete them as a
group: they must click the per-row delete button once per run. The delete-all control also sits in
the section header above the table, away from the per-row delete buttons, so its scope is easy to
misread.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.1:** The delete-all control moves into the **rightmost cell of the Recent Runs table header** — the same column as the per-row delete buttons.
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.2:** Delete-all acts only on the **currently active status view**:
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.3:** Filter "All" → every run of the automation is deleted (the existing single `automation.runs.delete_all` call).
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.4:** Any status filter → exactly the runs shown in that view are deleted; runs of every other status are untouched.
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.5:** The confirmation dialog names the scope: the existing copy when the filter is "All", and a status-named variant (e.g. "Delete all Skipped runs?") when a status filter is active.
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.6:** Deleting runs also deletes their associated tasks — identical to the per-run delete and the existing delete-all today.
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.7:** The delete-all control is hidden when the active view shows no runs.
- **AC-OFFICE-AUTOMATION-RUNS-DELETE-ALL-BY-STATUS-001.8:** A failed delete-all keeps the existing failure behavior: an error toast, then the store reverts to the server's run list (or to the pre-delete snapshot when the recovery refresh also fails).

## Migrated source detail

## Why

The Recent Runs table on an automation's settings page
(`/settings/workspaces/<id>/automations/<id>`) can filter by status (Skipped,
Archived, Cancelled, ...), but the "delete all runs" action always clears
every run for the automation regardless of the active filter. A user who
filters to one status to tidy just those runs cannot delete them as a group:
they must click the per-row delete button once per run. The delete-all
control also sits in the section header above the table, away from the
per-row delete buttons, so its scope is easy to misread.

## What

- The delete-all control moves into the **rightmost cell of the Recent Runs
  table header** — the same column as the per-row delete buttons.
- Delete-all acts only on the **currently active status view**:
  - Filter "All" → every run of the automation is deleted (the existing
    single `automation.runs.delete_all` call).
  - Any status filter → exactly the runs shown in that view are deleted;
    runs of every other status are untouched.
- The confirmation dialog names the scope: the existing copy when the filter
  is "All", and a status-named variant (e.g. "Delete all Skipped runs?") when
  a status filter is active.
- Deleting runs also deletes their associated tasks — identical to the
  per-run delete and the existing delete-all today.
- The delete-all control is hidden when the active view shows no runs.
- A failed delete-all keeps the existing failure behavior: an error toast,
  then the store reverts to the server's run list (or to the pre-delete
  snapshot when the recovery refresh also fails).

## Scenarios

- **GIVEN** the Recent Runs table expanded with runs of mixed statuses and
  the "All" filter active, **WHEN** the user clicks the delete-all button in
  the table header and confirms, **THEN** every run is removed and the table
  shows the no-runs empty state.
- **GIVEN** the "Skipped" filter active with 2 skipped and 1 succeeded run,
  **WHEN** the user clicks delete-all in the table header and confirms,
  **THEN** the 2 skipped runs are removed, the succeeded run remains, and
  switching the filter back to "All" still shows the succeeded run.
- **GIVEN** the "Archived" filter active, **WHEN** the user deletes all,
  **THEN** only runs whose displayed status is Archived are removed.
- **GIVEN** a filter whose view is empty ("No runs match this filter"),
  **WHEN** the user looks at the table header, **THEN** no delete-all control
  is shown.
- **GIVEN** the table is expanded, **WHEN** the user looks at the table
  header, **THEN** the delete-all control sits in the rightmost header cell,
  horizontally aligned with the per-row delete buttons.
- **GIVEN** the delete-all request fails, **WHEN** the user confirms the
  dialog, **THEN** an error toast appears and the table reverts to the
  server's run list.

## Out of scope

- No backend or API changes. The "All" view keeps the single
  `automation.runs.delete_all` call; a filtered view deletes the visible runs
  through the existing per-run `automation.run.delete` path. Filtered
  delete-all therefore covers the runs currently loaded in the table (the
  list is capped at 50 rows), not runs of the same status beyond the load
  window.
- No change to the per-run delete button, the status filter chips, or the
  workspace-wide runs feed (detail rail/drawer).
