---
status: active
system: tasks
created: 2026-09-29
owners:
  - kandev
---

# Configuration Chat Session Restart Requirements

## Overview

Users need to recover a broken Configuration Chat without leaving its Settings
panel. Restart stops and removes the current conversation and starts a blank
replacement. Tasks owns this capability because it changes the durable utility
task/session lifecycle, including the shared Quick Chat representation.

This is an explicit replacement operation, separate from the automatic recovery
in [Quick Chat session resumption](../system-design/quick-chat-session-resumption.md).
It extends the single-conversation contract in
[Quick Chat persistence](quick-chat-expiration.md).

## Requirements

### REQ-TASKS-CONFIG-CHAT-RESTART-001: Replace a configuration conversation

**Intent:** Recover an unusable configuration agent with a fresh session while
keeping the user in the same configuration workflow.

#### Acceptance criteria

- **AC-TASKS-CONFIG-CHAT-RESTART-001.1:** The Configuration Chat panel shall expose
  Restart session in its header before Expand and Close. It shall be available
  for a persisted configuration conversation regardless of whether that agent
  is running, waiting, failed, or stopped. It shall be disabled during a start
  or restart and when there is no persisted conversation to replace.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.2:** Activating Restart session shall explain
  that the current conversation will be deleted and ask for confirmation.
  Cancel shall preserve the session, transcript, and unsent input. Confirmed
  restart shall clear conversation context, pending questions, queued prompts,
  and unsent input; it shall not replay an earlier prompt or undo configuration
  changes already made.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.3:** Restart shall affect only the selected
  configuration conversation in the authorized workspace. An ordinary chat,
  tracked task, wrong task/session pair, inaccessible target, or stale target
  shall not cause any other conversation to be stopped or deleted.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.4:** The replacement shall use the current
  conversation's agent profile and effective executor, retain configuration
  capabilities, and have new task and session identities. Known missing,
  disabled, or incompatible selections shall produce an actionable error
  before deleting the old conversation; restart shall not silently choose
  another profile. Successful restart shall explicitly start the agent with
  no user prompt, including when automatic start on opening a chat is disabled.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.5:** The old agent shall be stopped before
  its conversation is deleted and before the replacement starts. Already
  absent runtimes shall not prevent recovery when absence is established.
  Failure to stop or delete shall prevent replacement creation and shall be
  reported in the panel; it shall not be presented as a successful restart.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.6:** If replacement creation or startup
  fails after deletion, the panel shall say that the old conversation has been
  cleared and offer recovery. A successfully created replacement shall remain
  reachable for retry. An uncertain network outcome shall be reconciled before
  another creation attempt; uncertainty shall not silently create a duplicate.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.7:** Repeated clicks and concurrent restart
  requests for the same captured conversation shall produce at most one
  replacement. The panel shall announce progress and prevent sending to the
  retiring conversation while restart is in progress.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.8:** The panel and expanded Quick Chat shall
  agree on the replacement identity and blank history. Reload shall restore
  only the replacement. Closing the panel or changing workspace after
  confirmation shall not reopen it, activate another workspace, or abandon an
  accepted replacement as an invisible task. Unrelated chats shall be preserved.
- **AC-TASKS-CONFIG-CHAT-RESTART-001.9:** Phone and desktop users shall complete
  the same restart and cancellation flow with localized copy and accessible
  button names. Phone confirmation shall use the established bottom confirmation
  surface. Header controls shall remain fixed and reachable, with at least
  44px phone/coarse-pointer hit targets, no horizontal clipping, predictable
  focus return, and visible progress/error feedback.

## Out of scope

- Restarting Kandev, the executor host, ordinary Quick Chats, or tracked tasks.
- Preserving, summarizing, exporting, or replaying the discarded conversation.
- Rolling back configuration changes performed by the agent.
- Multiple configuration conversations per workspace or a new tab store.
- A new restart button in the expanded Quick Chat toolbar.
- Global changes to normal task deletion, automatic session recovery, or runtime
  cleanup policy.

## Design and delivery

- [System design](../system-design/configuration-chat-restart.md)
- [Implementation plan](../../../plans/configuration-chat-restart/plan.md)
