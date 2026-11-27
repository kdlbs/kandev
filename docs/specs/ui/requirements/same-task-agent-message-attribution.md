---
status: draft
system: ui
created: 2026-09-27
owners:
  - kandev
---

# Same-Task Agent Message Attribution Requirements

## Overview

People reading a task with several agent sessions need to tell which session
sent each peer message. The task system already records sender task and session
identity. UI owns how that identity appears in transcript and queue chips.

## Terminology

- **Sender session label:** The readable label shown for the sending session in
  the task's session controls. A custom session name takes precedence over a
  derived model or agent label.
- **Same-task message:** An agent-origin message whose sender task is the task
  that contains the receiving session.

## Requirements

### REQ-UI-SAME-TASK-AGENT-ATTRIBUTION-001: Identify the sending agent session

**Intent:** Let a reader distinguish messages sent by different agents working
on the same task without repeatedly reading an identical task title.

**User story:** As a person reading a multi-agent task, I want each peer-message
chip to name its sender session so that I can follow the conversation.

#### Acceptance criteria

- **AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.1:** When a same-task agent message
  has a resolvable sender session, its transcript chip shall lead with that
  session's readable label instead of the task title. Different sender session
  labels shall remain distinguishable.
- **AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.2:** The same attribution shall appear
  while a same-task agent message is queued and after it enters the transcript.
  A session rename or model-label change shall update a loaded sender's chip.
- **AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.3:** The complete sender label and
  task title shall remain available without relying on truncated chip text.
  On a phone, the visible chip shall identify the sender without hover and fit
  within the message or queue surface without horizontal page overflow.
- **AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.4:** If a same-task sender session is
  unavailable, the chip shall use a captured session name when present. If no
  readable name is available but the sender session ID is known, it shall show
  a localized agent fallback with a stable short session identifier. If the
  sender session ID is absent, it shall retain the existing task attribution.
- **AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.5:** A message sent from another task
  shall continue to identify its source task and preserve its source-task
  navigation. The chip shall not change the message body or delivery behavior.

## Out of scope

- Changing agent-message routing, sender authorization, or stored sender IDs.
- Renaming sessions or changing how session tabs choose their labels.
- Introducing an agent name lookup across unloaded tasks or workspaces.

## Implementation plans

- [Same-task agent message chips](../../../plans/same-task-agent-message-chips/plan.md).
