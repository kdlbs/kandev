---
status: draft
created: 2026-05-02
owner: cfl
---

# Office Dashboard

## Why

The office dashboard is the first place a user looks to understand workspace health at a glance: which agents are working, how runs and tasks are trending, what changed recently, and how much the workspace is spending. Before this work it showed 4 metric cards and a recent-activity feed with raw UUIDs, with no visibility into per-agent state, run trends, or recently updated tasks.

## What

The dashboard is laid out top-to-bottom as:

1. Per-agent cards panel
2. Clickable stat cards (Agents Enabled, Tasks In Progress, Month Spend, Pending Approvals)
3. Chart grid (Run Activity, Tasks by Priority, Tasks by Status, Success Rate)
4. Recent Tasks section
5. Recent Activity feed

Live-update behaviour (WS events, sidebar live indicators, dashboard reactivity) is specified in `live-updates.md` and is not repeated here.

### Per-agent cards panel

- Renders **one card per workspace agent**, listed in the order agents are configured. The panel always renders, even when no agent has ever run; it never collapses to empty.
- Cards are laid out in a responsive grid: 1 / 2 / 4 columns based on viewport. All cards share the same width regardless of state, so the layout does not jump as state changes.
- Sort order: agents with a live session first, then by most-recent session, then alphabetically. Sort is stable so minor state changes do not reorder the panel.

Each card has three observable states:

| State | Trigger | Visual |
|---|---|---|
| `live` | Agent has any session in `RUNNING` | Pulsing emerald dot, `Live now` subtitle, current task pill, expanded recent runs |
| `finished` | Agent has at least one historical session, none currently `RUNNING` | Muted dot, `Finished {relativeTime}` subtitle, last task pill, expanded recent runs |
| `never_run` | Agent has zero session history | Muted card with agent identity only |

Card anatomy:

1. **Header** - pulsing dot when live, agent avatar + display name, external-link icon linking to `/office/agents/{id}`.
2. **Subtitle** - `Live now`, `Finished {relativeTime}`, or `Never run`.
3. **Current task pill** (when there is a most-recent session) - `{identifier} - {title}` (e.g. `KAN-3 - present yourself`); links to `/office/tasks/{taskId}`.
4. **Expanded run section** - up to 5 most recent sessions, one row each. Scrollable when long.
   - Completed row: `{Agent} worked for {duration} - ran {N} commands`
   - Active row: `working for {duration}` (animated)
   - Each row carries a relative-time stamp of session start.

### Clickable stat cards

Four metric cards at the top of the chart area. Each card is a link to its detail page and shows a richer breakdown subtitle (not the prior generic description).

| Card | Subtitle example | Links to |
|---|---|---|
| Agents Enabled | `2 running, 1 paused, 0 errors` | `/office/agents` |
| Tasks In Progress | `13 open, 1 blocked` | `/office/tasks` |
| Month Spend | (current period total + delta) | `/office/workspace/costs` |
| Pending Approvals | (count of pending approval requests) | `/office/inbox` |

### Charts

Four visualisations rendered with pure CSS bars (no charting library). All share a 14-day x-axis with date labels at days 0, 6, and 13. The tallest bar on a chart scales to 100% height; other bars scale proportionally.

| Chart | Encoding |
|---|---|
| Run Activity | Stacked bars per day: succeeded (green), failed (red), other (gray) |
| Tasks by Priority | Stacked bars per day: tasks created, grouped by priority (critical, high, medium, low) |
| Tasks by Status | Stacked bars per day: tasks created, grouped by status (todo, in_progress, done, blocked) |
| Success Rate | Single bar per day: percentage of succeeded runs. Colour: green (>=80%), yellow (50-79%), red (<50%) |

### Recent Tasks

A list of the 10 most recently updated tasks. Each row: status icon, identifier (monospace), title (truncated), assignee agent name, relative timestamp. Each row links to the task detail page.

### Recent Activity feed

A reverse-chronological feed of workspace activity. Raw UUIDs are resolved to human-readable names:

- Task IDs are rendered as identifiers (e.g. `KAN-14`).
- Agent IDs are rendered as agent names.
- Wakeup IDs are hidden entirely.
- Action descriptions are formatted as readable sentences: `CEO completed task KAN-14`, not `wakeup cancelled stale wakeup 64a8cdcd-...`.
- The `system` actor is rendered with a `SY` badge; agent actors are rendered with the agent's initials.

## Data model

The dashboard reads from existing office tables; no new tables are introduced for the dashboard surface itself. The agent-cards panel adds a new derived/aggregated read path.

