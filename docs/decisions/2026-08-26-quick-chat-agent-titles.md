# ADR-2026-08-26-quick-chat-agent-titles: Apply Agent-Generated Titles to Quick Chat

**Status:** accepted
**Date:** 2026-08-26
**Area:** backend, frontend, protocol

## Context

Ordinary Quick Chat tasks start with labels such as `Agent - Chat 2`. The labels do not describe the
user's request. Users must rename each tab manually to make several chats easy to identify.

Normal prompt-first tasks use one pending-title owner. The owner receives a title instruction and
the task-bound `set_task_title_kandev` tool. Quick Chat launches its agent before the first user
prompt to discover ACP commands and models. As a result, it does not use the normal first-turn
prompt path.

## Decision

If the user's `agent_generated_task_titles` preference is enabled, apply the existing pending-title
lifecycle to ordinary Quick Chat tasks.

Keep the current Quick Chat label as the provisional title. Add `agent_title_pending: true` at task
creation. The eager launch claims the owner before the agent process starts. The owner receives a
title-capable MCP profile from the start.

When a user sends a prompt to the running pending owner, add the canonical Kandev context. Structured
agents receive the hidden context. Passthrough agents receive the existing compact title instruction.
Repeat this treatment while the title remains pending. Stop after an agent or user title update
clears the pending state.

Keep Configuration Chat outside this lifecycle. Its fixed purpose and restricted MCP profile remain
authoritative. Quick Terminal has no task agent and also remains outside this lifecycle.

Use the existing title mutation, task event, and generated-branch handling. The Quick Chat tab reads
the accepted title from `task.updated` and from later server restoration.

## Consequences

- Ordinary Quick Chat tabs gain request-specific names without another title-generation request.
- Eager agent initialization and composer capability discovery remain unchanged.
- The same preference controls agent titles for normal tasks and ordinary Quick Chat tasks.
- If the agent ignores the instruction or a title call fails, the provisional label remains.
- The title instruction can repeat while pending. A successful or manual title update stops it.
- Repository-backed Quick Chats use the existing generated-branch rename and preservation rules.
- Older clients omit the new Quick Chat request field and retain their current behavior.

## Alternatives Considered

1. **Delay the agent launch until the first prompt.** Rejected because Quick Chat uses eager ACP
   initialization for commands, models, and modes.
2. **Generate a title with a separate utility request.** Rejected because it adds latency, cost, and
   another error path.
3. **Keep manual tab rename as the only title path.** Rejected because it repeats work that the task
   title capability already performs.
4. **Register the title tool without pending ownership.** Rejected because another session can
   overwrite a user title or compete for one task.
5. **Enable the capability for Configuration Chat.** Rejected because its fixed identity and
   restricted configuration MCP profile have a different contract.
