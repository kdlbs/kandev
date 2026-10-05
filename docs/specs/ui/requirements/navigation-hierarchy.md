---
status: active
system: ui
created: 2026-09-28
owners:
  - kandev
---

# Navigation hierarchy

## Overview

Make task creation, navigation destinations, expandable tool groups, and the
contextual task list visibly different. The supplied annotated desktop references
and written feedback prioritize menu hierarchy, a discoverable creation action,
compact task presentation, and easier access to issues on phones.

UI owns this reusable cross-page presentation contract. Task state, workflow
definitions, integration availability, workspace access, and account identity
retain their existing owners.

The 2026-10-05 update revises desktop action placement after feedback on PR #4063.
It restores the compact creation row, header shortcuts, and direct Stats control.
The task-panel and phone contracts retain their existing behavior.

## Relationship to current contracts

This implemented revision updates phone ordering and creation placement,
superseding only the
ordering in AC-UI-MOBILE-MENU-001.2 and 007.2, the create-plus placement in
006.2, the default ordering referenced by AC-UI-SIDEBAR-CUSTOMIZATION-001.3,
and the phone projection in AC-UI-SIDEBAR-CUSTOMIZATION-005.5.
Other guarantees in [unified mobile navigation](unified-mobile-navigation.md)
and [sidebar customization](sidebar-customization.md) continue to apply.
The change does not replace task-row field preferences or task grouping rules.

## Requirements

### REQ-UI-NAV-HIERARCHY-001: Distinct actions, destinations, and disclosures

**Intent:** Users can tell what creates something, what navigates, and what expands.

- **AC-UI-NAV-HIERARCHY-001.1:** In the default expanded desktop sidebar,
  workspace selection shall remain in the fixed header. New Task shall appear
  before Home as a compact, left-aligned navigation-style button. Its leading
  creation icon and label shall share one row with independent actions on the right.
  The button shall match ordinary navigation-row density and neutral styling.
  It shall open the existing creation flow for the active
  workspace, with a disabled state when no eligible workspace is available.
  It shall have no visible shortcut hint.
  The effective configured keyboard shortcut shall remain available.
- **AC-UI-NAV-HIERARCHY-001.2:** Home and eligible direct destinations shall
  be links. Home shall indicate the current destination across the regular
  Kanban, List, and Threads routes. Automations, Canvases, and Integrations
  shall use labelled icon-and-chevron disclosures; activating a disclosure
  shall only expand or collapse its children. Child destinations shall be
  visibly indented without vertical guide lines and shall navigate independently.
  Open automations shall be a labelled child after the expanded automation rows,
  matching the placement of Integration settings on desktop and phones.
- **AC-UI-NAV-HIERARCHY-001.3:** Integrations shall retain eligible provider
  and plugin destinations and an integration-settings path when no provider
  is configured. Availability shall follow the active workspace. A collapsed
  built-in disclosure shall not depend on an unlabelled icon strip for access.
  The expanded desktop sidebar shall also show eligible first-party provider
  shortcut icons on the right of the Integrations label, before its chevron.
  These shortcuts shall remain visible when the disclosure is closed.
  Each shortcut shall have a localized accessible name and navigate independently.
  The header shall show no provider shortcuts when none are eligible.
  Providers beyond the header capacity and plugin destinations shall remain in the children.
  Explicit user-created shortcut groups shall retain their direct icon actions.
- **AC-UI-NAV-HIERARCHY-001.4:** Quick Chat, Quick terminal, workspace actions,
  required inboxes, canvases, and eligible plugin navigation shall remain
  reachable. Desktop Quick terminal and Quick Chat shall be independent icon
  buttons on the right of New Task, in that order, without a second utility row.
  They shall retain accessible names, desktop tooltips, and Quick Chat activity cues.
  Desktop quick-action icons shall use the compact 24px size.
  Eligible workspace plugin actions shall follow these quick actions.
  Excess plugin actions shall wrap without displacing or overlapping the built-in controls.
  Phone quick actions shall retain their labelled utility bar and launch behavior.
  Phone and coarse-pointer actions shall retain 44px touch targets.
  Opening navigation shall not invoke any of these actions.
