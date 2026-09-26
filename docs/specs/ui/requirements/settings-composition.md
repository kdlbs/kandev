---
status: active
system: ui
created: 2026-09-21
owners:
  - Kandev frontend
---

# Settings Composition Requirements

## Overview

Users need consistent settings groups, descriptions, and actions without more settings pages.
UI owns this reusable presentation contract. Domain systems retain behavior, permissions, and persistence.
The accepted direction keeps Task behavior on one page and extends consistent composition across existing first-party settings.
The 2026-09-22 refinement removes collapsed sections and reduces visible text.
Tasks, Conversation, and Runtime header tabs are the proposed same-page organization.

This contract extends [settings typography](settings-typography.md).
It preserves [manual saving](settings-manual-save.md), [discovery](settings-discovery.md), and [header tabs](settings-header-tabs.md).

## Terminology

- **Group:** Related settings with one heading and one outer container.
- **Row:** One setting with its label, explanation, control, and optional status.
- **Info:** Optional technical explanation available from a labelled info button.

## Requirements

### REQ-UI-SETTINGS-COMPOSITION-001: Consistent settings groups

**Intent:** Equivalent content has the same hierarchy across settings pages.

#### Acceptance criteria

- **AC-UI-SETTINGS-COMPOSITION-001.1:** First-party settings pages shall use consistent page headings, group headings, card padding, row spacing, descriptions, and action placement.
- **AC-UI-SETTINGS-COMPOSITION-001.2:** Related simple preferences shall share one group container. A row shall not repeat its label as a separate card title.
- **AC-UI-SETTINGS-COMPOSITION-001.3:** Forms shall group related fields. Resource lists shall use consistent headings and actions while retaining their list, table, or editor interaction.
- **AC-UI-SETTINGS-COMPOSITION-001.4:** The change shall preserve existing page destinations, sidebar entries, and settings-search targets. Task behavior may add the header tabs defined below.
- **AC-UI-SETTINGS-COMPOSITION-001.5:** Equivalent content shall use the existing typography and control-size roles. Technical editors, diagnostics, and tables shall retain documented specialized layouts.

### REQ-UI-SETTINGS-COMPOSITION-002: Task behavior organization

**Intent:** Users can find task preferences by the activity they affect.

#### Acceptance criteria

- **AC-UI-SETTINGS-COMPOSITION-002.1:** Task behavior shall provide Tasks, Conversation, and Runtime tabs. Tasks shall contain Creating and opening tasks, followed by Archiving.
- **AC-UI-SETTINGS-COMPOSITION-002.2:** Creating and opening tasks shall contain automatic opening, generated titles, the profile for agent-created tasks, and automatic agent start on opening.
- **AC-UI-SETTINGS-COMPOSITION-002.3:** The Conversation tab shall contain the unread divider, transcript navigation, and todo checklist. Archiving shall contain archive confirmation.
- **AC-UI-SETTINGS-COMPOSITION-002.4:** Runtime shall contain session capacity, message queue limits and merging, and sleep prevention. All settings sections shall remain expanded within their tab.
- **AC-UI-SETTINGS-COMPOSITION-002.5:** Runtime shall show effective limits beside their controls when these differ from drafts or saved values. Loading and unavailable values shall remain explicit.
- **AC-UI-SETTINGS-COMPOSITION-002.6:** Runtime settings shall identify their instance scope. Sleep prevention shall identify the host computer, including when the user connects remotely.

### REQ-UI-SETTINGS-COMPOSITION-003: Concise descriptions and optional information

**Intent:** Short explanations preserve information needed to choose safely.

#### Acceptance criteria

