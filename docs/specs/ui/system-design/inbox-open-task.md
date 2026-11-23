---
status: current
system: ui
requirements:
  - REQ-UI-INBOX-OPEN-TASK-001
created: 2026-11-23
owners:
  - kandev
---

# Inbox conversation destination design

## Purpose and boundaries

Inbox chooses the destination. [Quick Chat selection](quick-chat-selection.md)
owns the state of the conversation dialog. [Task navigation](task-navigation.md)
owns the task route.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-INBOX-OPEN-TASK-001` | [Open task destination](#open-task-destination) |

## Open task destination

Use one Inbox action for the desktop and phone controls in all three tabs.
At activation, read the current Quick Chat inventory from the app store.
Boot hydration, live updates, and `useQuickChatResync` own this inventory.
Match the active workspace and task ID. For Needs you and History, also match
the row's session ID. For Failed, use the task's tab from the inventory. Only
use entries for ordinary Quick Chat conversations.

For a match, capture the launcher focus and call `openQuickChat` with the
existing tab's identifiers. Keep the Inbox URL and tab selection in place.
For a menu action, restore focus to the persistent menu trigger after close.
Reuse the floating dialog on desktop and the full-height dialog on phones.
The dialog retains its transcript scroll area, composer, and close control.

Keep the existing `linkToTask` URL on the action. Follow it for a missing
match or when the browser handles a modified click or new-tab action. A tab
absent from the current inventory remains unavailable to this action until
the existing sync adds it. Never create a tab from an Inbox row alone.

## Implementation plan

- [Open Inbox rows in Quick Chat](../../../plans/inbox-quick-chat/plan.md).
