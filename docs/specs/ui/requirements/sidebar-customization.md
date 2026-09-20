---
status: active
system: ui
created: 2026-09-19
owners:
  - kandev
---

# Sidebar Customization

## Overview

Users can hide navigation entries and organize mixed shortcuts into named,
collapsible sections. UI owns these reusable personal presentation preferences.
Domain systems retain resource identity, access, execution, and activity state.

The agreed section has a name and direct-action icons on one header line.
Expanding it shows the same shortcuts as labelled entries below the header.
A shortcut is a reference to an existing destination or supported host action.

## Requirements

### REQ-UI-SIDEBAR-CUSTOMIZATION-001: Personal workspace layout

- **AC-UI-SIDEBAR-CUSTOMIZATION-001.1:** Users shall hide, show, and reorder Home, New Task, Automations, Canvases, Integrations, and available plugin navigation entries. Hiding an entry shall not disable its underlying capability.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.2:** Each user's workspace shall retain its own saved layout across reloads and signed-in clients. Changes shall not affect other users or workspaces.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.3:** A workspace without a saved layout shall retain current navigation defaults. Restore defaults shall reset only that workspace's layout after Save changes.
- **AC-UI-SIDEBAR-CUSTOMIZATION-001.4:** Settings, workspace switching, Tasks, and required inbox navigation shall remain reachable. Hiding Home shall not change the startup destination or brand-link destination.
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

## Scope boundaries

The first version supports existing registered plugin navigation links, including
Slack when its installed plugin registers a link. Arbitrary plugin-rendered
buttons, new Slack behavior, Run automation actions, arbitrary URLs/scripts,
shared team layouts, and task-list filtering changes are excluded.
Office-only sections keep their existing composition in this version. Common
Home, New Task, plugin links, and shortcut groups remain customizable in Office.
Required inbox entries and the Tasks region remain fixed. Shortcut groups belong
above that region. The editor identifies these fixed entries.

## Implementation plans

- [Sidebar customization](../../../plans/sidebar-customization/plan.md)
