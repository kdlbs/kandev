---
status: active
system: agents
created: 2026-10-05
owners:
  - kandev
---

# Agent page options requirements

## Overview

The large navigation preference row separates the installed-agent toolbar from the agent cards.
An Options control gives this occasional preference a compact entry point.
Agents owns this contract because the control exposes an agent-profile preference.
The surface reuses existing dialog and drawer contracts.

## Requirements

### REQ-AGENTS-PAGE-OPTIONS-001: Agent page options

**Intent:** Keep profile management prominent while retaining access to the navigation preference.

#### Acceptance criteria

- **AC-AGENTS-PAGE-OPTIONS-001.1:** On `/settings/agents`, the installed-agent toolbar shall show a labelled Options button immediately before Terminal.
  The page shall remove the standalone navigation preference row from the agent list.
- **AC-AGENTS-PAGE-OPTIONS-001.2:** On desktop, Options shall open a compact dialog titled "Agent options" with one switch.
  Its label shall be "Hide disabled profiles from navigation".
  Its helper text shall be "Disabled profiles remain available on this page."
- **AC-AGENTS-PAGE-OPTIONS-001.3:** A successful toggle shall apply immediately and keep the surface open.
  The surface shall state "Changes apply immediately." and provide Done to close it.
  Done shall dismiss without a separate save operation.
- **AC-AGENTS-PAGE-OPTIONS-001.4:** Below 768px, Options shall open an inset bottom drawer with the same preference and actions.
  Options, Done, and the switch interaction shall have touch targets of at least 44px.
  The surface shall remain within the viewport and clear the bottom safe area.
  Overflowing content shall use one internal scroll region without horizontal document overflow.
- **AC-AGENTS-PAGE-OPTIONS-001.5:** Keyboard and touch users shall be able to open, change, and dismiss the preference.
  Desktop shall support Close and Escape. The drawer shall support its standard dismissal gestures.
  Dismissal shall return focus to Options. Dismissal shall retain a successful preference change.
- **AC-AGENTS-PAGE-OPTIONS-001.6:** All new and revised copy, including accessible names, shall follow the selected locale.
- **AC-AGENTS-PAGE-OPTIONS-001.7:** Reopening Options or reloading the page shall show the saved preference.
  Changing presentation across the phone breakpoint shall preserve its value and expose only one active surface.
- **AC-AGENTS-PAGE-OPTIONS-001.8:** Options shall remain available to users who can view the agents page, including users without agent-management permission.
  Its preference shall retain the behavior defined by `REQ-AGENTS-HIDE-DISABLED-PROFILES-NAV-001`.

## Related contracts

- [Navigation filtering](hide-disabled-profiles-nav.md) owns the default, filtering, immediate application, tab synchronization, and settings-page visibility.
- [Profile layout](settings-profile-layout.md) owns the existing agent cards and toolbar actions.

## Out of scope

- Additional options, profile editor changes, and changes to profile selection or enablement.
- New backend persistence, API contracts, runtime flags, and Office agent controls.

## Implementation plan

- [Agent page options](../../../plans/agent-page-options/plan.md).
