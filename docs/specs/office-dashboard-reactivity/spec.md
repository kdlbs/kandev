---
status: draft
created: 2026-05-03
owner: cfl
---

# Office Dashboard Reactivity

## Why

When a user is viewing the office dashboard and their CEO agent (or any agent)
creates, completes, or cancels a task, the dashboard does not reflect the change
until the page is refreshed. This breaks the illusion of "live" agent work and
forces the user to babysit by reloading. The `OfficeEventBroadcaster` and the
`useOfficeRefetch("dashboard")` plumbing already exist, so the gap is somewhere
between event publish and dashboard refetch.

## What

All updates MUST be driven by the existing WebSocket event stream
(`OfficeEventBroadcaster` → `useOfficeRefetch`). No polling, no `setInterval`,
no React Query refetch intervals.

- On `office.task.created`, `office.task.updated`, `office.task.moved`,
  `office.task.status_changed`, the dashboard MUST update `Recent Tasks`,
  `Tasks In Progress`, `Run Activity` chart, and `Recent Activity` feed.
- On `office.agent.completed` and `office.agent.failed`, the dashboard MUST
  update `Agents Enabled` (running/paused/errors line).
- Refresh MUST be scoped to the active workspace — events for other workspaces
  MUST NOT trigger refetches on this dashboard. Today the WS broadcaster sends
  to all clients; the fix is either (a) add `workspace_id` to the event
  payload and filter client-side in the office WS handler, or (b) workspace-
  scope the broadcast on the server. The plan picks one.

## Scenarios

- **GIVEN** the user is on `/office`, **WHEN** the CEO agent creates a delegated
  child task via the MCP `mcp.create_task` flow, **THEN** the new task appears
  in `Recent Tasks` and the `Tasks In Progress` count increments by 1, driven
  by the `office.task.created` WS event.
- **GIVEN** the user is on `/office` and a task is currently in progress,
  **WHEN** the agent transitions the task to `done`, **THEN** the
  `Tasks In Progress` count decrements and the `Run Activity` chart updates
  the current-day bar, driven by `office.task.status_changed`.
- **GIVEN** the user is on `/office`, **WHEN** an unrelated workspace fires
  `office.task.created`, **THEN** the dashboard does NOT refetch (no network
  request to `/api/v1/office/workspaces/{wsId}/dashboard`).

## Out of scope

- Polling fallbacks of any kind. If the WS connection is down, the dashboard
  stays as-is until the connection recovers and the next event arrives.
- Replacing the Zustand `refetchTrigger` mechanism with React Query / SWR.
  (Possible follow-up; not required for the user-visible fix.)
- Optimistic updates that mutate dashboard state before the server responds.
- Animating chart bar transitions on update.

## Open questions

- Should `task.created` events from the core task service include
  `workspace_id` in the payload so the office broadcaster can workspace-scope
  the broadcast on the server side, instead of relying on client-side filtering?
