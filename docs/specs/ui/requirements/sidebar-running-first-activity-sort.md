---
status: active
system: ui
created: 2026-10-06
updated: 2026-10-06
owners:
  - kandev
---

# Sidebar sort rules and color ranking requirements

## Overview

Users can order sidebar tasks with several sort rules. Each later rule breaks
only ties from earlier rules. The requested example is running first, red first,
then newest activity. UI owns this personal saved-view preference. Tasks owns
runtime state and activity publication.

This revision replaces the original fixed-preset proposal. It extends
[Last activity](sidebar-last-activity-sort.md) and uses the existing
[effective task color](sidebar-automatic-task-colors.md).

## Terminology

- **Sort chain:** An ordered list of one to ten rules, evaluated from first to last.
- **Running task:** A task whose sidebar primary session has runtime state `RUNNING`.
  Workflow placement alone does not establish that an agent runs.
- **Included subtree:** A task and all descendants remaining after view filters.
  Collapse hides rows without removing their contributions.
- **Tree activity:** The newest semantic activity in an included subtree.
- **Effective color:** The row's first matching automatic color, or its manual
  color when no automatic rule applies. Color is personal, not shared task priority.
- **Preferred color:** A named palette color selected in a color-ranking rule.
  That rule separates matching rows from all other rows.

## Requirements

### REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001: Composable sidebar task order

**Intent:** Users combine runtime, color priority, and activity without a preset
for every possible combination.

#### Acceptance criteria

- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.1:** Desktop and phone editors shall let
  users add, edit, remove, and reorder sort rules. Each rule shall show its field
  and order. Color rules shall also show the preferred color with a text label.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2:** A chain of Running first, Red first,
  and Last activity newest first shall place running red trees before running
  non-red trees, then non-running red trees before non-running non-red trees.
  Each set shall use newest tree activity first.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.3:** Running and activity rules shall use
  complete included subtrees before paging. Running descendants shall promote
  ancestors. Filtered descendants shall contribute neither status nor activity.
  Orphaned included children shall rank as roots. Collapse shall not change rank.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.4:** Nesting, pins, manual child order,
  and group-heading order shall retain precedence. The chain shall apply within
  the remaining group and sibling sets. Rows shall keep their own state and time.
  Any Last activity rule shall make rows show their own activity time.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.5:** Saved views and drafts shall retain
  all rules, rule order, directions, and selected colors across save, reload,
  boot hydration, settings updates, workspace switches, and another signed-in client.
  Existing single-sort views and default views shall retain their behavior.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6:** Server-backed pages and complete
  local data shall produce the same chain order. Only after all rules tie shall
  the existing canonical sidebar tie order apply. Pages shall retain the existing
  100-task-row limit and continuation behavior.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7:** Accepted state, activity, or effective
  color changes shall update ranking through existing refresh behavior.
  Changing a manual color or automatic rule shall invalidate affected ordering.
  Background freshness shall not replace activity. Reordering shall preserve
  the open task and conversation.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.8:** Phone users shall configure the same
  chain through the existing task picker and view-editor drawer. Controls shall
  remain touch-reachable, keyboard-accessible, and viewport-contained. Reorder
  and remove actions shall not depend on hover or drag gestures.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.9:** Unknown runtime state shall rank as
  non-running. Missing activity shall use existing task timestamp fallbacks.
  Loading, empty, query error, and settings-write error shall retain existing recovery behavior.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.10:** A color rule shall compare each
  subtree root's own effective marker token. Automatic color shall take precedence
  over manual color. No matching rule, cleared manual color, or missing color
  shall rank with other nonmatches. A child's color shall not recolor or promote
  its parent. Custom hexadecimal colors shall not count as a named red token.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.11:** Moving Color above Running shall
  make preferred-color tasks precede nonmatching tasks regardless of runtime.
  Moving Last activity above Color shall make activity take precedence over color.
  Removing Color shall remove its ranking effect without changing stored task colors.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.12:** Users shall combine existing Status,
  Updated, Last activity, Created, and Title sorts with Running and preferred-color
  ranking. Each field shall support its two orders. Custom manual order shall
  remain available as a standalone mode and preserve its existing behavior.
- **AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.13:** The editor shall keep at least one
  rule and allow at most ten. Ordinary fields shall occur once; different
  preferred colors can occupy separate rules. Invalid queries shall identify
  the invalid rule without silently discarding ranking. Unsupported stored data
  shall follow documented normalization without discarding valid rules.

### REQ-UI-SIDEBAR-GROUP-INDENT-001: Optional grouped-task indentation

**Intent:** Users can align grouped task rows with the normal sidebar task inset
while retaining group headings and subtask hierarchy.

#### Acceptance criteria

- **AC-UI-SIDEBAR-GROUP-INDENT-001.1:** Grouped-task indentation shall default
  to enabled for new views and existing views without an explicit preference.
  An explicit disabled preference shall survive normalization and reload.
- **AC-UI-SIDEBAR-GROUP-INDENT-001.2:** The localized **Indent grouped tasks**
  toggle shall appear below the grouping selector only while **Group by** is
  expanded. The collapsed section shall show its existing grouping summary and
  no toggle. Expansion or collapse shall not change the saved preference or draft.
- **AC-UI-SIDEBAR-GROUP-INDENT-001.3:** Disabling indentation shall remove only
  the additional group-body inset. Group headings, collapse controls, counts,
  subtask nesting, row markers, selection, and drag targets shall retain behavior.
  Enabling it shall restore the current inset for every grouping dimension.
- **AC-UI-SIDEBAR-GROUP-INDENT-001.4:** The preference shall belong to each
  workspace-scoped saved view and its draft. Save, reload, duplication, workspace
  switching, and settings synchronization shall preserve its explicit value.
  Changing the grouping dimension shall retain the preference. With no grouping,
  it shall have no visible effect.
- **AC-UI-SIDEBAR-GROUP-INDENT-001.5:** Desktop and phone shall share the same
  preference and disclosure rule. The expanded phone editor shall keep the labelled
  toggle touch-reachable and keyboard-accessible inside its existing scroll body.
- **AC-UI-SIDEBAR-GROUP-INDENT-001.6:** Changing indentation shall preserve task
  membership, sort order, page selection, tree depth, continuation context, and the
  current conversation. It shall not create a different task-ranking query.

## Out of scope

- Arbitrary expressions, numeric color scores, or changes to shared task priority.
- Inferring named priority from custom hexadecimal color appearance.
- Color aggregation from descendants or independent secondary-session ranking.
- Changing task state, activity publication, pins, manual child order, or group headings.
- Changing Kanban, List, Threads, Office, command-panel, or quick-chat ordering.
- Replacing the user's defaults with the example chain.

## Implementation plan

- [Sidebar sort chain and color ranking](../../../plans/sidebar-running-first-activity-sort/plan.md)
