---
id: "03-launch-materialization"
title: "Launch integration and owned import reconciliation"
status: complete
wave: 4
depends_on: ['01-discovery-engine', '02-profile-contract']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-002
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.5
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.6
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.7
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.9
  - AC-AGENTS-CURSOR-PLUGIN-MCP-002.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.2
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.4
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.5
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.6
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 03: Launch integration and owned import reconciliation

## Summary

Connect the discovery engine to both Cursor preparation paths, applying policy before emission and reconciling owned imports. Use the concrete persistence/crash design established by Task 00, not the generic merge writer unchanged.

## In scope

- Add a shared Cursor-only lifecycle composition helper; gate on strategy, execution profile, executor type, runtime HOME and workspace before discovery. Keep credential sharing independent.
- Preserve explicit profile, project and global/plugin precedence through deduplication and MCP policy enforcement. Preserve explicit profile name reservations even after policy filtering.
- Implement transport serialization proven by Task 00. Own imported-entry reconciliation, restart recovery and generation-safe cleanup, preserving project fields and user edits. Test disabled/removed plugins and unchanged versus edited imported entries.
- TDD in `manager_project_mcp_test.go`, `manager_passthrough_test.go` and new `cursor_plugin_mcp_test.go`: `TestCursorPluginMCPEligibility`, `TestCursorPluginMCPLaunchPaths`, `TestCursorPluginMCPPolicy`, `TestCursorPluginMCPReconciliation`, `TestCursorPluginMCPConcurrentCleanup`, `TestCursorPluginMCPCrashRecovery`. Extend `mcpconfig/passthrough_test.go` for network transport fixtures.
- Cover fresh/resume/restart in both modes, all four auth/import boolean combinations, missing profile/port, inaccessible HOME, remote/container/unknown, custom strategy value/pointer, malformed/symlink destination, policy deny/rewrite, write failure, backend restart, and stale cleanup in both orderings. Coordinate concurrency with barriers, not sleeps.

## Out of scope

Plugin installation, marketplace APIs, remote credential forwarding, unrelated CLI strategies, and production changes outside this work order.

## Acceptance

1. Both entry points materialize the same eligible, policy-filtered winners before launch without reading host files for ineligible executions.
2. Disable/removal/restart remove only unchanged owned imports; edited project data survives, and old cleanup cannot delete newer configuration.
3. Secret-bearing inputs remain out of logs/DTOs/metadata; unsafe stale cleanup prevents launch with a sanitized error.

## Verification

Run from the repository root. Implementation work uses TDD for changed logic. Commands below are planned, not executed evidence.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle/... -run 'TestCursorPluginMCP')
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `apps/backend/internal/agent/mcpconfig/passthrough.go`
- `apps/backend/internal/agent/mcpconfig/passthrough_test.go`

## Dependencies

01-discovery-engine, 02-profile-contract.

## Risks

Existing merge behavior is additive, and whole-file cleanup can erase successor configuration. Task 00 must name ownership storage files before this task starts.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/cursor-plugin-mcp-import.md).
- [System design](../../specs/agents/system-design/cursor-plugin-mcp-import.md), especially the sections named by this work order.
- Existing Cursor auth preference and lifecycle tests; ADR 0014.
- [Plan](plan.md) for compatibility, test mapping and remaining evidence gates.

## Results

Implemented `reconcileAndMaterializeCursorProjectMCP` in `cursor_plugin_mcp.go`, integrated across `manager_project_mcp.go` and `manager_passthrough.go`, and created `cursor_plugin_mcp_test.go`. Features policy enforcement on imported candidates, explicit profile and reserved name protection, project file precedence (preserving user project servers over global/plugin imports), and persistent backend-private import ownership tracking (`.cursor/.kandev-mcp-imports.json`). Stale unmodified imported servers are safely removed on disablement or plugin deletion, while user edits survive. All unit and race tests passed (`go test -race -v ./internal/agent/runtime/lifecycle/ -run "TestCursorPlugin"`).
