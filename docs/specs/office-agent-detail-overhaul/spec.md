---
status: draft
created: 2026-05-04
owner: cfl
---

# Office agent detail overhaul

## Why

Today's office agent detail page (`/office/agents/[id]`) is a single
React component with a tab strip — Overview / Permissions /
Instructions / Skills / Runs / Memory / Channels — and the Runs tab
is a flat list of runs with a status pill per row. There is no
per-run detail view, no charts, no cost surface, and no deep links
into individual sections.

The target UX: each agent has a dashboard with four 14-day charts (run
activity, tasks by priority, tasks by status, success rate) plus a
"Latest Run" card, recent tasks list, and an aggregate + per-run
costs section. Each run has its own page with a left-rail of recent
runs, a header carrying status / model / time-range / token-and-cost
summary, a Session collapsible, a "Tasks Touched" table, an
Invocation panel, a Transcript with Nice/Raw modes, and an Events
log. Sub-routes (`/agents/:id/dashboard`, `/agents/:id/runs/:runId`,
etc.) are real URLs — not tab state.

Two motivations:

1. **Operational visibility.** Without per-run detail and the dashboard
   charts, a user can't tell why an agent is failing, how much it
   costs, or which tasks it's been touching. The product feels
   informational at best.
2. **Deep linking.** Bookmarking a tab, sharing a run link in chat,
   linking from the inbox to a specific run — none of this is possible
   today because everything lives behind one URL. Real sub-routes
   remove that limitation.

The target shape: visual hierarchy, data model, and routing described below.
Naming: "issue" → "task", "heartbeat run" → "run" (renaming the internal
"wakeup" vocabulary at the same time — see the plan), "company" → "workspace",
"invocationSource" → run `reason`.

## What

### Rendering: SSR-first

Every new page is a Next.js Server Component that fetches its
initial data on the server (direct HTTP to the Go backend at
`http://localhost:<backendPort>` from the Next.js runtime) and
hydrates a Client Component with the response. This is the inverse
of the current pattern where most office pages are `"use client"`
with `useEffect`-driven fetches.

- The Server Component owns the data fetch; the Client Component
  owns interactivity (collapsibles, "Load more", live mode WS).
- The Server Component can pre-fetch in parallel — e.g., the run
  detail page kicks off `getRunDetail(runId)` and
  `listAgentRuns(agentId, limit=30)` together so the page returns
  with both the main panel and the sidebar populated.
- Live mode is a strict enhancement: when the run is RUNNING, the
  Client Component subscribes to the run WS channel and merges
  appended messages/events into the SSR-supplied initial state.
  This avoids the FOUC + spinner that today's CSR pattern produces
  on every navigation.

### Routing

The agent detail page splits from one big component into nested
routes. Each sub-section is a real bookmarkable URL.

```
/office/agents/[id]
├── /                          → redirect to /dashboard
├── /dashboard                 → charts, latest run, recent tasks, costs
├── /instructions              → existing instructions tab content
├── /skills                    → existing skills tab content
├── /configuration             → existing permissions + model + executor settings
├── /runs                      → list of runs (current Runs tab content)
├── /runs/[runId]              → run detail page (NEW)
├── /memory                    → existing memory tab content
├── /channels                  → existing channels tab content
└── /budget                    → cost limits + spend (currently inside Configuration)
```

The current tab strip remains as visual navigation but each tab is
implemented as a `<Link>` to its sub-route. `defaultValue` becomes
the matched path segment.

### Dashboard sub-page

`/office/agents/[id]/dashboard` is the agent's overview at a glance.

- **Latest Run card** at the top: status badge, short run id (8 chars),
  invocation-source pill (`task_assigned`, `task_comment`,
  `manual_resume_after_failure`, etc.), one-line replied-to summary,
  relative timestamp, click-through to the run detail.
