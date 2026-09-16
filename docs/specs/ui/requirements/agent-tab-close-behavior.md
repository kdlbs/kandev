---
status: active
system: ui
created: 2026-09-15
owners:
  - kandev
---
# Agent tab close behavior Requirements

## Requirements

### REQ-UI-AGENT-TAB-CLOSE-BEHAVIOR-001: Configurable Agent-tab close behavior

The account-level `agent_tab_close_behavior` preference controls only the desktop Agent-tab X. Missing or invalid values mean `delete_session`. `delete_session` preserves the existing confirmed deletion flow. `hide_panel` removes only a visible Dockview panel, retains the session and transcript, and permits explicit reopening from `+ > Agents`.

In hide-panel mode the X is unavailable for the last visible Agent panel. Hidden-panel records are session-storage state scoped to the task environment and owning task; they remain through a reload in the same browser tab and are pruned only after the owning task has an authoritative session list.

Mobile and tablet task session deletion behavior is unchanged. The responsive Settings control may edit the preference and states that it applies to desktop Agent tabs.

#### Acceptance criteria

- **AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.1:** With `delete_session`, the Agent-tab X uses the existing confirmed deletion flow.
- **AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.2:** With `hide_panel`, the X removes only a non-final visible Agent panel and does not call session deletion.
- **AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.3:** A hidden session remains available from **+ > Agents** and explicit reopening restores its existing conversation.
- **AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.4:** Hidden records are scoped to their owning task, survive a same-tab reload, and are pruned only after that task's sessions are authoritative.
- **AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.5:** Phone and tablet task views retain the existing Sessions picker and explicit deletion behavior.
