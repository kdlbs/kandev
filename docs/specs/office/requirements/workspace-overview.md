---
status: draft
system: office
created: 2026-09-29
owners:
  - kandev
---

# Office Workspace Overview Requirements

## Overview

An operator with several Office workspaces needs one view of workspace health.
The view shows task counts, pending approvals, agent counts, and recent activity. It does not add a write path or change workspace access.

## Requirements

### REQ-OFFICE-WORKSPACE-OVERVIEW-001: Read-only workspace overview

**Intent:** An operator can review the state of each Office workspace that the operator can access from one page.

**User story:** As an operator, I want to review several Office workspaces on one page, so that I can find work that needs attention.

#### Acceptance criteria

- **AC-OFFICE-WORKSPACE-OVERVIEW-001.1:** The Office navigation shall provide an Overview page on desktop and phone layouts.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.2:** The overview endpoint shall reject agent tokens in every authentication mode. When authentication is enabled, it shall require a real, non-synthetic user identity.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.3:** The endpoint shall return only Office workspaces from the caller's workspace list. It shall not change the existing workspace access rules.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.4:** Each workspace row shall show total, open, in-progress, blocked, and done task counts, pending approvals, total agents, and running agents.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.5:** The overview shall show the latest 20 activity entries across the returned workspaces, in newest-first order.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.6:** A workspace row shall open that workspace. An activity link shall keep the activity workspace in its destination.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.7:** The page shall show loading, error, and empty states. It shall refresh while mounted and keep loaded data when a later refresh fails.

## Out of scope

- Organization membership and organization-wide access rules.
- Cross-workspace task or agent changes.
- Changes to the existing per-workspace dashboard.
