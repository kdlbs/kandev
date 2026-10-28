---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Quick Chat Active Tab Visibility Requirements

## Overview

A selected Quick Chat tab should be identifiable when the shared tab row overflows. UI owns this presentation behavior; the [Quick Chat and Terminal contract](quick-terminal.md) continues to own tab selection, ordering, focus and lifecycle.

## Requirements

### REQ-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001: Reveal the active tab

**Intent:** Make a newly selected conversation, setup or terminal discoverable without interrupting the user's content or manual browsing.

#### Acceptance criteria

- **AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.1:** When the active identity changes or an already selected tab first appears with the dialog, the strip shall reveal the selected persisted conversation, temporary setup or terminal using only the smallest horizontal adjustment needed. A fully visible tab shall cause no adjustment.
- **AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.2:** When a selected tab is wider than the strip viewport, the visible part shall include as much of its label-bearing activation control as the viewport permits, aligned to its leading edge when that control itself is too wide. If the complete tab cannot fit, its remaining action shall remain reachable by manual horizontal scrolling; full tab visibility is not required.
- **AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.3:** Unrelated renders, renames, activity updates and reorder operations with an unchanged active identity shall not undo manual horizontal browsing.
- **AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.4:** Reveal shall preserve focus and document and dialog vertical scroll positions. The row shall contain horizontal overflow on desktop and phone, without introducing document horizontal overflow.
- **AC-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001.5:** Existing selection, close, rename, creation menu, keyboard and touch actions, drag and reorder behavior shall remain usable.

## Out of scope

- Selection persistence, session and terminal lifecycle, backend APIs and storage.
- New controls, labels, global styling, resize monitoring, polling or a changed focus policy.

## Implementation Plans

- [Quick Chat active tab visibility](../../../plans/quick-chat-active-tab-visibility/plan.md).
