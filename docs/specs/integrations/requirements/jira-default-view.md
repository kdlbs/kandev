---
status: active
system: integrations
created: 2026-09-24
owners:
  - kandev
---

# Jira Default View Requirements

## Overview

People who repeatedly use one Jira saved view must select it again on every visit to `/jira`. The integration system owns this behavior because the choice restores Jira-specific filters and JQL. The preference belongs to the user, while the Jira connection and its default project key belong to the workspace.

## Terminology

- **Default view:** The one saved Jira view a user explicitly chooses for future visits to `/jira`.
- **Available view:** A built-in view or a valid custom view still present in the user's saved views.

## Requirements

### REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001: Personal Jira landing view

**Intent:** Let a user return to their preferred Jira ticket search without selecting it on every visit.

**User story:** As a developer, I want `/jira` to open my chosen view so I can work from the same filtered ticket list each day.

#### Acceptance criteria

- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.1:** A user can mark one available built-in or custom Jira view as their default, replace that choice, or clear it. The choice persists for that user across visits and does not change another user's choice.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.2:** When `/jira` opens with an available default view, the page selects it and applies its saved filters or exact custom JQL before searching for the displayed ticket list.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.3:** When no default is set, `/jira` opens on **Assigned to me** and retains the existing workspace default-project-key behavior.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.4:** When the chosen default is missing, invalid, or deleted, `/jira` falls back to **Assigned to me** without failing. Deleting the default clears the choice; deleting the active default also updates the current page to the fallback view.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.5:** Changing the default affects future visits. It does not replace the view currently on screen. A user selection made while settings load also remains active after loading finishes.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.6:** The saved-view picker identifies the default and offers accessible set and clear actions on desktop and phone. The action does not select or delete its view, and phone actions remain reachable by touch.
- **AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.7:** If saving the default fails, the prior default remains selected for future visits and the page shows an error that permits retry.

## Out of scope

- Remembering the last selected view without an explicit default.
- Encoding the active view in the URL or changing bookmark and sharing behavior.
- Applying this preference to Linear, GitHub, or other providers.
- Changing Jira's workspace default project key or the saved-view filter model.