The agent-cards panel relies on:
- `office_agents` (workspace agent instances)
- `task_sessions` joined by `agent_instance_id`, ordered `started_at DESC`
- `office_tasks` for `task_identifier` and `task_title`
- `messages` for command counts (`type = 'tool_call'` count per session)

For the `status` field, an office session in state `IDLE` counts as `finished` (not `live`).

## API surface

The dashboard frontend hits an existing dashboard endpoint plus a new per-agent summaries endpoint introduced for the per-agent cards panel.

### Agent summaries

```
GET /api/v1/office/workspaces/:wsId/agent-summaries
```

Response:

```json
{
  "agents": [
    {
      "agent_id": "...",
      "agent_name": "CEO",
      "agent_role": "ceo",
      "status": "live | finished | never_run",
      "live_session": {
        "session_id": "...",
        "task_id": "...",
        "task_identifier": "KAN-3",
        "task_title": "present yourself",
        "started_at": "2026-05-04T..."
      },
      "last_session": {
        "session_id": "...",
        "task_id": "...",
        "task_identifier": "KAN-3",
        "task_title": "present yourself",
        "started_at": "...",
        "completed_at": "..."
      },
      "recent_sessions": [
        {
          "session_id": "...",
          "task_id": "...",
          "task_identifier": "KAN-3",
          "task_title": "present yourself",
          "state": "RUNNING | IDLE | COMPLETED | FAILED | CANCELLED",
          "started_at": "...",
          "completed_at": "...",
          "command_count": 3
        }
      ]
    }
  ]
}
```

Semantics:
- `live_session` is `null` unless `status == "live"`.
- `last_session` is `null` unless the agent has at least one session.
- `completed_at` may be `null` for office sessions still in `IDLE`.
- `recent_sessions` returns up to 5 entries, most recent first.

The pre-existing `GET /workspaces/:wsId/live-runs` endpoint stays for backward compatibility and continues to return the prior per-run shape it returned before agent cards landed.

### Dashboard payload

The existing `GET /api/v1/office/workspaces/:wsId/dashboard` endpoint backs the stat cards, charts, Recent Tasks list, and Recent Activity feed. (Shape evolves alongside the dashboard; cross-link to the implementation when adding new sections rather than re-restating here.)

## Permissions

The dashboard is a read-mostly surface scoped to a single workspace. Authorization rules:

- **Workspace scoping.** Every dashboard endpoint takes `:wsId` in the path. The `agents.AgentAuthMiddleware` rejects any agent-JWT request whose `claims.WorkspaceID` does not match `:wsId` with `403 token workspace mismatch`. UI callers (no JWT) currently pass through unchecked — single-user kandev does not yet have a user-session auth layer, so the dashboard treats all UI callers as the singleton human user (`decider_id = "user"`, `inbox_dismissals.user_id = "default"`).
- **Read endpoints** (`GET /dashboard`, `/agent-summaries`, `/agents/:id/summary`, `/agents/:id/runs`, `/agents/:id/runs/:runId`, `/inbox`, `/activity`, `/runs`, `/tasks`, `/tasks/search`, `/live-runs`) require only that the caller resolve to the workspace. No per-agent visibility filtering is applied — every agent in the workspace can see every other agent's cards, runs, and activity. The per-agent inbox view (`GetAgentInboxItems`) is invoked internally for agent-scoped wakeups and returns only that agent's pending review/approval requests; it is not a separate HTTP route.
- **Mutating endpoints** carry the same permission keys defined in `internal/office/shared/permissions.go`:
  - `POST /tasks/:id/approve`, `POST /tasks/:id/request-changes`: agent callers require `can_approve`. Cross-workspace decisions are rejected with `403`. An agent cannot decide an approval it requested (self-approval guard).
  - `POST /tasks/:id/blockers`, `DELETE /tasks/:id/blockers/:blockerId`, reviewer/approver mutations: open to all callers (no permission gate today).
  - `PATCH /tasks/:id` assignee mutation surfaces `shared.ErrForbidden` as `403` for agent callers without the required permission, anything else as `500`.
  - `POST /inbox/dismiss`: requires the `MarkFixedHandler` to be wired (`503` otherwise) and resolves under `user_id = "default"`.
  - Settings mutation (`PATCH /workspaces/:wsId/settings`): no gate today; UI-only.
- **Activity feed entries** carry `actor_type ∈ {user, agent, system}` and `actor_id` (user id, agent instance id, or literal `"system"`). The feed does not redact entries per viewer — every workspace member sees the full log.
- **Routing endpoints** (`/workspaces/:wsId/routing*`, `/agents/:id/route`, `/runs/:id/attempts`) are scoped the same way; their authorization rules live in `office/routing.md`.

