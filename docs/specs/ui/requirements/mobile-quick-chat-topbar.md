---
status: active
system: ui
created: 2026-07-17
updated: 2026-09-09
owners:
  - kandev
---

# Mobile Workspace Topbar Requirements

## Overview

Phone Kanban, List, and Threads share one compact listing header. Current
context and navigation remain visible while secondary workspace actions stay
available in a touch-friendly menu. UI owns this reusable presentation
contract; each listing retains its own filtering and navigation state.

## Requirements

### REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001: Mobile workspace topbar

**Intent:** Make listing modes feel like one application without crowding the
conversation, board, or task list.

#### Acceptance criteria

- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.1:** On phone Kanban, List, and Threads
  with an active workspace, the header menu shall expose Quick Chat and Quick
  Terminal as labeled touch actions.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.2:** Activating Quick Chat shall open it
  for the active workspace with existing chats and its new-chat action
  available. The listing menu shall close before the chat surface opens.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.3:** Activating Quick Terminal shall open
  the active workspace's terminal in the shared Quick Chat surface. The listing
  menu shall close before that surface opens.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.4:** Each phone listing menu shall expose
  Home through the existing workspace-aware home navigation. Changing header
  presentation shall not alter the selected listing preference or Home routing.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.5:** Each phone listing header shall show
  one unboxed, two-line current-context control. Kanban and List shall identify
  their mode and workspace; Threads shall identify its mode and active saved
  view. Long names shall truncate within the control.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.6:** The context control shall remain at
  the left and the navigation menu shall remain at the right, using the same
  header height, spacing, typography, and touch-target geometry across all
  three modes. Neither shall require horizontal scrolling to reach.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.7:** The idle phone listing header shall
  not contain a separate wordmark, redundant page breadcrumb, utility action
  strip, or extra navigation row. Threads pagination follows the
  [Threads deck contract](threads-conversation-deck.md).
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.8:** The header and its menu shall not
  cause document-level horizontal overflow, including with long translated
  labels, enabled metrics, or multiple plugin actions. A long menu shall scroll
  vertically inside the viewport and clear safe-area insets.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.9:** Activating the Kanban or List
  context control shall open the existing listing menu. Activating the Threads
  context control shall retain its saved-view picker. Mode switching,
  workspace selection, applicable filters, and display options shall remain
  available without introducing new saved-view semantics in Kanban or List.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.10:** When task search is supported, the
  menu shall expose its existing search action. Opening search shall dismiss
  the menu and focus a visible search input. Closing search shall clear its
  query and restore unfiltered results. No permanent search row shall appear
  while search is closed.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.11:** Page-specific plugin actions,
  workspace actions, enabled metrics, and system status shall remain reachable
  from the phone listing menu. Their existing availability settings, workspace
  scope, and action behavior shall be preserved.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.12:** The persistent menu control shall
  preserve connection warnings and Quick Chat activity feedback, including
  accessible descriptions. Quick Chat activity follows the existing
  [activity indicator contract](quick-chat-idle-dot.md).
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.13:** Context controls, standalone menu
  buttons, and utility rows shall provide touch targets of at least 44 CSS
  pixels in the active dimension. Dismissing an overlay shall restore focus to
  a mounted trigger. Without an active workspace, unusable workspace launchers
  shall not appear.

## Compatibility and exclusions

The compact phone composition supersedes the former fixed-wordmark and
scrolling-action-strip presentation. Workspace launchers move into the menu;
the underlying capabilities and navigation preferences do not change.

Tablet and desktop headers, task-session chrome, mobile bottom navigation,
floating task creation, session lifecycle, plugin APIs, and persistence are
outside this change. Selecting an explicit default Home view is separate work.

## System design

- [Mobile Workspace Topbar](../system-design/mobile-quick-chat-topbar.md)
