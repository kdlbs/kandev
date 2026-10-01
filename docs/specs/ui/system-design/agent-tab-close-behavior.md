---
status: current
system: ui
requirements:
  - REQ-UI-AGENT-TAB-CLOSE-BEHAVIOR-001
---

# Agent tab close behavior design

`UserSettings.AgentTabCloseBehavior` is normalized and persisted by the backend JSON settings payload. DTO, boot state, HTTP, and WebSocket settings paths project the effective value. The frontend uses the common settings mapper.

The hide path stores environment-scoped session IDs in session storage with task ownership. Dockview synchronization excludes hidden panels; reopening a session clears its record before adding the panel. This device-local layout state follows ADR 0041 while the choice of close behavior remains portable.

The control belongs to Task Behavior’s Conversation tab, including settings discovery and the tab-scoped save contributor. When implicit session selection targets a hidden session after navigation or reload, restoration and synchronization select a visible sibling without clearing the hidden record. Explicit reopen is the only selection action that clears it.
