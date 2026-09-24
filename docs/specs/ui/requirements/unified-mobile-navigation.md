---
status: active
system: ui
created: 2026-09-15
owners:
  - Kandev
---

# Unified mobile navigation requirements

## Overview

The phone hamburger consistently opens app navigation. Task switching remains
a two-tap interaction through the task title. Listing-title dropdowns open page display controls. UI owns this reusable cross-page interaction; tasks,
workspaces, permissions, saved views, and navigation destinations retain their
existing domain owners.

## Requirements

### REQ-UI-MOBILE-MENU-001: Consistent app navigation

**Intent:** Make the same navigation control predictable across phone pages.

#### Acceptance criteria

- **AC-UI-MOBILE-MENU-001.1:** Below 768 CSS pixels, the hamburger on Home,
  Kanban, List, Threads, task workbenches, and shared page shells shall open
  the same app-navigation surface with the same accessible name and relative
  section order. It shall not directly open the task picker or display settings.
- **AC-UI-MOBILE-MENU-001.2:** The menu shall show the active workspace first,
  followed by global destinations, a collapsible Tasks sidebar, workspace actions,
  integrations/plugin destinations, and utilities. Available destinations and
  their relative order shall depend on workspace mode and permissions, not the
  originating page. The current destination shall be visibly and semantically
  identified. Home shall represent all three listing modes; task selection is identified
  inside the embedded Tasks section.
- **AC-UI-MOBILE-MENU-001.3:** Existing Office and Settings local navigation
  shall remain reachable in a labelled section after global navigation. Office
  workspaces shall not gain Kanban-only task views or task shortcuts. Workspace
  selection shall retain existing access checks and route-recovery behavior.
- **AC-UI-MOBILE-MENU-001.4:** Phone listing pages shall open View
  options from their top-level Kanban, Threads, or List dropdown, without a
  separate View options button. It shall retain the applicable listing
  modes, search toggle, sorting, grouping, display options, saved-view actions,
  and their error/retry flows. Existing primary creation actions shall remain
  reachable. The menu shall not duplicate those display controls.
- **AC-UI-MOBILE-MENU-001.5:** The phone menu shall use an inset bottom drawer,
  a fixed heading, one vertical content scroller, bottom safe-area clearance,
  and at least 44-by-44 CSS-pixel standalone controls. Long content shall remain
  contained at phone widths including 767 CSS pixels, without document
  horizontal overflow. Dismissal shall restore focus to its visible opener;
  a subsequent dialog shall open after the menu closes without losing its draft.
- **AC-UI-MOBILE-MENU-001.6:** Tablet and desktop navigation and layout controls
  shall retain their existing composition. Phone changes shall not overwrite
  saved desktop listing preferences, including the Pipeline fallback, or
  discard an already-open task dialog on a breakpoint change.

### REQ-UI-MOBILE-MENU-002: Discoverable task switching

**Intent:** Keep task switching fast after the hamburger becomes global.

#### Acceptance criteria

- **AC-UI-MOBILE-MENU-002.1:** The phone task title shall be a labelled button
  with a visible downward chevron, expanded state, and a minimum 44 CSS-pixel
  height and width. Tapping it shall open the existing task picker. Selecting
  a visible task shall switch tasks in two taps from the workbench.
- **AC-UI-MOBILE-MENU-002.2:** Task selection shall retain the existing
  workbench history policy, selected-session handling, panel fallback, saved
  task views, filters, workspace context, and row-level task actions. Opening
  or dismissing either menu shall not start a session or change the active task.
- **AC-UI-MOBILE-MENU-002.3:** A long or loading task title shall leave both
  title picker and hamburger reachable without overlap. Task-list loading,
  empty, and error states shall remain distinguishable, with existing retry
  behavior. Dismissing the picker shall restore focus to its title trigger.

### REQ-UI-MOBILE-MENU-003: Bounded pinned-task shortcuts (superseded)

