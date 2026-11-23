---
created: 2026-11-23
status: implemented
requirements:
  - REQ-UI-INBOX-OPEN-TASK-001
system_design:
  - ../../specs/ui/system-design/inbox-open-task.md
legacy_specs: []
---

# Open Inbox rows in Quick Chat

The Inbox owns the choice of where its **Open task** action goes. Quick Chat owns
its conversation inventory, selection, and dialog.

Needs you and History pass a task ID and a session ID to `linkToTask`. Failed
passes a task ID. In each case `AppLink` opens the task route because the row
does not check the Quick Chat inventory. To reproduce, start a Quick Chat,
send `/e2e:clarification-multi`, open Inbox, and activate **Open task** on
its question. Source inspection confirms the task-route path. Browser
reproduction belongs to the work order.

## Work order

| Order | Work order | Status | Dependencies |
| --- | --- | --- | --- |
| 1 | [Use the Quick Chat destination](task-01-open-quick-chat.md) | done | None |

The work order covers all three Inbox tabs. Existing task links and Office Inbox
stay outside the routing change. A missing tab in the Quick Chat inventory uses
the existing task link. A stale or empty inventory during sync can therefore
affect the destination until the tab is available.

## ASCII UI preview

UI-01 shows the result of **Open task** on a Quick Chat row. The existing Inbox
action opens the task page before the fix.

```text
Desktop                           Phone
+----------------------------+    +--------------------+
| Inbox (same tab)           |    | Quick Chat [Close] |
|   +--------------------+   |    | Conversation tabs  |
|   | Quick Chat [Close] |   |    |                    |
|   | Conversation tabs  |   |    | Transcript         |
|   | Transcript         |   |    |                    |
|   | Composer           |   |    | Composer           |
|   +--------------------+   |    +--------------------+
+----------------------------+    Close returns to Inbox
```

The desktop dialog floats above the Inbox. The phone uses the full-height
dialog from the Home Quick Chat entry. The transcript scrolls within each
dialog. The dialog retains its existing viewport and safe-area handling.
The phone keeps its existing touch action and visible close control.
The two layouts share the same destination logic. The browser tests cover
AC-UI-INBOX-OPEN-TASK-001.1 and .2.
