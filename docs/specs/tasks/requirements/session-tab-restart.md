---
status: active
system: tasks
created: 2026-10-14
owners:
  - kandev
---

# Session tab restart requirements

## Overview

Users can restart an active agent from its session menu with one action.
Restart performs Stop followed by Resume on the same session.
Tasks owns this contract because it controls the session lifecycle.

The user confirmed that restart means "just stopping and resuming".
This action preserves conversation history. Configuration Chat retains its
separate [replacement behavior](configuration-chat-restart.md).

## Requirements

### REQ-TASKS-SESSION-TAB-RESTART-001: Restart the selected session

**Intent:** Make the existing stop-and-resume sequence available as one action.

#### Acceptance criteria

- **AC-TASKS-SESSION-TAB-RESTART-001.1:** A persisted session in RUNNING,
  STARTING, or WAITING_FOR_INPUT shall offer Restart session beside Stop.
  Desktop workbench and preview tabs shall expose the action in their context
  menus. Phone users shall access it through the Sessions picker and the
  session row's visible actions button. Other states shall retain their
  existing actions without offering Restart session.
- **AC-TASKS-SESSION-TAB-RESTART-001.2:** Selecting Restart session shall stop
  the targeted session, then request its normal resume after Stop succeeds.
  The action shall retain the session identity, conversation history, and
  workspace. It shall preserve unsent input and tab selection throughout the
  attempt. It shall preserve whether selection was automatic or explicitly pinned.
  A later explicit user selection during the restart shall take
  precedence. It shall not
  create a replacement session, submit a prompt, or explicitly select a primary
  session. The existing Stop lifecycle can promote another active session to primary.
- **AC-TASKS-SESSION-TAB-RESTART-001.3:** A failed Stop shall prevent Resume.
  A failed Resume shall retain the conversation and expose the normal recovery
  actions. Missing targets and unavailable connections shall produce visible
  errors. A timeout shall not trigger an automatic retry or report success.
- **AC-TASKS-SESSION-TAB-RESTART-001.4:** While the sequence is pending, the
  browser shall prevent duplicate restarts for that session across its mounted
  menus. Stop, Resume, and Delete for that target shall be unavailable during
  the sequence. Other sessions shall remain usable. Progress and the final
  request outcome shall be visible and accessible after the menu closes.
- **AC-TASKS-SESSION-TAB-RESTART-001.5:** Desktop and phone shall share action
  eligibility, ordering, and failure behavior. Labels and feedback shall be
  localized. Keyboard activation and focus return shall follow the existing
  menus. Phone and coarse-pointer action targets shall measure at least 44px.
  Menus shall remain within the viewport without horizontal page overflow.
- **AC-TASKS-SESSION-TAB-RESTART-001.6:** While Restart session is pending,
  every composer for that target shall prevent submission through Send,
  keyboard shortcuts, and the shared submit handler. The draft shall remain
  editable and retain its text and attachments. A visible, localized status
  shall explain the temporary block. Other sessions shall remain usable.
  After the restart requests settle, normal input readiness and recovery rules
  shall determine whether submission is available. Restart shall not bypass a
  recovery block or submit the unsent draft automatically.

## Out of scope

- Blank conversations, Configuration Chat replacement, and host restarts.
- New runtime recovery policies, permission overrides, or automatic retries.
- New lifecycle APIs, cross-browser restart transactions, and durable restart jobs.

## Implementation plans

- [Session tab restart](../../../plans/session-tab-restart/plan.md)
