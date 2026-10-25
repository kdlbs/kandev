---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Sidebar Title Overflow Requirements

## Overview

Long task titles currently end at a hard clipping edge. A short fade makes that boundary easier to read.
UI owns this presentation contract. Task title data and task lifecycle remain with their existing owners.

## Requirements

### REQ-UI-SIDEBAR-TITLE-OVERFLOW-001: Fade clipped task titles

**Intent:** Show that a task title continues beyond its available width without reducing the clarity of adjacent information.

#### Acceptance criteria

- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.1:** When a sidebar task title exceeds its available width, its final visible portion shall fade smoothly to transparent. The fade shall be no wider than 16 CSS pixels or one-third of the available title width, whichever is smaller.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.2:** When a title fits, all its characters shall remain fully visible. Resizing or changing the title shall update the fade.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.3:** The fade shall affect only title text. Status icons, badges, timestamps, menus, and row backgrounds shall retain their appearance and geometry.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.4:** Hover from a fine pointer, including on a hybrid device, shall preserve the existing scrolling disclosure and show the title ending without a fade. Leaving the title shall restore its idle appearance.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.5:** Desktop and phone task navigation shall retain the complete title text for assistive technology and task activation. Phone rows shall retain visible touch actions and cause no horizontal document overflow.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.6:** The fade shall work on default, selected, and hovered rows in light and dark themes without a visible background-colored strip.
- **AC-UI-SIDEBAR-TITLE-OVERFLOW-001.7:** In forced-colors mode, clipped task titles shall not use a mask.

## Out of scope

- New preferences, title wrapping, or changes to task data.
- New title disclosure interactions or changes to other clipped labels.
- Changes to row spacing, badges, trailing columns, or shared hover-scroll behavior.

## Implementation plan

[Sidebar title overflow plan](../../../plans/sidebar-title-overflow/plan.md).
