---
created: 2026-11-06
status: completed
requirements:
  - REQ-PLUGINS-AGENT-TOOL-WORKSPACE-001
system_design:
  - ../../specs/plugins/system-design/agent-tool-workspace-scope.md
legacy_specs: []
---
# Implementation plan: Workspace-scoped external plugin reads

## Overview

Let an external MCP client call a plugin read for one workspace. Kandev checks
the caller's existing workspace permissions and passes only the verified
workspace ID to the plugin. Source request:
[discussion #5102](https://github.com/kdlbs/kandev/discussions/5102), a follow-up
to PR #5038.

Contract: [workspace-scoped plugin tool requirements](../../specs/plugins/requirements/agent-tool-workspace-scope.md).
Design: [workspace-scoped plugin tools](../../specs/plugins/system-design/agent-tool-workspace-scope.md).

## Assumptions and scope

Planned against `main` at `08a36b3fe0d`. These choices were made during
planning, without a separate interview, and can be revised before
implementation:

- The scope value is `workspace`. The host-owned argument is
  `kandev_workspace_id`, so it cannot collide with a plugin input.
- Workspace scope is allowed on `external`, `kanban-task`, and `office-task`.
  Automation, coordinator, and managed-conversation surfaces are excluded.
- Reads only. The external catalog is not filtered by membership.

No protobuf, SDK type, database, UI, or runtime-flag change is needed. The
change adds a new declaration value that existing plugins never send, so
it needs no release toggle.

## Work orders

| ID | Title | Wave | Depends on |
| --- | --- | --- | --- |
| [01](task-01-workspace-scope-declaration.md) | Workspace scope declaration and dispatch gate | 1 | none |
| [02](task-02-external-workspace-authorization.md) | External workspace argument and authorization | 2 | 01 |

The order is sequential. Work order 02 needs the descriptor scope and dispatch
gate from 01.

## Tests

| Criterion | Test (planned name) | Work order |
| --- | --- | --- |
| 001.1, 001.2 | `TestAgentToolWorkspaceScopeValidation` (manifest) | 01 |
| 001.8 | `TestWorkspaceToolTaskSurfaceUsesExecutionWorkspace` (handlers) | 01 |
| 001.9 | `TestExternalDispatchGateScopeWorkspaceConsistency` (plugins) | 01 |
| 001.3 | `TestExternalCatalogInjectsWorkspaceArgument` (handlers) | 02 |
| 001.4, 001.5 | `TestExternalWorkspaceToolAuthorization` (handlers, table) | 02 |
| 001.6, 001.7 | `TestExternalWorkspaceToolCallerIdentity` (server, auth enabled and disabled, HTTP and SSE, revoked token) | 02 |

## Verification strategy

Unit tests cover manifest validation, the dispatch gate, and the handler
decisions with fake authorizers. The transport test in
`internal/mcp/server/plugin_contexts_e2e_test.go` covers the real MCP
initialize/list/call flow with real authentication. It confirms that the PAT
identity reaches the handler on both transports. That identity path is the
plan's main unverified assumption. No browser E2E: there is no UI change.

## Risks

- **Identity propagation (high impact):** if a transport loses the request
  context's `authn.Identity`, authorization would see no scoped caller and skip
  the check. Work order 02 must prove propagation on streamable HTTP and SSE,
  and fail closed when auth is enabled but no identity is present.
- **Existence leak:** not-found and forbidden must share one error.
- **Schema injection:** plugin schemas with top-level combinators are rejected
  by AC-001.2. The injected copy must not change the plugin catalog's
  `InputSchema`, which still validates plugin arguments.
- **Release floor:** the public manifest docs must name the first supporting
  host release once known.

## Exclusions

Automation/coordinator/managed workspace scope, workspace-scoped writes,
membership-filtered catalogs, plugin-visible user identity, and UI.

## Results

Implemented and verified. Plugin manifests accept `workspace` scope with
surface, read-only, schema, reserved-argument, and compatibility-floor
validation. External catalogs advertise the host-owned
`kandev_workspace_id` argument; each call resolves and authorizes the
workspace, strips the argument, and dispatches only the verified ID.
`ProjectLegacySafe` projects surviving workspace task descriptors to task
scope so retained pre-workspace agentctl builds keep the catalog. The
manifest contract change carries an additive plugin SDK `0.2.0` record.
Transport tests confirm caller identity propagation over streamable HTTP and
SSE, revoked-token rejection, failed-closed identity handling, and the
authentication-disabled path. Public manifest and authoring docs describe the
scope and the first supporting host release, `0.100.0`.
