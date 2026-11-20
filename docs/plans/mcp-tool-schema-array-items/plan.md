---
created: 2026-11-19
status: done
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
system_design:
  - ../../specs/integrations/system-design/mcp-tool-schema-portability.md
legacy_specs: []
---

# Implementation plan: Array item declarations in MCP tool schemas

## Overview

An agent whose model provider validates function parameters strictly (Gemini,
for example) fails every request in a Kandev session that exposes the
configuration tools: `reorder_workflow_steps_kandev` advertises `step_ids` as
`type: array` without `items`, and the provider rejects the whole tool list.
Kandev's own schema compiler accepts the omission, so nothing caught it. This
package declares the item schema and adds a guard over every Kandev-owned MCP
surface, as defined by `AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.5` and
the "Array item declarations" section of the system design.

## Scope

In scope: the `step_ids` item declaration, a read-only accessor for the
coordinator tool universe, and a registration test that walks every advertised
built-in schema.

Out of scope: plugin manifest schema admission, schema compiler changes, tool
availability, and handler behavior.

## Work orders

- [x] [Task 01: Declare items on every built-in array parameter](task-01-declare-array-items.md)

One work order: one registration fix plus its guard, with one verification
boundary.

## Verification strategy

Backend unit tests in `apps/backend/internal/mcp/server` and
`apps/backend/internal/mcp/profile`. No end-to-end test: the outcome is a schema
shape that the registration test asserts directly for every surface, and E2E has
no strict-provider agent.

## Risks

- A new built-in array parameter without `items` fails the guard. That is the
  intended outcome.
- A surface or capability group that stops registering its tools would hide
  schemas from the walk; the test pins a list of gated tool names to catch it.

## References

- [Requirement](../../specs/integrations/requirements/mcp-tool-argument-validation.md)
- [System design](../../specs/integrations/system-design/mcp-tool-schema-portability.md)
- [Portable MCP tool schemas package](../mcp-tool-schema-portability/plan.md)