Superseded on 2026-09-17 by REQ-UI-MOBILE-MENU-004 after user testing.
The criteria below record the first iteration, not the revised target.

**Intent:** Reach frequently used tasks from app navigation without browsing
the full task list.

#### Acceptance criteria

- **AC-UI-MOBILE-MENU-003.1:** In a Kanban workspace, the menu shall show up to
  three distinct pinned, unarchived tasks belonging to the active workspace,
  in existing pin order. Foreign, missing, deleted, and archived tasks shall
  not appear. With no eligible pins, the shortcut section shall be absent.
- **AC-UI-MOBILE-MENU-003.2:** A shortcut shall close navigation and open its
  task using the same navigation policy as the task picker for the originating
  surface. Selecting a shortcut from a non-task page shall preserve browser Back
  to that page. The existing Task views entry shall remain the full-list path,
  preserving saved-view selection and editing.
- **AC-UI-MOBILE-MENU-003.3:** While workspace data is loading or inaccessible,
  no shortcut from the preceding workspace shall appear. A failed task read
  shall leave global navigation usable and offer a labelled retry; task-read
  denial shall not expose cached task titles. Pin changes shall update the
  section without resetting saved task filters or persisting a new order.

### REQ-UI-MOBILE-MENU-004: Embedded task sidebar

**Intent:** Browse and manage tasks directly within the shared navigation menu.

#### Acceptance criteria

- **AC-UI-MOBILE-MENU-004.1:** The middle of the phone hamburger menu shall
  contain a collapsible Tasks section using the existing task sidebar. It shall
  start expanded, collapse in place, and retain its disclosure state while its
  host remains mounted. A separate pinned-shortcuts section and Task views
  navigation action shall not appear. Existing sidebar pin behavior remains
  part of the normal task list, not a second navigation section.
- **AC-UI-MOBILE-MENU-004.2:** The embedded sidebar shall retain saved views,
  filters, task grouping, task selection, creation, and row actions. Its rows
  shall share the menu's content scroller rather than create a nested vertical
  task scroller. Collapsing Tasks shall leave app destinations and utilities
  accessible without changing task/view state. The task-title picker remains.
- **AC-UI-MOBILE-MENU-004.3:** Embedded selection shall preserve push navigation
  from non-task pages and replacement within the workbench. Workspace changes,
  loading, denial, retry, focus return, action dialogs, and rotation shall retain
  existing protections. Office shall not gain a Kanban task sidebar.

### REQ-UI-MOBILE-MENU-005: Listing-title view options

**Intent:** Use the existing listing context to configure that listing.

#### Acceptance criteria

- **AC-UI-MOBILE-MENU-005.1:** Tapping the top-level Kanban, Threads, or List
  label and chevron shall open the corresponding phone view-options surface.
  No separate View options button or toolbar row shall consume content height.
- **AC-UI-MOBILE-MENU-005.2:** Applicable mode selection, search, sorting,
  grouping, display settings, saved views, editing, and error recovery shall
  remain reachable through that surface. Threads saved-view controls shall be
  included there, rather than intercept the top-level dropdown.
- **AC-UI-MOBILE-MENU-005.3:** Closing options shall restore focus to the
  listing-title dropdown unless transferring focus to search or another dialog.
  Desktop/tablet composition, saved preferences, and search-hide clearing remain
  unchanged. Both dropdown and Tasks disclosure have 44px touch targets.

## Menu hierarchy revision (2026-09-19)

### REQ-UI-MOBILE-MENU-006: One Home destination and coherent sections

- **AC-UI-MOBILE-MENU-006.1:** The phone menu shall expose Home as its only
  global task-listing destination, with no separate Tasks or Threads route rows.
  Kanban/List/Threads remain selectable through the listing-title dropdown.
  Home shall be current on all three listing routes and preserve the existing
  workspace-aware Home destination and saved listing preference behavior.
