---
status: active
system: ui
created: 2026-07-19
updated: 2026-10-11
owners:
  - kandev
---
# Task Layout Profiles Requirements

## Overview

Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, who prefer a different Files and Changes arrangement, or who want Pull Request details in a chosen pane need one durable layout surface that does not disturb layouts already customized for individual tasks. Panel placement belongs to the layout itself instead of a separate global appearance preference.

REQ-UI-TASK-LAYOUT-PROFILES-001 remains the active layout-profile contract.
REQ-UI-TASK-LAYOUT-PROFILES-002 is a draft extension for registered plugin task
panels; its acceptance is separate from delivery status in the new work orders.
UI owns the reusable presentation preference; the plugin system continues to
own registration, availability, and runtime lifecycle.

## Requirements

### REQ-UI-TASK-LAYOUT-PROFILES-001: Task Layout Profiles

**Intent:** Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, who prefer a different Files and Changes arrangement, or who want Pull Request details in a chosen pane need one durable layout surface that does not disturb layouts already customized for individual tasks. Panel placement belongs to the layout itself instead of a separate global appearance preference.

#### Acceptance criteria

- **AC-UI-TASK-LAYOUT-PROFILES-001.1:** `Settings > Preferences > Layouts` is the central manager for reusable desktop task-layout profiles and is reachable on desktop and mobile settings navigation.
- **AC-UI-TASK-LAYOUT-PROFILES-001.2:** The page lists the built-in Default, Plan Mode, Preview Mode, and VS Code layouts as stable rows. A user edits a built-in directly; Kandev stores a hidden override while keeping the built-in row selected and marks it `Customized`. Reset removes the override and restores the code-defined layout.
- **AC-UI-TASK-LAYOUT-PROFILES-001.3:** A user can create, rename, duplicate, edit, delete, and select the default custom profile. Names must be non-empty; profile IDs must be unique.
- **AC-UI-TASK-LAYOUT-PROFILES-001.4:** Exactly one layout is effective as the user default. A saved profile, including a reserved built-in override, marked `is_default` wins; when none is marked, the built-in Default layout is effective.
- **AC-UI-TASK-LAYOUT-PROFILES-001.5:** The visual editor supports one instance of each existing built-in reusable panel. Agent is required and cannot be removed.
- **AC-UI-TASK-LAYOUT-PROFILES-001.6:** PR Details is the canonical reusable `pr-detail` panel. Its position in the selected layout is a placement template, not an instruction to keep an empty runtime tab open. The tab is visible only while the active task has a linked GitHub pull request or GitLab merge request, and then renders that review through the existing provider-aware review surface.
- **AC-UI-TASK-LAYOUT-PROFILES-001.7:** The code-defined Default and compact desktop layouts omit PR Details. Agent remains initially selected in the Default center group, and Files remains initially selected in the top-right Files and Changes group.
- **AC-UI-TASK-LAYOUT-PROFILES-001.8:** When the active task gains a linked GitHub pull request or GitLab merge request, Kandev adds the canonical panel as an inactive tab in the group and tab index configured by the selected custom Default. If that layout does not configure PR Details, Kandev adds it beside the live Agent panel. The user's current tab remains selected.
- **AC-UI-TASK-LAYOUT-PROFILES-001.9:** When a saved profile is the effective desktop default, Kandev shall preserve its split proportions in fresh task environments and Reset Layout.
- **AC-UI-TASK-LAYOUT-PROFILES-001.10:** When the workbench size differs from the saved profile, Kandev shall scale its proportions and apply the desktop safety caps.
- **AC-UI-TASK-LAYOUT-PROFILES-001.11:** When a meaningful Git or commit update arrives, Kandev activates Changes only in the desktop Default layout's top-right group. That group must contain exactly the Files and Changes tabs. All other layouts and group contents keep the selected tab active.

### REQ-UI-TASK-LAYOUT-PROFILES-002: Plugin Panels in Reusable Profiles