- **AC-UI-NAV-HIERARCHY-001.5:** The expanded desktop footer shall keep a
  labelled Settings destination and, only for an authenticated user, the actual
  account avatar with identity and the existing action in its menu. The footer
  shall occupy one row, with Settings, a direct Stats icon button, theme switching,
  and a More Actions menu, in that order. Stats shall sit immediately left of the
  theme button, with a localized accessible name and tooltip.
  Stats shall not appear in the More Actions menu.
  Plugin footer destinations, Improve Kandev, and available release notes
  shall remain reachable as labelled menu items. Unseen release notes shall keep
  an indicator on the menu trigger. Plugin count shall not add footer rows. The collapsed rail shall retain named launchers,
  workspace switching, and its current expand/hover behavior.
- **AC-UI-NAV-HIERARCHY-001.6:** Saved desktop layout order and visibility
  shall survive the revision, reload, and workspace switches. Visible New Task
  shall receive the compact action row at its saved position; the default order
  applies only to uncustomized/reset layouts. All changed controls shall have
  keyboard operation, visible focus, truthful current/expanded states, and
  localized accessible names in every shipped locale.
- **AC-UI-NAV-HIERARCHY-001.7:** In the expanded desktop sidebar, the Canvases
  settings shortcut shall appear after its label and before its rightmost chevron.
  It shall remain available while the disclosure is closed.
  Activating the shortcut shall open the active workspace's canvas settings without toggling the disclosure.
  Activating the label or chevron shall only toggle the disclosure.
  Header actions shall remain separate controls without nested buttons or links.

### REQ-UI-NAV-HIERARCHY-002: Compact contextual task panel

**Intent:** Users scan and filter tasks without mistaking the list for app navigation.

- **AC-UI-NAV-HIERARCHY-002.1:** Regular workspaces shall show one contextual
  Tasks section, visually separated from app navigation, with a saved-view
  selector and filter control. The section heading shall expand/collapse its
  content without navigating or changing the selected task. Office shall retain
  its existing task navigation rather than gaining the regular task panel.
- **AC-UI-NAV-HIERARCHY-002.2:** The filter control shall indicate applied
  filters even after they have been saved and after reload. Applied filtering
  and unsaved view changes shall have distinguishable accessible descriptions.
  Sorting, grouping, or row-presentation edits alone shall not claim that task
  filters are active. Clearing all clauses shall remove the applied-filter cue.
- **AC-UI-NAV-HIERARCHY-002.3:** When grouping by task state, each group shall
  show a semantic status indicator, label, count badge, and expansion control.
  The indicator shall match the existing task-state meaning, including distinct
  waiting, blocked, failed, cancelled, and completed states. Other grouping
  modes shall retain their actual labels/counts without misleading status colors.
  Every named group, including workflow steps, shall use a leading disclosure
  chevron and a stronger heading above indented tasks. Spacing shall distinguish
  groups without vertical guide lines. Nested task content shall retain its
  deeper indent. Ungrouped lists shall not gain an empty heading or group indentation.
  Group collapse shall preserve selection, filters, order, and pagination.
- **AC-UI-NAV-HIERARCHY-002.4:** Each task shall be a plain row without a
  persistent card border or fill, with a stable status slot, truncated title,
  configured details, and configured trailing data. Hover, active selection,
  multiselection, and keyboard focus shall remain distinguishable. Missing
  metadata shall consume no placeholder space, and task colors,
  nested tasks, PR/MR badges, attention cues, and diff statistics shall retain
  their current meanings. Compact styling shall not widen the sidebar or add
  extra metadata lines beyond the user's row configuration.
