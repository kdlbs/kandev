---
status: active
system: ui
created: 2026-09-19
owners:
  - kandev
---

# Sidebar Customization

## Navigation hierarchy revision

[Navigation hierarchy](navigation-hierarchy.md) updates the default New Task/Home
order and puts tools before Tasks on phones. Saved desktop order, visibility,
custom shortcut groups, and settings persistence remain unchanged.

## Overview

Users can hide navigation entries and organize mixed shortcuts into named,
collapsible sections. UI owns these reusable personal presentation preferences.
Domain systems retain resource identity, access, execution, and activity state.

The agreed section has a name and direct-action icons on one header line.
Expanding it shows the same shortcuts as labelled entries below the header.
A shortcut is a reference to an existing destination or supported host action.

## Requirements

### REQ-UI-SIDEBAR-CUSTOMIZATION-001: Personal workspace layout

- **AC-UI-SIDEBAR-CUSTOMIZATION-001.1:** Users shall hide, show, and reorder Home, New Task, eligible Inbox entries, Automations, Canvases, Integrations, and available plugin navigation entries. Hiding an entry shall not disable its underlying capability.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.2:** Each user's workspace shall retain its own saved layout across reloads and signed-in clients. Changes shall not affect other users or workspaces.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.3:** A workspace without a saved layout shall place primary New Task before Home, followed by eligible tools. Restore defaults shall reset only that workspace's layout after Save changes.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.4:** Settings, workspace switching, and Tasks shall remain reachable. Hidden Inbox entries shall remain available through their existing routes and commands. Hiding Home shall not change the startup destination or brand-link destination.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.5:** Layout preferences shall affect the sidebar and corresponding phone navigation, without removing commands from search or changing settings navigation.

### REQ-UI-SIDEBAR-CUSTOMIZATION-002: Mixed shortcut sections

- **AC-UI-SIDEBAR-CUSTOMIZATION-002.1:** Users shall create, rename, hide, remove, and reorder shortcut sections. Each section shall accept built-in destinations, supported host actions, plugin navigation links, workspace canvases, and workspace automations.
- **AC-UI-SIDEBAR-CUSTOMIZATION-002.2:** The header shall show the section name and ordered shortcut icons. Activating its name or chevron shall toggle the labelled list below. Activating an icon shall invoke only its shortcut.
- **AC-UI-SIDEBAR-CUSTOMIZATION-002.3:** The expanded list shall contain the same shortcuts in the same order, with full accessible names. Header icons shall remain present when expanded.
- **AC-UI-SIDEBAR-CUSTOMIZATION-002.4:** Users shall reorder shortcuts within a section and move them between sections through drag and drop or explicit keyboard/touch controls.
- **AC-UI-SIDEBAR-CUSTOMIZATION-002.5:** Header overflow shall expose remaining shortcuts through a labelled More control. The collapsed sidebar rail shall provide a section launcher with access to every shortcut.
- **AC-UI-SIDEBAR-CUSTOMIZATION-002.6:** A pinned automation shall open its history, and a pinned canvas shall open that canvas. Pinning or opening shall not execute an automation, send a message, or create a task implicitly.

### REQ-UI-SIDEBAR-CUSTOMIZATION-003: Automation activity bubbles

- **AC-UI-SIDEBAR-CUSTOMIZATION-003.1:** An automation shortcut shall show a running, idle, or paused bubble using the automation list's current state semantics. The header bubble shall remain visible when the section is folded.
- **AC-UI-SIDEBAR-CUSTOMIZATION-003.2:** While its navigation surface is visible, a pinned automation shall refresh activity at least once every ten seconds. Starting and finishing runs shall update without expanding the section or reloading.
- **AC-UI-SIDEBAR-CUSTOMIZATION-003.3:** Tooltip and accessible text shall identify the automation and its activity. The expanded row shall show the same state. Loading or failed reads shall never claim idle activity.
- **AC-UI-SIDEBAR-CUSTOMIZATION-003.4:** Overflow and rail launchers shall indicate hidden running automation activity. Opening them shall identify the specific running shortcuts.

