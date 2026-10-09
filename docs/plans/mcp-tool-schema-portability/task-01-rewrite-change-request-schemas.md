---
id: "01-rewrite-change-request-schemas"
title: "Rewrite change-request schemas and move constraints to handlers"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
acceptance_criteria:
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.1
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.3
  - AC-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001.4
system_design:
  - ../../specs/integrations/system-design/mcp-tool-schema-portability.md
---

# Task 01: Rewrite change-request schemas and move constraints to handlers

## Summary

Rewrite `taskChangeRequestToolSchema` and `taskChangeRequestAutomationToolSchema`
to the portable subset, removing the root `oneOf`, the root `allOf`, and the
target-object `oneOf`. Move the operation, target, and prompt-exclusivity checks
into the `manage_task_change_request_kandev` and
`update_task_change_request_automation_kandev` handlers so an agent observes the
same rejections.

## In scope

- Keep every portable keyword the two schemas already declare, including root
  `required`, `additionalProperties: false`, enums, and numeric/string/array
  bounds. Remove only the combinator blocks.
- In the manage handler: reject `old_provider`, `old_repository_id`, and
  `old_number` on `link` and `unlink`; require all three on `replace`.
- In the automation handler: for an `association` target require `provider`,
  `repository_id`, and `number` and reject `providers`; for a `task` target
  require a nonempty unique `providers` array and reject `provider`,
  `repository_id`, and `number`; reject `auto_fix_prompt_override` on an
  association target.
- Each rejection returns a tool error before any backend side effect and names
  the violated constraint without returning argument values.
- Update the schema-shape pinning tests to assert the portable shape.

## Out of scope

Provider store, automation algorithm, transport, authorization, or backend action
payload changes. Nested combinators in other tools. Any UI change.

## Acceptance

- Both tool schemas declare no `oneOf`, `allOf`, or `anyOf` at the root or in the
  `target` object.
- The manage and automation handlers reject every invalid combination the old
  schemas rejected, with no mutation, and accept every previously valid payload.
- Error text identifies the violated constraint and never echoes argument values.

## Verification

Add failing tests first, then implement. Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers -count=1)
(cd apps/backend && go test ./internal/mcp/server -run 'TaskChangeRequest|ManageTaskChangeRequest|Automation' -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/task_change_request_tools_test.go`
- `apps/backend/internal/mcp/handlers/task_change_request.go`
- `apps/backend/internal/mcp/handlers/task_change_link.go`
- `apps/backend/internal/mcp/handlers/task_change_request_test.go`

## Dependencies

None. This task must land before Task 02 turns the portable rule into a hard
compile failure.

## Risks

Handler checks must match the dropped schema branches exactly, including
fail-before-mutation and non-echo of values.

## Parallelism

`sequential`. Owns the two shared schema functions and their handlers.

## Inputs

- [Requirements](../../specs/integrations/requirements/mcp-tool-argument-validation.md), portability REQ.
- [System design](../../specs/integrations/system-design/mcp-tool-schema-portability.md), change-request rewrite section.
- [Task change request MCP design](../../specs/integrations/system-design/task-change-link-mcp.md), Schema rules and Management/Automation sections.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Pending implementation.
