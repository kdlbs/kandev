---
status: draft
system: office
created: 2026-04-25
owners:
  - cfl
---
# Office: Personal Assistant Agent, Channels & Agent Memory Requirements

## Overview

Users interact with kandev through a web UI, but many want to manage their agent fleet from wherever they already are - Telegram while commuting, Slack during meetings, or email on mobile. There is no way to message an agent outside of the kandev web app, no way for an agent to proactively reach out to the user through external channels, and no way for agents to accumulate knowledge across sessions.

## Requirements

### REQ-OFFICE-ASSISTANT-001: Office: Personal Assistant Agent, Channels & Agent Memory

**Intent:** Users interact with kandev through a web UI, but many want to manage their agent fleet from wherever they already are - Telegram while commuting, Slack during meetings, or email on mobile. There is no way to message an agent outside of the kandev web app, no way for an agent to proactively reach out to the user through external channels, and no way for agents to accumulate knowledge across sessions.

#### Acceptance criteria

- **AC-OFFICE-ASSISTANT-001.1:** A personal assistant agent (`role=assistant`) is the user's primary conversational interface outside the web UI.
- **AC-OFFICE-ASSISTANT-001.2:** The assistant maintains long-running conversational context through channel tasks, unlike worker agents which execute tasks and exit.
- **AC-OFFICE-ASSISTANT-001.3:** Channels SHALL bridge external chat platforms (Telegram, Slack, Discord, email, webhook) to an agent instance via task comments.
- **AC-OFFICE-ASSISTANT-001.4:** Inbound messages from a channel become comments on the channel task; agent replies are relayed back through the platform.
- **AC-OFFICE-ASSISTANT-001.5:** The assistant can answer state questions, delegate work, relay status, and run proactive routines.
- **AC-OFFICE-ASSISTANT-001.6:** Agents with the memory skill SHALL persist knowledge across sessions in a per-agent memory store.
- **AC-OFFICE-ASSISTANT-001.7:** Agents with `can_manage_own_skills` SHALL create/edit skills in the workspace skill registry, subject to the approval flow when configured.
- **AC-OFFICE-ASSISTANT-001.8:** **GIVEN** a personal assistant with a Telegram channel configured, **WHEN** the user sends "what's the status of the auth task?" via Telegram, **THEN** the message arrives as a comment on the channel task, the assistant is woken, reads the task state, and replies via Telegram with a status summary.

## System design

The migrated technical source is split into [part 1](../system-design/assistant.md).
