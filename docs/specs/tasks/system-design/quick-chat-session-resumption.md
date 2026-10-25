---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-001
created: 2026-08-29
owners:
  - kandev
---

# Quick Chat Session Resumption System Design

## Purpose and boundaries

The task system owns recovery of persisted Quick Chat task sessions after the agent runtime stops
or the backend restarts. The shared Quick Chat dialog remains a presentation over those task
sessions. It does not own a separate runtime lifecycle. This design covers ordinary,
configuration, and passthrough Quick Chat conversations on desktop and mobile. It does not change
the responsive dialog composition, session protocol, or backend resume semantics.

## Requirement mapping

| Requirement                           | Design sections                                                              |
| ------------------------------------- | ---------------------------------------------------------------------------- |
| `REQ-TASKS-QUICK-CHAT-EXPIRATION-001` | Session identity, open-time control flow, failure and recovery, verification |

## Session identity

Every persisted `QuickChatSession` carries its backing task ID and session ID. Creation responses,
boot hydration, reconnect list responses, and task lifecycle events preserve both identities.
`taskSessions.items[sessionId].task_id` is the fallback when an older in-memory descriptor does not
yet contain its task ID. Client-only setup tabs have no backing task and never render the persisted
session view.

## Open-time control flow

`QuickChatSessionView` first ensures that the persisted task-session row is available. While the row
is absent, it passes a null task ID to the shared resumption hook so a status request cannot race the
authoritative HTTP hydration. Once the row exists, it resolves the backing task ID from the Quick
Chat descriptor or the hydrated task-session row and mounts the shared
`useSessionResumption(taskId, sessionId)` lifecycle for the visible conversation.

The shared hook owns the remaining lifecycle:

1. Wait for the WebSocket connection and request `task.session.status`.
2. Leave an already-running agent unchanged.
3. When the session needs resume and is resumable, apply
   `preventAutoStartAgentOnOpen`. Resume automatically when the preference is disabled. Otherwise,
   retain the stopped state and the existing composer hint behavior.
4. Fall back to workspace restoration when a runtime resume fails according to the existing task
   session recovery policy.
5. Accept agent startup and capability events through the existing session subscriptions so model,
   command, and configuration controls become available.

The hook is mounted before selecting the ordinary chat, configuration chat, or passthrough terminal
presentation. All persisted conversation kinds therefore use one recovery policy. Changing the
active tab changes the task/session inputs, resets the hook's guarded attempt state, and checks the
newly visible session once.

## Failure and recovery

If the descriptor lacks a task ID, session hydration can supply it and trigger the status check on a
later render. If neither source supplies a task ID, no resume request is sent. If the active tab
changes while hydration is in flight, the request is cancelled and its session ID becomes retryable
when the tab is revisited. If a status path inserts a placeholder row before the HTTP response,
hydration still merges the authoritative row. The existing tab resync then reconciles a deleted or
inaccessible conversation. Stale status responses remain subject to the shared monotonic
session-state guards.

The change adds no new error copy. Existing session resumption and workspace-restoration errors
retain their current handling.

## Responsive behavior

Desktop, tablet, and mobile Quick Chat use the same `QuickChatSessionView` and lifecycle hook. The
change does not alter layout, navigation, focus, scroll ownership, touch targets, safe-area
handling, or tab controls. The existing full-screen mobile Quick Chat surface remains the mobile
exemplar and receives the same restored runtime state as desktop.

## Verification

- A focused component test proves that a persisted Quick Chat resolves its backing task identity
  and mounts shared session resumption only after hydration, including descriptor precedence and the
  hydrated-row fallback.
- Deferred hydration tests prove that placeholder status rows do not cancel the authoritative row
  and that a switched-away tab can retry hydration when revisited.
- Existing `useSessionResumption` tests continue to prove status, automatic-resume preference,
  workspace fallback, and stale-state behavior.
- A desktop Playwright scenario restarts the backend, reloads the application, opens the restored
  Quick Chat tab, and proves the agent resumes and dynamic session model controls return.
- No new mobile Playwright scenario is required because the repair changes only shared state and
  lifecycle behavior inside the existing responsive component. It introduces no viewport-specific
  presentation or interaction.

## Related decisions

- [Typed Utility Chats Share the Quick Chat Session Model](../../../decisions/2026-07-14-typed-utility-chat-sessions.md)
