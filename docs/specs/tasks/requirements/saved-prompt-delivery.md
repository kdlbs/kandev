---
status: active
system: tasks
created: 2026-09-01
owners:
  - kandev
---

# Saved Prompt Delivery Requirements

## Overview

Chat users can reference a saved prompt by its `@name`. The agent must receive
the current saved definition, including on the first request in an eager Quick
Chat session.

The task system owns this contract because it persists each accepted message
and sends the same message to the session agent. The UI owns mention selection
and display, but it does not own saved-prompt authority.

## Terminology

- **Saved-prompt reference:** A visible `@name` that matches a saved prompt.
- **Trusted expansion:** Hidden context that Kandev creates from the current
  saved-prompt record.
- **Structured session:** An agent session that accepts ACP prompt content. A
  passthrough session writes visible text to a terminal instead.

## Requirements

### REQ-TASKS-SAVED-PROMPT-DELIVERY-001: Saved Prompt Delivery

**Intent:** A structured chat agent receives the current definition for each
saved-prompt reference without trusting browser-supplied definitions.

**User story:** As a chat user, I want `@name` to apply my saved prompt, so that
the agent follows the instructions that I selected.

#### Acceptance criteria

- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.1:** When a user sends a direct message
  with a known saved-prompt reference, the structured agent shall receive the
  visible reference and one hidden expansion from the current saved record.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.2:** When the direct message is the first
  request in an eagerly started Quick Chat, the expansion shall reach the agent
  after ACP initialization and before the agent processes the request.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.3:** The first Quick Chat request shall
  have the same behavior when the user selects a prompt or types its exact
  `@name`.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.4:** When browser content contains a
  prompt definition or an expansion block, Kandev shall remove that content and
  use only the current saved record.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.5:** When lookup fails or no saved prompt
  matches, Kandev shall send the visible reference as ordinary text. It shall
  not send a browser-supplied definition.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.6:** The stored user message shall contain
  the same canonical prompt-reference context that Kandev sends to the agent.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.7:** Desktop and phone Quick Chat shall
  provide the same saved-prompt outcome through their existing composer.
- **AC-TASKS-SAVED-PROMPT-DELIVERY-001.8:** A passthrough session shall keep the
  current literal terminal behavior and shall not receive hidden prompt
  expansions.

## Out of scope

- Changes to saved-prompt creation, editing, naming, or storage.
- Changes to mention ranking, suggestion geometry, or Quick Chat layout.
- Saved-prompt expansion for queued-message editing or queue merging.
- Saved-prompt expansion in terminal and passthrough sessions.

## System design

[Saved Prompt Delivery](../system-design/saved-prompt-delivery.md)
