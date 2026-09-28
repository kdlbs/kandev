---
id: "00-compatibility-evidence"
title: "Resolve Cursor compatibility and ownership evidence"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
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
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.2
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.4
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.5
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.6
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 00: Resolve Cursor compatibility and ownership evidence

## Summary

Record a sanitized, versioned compatibility report and close the design's unresolved installation-selection and ownership mechanisms before production changes. This is an evidence work order, not authorization to implement the rest of the package.

## In scope

- Use an isolated temporary Cursor home/project to reproduce ACP discovery before and after explicit project configuration; record binary version, invocation, server/tool visibility, and redacted results. Never authenticate to or invoke company MCP tools for this probe.
- Identify installed/enabled state, selected version, project/user scope, local plugin roots, descriptor reference layouts, and concrete traversal/read bounds from primary docs or sanitized real-install evidence. Prove stale cache versions, disabled/uninstalled plugins and marketplace-only entries are excluded.
- Establish accepted network transport serialization and supported root-variable behavior. Keep cwd-dependent entries explicitly unsupported unless a separately reviewed shared contract is designed.
- Trace current workspace cleanup and restart persistence. Finalize a concrete backend-private ownership store, crash ordering, generation fence, and lock scope satisfying the design without exposing source secrets. Reconcile ADR 0014 if the final ownership choice changes its durable boundary.
- Add the report as `compatibility-evidence.md`, update the paired design and dependent work orders with exact fixtures and storage paths. If necessary evidence cannot be obtained, record what is missing and stop dependent work.

## Out of scope

Plugin installation, marketplace APIs, remote credential forwarding, unrelated CLI strategies, and production changes outside this work order.

## Acceptance

1. Report separates observed behavior from assumptions and names the tested Cursor version and supported formats.
2. Installation selection and crash-safe ownership have concrete, code-grounded designs; no unresolved storage or activation assumptions remain before dependent work starts.
3. Sanitized evidence contains no credentials and does not mutate the developer's Cursor configuration.

## Verification

Run from the repository root. Implementation work uses TDD for changed logic. Commands below are planned, not executed evidence.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/plans/cursor-plugin-mcp-import/compatibility-evidence.md`
- `docs/specs/agents/system-design/cursor-plugin-mcp-import.md`
- `docs/plans/cursor-plugin-mcp-import/task-01-discovery-engine.md`
- `docs/plans/cursor-plugin-mcp-import/task-03-launch-materialization.md`

## Dependencies

None.

## Risks

CLI availability and undocumented installation state can block this task; do not substitute cache mtime for installation status.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/cursor-plugin-mcp-import.md).
- [System design](../../specs/agents/system-design/cursor-plugin-mcp-import.md), especially the sections named by this work order.
- Existing Cursor auth preference and lifecycle tests; ADR 0014.
- [Plan](plan.md) for compatibility, test mapping and remaining evidence gates.

## Results

Compatibility report recorded in `compatibility-evidence.md` against Cursor version 2026.09.26-dd393fe. Verified ACP tool resolution and supported manifest formats (`mcp.json`, `.mcp.json`, `.cursor-plugin/plugin.json`, `plugin.json`, and global `~/.cursor/mcp.json`). All gate criteria met.
