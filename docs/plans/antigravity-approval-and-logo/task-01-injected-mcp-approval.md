---
id: "01-injected-mcp-approval"
title: "Recognize injected MCP calls"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-INJECTED-MCP-APPROVAL-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-012
acceptance_criteria:
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.3
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.4
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.5
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.6
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.7
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.9
  - AC-AGENTS-ANTIGRAVITY-ACP-012.1
  - AC-AGENTS-ANTIGRAVITY-ACP-012.3
system_design:
  - ../../specs/agents/system-design/injected-mcp-approval.md
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01 Recognize injected MCP calls

## Summary

Match Antigravity CLI requests for the host-injected Kandev Core tools so the existing policy can select an offered approval. Use the actual ACP request shape and the current tool registration.

## In scope

- Use the recorded Antigravity CLI 1.5.15 notification evidence and Google's MCP/plugin configuration docs to qualify the flattened name and native registration sources. A raw permission-frame capture remains unavailable without an installed, authenticated `agy`; do not infer server origin from matching names.
- Write `TestAntigravityInjectedKandevPermissionACPFlow` through the existing ACP peer fixture. Run it before implementing the provider lookup. The regression starts with a verified injected call that currently emits a pending event.
- Resolve aliases only when a complete native registration inventory has no possible prefix collision. Reuse exact endpoint provenance, the live Core tool catalog, and offered allow option selection.
- Update the injected-approval paragraph in `docs/public/agents-and-profiles.md` with the qualified Antigravity behavior.

## Scope limits

The correction covers Kandev Core on qualified CLI runtimes. Usual permission policies govern unverified aliases, other runtimes, and additional MCP packages. MCP authentication and task reachability still apply.

## Acceptance

- On fresh and resumed CLI ACP calls, select the offered allow-once or allow-always option for an identifiable, exported Core tool on the injected endpoint. Normal tool updates still reach the conversation.
- A shell call, an ambiguous alias, a non-injected server, a tool absent from the live catalog, and a matching global, workspace, or plugin registration outside the Kandev-supplied server list all use their ordinary permission path. Cover them alongside the positive case.
- Fresh and resumed requests use the actual Core registry bound to the process manager, including legacy Core profiles without a package manifest. A missing or incomplete native inventory keeps the call pending.
- Existing approval tests cover provider-native questions, coordinator policy, endpoint provenance, and qualified ACP names.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/process -run '^Test(AntigravityInjectedKandevPermissionACPFlow|AntigravityHumanInputNeverAutoApproves|InjectedKandevPermissionACPFlow|InjectedKandevPermissionOptions|BuiltinPermissionRequiresRegisteredToolAndExactEndpoint|CoordinatorModeIgnoresBlanketApprovalAndGenericInjectedPolicy)$' -count=1)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager_permission_policy.go`
- `apps/backend/internal/agentctl/server/process/builtin_mcp_permission.go` and the ACP permission test fixture
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_permissions.go`, if wire evidence calls for notification correlation
- `apps/backend/internal/agentctl/ACP-PROTOCOL.md`
- `docs/public/agents-and-profiles.md`

## Dependencies

The observed CLI notification shape and official MCP/plugin source docs qualify the typed identity and conservative registration inventory. No raw permission-frame capture is available in this workspace.

## Risks

Antigravity's notification exposes a name with single underscores. The request may omit its name or have a different origin field. Keep approval pending when the origin cannot be verified. Use a sanitized raw fixture to record the successful case.

## Parallelism

`sequential`

## Inputs

- [Injected MCP approval design](../../specs/agents/system-design/injected-mcp-approval.md#antigravity-cli)
- [Antigravity native questions](../../specs/agents/system-design/antigravity-acp-agent.md#native-human-questions)
- [Evidence in the plan](plan.md#evidence)

## Results

Implemented the CLI-only alias resolver with live Core catalog, exact injected
endpoint, configured-server ambiguity, and matched ACP notification identity
checks. The ACP peer fixture covers fresh and resumed requests, including an
omitted permission name, and preserves ordinary flow for shell, third-party,
ambiguous, unknown, mismatched, and no-allow cases.

The process manager scans the global `mcp_config.json`, effective workspace
customization roots through the nearest repository root, and plugin MCP
configs using Antigravity's plugin namespace rule. A global or effective
workspace `plugins.json` discovery manifest keeps inventory incomplete because
it can reference plugin paths outside the scanned roots. Configs and plugin
JSON manifests must be regular, non-symlink files no larger than 1 MiB. ACP
peer fixtures create native workspace and plugin registrations outside the
Kandev-supplied server list, then use production `NewManager` inventory
loading. Both collisions remain pending. Positive fresh and resumed calls
bind `BuiltinCatalog` from a real MCP server instance; legacy Core profiles
verify the actual registered tool names.

The retained session log has typed `mcp_kandev_<tool>` notifications with
session and tool-call IDs, but it has no `session/request_permission` frame.
The `agy` executable is not installed in this workspace, so a raw permission
capture was not possible. The ACP peer fixture covers typed request identity,
same-session/call notification correlation, contradictory identity, and the
ordinary prompt path when identity or inventory is unproven.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp ./internal/agent/antigravityruntime -run '^(TestAntigravityInjectedKandevPermissionACPFlow|TestAntigravityHumanInputNeverAutoApproves|TestInjectedKandevPermissionACPFlow|TestInjectedKandevPermissionOptions|TestBuiltinPermissionRequiresRegisteredToolAndExactEndpoint|TestCoordinatorModeIgnoresBlanketApprovalAndGenericInjectedPolicy|TestAntigravityCallIdentityPublishesWithActiveCall|TestAntigravityCallIdentityConflictSurvivesTypedName|TestAntigravityPermissionIdentityWaitUsesSessionAndCallBoundary|TestScanNativeMCPServerNames.*)$' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Review follow-up validation also passed: the full affected backend package suite
under `go test -race -trimpath -tags fts5`, and
`bash scripts/lint-go-changed` across all changed Go files.
