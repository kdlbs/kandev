---
status: shipped
created: 2026-05-02
owner: cfl
---

# Office Dashboard

## Why

The office dashboard currently shows 4 metric cards and a recent activity feed. Users have no visibility into what agents are doing right now, how runs are trending over time, which tasks were recently updated, or how reliable agent execution is. The dashboard should be the first place a user looks to understand workspace health at a glance.

## What

### Agent cards panel

- The top of the dashboard shows up to 4 agent run cards in a responsive grid.
- Each card shows: agent name, status dot (active = pulsing, finished = gray), linked task (identifier + title), run outcome ("completed after 12s" / "failed after 45s" / "cancelled after 8s"), and relative timestamp.
- Active runs appear first, padded with recently finished runs to fill 4 slots.
- Each card links to the task detail page.

### Clickable stat cards

- The 4 existing metric cards (Agents Enabled, Tasks In Progress, Month Spend, Pending Approvals) become clickable links to their detail pages (`/office/agents`, `/office/tasks`, `/office/workspace/costs`, `/office/inbox`).
- Descriptions show richer breakdowns: "13 open, 1 blocked" instead of "Currently running or queued tasks".

### Charts (4 visualizations, 14-day window)

All charts use pure CSS bars (no charting library). They share a 14-day x-axis with date labels at days 0, 6, and 13.

- **Run Activity**: Stacked bars per day showing succeeded (green), failed (red), and other (gray) runs.
- **Tasks by Priority**: Stacked bars per day showing tasks created, grouped by priority (critical, high, medium, low).
- **Tasks by Status**: Stacked bars per day showing tasks created, grouped by status (todo, in_progress, done, blocked).
- **Success Rate**: Single bars per day showing the percentage of succeeded runs. Color-coded: green (≥80%), yellow (50–79%), red (<50%).

### Recent Tasks section

- Below the charts, a "Recent Tasks" section shows the 10 most recently updated tasks.
- Each row: status icon, identifier (monospace), title (truncated), assignee agent name, relative timestamp.
- Links to the task detail page.

## Scenarios

- **GIVEN** a workspace with 2 agents, 1 running and 1 idle, **WHEN** the user opens the dashboard, **THEN** the agent cards panel shows the running agent's card with a pulsing status dot and current task, plus up to 3 recently finished run cards.

- **GIVEN** 14 days of agent run history with some failures, **WHEN** the user views the Run Activity chart, **THEN** stacked bars show green/red/gray segments per day, with the tallest bar scaled to 100% height.

- **GIVEN** a workspace with 13 open tasks and 1 blocked task, **WHEN** the user views the Tasks In Progress card, **THEN** it shows "3" (in-progress count) with subtitle "13 open, 1 blocked", and clicking it navigates to `/office/tasks`.

- **GIVEN** 10 recently updated tasks, **WHEN** the user views the Recent Tasks section, **THEN** each row shows the task identifier, title, assignee agent, and "X ago" timestamp.

### Recent Activity polish

- The existing Recent Activity section resolves raw UUIDs to human-readable names: task IDs become identifiers (KAN-14), agent IDs become agent names, wakeup IDs are hidden.
- Action descriptions are formatted as readable sentences: "CEO completed task KAN-14" instead of "wakeup cancelled stale wakeup 64a8cdcd-...".
- System actor shows "SY" badge instead of "u", agent actor shows agent initials.

## Out of scope

- Live streaming of agent transcripts in dashboard cards (polling or static snapshot only)
- Plugin widget slots
- Budget incident banners (separate feature)
- Cost breakdown charts (belongs in the costs page)
