---
status: shipped
created: 2026-07-20
owner: cfl
---

# Agent-message inline comments

## Why

Users need to point an agent at one exact part of an ordinary settled reply without copying prose into a new prompt. Inline comments turn a message selection into durable, visible prompt context while keeping the existing chat and queue delivery paths intact.

## What

- In Task Chat and Quick Chat, users can select text from the ordinary prose portion of a settled agent reply (`message` or `content`). Rich blocks rendered beside that prose remain outside the selectable surface.
- Current streaming replies, user messages, raw views, thinking, tools, status/progress, `agent_plan`, and rich-block content do not expose inline-comment controls.
- Selection first reveals the same accent comment affordance used by plan selection. Activating it opens the shared plan-comment popover on desktop or the equivalent coarse-pointer/mobile bottom Drawer. The keyboard shortcut and Add/Run action hierarchy match plan comments.
- **Add** stores a pending agent-message comment in browser `sessionStorage`, renders the same accent highlight and comment badge used in plans when its message is mounted, and adds a removable context chip to the composer.
- Activating a pending comment's highlight or badge reopens the editor so the feedback can be updated or deleted.
- **Run** sends the single comment immediately when the session is idle or appends it to the existing queue when the agent is busy. Successful delivery removes the pending comment.
- Sending normal composer content includes all pending agent-message comments in Markdown and removes them only after successful direct or passthrough delivery. Failed sends preserve draft and context.
- Highlights identify selections using message-local text anchors, not global DOM positions. Virtualized/unmounted chat rows restore highlights when remounted.
- Existing diff, plan, PR-feedback, walkthrough, attachment, file, prompt, and passthrough context behavior remains unchanged.

## Data model and persistence

`AgentMessageComment` extends the frontend comment union with `source: "agent-message"`, session id, message id, selected text, user feedback, pending/sent status, and a message-local anchor containing start/end text offsets plus nearby prefix/suffix context. Comments are persisted per session using the existing browser `sessionStorage` comment store. No backend table, API, WebSocket schema, or `task_comments` change is in scope.

## Markdown and delivery

Agent-message comments format as a Markdown section containing the selected response text and user feedback as blockquotes. All existing comment types retain their current format. Normal, queued, immediate, and passthrough sends use the existing message/queue APIs; only the final client-side message composition changes.

## Mobile contract

- Desktop entry point: select settled agent prose, activate the plan-style comment affordance, then act from the shared anchored plan popover.
- Mobile entry point: select settled agent prose, activate a touch-sized version of the same affordance, then act from an inset bottom Drawer following existing phone context surfaces.
- Drawer hierarchy and language mirror the desktop popover: selected text preview, feedback input, attached Add/Run actions, and edit/delete controls. One internal scroll owner; controls have touch-sized hitboxes and safe-area bottom padding.
- Shared state and delivery handlers are reused. Only overlay presentation differs by pointer/viewport.
- Mobile E2E proves selection, Drawer presentation, Add, composer context, and Run delivery outcome without document horizontal overflow.

## Scenarios

- **GIVEN** a settled agent prose reply, **WHEN** the user selects non-empty text, **THEN** a plan-style comment affordance appears for that selection.
- **GIVEN** a visible selection affordance, **WHEN** the user activates it, **THEN** the plan-style editor opens with the exact selected text.
- **GIVEN** a current streaming reply or non-prose/rich message, **WHEN** the user selects text, **THEN** no inline-comment action surface opens.
- **GIVEN** feedback and a selection, **WHEN** the user chooses **Add**, **THEN** the comment is persisted in sessionStorage, highlighted in the message, and shown as a composer chip.
- **GIVEN** a pending comment, **WHEN** the user reloads the tab or the virtualized row remounts, **THEN** state and highlight restore for the same message-local anchor.
- **GIVEN** a pending comment highlight or badge, **WHEN** the user activates it, **THEN** they can update or delete the pending feedback.
- **GIVEN** a pending comment, **WHEN** the user chooses **Run** while idle, **THEN** one Markdown message is sent immediately and the comment is removed after success.
- **GIVEN** a pending comment, **WHEN** the user chooses **Run** while busy, **THEN** one Markdown queue entry is appended and the comment is removed after success.
- **GIVEN** pending comment context and composer text, **WHEN** the user submits successfully, **THEN** the Markdown context is included and pending comments are cleared.
- **GIVEN** a failed direct, queued, or passthrough send, **THEN** pending context and draft remain available.

## Out of scope

- Backend/API/schema/task-comment persistence.
- Comments on user messages, streaming replies, tools, status, thinking, plans, rich-block content, or raw views.
- Cross-message selections or server-side highlight rendering.
- Editing an already-sent inline comment.
