---
status: active
system: ui
created: 2026-11-23
owners:
  - kandev
---

# Inbox conversation destinations

## Overview

UI owns the shared **Open task** interaction for [Needs you](needs-you-inbox.md),
[Failed](inbox-failed.md), and [History](inbox-history.md).

## Requirements

### REQ-UI-INBOX-OPEN-TASK-001: Open the conversation from Inbox

Open the row's conversation in the surface where the user can continue it.

#### Acceptance criteria

- **AC-UI-INBOX-OPEN-TASK-001.1:** On desktop and phone, activating **Open
  task** for an available Quick Chat shall open its conversation in Quick Chat.
  This rule applies to Needs you, Failed, and History. When the row names a
  session, Quick Chat shall open that session if it is available for the task.
  Closing Quick Chat shall return the user to the same Inbox tab.
- **AC-UI-INBOX-OPEN-TASK-001.2:** When the row has no matching Quick Chat
  conversation, **Open task** shall follow the existing task link. If the row
  names a session, the task link shall preserve that session.

## Out of scope

- Office Inbox, task and session lifecycle, and changes to row membership.
