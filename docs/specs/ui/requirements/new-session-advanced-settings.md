---
status: active
system: ui
created: 2026-10-26
owners:
  - kandev
---

# New session advanced settings

## Overview

New Agent and handoff use the compact Advanced settings interaction already
established by New Task. UI owns the launch-form disclosure and overlay
presentation. Agents retains MCP selection, inheritance and runtime semantics
under [workspace MCP configuration](../../agents/requirements/workspace-mcp-configuration.md).
This package complements [agent launch prompt composer](agent-launch-prompt-composer.md).

## Requirements

### REQ-UI-NEW-SESSION-ADVANCED-001: Compact optional configuration

#### Acceptance criteria

- **AC-UI-NEW-SESSION-ADVANCED-001.1:** Each fresh opening shall show profile,
  context and prompt controls, followed by a collapsed Advanced settings
  disclosure before the launch actions, matching New Task's visual hierarchy.
- **AC-UI-NEW-SESSION-ADVANCED-001.2:** Expanding Advanced settings shall reveal
  one aligned MCP servers label and selected-additions count control. Search,
  helper paragraphs and server cards shall appear only inside the picker.
- **AC-UI-NEW-SESSION-ADVANCED-001.3:** Selecting the count control shall open a
  bounded anchored picker on fine-pointer desktop and a touch sheet on phone
  or coarse-pointer tablet. The list shall not expand the launch form.
- **AC-UI-NEW-SESSION-ADVANCED-001.4:** Collapsing Advanced settings or closing
  the picker shall preserve the current prompt, attachments, context, profile
  and MCP choices for the open form. Disclosure and picker actions shall not
  create a session or write selections to an existing session.

### REQ-UI-NEW-SESSION-ADVANCED-002: Accessible selection continuity

#### Acceptance criteria

- **AC-UI-NEW-SESSION-ADVANCED-002.1:** The picker shall retain inherited-origin
  summaries, session-addition choices and current loading, empty, unavailable
  and disabled feedback. The count shall represent explicit session additions.
- **AC-UI-NEW-SESSION-ADVANCED-002.2:** Keyboard users shall open the disclosure
  and picker, search and select, then close the picker with Escape and return
  focus to its trigger without closing the launch dialog. Touch controls shall
  provide at least 44 by 44 CSS-pixel targets.
- **AC-UI-NEW-SESSION-ADVANCED-002.3:** At narrow phone widths and on coarse
  tablets, the picker shall remain within the viewport with one list scroll
  owner, reachable close action and no horizontal document overflow. Expanded
  locale copy and long server names shall wrap without overlapping controls.
- **AC-UI-NEW-SESSION-ADVANCED-002.4:** Starting the agent shall submit the same
  explicit selections and prompt as the existing creation contract. Cancel and
  reopen shall follow existing draft reset behavior; handoff shall retain its
  existing profile and explicit-context rules.

## Out of scope

Existing-session Customize, inherited exclusions, configuration grants, native
MCP packages, profile settings, New Task behavior, backend APIs and lifecycle.
No new global preference for disclosure state.

## Implementation Plans

- [New session advanced settings](../../../plans/new-session-advanced-settings/plan.md).
