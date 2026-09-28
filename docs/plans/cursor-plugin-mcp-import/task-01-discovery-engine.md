---
id: "01-discovery-engine"
title: "Plugin MCP discovery engine"
status: complete
wave: 2
depends_on: ['00-compatibility-evidence']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.2
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.4
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.5
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.6
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.7
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.8
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.9
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 01: Plugin MCP discovery engine

## Summary

Implement the evidence-backed eligible-root selector and parser with backend-only provenance. Return validated candidates for precedence and policy composition without executing servers or modifying sources.

## In scope

- Implement the design's parser, scoped eligibility, referenced-file containment, root-variable handling, bounded I/O, deterministic precedence, and per-entry failure isolation.
- Use temporary fixture trees only. Cover all supported formats, missing/unreadable/malformed files, invalid sibling entries, equal timestamps, stale versions, disabled plugins, scope mismatch, traversal and symlink escapes, reserved names, unsupported cwd, unknown transports and unresolved variables.
- Write failing tests first in `cursor_plugins_test.go`: `TestDiscoverCursorPluginMCPServers`, with table-driven eligibility/format/error cases; `TestCursorPluginMCPPrecedence`; `TestCursorPluginMCPPathSafety`. Preserve source bytes and assert no secret-bearing diagnostics.

## Out of scope

Plugin installation, marketplace APIs, remote credential forwarding, unrelated CLI strategies, and production changes outside this work order.

## Acceptance

1. Fixture discovery imports only eligible supported entries and returns deterministic winners.
2. Invalid or inaccessible sources cannot fail launch discovery or mask valid siblings; no source file changes or executable side effects occur.
3. Provenance needed by project precedence and reconciliation remains available without entering browser DTOs.

## Verification

Run from the repository root. Implementation work uses TDD for changed logic. Commands below are planned, not executed evidence.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig/...)
```

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/cursor_plugins.go`
- `apps/backend/internal/agent/mcpconfig/cursor_plugins_test.go`

## Dependencies

00-compatibility-evidence.

## Risks

Cursor disk formats are not a public stable API. Unsupported formats must fail closed per source.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/cursor-plugin-mcp-import.md).
- [System design](../../specs/agents/system-design/cursor-plugin-mcp-import.md), especially the sections named by this work order.
- Existing Cursor auth preference and lifecycle tests; ADR 0014.
- [Plan](plan.md) for compatibility, test mapping and remaining evidence gates.

## Results

Implemented `DiscoverCursorPluginMCPServers` in `cursor_plugins.go` and unit tests in `cursor_plugins_test.go`. Discovers stdio and network servers across cached, local, and marketplace plugin directories and user global config with timestamp precedence and reserved name filtering. Tests passed with race detector.
