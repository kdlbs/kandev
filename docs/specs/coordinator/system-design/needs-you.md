---
id: coordinator-needs-you-design
title: Needs you and Queue design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
requirements:
  - REQ-COORDINATOR-NEEDS-YOU-001
  - REQ-COORDINATOR-NEEDS-YOU-002
  - REQ-COORDINATOR-NEEDS-YOU-003
  - REQ-COORDINATOR-NEEDS-YOU-004
  - REQ-COORDINATOR-NEEDS-YOU-005
  - REQ-COORDINATOR-NEEDS-YOU-006
  - REQ-COORDINATOR-NEEDS-YOU-007
  - REQ-COORDINATOR-NEEDS-YOU-008
---

# Needs you and Queue System Design

## Purpose and boundaries

Needs you and Queue are a client projection over three inputs: the workspace's
tasks as the web client already holds them, the coordinator's stall records
and its open proposals. The task system owns `statusSummary`, `task.stalled`
and task state; this design consumes them and adds no task field.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-NEEDS-YOU-001` | [Classification](#classification) |
| `REQ-COORDINATOR-NEEDS-YOU-002` | [Classification](#classification), [Screens](#screens) |
| `REQ-COORDINATOR-NEEDS-YOU-003` | [Screens](#screens) |
| `REQ-COORDINATOR-NEEDS-YOU-004` | [Classification](#classification), [Screens](#screens) |
| `REQ-COORDINATOR-NEEDS-YOU-005` | [Stall records](#stall-records) |
| `REQ-COORDINATOR-NEEDS-YOU-006` | [Routes and sidebar](#routes-and-sidebar) |
| `REQ-COORDINATOR-NEEDS-YOU-007` | [Failure and recovery](#failure-and-recovery) |
| `REQ-COORDINATOR-NEEDS-YOU-008` | [Screens](#screens) |

## Inputs

- **Tasks:** `useAllWorkflowSnapshots`
  (`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`), stored in
  `kanbanMulti.snapshots`, kept current by `task.status_summary.updated` and the
  task WebSocket handlers. Snapshots hold every open task, so there is no
  paging cap. Each task carries `statusSummary`
  (`apps/web/lib/types/task-status-summary.ts`): `pending_action` (the
  string `"clarification"` or `"permission"` when set), `active_error` and
  `task_error` (each `TaskStatusSummaryActiveError`, text in `preview`),
  `pull_request.aggregate_state`, `last_activity_at`, `primary_session`
  (`{id, state}` or null), and the task `state`.
- **Pull request detail:** the GitHub slice's `taskPRs.byTaskId`
  (`apps/web/lib/state/slices/github/types.ts`), loaded by `useWorkspacePRs`
  (`apps/web/hooks/domains/github/use-task-pr.ts`, which calls
  `listWorkspaceTaskPRs`) and kept current by the existing PR WebSocket
  handlers. The row uses the task's primary PR (`getPrimaryTaskPR`): `state`,
  `unresolved_review_threads` and `checks_state`. This design adds no PR
  field.
- **Stall records:** `GET /api/v1/workspaces/:id/coordinator-stalls`
  (`workspace.read`), returning `{stalls: [{task_id, stalled_for_ms,
  last_event_at, detected_at}]}` ordered by `task_id`. Re-read on each screen
  mount, on **Try again** and on the `task.stalled`-driven
  `coordinator.updated` event (below).
- **Proposals:** the proposal store of [proposals](proposals.md), fed by
  `GET .../coordinators/:cid/proposals?status=pending`.

## Classification

`apps/web/lib/coordinator/attention.ts` exports a pure function
`classify(tasks, stalls, proposals, now)` returning `{needsYou: Item[],
queue: {working, inReview, readyToMerge, done, other}, counts}`. It is covered
by Vitest, one case per group, one per precedence pair, the equal-timestamp
stall case, the unreadable-session cases with and without a stall row, and
the mixed-validity case below (unreadable session, `state == "COMPLETED"`).

Rules, first match wins, excluding tasks with `archived_at` set or
`is_ephemeral` true:

| Group | Rule |
| --- | --- |
| question or permission | `pending_action` is `"clarification"` or `"permission"` |
| stall | a stall row for the task and (`last_activity_at` absent or `last_activity_at <= detected_at`) |
| error | `active_error` or `task_error` present |
| Ready to merge | `pull_request.aggregate_state == "ready"` |
| In review | `pull_request.aggregate_state == "awaiting_review"` |
| Working | `primary_session.state` in `RUNNING`, `STARTING` |
| Done | task `state == "COMPLETED"` |
| Other | otherwise, including a task whose session is unreadable |

A task's session is **unreadable** when its `statusSummary` is absent, or
`primary_session` is present with no `state`. The rules are still evaluated in
order for it, with every `statusSummary`-derived field read as absent: the
question, error, pull request and Working rules cannot match, since each
reads a `statusSummary` field (`pending_action`, `active_error`/`task_error`,
`pull_request`, `primary_session.state`). The Done rule is not one of these:
it reads the task's own `state` column, which exists independently of the
session and is never absent, so Done still matches a task whose session is
unreadable. The stall rule matches when a stall row exists, because an absent
`last_activity_at` counts the stall as current
(`AC-COORDINATOR-NEEDS-YOU-001.4`), so such a task is a stall item. A task
with an unreadable session, no stall row and `state != "COMPLETED"` lands in
Other, and its row shows "position underivable: session unreadable" in place
of the agent state. A task with `primary_session` null has no session, which
is not unreadable: its row shows "No session".

Vitest covers the mixed-validity case: an unreadable session
(`statusSummary` absent) with `state == "COMPLETED"` classifies as Done, not
Other.

Each open proposal is an item of kind `proposal`, independent of tasks.

Item reference time: proposal `created_at`; stall `last_event_at`; otherwise
`last_activity_at`, falling back to the task's `updated_at`. Needs you sorts
by reference time ascending, then kind rank (proposal 0, question 1, stall 2,
error 3), then id by code-unit comparison. Queue groups sort by
`last_activity_at` descending with absent values last, then task id
ascending. Age is rendered from `now`, re-evaluated every 30 seconds by the
screen's timer.

The error why-text is "The agent reported an error: " plus
`active_error.preview` truncated to 140 characters when `active_error` is
present (an empty preview leaves the text ending after the colon and space).
With only `task_error` it is exactly "The task failed"; `task_error.preview`
is not shown on the item (**Open task** shows it on the task), matching
`AC-COORDINATOR-NEEDS-YOU-002.2`.

In review and Ready to merge rows show the primary PR's `state`,
`unresolved_review_threads` and `checks_state`. When `taskPRs.byTaskId` holds
no PR for the task (PR data not loaded, failed to load, or the provider is not
GitHub), the row shows the `pull_request.state` from `statusSummary` and the
text "PR detail unavailable" in place of the thread count and CI state.

A proposal item's head uses the source task when it is present in the
snapshots and not archived, else "New task" with no step. `Ask about this`
uses the source task identifier, else the proposal title.

## Stall records

Table `coordinator_stalls` in the coordinator store:

| Column | Type | Notes |
| --- | --- | --- |
| `task_id` | text primary key | |
| `workspace_id` | text not null | indexed |
| `stalled_for_ms` | integer not null | parsed from the event's Go duration string |
| `last_event_at` | timestamp not null | from the event |
| `detected_at` | timestamp not null | the subscriber's receive time, UTC |

The `task.stalled` subscriber (`internal/coordinator/stalls.go`) reads
`task_id`, `workspace_id`, `stalled_for` and `last_event_at` from the payload.
It checks for at least one coordinator in the workspace; with none it returns.
Otherwise it upserts the row with `INSERT ... ON CONFLICT (task_id) DO UPDATE
SET stalled_for_ms = excluded.stalled_for_ms, last_event_at =
excluded.last_event_at, detected_at = excluded.detected_at, workspace_id =
excluded.workspace_id WHERE excluded.last_event_at >
coordinator_stalls.last_event_at` (valid on SQLite and PostgreSQL). A newer
stall episode replaces the row; an event with an equal or earlier
`last_event_at` (a redelivery, or events handled out of order) changes
nothing, so detection time never moves forward for an episode already
recorded and a late event never overwrites a newer one. Only when a row was
inserted or updated (one affected row) does it publish `coordinator.updated`
for each coordinator of the workspace, so open screens re-read stalls. `detection_only` is ignored: main publishes it as
`true` on every event, and recording never acts. A payload that fails to parse
is logged at warn and dropped.

The startup pass deletes rows whose task is missing or archived (one query
joining the task table through the task repository's read interface) and rows
with `detected_at` older than 30 days. There is no timer.

## Routes and sidebar

- `apps/web/src/spa-routes.tsx` adds `/workspaces/:id/coordinator`,
  `/workspaces/:id/coordinator/:coordinatorId` and `.../queue`, resolved by a
  flag-gated resolver modelled on `resolveNeedsYouInboxRoute`; the pages live in
  `apps/web/app/coordinator/`.
- The generic route redirects to the first coordinator by the list order, or
  renders the no-coordinator state. An unknown id renders the same state with a
  link to the settings list.
- `components/app-sidebar/app-sidebar-primary-nav.tsx` renders the entries
  inside `AppSidebarFixedNav` after the Inbox row and before the
  collapsed-sidebar Quick Chat row (so before New Task), using
  `AppSidebarNavItem` with its badge; `MobileRequiredRows` in
  `components/navigation/mobile-sidebar-layout-navigation.tsx` renders the same
  rows. Badge values come from a small store keyed by coordinator id, seeded by
  the list route's `open_proposals` field and replaced by each
  `coordinator.updated` payload.

## Screens

- `apps/web/app/coordinator/` holds the Needs you and Queue pages, the header
  (name or selector, **Configure** for managers), the count strip (sticky,
  outside the scroller), the item card with per-kind actions, the Queue groups
  (Done and Other use a collapsed disclosure), and the empty, missing and error
  states.
- Phone: one column; the strip stays sticky; actions wrap with 44px minimum
  targets. Playwright checks run in the `mobile-chrome` project at 390px, and an
  axe scan asserts no critical violation on both screens.
- Copy uses `t()` in six locales; no Unicode em dash.

## Failure and recovery

Each input keeps its last successful value with its load time. A failed read
after a successful one shows "Could not load this workspace's tasks. Showing
what was loaded at <time>." with the oldest load time among the failed inputs.
A failed first load (no successful value yet for that input) shows "Could not
load this workspace's tasks." with no time; the lists render from the inputs
that did load. **Try again** re-issues only the failed reads.

The tasks input is read per workflow by `useAllWorkflowSnapshots`, which
reports one result per workspace refresh through `setWorkspaceSnapshotRead`
into `workspaceContextRead` (`snapshotPending`, `snapshotError`). The tasks
input has failed while `snapshotError` is set, which the hook does when any
workflow's fetch failed. Its load time is the time the screen last saw
`snapshotPending` turn false with `snapshotError` null; before that it has no
load time. The lists and counts always classify the tasks of every workflow
present in `kanbanMulti.snapshots`, so a partial failure shows the workflows
that loaded, with the banner. When no workflow snapshot is present and the
read failed, the lists and the count strip are replaced by the banner.
**Try again** for tasks calls `requestWorkspaceContextRefresh()`, which makes
the hook re-fetch only the workflows that failed. A failed PR detail read is not a banner case: rows fall back
as described in [Classification](#classification). A copilot session failure is contained in the popover and does not
affect these inputs.

## Security

The stalls route authorises `workspace.read` and filters by workspace. The
screens write nothing except proposal decisions, which are authorised by their
own routes.

## Observability

The subscriber logs recorded and skipped stalls at debug level and parse
failures at warn, with task and workspace ids. The startup pass logs counts
pruned.

## Related decisions

- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
