---
status: active
system: ui
created: 2026-09-09
owners:
  - Kandev frontend
---

# Control sizing requirements

## Overview

Equivalent controls must have consistent dimensions across Kandev.
The desktop Start Task dialog provides the reference density.
The UI system owns this reusable presentation contract across task, settings,
integration, and Office surfaces.

## Terminology

- **Standard control:** An ordinary action button or single-line field, including
  a select trigger or combobox trigger.
- **Compact control:** An explicit inline action within dense application chrome.
- **Touch context:** A phone viewport below 768px or a device whose primary pointer is coarse.
- **Desktop context:** A viewport of at least 768px with a fine primary pointer.
- **Content surface:** A selection card, navigation row, menu option, or multiline field.

## Requirements

### REQ-UI-CONTROL-SIZING-001: Consistent control dimensions

**Intent:** Equivalent actions and fields retain consistent desktop density
without reducing touch usability.

#### Acceptance criteria

- **AC-UI-CONTROL-SIZING-001.1:** In desktop context, standard controls shall
  have a height of 28px at the standard root font size.
- **AC-UI-CONTROL-SIZING-001.2:** In desktop context, explicit compact controls
  shall have a height of 24px at the standard root font size.
- **AC-UI-CONTROL-SIZING-001.3:** Adjacent controls with the same size role
  shall have equal heights, including attached picker, clear, and reveal actions.
- **AC-UI-CONTROL-SIZING-001.4:** In touch context, standard controls shall
  provide an active target at least 44px high. Standalone icon actions shall
  also provide an active target at least 44px wide.
- **AC-UI-CONTROL-SIZING-001.5:** When a viewport or pointer changes context,
  shared controls shall use that context's dimensions without losing their values or focus.
- **AC-UI-CONTROL-SIZING-001.6:** Disabled and busy states shall retain the
  control's size role. Primary styling shall not increase an ordinary action's height.
- **AC-UI-CONTROL-SIZING-001.7:** Long translated labels shall remain readable
  without overlapping neighboring controls. Equivalent controls shall retain equal heights.
- **AC-UI-CONTROL-SIZING-001.8:** Keyboard and touch activation shall retain
  existing actions, accessible names, focus behavior, and disabled behavior.
- **AC-UI-CONTROL-SIZING-001.9:** Text scaling shall remain available. Editable
  touch fields shall retain the existing focus anti-zoom behavior.
- **AC-UI-CONTROL-SIZING-001.10:** Content surfaces and documented specialized
  chrome shall retain their independent geometry. Their ordinary embedded actions
  shall follow the standard or compact control role.

## Compatibility

The dimensions use CSS pixels at a 16px root font and scale with that root font.
Existing documented compact mobile chrome, such as MobilePillButton, retains
its explicit exception. This exception does not apply to ordinary actions.

The settings typography contract remains authoritative for text roles.
This requirement adds the shared height contract for its controls.
The compact workflow navigation contract retains its overlay and task-move behavior.

## Out of scope

- Changes to task state, permissions, APIs, persistence, or action semantics.
- A new theme, density preference, or user-facing configuration.
- Uniform heights for multiline editors, selection cards, and menu options.
- Geometry inside third-party editors, terminals, or external plugin applications.

## Related documents

- [System design](../system-design/control-sizing.md)
- [Settings typography](settings-typography.md)
- [Compact workflow navigation](compact-workflow-step-navigation.md)
