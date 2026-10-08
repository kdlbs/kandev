---
status: active
system: ui
created: 2026-10-05
owners:
  - kandev
---

# Active-first port forwarding Requirements

## Overview

Developers need to find services they are already forwarding before browsing
other ports. UI owns this list prioritization and interaction contract, using
existing session tunnel state. Runtime transport and tunnel lifecycle remain
with the existing port APIs. The separate [discovery contract](port-forwarding-discovery.md)
owns task visibility preferences and entry points; this capability does not
replace it.

## Terminology

- **Forwarded port:** A target port with a confirmed active dedicated tunnel.
- **Other port:** A detected or manually added target port without an active tunnel.
- **Proxy URL:** The existing path-based access URL. Its availability alone does
  not mean a dedicated tunnel is running.

## Requirements

### REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001: Active-first port management

**Intent:** Make existing forwards easy to find, open, copy, and stop.

#### Acceptance criteria

- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1:** When active tunnels exist, the
  dialog shall place every forwarded target before every other target, under a
  visible Forwarded ports heading and active count. Each group shall use
  ascending numeric target-port order.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.2:** Each target shall appear once,
  even when detected, manually added, and forwarded at the same time. Active
  tunnels absent from detection shall remain visible, including when tunnel
  information arrives after port detection.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.3:** Each forwarded row shall show a
  text Forwarding status, its target port, its dedicated tunnel URL with Open
  and Copy actions, and a Stop action. Detected/manual provenance and proxy
  access shall remain available. The dedicated tunnel URL shall precede proxy
  access; color shall not be the only active indicator.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.4:** A successful Start shall promote
  the row and update the active count without a manual refresh. A successful
  Stop shall remove active status and retain the target under Other ports for
  the current dialog visit. Pending and failed operations shall preserve the
  last confirmed grouping; failures shall retain existing error feedback.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.5:** When no active tunnels exist,
  the dialog shall omit the Forwarded ports group and continue to offer other
  ports, refresh, and manual addition. Detection loading or an empty detection
  result shall not hide known forwarded ports or imply that those tunnels stopped.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.6:** On phones, the same ordering and
  operations shall be available through the existing task entry point. URLs and
  actions shall stack without horizontal document overflow; action hit areas
  shall be at least 44px on phone/coarse-pointer devices. The bounded dialog
  shall have one content scroller and safe-area clearance for its final controls.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.7:** Status, headings, counts, and
  action names shall be localized. Keyboard users shall reach all actions,
  retain focus within the acted-on row when it moves between groups, and return
  to the opener on dismissal. Long URLs and process names shall not obscure controls.
- **AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.8:** Session changes shall display
  only the selected session's ports and tunnels; delayed responses from an old
  session shall not populate the new session's list or active count.

## Out of scope

New transports, polling, persistent tunnels, automatic forwarding, bulk stop,
new top-bar counters, launcher changes, and changes to proxy/Browser panel routing.

## Implementation Plans

- [Active-first port forwarding](../../../plans/port-forwarding-active-first/plan.md)
