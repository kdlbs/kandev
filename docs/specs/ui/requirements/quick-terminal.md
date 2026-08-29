---
status: active
system: ui
created: 2026-08-03
updated: 2026-08-28
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
- **AC-UI-QUICK-TERMINAL-001.6:** Chat and terminal tabs share one horizontal tab strip. Configuration indicators and workspace-local terminal labels such as `Terminal 1` and `Terminal 2` remain unchanged.
- **AC-UI-QUICK-TERMINAL-001.7:** Renameable conversation tabs expose **Rename** from a context menu on fine-pointer devices and from a visible tab action on coarse-pointer devices. The backing-task rename persistence remains unchanged. Terminal labels stay fixed.
- **AC-UI-QUICK-TERMINAL-001.8:** Multiple terminal tabs can run concurrently. Input, output, resize, exit, and error state belong to the selected terminal and must not affect sibling terminals.
- **AC-UI-QUICK-TERMINAL-001.9:** When Escape closes the shared dialog, the system shall return focus to the launcher without a visible focus indicator. When focus later returns through ordinary keyboard navigation, the launcher shall show its normal focus indicator.
- **AC-UI-QUICK-TERMINAL-001.10:** The user-configurable Quick Chat keyboard shortcut shall toggle the shared dialog. Activating the configured shortcut while the dialog is closed opens ordinary Quick Chat; activating it while the dialog is open closes the dialog without removing its chat or terminal sessions.
- **AC-UI-QUICK-TERMINAL-001.11:** When a connected agent passthrough terminal owns keyboard focus inside the shared dialog, with its xterm AttachAddon installed on an open terminal WebSocket, unmodified Escape shall reach the agent TUI and the dialog shall remain open. Escape from non-terminal focus targets, or while that terminal connection is unavailable, retains the existing dialog and nested-widget behavior.
- **AC-UI-QUICK-TERMINAL-001.12:** Phone and coarse-pointer users shall retain the existing visible Quick Chat close action and touch dismissal paths without depending on a hardware keyboard shortcut.

## Out of scope

- Changing pointer dismissal, tooltip behavior, or focus indicators on unrelated controls.
- Changing the Quick Chat layout or its mobile touch controls.
- Adding a separate preference for whether Escape closes Quick Chat. Focus ownership determines the behavior.

### REQ-UI-QUICK-TERMINAL-002: Quick Chat Tab Organization

**Intent:** Users can keep a deliberate Quick Chat tab layout and can recognize when a tab title is ready for editing.

**User story:** As a Kandev user, I want to arrange and rename Quick Chat tabs, so that the dialog remains predictable after I return to it.

#### Acceptance criteria

- **AC-UI-QUICK-TERMINAL-002.1:** A workspace keeps one stable order for persisted conversation and terminal tabs. The order survives dialog close, page reload, browser restart, and a later load by the same user on another client.
- **AC-UI-QUICK-TERMINAL-002.2:** Fine-pointer users can drag persisted tabs into any position. Touch users can press and hold a tab to drag it without blocking ordinary taps or horizontal tab-strip scrolling.
- **AC-UI-QUICK-TERMINAL-002.3:** Keyboard users and coarse-pointer users can move a tab without a precision drag. Coarse-pointer tab actions have a target size of at least 44 by 44 CSS pixels.
- **AC-UI-QUICK-TERMINAL-002.4:** The system ignores unknown, duplicate, and stale saved tab references. It appends new persisted tabs in a stable baseline order. Temporary setup tabs remain at the trailing edge and do not enter the saved order.
- **AC-UI-QUICK-TERMINAL-002.5:** A failed order save leaves every tab available and keeps the active tab selected. The dialog shows a user-visible save error. A later load can restore the last order that the backend accepted.
- **AC-UI-QUICK-TERMINAL-002.6:** A working-state grid spinner has at least 6 CSS pixels of horizontal space before its tab title.
- **AC-UI-QUICK-TERMINAL-002.7:** Rename mode visibly distinguishes the tab with an edit-state background and border, shows a focused bordered input with selected text, and omits inline **Save** and **Cancel** actions. Enter commits a trimmed name, Escape restores the previous name, and blur commits without a duplicate rename.
- **AC-UI-QUICK-TERMINAL-002.8:** On phone and tablet viewports, the tab strip contains its own horizontal overflow. The selected content remains the dialog scroll owner, and the feature does not cause document horizontal overflow.

## System design

The migrated technical source is split into [part 1](../system-design/quick-terminal.md).