### REQ-UI-SIDEBAR-CUSTOMIZATION-004: Settings editor and recovery

- **AC-UI-SIDEBAR-CUSTOMIZATION-004.1:** Appearance settings and a Customize sidebar entry shall open an editor with workspace context, visibility controls, section editing, and a searchable shortcut picker.
- **AC-UI-SIDEBAR-CUSTOMIZATION-004.2:** The editor shall preview draft changes. Shared Save changes, discard, and navigation protection shall govern persistence. Failed saves shall retain the draft and explain the failure.
- **AC-UI-SIDEBAR-CUSTOMIZATION-004.3:** A workspace switch or delayed response shall not apply a draft or result to another workspace. Concurrent saves to different workspaces shall both survive. A conflicting save to the same layout shall retain the draft and require reconciliation.
- **AC-UI-SIDEBAR-CUSTOMIZATION-004.4:** Missing, disabled, inaccessible, or uninstalled targets shall appear unavailable without activation. Their saved positions shall remain until removal, and eligible returning targets shall resolve again.
- **AC-UI-SIDEBAR-CUSTOMIZATION-004.5:** Empty sections shall remain editable and offer Add shortcut in the editor. Invalid names, duplicate shortcuts within one section, and exceeded limits shall produce visible validation messages before saving.

### REQ-UI-SIDEBAR-CUSTOMIZATION-005: Phone and accessible navigation

- **AC-UI-SIDEBAR-CUSTOMIZATION-005.1:** Phone navigation shall expose the same saved shortcut groups and activity. Editing shall use a full-page layout with one focused section editor and a searchable picker.
- **AC-UI-SIDEBAR-CUSTOMIZATION-005.2:** All editing and navigation operations shall work without dragging, hovering, or long pressing. Touch targets shall measure at least 44 pixels.
- **AC-UI-SIDEBAR-CUSTOMIZATION-005.3:** Phone surfaces shall fit the viewport, contain long-content scrolling, and clear safe areas. Desktop and phone shall share layout data without persisting responsive overflow choices.
- **AC-UI-SIDEBAR-CUSTOMIZATION-005.4:** Controls shall have accessible names, keyboard operation, visible focus, and predictable focus return. New interface copy shall support every shipped locale.
- **AC-UI-SIDEBAR-CUSTOMIZATION-005.5:** Saving or resetting a layout shall preserve the phone menu hierarchy: visible New Task, Home and quick actions, customizable workspace tools and shortcut groups, then required task/local navigation. Workspace tools retain their saved relative order and visibility. Built-in Automations, Canvases, and Integrations shall use labelled disclosures with visible expansion cues and labelled destinations; integration and automation setup remain reachable when empty. The Tasks heading retains its create-task action only when the built-in New Task row is hidden in regular workspaces. Office keeps its existing creation entry. Phone composition shall not rewrite the saved desktop order.

### REQ-UI-SIDEBAR-CUSTOMIZATION-006: Direct sidebar customization

- **AC-UI-SIDEBAR-CUSTOMIZATION-006.1:** Right-clicking a desktop navigation
  entry or the navigation region's empty space shall open the same customization
  menu without activating its destination or disclosure. It shall list eligible
  built-in entries, Inbox entries, plugin destinations, and custom groups as
  checked visibility choices. Hidden entries shall remain listed unchecked for
  restoration, including when all optional entries are hidden.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.2:** The menu shall also expose Show fast
  action icons and the New Task style choices from REQ-UI-NAV-HIERARCHY-004.
  Open sidebar layout settings shall be its last action and shall open the
  Sidebar tab of Layout settings. Existing settings navigation guards shall apply.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.3:** A visibility choice or completed
  reorder shall save immediately to the same user's workspace layout edited by
  Settings. Menu presentation choices shall save to the same account preferences
  used by Settings. Reloads and signed-in clients shall restore these values.
  No separate sidebar-only or browser-only layout shall be created.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.4:** Users shall drag desktop navigation
  rows directly to change their top-level order. There shall be no drag handle,
  grip icon, hover handle, or edit-mode handle. The cursor shall change to
  `grabbing` only during an active drag, and an insertion indicator shall show
  the drop position. Dragging shall not navigate, create a task, launch a quick
  action, or toggle a disclosure. Independent action buttons shall retain clicks.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.5:** Cancelled, unchanged, or invalid drops
  shall not save. A failed save shall restore the authoritative layout and show
  a localized error. Conflicts shall preserve other clients' changes and offer
  retry or reconciliation. Workspace changes shall cancel active gestures and
  prevent delayed responses from replacing another workspace's layout.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.6:** Both direct editing and Settings shall
  reflect saved changes. A clean Settings draft shall refresh to the latest
  saved values. An unsaved Settings draft shall survive an external sidebar
  change and require revision reconciliation instead of silently overwriting it.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.7:** Keyboard users shall open the menu
  and reorder without dragging. Phone users shall have a visible customization
  action opening an inset drawer with the same choices and explicit Move up/down
  controls. Touch targets shall be at least 44px; one scroller and safe-area
  clearance shall contain the drawer. No function shall require long pressing.
