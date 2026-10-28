---
id: "01-inventory-discovery"
title: "Discover and resolve installed Copilot plugins"
status: completed
wave: 2
depends_on: ["00-native-compatibility"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-001.2
  - AC-AGENTS-COPILOT-PLUGINS-001.3
  - AC-AGENTS-COPILOT-PLUGINS-001.4
  - AC-AGENTS-COPILOT-PLUGINS-001.5
  - AC-AGENTS-COPILOT-PLUGINS-001.6
  - AC-AGENTS-COPILOT-PLUGINS-003.1
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
  - AC-AGENTS-COPILOT-PLUGINS-003.4
  - AC-AGENTS-COPILOT-PLUGINS-003.5
  - AC-AGENTS-COPILOT-PLUGINS-003.6
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-007.1
  - AC-AGENTS-COPILOT-PLUGINS-007.4
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 01: Discover and resolve installed plugins

## Summary

Deliver a safe, profile-aware inventory endpoint backed by the selected Copilot CLI.
Return verified availability metadata while keeping installation roots private.

## In scope

- Add `internal/agent/copilotplugins` with stable IDs, injectable runner, strict parsers, qualified JSONC adapter, and root/manifest validation.
- Use a pinned maintained JSONC parser instead of regex stripping. Keep its dependency update inside this work order.
- Implement the command/output/row/file bounds and cancellation/process-tree cleanup in the design.
- Preserve native-disabled installed rows. Distinguish true empty from every failed/unsupported result.
- Support qualified cached/direct/live roots and explicit unavailable reasons for built-ins, unknown sources, conflicts, and unsafe paths.
- Add `POST /api/v1/agents/:id/plugin-discovery`, request/response DTOs, controller wiring, and settings authority guards.
- Reuse profile context and secret resolution. Add bounded successful-result caching and runtime/refresh fencing without retaining roots or secrets.
- Forward only qualified config-directory flags to the inventory command. Prove that profile prompt/ACP/plugin flags cannot alter its operation.
- Keep discovery read-only. Command spies must reject accidental enable/disable/install/update operations.

## Out of scope

Profile persistence, browser UI, ACP mounts, Cursor adapters, native credential operations, or support not qualified by WO-00.

## Acceptance

1. Authorized saved/new-draft contexts produce safe inventory through the selected package, with cancellation and bounded output on every exit path.
2. Identity/root fixtures cover marketplace collisions, upgrades, JSONC, external live roots, direct installs, built-ins, malformed state, and unsafe paths.
3. Authority, secret privacy, stale-response fencing, failure-versus-empty semantics, and no-mutation behavior pass hermetic tests without Copilot installed.

## Verification

Start with failing tests, then implement the smallest adapter and endpoint.
Run from the repository root. Add every changed test to the command set before recording results.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins -run 'TestInventory|TestInstallationMetadata|TestPluginRootValidation|TestPluginMetadataPrivacy|TestPluginReadOnlyOperations' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/settings/controller ./internal/agent/settings/handlers -run TestCopilotPluginDiscovery -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/copilotplugins ./internal/agent/settings/controller -run 'TestInventoryContextFence|TestCopilotPluginDiscoveryContext' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the explicit changed Go-file list after tests.
If a neutral runner is extracted, rerun its existing Cursor native-runner tests too.
Do not mark a format supported from a synthetic fixture alone. Its runtime mapping must match WO-00 evidence.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/` (new `inventory`, `installation`, `runner`, `context`, and focused test files).
- `apps/backend/internal/agent/settings/controller/copilot_plugin_discovery.go` and `_test.go` (new).
- `apps/backend/internal/agent/settings/controller/profile_discovery.go` and controller dependency wiring.
- `apps/backend/internal/agent/settings/dto/` (discovery DTOs only).
- `apps/backend/internal/agent/settings/handlers/handlers.go` and `copilot_plugin_discovery_handlers.go`/tests (new).
- Agent settings/hostutility provider wiring where selected runtime identity is supplied.
- `apps/backend/go.mod`, `go.sum` for the pinned JSONC dependency.

## Dependencies

WO-00 must define the qualified runtime/source/metadata mappings and native policy boundary.

## Risks

Config files contain authentication alongside installation state. Decode and project only required fields.
Legacy envelope errors and child processes retaining pipes must never become an empty-success response.
Request caching must include the effective config directory, principal, runtime, and draft generation.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#discovery-api), identity, installation, context, and execution-safety sections.
- `controller/profile_discovery.go`, `handlers/profile_discovery_handlers.go`, and `hostutility/profile_probe.go`.
- `mcpconfig/cursor_native_mcp.go` and its runner tests as safety references only.

## Results

Completed 2026-10-27.

- `go test -trimpath ./internal/agent/copilotplugins -count=1`: passed.
- `go test -trimpath ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'TestCopilotPluginDiscovery' -count=1`: passed.
- Added `POST /api/v1/agents/:id/plugin-discovery`, safe DTOs, authority guards, context fencing, 30-second safe cache, and the HuJSON installation adapter.
- Native policy observation remains limited as recorded in WO-00.
