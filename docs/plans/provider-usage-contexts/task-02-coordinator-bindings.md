---
id: "02-coordinator-bindings"
title: "Bound coordinator plugin reads"
status: complete
wave: 2
depends_on:
  - 01-instance-external-tools
plan: plan.md
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-002
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOLS-002.1
  - AC-PLUGINS-AGENT-TOOLS-002.4
  - AC-PLUGINS-AGENT-TOOLS-002.6
system_design:
  - ../../specs/plugins/system-design/agent-tool-contexts.md
---
# Task 02: Bound coordinator plugin reads

## Summary

Bind exact eligible instance-read tools to phase-2 coordinator conversations.
Recheck those references for discovery and each invocation.

## In scope

Version-2 plugin references with installation/policy version; legacy parse;
conversation creation/reopen; server filters; plugin-specific action guard.

## Out of scope

Changing core action permissions, automatic proposals, UI controls, and
granting plugins to phase-1 coordinators.

## Acceptance

- A new phase-2 binding lists only active, configured tools with explicit
  coordinator instance scope; version-1 and absent bindings grant no plugins.
- Discovery and raw action calls require the current coordinator conversation,
  workspace, exact plugin reference, installation, and policy version.
- Plugin revocation or policy changes fail before RPC; core tools keep their
  existing policy and cross-workspace checks.

## Verification

Run from the host root after Task 01 setup:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/coordinator/... ./internal/mcp/... ./internal/plugins/...)
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

Implement the coordinator cases of `TestProviderUsageMCPContexts`.
Use the exact regression names mapped in [the plan](plan.md#tests).

## Files likely touched

- `apps/backend/internal/mcp/profile/coordinator_policy.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/handlers/coordinator_authorization.go`
- `apps/backend/internal/mcp/handlers/plugin_tools.go`
- Coordinator conversation binding builder, found via `CoordinatorToolPolicy`
- `docs/specs/coordinator/system-design/copilot-tools.md`
- Paired coordinator requirement if the delivered policy changes its criteria

## Dependencies

Task 01's declaration and descriptor contract.

## Risks

Do not interpret a missing field in a retained binding as approval.
Reopen is required to grant newly eligible tools.

## Parallelism

sequential

## Inputs

Owning design, coordinator principal resolution, phase-2 binding creation,
and the existing raw-action authorization test matrix.

## Results

Implemented on 2026-11-03.

- Coordinator bindings are version 2 and carry exact plugin references with
  installation and policy version, separate from core `ToolNames`.
- Version-1 bindings still parse and expose no plugin reads; malformed and
  absent bindings expose none.
- Conversation creation stamps the eligible active, configured instance-read
  catalog; catalog read failure opens the conversation with no plugin reads.
- Discovery intersects the live catalog with the binding; invocation
  revalidates the live installation, policy version, conversation, workspace,
  and coordinator, and rejects a forged non-coordinator surface.
- Tests: `TestCoordinatorPluginBindingCompatibility`,
  `TestStampToolPolicyBindsExactPluginReferences`,
  `TestStampToolPolicyCatalogFailureGrantsNoPlugins`,
  `TestPluginToolsCoordinatorRegistrationUsesExactBinding`,
  `TestPluginToolsCoordinatorLegacyBindingRegistersNothing`,
  `TestCoordinatorPluginToolAuthorization`,
  `TestCoordinatorPluginToolLegacyBindingGrantsNothing`,
  `TestCoordinatorPluginActionPolicyGuard`,
  `TestInstancePluginToolRevokedAfterDiscovery`.
