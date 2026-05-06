---
status: draft
created: 2026-05-03
owner: cfl
---

# Office Sidebar — Live Agent Indicators

## Why

The office sidebar lists agents but tells the user nothing about whether they're
*currently working*. A CEO agent with a status dot looks the same whether it's
idle, running one task, or running five in parallel. The sidebar should show a
pulsing dot plus an `{N} live` badge for agents with active runs — the user can
tell at a glance who's busy without leaving the page they're on.

## Why this matters now: kandev's orchestrate model lets agents trigger other
agents, so it's normal to have several agents running concurrently. Without a
live indicator, the user can't see fan-out happening in real time.

## What

- Each agent row in the sidebar (`SidebarAgentsList`) MUST display a visual
  indicator when the agent has one or more active sessions:
  - A pulsing dot (`animate-pulse` blue dot).
  - A small text badge showing the active session count (e.g. `2 live`).
- When the agent has zero active sessions, the indicator MUST collapse back to
  the static status dot already in place — no layout shift.
- The indicator MUST update in real time as sessions start/stop, driven
  exclusively by the existing WS event stream (`session.state_changed`,
  `office.agent.completed`, `office.agent.failed`). No polling.
- Active sessions MUST be counted per-agent, not globally — clicking through
  to an agent's detail page reveals which tasks are running.
- Indicator behavior MUST work for the CEO agent in the dedicated `AGENTS`
  section as well as for any other agents the workspace adds later.

## Scenarios

- **GIVEN** the CEO has no active sessions, **WHEN** the user views the
  sidebar, **THEN** the CEO row shows only the existing status dot (idle/paused
  styling), no badge.
- **GIVEN** the CEO is running 1 session, **WHEN** the sidebar renders,
  **THEN** the CEO row shows a pulsing indicator and `1 live` badge.
- **GIVEN** the CEO is running 1 session, **WHEN** another task starts and
  the agent now has 2 sessions, **THEN** the badge updates to `2 live` within
  2 seconds without a page refresh.
- **GIVEN** the CEO is running 2 sessions, **WHEN** both complete, **THEN**
  the indicator returns to the idle status dot within 2 seconds.

## Out of scope

- A global "N agents working" badge in the topbar.
- Per-task progress percentages (we don't have that data).
- Click-to-jump from the badge directly into a specific running session
  (clicking the agent row already navigates to the agent detail).

## Open questions

- Where do we source the per-agent active session count? Options:
  (a) compute it client-side from the existing `taskSessions` store keyed
  by `agent_instance_id`, kept fresh by the WS session events we already
  receive, or (b) include `live_runs_count` on the `office.agent.updated`
  WS payload so the sidebar reads it directly. Both are WS-driven; pick
  the simpler one in the plan.
