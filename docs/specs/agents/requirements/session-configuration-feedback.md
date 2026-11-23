---
status: active
system: agents
created: 2026-11-20
owners:
  - kandev
---

# Session configuration feedback requirements

## Overview

When users change a session setting in task chat, they need to know what the agent applied and what needs their attention. Muted system messages show picker outcomes and slash-command receipts in the conversation. [Issue #5376](https://github.com/kdlbs/kandev/issues/5376)

## Requirements

### REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001: Session configuration feedback

**Intent**

Users can check a configuration change and continue the same conversation.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.1:** When the current agent confirms a model, mode, or provider option, task chat shall name the effective values in a muted system message. The user shall be able to reopen the picker and use nearby controls while the message is visible.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.2:** When Kandev only saves a setting, the notice shall say it was saved for agent startup. When the agent reports a different value, the notice shall identify the requested and effective values. When the outcome is ambiguous, the notice shall say it could not be confirmed.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.3:** A rejected change shall show the agent's readable reason when available. A partial application shall identify the effective changes and the failed or unconfirmed setting and value. Users shall be able to retry eligible remaining options without repeating the confirmed changes.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.4:** When a recognized configuration slash command finishes silently, task chat shall show one muted system message under the command in place of the generic empty-turn hint. If the agent acknowledges only part of the request, the receipt shall distinguish the observation from the requested value. If the outcome is ambiguous, the system message shall say it could not be confirmed. Agent text, including rejection detail, shall remain visible in its ordinary chat row. Other empty turns shall retain their existing feedback.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.5:** Feedback shall belong to the initiating task session and change. Startup, restoration, replay, and unrelated reports shall never produce a confirmation. A superseded response shall never confirm a newer selection. Feedback shall stay out of saved conversation history and disappear on reload.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.6:** Desktop and phone shall announce feedback without moving focus or jumping to the latest message. A positive slash receipt shall clear after a brief visible interval. Failures and unconfirmed outcomes shall remain readable and dismissible. Text shall wrap within the transcript on phones, including with the keyboard open. Dismiss and eligible Retry actions shall have touch targets of at least 44 px.
- **AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.7:** Picker changes made before the same next turn in a session shall update one system message, regardless of pauses between selections. The row shall show the latest outcome for each changed setting. Agent startup and each new turn shall close the previous group and keep its row in the live transcript. Later picker changes shall begin a new group. A slash receipt shall stay with its own command.

## Related contracts

- Confirmed provider state, sequential composed settings, and retry are defined by `AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3`. [Dynamic provider model options](dynamic-provider-options.md#req-agents-dynamic-provider-options-004-shared-configuration-flow)
- Supported command metadata and invocation belong to the command autocomplete contract. [Agent command autocomplete](command-autocomplete.md)
- The empty-turn notice owns the general feedback for silent turns. A specialized configuration outcome satisfies that turn's inline feedback. [Empty-turn notice](../../ui/requirements/empty-turn-notice.md)

## Scope

The notice applies to explicit configuration actions in structured task chat. Profile editors, workflow settings, quick chat, configuration chat, CLI passthrough, and Office run output use their own feedback. Provider discovery, invocation syntax, routing policy for ordinary prompts, session settings persistence, and historical turn attribution follow their existing contracts.

ACP agents share the same behavior for the settings they expose and the
configuration commands whose outcome Kandev can attribute. An agent name
alone cannot establish that a setting or command is supported.
