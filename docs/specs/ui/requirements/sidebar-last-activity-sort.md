---
status: draft
system: ui
created: 2026-08-17
updated: 2026-08-17
owners:
  - kandev
---
# Sidebar Last Activity Sort Requirements

## Overview

Saved sidebar views can sort tasks by **Updated**. That value currently follows task-status summary freshness. Background pull-request updates can refresh many idle rows together, so this order does not answer which tasks a user or agent worked on most recently.

## Requirements

### REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-001: Sidebar Last Activity Sort

**Intent:** Saved sidebar views can sort tasks by **Updated**. That value currently follows task-status summary freshness. Background pull-request updates can refresh many idle rows together, so this order does not answer which tasks a user or agent worked on most recently.

#### Acceptance criteria

- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.1:** Desktop and mobile sidebar view editors offer **Last activity** as a sort option.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.2:** The saved-view wire key is `lastActivityAt`. The key persists in saved views and in-flight drafts through user settings, boot hydration, and live settings updates.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.3:** Ascending order places the oldest activity first. Descending order places the newest activity first.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.4:** Equal timestamps retain the existing input order. Sorting does not introduce row movement for a tie.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.5:** The existing `updatedAt` key and **Updated** label remain valid. Existing saved views keep their current behavior.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.6:** The built-in view and new-view defaults do not change.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.7:** When a view sorts by **Last activity**, each row displays that activity time. Other sort modes retain the existing row time behavior.
- **AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.8:** **GIVEN** several idle tasks receive pull-request status refreshes, **WHEN** a view sorts by **Last activity**, **THEN** their order and displayed activity times do not change.

## Migrated source detail

## Why

Saved sidebar views can sort tasks by **Updated**. That value currently follows
task-status summary freshness. Background pull-request updates can refresh many
idle rows together, so this order does not answer which tasks a user or agent
worked on most recently.

Users need a separate **Last activity** option that remains stable during
background status refreshes.

## What

- Desktop and mobile sidebar view editors offer **Last activity** as a sort
  option.
- The saved-view wire key is `lastActivityAt`. The key persists in saved views
  and in-flight drafts through user settings, boot hydration, and live settings
  updates.
- Ascending order places the oldest activity first. Descending order places the
  newest activity first.
- Equal timestamps retain the existing input order. Sorting does not introduce
  row movement for a tie.
- The existing `updatedAt` key and **Updated** label remain valid. Existing
  saved views keep their current behavior.
- The built-in view and new-view defaults do not change.
- When a view sorts by **Last activity**, each row displays that activity time.
  Other sort modes retain the existing row time behavior.

Decision:
[ADR-2026-08-17-separate-task-activity-from-summary-freshness](../../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md).

## Activity definition

`last_activity_at` is the greatest known timestamp from these durable actions:

| Source | Activity time |
|---|---|
| Task creation | `tasks.created_at` |
| Persisted task mutation | `tasks.updated_at` |
| User-authored prompt, including a queued prompt | message `created_at` |
| Agent turn starts | turn `started_at` |
| Agent turn completes | turn `completed_at` |

The following events do not advance activity:

- opening, focusing, or subscribing to a task or session
- Git or pull-request polling and reconciliation
- queued-prompt count changes and other queue bookkeeping
- status-summary rebuilds or transport delivery
- session metadata maintenance without a user prompt or agent turn
- streamed agent chunks inside an already-started turn

A task-state or workflow-placement change counts because it is a persisted task
mutation. A foreground-activity publication that carries an unchanged task
timestamp does not advance activity.

## Data model and derivation

- `TaskStatusSummary` gains optional semantic field
  `last_activity_at: RFC3339 timestamp`.
- `revision` and `updated_at` keep their existing meanings: task-local summary
  version and projection freshness.
- A change to `last_activity_at` is semantic. It advances the summary revision
  and uses the existing complete-replacement event.
- Live projection consumes both `task.updated` and `task.state_changed`, because
  some persisted state transitions publish only the latter. It takes the
  monotonic maximum of the stored value and a source event timestamp. An older
  replay cannot move activity backward.
- Initial load and repair use one batched query for all requested task IDs. The
  query combines task, user-message, and turn timestamps without one query per
  task.
- Repair updates older summaries that lack the field. It preserves a stored
  activity time that is newer than the rebuilt durable maximum.
- A missing summary or an older backend falls back to task `updated_at`, then
  task `created_at`. The row remains sortable during partial recovery.

## API and persistence

The existing task surfaces carry the additive field:

- boot state and task-list snapshots use `status_summary.last_activity_at`
- `task.status_summary.updated` uses the same complete summary payload
- sidebar views and drafts persist sort key `lastActivityAt` through the
  existing user-settings JSON contract

No new endpoint or WebSocket action is required. The summary remains bounded
and contains no transcript, file, or provider payload.

## Provider refresh stability

Equivalent GitHub status snapshots from the REST feedback path and the batched
GraphQL poller must produce one canonical task-PR state. A semantic no-op does
not emit `github.task_pr.updated`.

GitHub sync diagnostics record the names of changed semantic fields without
recording field values or provider content. This evidence makes future
cross-path oscillation visible without exposing sensitive data.

Provider changes can advance summary `updated_at`. They never advance
`last_activity_at`.

## Responsive behavior

Desktop uses the existing Tasks section view editor. Phones use the existing
task-switcher drawer and its shared view editor. Both surfaces use the same
sort key, comparator, draft, and saved setting.

The option stays inside the existing sort selector. It adds no mobile route,
drawer, popover, or scroll owner. The current task list remains the only
vertical scroll body.

## Failure modes

- If activity backfill fails, Kandev keeps the last valid summary and uses the
  frontend fallback. A later list load or source event retries convergence.
- If a summary event is dropped, normal task-list hydration restores the
  complete value.
- If user-settings sync fails, the existing saved-view rollback and error
  treatment restores the previous view and draft.
- An unknown stored sort key keeps the existing migration fallback to Status.
  `lastActivityAt` is added to the known-key set before it is offered in the
  editor.

## Scenarios

- **GIVEN** several idle tasks receive pull-request status refreshes, **WHEN** a
  view sorts by **Last activity**, **THEN** their order and displayed activity
  times do not change.
- **GIVEN** a task with an older summary refresh but a newer user prompt,
  **WHEN** the view sorts descending by **Last activity**, **THEN** that task
  appears before a task with newer provider status but older activity.
- **GIVEN** a user sends a queued prompt, **WHEN** the prompt is stored,
  **THEN** the task activity advances before the agent begins that turn.
- **GIVEN** an agent begins and completes a turn, **WHEN** each milestone is
  projected, **THEN** activity advances at start and again at completion but
  not for every streamed chunk.
- **GIVEN** an existing saved view sorted by **Updated**, **WHEN** Kandev is
  upgraded, **THEN** the view keeps that sort and its current order.
- **GIVEN** a saved view sorted by **Last activity**, **WHEN** the page reloads,
  **THEN** the selection, direction, order, and row timestamps persist.
- **GIVEN** a phone viewport, **WHEN** the user selects **Last activity** in the
  task-switcher drawer, **THEN** the same order is applied and the selector
  remains touch-reachable and viewport-contained.

## Out of scope

- Replacing or renaming the existing **Updated** sort.
- Changing the built-in view or new-view defaults.
- Sorting the Kanban board, Office task list, command panel, or quick-chat tabs.
- Treating passive focus, provider polling, or raw stream chunks as activity.
- Adding a general task activity feed or audit log.

## Implementation plan

- [Sidebar Last Activity Sort](../../../plans/sidebar-last-activity-sort/plan.md)