- **Four 14-day charts in a 2×2 grid (or 4×1 on wide screens):**
  - **Run Activity** — stacked bars: succeeded / failed+timed_out /
    other (cancelled, in-progress).
  - **Tasks by Priority** — stacked bars: critical / high / medium /
    low; counts the assignee tasks the agent worked on, by priority.
  - **Tasks by Status** — stacked bars across todo / in_progress /
    in_review / done / blocked / cancelled / backlog.
  - **Success Rate** — succeeded ÷ total per day, rendered as a
    percentage bar or thin line.
- All charts are custom-SVG flexbox bars (no chart library). The 14-day window is fixed for v1; a date-range picker is a follow-up.
- **Recent Tasks** — last 10 tasks the agent worked on (sorted by
  most recent activity), with identifier + title + status badge. Row
  click opens the task page.
- **Costs section:**
  - Aggregate row across all of the agent's runs: input tokens /
    output tokens / cached tokens / total cost.
  - Per-run table for the last 10 runs that have cost: date / run id
    (short) / input / output / cost.

All dashboard data comes from one new aggregate endpoint
(`GET /api/v1/office/agents/:id/summary?days=14`) so the page does a
single round-trip. The endpoint composes existing data —
`office_runs`, `tasks`, `office_cost_events`, `office_activity_log` —
into the precomputed shapes the four charts and the costs view need.

### Run detail page

`/office/agents/[id]/runs/[runId]` shows everything about a single
run.

- **Recent runs sidebar** on the left: chronological strip of the
  last ~30 runs, each row showing status icon (animated when
  RUNNING), short run id, invocation-source pill, timestamp,
  one-line summary, optional token + cost. The active row is
  highlighted; clicking switches the main panel.
- **Header strip** in the main panel:
  - Status badge (queued / running / failed / completed / cancelled
    / scheduled_retry).
  - Adapter family + model (e.g. `claude_local · claude-sonnet-4-6`).
  - Time range with absolute start/end + relative + duration.
  - Token + cost summary (input / output / cached / total).
  - Action buttons depending on status: **Cancel** (RUNNING),
    **Resume session** + **Start fresh** (FAILED), **Retry**
    (scheduled_retry).
  - Special "auth required" banner when the error indicates an
    expired token, with a link to the agent settings.
- **Session collapsible:** displays `session_id_before` and
  `session_id_after` (and the underlying ACP session id), plus a
  "Reset session for touched tasks" action that clears the resume
  token on each affected (task, agent) pair.
- **Tasks Touched table:** distinct tasks the agent acted on during
  the run. Each row links to the task. Sourced from a new query that
  joins `office_activity_log` rows whose `run_id` matches, plus
  the run's primary task from the run payload. See "Backend data
  shapes" below.
- **Invocation panel:** adapter type, working directory, optional
  Details collapsible with the command, environment vars, prompt
  context.
- **Transcript:**
  - **Raw mode** — plain log lines with timestamps; virtualized when
    >300 lines; "Load more" pagination.
  - **Nice mode** — adapter-aware parser turns the raw log into
    structured blocks: messages (user/assistant with timestamps),
    thinking, tool calls (name + input + result), stdout/stderr
    groups, file diffs, command groups, system events.
  - Toggle in the top-right corner.
- **Events log:** structured run events (init, adapter invoke,
  completion, errors) with timestamp, level (info/warn/error),
  stream (system / stdout / stderr).
- **Live mode:** when the run is RUNNING, transcript and events
  stream in via the existing session WS channel filtered by the
  run's `run_id`.

### Sub-route splits for existing tabs

The remaining tabs (Instructions, Skills, Configuration, Memory,
Channels, Budget) become real sub-routes serving the same content
they have today. The split is mostly mechanical — extract the tab
panel's contents into a `page.tsx` per sub-route.

### Backend data shapes

