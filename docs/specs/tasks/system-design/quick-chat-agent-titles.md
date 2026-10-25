---
status: draft
system: tasks
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001
created: 2026-08-26
owners:
  - kandev
---

# Quick Chat Agent Titles System Design

## Purpose and boundaries

The task system owns the title, pending state, session owner, MCP mutation, and task update event.
The Quick Chat UI supplies the provisional label and shows the resulting task update.

This design extends the existing generated-title lifecycle to ordinary Quick Chat tasks. It does
not change Configuration Chat, Quick Terminal, or manual tab rename behavior.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001` | [Control flow](#control-flow) |

## Components and responsibilities

- `use-quick-chat-modal.ts` reads `agentGeneratedTaskTitles`. When it creates an ordinary Quick Chat,
  it sends the choice.
- `httpStartQuickChat` keeps the current provisional label. It marks the task as title-pending when
  the request enables agent titles.
- The orchestrator claims the first eligible Quick Chat session through the existing title-owner
  compare-and-set before it starts the eager agent process.
- The executor derives the MCP profile from the pending task and owner session. This profile
  registers `set_task_title_kandev` before the agent accepts a user request.
- The task message handler adds the canonical Kandev context to a pending owner's prompt. If eager
  initialization moved the session to `WAITING_FOR_INPUT`, the same rule applies.
- The existing title mutation publishes `task.updated`. The Quick Chat task-event handler maps the
  accepted task title to the current tab.

## Data and contracts

The Quick Chat create request adds one optional field:

```json
{
  "auto_title": true
}
```

The frontend sends the current `agent_generated_task_titles` preference. An omitted or false field
keeps the current Quick Chat behavior.

When `auto_title` is true, the handler keeps the selected agent label and chat number as the
provisional title. It also adds `agent_title_pending: true` to task metadata. It does not call the
normal prompt-derived title helper because a Quick Chat has no user prompt at creation time.

The existing `agent_title_owner_session_id` field identifies the one owner. The existing
`set_task_title_kandev` schema, authorization, validation, branch handling, and response remain
unchanged.

## Control flow

1. The user selects an agent. The client creates an ordinary Quick Chat with a provisional title
   and the current agent-title preference.
2. The backend persists the task. If the preference is enabled, the task is title-pending.
3. Quick Chat keeps its eager launch. The launch claims title ownership before the process starts.
4. The executor starts the agent with the title-capable task MCP profile. Eager ACP capability
   discovery remains available to the composer.
5. When the user sends a prompt to the pending owner, the message handler adds the Kandev context
   before message persistence and agent dispatch.
6. A structured agent receives the normal hidden context and the title instruction. A passthrough
   agent receives the existing compact instruction before the prompt in its native terminal.
7. If the title remains pending, a later prompt includes the instruction again. This gives a failed
   or ignored call a retry path without assigning another owner.
8. A successful title call clears pending ownership and publishes `task.updated`. The active and
   restored Quick Chat tab names then use the accepted task title.

## Failure and recovery

If task creation or eager launch fails, the existing Quick Chat rollback removes the ephemeral task.
If title ownership cannot be persisted, the launch fails before it exposes the title tool.

If prompt context composition fails, Kandev does not send a prompt with an ambiguous title
capability. The ordinary message error path reports the error.

If the tool is unavailable, ignored, or rejected, the provisional title remains. The pending owner
keeps its capability after resume. A different session cannot become the owner.

A manual tab rename uses the existing task-title update. That update clears pending ownership. The
running MCP server can retain its registered schema, but a later call returns `title_not_pending`.

## Persistence and compatibility

This change uses existing task and session metadata. It adds no database column or migration. Older
clients omit `auto_title` and keep their current behavior.

The provisional and accepted titles survive reload through the existing task row. The pending state
and owner also retain their current restart guarantees.

## Permissions and security

The MCP server injects its task and session identifiers. The agent cannot select another task. The
existing owner compare-and-set and task authorization remain the mutation boundary.

The server creates all Kandev context blocks. The message handler canonicalizes user content before
it stores or sends the prompt. User text cannot forge the title instruction or tool authority.

## Test mapping

- Handler tests cover `auto_title`, provisional titles, disabled behavior, and config exclusion.
- Orchestrator and executor tests cover eager owner claim, title-capable MCP mode, and resume.
- Message-handler tests cover structured context, passthrough context, retry, and canonical storage.
- Frontend tests cover preference projection and the unchanged provisional tab state.
- Playwright uses a deterministic mock-agent title command. It proves that the tab title changes and
  remains after reload.

## Related decisions

- [Bind Agent Title Generation to Pending Tasks](../../../decisions/2026-07-31-agent-generated-task-titles.md)
- [Assign Agent Task Titles to One Session](../../../decisions/2026-08-02-single-owner-agent-task-titles.md)
- [Apply Agent-Generated Titles to Quick Chat](../../../decisions/2026-08-26-quick-chat-agent-titles.md)
