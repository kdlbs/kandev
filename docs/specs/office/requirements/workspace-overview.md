---
status: draft
system: office
created: 2026-09-29
owners:
  - kandev
---

# Office Workspace Overview Requirements

## Overview

An operator with several Office workspaces needs one read of workspace health.
The read returns task counts, pending approvals, agent counts, and recent
activity for every Office workspace the caller can reach. It adds no write path
and does not change workspace access. The integrated control dashboard consumes
it (see `REQ-UI-INTEGRATED-CONTROL-002`).

## Requirements

### REQ-OFFICE-WORKSPACE-OVERVIEW-001: Read-only multi-workspace aggregate

**Intent:** A browser user can read the state of each Office workspace they can access with one request.

**User story:** As an operator, I want one read that covers all my Office workspaces, so that a dashboard can show which ones need attention.

#### Acceptance criteria

- **AC-OFFICE-WORKSPACE-OVERVIEW-001.1:** `GET /api/v1/office/workspaces/aggregate` shall reject agent tokens in every authentication mode. When authentication is enabled, it shall require a real, non-synthetic user identity.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.2:** The endpoint shall return only Office workspaces from the caller's identity-scoped workspace list, sorted by name. It shall not change the existing workspace access rules.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.3:** Each workspace entry shall carry total, open, in-progress, blocked, and done task counts, pending approvals, total agents, and working agents. A workspace with no matching rows reports zero.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.4:** The response shall carry the latest 20 activity entries across the returned workspaces, newest first, with resolved actor and target labels. An activity run link shall keep the activity's workspace in its destination.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.5:** The endpoint shall read existing records only, add no schema, and keep every `IN (...)` query below SQLite's host-parameter limit for any workspace count.
- **AC-OFFICE-WORKSPACE-OVERVIEW-001.6:** Without a wired workspace lister the endpoint shall answer HTTP 503; a failed workspace, repository, or agent read shall answer HTTP 500 rather than a partial aggregate.

## Out of scope

- A standalone Office overview page. The integrated control dashboard is the consumer.
- Organization membership and organization-wide access rules.
- Cross-workspace task or agent changes.
- Changes to the existing per-workspace dashboard.
