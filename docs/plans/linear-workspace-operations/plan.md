---
created: 2026-11-19
status: completed
requirements:
  - REQ-INTEGRATIONS-WORKSPACE-RESOLUTION-001
  - REQ-INTEGRATIONS-LINEAR-WRITE-SCOPE-001
system_design:
  - ../../specs/integrations/system-design/linear-workspace-operations.md
legacy_specs: []
---

# Implementation Plan: Linear Workspace Operations

## Overview

This package records the fix for PR #5341. It keeps integration calls on the
workspace that owns their credentials and requires `task.write` before a user
changes a Linear issue state.

Requirements: [workspace resolution](../../specs/integrations/requirements/linear-workspace-operations.md#req-integrations-workspace-resolution-001-resolve-the-integration-workspace)
and [Linear write scope](../../specs/integrations/requirements/linear-workspace-operations.md#req-integrations-linear-write-scope-001-authorize-linear-issue-state-changes).
Design: [Linear workspace operations](../../specs/integrations/system-design/linear-workspace-operations.md).

## Scope

### In scope

- Follow PostgreSQL `search_path` visibility when the shared resolver checks
  for workspace tables.
- Test that the active workspace wins over an older fallback workspace.
- Apply `task.write` before user-facing Linear issue state changes over HTTP and
  the legacy WebSocket route.
- Preserve the plugin broker writeback path.
- Explain the Linear write permission in the public integration guide.

### Out of scope

- Write authorization changes for other integrations.
- Changes to plugin broker authorization.
- Frontend changes.

## Technical approach

`workspacescope.tableExists` uses PostgreSQL catalog visibility so its table
check matches later unqualified queries. `DefaultResolver` retains its active
workspace, oldest workspace, and `default` precedence.

The Linear service resolves the workspace before it calls the user write
authorizer. Backend wiring checks `task.write` through the task service. The
existing `clientFor` path continues to enforce workspace read access. The
plugin broker continues to call the existing writeback method.

## Tests

| Criterion | Evidence |
| --- | --- |
| `AC-INTEGRATIONS-WORKSPACE-RESOLUTION-001.1` | `TestDefaultResolverPostgres` uses an empty leading schema, an older workspace, and a distinct active workspace. |
| `AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.1` | `TestHTTPIssueDetailAndStateUseRequestedWorkspace` denies a viewer before the client call and allows a writer. |
| `AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.2` | `TestWSIssueStateWriteRequiresWorkspaceWriteScope` verifies default resolution and denial before the client call. |

## Work orders

- [x] [Task 01: Linear workspace operations](task-01-linear-workspace-operations.md)

## Verification results

- Focused Go tests passed for `internal/linear`,
  `internal/integrations/workspacescope`, and `internal/backendapp`.
- The changed-package Go lint and repository commit hooks passed.
- All 63 public docs validation tests passed, and the public docs validator
  accepted all 55 published pages.
- The docs index and full specification lint passed. The local PR documentation
  coverage preflight returned `covered` with no errors.

## Risks

- The PostgreSQL integration test requires `KANDEV_TEST_POSTGRES_DSN` for local
  execution. The backend CI matrix provides the remote PostgreSQL coverage.