- **AC-UI-MOBILE-MENU-006.2:** The embedded Tasks disclosure shall use the same
  heading typography, left alignment, divider treatment, and section spacing as
  Utilities. A subordinate chevron shall communicate expansion; the adjacent
  create-task plus shall remain visible in both states, independently usable,
  and accessible. Both controls retain separate 44px hit areas. Collapsing shall
  preserve existing data, selection, and dialog behavior.
- **AC-UI-MOBILE-MENU-006.3:** The phone menu shall expose an Automations section
  in regular workspaces, including the existing automation list/detail and setup
  paths, even when no automation exists. Available entries shall reflect the
  active workspace, with distinguishable loading, empty, and failed reads.
- **AC-UI-MOBILE-MENU-006.4:** Integrations shall remain discoverable when no
  configured provider links exist by exposing integration settings/setup. Actual
  provider links shall retain credential, permission, enabled-visibility, and
  plugin availability rules. The menu shall not label unconfigured providers as
  connected or change credentials. Switching workspace shall not show stale
  automation or integration entries from the previous workspace.

## Action-first menu revision (2026-09-19)

### REQ-UI-MOBILE-MENU-007: Quick actions and collapsible integrations

- **AC-UI-MOBILE-MENU-007.1:** In the phone menu, Home shall be immediately
  followed by side-by-side Quick Chat and Quick terminal actions, before any
  page-local navigation or expandable task content. Expanding Tasks shall not
  move those actions below the task list. Existing workspace context, activity
  indicators, launch behavior, and focus handoff shall be retained.
- **AC-UI-MOBILE-MENU-007.2:** Regular workspace sections shall follow Tasks,
  Automations, Integrations, Utilities order. Existing eligible plugin actions
  shall share one Plugins section, with workspace and task context distinguished
  when both are present. Optional system metrics shall follow navigation instead
  of separating plugin controls. Canvases and page-local destinations shall
  remain reachable. Utilities shall
  place Settings before Stats, followed by theme and support actions; existing
  conditional status and health affordances shall retain their behavior.
- **AC-UI-MOBILE-MENU-007.3:** Integrations shall use a labelled, keyboard- and
  touch-operable disclosure matching Tasks and Automations, initially collapsed.
  Expanding shall reveal eligible provider/plugin links and integration settings,
  including settings when no providers are configured. Collapsing shall hide
  those rows without changing configuration, route, or workspace. Its state is
  temporary for the mounted menu; no new saved preference is required.
- **AC-UI-MOBILE-MENU-007.4:** Phone quick actions and disclosures shall have
  at least 44px touch targets, remain readable without clipping at 393px and
  767px, and share the existing menu scroller. Integration availability and
  workspace isolation shall remain authoritative even while collapsed. Wider
  navigation composition, Tasks' initial expanded state, and Automations'
  initial collapsed state shall remain unchanged.

## Out of scope

- Global bottom navigation, a second top-bar task-actions menu, or redesigned
  task rows and task mutations.
- New visit-history persistence, pin settings, task APIs, feature toggles, or
  changes to Office task ownership.
- Desktop/tablet layout redesign and provider-specific local navigation redesign.

## Related contracts

- [Task chrome](mobile-task-chrome.md): the title becomes its retained
  task-drawer entry; task actions stay on task rows.
- [Task-view access](mobile-task-view-access.md): preserves controller lifetime, history, and focus guarantees; the embedded
  Tasks section supersedes its separate Task views navigation label.
- [Listing preferences](task-listing-display-preferences.md): retains storage,
  explicit Home semantics, and phone Pipeline fallback.
- [Workspace read recovery](../../workspaces/requirements/workspace-read-recovery.md).

## Implementation plans

- [Unified mobile navigation](../../../plans/unified-mobile-navigation/plan.md)
- [Coherent mobile plugin menu](../../../plans/mobile-plugin-menu-coherence/plan.md)
