---
id: "01-instance-external-tools"
title: "Instance tools through external MCP"
status: complete
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-001
  - REQ-PLUGINS-AGENT-TOOLS-002
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOLS-001.5
  - AC-PLUGINS-AGENT-TOOLS-002.1
  - AC-PLUGINS-AGENT-TOOLS-002.2
  - AC-PLUGINS-AGENT-TOOLS-002.6
system_design:
  - ../../specs/plugins/system-design/agent-tool-contexts.md
---
# Task 01: Instance tools through external MCP

## Summary

Add backward-compatible instance tool declarations and a trusted external
invocation path. Keep task identity checks and existing external authentication.

## In scope

Manifest defaults and validation; snapshot propagation and legacy projection;
external registration and dispatch; declaration and authoring documentation.

## Out of scope

Coordinator bindings, provider package changes, new authentication, and UI.

## Acceptance

- Old declarations remain task-bound. External tools require explicit instance
  scope and surface; configuration clients receive none.
- HTTP and SSE callers can list and call eligible tools with no synthetic task;
  forged context or transport markers fail before RPC.
- Lifecycle revocation, schemas, limits, cancellation, and no retries still apply.

## Verification

Run from the host root after completing the repo-required fresh-worktree setup:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/plugins/... ./internal/mcp/... )
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

Planned regression names and criterion mapping are in [the plan](plan.md#tests).
Add the external cases of `TestProviderUsageMCPContexts` in this order.

## Files likely touched

- `apps/backend/internal/plugins/manifest/manifest.go` and validation/tests
- `apps/backend/internal/plugins/agent_tools.go`
- `apps/backend/internal/mcp/plugintools/types.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/handlers/plugin_tools.go`
- `apps/backend/internal/mcp/server/plugin_contexts_e2e_test.go`
- `docs/public/plugins-manifest.md`, `docs/public/plugins-authoring.md`
- `pluginsdk/plugin.go` context comments, if needed; no protobuf fields

## Dependencies

None. Reconcile public task-only statements when implementing the new behavior.

## Risks

Older executors must never receive unsupported context descriptors as grants.

## Parallelism

sequential

## Inputs

Owning requirements/design, existing external trusted transport marker, and
task-backed plugin authorization tests.

## Results

Implemented on 2026-11-03.

- Manifest declarations accept optional `invocation_scope: task | instance`,
  defaulting omission to task at parse time.
- `coordinator` and `external` are accepted surfaces only with instance scope and
  a true read-only annotation; configuration remains unsupported.
- Runtime descriptors carry the normalized scope, installation ID, and policy
  version; legacy projections strip the new surfaces for older agentctl builds.
- External listing and invocation require the trusted in-process transport
  marker, reject task/identity claims, and dispatch with empty task context.
- Tests: `TestAgentToolInvocationScope`, `TestAgentToolInvocationScopeIsNormalizedOnParse`,
  `TestPluginToolsContextEligibility`, `TestInstancePluginToolTrustedExternalContext`,
  `TestInstancePluginToolRejectsForgedIdentity`, `TestProviderUsageMCPContexts`,
  `TestProviderUsageMCPContextsSSE`.
