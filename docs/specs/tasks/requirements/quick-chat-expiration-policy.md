---
status: active
system: tasks
created: 2026-11-16
owners:
  - kandev
---

# Quick Chat Manual Lifetime Requirements

## Overview

Quick Chat conversations remain available until the user explicitly deletes them or their workspace.
Inactivity, agent completion, and backend restarts do not delete a conversation.
The task system owns this contract because conversation deletion removes the backing task, sessions, messages, and owned resources.

This requirement replaces the unimplemented opt-in proposal `REQ-TASKS-QUICK-CHAT-EXPIRATION-002`.
That identifier and its acceptance criteria are retired.
The [implementation plan](../../../plans/remove-quick-chat-expiration/plan.md) records delivery status.

## Requirements

### REQ-TASKS-QUICK-CHAT-EXPIRATION-003: Manual Quick Chat lifetime

**Intent:** Users decide when to stop an agent and when to delete its Quick Chat conversation.

#### Acceptance criteria

- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.1:** The system shall not delete a Quick Chat because of its age or inactivity, including at startup and during scheduled maintenance.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.2:** Quick Chat conversations, sessions, messages, and owned resources shall survive reloads and backend restarts regardless of inactivity duration.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.3:** Stopping an agent shall preserve its Quick Chat tab and conversation. Closing the modal or switching tabs shall preserve them.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.4:** Confirmed closure of a persisted Quick Chat tab shall retain the existing task deletion lifecycle. Canceling closure shall preserve the conversation.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.5:** Explicit Quick Chat deletion shall retain existing resource and supported local agent-history cleanup, ownership checks, and retry behavior.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.6:** The removal shall add no expiration setting, opt-in switch, duration control, or expiration API.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.7:** Upgrades shall preserve undeleted chats. Cleanup already committed for a previously deleted chat shall remain recoverable.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.8:** Desktop and phone shall retain their existing manual stop, close-confirmation, and restoration behavior.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.9:** Tracked-task, task-session, Office, automation, integration, workspace-deletion, host storage, and agent-profile cleanup policies shall retain their separate behavior.
- **AC-TASKS-QUICK-CHAT-EXPIRATION-003.10:** Deleting an agent profile shall preserve ordinary Quick Chat tasks, sessions, messages, and owned resources. Profile cleanup shall continue deleting configuration chats identified by `config_mode` metadata.

## Out of scope

- Restoring the three conversations deleted on 16 November 2026.
- Changing stop into deletion, removing closure confirmation, or changing the tab layout.
- Preventing process suspension or reclaiming independently eligible resource remnants.
- Changing other cleanup policies, deploying the change, or editing a live database.

## System design

- [Quick Chat manual lifetime](../system-design/quick-chat-expiration-policy.md)
