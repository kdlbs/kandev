---
status: draft
system: office
created: 2026-05-02
owners:
  - cfl
---
# Office Live Updates Requirements

## Overview

Every office page initially fetches data on mount and never updates after that. When an agent completes a task, changes status, posts a comment, or fans out a subtask, the user sees stale data until they manually refresh. The office model lets agents trigger other agents, so it is normal for several agents to be running concurrently; without real-time updates the user cannot see fan-out happen, cannot tell if a submitted comment was actually received, and cannot tell whose turn it is to act.

## Requirements

### REQ-OFFICE-LIVE-UPDATES-001: Office Live Updates

**Intent:** Every office page initially fetches data on mount and never updates after that. When an agent completes a task, changes status, posts a comment, or fans out a subtask, the user sees stale data until they manually refresh. The office model lets agents trigger other agents, so it is normal for several agents to be running concurrently; without real-time updates the user cannot see fan-out happen, cannot tell if a submitted comment was actually received, and cannot tell whose turn it is to act.

#### Acceptance criteria

- **AC-OFFICE-LIVE-UPDATES-001.1:** A pulsing blue dot (`animate-pulse`).
- **AC-OFFICE-LIVE-UPDATES-001.2:** A small text badge with the active-session count (e.g. `2 live`).
- **AC-OFFICE-LIVE-UPDATES-001.3:** `office.task.created`, `office.task.updated`, `office.task.moved`, `office.task.status_changed` cause refetch / re-render of: `Recent Tasks`, `Tasks In Progress`, the `Run Activity` chart, and the `Recent Activity` feed.
- **AC-OFFICE-LIVE-UPDATES-001.4:** `office.agent.completed` and `office.agent.failed` update the `Agents Enabled` card subtitle (running / paused / errors line).
- **AC-OFFICE-LIVE-UPDATES-001.5:** `session.state_changed`, `office.task.updated`, `office.agent.updated` cause the per-agent cards panel to refetch `GET /api/v1/office/workspaces/:wsId/agent-summaries` and replace its state. No optimistic updates - the server is the source of truth and the response is small (N agents x <=5 sessions each).
- **AC-OFFICE-LIVE-UPDATES-001.6:** The task page header shows a small `<IconLoader2 animate-spin /> Working` indicator next to the task title. Clicking it scrolls the timeline so the active session entry is visible. Hidden when no active session, with no layout reservation.
- **AC-OFFICE-LIVE-UPDATES-001.7:** An **inline session entry** appears at its chronological position in the comments timeline (one entry per session for the task, ordered by `session.startedAt`):
- **AC-OFFICE-LIVE-UPDATES-001.8:** Active session entry is expanded by default. Header reads `RUNNING * Working * for {elapsed} * ran {N} commands`. Body embeds `<AdvancedChatPanel taskId sessionId hideInput />`. `{N}` is derived from `messages.bySession[sessionId]` filtered for `type === "tool_call"`.

## System design

The migrated technical source is split into [part 1](../system-design/live-updates-01.md), [part 2](../system-design/live-updates-02.md).
