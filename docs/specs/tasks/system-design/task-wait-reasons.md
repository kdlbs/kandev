---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-WAIT-REASONS-001
created: 2026-11-07
owners:
  - kandev
---

# Recorded Task Wait Reasons System Design

## Purpose and boundaries

The task status summary projects recorded waits into task reads so every task
surface can name why a task is parked. Agent routing owns the quota decision and
its deadline; this design only reads that recorded state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-WAIT-REASONS-001` | [Components and responsibilities](#components-and-responsibilities), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `statussummary.QuotaWaitSummaryFromRouteState` maps one session's durable
  route state to a bounded quota wait: only `retry_wait` or `waiting_for_reset`
  with `routingerr.CodeQuotaLimited` and a deadline qualify. A route parked
  because every candidate is suspended has no recorded cause and is not a quota
  wait.
- The SQLite status-summary quota read selects the current session's logical
  profile and route generation and excludes terminal and historical sessions.
- The projector derives the summary's quota wait and re-reads it when a session
  changes state or is removed.
- `resolveWaitReason` in `apps/web/lib/ui/waiting-reason.tsx` is the single
  frontend precedence: quota, then session capacity, then WIP queue. Pending
  clarification and permission keep their own icons, and `WAITING_FOR_INPUT`
  alone is not an input.
- Task-list rows, kanban cards, graph step nodes, the desktop sidebar, and the
  mobile switcher render the resolved reason with localized labels.

## Data and contracts

The task status summary carries an optional `quota_wait` with session ID,
reason, and UTC deadline. The policy body, classifier rule, and provider payload
stay in the routing tables.

## Failure and recovery

A failed optional quota read is returned from the projection rather than
treated as an empty observation, so a stale wait is not shown as current and
updates from other task-status sources continue. Missing summaries rebuild from
recorded route state. Existing summaries repair through the revisioned update
path; when a newer revision wins, the repair reads the latest summary and route
before retrying.

## Persistence

No migration. The wait is derived from existing route state and stored in the
existing status summary.

## Security

The projection exposes no provider payload, credential, or policy body.

## Observability

No new logs or metrics. A failed quota read surfaces as an error from the
status projection instead of a silent empty wait.
