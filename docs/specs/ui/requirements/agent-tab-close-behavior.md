# Agent tab close behavior

## REQ-UI-AGENT-TAB-CLOSE-BEHAVIOR-001

The account-level `agent_tab_close_behavior` preference controls only the desktop Agent-tab X. Missing or invalid values mean `delete_session`. `delete_session` preserves the existing confirmed deletion flow. `hide_panel` removes only a visible Dockview panel, retains the session and transcript, and permits explicit reopening from `+ > Agents`.

In hide-panel mode the X is unavailable for the last visible Agent panel. Hidden-panel records are session-storage state scoped to the task environment and owning task; they remain through a reload in the same browser tab and are pruned only after the owning task has an authoritative session list.

Mobile and tablet task session deletion behavior is unchanged. The responsive Settings control may edit the preference and states that it applies to desktop Agent tabs.