See `agents/` spec for the role → permission defaults and the no-escalation rule that governs the underlying permission keys.

## Persistence guarantees

The dashboard is a thin projection over durable office tables. Nothing the dashboard surfaces is held in process memory across the request boundary; on restart the next dashboard fetch recomputes everything from SQLite.

**Durable (survives restart):**

- Recent Activity feed and the status-change timeline reads `office_activity_log` rows; entries are append-only and have no TTL.
- Pending Approvals card count reads `office_approvals` rows; `pending` rows persist until the user decides them.
- Stat cards, charts, Recent Tasks, Tasks by Status / Priority — derived from `tasks`, `office_agents`, `office_runs`, `messages`, `office_cost_events`. None of this data is dashboard-owned.
- Run Activity / Success Rate bars query `office_runs` for the 14-day window; rows older than the window stay in the DB but are not visualised.
- Inbox dismissals persist in `office_inbox_dismissals` keyed by `(user_id, item_kind, item_id)` so a "Mark fixed" decision survives restart.
- Workspace governance flags (`require_approval_for_new_agents`, `require_approval_for_task_completion`, `require_approval_for_skill_changes`) persist in `office_workspace_governance`; `permission_handling_mode` and `recovery_lookback_hours` persist on the workspace's filesystem config via the `SettingsProvider` adapter.
- Workflow-domain decisions (used to compute "viewer needs decision" on inbox review-request items) persist in `workflow_step_decisions` and are superseded — never deleted — when re-recorded.

**Transient (recomputed on every read):**

- Per-agent cards `status` (`live` / `finished` / `never_run`), the sort order, `recent_sessions`, and `command_count` are computed live by `GetAgentSummaries` from `office_agents` + `task_sessions` + `messages` at request time. There is no cached cards payload; a kandev restart serves identical content from the first fetch onward.
- Inbox is a computed view (see `inbox.md` Persistence guarantees) — no `office_inbox` table exists. The badge count and the listing both recompute on every fetch.
- Frontend store hydration: SSR fetches dashboard data once on page load and hydrates the Zustand store. Live updates are WS-driven (`live-updates.md`); on hard reload the SSR fetch reseeds the store. WS subscriptions are not durable — the gateway re-subscribes on reconnect.
- The dashboard does NOT cache anything backend-side beyond the request lifetime; there is no Redis/in-memory dashboard cache. Adding one would require explicit invalidation hooks and is intentionally not implemented.

## Scenarios

- **GIVEN** a workspace with N agents configured and no recent sessions, **WHEN** the user opens the dashboard, **THEN** the agent cards panel renders exactly N cards, each in `never_run` state with no live indicator and no task pill.

- **GIVEN** a workspace with 2 agents, 1 running and 1 idle, **WHEN** the user opens the dashboard, **THEN** the running agent's card shows a pulsing dot, `Live now` subtitle, and current task pill; the idle agent's card shows a muted dot and `Finished {relativeTime}` subtitle.

- **GIVEN** 14 days of agent run history with some failures, **WHEN** the user views the Run Activity chart, **THEN** stacked bars show green / red / gray segments per day with the tallest bar scaled to 100% height.

- **GIVEN** a workspace with 13 open tasks and 1 blocked task, **WHEN** the user views the Tasks In Progress card, **THEN** the card shows the in-progress count with subtitle `13 open, 1 blocked`, and clicking it navigates to `/office/tasks`.

- **GIVEN** 10 recently updated tasks, **WHEN** the user views the Recent Tasks section, **THEN** each row shows the task identifier (monospace), title, assignee agent, and relative timestamp, and clicking a row opens the task detail page.

- **GIVEN** the Recent Activity feed contains a wakeup-cancellation entry, **WHEN** the user views the feed, **THEN** the row reads as a readable sentence with no raw UUIDs, the actor badge is `SY` for system actors, and any task references render as identifiers (e.g. `KAN-14`).

- **GIVEN** a workspace with mixed agent states, **WHEN** the agent cards panel sorts, **THEN** live agents render first, then by most-recent session, then alphabetically; the sort is stable across re-renders.

## Out of scope

- Live streaming of agent transcripts inside dashboard cards. v1 of the per-agent expanded run rows is header-only; an embedded `<AdvancedChatPanel>` per row may follow once dashboard visual weight is reassessed.
- Per-card cost / utilisation stats - existing dashboard-level metrics already cover this.
- Agent grouping (e.g. by role). The panel is a flat list, sorted by activity.
- Plugin widget slots.
- Budget incident banners (separate feature).
- Cost breakdown charts - belongs in the costs page.
- Polling. All dashboard live behaviour is specified in `live-updates.md` and is WS-driven.
