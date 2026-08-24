---
status: active
system: ui
created: 2026-07-17
updated: 2026-08-18
owners:
  - kandev
---
# Mobile Workspace Topbar Actions Requirements

## Overview

Mobile users need direct access to workspace actions from the Home and Tasks headers. The header must remain usable when resource metrics and plugin actions add more controls than a phone can fit.

## Requirements

### REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001: Mobile Workspace Topbar Actions

**Intent:** Mobile users need direct access to workspace actions from the Home and Tasks headers. The header must remain usable when resource metrics and plugin actions add more controls than a phone can fit.

#### Acceptance criteria

- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.1:** On mobile Home and Tasks headers with an active workspace, `Quick Terminal` appears before `Quick Chat`. `Quick Chat` appears immediately before the task-search button.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.2:** Activating `Quick Chat` opens Quick Chat for the active workspace. Existing workspace chats are available in the dialog, and its existing new-chat action remains available.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.3:** Activating `Quick Terminal` opens the active workspace's terminal in the shared Quick Chat surface.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.4:** The mobile Kandev wordmark is a link to the active workspace's Home board.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.5:** The Home header does not render a separate `Home` label. Non-Home headers retain their existing page-title visibility while keeping the Kandev home link available.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.6:** The Kandev link is the only fixed content at the left edge. The menu button is the only fixed content at the right edge.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.7:** A non-Home title, its workspace label, plugin actions, resource metrics, `Quick Terminal`, `Quick Chat`, and search use one horizontal strip between the fixed controls.
- **AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.8:** When the strip content fits, its actions are aligned against the fixed menu on the right. The middle strip scrolls horizontally only when its content does not fit. It does not add horizontal scrolling to the page.

## Migrated source detail

## Why

Mobile users need direct access to workspace actions from the Home and Tasks headers. The header
must remain usable when resource metrics and plugin actions add more controls than a phone can fit.

## What

- On mobile Home and Tasks headers with an active workspace, `Quick Terminal` appears before
  `Quick Chat`. `Quick Chat` appears immediately before the task-search button.
- Activating `Quick Chat` opens Quick Chat for the active workspace. Existing workspace chats
  are available in the dialog, and its existing new-chat action remains available.
- Activating `Quick Terminal` opens the active workspace's terminal in the shared Quick Chat
  surface.
- The mobile Kandev wordmark is a link to the active workspace's Home board.
- The Home header does not render a separate `Home` label. Non-Home headers retain their
  existing page-title visibility while keeping the Kandev home link available.
- The Kandev link is the only fixed content at the left edge. The menu button is the only fixed
  content at the right edge.
- A non-Home title, its workspace label, plugin actions, resource metrics, `Quick Terminal`,
  `Quick Chat`, and search use one horizontal strip between the fixed controls.
- When the strip content fits, its actions are aligned against the fixed menu on the right. The
  middle strip scrolls horizontally only when its content does not fit. It does not add horizontal
  scrolling to the page.
- A fade appears at each strip edge only when more actions exist beyond that edge. The fade
  disappears when the strip reaches that edge.
- Native icon buttons in this header use the same 32 by 32 CSS-pixel visible box and 16 CSS-pixel
  icon. Resource metric and host-rendered plugin icons use the same 16 CSS-pixel icon size.
- The resource metrics region uses the same 32 CSS-pixel height as the icon buttons.
- Quick Terminal and Quick Chat retain a 44 CSS-pixel coarse-pointer hit area around their 32
  CSS-pixel visible boxes without widening the action-strip items.
- The tablet header and mobile task switcher retain their existing Quick Chat entry points.
- Desktop headers do not change.

## Scenarios

- **GIVEN** a mobile Home header with an active workspace, **WHEN** the header renders, **THEN**
  the terminal, chat, search, and menu controls use the same visible size and no separate `Home`
  label is shown.
- **GIVEN** a mobile Home header, **WHEN** the user activates `Quick Chat`, **THEN** the active
  workspace's Quick Chat dialog opens and the user can access existing chats or start a new one.
- **GIVEN** a mobile Home header, **WHEN** the user activates `Quick Terminal`, **THEN** the active
  workspace's terminal opens in the shared Quick Chat surface.
- **GIVEN** resource metrics and plugin actions that exceed the middle strip width, **WHEN** the
  mobile header renders, **THEN** the Kandev link and menu remain fixed while the strip scrolls.
- **GIVEN** a scrollable middle strip, **WHEN** hidden actions exist to the left or right, **THEN**
  only the corresponding directional fade is visible.
- **GIVEN** a middle strip whose actions fit, **WHEN** the mobile header renders, **THEN** its
  actions align against the menu, no overflow fade is visible, and the document has no horizontal
  overflow.
- **GIVEN** a mobile non-Home workbench page, **WHEN** the user activates the Kandev wordmark,
  **THEN** the app navigates to that workspace's Home board.
- **GIVEN** no active workspace, **WHEN** the mobile header renders, **THEN** it does not show an
  unusable Quick Chat or Quick Terminal button.

## Out of scope

- Changes to Quick Chat creation, persistence, session selection, or modal layout.
- Changes to Quick Terminal lifecycle or terminal layout.
- Changes to desktop topbars.
- Changes to tablet header sizing.
- Changes to task-session topbars, mobile bottom navigation, or floating task creation.

## Implementation plan

- [Original Quick Chat topbar implementation](../../../plans/mobile-quick-chat-topbar/plan.md)
- [Scrollable mobile topbar repair](../../../plans/mobile-topbar-action-strip/plan.md)
