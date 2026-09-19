---
id: "01-enforce-registered-schemas"
title: "Enforce registered schemas at the MCP boundary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/mcp-tool-argument-validation.md"
---

# Task 01: Enforce registered schemas at the MCP boundary

## Acceptance

- Every active built-in MCP tool has a compiled validator derived from its registered schema, with unknown root arguments rejected and intentional nested maps preserved.
- Missing required fields, wrong types, declared constraint violations, and unknown top-level keys return sanitized MCP tool errors before handler invocation.
- Initial registration and `SetMode` replacement both install the matching validator set; omitted arguments work only for schemas that accept an empty object.

## Verification

Follow strict TDD, then run:

```bash
make -C apps/backend test
```

## Files likely touched

- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/server/tool_argument_validation_test.go`

## Dependencies

None.

## Parallelism

`sequential` — this task owns the shared registration and dispatch boundary plus the dependency files.

## Inputs

- Spec scenarios except the create-task compatibility scenarios.
- `ADR-2026-08-01-validate-mcp-tool-arguments`.
- `Server.wrapHandler`, `registerTools`, `SetMode`, and the typed/raw schema fields on `mcp.Tool`.
- The v6.0.2 compiler and `Schema.Validate` contract from `github.com/santhosh-tekuri/jsonschema/v6`.

## Risks

- Do not close nested arbitrary-key maps.
- Do not return full argument values in validation errors or logs added by this task.
- A schema compilation defect must fail closed and remain observable without changing constructor signatures.

## Output contract

Report red/green evidence, dependency and files changed, exact test results, schema mismatches discovered, residual risks, and task/plan status updates in the primary conversation.
