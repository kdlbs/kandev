---
status: active
system: ui
created: 2026-09-13
owners:
  - kandev
---

# Sidebar Hover Reveal Requirements

## Overview

Reveal the collapsed global navigation sidebar after an intentional half-second
hover so users can use its full navigation without permanently expanding it.
UI owns this reusable navigation presentation contract; task and workspace
state retain their existing owners.

## Intent and interpretation

The hover reveal is a temporary overlay that closes after interaction leaves it.
Permanent expansion remains explicit.

## Requirements

### REQ-UI-SIDEBAR-HOVER-001: Delayed temporary sidebar reveal

#### Acceptance criteria

- **AC-UI-SIDEBAR-HOVER-001.1:** While the global sidebar is collapsed and visible
  on a hover-capable fine-pointer device, continuous pointer hover shall begin
  revealing its full navigation after the configured delay (500 ms by default), when hover reveal is enabled.
  Leaving before the delay expires
  shall cancel the reveal; a later entry shall require a fresh full delay.
- **AC-UI-SIDEBAR-HOVER-001.2:** The reveal shall overlay the page at the existing
  expanded sidebar width without moving page content or the status bar, changing
  the saved collapsed preference, or duplicating navigation content.
- **AC-UI-SIDEBAR-HOVER-001.3:** The reveal shall remain usable while the pointer
  or keyboard focus is inside it or an interactive surface opened from it.
  Once those interactions end outside it, it shall collapse. Escape shall dismiss
  the reveal when no child surface consumes Escape, returning focus to a visible
  sidebar control if needed. Dismissal shall require a fresh pointer entry to reopen.
- **AC-UI-SIDEBAR-HOVER-001.4:** Explicit expansion through the existing button or
  shortcut shall keep the sidebar expanded after pointer exit. Explicit collapse
  shall cancel any pending reveal and shall not immediately reopen under a stationary
  pointer. Hover shall not alter an already expanded sidebar.
- **AC-UI-SIDEBAR-HOVER-001.5:** Touch input and phone layouts shall not trigger hover
  reveal. Existing mobile navigation shall remain accessible by tap, with contained
  scrolling and no horizontal page overflow. A change to an ineligible viewport or
  input mode shall cancel pending and visible hover reveals without changing the
  saved desktop preference.

### REQ-UI-SIDEBAR-HOVER-002: User-configurable hover activation

#### Acceptance criteria

- **AC-UI-SIDEBAR-HOVER-002.1:** Preferences > Appearance shall expose a Sidebar
  group with a “Show sidebar on hover” toggle and “Hover delay (ms)” numeric field.
  Hover shall default to enabled and the delay to 500 ms for new and existing
  users without saved values. Explicit disabled and zero-delay values shall survive reload.
- **AC-UI-SIDEBAR-HOVER-002.2:** Users shall save whole-millisecond delays from
  0 through 5000 inclusive. Zero shall reveal without a dwell. Empty, fractional,
  negative or greater-than-5000 input shall show a validation error and shall not
  save. Disabling hover shall retain the last valid delay and disable its editor.
- **AC-UI-SIDEBAR-HOVER-002.3:** Both preferences shall use the existing explicit
  Save changes and discard flow and persist per user across reloads and browsers.
  Save failure shall retain the editable draft and the previously effective values.
  Saving unrelated settings shall not reset either hover preference.
- **AC-UI-SIDEBAR-HOVER-002.4:** Saving or receiving a changed hover preference
  shall cancel pending hover activation and dismiss any temporary reveal safely,
  without changing saved collapse or hiding focus in the collapsed rail. A fresh
  pointer entry shall use the new setting. Disabled hover shall never activate
  a reveal; explicit expansion and shortcuts shall still work.
- **AC-UI-SIDEBAR-HOVER-002.5:** Both settings shall remain editable on phones
  with labelled touch controls and no horizontal overflow. Helper text shall
  explain that hover applies to a mouse or trackpad. Touch navigation shall remain
  tap-based regardless of the saved preference.

## Follow-up design choices

The settings extension was implemented and tested on 2026-09-14. Default-on is explicitly requested; 500 ms
preserves the tested default. The 0–5000 ms whole-number range is an implementation
choice providing immediate reveal through a deliberate five-second dwell.
Preferences follow the existing account-owned appearance settings and save flow.

## Out of scope

Changes to task or review sidebars, additional hover preferences, new endpoints,
new mobile navigation, resizing a temporary reveal, and navigation content redesign.

## Implementation plan

[Sidebar hover reveal](../../../plans/sidebar-hover-reveal/plan.md)
