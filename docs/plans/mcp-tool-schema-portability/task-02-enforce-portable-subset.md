---
id: "02-enforce-portable-subset"
title: "Enforce the portable subset at the compile seam"
status: pending
wave: 2
depends_on:
  - "01-rewrite-change-request-schemas"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
acceptance_criteria:
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.1
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.2
system_design:
  - ../../specs/integrations/system-design/mcp-tool-schema-portability.md
---

# Task 02: Enforce the portable subset at the compile seam

## Summary

Reject a root `oneOf`, `allOf`, or `anyOf` at the shared schema compile seam so a
violating schema fails closed at registration for both plugin and built-in tools,
and a built-in regression fails the all-modes registration test.

## In scope

- Add the root-combinator rejection to `toolschema.Compile`, next to its existing
  empty-document and object-root checks, so `validatePluginToolSnapshot` rejects a
  violating plugin tool before it is advertised and plugin invocation compilation
  fails closed.
- Apply the same rejection in `compileToolArgumentSchema` before it compiles a
  built-in tool validator, so `rebuildToolArgumentValidators` records a compile
  error and `validateToolArguments` returns `registered schema is invalid` for a
  call to that tool.
- Extend `TestAllRegisteredToolSchemasCompile` (or add an adjacent test) to assert
  that no advertised built-in schema declares a root combinator in any mode.
- Add a `toolschema` unit test that rejects each of `oneOf`, `allOf`, and `anyOf`
  at the root and accepts nested combinators.

## Out of scope

Rewriting nested combinators (for example `show_rich_output_kandev.blocks.items`).
Changing tool availability, transport, or handler logic. Any schema rewrite owned
by Task 01.

## Acceptance

- A schema with a root `oneOf`/`allOf`/`anyOf` is rejected by `toolschema.Compile`
  and by `compileToolArgumentSchema`; a nested combinator still compiles.
- A plugin snapshot carrying a root-combinator tool is rejected by
  `SetPluginTools` and the previous snapshot stays advertised.
- The registration test fails if any built-in schema reintroduces a root
  combinator, and passes on the current catalog after Task 01.

## Verification

Add failing tests first, then implement. Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/toolschema ./internal/mcp/server -count=1)
(cd apps/backend && go test ./internal/mcp/server -run 'TestAllRegisteredToolSchemasCompile|PluginTool' -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/toolschema/schema.go`
- `apps/backend/internal/mcp/toolschema/schema_test.go`
- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/server/tool_argument_validation_test.go`

## Dependencies

Task 01. The two change-request schemas must already be portable, or this task's
registration test fails on them.

## Risks

Another built-in schema may already use a root combinator; the registration test
run reveals it, and it must be rewritten under the same rule before merge.

## Parallelism

`sequential`. Owns the shared compile seam that both tool families use.

## Inputs

- [Requirements](../../specs/integrations/requirements/mcp-tool-argument-validation.md), portability REQ.
- [System design](../../specs/integrations/system-design/mcp-tool-schema-portability.md), enforcement seam section.
- Existing `toolschema.Compile`, `compileToolArgumentSchema`, and
  `TestAllRegisteredToolSchemasCompile`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Pending implementation.