- **AC-UI-SIDEBAR-CUSTOMIZATION-006.8:** Inbox entries shall follow the same
  saved visibility and order in Settings and navigation. Existing layouts shall
  retain their other nodes' order and receive missing Inbox entries at their
  current default position. Feature or workspace eligibility shall take precedence
  over visibility, without removing a saved Inbox node or changing its position.

### REQ-UI-SIDEBAR-CUSTOMIZATION-007: Persisted navigation and Tasks split

- **AC-UI-SIDEBAR-CUSTOMIZATION-007.1:** In expanded regular desktop navigation,
  users shall drag the navigation/Tasks divider vertically without reordering
  entries. The allocated navigation height and expanded state shall persist in
  the same user/workspace database layout across requests and reloads.
- **AC-UI-SIDEBAR-CUSTOMIZATION-007.2:** Clipped navigation shall retain normal
  button dimensions, show a bottom fading gradient, and expose a small bottom
  chevron. Its visible strip shall use no more than 12px of vertical space on
  fine-pointer desktop, with an overlapping accessible hit area. Hidden controls
  shall not remain keyboard-focusable outside the visible region.
- **AC-UI-SIDEBAR-CUSTOMIZATION-007.3:** The chevron shall expand navigation to
  reveal its entries, preserving the compressed height. Clicking again shall
  restore that height. Both choices shall survive reload. When all entries fit
  without expansion, no extra chevron or fade shall appear.
- **AC-UI-SIDEBAR-CUSTOMIZATION-007.4:** Resizing shall reserve usable Tasks space
  and retain its existing scroll owner. Expanded navigation exceeding available
  height shall scroll internally. Viewport changes shall clamp effective geometry
  without overwriting the saved height. Cancelled or unchanged drags shall not save.
- **AC-UI-SIDEBAR-CUSTOMIZATION-007.5:** Divider keyboard controls and coarse
  pointer targets shall remain accessible. Phone navigation and Office mode
  shall keep their existing scroll composition without writing desktop split
  preferences. Failure/conflict feedback and workspace isolation shall match 006.5.

## Scope boundaries

The first version supports existing registered plugin navigation links, including
Slack when its installed plugin registers a link. Arbitrary plugin-rendered
buttons, new Slack behavior, Run automation actions, arbitrary URLs/scripts,
shared team layouts, and task-list filtering changes are excluded.
Office-only sections keep their existing composition in this version. Common
Home, New Task, plugin links, and shortcut groups remain customizable in Office.
Inbox entries are customizable; the Tasks region remains fixed. Desktop shortcut groups
belong above that region; phone groups precede task navigation. The editor identifies
these fixed entries. Phone composition follows the
[unified mobile navigation contract](unified-mobile-navigation.md).

## Implementation plans

- [Sidebar customization](../../../plans/sidebar-customization/plan.md)
- [Mobile saved navigation](../../../plans/mobile-saved-navigation/plan.md)
- [Sidebar preferences and direct editing](../../../plans/sidebar-presentation-preferences/plan.md)
