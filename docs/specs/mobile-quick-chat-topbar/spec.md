---
status: building
created: 2026-07-17
owner: kandev
---

# Mobile Quick Chat Topbar

## Why

Mobile users cannot discover Quick Chat from the board header and must rely on a keyboard
shortcut or menu path. The same header also repeats `Home` beside the Kandev wordmark even
though the wordmark can provide the home navigation affordance.

## What

- On mobile Home and Tasks headers with an active workspace, an icon button labeled
  `Quick Chat` appears immediately before the task-search button.
- Activating the button opens Quick Chat for the active workspace. Existing workspace chats
  are available in the dialog, and its existing new-chat action remains available.
- The mobile Kandev wordmark is a link to the active workspace's Home board.
- The Home header does not render a separate `Home` label. Non-Home headers retain their
  existing page-title visibility while keeping the Kandev home link available.
- The tablet header and mobile task switcher retain the Quick Chat entry points supplied by
  the underlying mobile-access change.
- Desktop headers do not change.

## Scenarios

- **GIVEN** a mobile Home header with an active workspace, **WHEN** the header renders, **THEN**
  the `Quick Chat` button appears immediately to the left of `Search tasks` and no separate
  `Home` label is shown.
- **GIVEN** a mobile Home header, **WHEN** the user activates `Quick Chat`, **THEN** the active
  workspace's Quick Chat dialog opens and the user can access existing chats or start a new one.
- **GIVEN** a mobile non-Home workbench page, **WHEN** the user activates the Kandev wordmark,
  **THEN** the app navigates to that workspace's Home board.
- **GIVEN** no active workspace, **WHEN** the mobile header renders, **THEN** it does not show an
  unusable Quick Chat button.

## Out of scope

- Changes to Quick Chat creation, persistence, session selection, or modal layout.
- Changes to desktop topbars.
- Changes to search, menu, metrics, or floating task-creation behavior.

## Implementation plan

[Mobile Quick Chat Topbar implementation](../../plans/mobile-quick-chat-topbar/plan.md)
