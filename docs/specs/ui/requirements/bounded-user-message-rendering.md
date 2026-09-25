---
status: active
system: ui
created: 2026-09-18
owners:
  - kandev
---

# Bounded user-message rendering

## Overview

Large user messages need bounded previews so users can navigate the conversation.
UI owns this presentation contract. Tasks and agents retain message storage and delivery ownership.

## Requirements

### REQ-UI-BOUNDED-USER-MESSAGE-001: Bounded user-message previews

**Intent:** Keep large messages accessible without rendering their complete contents during conversation navigation.

#### Acceptance criteria

- **AC-UI-BOUNDED-USER-MESSAGE-001.1:** When a user message exceeds 200 logical lines or 16,000 UTF-16 code units, its preview shall contain no more than either limit. This applies to transcript bubbles, queued previews, and pinned prompts. A logical line ends at LF, CRLF, or CR.
- **AC-UI-BOUNDED-USER-MESSAGE-001.2:** A shortened preview shall identify omitted content and offer a full-text download. The download shall preserve the selected message representation exactly, including its final character.
- **AC-UI-BOUNDED-USER-MESSAGE-001.3:** Raw mode, queued-row expansion, pinned-prompt expansion, and workflow-instruction disclosure shall retain the rendering limit. No preview action shall mount the complete oversized message.
- **AC-UI-BOUNDED-USER-MESSAGE-001.4:** Preview shortening shall not alter stored messages, agent input, existing copy behavior, queue editing, or attachment data. Ordinary messages shall retain Markdown, mentions, file links, and instruction disclosure.
- **AC-UI-BOUNDED-USER-MESSAGE-001.5:** Desktop and phone users shall have a visible, keyboard-accessible download action. Phone targets shall measure at least 44 CSS pixels. Previews shall not cause document-level horizontal overflow.
- **AC-UI-BOUNDED-USER-MESSAGE-001.6:** With a 3,921-line generated log selected, users shall switch sessions and send a subsequent small message successfully. The regression scenario shall produce no WebSocket request-timeout errors.

## Compatibility

Pinned previews retain Markdown and the existing desktop-only navigation contract in
[last-prompt pinning](last-prompt-pinning-regressions.md).
The size limit governs source content passed to rendering, including expanded previews.
The phone retains its existing scroll-to-last-prompt control.

## Out of scope

- Message admission limits, persisted truncation, and automatic attachment conversion.
- Provider context limits, agent-stream recovery, and RPC timeout changes.
- Assistant output, repository Markdown, share snapshots, and prompt-history plugin views.

## Implementation plans

- [Oversized user-message rendering](../../../plans/oversized-user-message-rendering/plan.md)
