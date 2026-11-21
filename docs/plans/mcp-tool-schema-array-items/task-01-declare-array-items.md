---
id: "01-declare-array-items"
title: "Declare items on every built-in array parameter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
acceptance_criteria:
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.5
system_design:
  - ../../specs/integrations/system-design/mcp-tool-schema-portability.md
---

# Task 01: Declare items on every built-in array parameter

## Summary

`reorder_workflow_steps_kandev` declares `step_ids` items as strings, and a
registration test fails whenever any built-in tool on any Kandev-owned MCP
surface advertises an array node without `items`.

## In scope

- `mcp.Items(map[string]any{typeKey: stringType})` on `step_ids` in
  `registerConfigWorkflowStepTools`.
- `profile.CoordinatorToolUniverse()`, returning a copy of the coordinator tool
  universe so the test can enable every coordinator-bound tool.
- `TestRegisteredToolSchemasDeclareArrayItems` in a new file
  `apps/backend/internal/mcp/server/schema_array_items_test.go`.

## Out of scope

- Plugin tool schemas and `toolschema.Compile`.
- Any change to tool names, descriptions, required fields, or handlers.

## Acceptance

1. Every array node in every advertised built-in schema declares `items` on
   each MCP mode, the external endpoint, the automation, coordinator, and
   managed-conversation profiles, and profiles with all capabilities,
   providers, and coordinator bindings enabled.
2. The test fails when a capability-, provider-, or binding-gated tool is not
   registered, and its walker detects a synthetic array without `items`.

## Verification

Write the test first and confirm it fails on `step_ids` before the change.

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  -run TestRegisteredToolSchemasDeclareArrayItems ./internal/mcp/server/)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/mcp/server/ ./internal/mcp/profile/)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/config_handlers.go \
  apps/backend/internal/mcp/server/schema_array_items_test.go \
  apps/backend/internal/mcp/profile/coordinator_policy.go
```

## Likely files

- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/schema_array_items_test.go` (new)
- `apps/backend/internal/mcp/profile/coordinator_policy.go`

## Results

- The test failed before the change on every surface that registers the
  configuration tools: `tool "reorder_workflow_steps_kandev" schema
  inputSchema.properties.step_ids declares type array without items`.
- With the item declaration, `go test ./internal/mcp/server/
  ./internal/mcp/profile/` passes.
