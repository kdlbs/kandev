---
status: draft
system: ui
requirements:
  - REQ-UI-SAME-TASK-AGENT-ATTRIBUTION-001
---

# Same-Task Agent Message Attribution System Design

## Purpose and boundaries

The UI projects existing sender metadata into a chip that identifies the
sending session when the source and recipient belong to one task. Task message
delivery and session identity remain owned by the task system. The frontend
does not infer a sender from message text or trust a client-supplied name as a
new authorization signal.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SAME-TASK-AGENT-ATTRIBUTION-001` | [Data and label resolution](#data-and-label-resolution), [Rendering](#rendering), [Responsive behavior](#responsive-behavior), [Verification](#verification) |

## Data and label resolution

`handleMessageTask` already records `sender_task_id`, `sender_task_title`,
`sender_session_id`, and optionally `sender_session_name` through
`wrapAgentMessage`. It permits a task to message a distinct sibling session
when the caller supplies that destination in `session_id`; targeting the
sender's own session remains invalid. `ChatMessage` receives the destination `Message.task_id`;
`QueuedGhostMessage` receives the destination `QueuedMessage.task_id`. Pass
that destination task ID into `SenderTaskBadge`. Compare task IDs exactly;
the currently active global task is not reliable in Threads or preview layouts.

For a same-task message with a sender session ID, resolve that session from
`taskSessions.items` only if its `task_id` matches the destination task. Use the
same label precedence as `SessionTab` via `resolveSessionTabTitle`: custom
`session.name`, current/active model display name, profile label, then the
session profile's model snapshot. Extract the existing tab selector logic into
a shared pure helper or selector so the badge and tab cannot drift. Keep the
current send-time `sender_session_name` metadata as a fallback when the live
session is unavailable. If both are absent, display a localized generic agent
label plus a stable short form of `sender_session_id`; show the full ID in the
chip's contextual tooltip or accessible label. A missing sender session ID
uses the existing task title path.

No API field, database migration, or historical metadata rewrite is needed.
The label is a presentation projection. Loaded-session model or name changes
refresh it reactively; a message with an unloaded sender retains only its
existing name snapshot or stable ID fallback.

## Rendering

`SenderTaskBadge` remains the single renderer for both the transcript and
queued ghost rows. Its same-task branch shows the sender label in the visible
chip as a semantic button. Enter, Space, click, or tap opens the full context,
which names the sender and task. The button's accessible name also includes
that context. On coarse-pointer screens the button has a 44px minimum hit
height. Cross-task badges retain their existing source-task link and
title/session presentation. Text is truncated in the compact chip but remains
complete in contextual text and accessible names.
The message body and `<kandev-system>` wrapping remain unchanged.

All new visible strings use the `task` i18n namespace in the six maintained
locales and the pseudo-locale. The task title and session label are domain data
and are not translated.

## Responsive behavior

The chip stays inline above the message bubble or queue row on desktop and
phone. The existing phone transcript and queue are the nearest shipped mobile
surfaces. Tapping a long sender chip opens an anchored popover with the full
sender and task context; the popover stays within the viewport. The transcript
or queue keeps its current scroll owner and safe-area behavior.

## Failure and compatibility

Historical messages can lack `sender_session_id`. Those keep task attribution.
Messages with a sender ID but an unavailable session remain distinguishable
through their stable short ID; they never claim to know a model or custom name
that is no longer available. Cross-task badges continue to use the live task
title when loaded and their recorded title when not.

## Verification

Component tests cover same-task custom and model labels, updates, missing
sender sessions, transcript and queue consistency, cross-task behavior, and
full contextual text. Browser tests send through `message_task_kandev` with an
explicit sibling `session_id`, verify the persisted queue metadata, and inspect
the rendered chip. The desktop test opens the full context through normal
keyboard navigation; the phone test taps the chip and checks that its popover
and the page remain within the viewport. The backend sibling-session test
covers the persisted queue path independently of the browser.
