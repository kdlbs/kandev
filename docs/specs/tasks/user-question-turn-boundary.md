---
status: building
created: 2026-08-22
owner: kandev
---

# User Question Turn Boundary

## Why

An agent can ask for required user input and then continue the same turn before
the user answers. That behavior lets the agent make decisions without the input
it declared necessary, especially when an MCP client returns early because the
question wait timed out or disconnected.

## What

- The first-turn Kandev MCP overview for a normal task identifies
  `ask_user_question_kandev` as a hard user-input barrier.
- After calling `ask_user_question_kandev`, the agent does not call another tool,
  continue working, or produce a final response until the tool returns the
  user's answers.
- If the call returns without completed user answers, including a timeout,
  disconnect, or pending result, the agent ends the turn immediately. It does
  not infer an answer or continue the task.
- When the call returns completed user answers, the agent can continue using
  those answers.
- The guidance appears only when the task MCP profile exposes
  `ask_user_question_kandev`. Existing autopilot parent-question and no-question
  profiles keep their current guidance and capability boundaries.

## Failure modes

- If the MCP wait ends without answers, the prompt requires a fail-closed turn
  boundary. The existing clarification lifecycle remains responsible for
  preserving or resuming the pending question.
- If the user rejects the question bundle, the structured rejection is a
  completed user response. The agent can handle that response without inventing
  a selected option.

## Scenarios

- **GIVEN** a normal task whose MCP profile exposes
  `ask_user_question_kandev`, **WHEN** Kandev builds the first-turn system
  context, **THEN** the tool overview says not to make another tool call,
  continue work, or provide a final response until user answers return.
- **GIVEN** the agent calls `ask_user_question_kandev`, **WHEN** the tool returns
  completed answers, **THEN** the agent can continue the task using those
  answers.
- **GIVEN** the agent calls `ask_user_question_kandev`, **WHEN** the call returns
  without completed answers, **THEN** the agent ends the turn immediately and
  performs no further task work.
- **GIVEN** an autopilot child or root task, **WHEN** Kandev builds its first-turn
  system context, **THEN** the existing parent-question or no-question guidance
  remains unchanged and the user-question guidance is absent.

## Out of scope

- Changing the `ask_user_question_kandev` schema, response envelope, timeout,
  keepalive, persistence, or resume behavior.
- Changing configuration-mode, Office-mode, or external MCP prompt surfaces.
- Adding model-specific prompt variants or runtime enforcement that cancels an
  agent immediately after the tool call.
