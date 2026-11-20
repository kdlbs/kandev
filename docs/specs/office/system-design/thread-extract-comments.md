---
status: draft
system: office
requirements:
  - REQ-OFFICE-THREAD-EXTRACTS-001
---

# Office Thread Extract Comments System Design

## Purpose and boundaries

Office owns this design because the feature adds feedback to Office task comments and uses Office's comment-routing lifecycle. It reuses the shared web comment-selection surface and store. The shared comments package remains the owner of its generic types and persistence actions.

The design adds no database table, API route, WebSocket event, or plugin API. Existing task comment storage and routing remain authoritative after submission.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-THREAD-EXTRACTS-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `CommentExtractSurface` adapts an agent-authored Office `TaskComment` to the shared `MessageCommentSurface`. It provides the task ID, comment ID, author name, and read-only state.
- `MessageCommentSurface` captures selected text and its anchor. Its popover or touch drawer accepts the remark and adds the extract when the user chooses Add.
- `useThreadExtracts` reads and updates drafts in `useCommentsStore`. It scopes Office drafts with `office-task:<taskId>` and supplies the composer chips.
- `TaskChat` composes pending extracts with typed text and submits one ordinary user comment through `createComment`.
- The Office dashboard handler stores the resulting comment in `task_comments`. The existing scheduler applies mention routing. The existing `office.comment.created` event causes connected task views to refresh their comments.

## Data and contracts

An extract uses the shared `AgentMessageComment` shape. It stores the source comment ID, selected text, text anchor, remark, and the source author's display name. The author name marks an Office source comment; an extract without it follows the normal chat formatting path.

Pending drafts are stored in browser `sessionStorage` under the task scope. They are available after a page reload in the same tab. They do not sync to other tabs or devices. After submission, the formatted Markdown body is stored as an ordinary Office task comment.

The existing dashboard API accepts `POST /api/v1/office/tasks/:id/comments` with a body that identifies a user-authored comment. The backend authorizes and stores that comment through the existing dashboard service. The existing WebSocket event and task comment list refresh provide updates to connected views.

## Control flow

1. The user selects text in an agent-authored Office comment. The selection surface records the source comment and text anchor.
2. The user enters a remark and chooses Add. The comments store records the extract under the Office task scope and persists it in `sessionStorage`.
3. The composer shows the pending extract with other typed text. On submit, it snapshots the extracts and formats the source passage as a quote, followed by the user remark.
4. The frontend escapes mention syntax and `agent://` links in Office source passages. It does not escape the user's remark or typed feedback. The backend mention resolver ignores escaped source tokens and continues to resolve new user mentions.
5. The dashboard handler stores one normal task comment. The existing scheduler and WebSocket paths process and publish it.
6. On success, the store removes only rows that still equal the submitted snapshot. Edits and extracts added during the request remain pending. On failure, pending rows stay available.

## Failure and recovery

If the POST fails, the composer keeps its pending extracts and typed text and shows the existing send error. The user can retry. If the user edits an extract while the POST is pending, success for the older snapshot does not remove the newer draft.

Read-only task state disables extract creation. When the surface has no Run callback, the touch-drawer shortcut follows the Add path so it cannot close while discarding the remark.

## Persistence

The browser comment store owns unsent Office extracts in task-scoped `sessionStorage`. The Office task comment repository owns submitted comment bodies. There is no new migration or server-side draft lifecycle.

## Security

Agent comment text is untrusted source content. Office quote formatting prevents source mentions and `agent://` links from becoming new routing instructions. The backend still resolves explicit mentions in user-written feedback. The formatter applies this protection only when the extract has an Office source author, so normal chat extracts retain their existing text.

The existing dashboard authorization and task/workspace relationship checks protect comment writes. The feature adds no new permission path.

## Observability

The existing comment request, scheduler routing, comment-created event, and frontend refresh remain the observability path. The feature adds no metric or event type.
