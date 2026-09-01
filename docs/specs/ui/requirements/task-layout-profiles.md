---
status: active
system: ui
created: 2026-07-19
owners:
  - kandev
---
# Task Layout Profiles Requirements

## Overview

Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, who prefer a different Files and Changes arrangement, or who want Pull Request details in a chosen pane need one durable layout surface that does not disturb layouts already customized for individual tasks. Panel placement belongs to the layout itself instead of a separate global appearance preference.

## Requirements

### REQ-UI-TASK-LAYOUT-PROFILES-001: Task Layout Profiles

**Intent:** Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, who prefer a different Files and Changes arrangement, or who want Pull Request details in a chosen pane need one durable layout surface that does not disturb layouts already customized for individual tasks. Panel placement belongs to the layout itself instead of a separate global appearance preference.

#### Acceptance criteria

- **AC-UI-TASK-LAYOUT-PROFILES-001.1:** `Settings > General > Layouts` is the central manager for reusable desktop task-layout profiles and is reachable on desktop and mobile settings navigation.
- **AC-UI-TASK-LAYOUT-PROFILES-001.2:** The page lists the built-in Default, Plan Mode, Preview Mode, and VS Code layouts as stable rows. A user edits a built-in directly; Kandev stores a hidden override while keeping the built-in row selected and marks it `Customized`. Reset removes the override and restores the code-defined layout.
- **AC-UI-TASK-LAYOUT-PROFILES-001.3:** A user can create, rename, duplicate, edit, delete, and select the default custom profile. Names must be non-empty; profile IDs must be unique.
- **AC-UI-TASK-LAYOUT-PROFILES-001.4:** Exactly one layout is effective as the user default. A saved profile, including a reserved built-in override, marked `is_default` wins; when none is marked, the built-in Default layout is effective.
- **AC-UI-TASK-LAYOUT-PROFILES-001.5:** The visual editor supports one instance of each reusable panel: Agent, Files, Changes, PR Details, Terminal, Plan, Browser, and VS Code. Agent is required and cannot be removed.
- **AC-UI-TASK-LAYOUT-PROFILES-001.6:** PR Details is the canonical reusable `pr-detail` panel. Its position in the selected layout is a placement template, not an instruction to keep an empty runtime tab open. The tab is visible only while the active task has a linked GitHub pull request or GitLab merge request, and then renders that review through the existing provider-aware review surface.
- **AC-UI-TASK-LAYOUT-PROFILES-001.7:** The code-defined Default and compact desktop layouts omit PR Details. Agent remains initially selected in the Default center group, and Files remains initially selected in the top-right Files and Changes group.
- **AC-UI-TASK-LAYOUT-PROFILES-001.8:** When the active task gains a linked GitHub pull request or GitLab merge request, Kandev adds the canonical panel as an inactive tab in the group and tab index configured by the selected custom Default. If that layout does not configure PR Details, Kandev adds it beside the live Agent panel. The user's current tab remains selected.
- **AC-UI-TASK-LAYOUT-PROFILES-001.9:** When a saved profile is the effective desktop default, Kandev shall preserve its split proportions in fresh task environments and Reset Layout.
- **AC-UI-TASK-LAYOUT-PROFILES-001.10:** When the workbench size differs from the saved profile, Kandev shall scale its proportions and apply the desktop safety caps.
- **AC-UI-TASK-LAYOUT-PROFILES-001.11:** When a meaningful Git or commit update arrives, Kandev activates Changes only in the desktop Default layout's top-right group. That group must contain exactly the Files and Changes tabs. All other layouts and group contents keep the selected tab active.

## System design

The migrated technical source is split into [part 1](../system-design/task-layout-profiles.md).
