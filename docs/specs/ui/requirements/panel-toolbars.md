---
status: draft
system: ui
created: 2026-09-19
owners:
  - Kandev frontend
---

# Panel toolbar requirements

## Overview

The UI system owns the reusable geometry of toolbars inside task panels.
Feature owners retain actions, runtime states, and navigation semantics.
Adjacent panel toolbars must align below their Dockview tab strips.

## Requirements

### REQ-UI-PANEL-TOOLBARS-001: Consistent panel toolbar geometry

**Intent:** Panel actions occupy one predictable row across the workbench.

#### Acceptance criteria

- **AC-UI-PANEL-TOOLBARS-001.1:** At a 16px root font, each primary panel
  toolbar shall have a fixed border-box height of 30px on fine-pointer desktop.
  Adjacent panels in the same Dockview row shall align their toolbar boundaries.
- **AC-UI-PANEL-TOOLBARS-001.2:** Below 768px or with a coarse primary pointer,
  equivalent toolbars shall use a fixed 48px height and targets at least 44px.
  Geometry shall scale with the root font.
- **AC-UI-PANEL-TOOLBARS-001.3:** Narrow panels and long translated labels shall
  not wrap or increase toolbar height. Titles may truncate; secondary actions
  shall remain reachable through a visible overflow control.
- **AC-UI-PANEL-TOOLBARS-001.4:** Loading, disabled, busy, and empty states shall
  retain toolbar geometry, accessible names, keyboard operation, and focus.
- **AC-UI-PANEL-TOOLBARS-001.5:** Changing viewport or panel width shall not
  reset feature state or hide required actions without an alternate path.
  Toolbars shall remain outside the content scroll region.
- **AC-UI-PANEL-TOOLBARS-001.6:** Each toolbar control shall fit within its row
  without clipping, overlapping targets, or document horizontal overflow.

## Boundaries

Dockview tab strips, global page navigation, footers, section headings, chat
composers, and toolbars rendered inside third-party applications have independent
geometry. A panel need not gain a toolbar if it has no toolbar actions.
Nested per-file diff headers are content headings, not primary panel toolbars.

## Related documents

- [Design](../system-design/panel-toolbars.md)
- [Control sizing](control-sizing.md)
- [Canvas host requirements](../../canvases/requirements/agent-authored-web-apps.md)
- [Implementation plan](../../../plans/canvas-same-origin-auth/plan.md)
