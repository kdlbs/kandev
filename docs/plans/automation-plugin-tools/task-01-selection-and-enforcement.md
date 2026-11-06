---
id: "01-selection-and-enforcement"
title: "Persist and enforce selected plugin tools"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.1
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.2
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.3
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.4
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.5
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.6
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.7
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.1
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.2
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.3
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.4
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.5
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.6
system_design:
  - ../../specs/office/system-design/automation-plugin-tools.md
---

# Task 01: Persist and enforce selected plugin tools

## Summary

Deliver the backend selection, native save boundary, policy invalidation, filtered discovery, and per-call enforcement as one slice.
Use real service/handler integration and the existing packaged plugin fixture for read/write evidence.

## In scope

- The shared selection type, one automation JSON column, migrations, and update presence handling.
- Empty defaults, export omission, mode handling, and rejection of malformed stored authority.
- `AgentToolPolicyVersion` initialization and invalidation before package/configuration changes.
- The native workspace-scoped `automation.plugin_tool_options` action and validated create/update selection handling.
- Internal native configuration intent that MCP and plugin commands cannot supply.
- `automation` declarations in manifest and MCP snapshot validation.
- Trusted principal filtering, exact invocation checks, structured denial reasons, and existing gRPC limits.
- Global-push invalidation, serialized filtered refresh, same-catalog-revision selection changes, and older catalog compatibility.
- Fixture tools for an approved read and bounded report write.
- Reconcile current authority documentation with the implemented exception.

## Out of scope

- Editor markup, localization, or browser automation.
- New scheduler, task mode, SDK/protobuf, or managed-conversation policy behavior.
- Third-party plugin patches or releases.

## Acceptance

- Native saves persist exact selections and reject stale/foreign/non-native approval attempts.
  Migration, partial updates, export, and mode changes retain the documented empty-default behavior.
- Real list/invoke paths enforce current approval through lifecycle, removal, restart, retry, and raw-action cases.
  Global pushes and forged surfaces cannot widen access.
- Policy invalidation precedes package/configuration effects, including secret writes.
  The existing core catalog and ordinary plugin consumers pass their regression checks.

## Verification

Run from the repository root, after the initial failing tests and implementation:

```bash
(cd apps/backend && go test ./internal/automation ./internal/plugins ./internal/plugins/manifest ./internal/mcp/plugintools ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp ./cmd/plugin-fixture -run 'TestAutomationPluginTools|TestAgentToolPolicyVersion|TestValidate.*AgentTool|TestAgentToolCatalog|TestSetPluginTools|TestInvokeAgentTool|TestInvokeManagedAgentTool|TestManagedAgentTool|TestManagedConversationProfile|TestServerSurfaceAutomationHasFixedCoordinatorCatalog|TestAuthorizeAutomationRequest' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For changed Go files, run the repository's changed-file Go lint helper with their exact paths.
Add `TestAutomationPluginToolsPostgres` beside `TestPostgresStoreSchemaReplay` using `testutil.OpenIsolatedPostgres`.
It covers fresh schema, replayed migration, selection round trip, and omitted/empty updates.
With the existing isolated PostgreSQL test service configured, run:

```bash
test -n "${KANDEV_TEST_POSTGRES_DSN:-}"
(cd apps/backend && go test ./internal/automation -run '^TestAutomationPluginToolsPostgres$' -count=1 -v)
```

Record actual PostgreSQL execution rather than a skipped test before marking this work order done.
No implementation result is claimed by this design package.

## Files likely touched

- `apps/backend/internal/automation/models.go`, `store.go`, `service.go`, `handlers.go`, `provider.go`, and export tests.
- New `apps/backend/internal/automation/plugin_tools.go` and `plugin_tools_test.go`.
- `apps/backend/internal/plugins/store/store.go`, `registry.go`, `service_install.go`, `service_config.go`, and `service_sync.go`.
- `apps/backend/internal/plugins/agent_tools.go` and new `agent_tools_automation_test.go`.
- `apps/backend/internal/plugins/manifest/manifest.go`, `validate.go`, and manifest tests.
- `apps/backend/internal/mcp/plugintools/types.go` and tests.
- `apps/backend/internal/mcp/handlers/automation_authorization.go`, `plugin_tools.go`, and new `plugin_tools_automation_test.go`.
- `apps/backend/internal/mcp/server/server.go`, `server_automation_mode_test.go`, and catalog tests.
- `apps/backend/internal/backendapp/plugins.go`, `plugin_tool_refresher.go`, composition wiring, and integration tests.
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` if catalog projection belongs at its push boundary.
- `apps/backend/cmd/plugin-fixture/plugin.go`, `fixture-package/manifest.yaml`, and fixture tests.
- `docs/specs/office/requirements/automation-continuity.md` and the affected Office design authority sections.
- `docs/specs/plugins/requirements/agent-tools.md` and `system-design/agent-tool-config-readiness.md`.
- `docs/decisions/2026-08-22-user-configured-automation-continuity.md` and `2026-11-02-automation-plugin-tool-selection.md`.

## Dependencies

None. Read the design's Policy version, Native save boundary, Discovery and live updates, and Invocation sections.

## Risks

- Do not restore an old policy value after a failed update that leaves changed configuration effective.
- Do not attach `ManagedToolPolicy` to this surface or trust a global catalog as an approved list.
- Do not hold an automation lock during plugin callbacks.
- Do not weaken an existing assertion when the fixture gains another tool.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-plugin-tools.md).
- [System design](../../specs/office/system-design/automation-plugin-tools.md).
- [Decision](../../decisions/2026-11-02-automation-plugin-tool-selection.md).
- Existing `plugin_tools_managed_test.go`, `agent_tools_test.go`, `agent_tools_catalog_order_test.go`, and `service_config_restart_test.go` patterns.

## Results

Implemented November 3, 2026. PostgreSQL execution remains pending.

- Added the shared selection/option/authorization types and exact automation tool enforcement.
- Added the persisted `plugin_tools` column, native-only save intent, options action, stale rejection, export omission, and mode clearing.
- Added `AgentToolPolicyVersion` initialization, load/sideload migration, install rotation, configuration-save rotation, and notification on failed configuration writes.
- Added manifest/snapshot `automation` surface support, filtered discovery, per-call checks, and global-push invalidation with a separate automation catalog.
- Rejected MCP execution/principal contexts at both the shared native dispatcher boundary and the service intent backstop.
- Serialized automation catalog fetch-to-apply with an epoch fence, and projected automation surfaces out of ordinary pull responses as well as global pushes.
- Added backend unit/integration tests and the env-gated PostgreSQL round-trip test (skipped in this environment because `KANDEV_TEST_POSTGRES_DSN` is unset).
- Added fixture read/report tools for downstream browser evidence.
- Backend build, affected package tests, and changed-file Go lint passed.
