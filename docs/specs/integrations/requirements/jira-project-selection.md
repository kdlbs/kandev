---
status: active
system: integrations
created: 2026-10-18
owners:
  - kandev
---

# Jira Project Selection Requirements

## Overview

Users need the configured Jira connection's project inventory to choose a
project filter in My Jira. Integration owns this contract because project
identity and visibility come from the external provider. Saved-view preferences
remain governed by [Jira default views](jira-default-view.md); workflow status
options remain governed by [Jira status filtering](jira-status-filter.md).

## Terminology

- **Project inventory:** Project identities returned by the configured Jira
  connection's project-list operation, before My Jira applies its existing
  user-ticket project filter.
- **Eligible My Jira project:** An inventory entry whose key also occurs in
  the existing assigned-to-or-reported-by-current-user ticket search result.

## Requirements

### REQ-INTEGRATIONS-JIRA-PROJECT-SELECTION-001: Provider project inventory

**Intent:** Make project selection available for supported Jira connections
without changing the meaning of the user's ticket search.

**User story:** As a Jira Server user, I want my eligible projects to appear
in My Jira so I can choose a project filter.

#### Acceptance criteria

- **AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.1:** When a configured Jira
  Server or Data Center connection successfully returns its project inventory,
  Kandev shall expose every entry in that response with its project ID, key,
  and name, in response order, through the existing project-list interface.
- **AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.2:** Jira Cloud connections,
  including configurations without an explicit instance type, shall retain
  their existing project-list behavior and project identity shape.
- **AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.3:** Project listing shall use
  the configured workspace connection, credentials, and Jira base URL,
  including a configured context path, without requiring a credential change.
- **AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.4:** A successful empty inventory
  shall remain an empty successful result. An upstream project-list failure
  shall remain an error with its meaningful upstream status and diagnostic;
  it shall not become a successful empty inventory.
- **AC-INTEGRATIONS-JIRA-PROJECT-SELECTION-001.5:** When project listing and the
  existing user-ticket search both succeed, My Jira shall offer the eligible
  projects through its existing shared project selector on desktop and phone.
  Inventory entries absent from that ticket result shall remain excluded.
  A project-list failure can leave the selector empty while other search
  fields remain usable; it shall not block the whole Jira page.

## Out of scope

- Complete inventory beyond the current Cloud page or transport response limit,
  and changes to user-ticket search pagination or project eligibility.
- Saved views, default-project preferences, initial-load ordering, or freshness.
- New authentication methods, OAuth policy, retries, endpoint fallback, or
  version probing.
- Search, status, transition, watcher, UI composition, copy, or interaction changes.
