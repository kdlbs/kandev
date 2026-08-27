---
status: active
system: ui
created: 2026-08-03
updated: 2026-08-26
owners:
  - kandev
---
# Quick Chat and Terminal Tabs Requirements

## Overview

Quick Chat and Quick Terminal are both short-lived utilities reached from the same navigation surfaces, but they currently open separate dialogs with different tab and lifecycle behavior. Users should be able to keep several host terminals beside their utility conversations, switch between them without losing work, and return to the most recent terminal without managing another window.

## Requirements

### REQ-UI-QUICK-TERMINAL-001: Quick Chat and Terminal Tabs

**Intent:** Quick Chat and Quick Terminal are both short-lived utilities reached from the same navigation surfaces, but they currently open separate dialogs with different tab and lifecycle behavior. Users should be able to keep several host terminals beside their utility conversations, switch between them without losing work, and return to the most recent terminal without managing another window.

#### Acceptance criteria

- **AC-UI-QUICK-TERMINAL-001.1:** Quick Chat is the single responsive dialog for ordinary chats, configuration chats, and host terminal tabs. Quick Terminal no longer opens a separate dialog.
- **AC-UI-QUICK-TERMINAL-001.2:** Existing conversation launchers preserve their kind-specific behavior. A generic Quick Chat launcher selects an ordinary chat. A configuration entry point selects the workspace's configuration chat. Each launcher opens its setup when no matching conversation exists. On Settings routes, the Configuration Chat floating launcher remains visible and operable while its panel is open. Another activation closes the panel.
- **AC-UI-QUICK-TERMINAL-001.3:** The existing Quick Terminal launchers use a reuse-or-create policy scoped to the active workspace: they open the most recently activated terminal tab when one exists, and create the first terminal tab otherwise.
- **AC-UI-QUICK-TERMINAL-001.4:** The tab-strip plus button opens a creation menu grouped like the task-detail Dockview add menu: an **Agents** section with **New Agent**, a separator, and a **Terminals** section with **New Terminal**. Existing tabs remain directly selectable in the tab strip rather than being duplicated in the creation menu. Because the plus button sits at the leading edge of the tab strip, its menu opens toward the trailing edge (aligned to the button's start) so it does not overhang the workspace edge.
- **AC-UI-QUICK-TERMINAL-001.5:** Choosing **New Agent** preserves the current ordinary/configuration setup flow. Choosing **New Terminal** always creates and activates a distinct host-shell terminal, even when another terminal exists.
- **AC-UI-QUICK-TERMINAL-001.6:** Chat and terminal tabs share one horizontal tab strip. Conversation ordering and configuration indicators remain unchanged; terminal tabs are ordered by creation and use a terminal icon with workspace-local labels such as `Terminal 1`, `Terminal 2`.
- **AC-UI-QUICK-TERMINAL-001.7:** Renameable conversation tabs expose **Rename** from a context menu on desktop right-click and the equivalent touch long-press gesture. The existing inline editor and backing-task rename persistence remain unchanged; terminal labels stay fixed.
- **AC-UI-QUICK-TERMINAL-001.8:** Multiple terminal tabs can run concurrently. Input, output, resize, exit, and error state belong to the selected terminal and must not affect sibling terminals.

## System design

The migrated technical source is split into [part 1](../system-design/quick-terminal.md).
