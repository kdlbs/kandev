---
status: active
system: integrations
created: 2026-11-19
owners:
  - kandev
---

# Linear Workspace Operations Requirements

## Overview

This requirement covers workspace selection for legacy integration operations
and authorization for Linear issue state changes. The integration system owns
provider operations and workspace-owned credentials. The auth system owns the
meaning of workspace scopes.

## Requirements

### REQ-INTEGRATIONS-WORKSPACE-RESOLUTION-001: Resolve the integration workspace

**Intent:** Use the workspace that owns the saved integration connection.

#### Acceptance criteria

- **AC-INTEGRATIONS-WORKSPACE-RESOLUTION-001.1:** When user settings contain a
  valid workspace ID, the resolver shall select it before the oldest-workspace
  fallback. If no valid workspace ID exists, it shall select the oldest
  workspace. If no workspace exists, it shall return `default`.

### REQ-INTEGRATIONS-LINEAR-WRITE-SCOPE-001: Authorize Linear issue state changes

**Intent:** Prevent a caller with read access from using saved Linear
credentials to change an issue.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.1:** Before a user-facing Linear
  issue state change, Kandev shall require `task.write` on the resolved
  workspace. A caller with only `workspace.read` shall receive a forbidden
  result, and Kandev shall not send a state mutation to Linear. A caller with
  `task.write` shall be able to change the issue state.
- **AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.2:** The HTTP and legacy WebSocket
  state-change routes shall apply the same write check. A legacy request with no
  workspace ID shall resolve the default workspace before the check. Plugin
  broker writeback shall keep its existing authorization path.

## Out of scope

- Linear issue reads and search.
- Write authorization for other provider integrations.
- Changes to the plugin broker authorization policy.

## Implementation plans

- [Linear workspace operations](../../../plans/linear-workspace-operations/plan.md)
