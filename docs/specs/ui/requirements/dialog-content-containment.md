---
status: active
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Growing Dialog Content Containment Requirements

## Overview

Dialogs whose content grows from user or runtime data must remain usable when
that content is taller than the current browser viewport. The UI system owns
the reusable viewport-containment contract. Each consuming system continues to
own its data, actions, validation, and dismissal semantics.

The current consumers are the agent-profile deletion conflict dialog,
Marketplace Sources, System Health Issues, plugin-hosted dialog presentation,
and the Office Create Project dialog.

## Terminology

- **Growing content:** Repeated or plugin-provided content whose rendered height
  is not fixed, such as conflict rows, sources, health issues, plugin content,
  or repository chips.
- **Scroll body:** The single region inside a covered dialog that absorbs
  vertical growth.
- **Persistent controls:** Controls intentionally placed outside the scroll
  body because they must remain reachable, such as confirmation actions, an
  add form, or the dialog close control.

## Requirements

### REQ-UI-DIALOG-CONTENT-CONTAINMENT-001: Reachable dialog content and controls

**Intent:** Keep growing dialog content and required actions reachable at every
supported viewport size.

**User story:** As a user working in a dialog with many items, I want to scroll
the items without losing the dialog controls, so that I can review the content
and complete or cancel the operation.

#### Acceptance criteria

- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1:** When growing content exceeds the
  available dynamic viewport height, the system shall contain the outer dialog
  within that viewport and make exactly one designated body vertically
  scrollable.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.2:** While the body scrolls, the system
  shall keep the dialog title and any host-owned close control visible.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.3:** The system shall keep controls
  designated as persistent outside the scroll body and reachable without
  scrolling that body. This includes the agent conflict decision footer, the
  Marketplace add-source form, and the Office Create Project footer.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.4:** Every item and item-level action
  inside the body shall remain reachable by scrolling, with its existing
  content order and business behavior unchanged.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.5:** When growing content fits, the
  system shall retain a compact intrinsic dialog height without an unnecessary
  internal scrollbar.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.6:** When growing content changes while
  a dialog is open, the designated body shall absorb the new height without
  moving persistent controls outside the viewport.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.7:** On a phone viewport, each covered
  dialog shall remain within the dynamic viewport, shall not create document
  horizontal overflow, and shall expose required actions with touch targets at
  least 44 pixels high.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.8:** Each covered dialog shall retain
  its existing focus management, Escape behavior, outside-interaction behavior,
  action ordering, and semantic confirmation behavior.
- **AC-UI-DIALOG-CONTENT-CONTAINMENT-001.9:** Surface-specific safety rules shall
  remain unchanged: agent hard blockers omit Delete Anyway, health Fix actions
  navigate to their current targets, plugin dismissibility remains host
  enforced, and plugin drawer presentation remains unchanged.

## Out of scope

- Changing backend payloads, persisted models, validation, or business actions.
- Adding per-item or bulk remediation actions to the agent deletion conflict.
- Changing plugin APIs or a plugin's selected dialog versus drawer
  presentation.
- Changing conflict, marketplace, health, plugin, or Office content and order.
- Applying a global behavioral change to the shared Dialog or AlertDialog
  primitives.
- Retrofitting dialogs outside the five audited consumers in this package.
