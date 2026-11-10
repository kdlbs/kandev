---
id: "02-external-workspace-authorization"
title: "External workspace argument and authorization"
status: done
wave: 2
depends_on:
  - "01-workspace-scope-declaration"
plan: plan.md
requirements:
  - REQ-PLUGINS-AGENT-TOOL-WORKSPACE-001
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.3
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.4
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.5
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.6
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.7
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.9
system_design:
  - ../../specs/plugins/system-design/agent-tool-workspace-scope.md
---
# Task 02: External workspace argument and authorization

## Summary

Advertise the host-owned `kandev_workspace_id` argument on external workspace
tools. On each call, strip it, check that the workspace exists, authorize the
caller, and dispatch with the verified workspace ID. Update the public docs.

## In scope

- `handleListExternalPluginTools`: return a schema copy with the required
  `kandev_workspace_id` string for workspace definitions.
- `handleInvokeExternalPluginTool`: keep the existing guards, then validate
  and strip the argument, call `taskSvc.GetWorkspace` and
  `taskSvc.AuthorizeWorkspaceAccess`, map not-found and denied to one error,
  and dispatch with `WorkspaceID`.
- Fail closed when authentication is enabled but the context has no identity.
- Denial reasons `workspace_argument_invalid` and `workspace_unavailable`.
- Docs: `docs/public/plugins-manifest.md`, `docs/public/plugins-authoring.md`,
  and the `AgentToolContext` comment in `pluginsdk/types.go`, which currently
  says external calls never carry a workspace.

## Out of scope

Manifest validation (01), catalog membership filtering, and UI.

## Acceptance

- Over real streamable HTTP and SSE with authentication enabled, a PAT
  holder's call to a readable workspace reaches the plugin with that ID and
  stripped arguments. Unreadable, unknown, missing, and blank IDs fail with no
  RPC, and unknown and unreadable return the same error.
- After the token is revoked or the membership removed, the next call fails.
  With authentication disabled, any existing workspace passes.
- The public manifest and authoring docs describe the scope, the injected
  argument, and the minimum host version.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/mcp/... ./internal/plugins/... ./internal/backendapp)
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/plugin_tools.go` and `plugin_tools_contexts_test.go`
- `apps/backend/internal/mcp/server/plugin_contexts_e2e_test.go`
- `apps/backend/internal/backendapp/helpers.go` (only if test wiring needs it)
- `docs/public/plugins-manifest.md`, `docs/public/plugins-authoring.md`
- `pluginsdk/types.go` (comment only)

## Dependencies

Work order 01 (descriptor scope, dispatch gate).

## Risks

Identity propagation through mcp-go's SSE path is the main unverified
assumption. If it fails, stop and record a design change instead of falling
back to an unscoped check.

## Parallelism

sequential

## Results

Implemented. The external catalog returns a copy of each workspace tool with
a required `kandev_workspace_id` string (`minLength: 1`) and keeps the plugin
catalog schema unchanged. Root `minProperties` and `maxProperties` bounds are
widened for the injected argument so plugin-defined count constraints still
accept valid external calls. External invocations validate and strip the
argument, resolve the workspace, and require current caller read permission
through `GetWorkspace` plus `AuthorizeWorkspaceAccess` on every call.
Not-found and denied share `workspace is not available`; missing, blank, and
non-string values fail with `workspace_argument_invalid`; unavailable
workspaces log `workspace_unavailable`. With authentication enabled but no
real identity, the call fails closed; with authentication disabled any
existing workspace passes. Public docs and the SDK context comment describe
the scope, injected argument, and 0.100.0 floor.

Verified with `go test -trimpath ./internal/mcp/... ./internal/plugins/...
./internal/backendapp`, `node scripts/validate-public-docs.mjs`,
`python3 scripts/lint-spec-files.py --all`, and
`python3 scripts/list-docs.py validate`. Tests:
`TestExternalCatalogInjectsWorkspaceArgument`,
`TestExternalWorkspaceToolAuthorization`,
`TestExternalWorkspaceToolCallerIdentity` (streamable HTTP and SSE, revoked
token, disabled auth), and `TestWorkspaceAgentToolReceivesVerifiedContext`.
Real PAT authentication over both transports is covered by
`TestExternalWorkspaceToolPATIdentityHTTPAndSSE`. The additive manifest
contract bump is recorded as plugin SDK `0.2.0`.