**Contract status:** Draft extension; implemented and verified under
[the plugin layout-profile plan](../../../plans/plugin-task-layout-profiles/plan.md),
with contract acceptance still pending. The active baseline
REQ-UI-TASK-LAYOUT-PROFILES-001 is unchanged by this extension.

**Intent:** Users can include installed plugin task panels in the same reusable
profiles they use to arrange the desktop task workbench, without configuring a
task first or losing saved placement when plugin availability changes.

#### Acceptance criteria

- **AC-UI-TASK-LAYOUT-PROFILES-002.1:** At `/settings/preferences/layouts`,
  Add panel shall offer every currently registered plugin task panel, including
  Prompt History when registered, using its existing localized display title.
  Options shall react to registration, disable, enable, and reload changes
  without requiring a page refresh.
- **AC-UI-TASK-LAYOUT-PROFILES-002.2:** Add panel shall omit identities already
  present anywhere in the edited layout. It shall remain enabled when the only
  missing choices are plugin panels and retain its existing unavailable state
  when there are no missing choices or the editor is not editable.
- **AC-UI-TASK-LAYOUT-PROFILES-002.3:** Selecting a plugin option shall add exactly
  one panel in the selected split, using the existing placement fallback when
  no split is selected. Adding, moving, splitting, reordering, or removing that
  panel shall participate in the existing dirty, save, discard, and navigation
  behavior. Built-in actions and the required single Agent shall remain intact.
- **AC-UI-TASK-LAYOUT-PROFILES-002.4:** A reusable profile shall accept a
  structurally correct generic plugin panel and reject malformed or mismatched
  identity, component, tab component, or identity parameters. Duplicate
  identities across splits, missing or duplicate Agent, empty groups, and
  invalid active tabs shall remain invalid.
- **AC-UI-TASK-LAYOUT-PROFILES-002.5:** Profile validity and settings editing
  shall not depend on whether a referenced plugin is loading, reloading,
  failed, disabled, or uninstalled. Changes in that availability shall not
  remove or rewrite the user's reusable saved placement. Live task-panel
  revocation and reload behavior shall retain the existing plugin lifecycle.
- **AC-UI-TASK-LAYOUT-PROFILES-002.6:** Both a built-in override and a custom
  profile containing a plugin panel shall survive save and reload, preserving
  generic identity, identity parameters, canonical stored title, selected tabs,
  tab order, outer split orientation, nested split structure, and proportions.
  Applying either profile to a desktop task where the plugin is available
  shall render the actual plugin in that placement. Existing task-specific
  layouts shall keep their precedence until explicit profile selection or
  Reset Layout.
- **AC-UI-TASK-LAYOUT-PROFILES-002.7:** Pointer, keyboard, and touch users shall
  be able to choose and save plugin panels through the existing Layouts settings
  surface. A touch tap opening Add panel shall not also select an option.
  Phone settings shall retain reachable controls and no document-level
  horizontal scrolling; editing the profile shall not change mobile or tablet
  task-detail composition.
- **AC-UI-TASK-LAYOUT-PROFILES-002.8:** Settings previews shall render generic
  placeholders for plugin panels even when the plugin is unavailable, never
  mount plugin task/session content, and retain the saved title as a display
  fallback when a registration cannot provide a localized title.

## Out of scope

- Plugin-specific allowlists, changes to plugin packages or registration APIs,
  and restoring the retired built-in Prompt History panel.
- A new persistence store, layout schema migration, or automatic replacement
  of existing task-specific layouts.
- Mobile or tablet task-detail layout customization and production deployment.

## Implementation plans

- [Plugin task layout profiles](../../../plans/plugin-task-layout-profiles/plan.md)
  delivers the draft extension.
- [Original layout profiles](../../../plans/task-layout-profiles/plan.md) and
  [PR Details layout panel](../../../plans/pr-details-layout-panel/plan.md)
  retain their completed historical delivery records.

## System design

See the [baseline layout-profile design](../system-design/task-layout-profiles.md)
and the [plugin panel extension](../system-design/task-layout-plugin-panels.md).
