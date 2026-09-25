---
status: active
system: ui
created: 2026-09-15
owners:
  - kandev
---

# Chat Motion Requirements

## Overview

Make incoming chat content easier to follow with subtle motion and a single
Appearance control. UI owns this reusable presentation preference; message
content, delivery order, and session lifecycle remain owned by their systems.

## Requirements

### REQ-UI-CHAT-MOTION-001: Incoming content motion

**Intent:** Soften live text and item arrivals without delaying reading.

#### Acceptance criteria

- **AC-UI-CHAT-MOTION-001.1:** With motion effective, newly appended assistant
  prose, including inline text in an existing paragraph and visible thinking
  prose, shall fade into full readability within 200 ms of receipt. Content
  shall become available immediately, without a simulated typing queue.
- **AC-UI-CHAT-MOTION-001.2:** Newly delivered user/assistant messages and tool,
  thinking, plan, and rich-output rows shall enter once within 200 ms, with no
  more than 4 px of vertical movement. Updating an existing row shall not
  replay its entrance or fade previously readable prose.
- **AC-UI-CHAT-MOTION-001.3:** Initial history, pagination, refetch, session
  changes, panel remounts, and revealing collapsed existing content shall not
  replay entrances. Hidden-panel deliveries shall be readable immediately
  when the panel becomes visible.
- **AC-UI-CHAT-MOTION-001.4:** Markdown formatting, file links, inline comments,
  text selection, copying, search, and accessible reading order shall retain
  their behavior. Code and structured output shall remain immediately readable;
  their containing new row can enter once.

### REQ-UI-CHAT-MOTION-002: Appearance preference

**Intent:** Let the reader disable chat motion without disabling chat following.

#### Acceptance criteria

- **AC-UI-CHAT-MOTION-002.1:** Settings > Preferences > Appearance shall expose
  “Chat animations”, enabled by default for new and existing installations
  without a saved choice. Its description shall cover text, new items, and
  smooth scrolling. Saving shall retain the choice on this device after reload.
- **AC-UI-CHAT-MOTION-002.2:** The control shall follow Appearance preview,
  Save, Cancel, and dirty-state behavior. Cancel or leaving without saving
  shall restore the saved value; edits made during a save shall remain dirty.
- **AC-UI-CHAT-MOTION-002.3:** A disabled preference or an active OS
  reduced-motion preference shall suppress these text/entrance effects and
  smooth programmatic chat scrolling. Changes shall take effect during an
  active stream and cancel active effects without hiding content or moving a
  reader-owned position. OS changes shall not overwrite the saved choice.
- **AC-UI-CHAT-MOTION-002.4:** Desktop and phone shall expose the same setting
  and outcomes, with localized labels, keyboard access, visible focus, a
  minimum 44 px phone/coarse-pointer hit area, and no horizontal page overflow.
  Rich-output animations shall remain an independent preference.

### REQ-UI-CHAT-MOTION-003: Smooth transcript following

**Intent:** Follow live content without abrupt jumps or fighting the reader.

#### Acceptance criteria

- **AC-UI-CHAT-MOTION-003.1:** When motion is effective and existing auto-scroll
  policy permits following, content growth shall move continuously toward the
  latest bottom and settle within 300 ms after growth stops, within 2 px of the
  target. Repeated updates shall not queue separate scroll animations.
- **AC-UI-CHAT-MOTION-003.2:** Scrolling up by wheel, touch, keyboard, or scrollbar
  during motion shall stop following. Existing auto-scroll-off, unread-divider,
  history-anchor, navigation, session-placement, and panel-restoration behavior
  shall remain authoritative. Initial placement and restoration shall be instant.
- **AC-UI-CHAT-MOTION-003.3:** Explicit chat navigation and re-enabling auto-scroll
  shall use smooth movement when effective and immediate movement otherwise.
  Disabling motion shall not change the session's auto-scroll preference.
- **AC-UI-CHAT-MOTION-003.4:** Sustained streaming shall not create an ever-growing
  backlog of effects or per-character elements; completed content shall not be
  repeatedly animated. Hidden or unmounted transcripts shall stop motion work.

## Compatibility

This extends [transcript auto-scroll stability](transcript-auto-scroll.md).
For motion-enabled live following, “pinned” means a retained follow intent and
bounded settling under AC-UI-CHAT-MOTION-003.1. Disabled-motion following keeps
immediate pinning. Initial placement and history-anchor rules do not change.
The implementation work order reconciles that wording in the existing contract.

## Out of scope

Global UI motion controls, terminal output animation, chart-internal motion,
exit/reorder animations, typing delays, speed controls, account sync, runtime
release flags, and changes to message transport or Markdown security policy.