- **AC-UI-NAV-HIERARCHY-002.5:** Row navigation, keyboard selection, desktop
  context actions, and visible phone row actions shall remain functional.
  Long titles, large counts, empty results, read failures, and continued groups
  shall remain readable and recoverable without horizontal document overflow.
  Task scrolling shall leave desktop workspace and footer controls available.
- **AC-UI-NAV-HIERARCHY-002.6:** Existing saved view grouping, field visibility,
  sort order, and workspace scope shall be retained. Users shall still select
  state, repository, workflow, step, executor, or ungrouped views. The reference
  comparison shall use an explicitly selected state-grouped view, without
  silently changing existing users' default grouping.

### REQ-UI-NAV-HIERARCHY-003: Discoverable phone tools and issues

**Intent:** A long task list cannot put integration navigation beyond the list.

- **AC-UI-NAV-HIERARCHY-003.1:** Below 768 CSS pixels, the existing hamburger
  shall open the shared inset bottom navigation drawer. Its workspace control
  shall stay available above the content scroller. In a regular default layout,
  content shall show New Task, Home and quick actions, eligible tool disclosures,
  contextual Tasks, then utilities. Expanding Tasks shall not move integration
  entry points below task rows. Settings/Office local navigation shall remain
  explicitly labelled and reachable.
- **AC-UI-NAV-HIERARCHY-003.2:** Phone New Task shall be a labelled neutral
  creation control and shall open one existing workspace-correct creation flow
  after closing the menu. It shall not duplicate the regular Tasks-header plus
  when visible. If the user has hidden the built-in New Task entry, Tasks shall
  retain its independent creation action. Explicit custom shortcuts and Office's
  creation path shall remain available.
- **AC-UI-NAV-HIERARCHY-003.3:** From Home and a task workbench, users with an
  eligible GitHub connection shall be able to expand Integrations, activate
  GitHub, open its existing Issues selector, and reach a seeded issue list using
  labelled touch controls. No hover, long press, or desktop-only control shall be
  required. A workspace without that connection shall show setup without a
  falsely connected provider entry. Browser Back shall retain existing history
  behavior.
- **AC-UI-NAV-HIERARCHY-003.4:** Saved phone layouts shall project visible
  Home/creation actions followed by eligible tools and custom groups, then Tasks
  and utilities. Tool and custom-group relative order and visibility shall be
  retained. This phone composition shall never be saved over desktop order.
  Unavailable or foreign-workspace entries shall remain ineligible; optional
  plugin controls shall keep their existing ownership and deduplication.
- **AC-UI-NAV-HIERARCHY-003.5:** The drawer shall have one vertical content
  scroller, dynamic viewport bounds, safe-area clearance, no document horizontal
  overflow, and touch targets of at least 44 CSS pixels for changed controls.
  This shall hold on 360px, 393px, and 767px phone layouts, including a narrow
  fine-pointer window, and on coarse-pointer controls at the tablet boundary.
  Dismissal shall restore focus to its visible opener unless handing focus to a
  launched surface. Dark/light themes and localized long labels shall remain usable.
- **AC-UI-NAV-HIERARCHY-003.6:** Task-title switching, task selection history,
  saved-view drafts, loading/retry behavior, and action-dialog lifetime shall
  survive the menu reordering and phone/desktop breakpoint changes. An open
  creation draft shall not be lost when the viewport changes. App navigation
  shall remain usable when task data cannot load.

## Out of scope

- New task states, workflow changes, credentials, provider APIs, account pages,
  task mutations, assignee filtering, or a second navigation manifest.
- Enforcing the screenshots' sample workflow names or blue color across themes.
- Large padded task cards, a wider sidebar, or new card-style preferences.
- Redesigning provider dashboards or replacing the phone task-title picker.
- Commits, publication, marketing media, or real provider/agent activity.

## Delivery

- [Implementation plan and comparison work order](../../../plans/navigation-hierarchy/plan.md)
- [Desktop action-placement revision](../../../plans/sidebar-action-placement/plan.md)