- **AC-UI-SETTINGS-COMPOSITION-003.1:** Each setting shall show one short, plain-language sentence explaining its effect. Active errors, permission restrictions, and override reasons shall remain visible beside its control.
- **AC-UI-SETTINGS-COMPOSITION-003.2:** Technical explanations shall use a labelled info button beside the setting label. Desktop hover or keyboard focus shall reveal details. Touch shall open a drawer.
- **AC-UI-SETTINGS-COMPOSITION-003.3:** Tab changes and info interactions shall preserve drafts, validation, save contributors, and unsaved-change protection. Neither action shall persist settings.
- **AC-UI-SETTINGS-COMPOSITION-003.4:** A search result or direct fragment shall select its owning tab before focus and highlight. Repeated requests shall work after switching tabs.
- **AC-UI-SETTINGS-COMPOSITION-003.5:** Dirty tabs shall expose an accessible unsaved indicator. Invalid or failed saves in inactive tabs shall provide a visible route to affected controls.
- **AC-UI-SETTINGS-COMPOSITION-003.6:** Effective values shall remain distinct from unsaved changes. Save and Reset shall retain their existing route-level behavior across tabs.

### REQ-UI-SETTINGS-COMPOSITION-004: Accessible responsive composition

**Intent:** Users can operate the same settings on desktop and phone.

#### Acceptance criteria

- **AC-UI-SETTINGS-COMPOSITION-004.1:** Phones shall use the existing settings navigation and one vertical page scroll region. Header tabs shall occupy their own row, as on Data & Logs. Settings groups shall not collapse.
- **AC-UI-SETTINGS-COMPOSITION-004.2:** Phone fields and group actions shall stack when needed. Labels, descriptions, controls, and save actions shall remain inside the viewport.
- **AC-UI-SETTINGS-COMPOSITION-004.3:** Info buttons shall support keyboard and touch activation. Inactive tabs shall not receive focus. Labels and short descriptions shall remain associated with controls.
- **AC-UI-SETTINGS-COMPOSITION-004.4:** Long translations shall wrap without document horizontal scrolling. Shared phone and coarse-pointer actions shall retain targets of at least 44px.
- **AC-UI-SETTINGS-COMPOSITION-004.5:** Layout changes shall preserve settings values, permissions, runtime defaults, and confirmation behavior across reloads and viewport changes.

### REQ-UI-SETTINGS-COMPOSITION-005: Task behavior navigation and readable copy

**Intent:** Users can scan settings without reading implementation details.

#### Acceptance criteria

- **AC-UI-SETTINGS-COMPOSITION-005.1:** Ordinary entry shall select Tasks. A valid tab query shall select that tab. Unknown values shall select Tasks.
- **AC-UI-SETTINGS-COMPOSITION-005.2:** The header shall always expose the Runtime tab. Selecting it shall show runtime controls without an expansion action.
- **AC-UI-SETTINGS-COMPOSITION-005.3:** English row descriptions shall use at most 20 words. Technical identifiers and fallback algorithms shall appear in optional info content.
- **AC-UI-SETTINGS-COMPOSITION-005.4:** Profile choices shall have short labels and at most one short sentence each. Precedence and session-copy rules shall remain available through info.
- **AC-UI-SETTINGS-COMPOSITION-005.5:** Section introductions that repeat row descriptions shall be omitted. Runtime shall identify instance scope once and identify the host in sleep help.
- **AC-UI-SETTINGS-COMPOSITION-005.6:** Info dismissal shall preserve the draft and return touch focus to the opener. Details shall not change the setting or navigate away.
- **AC-UI-SETTINGS-COMPOSITION-005.7:** Existing fragments shall select the correct tab even when the query names another tab. Browser navigation shall retain the existing guard.

## Out of scope

- New settings pages, sidebar categories, or a global settings search redesign. Tabs are limited to Task behavior in this refinement.
- Backend schemas, policy, permissions, defaults, or save transaction changes.
- A global UI theme or changes to unrelated application cards.
- Restyling third-party plugin-owned content or changing the plugin SDK.
- Redesigning specialized workflow canvases, editors, terminals, or diagnostic tables.
- Inverting the automatic-start switch. This package keeps its existing semantics and translated label.

## Implementation plan

See [Settings composition](../../../plans/settings-composition/plan.md). The latest refinement is [Task 06](../../../plans/settings-composition/task-06-concise-help-and-tabs.md).