- **`agent_summary` endpoint** — new
  `GET /api/v1/office/agents/:id/summary?days=14` returning:
  ```jsonc
  {
    "agent_id": "...",
    "latest_run": { /* SessionSummary-shaped, including run id */ },
    "run_activity": [{ "date": "2026-04-21", "succeeded": 3, "failed": 1, "other": 0, "total": 4 }, ...],
    "tasks_by_priority": [{ "date": "...", "critical": 0, "high": 1, "medium": 2, "low": 0 }, ...],
    "tasks_by_status": [{ "date": "...", "todo": 1, "in_progress": 0, "in_review": 1, "done": 2, "blocked": 0, "cancelled": 0, "backlog": 0 }, ...],
    "success_rate": [{ "date": "...", "succeeded": 3, "total": 4 }, ...],
    "recent_tasks": [{ "task_id": "...", "identifier": "KAN-12", "title": "...", "status": "in_progress", "last_active_at": "..." }, ...],
    "cost_aggregate": { "input_tokens": 11, "output_tokens": 1224, "cached_tokens": 125300, "total_cost_cents": 0 },
    "recent_run_costs": [{ "run_id": "...", "run_id_short": "b064ad93", "date": "...", "input_tokens": 4, "output_tokens": 313, "cost_cents": 0 }, ...]
  }
  ```

- **`run_detail` endpoint** — new
  `GET /api/v1/office/agents/:id/runs/:runId` returning the run row
  with computed fields: short id, status, invocation source, started/
  finished timestamps, duration, agent adapter/model, token + cost
  rollup, session_id_before/after, error_message, tasks_touched,
  invocation (adapter + cwd + command + env), events list, log offset.

- **Tasks-touched plumbing** — add `run_id` and `session_id`
  columns to `office_activity_log`, indexed. Every agent-driven
  mutation path threads the originating run id through the
  `LogActivity` call so the read query is one `SELECT DISTINCT
  target_id WHERE run_id = ?` plus the run payload's
  `task_id`.

- **Per-run cost rollup** — `office_cost_events` already has
  `session_id` and `task_id`; add an aggregate query that returns
  per-run totals by joining cost_events to the run via the session
  it claimed.

- **Run events** — new dedicated `office_run_events` table:
  `(run_id, seq, event_type, level, payload JSON, created_at)`,
  indexed by `(run_id, seq)`. Captures lifecycle events (init,
  adapter.invoke, step, complete, error) at well-defined call sites
  in the orchestrator + office service. The table is small, the
  schema is trivial, and the read query is one indexed lookup —
  vs the alternative of joining run-status transitions +
  `task_session_messages` of `type=status` + activity_log on every
  read. The schema is trivial, one indexed lookup at read time.

- **Conversation view in the run detail** — we do **not** build a
  bespoke transcript renderer. Embed the existing session-messages
  component (`AdvancedChatPanel` from
  `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`,
  or its underlying `MessageList` / `MessageRenderer` if the panel
  is too coupled to a task) scoped to the run's `session_id`. The
  conversation already supports messages, tool calls, status rows,
  scrollback, and live updates — there is no need for a
  a bespoke "Nice" transcript parser.

- **Pagination for the runs list** — `GET /api/v1/office/agents/:id/runs`
  takes `?cursor=&limit=` (cursor = `requested_at` of the last row
  in the previous page; default limit 25, max 100). Both the
  full-page runs list and the recent-runs sidebar on the run detail
  consume this endpoint. The sidebar uses a fixed `limit=30` window
  with no pagination; the full-page list does cursor-based "Load
  more". An item count + sort order are stable across pages because
  the cursor is `(requested_at, id)` desc.

### What we are NOT doing in v1

- A configurable date range on the dashboard charts. Fixed 14 days.
- A bespoke transcript renderer or adapter-specific "Nice mode"
  parsers. We embed the existing session-messages component
  (`AdvancedChatPanel` / `MessageList`) — it already covers what
  the user needs.
- An object-store offload for run logs. The dedicated
  `office_run_events` table covers the structured event log; raw
  adapter stdout/stderr capture is a follow-up if needed.
- Per-run notifications (browser/system). The inbox already covers
  the failure path.
- Pagination for the recent-runs sidebar beyond a fixed window of
  ~30 rows. "Load more" / infinite scroll for the sidebar is
  follow-up; the full-page runs list at `/runs` does have cursor
  pagination.
