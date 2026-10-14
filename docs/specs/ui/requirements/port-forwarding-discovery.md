---
status: active
system: ui
created: 2026-08-07
updated: 2026-10-09
owners:
  - kandev
---

# Task-scoped port-forwarding discovery Requirements

## Overview

Developers accessing Kandev remotely need to find services in the selected task
session. UI owns discovery and shortcut presentation; existing port APIs retain
runtime transport, authorization, and tunnel lifecycle ownership.

Phone access uses the session's Panels picker rather than task navigation.
Desktop and tablet retain compatible launchers. Technical details live in the
paired system design.

## Terminology

- **Phone:** Viewport below 768px, regardless of pointer precision.
- **Header shortcut preference:** Existing per-task preference controlling the
  network button in the task header. It does not enable or disable tunnels.
- **Panels picker:** The session bottom dock's picker for tools, canvases, and
  mobile-enabled plugin panels.

## Requirements

### REQ-UI-PORT-FORWARDING-DISCOVERY-001: Task-scoped port-forwarding discovery

**Intent:** Make port management reachable without mixing task navigation,
shortcut visibility, and tunnel operations.

#### Acceptance criteria

- **AC-UI-PORT-FORWARDING-DISCOVERY-001.1:** The desktop task `+` launcher shall
  retain its checkable Port forwarding entry without creating a Dockview panel.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.2:** Phone task sessions shall expose
  Port forwarding through bottom-dock Panels, including tasks without canvases
  or plugins. Phone app navigation and task pickers shall omit the action.
  Tablet task-switcher sheets shall retain their checkable task action.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.3:** Local and remote executors shall
  expose the same action. Missing session, unavailable agentctl, archived task,
  or pending preference write shall disable it with an accessible explanation.
  Disabled selection shall issue no runtime or preference mutations.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.4:** Selecting the unchecked desktop or
  tablet entry shall persist the preference, reveal the eligible header control,
  and open management after successful persistence.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.5:** Disabling the header shortcut shall
  hide its control without modifying tunnels. Desktop/tablet checked launchers
  shall retain their toggle behavior. Re-enabling shall expose existing tunnels.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.6:** The header control shall appear when
  the preference is enabled and the selected session is eligible. Selecting it
  shall open management; stopping the last tunnel shall not hide the control.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.7:** The preference shall remain per task,
  default off, persist across reloads, backend restarts and session replacement,
  and reconcile across clients. Phone navigation and viewport changes shall not
  overwrite it.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.8:** A failed preference write shall
  restore the prior state and show the existing actionable error. Failed wider
  enable attempts shall not open management. Failed phone preference changes
  shall leave an already open manager usable.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.9:** Selecting the phone row shall open
  management with the shortcut either off or on, without changing that preference
  or starting/stopping tunnels. It shall close Panels, preserve the selected
  content panel, and reopen management on subsequent visits rather than toggle off.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.10:** Both phone entry points shall open
  one bottom drawer with existing active-first ordering, Open, Copy, Start, Stop,
  refresh, and manual addition. A separate Show in task header preference shall
  remain available there; changing it shall keep the phone drawer open. Closing
  either surface shall leave tunnels unchanged.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.11:** Picker and manager shall each have
  one vertical scroller, dynamic viewport containment, safe-area clearance, and
  no document horizontal overflow. At 320px, canonical phone width, and 767px,
  actions shall remain reachable with at least 44px hit areas. Long URLs and
  translations shall not cover controls; manual addition shall remain reachable
  with the keyboard open.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.12:** All actions shall be keyboard
  accessible and localized. Picker handoff shall not focus behind the manager.
  Dismissal shall focus the visible Panels button or the visible header opener
  used for that visit, falling back to Panels if the header shortcut was hidden.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.13:** Task/session change, readiness loss,
  or archival shall close the phone manager. Late results shall not reopen it or
  show another session's ports. Reload shall not reopen it. Responsive changes
  shall preserve an open visit's manual draft and selected content panel.
- **AC-UI-PORT-FORWARDING-DISCOVERY-001.14:** Existing canvas/plugin selection
  shall remain available in Panels. Port management shall not become a saved
  mobile panel or change Dockview persistence. Wider layouts shall retain their
  Dialog and existing Browser-panel capability.

## Related contracts

- [Active-first management](port-forwarding-active-first.md): existing row/status
  and mutation semantics apply to the phone drawer.
- [Proxy Browser access](port-proxy-browser-panel.md): existing URL targets and
  phone Open/Copy remain unchanged.
- [System design](../system-design/port-forwarding-discovery.md).

## Out of scope

New transports, proxy modes, detection, automatic forwarding, durable tunnels,
global tunnel management, runtime flags, or desktop Dockview panels.

## Implementation Plans

- [Mobile port forwarding](../../../plans/mobile-port-forwarding/plan.md).
