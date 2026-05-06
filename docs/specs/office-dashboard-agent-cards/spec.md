---
status: draft
created: 2026-05-04
owner: cfl
---

# Office: Dashboard agent cards

## Why

The office dashboard's "Agents" panel today renders one card per
*recent live session* (max 4) and disappears entirely when there are
no live sessions. After our fire-and-forget lifecycle landed, sessions
spend most of their time IDLE — so the panel is empty most of the time.
Even when a card does appear, it's per-run (you'd see two cards if the
same agent ran twice today) rather than per-agent.

The target design shows a **persistent card per agent**, always visible, with three states:

- **Live now** — pulsing dot, current task pill, expanded run header
- **Finished N ago** — muted dot, last task pill, expanded recent-runs
- **Idle / Never run** — empty muted card with the agent's identity

This is the dashboard's "is anyone doing anything?" surface. With the
office model we just shipped (agents cycle RUNNING ↔ IDLE per turn),
the persistent-per-agent shape is exactly what users want — at a
glance you see every agent, what they're doing right now, and what
they last did.

## What

### A. Per-agent card model

The dashboard renders **one card per workspace agent**, listed in the
order agents are configured. Each card has:

1. **Header**
   - Pulsing emerald dot when the agent has any RUNNING session.
   - Agent avatar + display name.
   - External-link icon → routes to `/office/agents/{id}`.
2. **Subtitle**
   - `Live now` when any session for this agent is RUNNING.
   - `Finished {relativeTime}` when the agent has at least one
     historical session and none are currently RUNNING.
   - `Never run` when the agent has zero session history.
3. **Current task pill** (when there is a most-recent session)
   - Format: `{identifier} - {title}` (e.g. `KAN-3 - present yourself`)
   - Click → routes to `/office/tasks/{taskId}`.
4. **Expanded run section** (always rendered, scrollable when long)
   - One row per recent session, capped at 5 most recent.
   - Format: `{agent name} worked for {duration} · ran {N commands}` for
     completed sessions; `working for {duration}` (animated) for the
     active one.
   - Per-row timestamp: `{relativeTime}` of session start.
   - Each row is expandable to show the streaming chat embed (same
     `<AdvancedChatPanel>` used elsewhere). v1: header-only — the embed
     can be a follow-up if the dashboard becomes too visually heavy.

Visual: cards laid out in a responsive grid (1 / 2 / 4 columns based
on viewport). Same width regardless of state (no card collapses/grows
based on activity).

### B. Backend — agent-summaries endpoint

A new endpoint scoped to one fetch per dashboard load + refetches:

```
GET /api/v1/office/workspaces/:wsId/agent-summaries
```

Response shape:

```json
{
  "agents": [
    {
      "agent_id": "...",
      "agent_name": "CEO",
      "agent_role": "ceo",
      "status": "live | finished | never_run",
      "live_session": {                   // null when status != "live"
        "session_id": "...",
        "task_id": "...",
        "task_identifier": "KAN-3",
        "task_title": "present yourself",
        "started_at": "2026-05-04T..."
      },
      "last_session": {                   // null when status == "never_run"
        "session_id": "...",
        "task_id": "...",
        "task_identifier": "KAN-3",
        "task_title": "present yourself",
        "started_at": "...",
        "completed_at": "..."             // may be null for IDLE office sessions
      },
      "recent_sessions": [                // up to 5, most recent first
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

Implementation in `dashboard/service.go` `GetAgentSummaries(wsID)`:
- List all agent instances for the workspace.
- For each agent, query `task_sessions WHERE agent_instance_id = ?`
  ordered by `started_at DESC LIMIT 5`.
- Pick the live session (if any), the last session (newest), and the
  recent-sessions array.
- For office sessions with `state = IDLE`, treat as "finished" for the
  status field.
- Resolve task identifier + title via a single batched lookup.
- Resolve command count per session via a `COUNT(*) FROM messages WHERE
  type = 'tool_call'` subquery (or a similar derivation). Defer to
  client-side if backend is too expensive — see plan B6.

The existing `GET /workspaces/:wsId/live-runs` endpoint stays for
backward compatibility and returns whatever it returned before.

### C. Frontend — `AgentCardsPanel` rewrite

- Drop the `runs.length === 0 → return null` early-out. The panel
  always renders.
- Fetch from the new endpoint via `getAgentSummaries(wsId)` (added to
  `office-api`).
- Render one `<AgentCard>` per entry returned from the backend.
- `<AgentCard>` is a new component pulled out of the existing
  `RunCard` plus the additional layout (header / subtitle /
  task pill / expanded runs).
- Sort: agents with a live session first, then by most-recent session,
  then alphabetically. Stable sort so the layout doesn't jump on minor
  state changes.

### D. Reactivity — WS-driven, no polling

The cards must update without page refresh in three scenarios:

1. **Agent's session enters RUNNING.** A wakeup landed and the agent
   is now working. Card flips to "Live now" + shows the task pill.
2. **Agent's session leaves RUNNING (IDLE / COMPLETED / FAILED).**
   Card flips to "Finished {relativeTime}" + shows the same task pill
   (the most recent one).
3. **Agent's task changes.** A new wakeup on a different task. The
   pill updates to the new task identifier + title.

WS events that should trigger a refetch (handler in `office.ts`):
- `session.state_changed` — already exists. The handler updates
  `taskSessions` in the store; the dashboard subscribes via the
  refetch trigger pattern.
- `office.task.updated` — handles assignee changes and updates the
  per-agent surfaces.
- `office.agent.updated` — handles agent identity / status changes.

Refetch policy: on any of those events, the panel hits
`getAgentSummaries(wsId)` and replaces its state. No optimistic
updates — the server is the source of truth and the response is small
(N agents × ≤5 sessions each).

### E. Out of scope

- Streaming chat embed inside expanded run rows. v1 is header-only.
- Per-card cost / utilization stats — the existing dashboard-level
  metrics already cover this.
- Agent grouping (e.g. "by role"). Flat list, sorted by activity.
- Polling. WS-driven only, matching the project's "no polling" rule.

## Acceptance

1. Open the dashboard with N agents configured. The "Agents" panel
   renders exactly N cards, regardless of how many sessions exist.
2. With no agent activity, every card shows "Never run" (when no
   session history) or "Finished {relativeTime}" (with history). No
   live indicator pulses.
3. Launch an agent on a task (real wakeup, not seeded). Within ~1
   second of `session.state_changed → RUNNING`, the corresponding
   card flips to "Live now" with the agent's pulsing dot and the
   task pill. No page refresh needed.
4. When the turn completes (session goes IDLE), the card flips back
   to "Finished {relativeTime}" without manual refresh.
5. Adding a comment that wakes the agent again updates the same
   card — the live dot reappears, the task pill stays the same.
6. Reassigning a task to a different agent moves the task pill from
   the old agent's card to the new agent's card on the next
   render.
7. The WS events that drive these updates are exactly:
   `session.state_changed`, `office.task.updated`,
   `office.agent.updated` — no others, no polling.
